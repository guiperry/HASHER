package host

import (
	"context"
	"fmt"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	pb "hasher/internal/proto/hasher/v1"
)

const (
	// DefaultHasherServerAddress is the default address for the hasher-server on ASIC device
	DefaultHasherServerAddress = "192.168.1.99:8888"
)

// ASICDevice represents a connection to the ASIC hasher-server
type ASICDevice struct {
	hasherClient pb.HasherServiceClient
	hasherConn   *grpc.ClientConn
	chipCount    int
	frequency    int
	serverAddr   string
}

// NewASICDevice creates a new ASIC driver that connects to the hasher-server at default address
func NewASICDevice() (*ASICDevice, error) {
	return NewASICDeviceWithAddress(DefaultHasherServerAddress)
}

// NewASICDeviceWithAddress creates a new ASIC driver that connects to the specified hasher-server address
func NewASICDeviceWithAddress(serverAddr string) (*ASICDevice, error) {
	d := &ASICDevice{
		serverAddr: serverAddr,
	}

	// Connect to hasher-server
	if err := d.connectHasher(); err != nil {
		return nil, fmt.Errorf("failed to connect to hasher-server: %w", err)
	}

	return d, nil
}

// connectHasher establishes a gRPC connection to hasher-server
func (d *ASICDevice) connectHasher() error {
	conn, err := grpc.Dial(d.serverAddr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return err
	}

	d.hasherConn = conn
	d.hasherClient = pb.NewHasherServiceClient(conn)

	// Verify connection is working
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	deviceInfo, err := d.hasherClient.GetDeviceInfo(ctx, &pb.GetDeviceInfoRequest{})
	if err != nil {
		conn.Close()
		return err
	}

	d.chipCount = int(deviceInfo.ChipCount)

	return nil
}

// Close the device
func (d *ASICDevice) Close() error {
	if d.hasherConn != nil {
		return d.hasherConn.Close()
	}
	return nil
}

// ComputeLayer sends a batch of hash computations for a single network layer
// to the ASIC using the hasher-driver and returns the results.
func (d *ASICDevice) ComputeLayer(input []byte, seeds [][32]byte) []byte {
	numNeurons := len(seeds)
	results := make([]byte, numNeurons*32)

	// Prepare all computation jobs for the layer
	jobs := make([][]byte, numNeurons)
	for i := 0; i < numNeurons; i++ {
		jobs[i] = append(input, seeds[i][:]...)
	}

	// Use hasher-driver (gRPC) for computation
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	resp, err := d.hasherClient.ComputeBatch(ctx, &pb.ComputeBatchRequest{
		Data:         jobs,
		MaxBatchSize: 32,
	})
	if err != nil {
		fmt.Printf("ComputeBatch failed: %v", err)
		return nil
	}

	for i, hash := range resp.Hashes {
		copy(results[i*32:(i+1)*32], hash)
	}

	return results
}

// ComputeHash computes a single hash using the hasher-driver
func (d *ASICDevice) ComputeHash(data []byte) ([]byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	resp, err := d.hasherClient.ComputeHash(ctx, &pb.ComputeHashRequest{
		Data: data,
	})
	if err != nil {
		return nil, err
	}

	return resp.Hash, nil
}

// GetMetrics returns performance metrics from the ASIC driver
func (d *ASICDevice) GetMetrics() (*pb.GetMetricsResponse, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	return d.hasherClient.GetMetrics(ctx, &pb.GetMetricsRequest{})
}

// GetInfo retrieves device information
func (d *ASICDevice) GetInfo() (*pb.GetDeviceInfoResponse, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	return d.hasherClient.GetDeviceInfo(ctx, &pb.GetDeviceInfoRequest{})
}
