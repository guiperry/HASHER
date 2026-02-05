// cmd/driver/hasher-server/main.go
// Hasher Server - runs on ASIC device and exposes gRPC service for hash computations
package main

import (
	"flag"
	"fmt"
	"log"
	"net"
	"os"
	"os/exec"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/reflection"

	"hasher/internal/driver/device"
	pb "hasher/internal/proto/hasher/v1"
)

var (
	port          = flag.Int("port", 8888, "gRPC server port")
	enableTracing = flag.Bool("trace", true, "enable eBPF tracing")
	enableTLS     = flag.Bool("tls", false, "enable TLS")
	certFile      = flag.String("cert", "", "TLS certificate file")
	keyFile       = flag.String("key", "", "TLS key file")
	autoReboot    = flag.Bool("auto-reboot", true, "automatically reboot on fatal device errors")
)

// attemptDeviceRecovery attempts to open the ASIC device with all available strategies.
// If all strategies fail and auto-reboot is enabled, it triggers a system reboot.
func attemptDeviceRecovery(enableTracing bool) (*device.Device, error) {
	log.Printf("Attempting device recovery with multiple strategies...")

	// Try to open device directly first
	dev, err := device.OpenDevice(enableTracing)
	if err == nil {
		log.Printf("Device opened successfully via OpenDevice")
		return dev, nil
	}

	log.Printf("Initial device open failed: %v", err)

	// Check if the error is recoverable (device busy/locked)
	if !isRecoverableError(err) {
		return nil, fmt.Errorf("non-recoverable device error: %w", err)
	}

	// Strategy 1: Wait and retry with exponential backoff
	log.Printf("Strategy 1: Waiting and retrying with exponential backoff...")
	backoff := 500 * time.Millisecond
	maxBackoff := 5 * time.Second
	for attempt := 0; attempt < 5; attempt++ {
		time.Sleep(backoff)
		dev, err = device.OpenDevice(enableTracing)
		if err == nil {
			log.Printf("Device opened successfully after backoff (attempt %d)", attempt+1)
			return dev, nil
		}
		backoff *= 2
		if backoff > maxBackoff {
			backoff = maxBackoff
		}
	}
	log.Printf("Backoff retry strategy failed after 5 attempts")

	// Strategy 2: Try to unload kernel module and retry
	log.Printf("Strategy 2: Attempting kernel module unload...")
	if err := unloadKernelModule(); err == nil {
		time.Sleep(2 * time.Second) // Wait for device release
		dev, err = device.OpenDevice(enableTracing)
		if err == nil {
			log.Printf("Device opened successfully after kernel module unload")
			return dev, nil
		}
		// Try to reload module for next strategies
		reloadKernelModule()
	} else {
		log.Printf("Kernel module unload failed: %v", err)
	}

	// Strategy 3: Check for competing processes and kill them
	log.Printf("Strategy 3: Checking for competing processes...")
	killCompetingProcesses()
	time.Sleep(1 * time.Second)
	dev, err = device.OpenDevice(enableTracing)
	if err == nil {
		log.Printf("Device opened successfully after killing competing processes")
		return dev, nil
	}
	log.Printf("Process cleanup strategy failed")

	// All strategies failed - trigger reboot if enabled
	if *autoReboot {
		log.Printf("AUTO_REBOOT_TRIGGERED: attempting system reboot to clear kernel module lock")
		log.Printf("All device recovery strategies failed. Initiating system reboot...")

		// Give time for logs to flush
		time.Sleep(1 * time.Second)

		// Sync filesystem before reboot
		syscall.Sync()

		// Trigger forced reboot
		cmd := exec.Command("reboot", "-f")
		output, cmdErr := cmd.CombinedOutput()
		if cmdErr != nil {
			// If forced reboot fails, try regular reboot
			log.Printf("Forced reboot failed (%v: %s), trying regular reboot...", cmdErr, string(output))
			cmd = exec.Command("reboot")
			output, cmdErr = cmd.CombinedOutput()
			if cmdErr != nil {
				return nil, fmt.Errorf("all recovery strategies failed and reboot failed: %w (output: %s)", cmdErr, string(output))
			}
		}

		// Reboot command succeeded, but we should never reach here
		// Give the system time to reboot
		time.Sleep(30 * time.Second)
		return nil, fmt.Errorf("reboot command executed but process still running")
	}

	return nil, fmt.Errorf("all recovery strategies failed and auto-reboot is disabled: %w", err)
}

// isRecoverableError checks if the error is recoverable through retry/reboot
func isRecoverableError(err error) bool {
	if err == nil {
		return false
	}
	errStr := err.Error()
	return strings.Contains(errStr, "device or resource busy") ||
		strings.Contains(errStr, "Device or resource busy") ||
		strings.Contains(errStr, "operation not permitted") ||
		strings.Contains(errStr, "Operation not permitted") ||
		strings.Contains(errStr, "permission denied") ||
		strings.Contains(errStr, "Permission denied") ||
		strings.Contains(errStr, "resource temporarily unavailable") ||
		strings.Contains(errStr, "Resource temporarily unavailable") ||
		strings.Contains(errStr, "device not found") ||
		strings.Contains(errStr, "no such device") ||
		strings.Contains(errStr, "input/output error") ||
		strings.Contains(errStr, "I/O error")
}

// unloadKernelModule attempts to unload the bitmain_asic kernel module
func unloadKernelModule() error {
	log.Printf("Checking if kernel module is loaded...")

	cmd := exec.Command("lsmod")
	output, err := cmd.Output()
	if err != nil {
		return fmt.Errorf("failed to list modules: %w", err)
	}

	if !strings.Contains(string(output), "bitmain_asic") {
		log.Printf("Kernel module bitmain_asic not loaded")
		return nil
	}

	log.Printf("Unloading bitmain_asic kernel module...")

	// Try graceful removal first
	cmd = exec.Command("rmmod", "bitmain_asic")
	output, err = cmd.CombinedOutput()
	if err == nil {
		log.Printf("Successfully unloaded kernel module")
		return nil
	}

	// Try forced removal
	log.Printf("Graceful unload failed, trying forced removal...")
	cmd = exec.Command("rmmod", "-f", "bitmain_asic")
	output, err = cmd.CombinedOutput()
	if err == nil {
		log.Printf("Successfully forced unload of kernel module")
		return nil
	}

	return fmt.Errorf("failed to unload kernel module: %v (output: %s)", err, string(output))
}

// reloadKernelModule attempts to reload the bitmain_asic kernel module
func reloadKernelModule() error {
	log.Printf("Reloading bitmain_asic kernel module...")

	// Try modprobe first
	cmd := exec.Command("modprobe", "bitmain_asic")
	output, err := cmd.CombinedOutput()
	if err == nil {
		log.Printf("Successfully reloaded kernel module via modprobe")
		return nil
	}

	// Try insmod with common path
	cmd = exec.Command("insmod", "/lib/modules/$(uname -r)/kernel/drivers/usb/bitmain_asic.ko")
	output, err = cmd.CombinedOutput()
	if err == nil {
		log.Printf("Successfully reloaded kernel module via insmod")
		return nil
	}

	return fmt.Errorf("failed to reload kernel module: %v (output: %s)", err, string(output))
}

// killCompetingProcesses kills any processes that might be using the device
func killCompetingProcesses() {
	log.Printf("Killing competing processes...")

	processes := []string{"cgminer", "bmminer", "hasher-server", "bitmain-asic"}

	for _, proc := range processes {
		// Try graceful kill first
		exec.Command("pkill", "-15", proc).Run()
	}

	time.Sleep(500 * time.Millisecond)

	for _, proc := range processes {
		// Force kill if still running
		exec.Command("pkill", "-9", proc).Run()
		exec.Command("killall", "-9", proc).Run()
	}

	log.Printf("Competing processes terminated")
}

func configureFirewall(port int) error {
	// Check if iptables is available
	if _, err := exec.LookPath("iptables"); err != nil {
		log.Printf("iptables not found, skipping firewall configuration")
		return nil
	}

	// Check if rule already exists
	cmd := exec.Command("iptables", "-L", "INPUT", "-n")
	output, err := cmd.Output()
	if err == nil && len(output) > 0 {
		// Check if port is already open
		rule := fmt.Sprintf("dpt:%d", port)
		if len(output) > 0 {
			for _, line := range strings.Split(string(output), "\n") {
				if strings.Contains(line, rule) && strings.Contains(line, "ACCEPT") {
					log.Printf("Firewall rule for port %d already exists", port)
					return nil
				}
			}
		}
	}

	// Add rule to accept incoming connections on the port
	cmd = exec.Command("iptables", "-I", "INPUT", "-p", "tcp", "--dport", fmt.Sprintf("%d", port), "-j", "ACCEPT")
	if output, err := cmd.CombinedOutput(); err != nil {
		// Don't fail if we can't configure firewall - might not have permissions
		log.Printf("Warning: Failed to configure firewall rule: %v (output: %s)", err, output)
		return nil
	}

	log.Printf("Configured firewall to accept connections on port %d", port)
	return nil
}

func main() {
	flag.Parse()

	log.Printf("Hasher Server starting on port %d...", *port)
	log.Printf("eBPF tracing: %v", *enableTracing)
	log.Printf("Auto-reboot: %v", *autoReboot)

	// Configure firewall to allow incoming connections
	if err := configureFirewall(*port); err != nil {
		log.Printf("Warning: Firewall configuration failed: %v", err)
	}

	// Create gRPC server
	var opts []grpc.ServerOption

	if *enableTLS {
		if *certFile == "" || *keyFile == "" {
			log.Fatal("TLS enabled but cert/key files not provided")
		}
		// Add TLS credentials here
		// creds, err := credentials.NewServerTLSFromFile(*certFile, *keyFile)
		// opts = append(opts, grpc.Creds(creds))
	}

	grpcServer := grpc.NewServer(opts...)

	// Create Hasher server using device package with recovery
	hasherServer, err := NewHasherServerWithRecovery(*enableTracing)
	if err != nil {
		// If we get here and auto-reboot was enabled, the reboot failed
		log.Fatalf("Failed to create Hasher server: %v", err)
	}
	defer hasherServer.Close()

	// Register service
	pb.RegisterHasherServiceServer(grpcServer, hasherServer)

	// Enable reflection for debugging
	reflection.Register(grpcServer)

	// Listen on TCP port (bind to all interfaces to accept remote connections)
	addr := fmt.Sprintf("0.0.0.0:%d", *port)
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		log.Fatalf("Failed to listen: %v", err)
	}

	log.Printf("Hasher gRPC server starting on %s", addr)
	log.Printf("eBPF tracing: %v", *enableTracing)

	// Handle graceful shutdown
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)

	go func() {
		<-sigCh
		log.Println("Shutting down server...")
		grpcServer.GracefulStop()
	}()

	// Start serving
	if err := grpcServer.Serve(listener); err != nil {
		log.Fatalf("Failed to serve: %v", err)
	}
}

// NewHasherServerWithRecovery creates a new Hasher gRPC server with automatic device recovery
func NewHasherServerWithRecovery(enableTracing bool) (*device.HasherServer, error) {
	dev, err := attemptDeviceRecovery(enableTracing)
	if err != nil {
		return nil, fmt.Errorf("device recovery failed: %w", err)
	}

	return device.NewHasherServerWithDevice(dev), nil
}
