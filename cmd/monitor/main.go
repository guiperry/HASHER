// Hasher: Neural Inference Engine Powered by SHA-256 ASICs
// Copyright (C) 2026  Guillermo Perry
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
package main

import (
	"context"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/google/gousb"
	"hasher/internal/analyzer/phases"
)

const (
	DevicePath = "/dev/bitmain-asic"

	// Token types
	BITMAIN_TOKEN_TYPE_TXCONFIG = 0x51
	BITMAIN_TOKEN_TYPE_TXTASK   = 0x52
	BITMAIN_TOKEN_TYPE_RXSTATUS = 0x53

	// Data types
	BITMAIN_DATA_TYPE_RXSTATUS = 0xa1
	BITMAIN_DATA_TYPE_RXNONCE  = 0xa2

	// Constants
	BITMAIN_MAX_CHAIN_NUM   = 16
	BITMAIN_MAX_TEMP_NUM    = 32
	BITMAIN_MAX_FAN_NUM     = 16
	BITMAIN_ASICS_PER_CHAIN = 8
	BITMAIN_READ_TIMEOUT    = 100 * time.Millisecond // Increased timeout for better reliability
	BITMAIN_READ_RETRIES    = 3                      // Number of retry attempts
	BITMAIN_RETRY_DELAY     = 50 * time.Millisecond  // Delay between retries
)

// PacketHead represents the header of a Bitmain packet
type PacketHead struct {
	TokenType uint8
	Version   uint8
	Length    uint16
}

// RxStatusData represents the status response from device (full S2 structure)
type RxStatusData struct {
	DataType        uint8                         `json:"data_type"`
	Version         uint8                         `json:"version"`
	Length          uint16                        `json:"length"`
	ChipValueEft    uint8                         `json:"chip_value_eft"`
	ChainNum        uint8                         `json:"chain_num"`
	FifoSpace       uint16                        `json:"fifo_space"`
	HwVersion       [4]uint8                      `json:"hw_version"`
	FanNum          uint8                         `json:"fan_num"`
	TempNum         uint8                         `json:"temp_num"`
	FanExist        uint16                        `json:"fan_exist"`
	TempExist       uint32                        `json:"temp_exist"`
	NonceError      uint32                        `json:"nonce_error"`
	RegValue        [BITMAIN_MAX_CHAIN_NUM]uint8  `json:"reg_value"`
	ChainAsicExist  [BITMAIN_MAX_CHAIN_NUM]uint32 `json:"chain_asic_exist"`
	ChainAsicStatus [BITMAIN_MAX_CHAIN_NUM]uint32 `json:"chain_asic_status"`
	ChainAsicNum    [BITMAIN_MAX_CHAIN_NUM]uint8  `json:"chain_asic_num"`
	Temp            [BITMAIN_MAX_TEMP_NUM]int8    `json:"temp"`
	Fan             [BITMAIN_MAX_FAN_NUM]int8     `json:"fan"`
	CRC             uint16                        `json:"crc"`
	CRCValid        bool                          `json:"crc_valid"`
}

func main() {
	fmt.Println("🛡️  ASIC Device Monitor Tool (v4 - Direct USB)")
	fmt.Println("=================================================")
	fmt.Println()

	// Parse CLI flags
	dumpStatus := flag.Bool("dump-status", false, "Dump parsed RxStatus periodically to logs")
	dumpInterval := flag.Int("dump-interval", 2, "Interval in seconds between status polls when --dump-status is set")
	simpleTest := flag.Bool("simple-test", false, "Simple test: send one RxStatus and exit")
	tryInterrupt := flag.Bool("try-interrupt", false, "Try interrupt endpoints instead of bulk (experimental)")
	tryCharDev := flag.Bool("try-char-dev", false, "Try /dev/bitmain-asic character device instead of USB")
	runDiagnostics := flag.Bool("diagnostics", false, "Run initial diagnostic phase before monitoring")
	diagnosticPhase := flag.String("diagnostic-phase", "all", "Diagnostic phase to run: all, system, device, process, protocol, access")
	jsonDiagnostics := flag.Bool("json-diagnostics", false, "Output diagnostic results as JSON")
	flag.Parse()

	// Phase 0: Diagnostics (if requested)
	if *runDiagnostics {
		fmt.Println("Phase 0: Running System Diagnostics...")
		fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
		runDiagnosticPhase(*diagnosticPhase, *jsonDiagnostics)
		fmt.Println()
	}

	// Stop CGMiner
	fmt.Println("Phase 1: Stopping CGMiner...")
	stopCGMiner()
	time.Sleep(2 * time.Second)
	fmt.Println()

	// Initialize USB context
	fmt.Println("Phase 2: Initializing USB...")
	ctx := gousb.NewContext()
	defer ctx.Close()

	// Open device by VID/PID (Bitmain ASIC: 0x4254:0x4153)
	fmt.Printf("Phase 3: Opening USB device (VID:0x4254 PID:0x4153)...\n")
	dev, err := ctx.OpenDeviceWithVIDPID(0x4254, 0x4153)
	if err != nil || dev == nil {
		fmt.Printf("❌ Could not open USB device: %v\n", err)
		fmt.Println("\nTroubleshooting:")
		fmt.Println("1. Check if device is connected: lsusb | grep 4254")
		fmt.Println("2. Check permissions: ls -la /dev/bus/usb/")
		return
	}
	defer dev.Close()
	fmt.Println("✅ USB device opened")
	fmt.Println()

	// Enable automatic kernel driver detachment
	fmt.Println("Phase 4: Detaching kernel driver...")
	err = dev.SetAutoDetach(true)
	if err != nil {
		fmt.Printf("⚠️  Could not enable auto-detach: %v\n", err)
		fmt.Println("   (This is OK on some systems)")
	} else {
		fmt.Println("✅ Auto-detach enabled")
	}
	fmt.Println()

	// Claim interface
	fmt.Println("Phase 5: Claiming USB interface...")
	intf, done, err := dev.DefaultInterface()
	if err != nil {
		fmt.Printf("❌ Could not claim interface: %v\n", err)
		return
	}
	defer done()
	fmt.Println("✅ Interface claimed (kernel driver detached)")
	fmt.Println()

	// Get endpoints
	var epOut *gousb.OutEndpoint
	var epIn *gousb.InEndpoint

	if *tryInterrupt {
		fmt.Println("Phase 6: Opening interrupt endpoints (experimental)...")
		epOut, err = intf.OutEndpoint(1) // Try interrupt OUT
		if err != nil {
			fmt.Printf("❌ Could not open interrupt OUT endpoint: %v\n", err)
			return
		}
		epIn, err = intf.InEndpoint(0x81) // Try interrupt IN
		if err != nil {
			fmt.Printf("❌ Could not open interrupt IN endpoint: %v\n", err)
			return
		}
		fmt.Println("✅ Interrupt endpoints ready (OUT:0x01, IN:0x81)")
	} else {
		fmt.Println("Phase 6: Opening bulk endpoints...")
		epOut, err = intf.OutEndpoint(1) // Bulk OUT endpoint 0x01
		if err != nil {
			fmt.Printf("❌ Could not open OUT endpoint: %v\n", err)
			return
		}
		epIn, err = intf.InEndpoint(0x81) // Bulk IN endpoint 0x81
		if err != nil {
			fmt.Printf("❌ Could not open IN endpoint: %v\n", err)
			return
		}
		fmt.Println("✅ Endpoints ready (OUT:0x01, IN:0x81)")
	}
	fmt.Println()

	// Debug: Print endpoint information
	fmt.Println("🔍 Endpoint Information:")
	fmt.Printf("   OUT Endpoint: Address=0x%02x, MaxPacketSize=%d, TransferType=%v\n",
		epOut.Desc.Address, epOut.Desc.MaxPacketSize, epOut.Desc.TransferType)
	fmt.Printf("   IN Endpoint: Address=0x%02x, MaxPacketSize=%d, TransferType=%v\n",
		epIn.Desc.Address, epIn.Desc.MaxPacketSize, epIn.Desc.TransferType)
	fmt.Println()

	// Phase 7: Querying device status with RxStatus (initial query)
	fmt.Println("Phase 7: Querying device status with RxStatus (initial query)...")
	fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	statusPacket := createRxStatusPacket()
	fmt.Printf("📤 RxStatus (%d bytes): %s\n", len(statusPacket), hex.EncodeToString(statusPacket))

	if *simpleTest {
		// Simple test: send one RxStatus and exit
		fmt.Println("🧪 Running simple test: send one RxStatus and exit")
		fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")

		statusPacket := createRxStatusPacket()
		fmt.Printf("📤 RxStatus (%d bytes): %s\n", len(statusPacket), hex.EncodeToString(statusPacket))

		n, err := epOut.Write(statusPacket)
		if err != nil {
			fmt.Printf("❌ Write failed: %v\n", err)
			return
		}
		fmt.Printf("✅ Sent %d bytes\n", n)

		// Wait a bit longer before reading
		fmt.Println("⏳ Waiting 500ms before reading...")
		time.Sleep(500 * time.Millisecond)

		// Read response with longer timeout
		fmt.Println("📖 Reading response...")
		response := make([]byte, 2048)
		readCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()

		numRead, err := epIn.ReadContext(readCtx, response)
		if err != nil {
			fmt.Printf("⚠️  Read error: %v\n", err)
		} else if numRead > 0 {
			fmt.Printf("📖 Received %d bytes:\n", numRead)
			fmt.Println(hex.Dump(response[:numRead]))
			if numRead >= 4 {
				dataType := response[0]
				fmt.Printf("\nData Type: 0x%02x ", dataType)
				switch dataType {
				case BITMAIN_DATA_TYPE_RXSTATUS:
					fmt.Println("(RxStatus Response)")
					rs, err := parseRxStatus(response[:numRead])
					if err != nil {
						fmt.Printf("   ⚠️  Error parsing RxStatus: %v\n", err)
					} else {
						prettyPrintRxStatus(rs)
					}
				case BITMAIN_DATA_TYPE_RXNONCE:
					fmt.Println("(RxNonce Response)")
					parseRxNonce(response[:numRead])
				default:
					fmt.Println("(Unknown)")
				}
			}
		} else {
			fmt.Println("⚠️  No data received")
		}
		return
	}

	if *tryCharDev {
		// Try character device approach instead of USB
		fmt.Println("🔄 Trying character device approach (/dev/bitmain-asic)...")
		runCharDevMode()
		return
	}

	if *dumpStatus {
		// Enter dump loop mode (send periodic RxStatus and log parsed JSON)
		runDumpMode(epOut, epIn, *dumpInterval)
		return
	}

	n, err := epOut.Write(statusPacket)
	if err != nil {
		fmt.Printf("❌ Write failed: %v\n", err)
		return
	}
	fmt.Printf("✅ Sent %d bytes\n", n)

	// Read RxStatus response (initial read with retry)
	fmt.Println("Phase 8: Reading initial RxStatus response...")
	fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	fmt.Println("⏳ Waiting 200ms before reading...")
	time.Sleep(200 * time.Millisecond)

	response := make([]byte, 2048)
	numRead, err := readWithRetry(epIn, response, BITMAIN_READ_RETRIES)
	if err != nil {
		fmt.Printf("⚠️  Read failed after retries: %v\n", err)
		fmt.Println("   Possible causes:")
		fmt.Println("   - Device not initialized properly")
		fmt.Println("   - Wrong packet sequence")
		fmt.Println("   - Timing issues")
		fmt.Println("   - Endpoint configuration")
	} else if numRead > 0 {
		fmt.Printf("📖 Received %d bytes:\n", numRead)
		fmt.Println(hex.Dump(response[:numRead]))
		if numRead >= 4 {
			dataType := response[0]
			fmt.Printf("\nData Type: 0x%02x ", dataType)
			switch dataType {
			case BITMAIN_DATA_TYPE_RXSTATUS:
				fmt.Println("(RxStatus Response)")
				rs, err := parseRxStatus(response[:numRead])
				if err != nil {
					fmt.Printf("   ⚠️  Error parsing RxStatus: %v\n", err)
				} else {
					prettyPrintRxStatus(rs)
				}
			case BITMAIN_DATA_TYPE_RXNONCE:
				fmt.Println("(RxNonce Response)")
			default:
				fmt.Println("(Unknown)")
			}
		}
	} else {
		fmt.Println("⚠️  No data received for initial RxStatus")
	}
	fmt.Println()

	// Phase 9: Initializing device with TxConfig (after initial RxStatus as per cgminer)
	fmt.Println("Phase 9: Initializing device with TxConfig...")
	fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	configPacket := createTxConfigPacket(250) // Removed voltage parameter
	fmt.Printf("📤 TxConfig (%d bytes): %s\n", len(configPacket), hex.EncodeToString(configPacket))

	n, err = epOut.Write(configPacket)
	if err != nil {
		fmt.Printf("❌ Write failed: %v\n", err)
		return
	}
	fmt.Printf("✅ Sent %d bytes\n", n)

	// Wait for device to process configuration (cgminer uses 1 second)
	fmt.Println("⏳ Waiting for device initialization...")
	time.Sleep(1 * time.Second)
	fmt.Println()

	// Phase 10: Submitting mining work with TxTask (after TxConfig)
	fmt.Println("Phase 10: Submitting mining work with TxTask...")
	fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	txTaskPacket := createTxTaskPacket()
	fmt.Printf("📤 TxTask (%d bytes): %s\n", len(txTaskPacket), hex.EncodeToString(txTaskPacket))

	n, err = epOut.Write(txTaskPacket)
	if err != nil {
		fmt.Printf("❌ Write failed: %v\n", err)
		return
	}
	fmt.Printf("✅ Sent %d bytes\n", n)

	// Wait for device to process work (a small delay)
	fmt.Println("⏳ Waiting for device to process work...")
	time.Sleep(100 * time.Millisecond) // A small delay
	fmt.Println()

	// Phase 11: Querying device status with RxStatus (after TxTask)
	fmt.Println("Phase 11: Querying device status with RxStatus...")
	fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	statusPacket = createRxStatusPacket() // Re-use the RxStatus packet
	fmt.Printf("📤 RxStatus (%d bytes): %s\n", len(statusPacket), hex.EncodeToString(statusPacket))

	n, err = epOut.Write(statusPacket)
	if err != nil {
		fmt.Printf("❌ Write failed: %v\n", err)
		return
	}
	fmt.Printf("✅ Sent %d bytes\n", n)

	// Read RxStatus response
	fmt.Println("Phase 12: Reading RxStatus response...")
	fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	fmt.Println("⏳ Waiting 500ms before final read...")
	time.Sleep(500 * time.Millisecond)

	response = make([]byte, 2048)
	fmt.Printf("📖 Attempting final read (timeout: %v)...\n", BITMAIN_READ_TIMEOUT)
	readCtx2, cancel2 := context.WithTimeout(context.Background(), BITMAIN_READ_TIMEOUT)
	startTime := time.Now()
	numRead, err = epIn.ReadContext(readCtx2, response)
	duration := time.Since(startTime)
	cancel2()

	fmt.Printf("📖 Final read completed in %v\n", duration)

	if err != nil {
		fmt.Printf("⚠️  Read timeout or error: %v\n", err)
		fmt.Println("   Troubleshooting:")
		fmt.Println("   1. Check if device is still connected")
		fmt.Println("   2. Verify packet sequence (TxConfig → TxTask → RxStatus)")
		fmt.Println("   3. Try different timing between packets")
		fmt.Println("   4. Check kernel logs for USB errors")
	} else if numRead > 0 {
		fmt.Printf("📖 Received %d bytes:\n", numRead)
		fmt.Println(hex.Dump(response[:numRead]))

		if numRead >= 4 {
			dataType := response[0]
			fmt.Printf("\nData Type: 0x%02x ", dataType)
			switch dataType {
			case BITMAIN_DATA_TYPE_RXSTATUS:
				fmt.Println("(RxStatus Response)")
				rs, err := parseRxStatus(response[:numRead])
				if err != nil {
					fmt.Printf("   ⚠️  Error parsing RxStatus: %v\n", err)
				} else {
					prettyPrintRxStatus(rs)
				}
			case BITMAIN_DATA_TYPE_RXNONCE:
				fmt.Println("(RxNonce Response)")
				parseRxNonce(response[:numRead])
			default:
				fmt.Println("(Unknown)")
			}
		}
	} else {
		fmt.Println("⚠️  No data received")
	}

	fmt.Println()
	fmt.Println("✅ USB communication test complete!")
}

// runDiagnosticPhase executes the diagnostic phase
func runDiagnosticPhase(phase string, jsonOutput bool) {
	var results []phases.DiagnosticResult

	switch phase {
	case "all":
		results = phases.RunAllDiagnostics()
	case "system":
		results = append(results, phases.SystemInfo())
	case "device":
		results = append(results, phases.DeviceInfo())
	case "process":
		results = append(results, phases.ProcessInfo())
	case "protocol":
		results = append(results, phases.ProtocolInfo())
	case "access":
		results = append(results, phases.DeviceAccessTest())
	default:
		fmt.Fprintf(os.Stderr, "Unknown diagnostic phase: %s\n", phase)
		fmt.Fprintln(os.Stderr, "Available phases: all, system, device, process, protocol, access")
		return
	}

	if jsonOutput {
		phases.PrintJSON(results)
	} else {
		phases.PrintText(results)
	}

	// Check for any failures and warn
	for _, result := range results {
		if !result.Success {
			fmt.Printf("⚠️  Diagnostic phase '%s' reported issues\n", result.Phase)
		}
	}
}

func createRxStatusPacket() []byte {
	// RxStatus request: 16 bytes total (with 4-byte alignment padding)
	// [token_type | version | length | flags | reserved2[3] | chip_address | reg_address | padding[4] | crc]
	packet := make([]byte, 16)

	// Header (4 bytes)
	packet[0] = BITMAIN_TOKEN_TYPE_RXSTATUS        // 0x53
	packet[1] = 0x00                               // Version
	binary.LittleEndian.PutUint16(packet[2:4], 12) // Length = payload size (excludes 4-byte header)

	// Payload (6 bytes)
	packet[4] = 0x00 // flags (status flags, 2 bits used)
	packet[5] = 0x00 // reserved2[0]
	packet[6] = 0x00 // reserved2[1]
	packet[7] = 0x00 // reserved2[2]
	packet[8] = 0x00 // chip_address (0x00 = all chips)
	packet[9] = 0x00 // reg_address (0x00 = default)

	// Padding for 4-byte alignment (4 bytes)
	packet[10] = 0x00
	packet[11] = 0x00
	packet[12] = 0x00
	packet[13] = 0x00

	// CRC-16 over bytes 0-13 (2 bytes)
	crc := calculateCRC16(packet[:14])
	binary.LittleEndian.PutUint16(packet[14:16], crc)

	return packet
}

func createTxConfigPacket(frequency uint16) []byte {
	// TxConfig: Total length for S2 is 28 bytes (sizeof(struct bitmain_txconfig_token))
	// Header (4 bytes) + Payload (22 bytes) + CRC (2 bytes) = 28 bytes
	packet := make([]byte, 28)

	// Header (4 bytes)
	packet[0] = BITMAIN_TOKEN_TYPE_TXCONFIG        // 0x51
	packet[1] = 0x00                               // Version
	binary.LittleEndian.PutUint16(packet[2:4], 24) // Length = payload size (22) + reserved_bytes (2) = 24. This matches (datalen - 4) from cgminer

	// Control flags byte 4: (reset:0, fan_eft:1, timeout_eft:1, freq_eft:1, volt_eft:1, chain_check_time_eft:0, chip_config_eft:0, hw_error_eft:0)
	// (1<<1) | (1<<2) | (1<<3) | (1<<4) = 0x02 | 0x04 | 0x08 | 0x10 = 0x1E
	packet[4] = 0x1E

	// Control flags byte 5: (beeper_ctrl:0, temp_over_ctrl:0, reserved1:0 (6 bits))
	packet[5] = 0x00

	// Reserved bytes (2 bytes, from struct bitmain_txconfig_token in driver-bitmain.h.tmp for S2)
	packet[6] = 0x00
	packet[7] = 0x00

	// ASIC Configuration (4 bytes)
	packet[8] = 8     // chain_num (8 chains for Antminer S3)
	packet[9] = 32    // asic_num (32 chips per chain)
	packet[10] = 0x60 // fan_pwm_data (96% duty cycle, BITMAIN_PWM_MAX 0xA0, BITMAIN_DEFAULT_FAN_MAX_PWM 0xA0)
	packet[11] = 0x2D // timeout_data (BITMAIN_DEFAULT_TIMEOUT 0x2D)

	// Frequency (2 bytes, little-endian)
	binary.LittleEndian.PutUint16(packet[12:14], frequency)

	// Voltage (1 byte, BITMAIN_DEFAULT_VOLTAGE 5)
	packet[14] = 0x05

	// Chain Check Time (1 byte, guessed 10 from other constants)
	packet[15] = 0x0A // BITMAIN_DEFAULT_CHAIN_CHECK_TIME

	// Register data (4 bytes)
	packet[16] = 0x00 // reg_data[0]
	packet[17] = 0x00 // reg_data[1]
	packet[18] = 0x00 // reg_data[2]
	packet[19] = 0x00 // reg_data[3]

	// Chip and register addresses (2 bytes)
	packet[20] = 0x00 // chip_address (0x00 = all chips)
	packet[21] = 0x00 // reg_address (0x00 = default)

	// Padding (4 bytes to reach 28 bytes total before CRC)
	// These correspond to the end of the struct after reg_address and before crc
	// The struct in driver-bitmain.h.tmp has total 28 bytes, which implies this padding.
	packet[22] = 0x00
	packet[23] = 0x00
	packet[24] = 0x00
	packet[25] = 0x00

	// CRC-16 over bytes 0-25 (2 bytes)
	crc := calculateCRC16(packet[:26])
	binary.LittleEndian.PutUint16(packet[26:28], crc)

	return packet
}

func parseRxStatus(data []byte) (RxStatusData, error) {
	var rs RxStatusData
	if len(data) < 8 {
		return rs, fmt.Errorf("packet too short: %d bytes", len(data))
	}

	rs.DataType = data[0]
	rs.Version = data[1]
	rs.Length = binary.LittleEndian.Uint16(data[2:4])

	// Validate expected length
	expectedLen := int(rs.Length) + 4
	if expectedLen != len(data) {
		// Not fatal; continue parsing but warn
		fmt.Printf("   ⚠️  RxStatus length mismatch. Expected %d, got %d\n", expectedLen, len(data))
	}

	// CRC check
	if len(data) >= 2 {
		calculatedCRC := calculateCRC16(data[:len(data)-2])
		rs.CRC = binary.LittleEndian.Uint16(data[len(data)-2:])
		if calculatedCRC != rs.CRC {
			fmt.Printf("   ⚠️  RxStatus CRC mismatch. Calculated 0x%04x, received 0x%04x\n", calculatedCRC, rs.CRC)
			rs.CRCValid = false
		} else {
			rs.CRCValid = true
		}
	}

	offset := 4 // Start after header

	// Parse fields according to bitmain_rxstatus_data structure
	if len(data) > offset {
		rs.ChipValueEft = data[offset]
		offset++
	}

	if len(data) > offset {
		rs.ChainNum = data[offset]
		offset++
	}

	if len(data) >= offset+2 {
		rs.FifoSpace = binary.LittleEndian.Uint16(data[offset : offset+2])
		offset += 2
	}

	if len(data) >= offset+4 {
		copy(rs.HwVersion[:], data[offset:offset+4])
		offset += 4
	}

	if len(data) > offset {
		rs.FanNum = data[offset]
		offset++
	}

	if len(data) > offset {
		rs.TempNum = data[offset]
		offset++
	}

	if len(data) >= offset+2 {
		rs.FanExist = binary.LittleEndian.Uint16(data[offset : offset+2])
		offset += 2
	}

	if len(data) >= offset+4 {
		rs.TempExist = binary.LittleEndian.Uint32(data[offset : offset+4])
		offset += 4
	}

	if len(data) >= offset+4 {
		rs.NonceError = binary.LittleEndian.Uint32(data[offset : offset+4])
		offset += 4
	}

	// Parse reg_value array
	if len(data) >= offset+BITMAIN_MAX_CHAIN_NUM {
		copy(rs.RegValue[:], data[offset:offset+BITMAIN_MAX_CHAIN_NUM])
		offset += BITMAIN_MAX_CHAIN_NUM
	}

	// Parse chain_asic_exist array
	for i := 0; i < BITMAIN_MAX_CHAIN_NUM && len(data) >= offset+4; i++ {
		rs.ChainAsicExist[i] = binary.LittleEndian.Uint32(data[offset : offset+4])
		offset += 4
	}

	// Parse chain_asic_status array
	for i := 0; i < BITMAIN_MAX_CHAIN_NUM && len(data) >= offset+4; i++ {
		rs.ChainAsicStatus[i] = binary.LittleEndian.Uint32(data[offset : offset+4])
		offset += 4
	}

	// Parse chain_asic_num array
	if len(data) >= offset+BITMAIN_MAX_CHAIN_NUM {
		copy(rs.ChainAsicNum[:], data[offset:offset+BITMAIN_MAX_CHAIN_NUM])
		offset += BITMAIN_MAX_CHAIN_NUM
	}

	// Parse temp array
	if len(data) >= offset+BITMAIN_MAX_TEMP_NUM {
		for i := 0; i < BITMAIN_MAX_TEMP_NUM; i++ {
			rs.Temp[i] = int8(data[offset+i])
		}
		offset += BITMAIN_MAX_TEMP_NUM
	}

	// Parse fan array
	if len(data) >= offset+BITMAIN_MAX_FAN_NUM {
		for i := 0; i < BITMAIN_MAX_FAN_NUM; i++ {
			rs.Fan[i] = int8(data[offset+i])
		}
		offset += BITMAIN_MAX_FAN_NUM
	}

	// Return structured status
	return rs, nil
}

// RxNonceNonce represents a single nonce found by the ASIC
type RxNonceNonce struct {
	WorkID uint32
	Nonce  uint32
}

// RxNonceData represents the RxNonce response from the device (S2 version)
type RxNonceData struct {
	DataType      uint8
	Version       uint8
	Length        uint16
	FifoSpace     uint16
	Diff          uint16
	TotalNonceNum uint64
	Nonces        []RxNonceNonce
	CRC           uint16
}

func prettyPrintRxStatus(rs RxStatusData) {
	fmt.Println()
	fmt.Println("   Parsed RxStatus:")
	fmt.Printf("   - DataType: 0x%02x\n", rs.DataType)
	fmt.Printf("   - Version: %d\n", rs.Version)
	fmt.Printf("   - Length: %d bytes\n", rs.Length)
	fmt.Printf("   - Chip value effective: %d\n", rs.ChipValueEft)
	fmt.Printf("   - Chain count: %d\n", rs.ChainNum)
	fmt.Printf("   - FIFO space: %d\n", rs.FifoSpace)
	fmt.Printf("   - HW Version: %d.%d.%d.%d\n", rs.HwVersion[0], rs.HwVersion[1], rs.HwVersion[2], rs.HwVersion[3])
	fmt.Printf("   - Fan count: %d\n", rs.FanNum)
	fmt.Printf("   - Temp sensor count: %d\n", rs.TempNum)
	fmt.Printf("   - Fan exist bitmap: 0x%04x\n", rs.FanExist)
	fmt.Printf("   - Temp exist bitmap: 0x%08x\n", rs.TempExist)
	fmt.Printf("   - Nonce error count: %d\n", rs.NonceError)
	fmt.Printf("   - CRC: 0x%04x (valid: %v)\n", rs.CRC, rs.CRCValid)

	// Print chain information
	fmt.Println("   - Chain details:")
	for i := 0; i < int(rs.ChainNum) && i < BITMAIN_MAX_CHAIN_NUM; i++ {
		fmt.Printf("     Chain %d: reg=0x%02x, asic_exist=0x%08x, status=0x%08x, asic_num=%d\n",
			i, rs.RegValue[i], rs.ChainAsicExist[i], rs.ChainAsicStatus[i], rs.ChainAsicNum[i])
	}

	// Print temperature readings
	fmt.Print("   - Temperatures:")
	for i := 0; i < int(rs.TempNum) && i < BITMAIN_MAX_TEMP_NUM; i++ {
		if i > 0 {
			fmt.Print(",")
		}
		fmt.Printf(" %d°C", rs.Temp[i])
	}
	fmt.Println()

	// Print fan speeds
	fmt.Print("   - Fan speeds:")
	for i := 0; i < int(rs.FanNum) && i < BITMAIN_MAX_FAN_NUM; i++ {
		if i > 0 {
			fmt.Print(",")
		}
		fmt.Printf(" %d%%", rs.Fan[i])
	}
	fmt.Println()
}

func runDumpMode(epOut *gousb.OutEndpoint, epIn *gousb.InEndpoint, intervalSec int) {
	fmt.Println("📡 Entering --dump-status mode: logging parsed RxStatus to logs/")
	_ = os.MkdirAll("logs", 0755)
	logFilename := filepath.Join("logs", fmt.Sprintf("asic-monitor-status_%s.log", time.Now().Format("20060102_150405")))
	f, err := os.OpenFile(logFilename, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		fmt.Printf("⚠️  Could not open log file: %v\n", err)
		return
	}
	defer f.Close()

	statusPacket := createRxStatusPacket()
	for {
		// Send request
		n, err := epOut.Write(statusPacket)
		if err != nil {
			fmt.Printf("⚠️  Write failed: %v\n", err)
			return
		}
		_ = n

		// Read response
		response := make([]byte, 2048)
		fmt.Printf("📖 Dump mode read attempt (timeout: 200ms)...\n")
		readCtx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
		startTime := time.Now()
		numRead, err := epIn.ReadContext(readCtx, response)
		duration := time.Since(startTime)
		cancel()
		fmt.Printf("📖 Dump mode read completed in %v\n", duration)

		if err != nil {
			fmt.Printf("⚠️  Read failed: %v\n", err)
		} else if numRead > 0 {
			rs, err := parseRxStatus(response[:numRead])
			if err != nil {
				fmt.Printf("⚠️  Parse error: %v\n", err)
			} else {
				// Marshal to JSON and append to log
				j, _ := json.Marshal(rs)
				line := fmt.Sprintf("%s %s\n", time.Now().Format(time.RFC3339), string(j))
				if _, err := f.WriteString(line); err != nil {
					fmt.Printf("⚠️  Could not write to log: %v\n", err)
				}
				prettyPrintRxStatus(rs)
			}
		}

		time.Sleep(time.Duration(intervalSec) * time.Second)
	}
}

func parseRxNonce(data []byte) {
	if len(data) < 14 { // Minimum size for header + fifo_space + diff + total_nonce_num
		fmt.Println("   ⚠️  RxNonce packet too short to parse")
		return
	}

	fmt.Println()
	fmt.Println("   Parsing RxNonce:")

	var rxNonce RxNonceData
	rxNonce.DataType = data[0]
	rxNonce.Version = data[1]
	rxNonce.Length = binary.LittleEndian.Uint16(data[2:4])

	// Validate length
	expectedLen := int(rxNonce.Length) + 4 // Payload length + 4 (header)
	if expectedLen != len(data) {
		fmt.Printf("   ⚠️  RxNonce length mismatch. Expected %d bytes, got %d\n", expectedLen, len(data))
		// Continue parsing what we have, but indicate error
	}

	// CRC check
	calculatedCRC := calculateCRC16(data[:len(data)-2])
	receivedCRC := binary.LittleEndian.Uint16(data[len(data)-2:])
	if calculatedCRC != receivedCRC {
		fmt.Printf("   ⚠️  RxNonce CRC mismatch. Calculated 0x%04x, received 0x%04x\n", calculatedCRC, receivedCRC)
	} else {
		fmt.Printf("   ✅ RxNonce CRC check passed (0x%04x)\n", calculatedCRC)
	}

	rxNonce.FifoSpace = binary.LittleEndian.Uint16(data[4:6])
	rxNonce.Diff = binary.LittleEndian.Uint16(data[6:8])
	rxNonce.TotalNonceNum = binary.LittleEndian.Uint64(data[8:16])

	fmt.Printf("   - DataType: 0x%02x\n", rxNonce.DataType)
	fmt.Printf("   - Version: %d\n", rxNonce.Version)
	fmt.Printf("   - Length: %d bytes (payload)\n", rxNonce.Length)
	fmt.Printf("   - FIFO Space: %d\n", rxNonce.FifoSpace)
	fmt.Printf("   - Diff: %d\n", rxNonce.Diff)
	fmt.Printf("   - Total Nonce Num: %d\n", rxNonce.TotalNonceNum)

	// Parse nonces if available
	nonceOffset := 16                                                  // After total_nonce_num
	nonceSize := 8                                                     // work_id (4 bytes) + nonce (4 bytes)
	numNonces := (int(rxNonce.Length) - (nonceOffset - 4)) / nonceSize // Payload len - (offset to nonces) / nonce size

	if numNonces > 0 && len(data[nonceOffset:len(data)-2]) >= numNonces*nonceSize {
		rxNonce.Nonces = make([]RxNonceNonce, numNonces)
		for i := 0; i < numNonces; i++ {
			start := nonceOffset + i*nonceSize
			rxNonce.Nonces[i].WorkID = binary.LittleEndian.Uint32(data[start : start+4])
			rxNonce.Nonces[i].Nonce = binary.LittleEndian.Uint32(data[start+4 : start+8])
			fmt.Printf("     Nonce %d: WorkID 0x%08x, Nonce 0x%08x\n", i+1, rxNonce.Nonces[i].WorkID, rxNonce.Nonces[i].Nonce)
		}
	} else if numNonces > 0 {
		fmt.Printf("   ⚠️  Incomplete nonce data. Expected %d nonces, but data too short.\n", numNonces)
	}
}

// CRC16 lookup tables from Bitmain driver source
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

func calculateCRC16(data []byte) uint16 {
	// CRC-16 using Bitmain's lookup tables (from official driver source)
	chCRCHi := uint8(0xFF)
	chCRCLo := uint8(0xFF)

	for _, b := range data {
		wIndex := chCRCLo ^ b
		chCRCLo = chCRCHi ^ chCRCHTalbe[wIndex]
		chCRCHi = chCRCLTalbe[wIndex]
	}

	return (uint16(chCRCHi) << 8) | uint16(chCRCLo)
}

func releaseDeviceLock() {
	// Try to unload and reload the kernel module to clear any locks
	fmt.Println("Attempting to reload kernel module...")

	// Try common module names
	moduleNames := []string{"bitmain_asic", "bitmain-asic", "bitmaindrv"}

	for _, modName := range moduleNames {
		cmd := exec.Command("rmmod", modName)
		if err := cmd.Run(); err == nil {
			fmt.Printf("✅ Unloaded module: %s\n", modName)
			time.Sleep(1 * time.Second)

			cmd = exec.Command("modprobe", modName)
			if err := cmd.Run(); err == nil {
				fmt.Printf("✅ Reloaded module: %s\n", modName)
			}
			return
		}
	}

	fmt.Println("⚠️  Could not reload kernel module (may not have permissions)")
	fmt.Println("   Device lock may still be held - proceeding anyway...")
}

func createTxTaskPacket() []byte {
	// TxTask packet: sends mining work to ASIC
	// Simplified test packet with dummy mining data
	packet := make([]byte, 64)

	packet[0] = BITMAIN_TOKEN_TYPE_TXTASK // 0x52
	packet[1] = 0x00                      // Version

	// Payload: work_id + midstate (32 bytes) + data2 (12 bytes)
	payloadStart := 4
	packet[payloadStart] = 0x01 // work_id

	// Dummy midstate (32 bytes of test pattern)
	for i := 0; i < 32; i++ {
		packet[payloadStart+1+i] = byte(i)
	}

	// Dummy data2 (12 bytes)
	for i := 0; i < 12; i++ {
		packet[payloadStart+33+i] = byte(0xFF - i)
	}

	offset := payloadStart + 45

	// Set length
	length := uint16(offset - 4 + 2) // +2 for CRC
	binary.LittleEndian.PutUint16(packet[2:4], length)

	// Calculate CRC
	crc := calculateCRC16(packet[:offset])
	binary.LittleEndian.PutUint16(packet[offset:offset+2], crc)
	offset += 2

	return packet[:offset]
}

func stopCGMiner() {
	// Check if running
	cmd := exec.Command("pgrep", "cgminer")
	err := cmd.Run()
	if err != nil {
		fmt.Println("✅ CGMiner is not running")
		return
	}

	// Stop gracefully
	fmt.Println("Stopping CGMiner gracefully...")
	cmd = exec.Command("/etc/init.d/cgminer", "stop")
	cmd.Run()

	time.Sleep(5 * time.Second)

	// Check again
	cmd = exec.Command("pgrep", "cgminer")
	err = cmd.Run()
	if err != nil {
		fmt.Println("✅ CGMiner stopped")
		return
	}

	// Force kill
	fmt.Println("Force killing CGMiner...")
	cmd = exec.Command("killall", "-9", "cgminer")
	cmd.Run()
	time.Sleep(2 * time.Second)

	fmt.Println("✅ CGMiner stopped")
}

// readWithRetry attempts to read from USB endpoint with retry logic
func readWithRetry(epIn *gousb.InEndpoint, buffer []byte, maxRetries int) (int, error) {
	var lastErr error

	for attempt := 0; attempt < maxRetries; attempt++ {
		if attempt > 0 {
			fmt.Printf("   🔄 Retry %d/%d...\n", attempt, maxRetries-1)
			time.Sleep(BITMAIN_RETRY_DELAY)
		}

		fmt.Printf("   📖 Attempting read (timeout: %v)...\n", BITMAIN_READ_TIMEOUT)
		readCtx, cancel := context.WithTimeout(context.Background(), BITMAIN_READ_TIMEOUT)
		startTime := time.Now()
		numRead, err := epIn.ReadContext(readCtx, buffer)
		duration := time.Since(startTime)
		cancel()

		fmt.Printf("   📖 Read attempt completed in %v\n", duration)

		if err == nil {
			fmt.Printf("   ✅ Read successful: %d bytes\n", numRead)
			return numRead, nil
		}

		fmt.Printf("   ❌ Read failed: %v\n", err)
		lastErr = err

		// Don't retry on certain errors
		if err.Error() == "transfer was cancelled" || err.Error() == "device not opened" {
			fmt.Printf("   ⚠️  Non-retryable error: %v\n", err)
			break
		}
	}

	fmt.Printf("   ❌ All %d read attempts failed, last error: %v\n", maxRetries, lastErr)
	return 0, lastErr
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func runCharDevMode() {
	fmt.Println("📡 Character Device Mode: Using /dev/bitmain-asic")
	fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")

	// Open character device
	file, err := os.OpenFile(DevicePath, os.O_RDWR, 0644)
	if err != nil {
		fmt.Printf("❌ Could not open %s: %v\n", DevicePath, err)
		fmt.Println("   Troubleshooting:")
		fmt.Println("   1. Check if device exists: ls -la /dev/bitmain-asic")
		fmt.Println("   2. Check permissions")
		fmt.Println("   3. Ensure CGMiner is stopped")
		return
	}
	defer file.Close()

	fmt.Printf("✅ Opened %s successfully\n", DevicePath)
	fmt.Println()

	// Test sequence: RxStatus → TxConfig → TxTask → RxStatus
	statusPacket := createRxStatusPacket()

	// Send RxStatus
	fmt.Println("📤 Sending RxStatus...")
	n, err := file.Write(statusPacket)
	if err != nil {
		fmt.Printf("❌ Write failed: %v\n", err)
		return
	}
	fmt.Printf("✅ Sent %d bytes\n", n)

	// Try to read response
	fmt.Println("📖 Reading response...")
	response := make([]byte, 2048)

	// Set read deadline
	file.SetReadDeadline(time.Now().Add(500 * time.Millisecond))

	numRead, err := file.Read(response)
	if err != nil {
		if os.IsTimeout(err) {
			fmt.Println("⚠️  Read timeout (expected for character device)")
		} else {
			fmt.Printf("❌ Read error: %v\n", err)
		}
	} else if numRead > 0 {
		fmt.Printf("📖 Received %d bytes:\n", numRead)
		fmt.Println(hex.Dump(response[:numRead]))
		if numRead >= 4 {
			dataType := response[0]
			fmt.Printf("\nData Type: 0x%02x ", dataType)
			switch dataType {
			case BITMAIN_DATA_TYPE_RXSTATUS:
				fmt.Println("(RxStatus Response)")
				rs, err := parseRxStatus(response[:numRead])
				if err != nil {
					fmt.Printf("   ⚠️  Error parsing RxStatus: %v\n", err)
				} else {
					prettyPrintRxStatus(rs)
				}
			case BITMAIN_DATA_TYPE_RXNONCE:
				fmt.Println("(RxNonce Response)")
				parseRxNonce(response[:numRead])
			default:
				fmt.Println("(Unknown)")
			}
		}
	} else {
		fmt.Println("⚠️  No data received (EOF)")
	}

	fmt.Println()
	fmt.Println("✅ Character device test complete!")
	fmt.Println("   Note: Character device typically returns EOF for reads")
	fmt.Println("   CGMiner uses USB directly for bidirectional communication")
}
