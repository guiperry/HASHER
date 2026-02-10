package asic

import (
	"context"
	"crypto/sha256"
	"fmt"
	"log" // Added for debugging output
	"sync"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	pb "hasher/internal/proto/hasher/v1"
)

const (
	// DefaultASICServerAddress is the default gRPC server address for hasher-server
	DefaultASICServerAddress = "localhost:8888"
	// ConnectionTimeout for initial gRPC connection
	ConnectionTimeout = 5 * time.Second
	// OperationTimeout for individual hash operations
	OperationTimeout = 30 * time.Second
)

// ASICClient provides hash computation using ASIC hardware with software fallback
type ASICClient struct {
	client            pb.HasherServiceClient
	conn              *grpc.ClientConn
	useFallback       bool   // true = use software SHA-256 instead of ASIC
	wasConnected      bool   // true if we ever successfully connected (prevents silent fallback after connection)
	allowSoftFallback bool   // if true, fall back to software on operation errors (default: false after successful connection)
	address           string // gRPC server address
	chipCount         int    // Number of ASIC chips (from device info)
	mu                sync.RWMutex
}

// NewASICClient creates a new ASICClient with the specified server address
// It attempts to connect to the ASIC server and automatically falls back
// to software SHA-256 if the connection fails
func NewASICClient(address string) (*ASICClient, error) {
	if address == "" {
		address = DefaultASICServerAddress
	}

	c := &ASICClient{
		address:           address,
		useFallback:       false,
		wasConnected:      false,
		allowSoftFallback: true, // Allow fallback during initial connection
	}

	// Try to connect to ASIC server
	if err := c.Connect(); err != nil {
		// Fall back to software mode (only acceptable during initial connection)
		c.useFallback = true
		c.wasConnected = false
		return c, nil // Return successfully in fallback mode
	}

	// Connection succeeded - disable automatic soft fallback for operations
	c.wasConnected = true
	c.allowSoftFallback = false // After successful connection, don't silently fall back

	return c, nil
}

// Connect attempts to establish a gRPC connection to the hasher-server
// Sets fallback mode if connection fails
func (c *ASICClient) Connect() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	ctx, cancel := context.WithTimeout(context.Background(), ConnectionTimeout)
	defer cancel()

	conn, err := grpc.DialContext(ctx, c.address,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithBlock(),
	)
	if err != nil {
		c.useFallback = true
		return fmt.Errorf("failed to connect to hasher-server at %s: %w", c.address, err)
	}

	c.conn = conn
	c.client = pb.NewHasherServiceClient(conn)

	// Verify connection by getting device info
	infoCtx, infoCancel := context.WithTimeout(context.Background(), ConnectionTimeout)
	defer infoCancel()

	info, err := c.client.GetDeviceInfo(infoCtx, &pb.GetDeviceInfoRequest{})
	if err != nil {
		conn.Close()
		c.useFallback = true
		return fmt.Errorf("failed to get device info: %w", err)
	}

	// Check if the ASIC device is actually operational
	if !info.IsOperational {
		conn.Close()
		c.useFallback = true
		return fmt.Errorf("ASIC device is not operational (device path: %s)", info.DevicePath)
	}

	c.chipCount = int(info.ChipCount)
	c.useFallback = false
	c.wasConnected = true
	c.allowSoftFallback = false // Once connected, don't allow silent fallback

	return nil
}

// IsUsingFallback returns true if the client is using software SHA-256 fallback
func (c *ASICClient) IsUsingFallback() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.useFallback
}

// IsConnected returns true if the client is connected to ASIC hardware
func (c *ASICClient) IsConnected() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.conn != nil && !c.useFallback
}

// WasEverConnected returns true if the client ever successfully connected to ASIC
func (c *ASICClient) WasEverConnected() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.wasConnected
}

// SetAllowSoftFallback enables or disables software fallback for operations
// When disabled, operation errors will be reported instead of silently using software
func (c *ASICClient) SetAllowSoftFallback(allow bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.allowSoftFallback = allow
}

// Reconnect attempts to reconnect to the ASIC server
// Returns error if reconnection fails
func (c *ASICClient) Reconnect() error {
	c.mu.Lock()
	// Close existing connection if any
	if c.conn != nil {
		c.conn.Close()
		c.conn = nil
		c.client = nil
	}
	c.mu.Unlock()

	return c.Connect()
}

// GetChipCount returns the number of ASIC chips (0 if in fallback mode)
func (c *ASICClient) GetChipCount() int {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.chipCount
}

// ComputeHash computes a single SHA-256 hash using ASIC or software fallback
func (c *ASICClient) ComputeHash(data []byte) ([32]byte, error) {
	c.mu.RLock()
	useFallback := c.useFallback
	allowFallback := c.allowSoftFallback
	wasConnected := c.wasConnected
	client := c.client
	c.mu.RUnlock()

	if useFallback {
		return c.computeHashSoftware(data), nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), OperationTimeout)
	defer cancel()

	resp, err := client.ComputeHash(ctx, &pb.ComputeHashRequest{
		Data: data,
	})
	if err != nil {
		// Only fall back to software if allowed (initial connection failed)
		// If we were previously connected, report the error instead of silently degrading
		if allowFallback || !wasConnected {
			return c.computeHashSoftware(data), nil
		}
		return [32]byte{}, fmt.Errorf("ASIC hash operation failed (no fallback): %w", err)
	}

	var result [32]byte
	copy(result[:], resp.Hash)
	return result, nil
}

// ComputeDoubleHash computes double SHA-256 using ASIC or software fallback
func (c *ASICClient) ComputeDoubleHash(data []byte) ([32]byte, error) {
	// First hash
	first, err := c.ComputeHash(data)
	if err != nil {
		return [32]byte{}, fmt.Errorf("first hash failed: %w", err)
	}

	// Second hash
	second, err := c.ComputeHash(first[:])
	if err != nil {
		return [32]byte{}, fmt.Errorf("second hash failed: %w", err)
	}

	return second, nil
}

// ComputeBatch computes multiple SHA-256 hashes in a batch
func (c *ASICClient) ComputeBatch(data [][]byte) ([][32]byte, error) {
	c.mu.RLock()
	useFallback := c.useFallback
	allowFallback := c.allowSoftFallback
	wasConnected := c.wasConnected
	client := c.client
	c.mu.RUnlock()

	if useFallback {
		return c.computeBatchSoftware(data), nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), OperationTimeout)
	defer cancel()

	resp, err := client.ComputeBatch(ctx, &pb.ComputeBatchRequest{
		Data:         data,
		MaxBatchSize: 256,
	})
	if err != nil {
		// Only fall back to software if allowed
		if allowFallback || !wasConnected {
			return c.computeBatchSoftware(data), nil
		}
		return nil, fmt.Errorf("ASIC batch operation failed (no fallback): %w", err)
	}

	results := make([][32]byte, len(resp.Hashes))
	for i, hash := range resp.Hashes {
		copy(results[i][:], hash)
	}
	return results, nil
}

// StreamComputeFunc is a callback function for streaming results
type StreamComputeFunc func(requestID uint64, hash [32]byte, latencyUs uint64)

// StreamCompute performs streaming hash computation for high-throughput scenarios
func (c *ASICClient) StreamCompute(data [][]byte, callback StreamComputeFunc) error {
	c.mu.RLock()
	useFallback := c.useFallback
	allowFallback := c.allowSoftFallback
	wasConnected := c.wasConnected
	client := c.client
	c.mu.RUnlock()

	if useFallback {
		// Software fallback for streaming
		for i, d := range data {
			start := time.Now()
			hash := c.computeHashSoftware(d)
			latency := time.Since(start).Microseconds()
			callback(uint64(i+1), hash, uint64(latency))
		}
		return nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), OperationTimeout*time.Duration(len(data)))
	defer cancel()

	stream, err := client.StreamCompute(ctx)
	if err != nil {
		// Only fall back to software if allowed
		if allowFallback || !wasConnected {
			for i, d := range data {
				start := time.Now()
				hash := c.computeHashSoftware(d)
				latency := time.Since(start).Microseconds()
				callback(uint64(i+1), hash, uint64(latency))
			}
			return nil
		}
		return fmt.Errorf("ASIC stream operation failed (no fallback): %w", err)
	}

	// Start receiver goroutine
	done := make(chan error, 1)
	go func() {
		for {
			resp, err := stream.Recv()
			if err != nil {
				done <- err
				return
			}
			var hash [32]byte
			copy(hash[:], resp.Hash)
			callback(resp.RequestId, hash, resp.LatencyUs)
		}
	}()

	// Send all requests
	for i, d := range data {
		err := stream.Send(&pb.StreamComputeRequest{
			Data:      d,
			RequestId: uint64(i + 1),
		})
		if err != nil {
			stream.CloseSend()
			return fmt.Errorf("stream send failed: %w", err)
		}
	}

	stream.CloseSend()
	<-done

	return nil
}

// GetMetrics returns metrics from the ASIC server (nil if in fallback mode)
func (c *ASICClient) GetMetrics() (*pb.GetMetricsResponse, error) {
	c.mu.RLock()
	useFallback := c.useFallback
	client := c.client
	c.mu.RUnlock()

	if useFallback {
		return nil, fmt.Errorf("metrics not available in software fallback mode")
	}

	ctx, cancel := context.WithTimeout(context.Background(), ConnectionTimeout)
	defer cancel()

	return client.GetMetrics(ctx, &pb.GetMetricsRequest{})
}

// GetDeviceInfo returns device info from the ASIC server (nil if in fallback mode)
func (c *ASICClient) GetDeviceInfo() (*pb.GetDeviceInfoResponse, error) {
	c.mu.RLock()
	useFallback := c.useFallback
	client := c.client
	c.mu.RUnlock()

	if useFallback {
		return &pb.GetDeviceInfoResponse{
			DevicePath:      "software",
			ChipCount:       0,
			FirmwareVersion: "software-fallback",
			IsOperational:   true,
			UptimeSeconds:   0,
		}, nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), ConnectionTimeout)
	defer cancel()

	return client.GetDeviceInfo(ctx, &pb.GetDeviceInfoRequest{})
}

// Close closes the gRPC connection
func (c *ASICClient) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.conn != nil {
		err := c.conn.Close()
		c.conn = nil
		c.client = nil
		return err
	}
	return nil
}

// computeHashSoftware computes SHA-256 using software (fallback mode)
func (c *ASICClient) computeHashSoftware(data []byte) [32]byte {
	return sha256.Sum256(data)
}

// computeBatchSoftware computes multiple SHA-256 hashes using software
func (c *ASICClient) computeBatchSoftware(data [][]byte) [][32]byte {
	results := make([][32]byte, len(data))
	for i, d := range data {
		results[i] = sha256.Sum256(d)
	}
	return results
}

// MineHeader performs mining on an 80-byte Bitcoin-style header to find the first valid nonce
// The mining uses Difficulty 1 target (nBits = 0x1d00ffff), meaning any hash with sufficient
// leading zeros is valid. At 500 GH/s, this typically finds a nonce in nanoseconds.
//
// This is the core operation for mining-based neural network activation:
// - Same header + same nonce range = same first valid nonce (deterministic)
// - The nonce becomes the activation value for the MiningNeuron
//
// Now offloads to ASIC hardware via gRPC MineWork method.
func (c *ASICClient) MineHeader(header []byte, nonceStart, nonceEnd uint32) (uint32, error) {
	if len(header) != 80 {
		return 0, fmt.Errorf("mining header must be exactly 80 bytes, got %d", len(header))
	}

	c.mu.RLock()
	useFallback := c.useFallback
	client := c.client // Capture client for gRPC call
	c.mu.RUnlock()

	if useFallback {
		return c.mineSoftware(header, nonceStart, nonceEnd)
	}

	// Use ASIC-accelerated mining via gRPC MineWork method
	ctx, cancel := context.WithTimeout(context.Background(), OperationTimeout)
	defer cancel()

	resp, err := client.MineWork(ctx, &pb.MineWorkRequest{
		Header:     header,
		NonceStart: nonceStart,
		NonceEnd:   nonceEnd,
	})
	if err != nil {
		// If ASIC fails, fall back to software if allowed by current configuration (unlikely after successful connection)
		// Or if we were never successfully connected
		if c.allowSoftFallback || !c.wasConnected {
			log.Printf("Warning: ASIC MineWork failed, falling back to software mining: %v", err)
			return c.mineSoftware(header, nonceStart, nonceEnd)
		}
		return 0, fmt.Errorf("ASIC MineWork operation failed (no fallback): %w", err)
	}

	return resp.Nonce, nil
}

// mineSoftware performs software-based mining to find the first valid nonce
// Uses double SHA-256 (Bitcoin's hash function) with Difficulty 1 target
func (c *ASICClient) mineSoftware(header []byte, nonceStart, nonceEnd uint32) (uint32, error) {
	workHeader := make([]byte, 80)
	copy(workHeader, header)

	for nonce := nonceStart; nonce <= nonceEnd; nonce++ {
		// Set nonce in header (bytes 76-79, little-endian)
		workHeader[76] = byte(nonce)
		workHeader[77] = byte(nonce >> 8)
		workHeader[78] = byte(nonce >> 16)
		workHeader[79] = byte(nonce >> 24)

		// Double SHA-256 (Bitcoin mining)
		hash := c.doubleSHA256(workHeader)

		// Check if hash meets Difficulty 1 target
		// For Difficulty 1, the hash must be less than:
		// 0x00000000FFFF0000000000000000000000000000000000000000000000000000
		// Simplified check: first 4 bytes should have enough leading zeros
		if hash[0] == 0 && hash[1] == 0 && hash[2] == 0 && hash[3] < 0x10 {
			return nonce, nil
		}
	}

	// If no valid nonce found in range, return the last one
	// This shouldn't happen at Difficulty 1 with a reasonable range
	return nonceEnd, nil
}

// doubleSHA256 computes SHA256(SHA256(data)) - Bitcoin's hash function
func (c *ASICClient) doubleSHA256(data []byte) [32]byte {
	first := sha256.Sum256(data)
	return sha256.Sum256(first[:])
}

// MineHeaderBatch performs mining on multiple headers in parallel
// Useful for batch inference with MiningNeuron
func (c *ASICClient) MineHeaderBatch(headers [][]byte, nonceStart, nonceEnd uint32) ([]uint32, error) {
	results := make([]uint32, len(headers))
	var wg sync.WaitGroup
	var firstErr error
	var errMu sync.Mutex

	for i, header := range headers {
		wg.Add(1)
		go func(idx int, h []byte) {
			defer wg.Done()
			nonce, err := c.MineHeader(h, nonceStart, nonceEnd)
			if err != nil {
				errMu.Lock()
				if firstErr == nil {
					firstErr = err
				}
				errMu.Unlock()
				return
			}
			results[idx] = nonce
		}(i, header)
	}

	wg.Wait()
	return results, firstErr
}
