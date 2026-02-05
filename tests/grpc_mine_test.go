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
	serverAddressMine = "192.168.12.151:8889"
	timeoutMine       = 30 * time.Second
)

func mainMine() {
	log.Printf("Attempting to connect to hasher-server at %s", serverAddressMine)

	// Set up a connection to the gRPC server.
	conn, err := grpc.Dial(serverAddressMine, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		log.Fatalf("did not connect: %v", err)
	}
	defer conn.Close()
	c := pb.NewHasherServiceClient(conn)

	// Prepare a dummy MineWorkRequest
	dummyHeader := make([]byte, 80)
	// Fill with some arbitrary data for the header (e.g., first 4 bytes are version, next 32 prev hash, etc.)
	// For testing connectivity, the actual content doesn't matter as much as the format.
	for i := 0; i < 80; i++ {
		dummyHeader[i] = byte(i)
	}
	// Use proper Bitcoin Difficulty 1 target (0x1d00ffff) - this is the correct format
	// The ASIC will find the first nonce that makes SHA256(SHA256(header+nonce)) < target
	dummyHeader[72] = 0xff
	dummyHeader[73] = 0xff
	dummyHeader[74] = 0x00
	dummyHeader[75] = 0x1d

	nonceStart := uint32(0)
	nonceEnd := uint32(1000000) // Search 1 million nonces

	log.Printf("Sending MineWork request with dummy header...")

	ctx, cancel := context.WithTimeout(context.Background(), timeoutMine)
	defer cancel()

	r, err := c.MineWork(ctx, &pb.MineWorkRequest{
		Header:     dummyHeader,
		NonceStart: nonceStart,
		NonceEnd:   nonceEnd,
	})
	if err != nil {
		log.Fatalf("could not mine work: %v", err)
	}

	log.Printf("MineWork successful! Nonce: %d, Latency: %d us", r.GetNonce(), r.GetLatencyUs())
}
