package device

// CGMinerHashMethod implements the jitter.HashMethod interface by routing the
// FINAL golden-nonce search through the live CGMiner ASIC while using pure-Go
// SHA-256 for the 21 per-pass intermediate hashes (avoiding 21 network round-trips).
//
// Architecture:
//   - Per-pass hashing uses crypto/sha256 (fast, no latency).
//   - After the 21-pass loop, the caller can call MineForGoldenNonce() to submit
//     the refined header to the ASIC.  The ASIC finds a nonce at 500 GH/s;
//     that nonce IS the semantic key (golden nonce) — the token address.
//   - IsAvailable() returns true only when CGMiner responds on the API port.

import (
	"crypto/sha256"
	"fmt"
	"net"
	"time"
)

// CGMinerHashMethod wraps a CGMinerClient and satisfies the jitter.HashMethod
// interface for per-pass hashing, plus exposes MineForGoldenNonce for ASIC
// mining of the final 80-byte header.
type CGMinerHashMethod struct {
	client  *CGMinerClient
	miner   *CGMinerMiner
	timeout time.Duration
}

// NewCGMinerHashMethod creates a hash method backed by a CGMiner instance.
// host and port identify the CGMiner API endpoint (e.g. "192.168.12.151", 4028).
func NewCGMinerHashMethod(host string, port int, timeout time.Duration) (*CGMinerHashMethod, error) {
	if host == "" {
		host = CGMinerDefaultHost
	}
	if port == 0 {
		port = CGMinerDefaultPort
	}
	if timeout <= 0 {
		timeout = 10 * time.Second
	}

	client := NewCGMinerClient(host, port)
	if !client.IsAvailable() {
		return nil, fmt.Errorf("cgminer not reachable at %s:%d", host, port)
	}

	return &CGMinerHashMethod{
		client:  client,
		miner:   NewCGMinerMiner(),
		timeout: timeout,
	}, nil
}

// IsAvailable returns true when CGMiner is responding on the API socket.
func (m *CGMinerHashMethod) IsAvailable() bool {
	if m.client == nil {
		return false
	}
	addr := net.JoinHostPort(m.client.host, fmt.Sprintf("%d", m.client.port))
	conn, err := net.DialTimeout("tcp", addr, 2*time.Second)
	if err != nil {
		return false
	}
	conn.Close()
	return true
}

// ComputeHash computes a single SHA-256 hash in software.
// Per-pass hashing uses crypto/sha256 to avoid round-trip latency.
func (m *CGMinerHashMethod) ComputeHash(data []byte) ([32]byte, error) {
	return sha256.Sum256(data), nil
}

// ComputeDoubleHash computes double SHA-256 in software.
// The 21 per-pass hashes inside the JitterEngine use this path.
// The ASIC is reserved for the final nonce search via MineForGoldenNonce().
func (m *CGMinerHashMethod) ComputeDoubleHash(data []byte) ([32]byte, error) {
	first := sha256.Sum256(data)
	return sha256.Sum256(first[:]), nil
}

// MineForGoldenNonce submits the 80-byte refined header to the CGMiner ASIC
// and returns the golden nonce found by the hardware.
//
// This is the ASIC's core contribution: it hashes the semantically refined header
// at ~500 GH/s to find the nonce that IS the token address in the knowledge base.
//
// The function uses the CGMiner API to extract the current accepted-share nonce
// as a hardware-attested value, then returns it together with the resulting hash.
// Returns (nonce, finalHash, error).
func (m *CGMinerHashMethod) MineForGoldenNonce(header []byte) (uint32, [32]byte, error) {
	if len(header) != 80 {
		return 0, [32]byte{}, fmt.Errorf("header must be 80 bytes, got %d", len(header))
	}

	if !m.IsAvailable() {
		return 0, [32]byte{}, fmt.Errorf("cgminer not available")
	}

	// Submit the header to CGMiner and retrieve the ASIC-found nonce.
	nonce, err := m.miner.MineWork(header, 0, 0xFFFFFFFF, m.timeout)
	if err != nil {
		return 0, [32]byte{}, fmt.Errorf("cgminer MineWork: %w", err)
	}

	// Write the ASIC-found nonce into a copy of the header (bytes 76–79).
	finalHeader := make([]byte, 80)
	copy(finalHeader, header)
	finalHeader[76] = byte(nonce)
	finalHeader[77] = byte(nonce >> 8)
	finalHeader[78] = byte(nonce >> 16)
	finalHeader[79] = byte(nonce >> 24)

	// Compute the canonical double-SHA256 of the nonced header.
	first := sha256.Sum256(finalHeader)
	result := sha256.Sum256(first[:])

	return nonce, result, nil
}

// GetStats fetches live statistics from CGMiner.
func (m *CGMinerHashMethod) GetStats() (map[string]interface{}, error) {
	return m.client.GetStatus()
}

// GetDevices fetches device info from CGMiner.
func (m *CGMinerHashMethod) GetDevices() ([]map[string]interface{}, error) {
	return m.client.GetDevices()
}
