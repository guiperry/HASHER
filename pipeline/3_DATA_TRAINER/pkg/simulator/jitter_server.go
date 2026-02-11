package simulator

import (
	"encoding/binary"
	"fmt"
	"net"
	"os"
	"sync"
)

// JitterServer handles the high-speed Jitter RPC via Unix Domain Socket
type JitterServer struct {
	socketPath string
	listener   net.Listener
	mu         sync.Mutex
	running    bool
	jitterFunc func(uint32) uint32
}

func NewJitterServer(socketPath string, jitterFunc func(uint32) uint32) *JitterServer {
	return &JitterServer{
		socketPath: socketPath,
		jitterFunc: jitterFunc,
	}
}

func (js *JitterServer) Start() error {
	js.mu.Lock()
	defer js.mu.Unlock()

	if js.running {
		return nil
	}

	// Cleanup old socket
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

	fmt.Printf("[JITTER] Server started on %s\n", js.socketPath)
	return nil
}

func (js *JitterServer) Stop() {
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

func (js *JitterServer) acceptLoop() {
	for {
		conn, err := js.listener.Accept()
		if err != nil {
			if js.running {
				fmt.Printf("[JITTER] Accept error: %v\n", err)
			}
			return
		}

		go js.handleConnection(conn)
	}
}

func (js *JitterServer) handleConnection(conn net.Conn) {
	defer conn.Close()

	buf := make([]byte, 4)
	resp := make([]byte, 4)

	for {
		// Read 4-byte hash from Simulator
		_, err := conn.Read(buf)
		if err != nil {
			return
		}

		hash := binary.LittleEndian.Uint32(buf)

		// Get jitter from Host (FlashManager)
		jitter := js.jitterFunc(hash)

		binary.LittleEndian.PutUint32(resp, jitter)

		// Send 4-byte jitter back
		_, err = conn.Write(resp)
		if err != nil {
			return
		}
	}
}
