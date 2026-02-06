// Hasher: Neural Inference Engine Powered by SHA-256 ASICs
// Copyright (C) 2026  Guillermo Perry
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
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

// ensureCGMinerRunning checks if CGMiner is running and restarts it if needed
func ensureCGMinerRunning() error {
	log.Printf("Checking CGMiner status...")

	// Check if CGMiner process is running
	cmd := exec.Command("sh", "-c", "ps | grep cgminer | grep -v grep")
	if output, err := cmd.Output(); err == nil && len(output) > 0 {
		log.Printf("CGMiner process found, checking API...")

		// Check if API is responding
		if cmd := exec.Command("sh", "-c", "echo '{\"command\":\"version\"}' | nc 127.0.0.1 4028"); cmd.Run() == nil {
			log.Printf("✓ CGMiner is running and API is responding")
			return nil
		}

		log.Printf("CGMiner running but API not responding, will restart...")
	} else {
		log.Printf("CGMiner not found, starting...")
	}

	// Kill any existing CGMiner processes
	exec.Command("sh", "-c", "killall cgminer 2>/dev/null").Run()
	time.Sleep(2 * time.Second)

	// Start CGMiner with proper options
	startCmd := exec.Command("sh", "-c", "cgminer --api-allow W:127.0.0.1 --api-listen --bitmain-options 115200:32:8:16:250:0982 --benchmark -o http://dummy.pool:8080 -u dummyuser -p dummypass > /tmp/cgminer.log 2>&1 &")
	if err := startCmd.Run(); err != nil {
		return fmt.Errorf("failed to start CGMiner: %w", err)
	}

	// Wait for CGMiner to initialize and API to become available
	for i := 0; i < 10; i++ {
		time.Sleep(2 * time.Second)
		if cmd := exec.Command("sh", "-c", "echo '{\"command\":\"version\"}' | nc 127.0.0.1 4028"); cmd.Run() == nil {
			log.Printf("✓ CGMiner API is responding after restart")
			return nil
		}
		log.Printf("Waiting for CGMiner API... (attempt %d/10)", i+1)
	}

	return fmt.Errorf("CGMiner failed to start or API not responding after 20 seconds")
}

// attemptDeviceRecovery attempts to open the ASIC device with CGMiner priority.
// It ensures CGMiner is running before attempting any device access.
func attemptDeviceRecovery(enableTracing bool) (*device.Device, error) {
	log.Printf("Attempting device recovery with CGMiner-first strategy...")

	// Primary Strategy: Ensure CGMiner is running and use it
	log.Printf("Primary Strategy: Ensuring CGMiner is available...")
	cgminerErr := ensureCGMinerRunning()
	if cgminerErr == nil {
		log.Printf("✓ CGMiner is available, attempting to open device...")
		// Try to open device (CGMiner should be detected and prioritized)
		dev, err := device.OpenDevice(enableTracing)
		if err == nil {
			log.Printf("✓ Device opened successfully with CGMiner mode")
			return dev, nil
		}
		log.Printf("Failed to open device even with CGMiner running: %v", err)
		return nil, fmt.Errorf("CGMiner available but device open failed: %w", err)
	}

	// If CGMiner setup failed, log and return error
	return nil, fmt.Errorf("failed to ensure CGMiner availability: %w", cgminerErr)
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
