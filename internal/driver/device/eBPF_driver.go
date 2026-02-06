package device

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"log"
	"net"

	"github.com/cilium/ebpf"
	"github.com/cilium/ebpf/link"
	"github.com/cilium/ebpf/ringbuf"
	"github.com/cilium/ebpf/rlimit"
)

// NonceEvent matches the struct in nonce_batcher.bpf.c
type NonceEvent struct {
	Nonce uint32
}

// BpfObjects contains eBPF programs and maps (stub)
type BpfObjects struct {
	XdpFilterUsb  *ebpf.Program `ebpf:"xdp_filter_usb"`
	NonceEvents   *ebpf.Map     `ebpf:"nonce_events"`
	TxTaskHeaders *ebpf.Map     `ebpf:"tx_task_headers"`
}

// Close closes all eBPF objects
func (o *BpfObjects) Close() error {
	if o.XdpFilterUsb != nil {
		o.XdpFilterUsb.Close()
	}
	if o.NonceEvents != nil {
		o.NonceEvents.Close()
	}
	if o.TxTaskHeaders != nil {
		o.TxTaskHeaders.Close()
	}
	return nil
}

// LoadBpfObjects loads eBPF objects (stub)
func LoadBpfObjects(obj interface{}, opts *ebpf.CollectionOptions) error {
	// This is a stub that returns nil for compilation
	// In a real implementation, this would load the actual eBPF objects
	return nil
}

type EBPFDriver struct {
	objs    BpfObjects
	xdpLink link.Link
	reader  *ringbuf.Reader
	iface   string // USB interface name
}

func NewEBPFDriver(usbInterface string) (*EBPFDriver, error) {
	if err := rlimit.RemoveMemlock(); err != nil {
		return nil, fmt.Errorf("failed to remove memlock rlimit: %w", err)
	}

	driver := &EBPFDriver{
		iface: usbInterface,
	}

	// Load the eBPF objects
	objs := BpfObjects{}
	if err := LoadBpfObjects(&objs, nil); err != nil {
		return nil, fmt.Errorf("failed to load eBPF objects: %w", err)
	}
	driver.objs = objs

	// Convert interface name to index
	iface, err := net.InterfaceByName(usbInterface)
	if err != nil {
		return nil, fmt.Errorf("failed to get interface %s: %w", usbInterface, err)
	}
	// Attach the XDP program to the USB interface
	l, err := link.AttachXDP(link.XDPOptions{
		Program:   objs.XdpFilterUsb,
		Interface: iface.Index,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to attach XDP program to %s: %w", usbInterface, err)
	}
	driver.xdpLink = l

	// Create a new ring buffer reader
	reader, err := ringbuf.NewReader(objs.NonceEvents)
	if err != nil {
		return nil, fmt.Errorf("failed to create ring buffer reader: %w", err)
	}
	driver.reader = reader

	log.Printf("EBPFDriver initialized on interface: %s", usbInterface)
	return driver, nil
}

func (d *EBPFDriver) Close() {
	if d.xdpLink != nil {
		if err := d.xdpLink.Close(); err != nil {
			log.Printf("Error closing XDP link: %v", err)
		}
	}
	if d.reader != nil {
		if err := d.reader.Close(); err != nil {
			log.Printf("Error closing ring buffer reader: %v", err)
		}
	}
	if d.objs.XdpFilterUsb != nil {
		d.objs.Close()
	}
	log.Printf("EBPFDriver closed for interface: %s", d.iface)
}

// WriteTxTaskHeader constructs and sends a TxTask packet via eBPF.
// This is a conceptual implementation. A real eBPF solution for USB
// writes would be more complex, potentially involving a kprobe/uprobe
// on a userspace USB write function or a dedicated kernel module.
// For this PoC, we will simulate passing the header to the eBPF program
// via an eBPF map.
func (d *EBPFDriver) WriteTxTaskHeader(header []byte) error {
	// The header is the 80-byte Bitcoin block header.
	// We need to construct the full TxTask packet (4-byte header + 80-byte payload + 2-byte CRC).
	// For simplicity in this PoC, we assume the eBPF program handles the TxTask packet
	// construction and CRC calculation around the 80-byte header.
	// We'll pass the 80-byte header to the eBPF program via a map.

	if len(header) != 80 {
		return fmt.Errorf("TxTask header must be 80 bytes, got %d", len(header))
	}

	// For a real implementation, we would write the header to a specific
	// USB OUT endpoint. This is a placeholder for that complex interaction.
	// We'll use an eBPF map to simulate passing the data.
	key := uint32(0) // Use a fixed key for simplicity, or manage multiple queues

	// Create a byte array with a fixed size to match the eBPF map value
	var ebpfHeader [80]byte
	copy(ebpfHeader[:], header)

	if err := d.objs.TxTaskHeaders.Put(key, ebpfHeader); err != nil {
		return fmt.Errorf("failed to write TxTask header to eBPF map: %w", err)
	}

	// Trigger the eBPF program to process this header
	// This would typically be done by a kernel hook, e.g., when a write to /dev/bitmain-asic occurs
	// For this PoC, we'll assume the XDP program can be "prodded" or that
	// a kprobe on the USB driver's write function will pick this up.
	// This is highly conceptual and needs a dedicated mechanism to signal the eBPF program.

	log.Printf("eBPFDriver: TxTask header (80 bytes) sent to eBPF map. Key: %d", key)
	return nil
}

// ComputeNonceBucket sends a TxTask header and reads the resulting nonce.
// This function will encapsulate the interaction with the eBPF program for
// sending the header and receiving the nonce.
func (d *EBPFDriver) ComputeNonceBucket(header []byte) (uint32, error) {
	if err := d.WriteTxTaskHeader(header); err != nil {
		return 0, fmt.Errorf("failed to write TxTask header: %w", err)
	}

	// Read the nonce from the eBPF ring buffer
	// This assumes the eBPF program has already sent the header
	// and processed the RxNonce response from the ASIC.
	return d.ReadNonce()
}

// ReadNonce reads a single nonce from the eBPF ring buffer.
func (d *EBPFDriver) ReadNonce() (uint32, error) {
	record, err := d.reader.Read()
	if err != nil {
		if errors.Is(err, ringbuf.ErrClosed) {
			return 0, fmt.Errorf("ring buffer closed: %w", err)
		}
		return 0, fmt.Errorf("failed to read from ring buffer: %w", err)
	}

	var event NonceEvent // Use the generated type
	if err := binary.Read(bytes.NewBuffer(record.RawSample), binary.LittleEndian, &event); err != nil {
		return 0, fmt.Errorf("failed to decode nonce event from ring buffer: %w", err)
	}

	log.Printf("eBPFDriver: Received nonce from ring buffer: %d", event.Nonce)
	return event.Nonce, nil
}
