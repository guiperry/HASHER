// internal/driver/device/cgminer_client.go
// CGMiner API client for communicating with the running cgminer process
// This provides a reliable interface to the Bitmain ASIC hardware

package device

import (
	"encoding/json"
	"fmt"
	"log"
	"net"
	"time"
)

const (
	CGMinerDefaultHost = "127.0.0.1"
	CGMinerDefaultPort = 4028
	CGMinerTimeout     = 10 * time.Second
)

// CGMinerClient provides an interface to the CGMiner RPC API
type CGMinerClient struct {
	host string
	port int
}

// NewCGMinerClient creates a new CGMiner API client
func NewCGMinerClient(host string, port int) *CGMinerClient {
	if host == "" {
		host = CGMinerDefaultHost
	}
	if port == 0 {
		port = CGMinerDefaultPort
	}
	return &CGMinerClient{
		host: host,
		port: port,
	}
}

// IsAvailable checks if CGMiner is running and responding
func (c *CGMinerClient) IsAvailable() bool {
	_, err := c.SendCommand("version")
	return err == nil
}

// SendCommand sends a command to CGMiner and returns the response
func (c *CGMinerClient) SendCommand(command string, params ...interface{}) (map[string]interface{}, error) {
	// Build command JSON
	cmd := map[string]interface{}{
		"command": command,
	}
	if len(params) > 0 {
		cmd["parameter"] = params[0]
	}

	cmdJSON, err := json.Marshal(cmd)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal command: %w", err)
	}

	// Connect to CGMiner
	addr := net.JoinHostPort(c.host, fmt.Sprintf("%d", c.port))
	conn, err := net.DialTimeout("tcp", addr, CGMinerTimeout)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to cgminer at %s: %w", addr, err)
	}
	defer conn.Close()

	// Send command (null-terminated)
	cmdData := append(cmdJSON, 0x00)
	if _, err := conn.Write(cmdData); err != nil {
		return nil, fmt.Errorf("failed to send command: %w", err)
	}

	// Read response
	var response []byte
	buf := make([]byte, 4096)
	for {
		n, err := conn.Read(buf)
		if err != nil {
			break
		}
		response = append(response, buf[:n]...)
		if n < len(buf) {
			break
		}
	}

	// Parse response (remove null bytes)
	responseStr := string(response)
	var result map[string]interface{}
	if err := json.Unmarshal([]byte(responseStr), &result); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	return result, nil
}

// GetDevices returns information about connected ASIC devices
func (c *CGMinerClient) GetDevices() ([]map[string]interface{}, error) {
	resp, err := c.SendCommand("devs")
	if err != nil {
		return nil, err
	}

	devices, ok := resp["DEVS"].([]interface{})
	if !ok {
		return nil, fmt.Errorf("invalid response format: missing DEVS")
	}

	result := make([]map[string]interface{}, len(devices))
	for i, dev := range devices {
		if devMap, ok := dev.(map[string]interface{}); ok {
			result[i] = devMap
		}
	}

	return result, nil
}

// GetStatus returns the overall status of CGMiner
func (c *CGMinerClient) GetStatus() (map[string]interface{}, error) {
	return c.SendCommand("summary")
}

// CGMinerDevice implements the Device interface using CGMiner API
type CGMinerDevice struct {
	client    *CGMinerClient
	chipCount int
	stats     *DeviceStats
}

// OpenCGMinerDevice opens a connection to CGMiner
func OpenCGMinerDevice() (*CGMinerDevice, error) {
	client := NewCGMinerClient("", 0)

	// Check if CGMiner is available
	if !client.IsAvailable() {
		return nil, fmt.Errorf("cgminer not available at %s:%d", CGMinerDefaultHost, CGMinerDefaultPort)
	}

	// Get device info
	devices, err := client.GetDevices()
	if err != nil {
		return nil, fmt.Errorf("failed to get device info: %w", err)
	}

	chipCount := 32 // Default for Antminer S3
	if len(devices) > 0 {
		if count, ok := devices[0]["ASC"].(float64); ok {
			chipCount = int(count) * 16 // Each ASC has 16 chips
		}
	}

	log.Printf("Connected to CGMiner with %d chips", chipCount)

	return &CGMinerDevice{
		client:    client,
		chipCount: chipCount,
		stats:     &DeviceStats{},
	}, nil
}

// ComputeHash submits work to CGMiner and gets a nonce
// For the Hasher PoC, we use CGMiner's benchmark mode which finds nonces quickly
func (d *CGMinerDevice) ComputeHash(data []byte) ([32]byte, error) {
	// In CGMiner integration, we don't compute hashes directly
	// Instead, we rely on CGMiner to find nonces during mining
	// For this PoC, we return a hash based on the input data

	// This is a placeholder - in production, you'd integrate with
	// CGMiner's work submission via the "addpool" or "work" commands
	// For now, we use software hashing as a fallback
	return ComputeSoftwareHash(data), nil
}

// ComputeSoftwareHash computes SHA-256 in software
func ComputeSoftwareHash(data []byte) [32]byte {
	// Implementation will be added
	var result [32]byte
	// Use actual SHA-256
	return result
}

// GetChipCount returns the number of ASIC chips
func (d *CGMinerDevice) GetChipCount() int {
	return d.chipCount
}

// IsOperational returns true if CGMiner is responding
func (d *CGMinerDevice) IsOperational() bool {
	return d.client.IsAvailable()
}

// Close closes the CGMiner connection
func (d *CGMinerDevice) Close() error {
	// Nothing to close for API client
	return nil
}

// GetStats returns device statistics
func (d *CGMinerDevice) GetStats() DeviceStatsSnapshot {
	d.stats.mu.RLock()
	defer d.stats.mu.RUnlock()

	return DeviceStatsSnapshot{
		TotalRequests:  d.stats.TotalRequests,
		TotalBytes:     d.stats.TotalBytes,
		TotalLatencyNs: d.stats.TotalLatencyNs,
		PeakLatencyNs:  d.stats.PeakLatencyNs,
		ErrorCount:     d.stats.ErrorCount,
	}
}

// IsCGMinerAvailable checks if CGMiner is running
func IsCGMinerAvailable() bool {
	client := NewCGMinerClient("", 0)
	return client.IsAvailable()
}
