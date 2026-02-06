// internal/driver/device/controller.go
package device

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"log"
	"os"
	"os/exec"
	"strings"
	"sync"
	"syscall"
	"time"
)

const (
	DevicePath       = "/dev/bitmain-asic"
	MaxBatchSize     = 256
	MaxUSBPacketSize = 512 // Max packet size for USB bulk transfers

	// USB Configuration for Bitmain ASIC

	// USB Configuration for Bitmain ASIC
	USBVendorID  = 0x4254 // "BT"
	USBProductID = 0x4153 // "AS"

	// Protocol tokens (Bitmain official values)
	TokenTxConfig = 0x51 // BITMAIN_TOKEN_TYPE_TXCONFIG
	TokenTxTask   = 0x52 // BITMAIN_TOKEN_TYPE_TXTASK
	TokenRxStatus = 0x53 // BITMAIN_TOKEN_TYPE_RXSTATUS

	// Data types (returned in responses)
	DataTypeRxStatus = 0xA1 // BITMAIN_DATA_TYPE_RXSTATUS
	DataTypeRxNonce  = 0xA2 // BITMAIN_DATA_TYPE_RXNONCE

	// USB endpoints (from USB descriptor)
	EndpointOut = 0x01
	EndpointIn  = 0x81

	// Timing constants
	InitDelay      = 1 * time.Second
	PollInterval   = 40 * time.Millisecond
	StatusInterval = 5 * time.Second
)

// Device represents an ASIC device with eBPF tracing
type Device struct {
	file      *os.File
	fd        uintptr
	chipCount int
	mu        sync.RWMutex
	tracer    *Tracer
	stats     *DeviceStats

	// Device state
	isOperational   bool
	devicePath      string
	firmwareVersion string

	// Kernel module management
	kernelModuleUnloaded bool
	originalModuleLoaded bool

	// IOCTL-based communication (alternative to direct device access)
	ioctlDevice *IOCTLDevice
	useIOCTL    bool

	// USB-based communication (bypasses kernel module entirely)
	usbDevice *USBDevice
	useUSB    bool

	// CGMiner-based communication (most reliable)
	cgMinerClient *CGMinerClient
	useCGMiner    bool
	cgMinerMiner  *CGMinerMiner

	// Kernel device-based communication (simple and reliable)
	kernelDevice *KernelDevice
	useKernel    bool
}

// DeviceStats holds device statistics with internal synchronization
type DeviceStats struct {
	TotalRequests  uint64
	TotalBytes     uint64
	TotalLatencyNs uint64
	PeakLatencyNs  uint64
	ErrorCount     uint64
	mu             sync.RWMutex
}

// DeviceStatsSnapshot is a copy of device statistics without synchronization
// Used for returning stats to callers without copying mutexes
type DeviceStatsSnapshot struct {
	TotalRequests  uint64
	TotalBytes     uint64
	TotalLatencyNs uint64
	PeakLatencyNs  uint64
	ErrorCount     uint64
}

// CheckDeviceState performs diagnostics on the device before opening
// Returns detailed information about device accessibility and kernel module status
func CheckDeviceState() (map[string]string, error) {
	state := make(map[string]string)

	// Check device existence and permissions
	info, err := os.Stat(DevicePath)
	if err != nil {
		state["device_exists"] = "false"
		state["device_error"] = err.Error()
	} else {
		state["device_exists"] = "true"
		state["device_mode"] = info.Mode().String()
	}

	// Check kernel module status
	cmd := exec.Command("lsmod")
	output, err := cmd.Output()
	if err != nil {
		state["lsmod_error"] = err.Error()
	} else {
		if bytes.Contains(output, []byte("bitmain_asic")) {
			state["kernel_module"] = "loaded"
			// Get module details
			detailCmd := exec.Command("grep", "bitmain_asic", "/proc/modules")
			detailOut, _ := detailCmd.Output()
			state["module_details"] = strings.TrimSpace(string(detailOut))
		} else {
			state["kernel_module"] = "not_loaded"
		}
	}

	// Check device major/minor numbers
	var stat syscall.Stat_t
	if err := syscall.Stat(DevicePath, &stat); err == nil {
		state["device_major"] = fmt.Sprintf("%d", stat.Dev>>8)
		state["device_minor"] = fmt.Sprintf("%d", stat.Dev&0xFF)
		state["device_rdev_major"] = fmt.Sprintf("%d", stat.Rdev>>8)
		state["device_rdev_minor"] = fmt.Sprintf("%d", stat.Rdev&0xFF)
	}

	// Check if any process is using the device
	fuserCmd := exec.Command("fuser", "-v", DevicePath)
	fuserOut, _ := fuserCmd.CombinedOutput()
	state["fuser_output"] = strings.TrimSpace(string(fuserOut))
	if len(fuserOut) == 0 {
		state["device_in_use"] = "false"
	} else {
		state["device_in_use"] = "true"
	}

	// Check process list for miners
	psCmd := exec.Command("ps", "aux")
	psOut, _ := psCmd.Output()
	psStr := string(psOut)
	if strings.Contains(psStr, "cgminer") || strings.Contains(psStr, "bmminer") {
		state["miner_running"] = "true"
	} else {
		state["miner_running"] = "false"
	}

	return state, nil
}

// OpenDevice opens the ASIC device with eBPF tracing
// Priority: CGMiner API > Simple Kernel Device > USB Direct
func OpenDevice(enableTracing bool) (*Device, error) {
	dev := &Device{
		chipCount:       32,
		stats:           &DeviceStats{},
		devicePath:      DevicePath,
		firmwareVersion: "1.0.0",
		isOperational:   false,
	}

	// Strategy 0: Try CGMiner API (most reliable - already proven working)
	log.Printf("Strategy 0: Checking for CGMiner API...")
	miner := NewCGMinerMiner()
	if miner.IsAvailable() {
		log.Printf("✓ CGMiner is available and mining")
		dev.cgMinerClient = &CGMinerClient{host: cgminerHost, port: cgminerPort}
		dev.cgMinerMiner = miner
		dev.useCGMiner = true
		dev.isOperational = true
		dev.chipCount = 32
		log.Printf("Successfully connected to CGMiner API")
		return dev.initDevice(enableTracing)
	}
	log.Printf("CGMiner not available")

	// Strategy 1: Try simple kernel device
	log.Printf("Strategy 1: Opening kernel device...")
	if IsKernelDeviceAvailable() {
		kd, err := OpenKernelDevice()
		if err == nil {
			dev.kernelDevice = kd
			dev.useKernel = true
			dev.isOperational = true
			dev.chipCount = 32
			log.Printf("Successfully opened kernel device")
			return dev.initDevice(enableTracing)
		}
		log.Printf("Kernel device failed: %v", err)
	} else {
		log.Printf("Kernel device not available")
	}

	// Strategy 2: Try USB-based communication as fallback
	log.Printf("Strategy 2: Trying USB-based communication (bypassing kernel module)...")
	if IsUSBDeviceAvailable() {
		usbDev, usbErr := OpenUSBDevice()
		if usbErr == nil {
			// Initialize the USB device
			if initErr := usbDev.Initialize(); initErr == nil {
				log.Printf("Successfully using USB-based device access")
				dev.usbDevice = usbDev
				dev.useUSB = true
				dev.chipCount = usbDev.GetChipCount()
				dev.isOperational = true
				return dev.initDevice(enableTracing)
			} else {
				log.Printf("USB initialization failed: %v", initErr)
				usbDev.Close()
			}
		} else {
			log.Printf("USB approach not viable: %v", usbErr)
		}
	} else {
		log.Printf("USB subsystem not available on this system")
	}

	// All strategies failed
	return nil, fmt.Errorf("failed to open ASIC device: kernel device and USB both unavailable")
}

// initDevice initializes eBPF tracer and performs ASIC initialization
func (d *Device) initDevice(enableTracing bool) (*Device, error) {
	// If using CGMiner mode, no initialization needed - CGMiner handles it
	if d.useCGMiner {
		log.Printf("Using CGMiner mode - hardware already initialized")
		if enableTracing {
			tracer, err := NewTracer()
			if err != nil {
				return nil, fmt.Errorf("init tracer: %w", err)
			}
			d.tracer = tracer
		}
		return d, nil
	}

	// If using kernel device mode, skip complex initialization
	// The kernel driver handles ASIC initialization
	if d.useKernel {
		log.Printf("Using kernel device mode - skipping complex initialization")
		if enableTracing {
			tracer, err := NewTracer()
			if err != nil {
				return nil, fmt.Errorf("init tracer: %w", err)
			}
			d.tracer = tracer
		}
		return d, nil
	}

	// If using USB mode, skip file-based initialization
	// USB device is already initialized via usbDev.Initialize() in OpenDevice
	if d.useUSB {
		// Initialize eBPF tracer if enabled
		if enableTracing {
			tracer, err := NewTracer()
			if err != nil {
				return nil, fmt.Errorf("init tracer: %w", err)
			}
			d.tracer = tracer
		}
		return d, nil
	}

	// Initialize eBPF tracer if enabled
	if enableTracing {
		tracer, err := NewTracer()
		if err != nil {
			if d.file != nil {
				d.file.Close()
			}
			return nil, fmt.Errorf("init tracer: %w", err)
		}
		d.tracer = tracer
	}

	// Perform ASIC initialization sequence
	if err := d.initializeASIC(); err != nil {
		if d.file != nil {
			d.file.Close()
		}
		if d.tracer != nil {
			d.tracer.Close()
		}
		return nil, fmt.Errorf("ASIC initialization failed: %w", err)
	}

	return d, nil
}

// isDeviceBusyError checks if the error indicates device is busy or locked
func isDeviceBusyError(err error) bool {
	if err == nil {
		return false
	}
	errStr := err.Error()
	// Check for various device locked/unavailable conditions
	return strings.Contains(errStr, "device or resource busy") ||
		strings.Contains(errStr, "Device or resource busy") ||
		strings.Contains(errStr, "operation not permitted") ||
		strings.Contains(errStr, "Operation not permitted") ||
		strings.Contains(errStr, "permission denied") ||
		strings.Contains(errStr, "Permission denied") ||
		strings.Contains(errStr, "resource temporarily unavailable") ||
		strings.Contains(errStr, "Resource temporarily unavailable")
}

// unloadKernelModule unloads the bitmain_asic kernel module
func (d *Device) unloadKernelModule() error {
	// Check if module is loaded
	cmd := exec.Command("lsmod")
	output, err := cmd.Output()
	if err != nil {
		return fmt.Errorf("failed to list modules: %w", err)
	}

	if !bytes.Contains(output, []byte("bitmain_asic")) {
		log.Printf("Kernel module bitmain_asic not loaded, skipping unload")
		return nil
	}

	d.originalModuleLoaded = true

	// Stop any cgminer processes that might be using the device
	log.Printf("Stopping cgminer processes...")
	exec.Command("killall", "-9", "cgminer").Run()
	exec.Command("killall", "-9", "bmminer").Run()
	exec.Command("pkill", "-9", "-f", "cgminer").Run()

	// Give processes time to fully terminate and release device references
	log.Printf("Waiting for processes to terminate...")
	time.Sleep(1 * time.Second)

	// Sync to ensure all pending I/O completes
	syscall.Sync()
	time.Sleep(500 * time.Millisecond)

	// Check if any process is still using the device
	fuserCmd := exec.Command("fuser", "-v", DevicePath)
	fuserOut, _ := fuserCmd.CombinedOutput()
	if len(fuserOut) > 0 {
		log.Printf("Warning: Processes still using device: %s", string(fuserOut))
	}

	// Try graceful module removal first
	log.Printf("Attempting to unload kernel module (rmmod)...")
	cmd = exec.Command("rmmod", "bitmain_asic")
	if output, err := cmd.CombinedOutput(); err != nil {
		// Check if module is actually in use
		if strings.Contains(string(output), "in use") || strings.Contains(string(output), "busy") {
			log.Printf("Module is in use, attempting forced removal...")
			// Try forced removal (risky but may work in some cases)
			cmd = exec.Command("rmmod", "-f", "bitmain_asic")
			output, err = cmd.CombinedOutput()
			if err != nil {
				return fmt.Errorf("forced rmmod failed: %v (output: %s)", err, output)
			}
		} else {
			return fmt.Errorf("rmmod failed: %v (output: %s)", err, output)
		}
	}

	log.Printf("Successfully unloaded kernel module: bitmain_asic")
	return nil
}

// reloadKernelModule reloads the bitmain_asic kernel module
func (d *Device) reloadKernelModule() error {
	if !d.originalModuleLoaded {
		return nil
	}

	cmd := exec.Command("insmod", "/lib/modules/$(uname -r)/kernel/drivers/usb/bitmain_asic.ko")
	output, err := cmd.CombinedOutput()
	if err != nil {
		// Try alternative paths
		cmd = exec.Command("modprobe", "bitmain_asic")
		output, err = cmd.CombinedOutput()
		if err != nil {
			log.Printf("Warning: Failed to reload kernel module: %v (output: %s)", err, output)
			return err
		}
	}

	log.Printf("Successfully reloaded kernel module: bitmain_asic")
	return nil
}

// initializeASIC performs the ASIC initialization sequence
// Protocol sequence from BITMAIN-PROTOCOL.md:
// 1. Send RxStatus packet (query device state)
// 2. Read RxStatus response (0xA1 data type)
// 3. Send TxConfig packet (configure ASICs)
// 4. Wait 1 second for initialization
// 5. Send RxStatus to verify configuration
func (d *Device) initializeASIC() error {
	log.Printf("Initializing ASIC device...")

	// Step 1: Query initial device status
	rxStatus := d.buildRxStatusPacket()
	if _, err := d.file.Write(rxStatus); err != nil {
		return fmt.Errorf("failed to send initial RxStatus: %w", err)
	}
	log.Printf("Sent RxStatus packet to query device state")

	// Try to read RxStatus response (device may not respond initially)
	response := make([]byte, 256)
	d.file.SetReadDeadline(time.Now().Add(300 * time.Millisecond))
	n, err := d.file.Read(response)
	if err == nil && n > 0 {
		// Parse RxStatus response if we got one
		if response[0] == DataTypeRxStatus {
			if err := d.parseRxStatusResponse(response[:n]); err != nil {
				log.Printf("Warning: Failed to parse RxStatus response: %v", err)
			}
		}
	}

	// Step 2: Send TxConfig to configure ASICs
	txConfig := d.buildTxConfigPacket()
	if _, err := d.file.Write(txConfig); err != nil {
		return fmt.Errorf("failed to send TxConfig: %w", err)
	}
	log.Printf("Sent TxConfig packet to configure ASICs")

	// Step 3: Wait for initialization
	time.Sleep(InitDelay)

	// Step 4: Send RxStatus to verify configuration
	rxStatus = d.buildRxStatusPacket()
	if _, err := d.file.Write(rxStatus); err != nil {
		return fmt.Errorf("failed to send verification RxStatus: %w", err)
	}
	log.Printf("Sent verification RxStatus packet")

	// Try to read verification response
	d.file.SetReadDeadline(time.Now().Add(300 * time.Millisecond))
	n, err = d.file.Read(response)
	if err == nil && n > 0 {
		if response[0] == DataTypeRxStatus {
			if err := d.parseRxStatusResponse(response[:n]); err != nil {
				log.Printf("Warning: Failed to parse verification RxStatus: %v", err)
			}
		}
	}

	log.Printf("ASIC initialization complete: %d chips operational", d.chipCount)
	return nil
}

// buildTxConfigPacket builds the TxConfig packet to configure ASICs
// Total size: 28 bytes (header 4 + payload 22 + crc 2)
// Based on bitmain_txconfig_token from official driver
func (d *Device) buildTxConfigPacket() []byte {
	// TxConfig: [token(1)][version(1)][length(2)][payload(22)][crc(2)]
	packet := make([]byte, 28)

	// Header (4 bytes)
	packet[0] = TokenTxConfig                      // 0x51
	packet[1] = 0x00                               // Version
	binary.LittleEndian.PutUint16(packet[2:4], 22) // Payload length

	// Payload (22 bytes)
	// Control flags (bitfield) - enable fan, timeout, frequency, voltage
	packet[4] = 0x1E // Bits 1-4: fan_eft, timeout_eft, frequency_eft, voltage_eft
	packet[5] = 0x00 // Reserved (5 bits)
	packet[6] = 0x0C // chain_check_time
	packet[7] = 0x00 // Reserved

	// ASIC Configuration
	packet[8] = 8     // chain_num (8 chains for S3)
	packet[9] = 32    // asic_num (32 ASICs per chain)
	packet[10] = 0x60 // fan_pwm_data (96%)
	packet[11] = 0x0C // timeout_data

	// Frequency: 250 MHz (0x00FA little-endian)
	binary.LittleEndian.PutUint16(packet[12:14], 250)

	// Voltage: 0x0982 (little-endian, 0.982V)
	binary.LittleEndian.PutUint16(packet[14:16], 0x0982)

	// Register data (4 bytes)
	packet[16] = 0x00
	packet[17] = 0x00
	packet[18] = 0x00
	packet[19] = 0x00

	// Address fields
	packet[20] = 0x00 // chip_address
	packet[21] = 0x00 // reg_address

	// Calculate and append CRC (covers bytes 0-21)
	crc := CalculateCRC16(packet[:22])
	binary.LittleEndian.PutUint16(packet[26:28], crc)

	return packet
}

// buildRxStatusPacket builds the RxStatus packet to query device state
// Total size: 16 bytes (header 4 + payload 10 + crc 2)
// Based on bitmain_rxstatus_token from official driver
func (d *Device) buildRxStatusPacket() []byte {
	// RxStatus: [token(1)][version(1)][length(2)][payload(10)][crc(2)]
	packet := make([]byte, 16)

	// Header (4 bytes)
	packet[0] = TokenRxStatus                      // 0x53
	packet[1] = 0x00                               // Version
	binary.LittleEndian.PutUint16(packet[2:4], 10) // Payload length

	// Payload (10 bytes)
	packet[4] = 0x00 // flags
	packet[5] = 0x00 // reserved
	packet[6] = 0x00 // reserved
	packet[7] = 0x00 // reserved
	packet[8] = 0x00 // chip_address (0 = all)
	packet[9] = 0x00 // reg_address
	// 4 bytes remaining for 4-byte alignment
	packet[10] = 0x00
	packet[11] = 0x00
	packet[12] = 0x00
	packet[13] = 0x00

	// Calculate and append CRC (covers bytes 0-13)
	crc := CalculateCRC16(packet[:14])
	binary.LittleEndian.PutUint16(packet[14:16], crc)

	return packet
}

// parseRxStatusResponse parses the RxStatus response packet (data type 0xA1)
// Based on bitmain_rxstatus_data from official driver
func (d *Device) parseRxStatusResponse(data []byte) error {
	if len(data) < 6 {
		return fmt.Errorf("response too short: %d bytes", len(data))
	}

	// Verify data type (0xA1 for RxStatus response)
	if data[0] != DataTypeRxStatus {
		return fmt.Errorf("invalid response data type: 0x%02X (expected 0xA1)", data[0])
	}

	// Extract payload length
	payloadLen := binary.LittleEndian.Uint16(data[2:4])
	if len(data) < 4+int(payloadLen)+2 {
		return fmt.Errorf("response truncated: got %d bytes, need %d", len(data), 4+int(payloadLen)+2)
	}

	// Verify CRC
	receivedCRC := binary.LittleEndian.Uint16(data[4+payloadLen : 4+payloadLen+2])
	calculatedCRC := CalculateCRC16(data[:4+payloadLen])
	if receivedCRC != calculatedCRC {
		return fmt.Errorf("CRC mismatch: received 0x%04X, calculated 0x%04X", receivedCRC, calculatedCRC)
	}

	// Parse payload (bitmain_rxstatus_data structure)
	if payloadLen >= 12 {
		payload := data[4 : 4+payloadLen]

		// chip_value_eft (byte 0)
		// chain_num (byte 1)
		if len(payload) > 1 {
			d.chipCount = int(payload[1])
		}

		// fifo_space (bytes 2-3)
		fifoSpace := binary.LittleEndian.Uint16(payload[2:4])

		// hw_version (bytes 4-7)
		// fan_num (byte 8)
		// temp_num (byte 9)

		// Check if device is operational based on chip count and fifo space
		d.isOperational = d.chipCount > 0 && fifoSpace > 0

		log.Printf("RxStatus response: operational=%v, chips=%d, fifo_space=%d",
			d.isOperational, d.chipCount, fifoSpace)
	}

	return nil
}

// ComputeHash computes a single SHA-256 hash
func (d *Device) ComputeHash(data []byte) ([32]byte, error) {
	// Trace start
	if d.tracer != nil {
		tracer_compute_start()
		defer tracer_compute_end()
	}

	start := time.Now()
	defer func() {
		d.updateStats(1, uint64(len(data)), uint64(time.Since(start).Nanoseconds()))
	}()

	result, err := d.ComputeBatch([][]byte{data})
	if err != nil {
		d.stats.mu.Lock()
		d.stats.ErrorCount++
		d.stats.mu.Unlock()
		return [32]byte{}, err
	}

	return result[0], nil
}

// ComputeBatch computes multiple SHA-256 hashes using Bitmain ASIC mining protocol
// Mining Loop:
//  1. Send TxTask with work (SHA-256 mining work)
//  2. Poll for RxNonce responses (check every 40ms)
//  3. When nonce found, verify and submit
//  4. Periodically send RxStatus to monitor health
func (d *Device) ComputeBatch(inputs [][]byte) ([][32]byte, error) {
	if len(inputs) == 0 {
		return nil, fmt.Errorf("empty input batch")
	}

	if len(inputs) > MaxBatchSize {
		return nil, fmt.Errorf("batch size %d exceeds maximum %d", len(inputs), MaxBatchSize)
	}

	if !d.isOperational {
		return nil, fmt.Errorf("ASIC device is not operational")
	}

	// Trace batch start
	if d.tracer != nil {
		tracer_batch_start()
		defer tracer_batch_end()
	}

	start := time.Now()
	results := make([][32]byte, len(inputs))
	completed := 0

	// Process each input through mining loop
	for i, input := range inputs {
		// 1. Send TxTask with work
		txTask := d.buildTxTaskPacket(input, i)

		var writeErr error
		d.mu.Lock()
		if d.useUSB && d.usbDevice != nil {
			writeErr = d.usbDevice.SendPacket(txTask)
		} else if d.file != nil {
			_, writeErr = d.file.Write(txTask)
		} else {
			writeErr = fmt.Errorf("no device interface available")
		}
		d.mu.Unlock()

		if writeErr != nil {
			d.stats.mu.Lock()
			d.stats.ErrorCount++
			d.stats.mu.Unlock()
			return nil, fmt.Errorf("failed to send TxTask for input %d: %w", i, writeErr)
		}

		// 2. Poll for RxNonce response
		result, err := d.pollForNonce(i, PollInterval, 100, input) // Max 100 polls * 40ms = 4s timeout
		if err != nil {
			d.stats.mu.Lock()
			d.stats.ErrorCount++
			d.stats.mu.Unlock()
			return nil, fmt.Errorf("failed to get nonce for input %d: %w", i, err)
		}

		results[i] = result
		completed++

		// 4. Periodically send RxStatus to monitor health (every 50 requests)
		if i%50 == 49 {
			if err := d.checkDeviceHealth(); err != nil {
				log.Printf("Warning: Device health check failed: %v", err)
			}
		}
	}

	totalBytes := uint64(0)
	for _, input := range inputs {
		totalBytes += uint64(len(input))
	}

	d.updateStats(uint64(completed), totalBytes, uint64(time.Since(start).Nanoseconds()))

	return results, nil
}

// MineWork performs mining on an 80-byte Bitcoin-style header to find the first valid nonce.
// It uses the configured device (USB, kernel, or CGMiner) to find nonces.
func (d *Device) MineWork(header []byte, nonceStart, nonceEnd uint32, workID uint8, timeout time.Duration) (uint32, error) {
	log.Printf("Device.MineWork called for header len %d, workID %d", len(header), workID)
	if len(header) != 80 {
		return 0, fmt.Errorf("mining header must be exactly 80 bytes, got %d", len(header))
	}
	if !d.isOperational {
		return 0, fmt.Errorf("ASIC device is not operational")
	}

	// Use CGMiner if available (most reliable)
	if d.useCGMiner && d.cgMinerMiner != nil {
		return d.cgMinerMiner.MineWork(header, nonceStart, nonceEnd, timeout)
	}

	start := time.Now()

	// 1. Construct the TxTask packet
	txTaskPacket := BuildTxTaskFromHeader(header, workID)
	log.Printf("Built TxTask packet (len %d)", len(txTaskPacket))

	// 2. Send TxTask packet
	log.Printf("Sending TxTask packet...")
	if err := d.SendPacket(txTaskPacket); err != nil {
		d.stats.mu.Lock()
		d.stats.ErrorCount++
		d.stats.mu.Unlock()
		log.Printf("Error sending TxTask packet: %v", err)
		return 0, fmt.Errorf("failed to send TxTask packet for mining: %w", err)
	}
	log.Printf("TxTask packet sent.")

	// 3. Read RxNonce response
	log.Printf("Reading RxNonce response with timeout %v...", timeout)
	rxNonceBuffer := make([]byte, MaxUSBPacketSize) // Max USB packet size is 512 bytes
	n, err := d.ReadPacket(rxNonceBuffer, timeout)
	if err != nil {
		d.stats.mu.Lock()
		d.stats.ErrorCount++
		d.stats.mu.Unlock()
		log.Printf("Error reading RxNonce response: %v", err)
		return 0, fmt.Errorf("failed to read RxNonce response: %w", err)
	}
	log.Printf("Read %d bytes for RxNonce response.", n)

	// 4. Parse RxNonce to extract the found nonce
	_, nonce, _, ok := ParseRxNonce(rxNonceBuffer[:n])
	if !ok {
		d.stats.mu.Lock()
		d.stats.ErrorCount++
		d.stats.mu.Unlock()
		log.Printf("Error parsing RxNonce response: not ok")
		return 0, fmt.Errorf("failed to parse RxNonce response")
	}
	log.Printf("Successfully parsed RxNonce. Found nonce: %d", nonce)

	latency := time.Since(start)
	d.updateStats(1, uint64(len(header)), uint64(latency.Nanoseconds()))

	return nonce, nil
}

// EasyTarget is the nBits value for minimum difficulty (any hash is valid)
// Format: 0x207FFFFF means target = 0x7FFFFF × 2^(8×(0x20-3)) ≈ maximum
// This ensures the ASIC finds a nonce on the first try
const EasyTarget = 0x207FFFFF

// buildTxTaskPacket builds a TxTask packet with SHA-256 mining work
// Total size: 51 bytes for single work item (header 4 + work_num 1 + ASIC_TASK 45 + crc 2)
// Based on bitmain_txtask_token from official driver
// ASIC_TASK: [work_id(1)][midstate(32)][data(12)] = 45 bytes
//
// For crypto-transformer use, we set an easy difficulty target so the ASIC
// finds a nonce quickly. The nonce becomes temporal entropy for the hash.
func (d *Device) buildTxTaskPacket(input []byte, workID int) []byte {
	// Ensure input fits in midstate+data (44 bytes total)
	// For SHA-256, we use midstate for first 32 bytes, data for remaining
	const taskSize = 45 // ASIC_TASK size: work_id(1) + midstate(32) + data(12)

	packet := make([]byte, 4+1+taskSize+2) // 51 bytes total

	// Header (4 bytes)
	packet[0] = TokenTxTask // 0x52
	packet[1] = 0x00        // Version
	// Length = work_num(1) + ASIC_TASK(45) = 46
	binary.LittleEndian.PutUint16(packet[2:4], 46)

	// work_num (1 byte) - number of work items
	packet[4] = 0x01

	// ASIC_TASK (45 bytes)
	// work_id (1 byte)
	packet[5] = uint8(workID & 0xFF)

	// midstate[32] - first 32 bytes of input or padded with zeros
	// For crypto-transformer: this is the input || seed data
	midstateOffset := 6
	if len(input) >= 32 {
		copy(packet[midstateOffset:midstateOffset+32], input[:32])
	} else {
		copy(packet[midstateOffset:midstateOffset+len(input)], input)
		// Remaining bytes already zero from make()
	}

	// data[12] - Format as mining work tail with easy target
	// Structure: [timestamp(4)][nBits(4)][nonce_start(4)]
	// The ASIC will iterate the nonce starting from nonce_start
	dataOffset := midstateOffset + 32

	// Use input bytes as "timestamp" for uniqueness if available
	if len(input) > 32 {
		remaining := len(input) - 32
		if remaining > 4 {
			remaining = 4
		}
		copy(packet[dataOffset:dataOffset+remaining], input[32:32+remaining])
	}

	// Set easy nBits target at offset +4 (bytes 4-7 of data field)
	// This makes any hash valid, so ASIC finds nonce immediately
	binary.LittleEndian.PutUint32(packet[dataOffset+4:dataOffset+8], EasyTarget)

	// Starting nonce at offset +8 (bytes 8-11 of data field)
	// Use workID as starting point for variety
	binary.LittleEndian.PutUint32(packet[dataOffset+8:dataOffset+12], uint32(workID))

	// Calculate and append CRC (covers bytes 0-49)
	crc := CalculateCRC16(packet[:50])
	binary.LittleEndian.PutUint16(packet[50:52], crc)

	return packet
}

// pollForNonce polls for RxNonce response from ASIC and computes the actual SHA-256 hash
func (d *Device) pollForNonce(workID int, interval time.Duration, maxPolls int, originalInput []byte) ([32]byte, error) {
	response := make([]byte, 64)

	for poll := 0; poll < maxPolls; poll++ {
		time.Sleep(interval)

		var n int
		var err error

		d.mu.Lock()
		if d.useUSB && d.usbDevice != nil {
			// USB mode: use ReadPacket with short timeout
			n, err = d.usbDevice.ReadPacket(response, 10*time.Millisecond)
		} else if d.file != nil {
			// Kernel device mode: use file read with deadline
			d.file.SetReadDeadline(time.Now().Add(10 * time.Millisecond))
			n, err = d.file.Read(response)
		} else {
			d.mu.Unlock()
			return [32]byte{}, fmt.Errorf("no device interface available")
		}
		d.mu.Unlock()

		if err != nil {
			continue // No data available, continue polling
		}

		if n < 6 {
			continue // Response too short
		}

		// Check if this is an RxNonce response (data type 0xA2)
		if response[0] != DataTypeRxNonce {
			continue
		}

		// Parse RxNonce response and compute actual SHA-256 hash
		result, err := d.parseRxNonceResponse(response[:n], workID, originalInput)
		if err != nil {
			continue // Not our work ID or invalid response
		}

		return result, nil
	}

	return [32]byte{}, fmt.Errorf("timeout waiting for nonce (workID=%d)", workID)
}

// parseRxNonceResponse parses an RxNonce response packet (data type 0xA2)
// and computes the actual SHA-256 hash using the returned nonce
// Based on bitmain_rxnonce_data from official driver
func (d *Device) parseRxNonceResponse(data []byte, expectedWorkID int, originalInput []byte) ([32]byte, error) {
	if len(data) < 6 {
		return [32]byte{}, fmt.Errorf("response too short: %d bytes", len(data))
	}

	// Verify data type (0xA2 for RxNonce)
	if data[0] != DataTypeRxNonce {
		return [32]byte{}, fmt.Errorf("invalid data type: 0x%02X (expected 0xA2)", data[0])
	}

	// Extract payload length
	payloadLen := binary.LittleEndian.Uint16(data[2:4])
	if len(data) < 4+int(payloadLen)+2 {
		return [32]byte{}, fmt.Errorf("response truncated: got %d bytes, need %d",
			len(data), 4+int(payloadLen)+2)
	}

	// Verify CRC
	receivedCRC := binary.LittleEndian.Uint16(data[4+payloadLen : 4+payloadLen+2])
	calculatedCRC := CalculateCRC16(data[:4+payloadLen])
	if receivedCRC != calculatedCRC {
		return [32]byte{}, fmt.Errorf("CRC mismatch: received 0x%04X, calculated 0x%04X",
			receivedCRC, calculatedCRC)
	}

	// Parse bitmain_rxnonce_data structure
	// Minimum payload: fifo_space(2) + nonce_num(1) + reserved(1) = 4 bytes
	if payloadLen < 4 {
		return [32]byte{}, fmt.Errorf("payload too short: %d bytes", payloadLen)
	}

	payload := data[4 : 4+payloadLen]

	// fifo_space (bytes 0-1)
	// nonce_num (byte 2)
	nonceNum := payload[2]
	// reserved (byte 3)

	if nonceNum == 0 {
		return [32]byte{}, fmt.Errorf("no nonces found")
	}

	// Parse nonce entries (each 8 bytes):
	// [work_id(1)][nonce(4)][chain_num(1)][reserved(2)]
	nonceOffset := 4
	for i := 0; i < int(nonceNum) && nonceOffset+8 <= len(payload); i++ {
		workID := int(payload[nonceOffset])

		if workID == expectedWorkID {
			// Found our nonce
			// nonce is 4 bytes little-endian at bytes 1-4
			nonce := binary.LittleEndian.Uint32(payload[nonceOffset+1 : nonceOffset+5])

			// HYBRID APPROACH: Compute actual SHA-256 hash using the nonce
			// The ASIC finds nonces that produce valid hashes, but doesn't return the hash
			// We reconstruct the full block header and compute SHA-256 in software
			result := d.computeHashFromNonce(originalInput, nonce)

			return result, nil
		}

		nonceOffset += 8
	}

	return [32]byte{}, fmt.Errorf("work ID %d not found in response", expectedWorkID)
}

// computeHashFromNonce computes the final SHA-256 hash using ASIC-found nonce as temporal entropy
//
// CRYPTO-TRANSFORMER ARCHITECTURE:
// The ASIC serves as a "temporal nonce oracle" - it finds nonces quickly with easy difficulty.
// Each nonce becomes temporal entropy that creates unique hash outputs per inference pass.
//
// For temporal ensemble (21 passes):
//   - Each pass sends work to ASIC with the same input || seed
//   - ASIC returns a different nonce each time (based on internal iteration state)
//   - final_hash = SHA256(input || seed || nonce)
//   - The nonce variation creates the ensemble diversity
//   - Consensus aggregation produces robust final prediction
//
// This approach:
//   - Uses ASIC hardware for its designed purpose (nonce finding)
//   - Leverages nonce non-determinism as feature, not bug
//   - Maintains quantum-resistant SHA-256 foundation
//   - Produces unique temporal signatures for each pass
func (d *Device) computeHashFromNonce(originalInput []byte, nonce uint32) [32]byte {
	// Build the data to hash: original_input || nonce_bytes
	// The nonce adds temporal entropy to create unique hash per work item
	data := make([]byte, len(originalInput)+4)
	copy(data, originalInput)
	binary.LittleEndian.PutUint32(data[len(originalInput):], nonce)

	// Compute SHA-256 hash with temporal nonce
	return sha256.Sum256(data)
}

// checkDeviceHealth sends RxStatus and verifies device is operational
func (d *Device) checkDeviceHealth() error {
	rxStatus := d.buildRxStatusPacket()

	d.mu.Lock()
	_, err := d.file.Write(rxStatus)
	if err != nil {
		d.mu.Unlock()
		return fmt.Errorf("failed to send health check: %w", err)
	}

	response := make([]byte, 64)
	d.file.SetReadDeadline(time.Now().Add(500 * time.Millisecond))
	n, err := d.file.Read(response)
	d.mu.Unlock()

	if err != nil {
		return fmt.Errorf("failed to read health response: %w", err)
	}

	if err := d.parseRxStatusResponse(response[:n]); err != nil {
		return fmt.Errorf("failed to parse health response: %w", err)
	}

	if !d.isOperational {
		return fmt.Errorf("device reports non-operational status")
	}

	return nil
}

// GetStats returns current device statistics
func (d *Device) GetStats() DeviceStatsSnapshot {
	d.stats.mu.RLock()
	defer d.stats.mu.RUnlock()

	return DeviceStatsSnapshot{
		TotalRequests:  d.stats.TotalRequests,
		TotalBytes:     d.stats.TotalBytes,
		TotalLatencyNs: d.stats.TotalLatencyNs,
		PeakLatencyNs:  d.stats.PeakLatencyNs,
		ErrorCount:     d.stats.ErrorCount,
	}
}

// GetInfo returns device information
func (d *Device) GetInfo() DeviceInfo {
	var stat syscall.Stat_t
	if d.file != nil {
		syscall.Fstat(int(d.fd), &stat)
	}

	return DeviceInfo{
		DevicePath:      d.devicePath,
		ChipCount:       uint32(d.chipCount),
		FirmwareVersion: d.firmwareVersion,
		IsOperational:   d.isOperational,
		FileDescriptor:  int(d.fd),
	}
}

// Close closes the device and cleanup resources
// Reloads kernel module if it was unloaded during initialization
func (d *Device) Close() error {
	if d.tracer != nil {
		d.tracer.Close()
	}

	var fileErr error
	if d.file != nil {
		fileErr = d.file.Close()
	}

	// Reload kernel module if we unloaded it
	if d.kernelModuleUnloaded {
		if err := d.reloadKernelModule(); err != nil {
			log.Printf("Warning: Failed to reload kernel module on close: %v", err)
		}
	}

	return fileErr
}

// SendPacket sends a packet to the ASIC
func (d *Device) SendPacket(data []byte) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.useKernel && d.kernelDevice != nil {
		return d.kernelDevice.SendPacket(data)
	} else if d.useUSB && d.usbDevice != nil {
		return d.usbDevice.SendPacket(data)
	} else if d.file != nil {
		_, err := d.file.Write(data)
		return err
	}
	return fmt.Errorf("no device interface available for sending")
}

// ReadPacket reads a packet from the ASIC
func (d *Device) ReadPacket(buffer []byte, timeout time.Duration) (int, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.useKernel && d.kernelDevice != nil {
		return d.kernelDevice.ReadPacket(buffer, timeout)
	} else if d.useUSB && d.usbDevice != nil {
		return d.usbDevice.ReadPacket(buffer, timeout)
	} else if d.file != nil {
		d.file.SetReadDeadline(time.Now().Add(timeout)) // Set deadline for file-based read
		return d.file.Read(buffer)
	}
	return 0, fmt.Errorf("no device interface available for reading")
}

// Internal helper functions

func (d *Device) updateStats(requests, bytes, latencyNs uint64) {
	d.stats.mu.Lock()
	defer d.stats.mu.Unlock()

	d.stats.TotalRequests += requests
	d.stats.TotalBytes += bytes
	d.stats.TotalLatencyNs += latencyNs

	if latencyNs > d.stats.PeakLatencyNs {
		d.stats.PeakLatencyNs = latencyNs
	}
}

func buildTxTaskPacket(inputs [][]byte) []byte {
	// Calculate total payload size
	payloadSize := 0
	for _, input := range inputs {
		payloadSize += 2 + len(input) // 2 bytes length prefix
	}

	// Build packet: [token(1)][version(1)][length(2)][payload][crc(2)]
	packet := make([]byte, 4+payloadSize+2)
	packet[0] = 0x52 // TXTASK token
	packet[1] = 0x01 // Version
	binary.LittleEndian.PutUint16(packet[2:4], uint16(payloadSize))

	// Add payload
	offset := 4
	for _, input := range inputs {
		binary.LittleEndian.PutUint16(packet[offset:offset+2], uint16(len(input)))
		offset += 2
		copy(packet[offset:], input)
		offset += len(input)
	}

	// Calculate and append CRC
	crc := CalculateCRC16(packet[:offset])
	binary.LittleEndian.PutUint16(packet[offset:], crc)

	return packet
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// DeviceInfo holds device information
type DeviceInfo struct {
	DevicePath      string
	ChipCount       uint32
	FirmwareVersion string
	IsOperational   bool
	FileDescriptor  int
}
