package training

import (
	"encoding/binary"
	"fmt"
	"testing"

	"hasher/pkg/hashing/core"
	"hasher/pkg/hashing/methods/cuda"
)

func TestCudaStandardSha256(t *testing.T) {
	bridge := cuda.NewCudaBridge()
	if bridge == nil {
		t.Skip("CUDA bridge not available")
	}

	canon := core.NewCanonicalSHA256()

	// Test case: 80-byte Bitcoin header
	header := make([]byte, 80)
	for i := range header {
		header[i] = byte(i)
	}

	// Software (Canonical) result
	expectedHash := canon.ComputeDoubleSHA256(header)

	// CUDA result
	cudaResults, err := bridge.ComputeDoubleHashFull([][]byte{header})
	if err != nil {
		t.Fatalf("CUDA ComputeDoubleHashFull failed: %v", err)
	}

	if len(cudaResults) != 1 {
		t.Fatalf("Expected 1 result, got %d", len(cudaResults))
	}

	actualHash := cudaResults[0]

	if actualHash != expectedHash {
		t.Errorf("CUDA hash mismatch!\nExpected: %x\nActual:   %x", expectedHash, actualHash)
	} else {
		fmt.Printf("CUDA hash matched Canonical SHA-256: %x\n", actualHash)
	}
}

func TestEndiannessMatching(t *testing.T) {
	// If a hash starts with 00 00 04 D2 (Target 1234)
	hash := [32]byte{0x00, 0x00, 0x04, 0xD2}
	targetToken := uint32(1234)

	be := binary.BigEndian.Uint32(hash[:4])
	le := binary.LittleEndian.Uint32(hash[:4])

	fmt.Printf("Hash bytes: %x\n", hash[:4])
	fmt.Printf("Target: %d (0x%x)\n", targetToken, targetToken)
	fmt.Printf("BigEndian extraction: %d (0x%x)\n", be, be)
	fmt.Printf("LittleEndian extraction: %d (0x%x)\n", le, le)

	if be == targetToken {
		fmt.Println("BigEndian matches target token!")
	} else {
		fmt.Println("BigEndian DOES NOT match target token!")
	}

	if le == targetToken {
		fmt.Println("LittleEndian matches target token!")
	} else {
		fmt.Println("LittleEndian DOES NOT match target token!")
	}
}
