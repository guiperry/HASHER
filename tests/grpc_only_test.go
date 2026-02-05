package tests

import (
	"context"
	"log"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	pb "hasher/internal/proto/hasher/v1"
)

const (
	serverAddressOnly = "192.168.12.151:8888"
)

func mainGrpcOnly() {
	log.Printf("Testing gRPC connectivity to hasher-server at %s", serverAddressOnly)

	// Set up a connection to the gRPC server.
	conn, err := grpc.Dial(serverAddressOnly, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		log.Fatalf("did not connect: %v", err)
	}
	defer conn.Close()
	c := pb.NewHasherServiceClient(conn)

	// Test 1: GetDeviceInfo - should work even without hardware
	log.Printf("Test 1: Getting device info...")
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	info, err := c.GetDeviceInfo(ctx, &pb.GetDeviceInfoRequest{})
	if err != nil {
		log.Printf("GetDeviceInfo failed: %v", err)
	} else {
		log.Printf("✅ GetDeviceInfo SUCCESS: %s", info.String())
	}

	// Test 2: Try MineWork - expect to fail due to hardware issues, but should show gRPC works
	log.Printf("Test 2: Testing MineWork (expecting hardware failure)...")
	dummyHeader := make([]byte, 80)
	for i := 0; i < 80; i++ {
		dummyHeader[i] = byte(i)
	}
	dummyHeader[72] = 0xff
	dummyHeader[73] = 0xff
	dummyHeader[74] = 0x00
	dummyHeader[75] = 0x1d

	ctx2, cancel2 := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel2()

	_, err = c.MineWork(ctx2, &pb.MineWorkRequest{
		Header:     dummyHeader,
		NonceStart: 0,
		NonceEnd:   1000,
	})
	if err != nil {
		log.Printf("❌ MineWork failed as expected: %v", err)
		log.Printf("   (This confirms gRPC is working, but hardware has issues)")
	} else {
		log.Printf("🎉 MineWork UNEXPECTED SUCCESS!")
	}

	log.Printf("gRPC connectivity test complete.")
}
