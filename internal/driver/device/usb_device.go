//go:build !mips && !mipsle
// +build !mips,!mipsle

// internal/driver/device/usb_device.go
// USB-based communication with Bitmain ASIC hardware
// Bypasses the kernel module by using direct USB access
// NOTE: This file is excluded on MIPS builds due to gousb dependency

package device

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"log"
	"time"

	"github.com/google/gousb"
)

// CRC lookup tables from Bitmain protocol
var chCRCHTalbe = [256]uint8{
	0x00, 0xC1, 0x81, 0x40, 0x01, 0xC0, 0x80, 0x41, 0x01, 0xC0, 0x80, 0x41,
	0x00, 0xC1, 0x81, 0x40, 0x01, 0xC0, 0x80, 0x41, 0x00, 0xC1, 0x81, 0x40,
	0x00, 0xC1, 0x81, 0x40, 0x01, 0xC0, 0x80, 0x41, 0x01, 0xC0, 0x80, 0x41,
	0x00, 0xC1, 0x81, 0x40, 0x00, 0xC1, 0x81, 0x40, 0x01, 0xC0, 0x80, 0x41,
	0x00, 0xC1, 0x81, 0x40, 0x01, 0xC0, 0x80, 0x41, 0x01, 0xC0, 0x80, 0x41,
	0x00, 0xC1, 0x81, 0x40, 0x01, 0xC0, 0x80, 0x41, 0x00, 0xC1, 0x81, 0x40,
	0x00, 0xC1, 0x81, 0x40, 0x01, 0xC0, 0x80, 0x41, 0x00, 0xC1, 0x81, 0x40,
	0x01, 0xC0, 0x80, 0x41, 0x01, 0xC0, 0x80, 0x41, 0x00, 0xC1, 0x81, 0x40,
	0x00, 0xC1, 0x81, 0x40, 0x01, 0xC0, 0x80, 0x41, 0x01, 0xC0, 0x80, 0x41,
	0x00, 0xC1, 0x81, 0x40, 0x01, 0xC0, 0x80, 0x41, 0x00, 0xC1, 0x81, 0x40,
	0x00, 0xC1, 0x81, 0x40, 0x01, 0xC0, 0x80, 0x41, 0x01, 0xC0, 0x80, 0x41,
	0x00, 0xC1, 0x81, 0x40, 0x00, 0xC1, 0x81, 0x40, 0x01, 0xC0, 0x80, 0x41,
	0x00, 0xC1, 0x81, 0x40, 0x01, 0xC0, 0x80, 0x41, 0x01, 0xC0, 0x80, 0x41,
	0x00, 0xC1, 0x81, 0x40, 0x00, 0xC1, 0x81, 0x40, 0x01, 0xC0, 0x80, 0x41,
	0x01, 0xC0, 0x80, 0x41, 0x00, 0xC1, 0x81, 0x40, 0x01, 0xC0, 0x80, 0x41,
	0x00, 0xC1, 0x81, 0x40, 0x00, 0xC1, 0x81, 0x40, 0x01, 0xC0, 0x80, 0x41,
	0x00, 0xC1, 0x81, 0x40, 0x01, 0xC0, 0x80, 0x41, 0x01, 0xC0, 0x80, 0x41,
	0x00, 0xC1, 0x81, 0x40, 0x01, 0xC0, 0x80, 0x41, 0x00, 0xC1, 0x81, 0x40,
	0x00, 0xC1, 0x81, 0x40, 0x01, 0xC0, 0x80, 0x41, 0x01, 0xC0, 0x80, 0x41,
	0x00, 0xC1, 0x81, 0x40, 0x00, 0xC1, 0x81, 0x40, 0x01, 0xC0, 0x80, 0x41,
	0x00, 0xC1, 0x81, 0x40, 0x01, 0xC0, 0x80, 0x41, 0x01, 0xC0, 0x80, 0x41,
	0x00, 0xC1, 0x81, 0x40,
}

var chCRCLTalbe = [256]uint8{
	0x00, 0xC0, 0xC1, 0x01, 0xC3, 0x03, 0x02, 0xC2, 0xC6, 0x06, 0x07, 0xC7,
	0x05, 0xC5, 0xC4, 0x04, 0xCC, 0x0C, 0x0D, 0xCD, 0x0F, 0xCF, 0xCE, 0x0E,
	0x0A, 0xCA, 0xCB, 0x0B, 0xC9, 0x09, 0x08, 0xC8, 0xD8, 0x18, 0x19, 0xD9,
	0x1B, 0xDB, 0xDA, 0x1A, 0x1E, 0xDE, 0xDF, 0x1F, 0xDD, 0x1D, 0x1C, 0xDC,
	0x14, 0xD4, 0xD5, 0x15, 0xD7, 0x17, 0x16, 0xD6, 0xD2, 0x12, 0x13, 0xD3,
	0x11, 0xD1, 0xD0, 0x10, 0xF0, 0x30, 0x31, 0xF1, 0x33, 0xF3, 0xF2, 0x32,
	0x36, 0xF6, 0xF7, 0x37, 0xF5, 0x35, 0x34, 0xF4, 0x3C, 0xFC, 0xFD, 0x3D,
	0xFF, 0x3F, 0x3E, 0xFE, 0xFA, 0x3A, 0x3B, 0xFB, 0x39, 0xF9, 0xF8, 0x38,
	0x28, 0xE8, 0xE9, 0x29, 0xEB, 0x2B, 0x2A, 0xEA, 0xEE, 0x2E, 0x2F, 0xEF,
	0x2D, 0xED, 0xEC, 0x2C, 0xE4, 0x24, 0x25, 0xE5, 0x27, 0xE7, 0xE6, 0x26,
	0x22, 0xE2, 0xE3, 0x23, 0xE1, 0x21, 0x20, 0xE0, 0xA0, 0x60, 0x61, 0xA1,
	0x63, 0xA3, 0xA2, 0x62, 0x66, 0xA6, 0xA7, 0x67, 0xA5, 0x65, 0x64, 0xA4,
	0x6C, 0xAC, 0xAD, 0x6D, 0xAF, 0x6F, 0x6E, 0xAE, 0xAA, 0x6A, 0x6B, 0xAB,
	0x69, 0xA9, 0xA8, 0x68, 0x78, 0xB8, 0xB9, 0x79, 0xBB, 0x7B, 0x7A, 0xBA,
	0xBE, 0x7E, 0x7F, 0xBF, 0x7D, 0xBD, 0xBC, 0x7C, 0xB4, 0x74, 0x75, 0xB5,
	0x77, 0xB7, 0xB6, 0x76, 0x72, 0xB2, 0xB3, 0x73, 0xB1, 0x71, 0x70, 0xB0,
	0x50, 0x90, 0x91, 0x51, 0x93, 0x53, 0x52, 0x92, 0x96, 0x56, 0x57, 0x97,
	0x55, 0x95, 0x94, 0x54, 0x9C, 0x5C, 0x5D, 0x9D, 0x5F, 0x9F, 0x9E, 0x5E,
	0x5A, 0x9A, 0x9B, 0x5B, 0x99, 0x59, 0x58, 0x98, 0x88, 0x48, 0x49, 0x89,
	0x4B, 0x8B, 0x8A, 0x4A, 0x4E, 0x8E, 0x8F, 0x4F, 0x8D, 0x4D, 0x4C, 0x8C,
	0x44, 0x84, 0x85, 0x45, 0x87, 0x47, 0x46, 0x86, 0x82, 0x42, 0x43, 0x83,
	0x41, 0x81, 0x80, 0x40,
}

// CalculateCRC16 computes CRC-16 using Bitmain's Modbus-style lookup tables
func CalculateCRC16(data []byte) uint16 {
	chCRCHi := uint8(0xFF)
	chCRCLo := uint8(0xFF)

	for _, b := range data {
		wIndex := chCRCLo ^ b
		chCRCLo = chCRCHi ^ chCRCHTalbe[wIndex]
		chCRCHi = chCRCLTalbe[wIndex]
	}

	return (uint16(chCRCHi) << 8) | uint16(chCRCLo)
}

// computeMidstate computes the SHA-256 midstate for the first 64 bytes
func computeMidstate(header []byte) [32]byte {
	if len(header) < 64 {
		padded := make([]byte, 64)
		copy(padded, header)
		header = padded
	}

	h := sha256.New()
	h.Write(header[:64])
	sum := h.Sum(nil)
	var midstate [32]byte
	copy(midstate[:], sum)
	return midstate
}

// BuildTxTaskFromHeader constructs a TxTask packet from an 80-byte mining header
func BuildTxTaskFromHeader(header []byte, workID uint8) []byte {
	const (
		TokenTxTask = 0x52
		taskSize    = 45 // work_id(1) + midstate(32) + data(12)
	)

	packet := make([]byte, 4+1+taskSize+2) // 52 bytes total

	// Header (4 bytes)
	packet[0] = TokenTxTask
	packet[1] = 0x00                               // Version
	binary.LittleEndian.PutUint16(packet[2:4], 46) // Length = work_num(1) + ASIC_TASK(45)

	// work_num (1 byte)
	packet[4] = 0x01

	// ASIC_TASK (45 bytes)
	// work_id (1 byte)
	packet[5] = workID

	// midstate[32] - compute SHA-256 midstate for first 64 bytes
	midstate := computeMidstate(header)
	copy(packet[6:38], midstate[:])

	// data[12] - last 12 bytes relevant for mining
	// These are: merkle_root_suffix(4) + timestamp(4) + nBits(4)
	// From header bytes 64-75 (before nonce at 76-79)
	if len(header) >= 76 {
		copy(packet[38:50], header[64:76])
	}

	// Calculate and append CRC (covers bytes 0-49)
	crc := CalculateCRC16(packet[:50])
	binary.LittleEndian.PutUint16(packet[50:52], crc)

	return packet
}

// ParseRxNonce parses an RxNonce response from the ASIC
func ParseRxNonce(data []byte) (workID uint8, nonce uint32, chainNum uint8, ok bool) {
	// RxNonce structure (0xA2):
	// [data_type(1)][version(1)][length(2)][fifo_space(2)][nonce_num(1)][reserved(1)]
	// [nonce_data...]
	// Each nonce_data: [work_id(1)][nonce(4)][chain_num(1)][reserved(2)]

	if len(data) < 16 {
		return 0, 0, 0, false
	}

	// Verify data type
	if data[0] != 0xA2 {
		return 0, 0, 0, false
	}

	// Get nonce count
	nonceNum := data[6]
	if nonceNum == 0 {
		return 0, 0, 0, false
	}

	// Parse first nonce entry (offset 8)
	if len(data) < 16 {
		return 0, 0, 0, false
	}

	workID = data[8]
	nonce = binary.LittleEndian.Uint32(data[9:13])
	chainNum = data[13]

	return workID, nonce, chainNum, true
}

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
