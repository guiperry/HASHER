// internal/driver/device/server.go
package device

import (
	"context"
	"fmt"
	"io"
	"log" // Added for logging
	"sync"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	pb "hasher/internal/proto/hasher/v1"
)

// HasherServer implements the gRPC HasherService
type HasherServer struct {
	pb.UnimplementedHasherServiceServer

	device    *Device
	startTime time.Time
	mu        sync.RWMutex
}

// NewHasherServer creates a new Hasher gRPC server
func NewHasherServer(enableTracing bool) (*HasherServer, error) {
	device, err := OpenDevice(enableTracing)
	if err != nil {
		return nil, fmt.Errorf("open device: %w", err)
	}

	return NewHasherServerWithDevice(device), nil
}

// NewHasherServerWithDevice creates a new Hasher gRPC server with a pre-initialized device
func NewHasherServerWithDevice(dev *Device) *HasherServer {
	return &HasherServer{
		device:    dev,
		startTime: time.Now(),
	}
}

// ComputeHash implements single hash computation
func (s *HasherServer) ComputeHash(ctx context.Context, req *pb.ComputeHashRequest) (*pb.ComputeHashResponse, error) {
	if len(req.Data) == 0 {
		return nil, status.Error(codes.InvalidArgument, "data cannot be empty")
	}

	start := time.Now()

	hash, err := s.device.ComputeHash(req.Data)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "compute failed: %v", err)
	}

	latency := time.Since(start)

	return &pb.ComputeHashResponse{
		Hash:      hash[:],
		LatencyUs: uint64(latency.Microseconds()),
	}, nil
}

// ComputeBatch implements batch hash computation
func (s *HasherServer) ComputeBatch(ctx context.Context, req *pb.ComputeBatchRequest) (*pb.ComputeBatchResponse, error) {
	if len(req.Data) == 0 {
		return nil, status.Error(codes.InvalidArgument, "batch cannot be empty")
	}

	maxBatch := MaxBatchSize
	if req.MaxBatchSize > 0 && req.MaxBatchSize < uint32(MaxBatchSize) {
		maxBatch = int(req.MaxBatchSize)
	}

	if len(req.Data) > maxBatch {
		return nil, status.Errorf(codes.InvalidArgument, "batch size %d exceeds maximum %d", len(req.Data), maxBatch)
	}

	start := time.Now()

	hashes, err := s.device.ComputeBatch(req.Data)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "batch compute failed: %v", err)
	}

	latency := time.Since(start)

	// Convert to byte slices
	hashBytes := make([][]byte, len(hashes))
	for i, h := range hashes {
		hashBytes[i] = h[:]
	}

	return &pb.ComputeBatchResponse{
		Hashes:         hashBytes,
		TotalLatencyUs: uint64(latency.Microseconds()),
		ProcessedCount: uint32(len(hashes)),
	}, nil
}

// StreamCompute implements streaming hash computation
func (s *HasherServer) StreamCompute(stream pb.HasherService_StreamComputeServer) error {
	ctx := stream.Context()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		// Receive request
		req, err := stream.Recv()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return status.Errorf(codes.Internal, "receive failed: %v", err)
		}

		if len(req.Data) == 0 {
			continue
		}

		// Compute hash
		start := time.Now()
		hash, err := s.device.ComputeHash(req.Data)
		if err != nil {
			return status.Errorf(codes.Internal, "compute failed: %v", err)
		}
		latency := time.Since(start)

		// Send response
		resp := &pb.StreamComputeResponse{
			Hash:      hash[:],
			RequestId: req.RequestId,
			LatencyUs: uint64(latency.Microseconds()),
		}

		if err := stream.Send(resp); err != nil {
			return status.Errorf(codes.Internal, "send failed: %v", err)
		}
	}
}

// GetMetrics retrieves performance metrics
func (s *HasherServer) GetMetrics(ctx context.Context, req *pb.GetMetricsRequest) (*pb.GetMetricsResponse, error) {
	stats := s.device.GetStats()

	avgLatency := uint64(0)
	if stats.TotalRequests > 0 {
		avgLatency = stats.TotalLatencyNs / stats.TotalRequests / 1000 // Convert to microseconds
	}

	peakLatency := stats.PeakLatencyNs / 1000 // Convert to microseconds

	resp := &pb.GetMetricsResponse{
		TotalRequests:       stats.TotalRequests,
		TotalBytesProcessed: stats.TotalBytes,
		AverageLatencyUs:    avgLatency,
		PeakLatencyUs:       peakLatency,
		TotalErrors:         stats.ErrorCount,
		CacheHits:           0, // Not implemented yet
		CacheMisses:         0, // Not implemented yet
		DeviceStats:         make(map[string]uint64),
	}

	// Add device-specific stats
	resp.DeviceStats["total_requests"] = stats.TotalRequests
	resp.DeviceStats["total_bytes"] = stats.TotalBytes

	return resp, nil
}

// GetDeviceInfo retrieves device information
func (s *HasherServer) GetDeviceInfo(ctx context.Context, req *pb.GetDeviceInfoRequest) (*pb.GetDeviceInfoResponse, error) {
	info := s.device.GetInfo()

	uptime := time.Since(s.startTime)

	return &pb.GetDeviceInfoResponse{
		DevicePath:      info.DevicePath,
		ChipCount:       uint32(info.ChipCount),
		FirmwareVersion: info.FirmwareVersion,
		IsOperational:   info.IsOperational,
		UptimeSeconds:   uint64(uptime.Seconds()),
	}, nil
}

// MineWork implements mining on a Bitcoin-style header to find a valid nonce
func (s *HasherServer) MineWork(ctx context.Context, req *pb.MineWorkRequest) (*pb.MineWorkResponse, error) {
	log.Printf("MineWork RPC received for header len %d, nonceStart %d, nonceEnd %d", len(req.Header), req.NonceStart, req.NonceEnd) // Log RPC call
	if len(req.Header) != 80 {
		return nil, status.Error(codes.InvalidArgument, "mining header must be exactly 80 bytes")
	}

	start := time.Now()

	// Use a fixed workID for now, and a default timeout
	// The MineWork method in Device handles the actual interaction
	nonce, err := s.device.MineWork(req.Header, req.NonceStart, req.NonceEnd, 1, 5*time.Second) // Increased timeout
	if err != nil {
		log.Printf("Error from s.device.MineWork: %v", err) // Log internal error
		return nil, status.Errorf(codes.Internal, "mining failed: %v", err)
	}

	latency := time.Since(start)

	log.Printf("MineWork RPC successful! Nonce: %d, Latency: %d us", nonce, latency.Microseconds()) // Log success
	return &pb.MineWorkResponse{
		Nonce:     nonce,
		LatencyUs: uint64(latency.Microseconds()),
	}, nil
}

// Close closes the server and releases resources
func (s *HasherServer) Close() error {
	return s.device.Close()
}
