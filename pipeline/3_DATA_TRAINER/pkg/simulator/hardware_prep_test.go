package simulator

import (
	"encoding/binary"
	"testing"
)

func TestNewHardwarePrep(t *testing.T) {
	t.Run("NewHardwarePrep", func(t *testing.T) {
		// Test with caching enabled
		hp := NewHardwarePrep(true)
		if hp == nil {
			t.Error("NewHardwarePrep returned nil")
			return
		}

		if !hp.enableCaching {
			t.Error("Caching should be enabled")
			return
		}

		// Test cache operations
		testSlots := [12]uint32{0x12345678, 0x23456789, 0x34567890, 0x45678901, 0x56789012, 0x67890123, 0x78901234, 0x89012345, 0x90123456, 0x01234567, 0x11223344, 0x55667788}
		testNonce := uint32(0xdeadbeef)

		// Test header creation
		header := hp.PrepareAsicJob(testSlots, testNonce)
		if len(header) != 80 {
			t.Errorf("Expected 80-byte header, got %d", len(header))
			return
		}

		// Test nonce extraction
		extractedNonce := hp.ExtractNonce(header)
		if extractedNonce != testNonce {
			t.Errorf("Nonce extraction failed: expected 0x%08x, got 0x%08x", testNonce, extractedNonce)
			return
		}

		// Test slots extraction
		extractedSlots := hp.ExtractSlots(header)
		for i := 0; i < 12; i++ {
			if extractedSlots[i] != testSlots[i] {
				t.Errorf("Slot extraction failed at index %d: expected 0x%08x, got 0x%08x", i, testSlots[i], extractedSlots[i])
				return
			}
		}

		// Test validation
		if !hp.ValidateHeader(header) {
			t.Error("Header validation failed")
			return
		}

		t.Log("Hardware preparation test passed")
	})

	t.Run("NewHardwarePrep_NoCache", func(t *testing.T) {
		// Test with caching disabled
		hp := NewHardwarePrep(false)
		if hp == nil {
			t.Error("NewHardwarePrep returned nil")
			return
		}

		if hp.enableCaching {
			t.Error("Caching should be disabled")
			return
		}

		// Test cache stats
		cacheSize, cacheEntries := hp.GetCacheStats()
		if cacheSize != 0 || cacheEntries != 0 {
			t.Errorf("Cache should be empty when disabled, got size: %d, entries: %d", cacheSize, cacheEntries)
			return
		}

		t.Log("No-cache hardware preparation test passed")
	})
}

func TestPrepareAsicJob(t *testing.T) {
	t.Run("PrepareAsicJob", func(t *testing.T) {
		hp := NewHardwarePrep(true)
		testSlots := [12]uint32{0x01010101, 0x02020202, 0x03030303, 0x04040404, 0x05050505,
			0x06060606, 0x07070707, 0x08080808, 0x09090909, 0x0A0A0A0A, 0x0B0B0B0B, 0x0C0C0C0C}
		testNonce := uint32(0x12345678)

		// Test header creation
		header := hp.PrepareAsicJob(testSlots, testNonce)
		if len(header) != 80 {
			t.Errorf("Expected 80-byte header, got %d", len(header))
			return
		}

		// Test header structure
		// Bytes 0-4: Version
		version := binary.LittleEndian.Uint32(header[0:4])
		if version != 0x00000002 {
			t.Errorf("Incorrect version: expected 0x00000002, got 0x%08x", version)
		}

		// Bytes 72-76: Bits
		bits := binary.LittleEndian.Uint32(header[72:76])
		if bits != 0x1d00ffff {
			t.Errorf("Incorrect bits: expected 0x1d00ffff, got 0x%08x", bits)
		}

		// Test slots in header
		extractedSlots := hp.ExtractSlots(header)
		for i := 0; i < 12; i++ {
			if extractedSlots[i] != testSlots[i] {
				t.Errorf("Slot %d not preserved: expected 0x%08x, got 0x%08x", i, testSlots[i], extractedSlots[i])
			}
		}

		// Test nonce position
		nonce := hp.ExtractNonce(header)
		if nonce != testNonce {
			t.Errorf("Nonce not preserved: expected 0x%08x, got 0x%08x", testNonce, nonce)
		}

		t.Log("ASIC job preparation test passed")
	})
}

func TestPrepareAsicJobBatch(t *testing.T) {
	t.Run("PrepareAsicJobBatch", func(t *testing.T) {
		hp := NewHardwarePrep(true)
		testSlots := [12]uint32{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12}
		testNonces := []uint32{0x11111111, 0x22222222, 0x33333333}

		headers := hp.PrepareAsicJobBatch(testSlots, testNonces)
		if len(headers) != len(testNonces) {
			t.Errorf("Batch size mismatch: expected %d, got %d", len(testNonces), len(headers))
			return
		}

		// Verify each header
		for i, header := range headers {
			// Each header should be 80 bytes
			if len(header) != 80 {
				t.Errorf("Header %d is not 80 bytes: got %d", i, len(header))
				return
			}

			// Verify nonce is correctly set
			expectedNonce := testNonces[i]
			actualNonce := hp.ExtractNonce(header)
			if actualNonce != expectedNonce {
				t.Errorf("Nonce mismatch in header %d: expected 0x%08x, got 0x%08x", i, expectedNonce, actualNonce)
				return
			}
		}

		t.Log("Batch ASIC job preparation test passed")
	})
}

func TestValidateHeader(t *testing.T) {
	t.Run("ValidateHeader", func(t *testing.T) {
		hp := NewHardwarePrep(true)

		// Test valid header
		validHeader := make([]byte, 80)
		binary.LittleEndian.PutUint32(validHeader[0:4], 0x00000002)   // Version
		binary.LittleEndian.PutUint32(validHeader[72:76], 0x1d00ffff) // Bits
		// Fill slots with valid data
		for i := 0; i < 8; i++ {
			binary.BigEndian.PutUint32(validHeader[4+(i*4):], uint32(i+1))
		}
		for i := 0; i < 4; i++ {
			binary.BigEndian.PutUint32(validHeader[36+(i*4):], uint32(i+9))
		}
		// Remaining padding bytes are already zeros

		if !hp.ValidateHeader(validHeader) {
			t.Error("Valid header validation failed")
			return
		}

		// Test invalid headers
		invalidTests := []struct {
			name        string
			header      []byte
			description string
		}{
			{"wrong length", make([]byte, 79), "Header with 79 bytes"},
			{"wrong version", []byte{0xFF, 0xFF, 0xFF, 0xFF}, "Header with wrong version"},
			{"wrong bits", []byte{0x00, 0x00, 0x00, 0x00}, "Header with wrong bits"},
			{"empty header", make([]byte, 0), "Empty header"},
		}

		for _, test := range invalidTests {
			if hp.ValidateHeader(test.header) {
				t.Errorf("Validation should have failed for %s: %s", test.name, test.description)
			}
		}

		t.Log("Header validation test passed")
	})
}

func TestExtractNonce(t *testing.T) {
	t.Run("ExtractNonce", func(t *testing.T) {
		hp := NewHardwarePrep(false)

		// Test nonce extraction from valid headers
		testCases := []struct {
			nonce    uint32
			expected string
		}{
			{0x12345678, "Little-endian nonce 0x12345678"},
			{0x78563412, "Big-endian nonce 0x78563412"},
			{0xdeadbeef, "Nonce 0xdeadbeef at end"},
		}

		for _, test := range testCases {
			// Create header with test nonce
			header := make([]byte, 80)
			binary.LittleEndian.PutUint32(header[0:4], 0x00000002)
			// Fill middle section with zeros
			for i := 4; i < 72; i++ {
				header[i] = 0
			}
			binary.LittleEndian.PutUint32(header[68:72], 0x1d00ffff)
			binary.LittleEndian.PutUint32(header[76:80], test.nonce) // Nonce at end

			extractedNonce := hp.ExtractNonce(header)
			if extractedNonce != test.nonce {
				t.Errorf("Nonce extraction failed for %s: expected 0x%08x, got 0x%08x", test.expected, test.nonce, extractedNonce)
			}

			// Verify description matches
			if test.expected == "Little-endian nonce 0x12345678" {
				// Test little-endian representation
				expectedBytes := []byte{0x78, 0x56, 0x34, 0x12}
				if header[76] != expectedBytes[0] || header[77] != expectedBytes[1] ||
					header[78] != expectedBytes[2] || header[79] != expectedBytes[3] {
					t.Errorf("Little-endian encoding failed for nonce 0x%08x", test.nonce)
				}
			}

			if test.expected == "Big-endian nonce 0x78563412" {
				// Test big-endian representation
				expectedBytes := []byte{0x12, 0x34, 0x56, 0x78}
				if header[76] != expectedBytes[0] || header[77] != expectedBytes[1] ||
					header[78] != expectedBytes[2] || header[79] != expectedBytes[3] {
					t.Errorf("Big-endian encoding failed for nonce 0x%08x", test.nonce)
				}
			}

			if test.expected == "Nonce 0xdeadbeef at end" {
				// Test extraction from end of header
				if extractedNonce != 0xdeadbeef {
					t.Errorf("Failed to extract final nonce: expected 0x%08x, got 0x%08x", test.nonce, extractedNonce)
				}
			}
		}

		t.Log("Nonce extraction test passed")
	})
}

func TestExtractSlots(t *testing.T) {
	t.Run("ExtractSlots", func(t *testing.T) {
		hp := NewHardwarePrep(false)

		// Test slots extraction from valid headers
		testSlots := [12]uint32{0x01010101, 0x02020202, 0x03030303, 0x04040404, 0x05050505,
			0x06060606, 0x07070707, 0x08080808, 0x09090909, 0x0A0A0A0A, 0x0B0B0B0B, 0x0C0C0C0C}

		header := make([]byte, 80)
		binary.LittleEndian.PutUint32(header[0:4], 0x00000002)   // Version
		binary.LittleEndian.PutUint32(header[72:76], 0x1d00ffff) // Bits

		// Set slots in Big-Endian format
		for i := 0; i < 8; i++ {
			binary.BigEndian.PutUint32(header[4+(i*4):], testSlots[i])
		}
		for i := 0; i < 4; i++ {
			binary.BigEndian.PutUint32(header[36+(i*4):], testSlots[i+8])
		}
		// Remaining padding bytes are zeros

		extractedSlots := hp.ExtractSlots(header)
		for i := 0; i < 12; i++ {
			if extractedSlots[i] != testSlots[i] {
				t.Errorf("Slot extraction failed at index %d: expected 0x%08x, got 0x%08x", i, testSlots[i], extractedSlots[i])
				return
			}
		}

		t.Log("Slots extraction test passed")
	})
}
