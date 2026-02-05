// internal/driver/device/ioctl.go
// IOCTL-based communication with Bitmain ASIC kernel driver
// This provides an alternative to direct device access when the kernel module is loaded

package device

import (
	"fmt"
	"os"
	"syscall"
	"unsafe"
)

// IOCTL command construction for MIPS (32-bit) Linux
// Following standard Linux IOCTL encoding from <asm/ioctl.h>
const (
	// Direction bits
	IOC_NONE  = 0x0
	IOC_WRITE = 0x1
	IOC_READ  = 0x2

	// Bit shift amounts
	IOC_NRBITS   = 8
	IOC_TYPEBITS = 8
	IOC_SIZEBITS = 13
	IOC_DIRBITS  = 3

	IOC_NRMASK   = (1 << IOC_NRBITS) - 1
	IOC_TYPEMASK = (1 << IOC_TYPEBITS) - 1
	IOC_SIZEMASK = (1 << IOC_SIZEBITS) - 1
	IOC_DIRMASK  = (1 << IOC_DIRBITS) - 1

	IOC_NRSHIFT   = 0
	IOC_TYPESHIFT = IOC_NRSHIFT + IOC_NRBITS
	IOC_SIZESHIFT = IOC_TYPESHIFT + IOC_TYPEBITS
	IOC_DIRSHIFT  = IOC_SIZESHIFT + IOC_SIZEBITS
)

// Bitmain-specific IOCTL magic numbers (discovered from driver analysis)
// These are educated guesses based on common patterns
const (
	BitmainMagic     = 0x42 // 'B' for Bitmain
	BitmainAltMagic  = 0x10 // Alternative magic seen in some drivers
	BitmainASICMagic = 0x43 // 'C' for Chip

	// Standard IOCTL numbers for ASIC control
	IOCTL_GET_VERSION     = 0x01
	IOCTL_GET_STATUS      = 0x02
	IOCTL_RESET_DEVICE    = 0x03
	IOCTL_SET_FREQUENCY   = 0x04
	IOCTL_GET_TEMPERATURE = 0x05
	IOCTL_GET_HASHRATE    = 0x06
	IOCTL_ENABLE_CHIP     = 0x07
	IOCTL_DISABLE_CHIP    = 0x08
	IOCTL_SEND_WORK       = 0x10
	IOCTL_READ_RESULT     = 0x11
)

// IOCTL command constructors
func IOC(dir, typ, nr, size uint32) uintptr {
	return uintptr((dir << IOC_DIRSHIFT) | (size << IOC_SIZESHIFT) | (typ << IOC_TYPESHIFT) | (nr << IOC_NRSHIFT))
}

func IO(typ, nr uint32) uintptr         { return IOC(IOC_NONE, typ, nr, 0) }
func IOR(typ, nr, size uint32) uintptr  { return IOC(IOC_READ, typ, nr, size) }
func IOW(typ, nr, size uint32) uintptr  { return IOC(IOC_WRITE, typ, nr, size) }
func IOWR(typ, nr, size uint32) uintptr { return IOC(IOC_READ|IOC_WRITE, typ, nr, size) }

// IOCTLDevice provides IOCTL-based communication with the ASIC
type IOCTLDevice struct {
	file *os.File
	fd   uintptr
}

// OpenIOCTLDevice attempts to open device and use IOCTL commands
// This works WITH the loaded kernel module instead of bypassing it
func OpenIOCTLDevice() (*IOCTLDevice, error) {
	// Try to open device - may succeed even with module loaded
	file, err := os.OpenFile(DevicePath, os.O_RDWR, 0)
	if err != nil {
		// Try read-only as fallback
		file, err = os.OpenFile(DevicePath, os.O_RDONLY, 0)
		if err != nil {
			return nil, fmt.Errorf("failed to open device for IOCTL: %w", err)
		}
	}

	return &IOCTLDevice{
		file: file,
		fd:   file.Fd(),
	}, nil
}

// Close closes the IOCTL device
func (d *IOCTLDevice) Close() error {
	if d.file != nil {
		return d.file.Close()
	}
	return nil
}

// TryIOCTL attempts to execute an IOCTL command
// Returns true if successful, false if not supported
func (d *IOCTLDevice) TryIOCTL(cmd uintptr, data unsafe.Pointer) (bool, error) {
	_, _, errno := syscall.Syscall(
		syscall.SYS_IOCTL,
		d.fd,
		cmd,
		uintptr(data),
	)

	if errno != 0 {
		// Check if it's "not supported" vs "permission denied"
		if errno == syscall.ENOTTY || errno == syscall.EINVAL {
			// Command not supported by driver
			return false, nil
		}
		return false, errno
	}

	return true, nil
}

// DiscoverIOCTLs probes the driver to find valid IOCTL commands
func (d *IOCTLDevice) DiscoverIOCTLs() map[string]uintptr {
	validIOCTLs := make(map[string]uintptr)

	// Test various magic numbers
	magics := []uint32{BitmainMagic, BitmainAltMagic, BitmainASICMagic, 0x00, 0x01}

	// Buffer for testing
	var buf [256]byte

	for _, magic := range magics {
		for nr := uint32(0); nr < 32; nr++ {
			// Try simple _IO commands first
			cmd := IO(magic, nr)
			if ok, _ := d.TryIOCTL(cmd, unsafe.Pointer(&buf[0])); ok {
				validIOCTLs[fmt.Sprintf("IO_MAGIC%d_NR%d", magic, nr)] = cmd
			}
		}
	}

	return validIOCTLs
}

// GetDeviceInfoViaIOCTL attempts to get device info using IOCTL
func (d *IOCTLDevice) GetDeviceInfoViaIOCTL() (*DeviceInfo, error) {
	// Try to get version info
	var version uint32
	cmd := IOR(BitmainMagic, IOCTL_GET_VERSION, 4)

	if ok, err := d.TryIOCTL(cmd, unsafe.Pointer(&version)); ok {
		return &DeviceInfo{
			DevicePath:      DevicePath,
			ChipCount:       32, // Default for S3
			FirmwareVersion: fmt.Sprintf("%d", version),
			IsOperational:   true,
			FileDescriptor:  int(d.fd),
		}, nil
	} else if err != nil {
		return nil, fmt.Errorf("IOCTL get version failed: %w", err)
	}

	return nil, fmt.Errorf("IOCTL commands not supported by driver")
}

// SendWorkViaIOCTL sends mining work via IOCTL
func (d *IOCTLDevice) SendWorkViaIOCTL(data []byte) error {
	// This would implement the actual work submission via IOCTL
	// For now, return not implemented
	return fmt.Errorf("IOCTL work submission not yet implemented")
}

// ReadResultViaIOCTL reads mining results via IOCTL
func (d *IOCTLDevice) ReadResultViaIOCTL() ([]byte, error) {
	// This would implement result reading via IOCTL
	// For now, return not implemented
	return nil, fmt.Errorf("IOCTL result reading not yet implemented")
}

// IsIOCTLSupported checks if the device supports IOCTL commands
func (d *IOCTLDevice) IsIOCTLSupported() bool {
	// Try a simple IOCTL to test support
	var dummy uint32
	cmd := IO(BitmainMagic, 0)
	ok, _ := d.TryIOCTL(cmd, unsafe.Pointer(&dummy))
	return ok
}
