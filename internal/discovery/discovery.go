// internal/hasher/discovery.go
package discovery

import (
	"context"
	"fmt"
	"net"
	"strings"
	"sync"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	pb "hasher/internal/proto/hasher/v1"
	"hasher/pkg/hashing/methods/asic"
)

// DiscoveryResult contains information about a discovered hasher-server
type DiscoveryResult struct {
	Address    string `json:"address"`
	IPAddress  string `json:"ip_address"`
	Port       int    `json:"port"`
	ChipCount  int    `json:"chip_count"`
	Version    string `json:"firmware_version"`
	LatencyMs  int64  `json:"latency_ms"`
	Responding bool   `json:"responding"`
	Error      string `json:"error,omitempty"`
}

// DiscoveryConfig holds configuration for network discovery
type DiscoveryConfig struct {
	Subnet          string        `json:"subnet"`           // CIDR notation, e.g., "192.168.1.0/24"
	Port            int           `json:"port"`             // gRPC port to scan (default: 8888)
	Timeout         time.Duration `json:"timeout"`          // Connection timeout per host
	ConcurrentScans int           `json:"concurrent_scans"` // Number of concurrent workers
	SkipLocalhost   bool          `json:"skip_localhost"`   // Skip localhost scanning
}

// NewDiscoveryConfig creates a default discovery configuration
func NewDiscoveryConfig() DiscoveryConfig {
	return DiscoveryConfig{
		Subnet:          "192.168.12.0/24", // Antminer network
		Port:            8888,
		Timeout:         2 * time.Second,
		ConcurrentScans: 20,
		SkipLocalhost:   false,
	}
}

// DiscoverServers scans the network for hasher-server instances
func DiscoverServers(config DiscoveryConfig) ([]DiscoveryResult, error) {
	// Get local network to scan if subnet not specified
	if config.Subnet == "" {
		subnet, err := getLocalSubnet()
		if err != nil {
			return nil, fmt.Errorf("failed to determine local subnet: %w", err)
		}
		config.Subnet = subnet
	}

	ip, ipnet, err := net.ParseCIDR(config.Subnet)
	if err != nil {
		return nil, fmt.Errorf("invalid subnet %s: %w", config.Subnet, err)
	}

	var wg sync.WaitGroup
	semaphore := make(chan struct{}, config.ConcurrentScans)
	results := make(chan DiscoveryResult, 100)
	var mu sync.Mutex
	var discoveries []DiscoveryResult

	// Generate list of IPs to scan
	var ips []string
	for ip := ip.Mask(ipnet.Mask); ipnet.Contains(ip); incrementIP(ip) {
		ips = append(ips, ip.String())
	}

	// Add localhost first if not skipped
	if !config.SkipLocalhost {
		localhostAddr := fmt.Sprintf("localhost:%d", config.Port)
		wg.Add(1)
		go func() {
			defer wg.Done()
			result := probeServer(localhostAddr, "127.0.0.1", config.Port, config.Timeout)
			results <- result
		}()
	}

	// Scan network IPs
	for _, ipStr := range ips {
		// Skip if this is our own IP
		if isLocalIP(ipStr) {
			continue
		}

		wg.Add(1)
		semaphore <- struct{}{}

		go func(ip string) {
			defer wg.Done()
			defer func() { <-semaphore }()

			address := fmt.Sprintf("%s:%d", ip, config.Port)
			result := probeServer(address, ip, config.Port, config.Timeout)
			results <- result
		}(ipStr)
	}

	// Wait for all scans to complete
	go func() {
		wg.Wait()
		close(results)
	}()

	// Collect results
	for result := range results {
		mu.Lock()
		discoveries = append(discoveries, result)
		mu.Unlock()
	}

	return discoveries, nil
}

// probeServer attempts to connect to a hasher-server and get its info
func probeServer(address, ipAddress string, port int, timeout time.Duration) DiscoveryResult {
	start := time.Now()
	result := DiscoveryResult{
		Address:    address,
		IPAddress:  ipAddress,
		Port:       port,
		Responding: false,
	}

	// Try to connect with timeout
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	conn, err := grpc.DialContext(ctx, address,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithBlock(),
	)
	if err != nil {
		result.Error = fmt.Sprintf("Connection failed: %v", err)
		result.LatencyMs = time.Since(start).Milliseconds()
		return result
	}
	defer conn.Close()

	// Try to get device info
	client := pb.NewHasherServiceClient(conn)
	infoCtx, infoCancel := context.WithTimeout(context.Background(), timeout/2)
	defer infoCancel()

	info, err := client.GetDeviceInfo(infoCtx, &pb.GetDeviceInfoRequest{})
	if err != nil {
		result.Error = fmt.Sprintf("Device info failed: %v", err)
		result.LatencyMs = time.Since(start).Milliseconds()
		return result
	}

	result.Responding = true
	result.ChipCount = int(info.ChipCount)
	result.Version = info.FirmwareVersion
	result.LatencyMs = time.Since(start).Milliseconds()

	return result
}

// getLocalSubnet attempts to determine the local network subnet
func getLocalSubnet() (string, error) {
	// Get all network interfaces
	interfaces, err := net.Interfaces()
	if err != nil {
		return "", err
	}

	// Look for IPv4 interfaces that are up and not loopback
	for _, iface := range interfaces {
		if iface.Flags&net.FlagUp == 0 || iface.Flags&net.FlagLoopback != 0 {
			continue
		}

		addrs, err := iface.Addrs()
		if err != nil {
			continue
		}

		for _, addr := range addrs {
			var ip net.IP
			switch v := addr.(type) {
			case *net.IPNet:
				ip = v.IP
			case *net.IPAddr:
				ip = v.IP
			}

			if ip == nil || ip.To4() == nil {
				continue
			}

			// Convert to subnet (assume /24 for home networks)
			parts := strings.Split(ip.String(), ".")
			if len(parts) == 4 {
				return fmt.Sprintf("%s.%s.%s.0/24", parts[0], parts[1], parts[2]), nil
			}
		}
	}

	return "", fmt.Errorf("no suitable network interface found")
}

// incrementIP increments an IP address
func incrementIP(ip net.IP) {
	for j := len(ip) - 1; j >= 0; j-- {
		ip[j]++
		if ip[j] > 0 {
			break
		}
	}
}

// isLocalIP checks if an IP address is local
func isLocalIP(ipStr string) bool {
	ip := net.ParseIP(ipStr)
	if ip == nil {
		return false
	}

	// Check if it's loopback
	if ip.IsLoopback() {
		return true
	}

	// Get local interfaces and check if any match
	interfaces, err := net.Interfaces()
	if err != nil {
		return false
	}

	for _, iface := range interfaces {
		addrs, err := iface.Addrs()
		if err != nil {
			continue
		}

		for _, addr := range addrs {
			var ifaceIP net.IP
			switch v := addr.(type) {
			case *net.IPNet:
				ifaceIP = v.IP
			case *net.IPAddr:
				ifaceIP = v.IP
			}

			if ifaceIP != nil && ifaceIP.Equal(ip) {
				return true
			}
		}
	}

	return false
}

// FindBestServer selects the best hasher-server from discovered results
func FindBestServer(discoveries []DiscoveryResult) *DiscoveryResult {
	var best *DiscoveryResult

	for i := range discoveries {
		result := &discoveries[i]

		// Skip non-responding servers
		if !result.Responding {
			continue
		}

		// First responding server wins, or prefer one with more chips
		if best == nil || result.ChipCount > best.ChipCount ||
			(result.ChipCount == best.ChipCount && result.LatencyMs < best.LatencyMs) {
			best = result
		}
	}

	return best
}

// DiscoverAndConnect scans network and connects to the best available server
func DiscoverAndConnect(config DiscoveryConfig) (*asic.ASICClient, *DiscoveryResult, error) {
	discoveries, err := DiscoverServers(config)
	if err != nil {
		return nil, nil, fmt.Errorf("network discovery failed: %w", err)
	}

	best := FindBestServer(discoveries)
	if best == nil {
		return nil, nil, fmt.Errorf("no hasher-server instances found on network")
	}

	// Connect to the best server
	client, err := asic.NewASICClient(best.Address)
	if err != nil {
		return nil, best, fmt.Errorf("failed to connect to best server %s: %w", best.Address, err)
	}

	// Verify the client actually connected to real hardware, not software fallback
	if client.IsUsingFallback() {
		client.Close()
		return nil, best, fmt.Errorf("connected to %s but server is in fallback mode (ASIC not operational)", best.Address)
	}

	return client, best, nil
}
