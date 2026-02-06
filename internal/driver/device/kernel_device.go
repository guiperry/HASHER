// internal/driver/device/kernel_device.go
// Direct kernel driver interface for Bitmain ASIC
// Uses /dev/bitmain-asic with proper ioctl or write/read

package device

import (
	"encoding/binary"
	"fmt"
	"log"
	"os"
	"time"
)

// KernelDevice provides direct access via the bitmain_asic kernel driver
type KernelDevice struct {
	file      *os.File
	chipCount int
}

// OpenKernelDevice opens the ASIC via the kernel driver
func OpenKernelDevice() (*KernelDevice, error) {
	// Open the device
	file, err := os.OpenFile("/dev/bitmain-asic", os.O_RDWR, 0)
	if err != nil {
		return nil, fmt.Errorf("failed to open kernel device: %w", err)
	}

	dev := &KernelDevice{
		file:      file,
		chipCount: 32, // Antminer S3 has 32 chips
	}

	log.Printf("Opened kernel device /dev/bitmain-asic")

	// Initialize the ASIC
	if err := dev.Initialize(); err != nil {
		file.Close()
		return nil, fmt.Errorf("failed to initialize ASIC: %w", err)
	}

	return dev, nil
}

// Initialize performs ASIC initialization via kernel driver
func (d *KernelDevice) Initialize() error {
	log.Printf("Initializing ASIC via kernel driver...")

	// Send TxConfig packet first (as per protocol)
	txConfig := d.buildTxConfigPacket()
	log.Printf("Sending TxConfig (%d bytes)...", len(txConfig))
	if err := d.SendPacket(txConfig); err != nil {
		return fmt.Errorf("failed to send TxConfig: %w", err)
	}

	// Wait for initialization
	time.Sleep(1 * time.Second)

	// Send RxStatus to verify
	rxStatus := d.buildRxStatusPacket()
	log.Printf("Sending RxStatus (%d bytes)...", len(rxStatus))
	if err := d.SendPacket(rxStatus); err != nil {
		return fmt.Errorf("failed to send RxStatus: %w", err)
	}

	// Try to read response
	response := make([]byte, 256)
	n, err := d.ReadPacket(response, 2*time.Second)
	if err != nil {
		log.Printf("RxStatus read timeout (expected): %v", err)
	} else if n > 0 {
		log.Printf("RxStatus response: %d bytes, type=0x%02x", n, response[0])
	}

	log.Printf("ASIC initialization complete")
	return nil
}

// buildTxConfigPacket creates the TxConfig packet for ASIC initialization
func (d *KernelDevice) buildTxConfigPacket() []byte {
	// TxConfig: [token(1)][version(1)][length(2)][payload(22)][crc(2)]
	packet := make([]byte, 28)

	// Header
	packet[0] = 0x51                               // TokenTxConfig
	packet[1] = 0x00                               // Version
	binary.LittleEndian.PutUint16(packet[2:4], 22) // Payload length

	// Control flags
	packet[4] = 0x1E // fan, timeout, frequency, voltage enabled
	packet[5] = 0x00
	packet[6] = 0x0C // chain_check_time
	packet[7] = 0x00

	// ASIC configuration
	packet[8] = 8     // chain_num
	packet[9] = 32    // asic_num
	packet[10] = 0x60 // fan_pwm_data
	packet[11] = 0x0C // timeout_data

	// Frequency: 250 MHz
	binary.LittleEndian.PutUint16(packet[12:14], 250)

	// Voltage: 0x0982
	binary.LittleEndian.PutUint16(packet[14:16], 0x0982)

	// Register data
	packet[16] = 0x00
	packet[17] = 0x00
	packet[18] = 0x00
	packet[19] = 0x00

	// Address fields
	packet[20] = 0x00
	packet[21] = 0x00

	// Padding
	packet[22] = 0x00
	packet[23] = 0x00
	packet[24] = 0x00
	packet[25] = 0x00

	// CRC
	crc := CalculateCRC16(packet[:26])
	binary.LittleEndian.PutUint16(packet[26:28], crc)

	return packet
}

// buildRxStatusPacket creates the RxStatus packet
func (d *KernelDevice) buildRxStatusPacket() []byte {
	// RxStatus: [token(1)][version(1)][length(2)][payload(10)][crc(2)]
	packet := make([]byte, 16)

	// Header
	packet[0] = 0x53                               // TokenRxStatus
	packet[1] = 0x00                               // Version
	binary.LittleEndian.PutUint16(packet[2:4], 10) // Payload length

	// Payload
	packet[4] = 0x00
	packet[5] = 0x00
	packet[6] = 0x00
	packet[7] = 0x00
	packet[8] = 0x00
	packet[9] = 0x00
	packet[10] = 0x00
	packet[11] = 0x00
	packet[12] = 0x00
	packet[13] = 0x00

	// CRC
	crc := CalculateCRC16(packet[:14])
	binary.LittleEndian.PutUint16(packet[14:16], crc)

	return packet
}

// SendPacket writes data to the kernel device
func (d *KernelDevice) SendPacket(data []byte) error {
	if d.file == nil {
		return fmt.Errorf("device not open")
	}

	_, err := d.file.Write(data)
	if err != nil {
		return fmt.Errorf("write failed: %w", err)
	}

	return nil
}

// ReadPacket reads data from the kernel device with timeout
func (d *KernelDevice) ReadPacket(buffer []byte, timeout time.Duration) (int, error) {
	if d.file == nil {
		return 0, fmt.Errorf("device not open")
	}

	// Set read deadline (ignore error if not supported)
	if err := d.file.SetReadDeadline(time.Now().Add(timeout)); err != nil {
		// Character devices often don't support deadlines, so ignore this error
		log.Printf("Warning: cannot set read deadline on device (continuing): %v", err)
	}

	n, err := d.file.Read(buffer)
	if err != nil {
		return 0, fmt.Errorf("read failed: %w", err)
	}

	return n, nil
}

// Close closes the kernel device
func (d *KernelDevice) Close() error {
	if d.file != nil {
		return d.file.Close()
	}
	return nil
}

// GetChipCount returns the number of chips
func (d *KernelDevice) GetChipCount() int {
	return d.chipCount
}

// IsKernelDeviceAvailable checks if the kernel device is accessible
func IsKernelDeviceAvailable() bool {
	_, err := os.Stat("/dev/bitmain-asic")
	return err == nil
}
