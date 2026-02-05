// examples/basic_usage.go
package main

import (
	"context"
	"fmt"
	"log"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	pb "hasher/internal/proto/hasher/v1"
)

func main() {
	// Connect to Hasher server
	conn, err := grpc.Dial("localhost:8888", grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		log.Fatalf("Failed to connect: %v", err)
	}
	defer conn.Close()

	client := pb.NewHasherServiceClient(conn)
	ctx := context.Background()

	// Example 1: Single hash computation
	fmt.Println("=== Example 1: Single Hash ===")
	singleHashExample(ctx, client)

	// Example 2: Batch computation
	fmt.Println("\n=== Example 2: Batch Computation ===")
	batchExample(ctx, client)

	// Example 3: Streaming computation
	fmt.Println("\n=== Example 3: Streaming ===")
	streamExample(ctx, client)

	// Example 4: Get metrics
	fmt.Println("\n=== Example 4: Metrics ===")
	metricsExample(ctx, client)

	// Example 5: Device info
	fmt.Println("\n=== Example 5: Device Info ===")
	deviceInfoExample(ctx, client)
}

func singleHashExample(ctx context.Context, client pb.HasherServiceClient) {
	data := []byte("Hello, Hasher!")

	resp, err := client.ComputeHash(ctx, &pb.ComputeHashRequest{
		Data: data,
	})
	if err != nil {
		log.Fatalf("ComputeHash failed: %v", err)
	}

	fmt.Printf("Input:    %s\n", data)
	fmt.Printf("Hash:     %x\n", resp.Hash)
	fmt.Printf("Latency:  %d µs\n", resp.LatencyUs)
}

func batchExample(ctx context.Context, client pb.HasherServiceClient) {
	// Prepare batch of data
	batch := [][]byte{
		[]byte("data1"),
		[]byte("data2"),
		[]byte("data3"),
		[]byte("data4"),
	}

	start := time.Now()

	resp, err := client.ComputeBatch(ctx, &pb.ComputeBatchRequest{
		Data:         batch,
		MaxBatchSize: 4,
	})
	if err != nil {
		log.Fatalf("ComputeBatch failed: %v", err)
	}

	elapsed := time.Since(start)

	fmt.Printf("Batch size:       %d\n", len(batch))
	fmt.Printf("Processed:        %d\n", resp.ProcessedCount)
	fmt.Printf("Total latency:    %d µs\n", resp.TotalLatencyUs)
	fmt.Printf("Client latency:   %v\n", elapsed)
	fmt.Printf("First hash:       %x\n", resp.Hashes[0][:16])
}

func streamExample(ctx context.Context, client pb.HasherServiceClient) {
	stream, err := client.StreamCompute(ctx)
	if err != nil {
		log.Fatalf("StreamCompute failed: %v", err)
	}

	// Send requests concurrently
	go func() {
		for i := 0; i < 5; i++ {
			data := []byte(fmt.Sprintf("stream_data_%d", i))
			err := stream.Send(&pb.StreamComputeRequest{
				Data:      data,
				RequestId: uint64(i),
			})
			if err != nil {
				log.Printf("Send error: %v", err)
				return
			}
			time.Sleep(50 * time.Millisecond)
		}
		stream.CloseSend()
	}()

	// Receive responses
	count := 0
	for {
		resp, err := stream.Recv()
		if err != nil {
			break
		}

		count++
		fmt.Printf("Request #%d: hash=%x, latency=%dµs\n",
			resp.RequestId, resp.Hash[:8], resp.LatencyUs)
	}

	fmt.Printf("Streamed %d hashes\n", count)
}

func metricsExample(ctx context.Context, client pb.HasherServiceClient) {
	resp, err := client.GetMetrics(ctx, &pb.GetMetricsRequest{})
	if err != nil {
		log.Fatalf("GetMetrics failed: %v", err)
	}

	fmt.Printf("Total requests:   %d\n", resp.TotalRequests)
	fmt.Printf("Total bytes:      %d\n", resp.TotalBytesProcessed)
	fmt.Printf("Avg latency:      %d µs\n", resp.AverageLatencyUs)
	fmt.Printf("Peak latency:     %d µs\n", resp.PeakLatencyUs)
	fmt.Printf("Errors:           %d\n", resp.TotalErrors)
}

func deviceInfoExample(ctx context.Context, client pb.HasherServiceClient) {
	resp, err := client.GetDeviceInfo(ctx, &pb.GetDeviceInfoRequest{})
	if err != nil {
		log.Fatalf("GetDeviceInfo failed: %v", err)
	}

	fmt.Printf("Device:           %s\n", resp.DevicePath)
	fmt.Printf("Chips:            %d\n", resp.ChipCount)
	fmt.Printf("Firmware:         %s\n", resp.FirmwareVersion)
	fmt.Printf("Operational:      %v\n", resp.IsOperational)
	fmt.Printf("Uptime:           %ds\n", resp.UptimeSeconds)
}
