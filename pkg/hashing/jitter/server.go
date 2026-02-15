package jitter

import (
	"encoding/binary"
	"fmt"
	"net"
	"os"
	"sync"
)

// Server handles the high-speed Jitter RPC via Unix Domain Socket
// This provides associative nudges to ASICs or simulators during the 21-pass loop
type Server struct {
	socketPath string
	listener   net.Listener
	mu         sync.Mutex
	running    bool
	jitterFunc func([12]uint32, uint32, int) uint32
}

// NewServer creates a new Jitter RPC server with the given nudge function
func NewServer(socketPath string, jitterFunc func([12]uint32, uint32, int) uint32) *Server {
	return &Server{
		socketPath: socketPath,
		jitterFunc: jitterFunc,
	}
}

// Start begins listening for Jitter RPC requests
func (js *Server) Start() error {
	js.mu.Lock()
	defer js.mu.Unlock()

	if js.running {
		return nil
	}

	// Cleanup old socket if it exists
	if _, err := os.Stat(js.socketPath); err == nil {
		if err := os.Remove(js.socketPath); err != nil {
			return fmt.Errorf("failed to remove old socket: %w", err)
		}
	}

	listener, err := net.Listen("unix", js.socketPath)
	if err != nil {
		return fmt.Errorf("failed to listen on socket: %w", err)
	}

	js.listener = listener
	js.running = true

	go js.acceptLoop()

	return nil
}

// Stop halts the Jitter RPC server
func (js *Server) Stop() {
	js.mu.Lock()
	defer js.mu.Unlock()

	if !js.running {
		return
	}

	if js.listener != nil {
		js.listener.Close()
	}
	js.running = false
}

func (js *Server) acceptLoop() {
	for {
		conn, err := js.listener.Accept()
		if err != nil {
			if js.running {
				// Only log errors if we're actually running
			}
			return
		}

		go js.handleConnection(conn)
	}
}

func (js *Server) handleConnection(conn net.Conn) {
	defer conn.Close()

	// Protocol: [Slots 48 bytes] + [Hash 4 bytes] + [Pass 4 bytes] = 56 bytes
	buf := make([]byte, 56)
	resp := make([]byte, 4)

	for {
		// Read 56-byte request from Simulator/ASIC
		_, err := conn.Read(buf)
		if err != nil {
			return
		}

		// Extract slots
		var slots [12]uint32
		for i := 0; i < 12; i++ {
			slots[i] = binary.LittleEndian.Uint32(buf[i*4:])
		}
		
		// Extract hash and pass
		hash := binary.LittleEndian.Uint32(buf[48:52])
		pass := int(binary.LittleEndian.Uint32(buf[52:56]))

		// Get jitter from Host logic (FlashManager)
		jitter := js.jitterFunc(slots, hash, pass)

		// Send 4-byte jitter back
		binary.LittleEndian.PutUint32(resp, jitter)
		_, err = conn.Write(resp)
		if err != nil {
			return
		}
	}
}
