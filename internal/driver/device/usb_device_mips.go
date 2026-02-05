//go:build mips || mipsle
// +build mips mipsle

// internal/driver/device/usb_device_mips.go
// Raw USB access implementation for MIPS builds
// Bypasses the kernel module by using direct USB device files with IOCTLs

package device

import (
	"encoding/binary"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"syscall"
	"time"
	"unsafe"
)

// USB IOCTL constants (from linux/usbdevice_fs.h and usb.h)
const (
	// USB control request types
	USB_TYPE_STANDARD   = 0x00 << 5
	USB_TYPE_CLASS      = 0x01 << 5
	USB_TYPE_VENDOR     = 0x02 << 5
	USB_TYPE_RESERVED   = 0x03 << 5
	USB_RECIP_DEVICE    = 0x00
	USB_RECIP_INTERFACE = 0x01
	USB_RECIP_ENDPOINT  = 0x02
	USB_RECIP_OTHER     = 0x03

	// USB directions
	USB_DIR_OUT = 0
	USB_DIR_IN  = 0x80

	// USB device request codes
	USB_REQ_GET_STATUS        = 0x00
	USB_REQ_CLEAR_FEATURE     = 0x01
	USB_REQ_SET_FEATURE       = 0x03
	USB_REQ_SET_ADDRESS       = 0x05
	USB_REQ_GET_DESCRIPTOR    = 0x06
	USB_REQ_SET_DESCRIPTOR    = 0x07
	USB_REQ_GET_CONFIGURATION = 0x08
	USB_REQ_SET_CONFIGURATION = 0x09
	USB_REQ_GET_INTERFACE     = 0x0A
	USB_REQ_SET_INTERFACE     = 0x0B

	// USB descriptors
	USB_DT_DEVICE    = 0x01
	USB_DT_CONFIG    = 0x02
	USB_DT_STRING    = 0x03
	USB_DT_INTERFACE = 0x04
	USB_DT_ENDPOINT  = 0x05

	// USB device speed
	USB_SPEED_UNKNOWN = 0
	USB_SPEED_LOW     = 1
	USB_SPEED_FULL    = 2
	USB_SPEED_HIGH    = 3

	// Capabilities
	USBDEVFS_CAP_ZERO_PACKET         = 0x01
	USBDEVFS_CAP_BULK_CONTINUATION   = 0x02
	USBDEVFS_CAP_NO_PACKET_SIZE_LIM  = 0x04
	USBDEVFS_CAP_BULK_SCATTER_GATHER = 0x08

	// IOCTLs for usbdevfs - MIPS architecture encoding
	// MIPS uses different IOCTL bit layout:
	//   bits 0-7: number, bits 8-15: type ('U'=0x55)
	//   bits 16-28: size, bits 29-31: direction (1=none, 2=read, 4=write, 6=r/w)
	// Formula: (dir << 29) | (size << 16) | (type << 8) | nr

	// _IOWR('U', 0, ctrltransfer) - size ~24 bytes on 32-bit
	USBDEVFS_CONTROL = 0xc0185500
	// _IOWR('U', 2, bulktransfer) - size 16 bytes on 32-bit MIPS
	USBDEVFS_BULK = 0xc0105502
	// _IOR('U', 3, unsigned int)
	USBDEVFS_RESETEP = 0x40045503
	// _IOR('U', 4, setinterface)
	USBDEVFS_SETINTERFACE = 0x40085504
	// _IOR('U', 5, unsigned int)
	USBDEVFS_SETCONFIGURATION = 0x40045505
	// _IOW('U', 8, getdriver)
	USBDEVFS_GETDRIVER = 0x80045508
	// _IOR('U', 10, urb)
	USBDEVFS_SUBMITURB = 0x4038550a
	// _IO('U', 11)
	USBDEVFS_DISCARDURB = 0x2000550b
	// _IOW('U', 12, void*)
	USBDEVFS_REAPURB = 0x8004550c
	// _IOW('U', 13, void*)
	USBDEVFS_REAPURBNDELAY = 0x8004550d
	// _IO('U', 22)
	USBDEVFS_DISCONNECT = 0x20005516
	// _IO('U', 23)
	USBDEVFS_CONNECT = 0x20005517
	// _IOR('U', 15, unsigned int)
	USBDEVFS_CLAIMINTERFACE = 0x4004550f
	// _IOR('U', 16, unsigned int)
	USBDEVFS_RELEASEINTERFACE = 0x40045510
	// _IOWR('U', 18, ioctl)
	USBDEVFS_IOCTL = 0xc0085512
	// _IOR('U', 26, __u32)
	USBDEVFS_CAP = 0x4004551a
	// _IO('U', 20)
	USBDEVFS_RESET = 0x20005514
	// _IOR('U', 21, unsigned int)
	USBDEVFS_CLEAR_HALT = 0x40045515
	// _IO('U', 28)
	USBDEVFS_GET_SPEED = 0x2000551c
)

// USB device descriptor
type usbDeviceDescriptor struct {
	bLength            uint8
	bDescriptorType    uint8
	bcdUSB             uint16
	bDeviceClass       uint8
	bDeviceSubClass    uint8
	bDeviceProtocol    uint8
	bMaxPacketSize0    uint8
	idVendor           uint16
	idProduct          uint16
	bcdDevice          uint16
	iManufacturer      uint8
	iProduct           uint8
	iSerialNumber      uint8
	bNumConfigurations uint8
}

// USB devio control transfer structure
type usbdevfsCtrlTransfer struct {
	RequestType uint8
	Request     uint8
	Value       uint16
	Index       uint16
	Length      uint16
	Timeout     uint32
	Data        unsafe.Pointer
}

// USB devio bulk transfer structure (16 bytes on 32-bit MIPS)
type usbdevfsBulkTransfer struct {
	Ep      uint32         // endpoint
	Len     uint32         // length
	Timeout uint32         // timeout in ms
	Data    unsafe.Pointer // data buffer (4 bytes on 32-bit)
}

// USB devio set interface structure
type usbdevfsSetInterface struct {
	Interface  uint32
	AltSetting uint32
}

// USB devio disconnect claim structure (for USBDEVFS_DISCONNECT_CLAIM)
// Note: Simple USBDEVFS_DISCONNECT just takes interface number directly
type usbdevfsDisconnectClaim struct {
	Interface  uint32
	Flags      uint32
	DriverName [256]byte
}

// USBDevice provides direct USB communication with ASIC via raw USB device files
type USBDevice struct {
	fd        int
	chipCount int
	isClaimed bool
}

// OpenUSBDevice opens the ASIC via direct USB device file access
// This bypasses the kernel module completely
func OpenUSBDevice() (*USBDevice, error) {
	// Find the USB device in /dev/bus/usb
	devicePath, err := findUSBDevice(USBVendorID, USBProductID)
	if err != nil {
		return nil, fmt.Errorf("failed to find USB device: %w", err)
	}

	log.Printf("Found USB device at %s", devicePath)

	// Open the USB device file
	fd, err := syscall.Open(devicePath, syscall.O_RDWR, 0)
	if err != nil {
		return nil, fmt.Errorf("failed to open USB device %s: %w", devicePath, err)
	}

	usbDev := &USBDevice{
		fd:        fd,
		chipCount: 32, // Antminer S3 has 32 chips
	}

	// Claim interface 0
	if err := usbDev.claimInterface(0); err != nil {
		syscall.Close(fd)
		return nil, fmt.Errorf("failed to claim interface: %w", err)
	}

	log.Printf("Successfully opened USB device via %s (bypassing kernel module)", devicePath)
	return usbDev, nil
}

// findUSBDevice searches for a USB device with the given VID/PID in /dev/bus/usb
func findUSBDevice(vid, pid uint16) (string, error) {
	busPath := "/dev/bus/usb"

	log.Printf("Scanning %s for USB device %04x:%04x...", busPath, vid, pid)

	// Read all bus directories
	busDirs, err := os.ReadDir(busPath)
	if err != nil {
		return "", fmt.Errorf("failed to read %s: %w", busPath, err)
	}

	for _, busDir := range busDirs {
		if !busDir.IsDir() {
			continue
		}

		busNum := busDir.Name()
		deviceDir := filepath.Join(busPath, busNum)

		// Read all device files in this bus
		deviceFiles, err := os.ReadDir(deviceDir)
		if err != nil {
			log.Printf("Failed to read bus %s: %v", busNum, err)
			continue
		}

		for _, deviceFile := range deviceFiles {
			devicePath := filepath.Join(deviceDir, deviceFile.Name())

			// Try to open and check the device
			fd, err := syscall.Open(devicePath, syscall.O_RDONLY, 0)
			if err != nil {
				log.Printf("Failed to open %s: %v", devicePath, err)
				continue
			}

			// Read device descriptor
			desc, err := readDeviceDescriptor(fd)
			syscall.Close(fd)

			if err != nil {
				log.Printf("Failed to read descriptor from %s: %v", devicePath, err)
				continue
			}

			log.Printf("Device %s: VID=%04x PID=%04x", devicePath, desc.idVendor, desc.idProduct)

			// Check if this is our device
			if desc.idVendor == vid && desc.idProduct == pid {
				log.Printf("Found target device at %s", devicePath)
				return devicePath, nil
			}
		}
	}

	return "", fmt.Errorf("USB device %04x:%04x not found", vid, pid)
}

// readDeviceDescriptor reads the USB device descriptor
// On MIPS/embedded systems, reading directly from the file is more reliable
// than using IOCTL control transfers which require proper driver support
func readDeviceDescriptor(fd int) (*usbDeviceDescriptor, error) {
	// USB device files in /dev/bus/usb contain the device descriptor at the beginning
	// This is more portable than IOCTL control transfers
	buf := make([]byte, 18) // Standard device descriptor size

	// Seek to beginning of file
	_, err := syscall.Seek(fd, 0, 0)
	if err != nil {
		return nil, fmt.Errorf("failed to seek: %w", err)
	}

	// Read the device descriptor directly from the file
	n, err := syscall.Read(fd, buf)
	if err != nil {
		return nil, fmt.Errorf("failed to read descriptor: %w", err)
	}
	if n < 18 {
		return nil, fmt.Errorf("short read: got %d bytes, expected 18", n)
	}

	// Validate descriptor
	if buf[0] != 18 || buf[1] != USB_DT_DEVICE {
		return nil, fmt.Errorf("invalid descriptor: length=%d type=%d", buf[0], buf[1])
	}

	// Note: Standard USB stores VID/PID as little-endian, but the Bitmain ASIC
	// stores them as ASCII "BTAS" which appears as big-endian when read.
	// lsusb shows 4254:4153 but little-endian reading gives 5442:5341.
	// We read as big-endian to match what lsusb reports.
	desc := &usbDeviceDescriptor{
		bLength:            buf[0],
		bDescriptorType:    buf[1],
		bcdUSB:             binary.LittleEndian.Uint16(buf[2:4]),
		bDeviceClass:       buf[4],
		bDeviceSubClass:    buf[5],
		bDeviceProtocol:    buf[6],
		bMaxPacketSize0:    buf[7],
		idVendor:           binary.BigEndian.Uint16(buf[8:10]),
		idProduct:          binary.BigEndian.Uint16(buf[10:12]),
		bcdDevice:          binary.LittleEndian.Uint16(buf[12:14]),
		iManufacturer:      buf[14],
		iProduct:           buf[15],
		iSerialNumber:      buf[16],
		bNumConfigurations: buf[17],
	}

	return desc, nil
}

// detachKernelDriver detaches the kernel driver from a USB interface
func (d *USBDevice) detachKernelDriver(iface uint32) error {
	// USBDEVFS_DISCONNECT on MIPS: _IO('U', 22) = 0x20005516
	// This takes the interface number directly, not a struct
	log.Printf("Attempting to detach driver from interface %d with IOCTL 0x%x", iface, USBDEVFS_DISCONNECT)

	_, _, errno := syscall.Syscall(
		syscall.SYS_IOCTL,
		uintptr(d.fd),
		USBDEVFS_DISCONNECT,
		uintptr(iface), // Pass interface number directly for _IO style IOCTL
	)

	// Ignore errors - driver might not be attached
	if errno != 0 {
		log.Printf("DISCONNECT returned %v (driver may not be attached or IOCTL not supported)", errno)
	} else {
		log.Printf("Successfully detached kernel driver from interface %d", iface)
	}

	return nil
}

// claimInterface claims a USB interface using usbdevfs IOCTL
func (d *USBDevice) claimInterface(iface uint32) error {
	// Try to detach any kernel driver first (ignore errors - driver might not be attached)
	d.detachKernelDriver(iface)
	time.Sleep(50 * time.Millisecond)

	// Try claiming the interface
	// On MIPS, USBDEVFS_CLAIMINTERFACE = _IOR('U', 15, unsigned int) = 0x4004550f
	log.Printf("Attempting to claim interface %d with IOCTL 0x%x", iface, USBDEVFS_CLAIMINTERFACE)

	_, _, errno := syscall.Syscall(
		syscall.SYS_IOCTL,
		uintptr(d.fd),
		USBDEVFS_CLAIMINTERFACE,
		uintptr(unsafe.Pointer(&iface)),
	)

	if errno != 0 {
		log.Printf("CLAIMINTERFACE failed: errno=%v (%d)", errno, errno)

		// If claim fails, try a different approach - reset the device first
		log.Printf("Trying device reset before claim...")
		_, _, resetErr := syscall.Syscall(
			syscall.SYS_IOCTL,
			uintptr(d.fd),
			USBDEVFS_RESET,
			0,
		)
		if resetErr != 0 {
			log.Printf("Reset failed: %v", resetErr)
		} else {
			log.Printf("Reset succeeded, retrying claim...")
			time.Sleep(100 * time.Millisecond)

			_, _, errno = syscall.Syscall(
				syscall.SYS_IOCTL,
				uintptr(d.fd),
				USBDEVFS_CLAIMINTERFACE,
				uintptr(unsafe.Pointer(&iface)),
			)
			if errno == 0 {
				d.isClaimed = true
				return nil
			}
			log.Printf("Claim after reset also failed: %v", errno)
		}

		return fmt.Errorf("failed to claim interface %d: %v", iface, errno)
	}

	d.isClaimed = true
	log.Printf("Successfully claimed interface %d", iface)
	return nil
}

// Close closes the USB device
func (d *USBDevice) Close() error {
	if d.isClaimed {
		// Release interface
		iface := uint32(0)
		syscall.Syscall(
			syscall.SYS_IOCTL,
			uintptr(d.fd),
			USBDEVFS_RELEASEINTERFACE,
			uintptr(unsafe.Pointer(&iface)),
		)
	}

	if d.fd >= 0 {
		syscall.Close(d.fd)
	}

	return nil
}

// SendPacket sends a packet to the ASIC via USB bulk transfer
func (d *USBDevice) SendPacket(data []byte) error {
	if len(data) == 0 {
		return fmt.Errorf("empty packet")
	}

	// Bitmain ASIC uses endpoint 0x01 (OUT)
	buf := make([]byte, len(data))
	copy(buf, data)

	bulk := usbdevfsBulkTransfer{
		Ep:      EndpointOut,
		Len:     uint32(len(buf)),
		Timeout: 5000,
		Data:    unsafe.Pointer(&buf[0]),
	}

	_, _, errno := syscall.Syscall(
		syscall.SYS_IOCTL,
		uintptr(d.fd),
		USBDEVFS_BULK,
		uintptr(unsafe.Pointer(&bulk)),
	)

	if errno != 0 {
		return fmt.Errorf("USB bulk write failed: %v", errno)
	}

	return nil
}

// ReadPacket reads a packet from the ASIC via USB bulk transfer
func (d *USBDevice) ReadPacket(buffer []byte, timeout time.Duration) (int, error) {
	if len(buffer) == 0 {
		return 0, fmt.Errorf("empty buffer")
	}

	// Bitmain ASIC uses endpoint 0x81 (IN)
	bulk := usbdevfsBulkTransfer{
		Ep:      EndpointIn,
		Len:     uint32(len(buffer)),
		Timeout: uint32(timeout.Milliseconds()),
		Data:    unsafe.Pointer(&buffer[0]),
	}

	_, _, errno := syscall.Syscall(
		syscall.SYS_IOCTL,
		uintptr(d.fd),
		USBDEVFS_BULK,
		uintptr(unsafe.Pointer(&bulk)),
	)

	if errno != 0 {
		if errno == syscall.ETIMEDOUT {
			return 0, fmt.Errorf("USB read timeout")
		}
		return 0, fmt.Errorf("USB bulk read failed: %v", errno)
	}

	// Note: The actual transferred length is not returned by this IOCTL
	// We assume full packet was read, or we need to track it differently
	return len(buffer), nil
}

// GetChipCount returns the number of ASIC chips
func (d *USBDevice) GetChipCount() int {
	return d.chipCount
}

// Initialize performs USB-based ASIC initialization
// Follows the Bitmain protocol sequence:
//  1. Send TxConfig to initialize ASICs
//  2. Wait for initialization to complete
//  3. Send RxStatus to query device state
//  4. Read RxStatus response
func (d *USBDevice) Initialize() error {
	log.Printf("Initializing ASIC via USB...")

	// Note: SetConfiguration should be called BEFORE claiming interface
	// Interface is already claimed in OpenUSBDevice, so we skip SetConfiguration here
	// to avoid "interface 0 claimed by usbfs while setting config" kernel warnings

	// Step 1: Send TxConfig packet to initialize ASICs (MUST be first!)
	log.Printf("Step 1: Sending TxConfig packet (28 bytes)...")
	txConfigPacket := d.buildTxConfigPacket()
	log.Printf("TxConfig packet: %x", txConfigPacket)
	if err := d.SendPacket(txConfigPacket); err != nil {
		return fmt.Errorf("failed to send TxConfig: %w", err)
	}

	// Wait for ASICs to initialize (protocol says at least 1 second)
	log.Printf("Waiting for ASIC initialization (1 second)...")
	time.Sleep(1 * time.Second)

	// Step 2: Send RxStatus to query device state
	log.Printf("Step 2: Sending RxStatus packet (16 bytes)...")
	rxStatusPacket := d.buildRxStatusPacket()
	log.Printf("RxStatus packet: %x", rxStatusPacket)
	if err := d.SendPacket(rxStatusPacket); err != nil {
		return fmt.Errorf("failed to send RxStatus: %w", err)
	}

	// Try to read response
	response := make([]byte, 512)
	n, err := d.ReadPacket(response, 2*time.Second)
	if err != nil {
		log.Printf("RxStatus read: %v", err)
	} else if n > 0 {
		log.Printf("RxStatus response: %d bytes, type=0x%02x", n, response[0])
	}

	// Read verification response with retry logic
	var finalResponse []byte
	var finalN int
	for attempt := 0; attempt < 5; attempt++ {
		if attempt > 0 {
			log.Printf("Retry attempt %d/5 reading RxStatus response...", attempt)
			time.Sleep(500 * time.Millisecond)
		}

		n, err = d.ReadPacket(response, 2*time.Second)
		if err == nil && n > 0 {
			finalResponse = make([]byte, n)
			copy(finalResponse, response[:n])
			finalN = n
			break
		}
	}

	if finalN == 0 {
		// Device didn't respond to status query, but may still be operational
		// Some ASICs don't respond to RxStatus until they have work
		log.Printf("Warning: No RxStatus response received, but device may still be operational")
		log.Printf("ASIC USB initialization completed (device responsive, status query pending)")
		return nil
	}

	// Parse response header
	if finalN >= 4 {
		dataType := finalResponse[0]
		version := finalResponse[1]
		length := binary.LittleEndian.Uint16(finalResponse[2:4])
		log.Printf("RxStatus response: type=0x%02x version=%d length=%d total=%d bytes",
			dataType, version, length, finalN)

		if dataType == 0xA1 && finalN >= 12 {
			// Parse key fields from RxStatus response
			chainNum := finalResponse[5]
			fifoSpace := binary.LittleEndian.Uint16(finalResponse[6:8])
			hwVersion := finalResponse[8:12]
			log.Printf("ASIC Info: chains=%d fifo=%d hw_version=%d.%d.%d.%d",
				chainNum, fifoSpace, hwVersion[0], hwVersion[1], hwVersion[2], hwVersion[3])

			// Update chip count based on response
			if chainNum > 0 {
				d.chipCount = int(chainNum) * 4 // 4 chips per chain on S3
				log.Printf("Updated chip count: %d", d.chipCount)
			}
		}
	}

	log.Printf("USB ASIC initialization successful (%d bytes response)", finalN)
	return nil
}

// buildTxConfigPacket builds configuration packet for ASIC initialization
// Note: CRC calculation uses calculateCRC16 from controller.go
// Based on Antminer S3 defaults: 8 chains, 32 ASICs/chain, 250MHz, 0.982V
func (d *USBDevice) buildTxConfigPacket() []byte {
	// TxConfig packet is 28 bytes total
	packet := make([]byte, 28)

	// Header
	packet[0] = TokenTxConfig // 0x51
	packet[1] = 0x00          // version
	// Length: payload size (28 - 4 header = 24), little-endian
	binary.LittleEndian.PutUint16(packet[2:4], 24)

	// Control flags: fan, timeout, frequency, voltage enabled = 0x1E
	packet[4] = 0x1E
	packet[5] = 0x00 // reserved
	packet[6] = 0x0C // chain_check_time (12)
	packet[7] = 0x00 // reserved

	// ASIC configuration
	packet[8] = 8     // chain_num (8 chains for S3)
	packet[9] = 32    // asic_num (32 ASICs per chain)
	packet[10] = 0x60 // fan_pwm_data (96%)
	packet[11] = 0x0C // timeout_data (12)

	// Frequency: 250 MHz in little-endian
	binary.LittleEndian.PutUint16(packet[12:14], 250)

	// Voltage: 0x0982 (little-endian)
	packet[14] = 0x82
	packet[15] = 0x09

	// reg_data (4 bytes) - zeros for default
	packet[16] = 0x00
	packet[17] = 0x00
	packet[18] = 0x00
	packet[19] = 0x00

	// chip_address and reg_address
	packet[20] = 0x00 // chip_address (all chips)
	packet[21] = 0x00 // reg_address

	// Padding to align
	packet[22] = 0x00
	packet[23] = 0x00
	packet[24] = 0x00
	packet[25] = 0x00

	// Calculate and append CRC over bytes 0-25
	crc := calculateCRC16(packet[0:26])
	binary.LittleEndian.PutUint16(packet[26:28], crc)

	return packet
}

// buildRxStatusPacket builds a status query packet for Bitmain ASIC
// RxStatus packet is 16 bytes total including CRC
func (d *USBDevice) buildRxStatusPacket() []byte {
	packet := make([]byte, 16)

	// Header
	packet[0] = TokenRxStatus // 0x53
	packet[1] = 0x00          // version
	// Length: payload size (16 - 4 header = 12), little-endian
	binary.LittleEndian.PutUint16(packet[2:4], 12)

	// Flags
	packet[4] = 0x00 // flags (query all)
	packet[5] = 0x00 // reserved
	packet[6] = 0x00 // reserved
	packet[7] = 0x00 // reserved

	// Target addresses
	packet[8] = 0x00 // chip_address (all chips)
	packet[9] = 0x00 // reg_address

	// Padding
	packet[10] = 0x00
	packet[11] = 0x00
	packet[12] = 0x00
	packet[13] = 0x00

	// Calculate and append CRC over bytes 0-13
	crc := calculateCRC16(packet[0:14])
	binary.LittleEndian.PutUint16(packet[14:16], crc)

	return packet
}

// IsUSBDeviceAvailable returns true if USB access is available on this system
func IsUSBDeviceAvailable() bool {
	// Check if /dev/bus/usb exists and is accessible
	info, err := os.Stat("/dev/bus/usb")
	if err != nil {
		return false
	}
	return info.IsDir()
}
