//go:build !mips && !mipsle
// +build !mips,!mipsle

// internal/driver/device/usb_device.go
// USB-based communication with Bitmain ASIC hardware
// Bypasses the kernel module by using direct USB access
// NOTE: This file is excluded on MIPS builds due to gousb dependency

package device

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/google/gousb"
)

// USBDevice provides direct USB communication with ASIC
type USBDevice struct {
	ctx       *gousb.Context
	device    *gousb.Device
	config    *gousb.Config
	intf      *gousb.Interface
	epOut     *gousb.OutEndpoint
	epIn      *gousb.InEndpoint
	chipCount int
}

// OpenUSBDevice opens the ASIC via direct USB access
// This bypasses the kernel module completely
func OpenUSBDevice() (*USBDevice, error) {
	ctx := gousb.NewContext()

	// Open device by vendor/product ID
	device, err := ctx.OpenDeviceWithVIDPID(USBVendorID, USBProductID)
	if err != nil {
		ctx.Close()
		return nil, fmt.Errorf("failed to open USB device: %w", err)
	}

	if device == nil {
		ctx.Close()
		return nil, fmt.Errorf("USB device not found (VID:0x%04x PID:0x%04x)", USBVendorID, USBProductID)
	}

	// Set configuration
	config, err := device.Config(1)
	if err != nil {
		device.Close()
		ctx.Close()
		return nil, fmt.Errorf("failed to set USB config: %w", err)
	}

	// Claim interface
	intf, err := config.Interface(0, 0)
	if err != nil {
		config.Close()
		device.Close()
		ctx.Close()
		return nil, fmt.Errorf("failed to claim USB interface: %w", err)
	}

	// Get endpoints
	// Endpoint 0x01 (OUT) - for sending commands
	// Endpoint 0x81 (IN) - for receiving data
	epOut, err := intf.OutEndpoint(EndpointOut)
	if err != nil {
		intf.Close()
		config.Close()
		device.Close()
		ctx.Close()
		return nil, fmt.Errorf("failed to open OUT endpoint: %w", err)
	}

	epIn, err := intf.InEndpoint(EndpointIn)
	if err != nil {
		intf.Close()
		config.Close()
		device.Close()
		ctx.Close()
		return nil, fmt.Errorf("failed to open IN endpoint: %w", err)
	}

	usbDev := &USBDevice{
		ctx:       ctx,
		device:    device,
		config:    config,
		intf:      intf,
		epOut:     epOut,
		epIn:      epIn,
		chipCount: 32, // Antminer S3 has 32 chips
	}

	log.Printf("Successfully opened USB device (bypassing kernel module)")
	return usbDev, nil
}

// Close closes the USB device
func (d *USBDevice) Close() error {
	if d.intf != nil {
		d.intf.Close()
	}
	if d.config != nil {
		d.config.Close()
	}
	if d.device != nil {
		d.device.Close()
	}
	if d.ctx != nil {
		d.ctx.Close()
	}
	return nil
}

// claimInterface ensures the USB interface is claimed for operations
func (d *USBDevice) claimInterface() error {
	// Check if interface is already claimed
	if d.intf != nil {
		return nil
	}

	// Re-claim the interface if needed
	intf, err := d.config.Interface(0, 0)
	if err != nil {
		return fmt.Errorf("failed to claim USB interface: %w", err)
	}
	d.intf = intf

	// Re-open endpoints after claiming interface
	epOut, err := d.intf.OutEndpoint(EndpointOut)
	if err != nil {
		return fmt.Errorf("failed to open OUT endpoint: %w", err)
	}
	d.epOut = epOut

	epIn, err := d.intf.InEndpoint(EndpointIn)
	if err != nil {
		return fmt.Errorf("failed to open IN endpoint: %w", err)
	}
	d.epIn = epIn

	return nil
}

// releaseInterface releases the USB interface
func (d *USBDevice) releaseInterface() error {
	if d.intf == nil {
		return nil
	}

	// Close the interface but keep config and device open
	d.intf.Close()
	d.intf = nil
	d.epOut = nil
	d.epIn = nil
	return nil
}

// SendPacket sends a packet to the ASIC via USB
func (d *USBDevice) SendPacket(data []byte) error {
	_, err := d.epOut.Write(data)
	if err != nil {
		return fmt.Errorf("USB write failed: %w", err)
	}
	return nil
}

// ReadPacket reads a packet from the ASIC via USB with optimized timing
func (d *USBDevice) ReadPacket(buffer []byte, timeout time.Duration) (int, error) {
	// Use a single, well-timed read for ASIC communication
	// The ASIC typically responds within 100-500ms for Difficulty 1 targets
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	n, err := d.epIn.ReadContext(ctx, buffer)
	if err != nil {
		return 0, fmt.Errorf("USB read failed: %w", err)
	}
	return n, nil
}

// SendTxTaskAndReadRxNonce sends a TxTask packet to the ASIC and reads the RxNonce response
func (d *USBDevice) SendTxTaskAndReadRxNonce(header []byte, workID uint8, timeout time.Duration) (uint32, error) {
	if len(header) != 80 {
		return 0, fmt.Errorf("mining header must be exactly 80 bytes, got %d", len(header))
	}

	// Ensure interface is claimed for this operation
	if err := d.claimInterface(); err != nil {
		return 0, fmt.Errorf("failed to claim USB interface: %w", err)
	}
	defer d.releaseInterface()

	// 1. Construct the TxTask packet
	txTaskPacket := BuildTxTaskFromHeader(header, workID)

	// 2. Send TxTask packet
	log.Printf("Sending TxTask packet: %x", txTaskPacket)
	if err := d.SendPacket(txTaskPacket); err != nil {
		return 0, fmt.Errorf("failed to send TxTask packet: %w", err)
	}
	log.Printf("TxTask packet sent successfully")

	// 3. Read RxNonce response
	// For Difficulty 1, ASIC should find nonce very quickly (typically <100ms)
	// Use the full timeout for this single read attempt
	rxNonceBuffer := make([]byte, MaxUSBPacketSize)
	n, err := d.ReadPacket(rxNonceBuffer, timeout)
	if err != nil {
		return 0, fmt.Errorf("failed to read RxNonce response: %w", err)
	}

	// 4. Parse RxNonce to extract the found nonce
	_, nonce, _, ok := ParseRxNonce(rxNonceBuffer[:n])
	if !ok {
		return 0, fmt.Errorf("failed to parse RxNonce response")
	}

	return nonce, nil
}

// GetChipCount returns the number of ASIC chips
func (d *USBDevice) GetChipCount() int {
	return d.chipCount
}

// Initialize performs USB-based ASIC initialization
func (d *USBDevice) Initialize() error {
	// Send RxStatus packet to query device state
	rxStatusPacket := d.buildRxStatusPacket()
	if err := d.SendPacket(rxStatusPacket); err != nil {
		return fmt.Errorf("failed to send RxStatus: %w", err)
	}

	log.Printf("Sent RxStatus packet via USB")

	// Send TxConfig packet to configure ASICs
	txConfigPacket := d.buildTxConfigPacket()
	if err := d.SendPacket(txConfigPacket); err != nil {
		return fmt.Errorf("failed to send TxConfig: %w", err)
	}

	log.Printf("Sent TxConfig packet via USB")

	// Wait a moment for configuration to take effect
	time.Sleep(InitDelay)

	// Send verification RxStatus packet
	if err := d.SendPacket(rxStatusPacket); err != nil {
		return fmt.Errorf("failed to send verification RxStatus: %w", err)
	}

	log.Printf("USB-based ASIC initialization complete: %d chips", d.chipCount)
	return nil
}

// buildTxConfigPacket builds the TxConfig packet
func (d *USBDevice) buildTxConfigPacket() []byte {
	packet := make([]byte, 28)

	// Header (4 bytes)
	packet[0] = TokenTxConfig
	packet[1] = 0x00
	packet[2] = 22 // length low
	packet[3] = 0  // length high

	// Payload (22 bytes)
	packet[4] = 0x1E
	packet[5] = 0x00
	packet[6] = 0x0C
	packet[7] = 0x00
	packet[8] = 8  // chain_num
	packet[9] = 32 // asic_num
	packet[10] = 0x60
	packet[11] = 0x0C
	packet[12] = 250 // frequency low
	packet[13] = 0   // frequency high
	packet[14] = 0x82
	packet[15] = 0x09

	// CRC (2 bytes)
	crc := CalculateCRC16(packet[:26])
	packet[26] = byte(crc & 0xFF)
	packet[27] = byte(crc >> 8)

	return packet
}

// buildRxStatusPacket builds the RxStatus packet
func (d *USBDevice) buildRxStatusPacket() []byte {
	packet := make([]byte, 16)

	// Header (4 bytes)
	packet[0] = TokenRxStatus
	packet[1] = 0x00
	packet[2] = 10 // length low
	packet[3] = 0  // length high

	// Payload (10 bytes) - mostly zeros
	for i := 4; i < 14; i++ {
		packet[i] = 0x00
	}

	// CRC (2 bytes)
	crc := CalculateCRC16(packet[:14])
	packet[14] = byte(crc & 0xFF)
	packet[15] = byte(crc >> 8)

	return packet
}

// IsAvailable checks if USB device is available
func IsUSBDeviceAvailable() bool {
	ctx := gousb.NewContext()
	defer ctx.Close()

	device, err := ctx.OpenDeviceWithVIDPID(USBVendorID, USBProductID)
	if err != nil || device == nil {
		return false
	}

	device.Close()
	return true
}
