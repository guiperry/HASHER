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
	serverAddressSmall = "192.168.12.151:8888"
	timeoutSmall       = 10 * time.Second
)

func mainSmall() {
	log.Printf("Attempting to connect to hasher-server at %s", serverAddressSmall)

	// Set up a connection to the gRPC server.
	conn, err := grpc.Dial(serverAddressSmall, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		log.Fatalf("did not connect: %v", err)
	}
	defer conn.Close()
	c := pb.NewHasherServiceClient(conn)

	// Test with a very small nonce range first
	dummyHeader := make([]byte, 80)
	for i := 0; i < 80; i++ {
		dummyHeader[i] = byte(i)
	}
	dummyHeader[72] = 0xff
	dummyHeader[73] = 0xff
	dummyHeader[74] = 0x00
	dummyHeader[75] = 0x1d

	nonceStart := uint32(0)
	nonceEnd := uint32(1000) // Small range first

	log.Printf("Sending small range test (0-1000)...")

	ctx, cancel := context.WithTimeout(context.Background(), timeoutSmall)
	defer cancel()

	r, err := c.MineWork(ctx, &pb.MineWorkRequest{
		Header:     dummyHeader,
		NonceStart: nonceStart,
		NonceEnd:   nonceEnd,
	})
	if err != nil {
		log.Fatalf("could not mine work: %v", err)
	}

	log.Printf("SUCCESS! Nonce: %d, Latency: %d us", r.GetNonce(), r.GetLatencyUs())
}
