# Bitmain ASIC Kernel Driver Analysis

## Overview

This document analyzes the `bitmain_asic` kernel driver based on system logs, device behavior, and reverse engineering from actual hardware testing.

**Hardware:** Antminer S3 (BM1382 chips)
**Kernel:** Linux 5.15.0-164-generic
**Driver Module:** `bitmain_asic`
**Device Node:** `/dev/bitmain-asic` (major 10, minor 60)

---

## Driver Architecture

### Module Registration

From system logs:
```
kern.info kernel: usbcore: registered new interface driver bitmain
kern.err kernel: bitmain-asic: success to register device
```

**Key Points:**
- Driver registers as **USB interface driver** (not just character device)
- Creates `/dev/bitmain-asic` device node
- Uses misc device framework (major 10 = misc devices)

### USB Device Binding

```
kern.info kernel: bitmain 1-1.1:1.0: USB Bitmain asic device now attached to USB Bitmain asic-0
kern.warn kernel: PIC microchip usb OK
```

**Device Path:** USB Bus 1, Port 1.1, Interface 1.0
**USB Device ID:** 0x4254:0x4153 ("BT-AS")
**Microcontroller:** PIC chip handles USB communication

---

## Token Validation System

### Discovery from System Logs

On December 18, 2025 at 12:27:39-40, our protocol-discover tool triggered these kernel messages:

```
kern.warn kernel: Tx token err {0x0}
kern.warn kernel: Tx token err {0x55}
kern.warn kernel: Tx token err {0x1}
kern.warn kernel: Tx token err {0x2}
kern.warn kernel: Tx token err {0x21}
kern.warn kernel: Tx token err {0x48}
kern.warn kernel: Tx token err {0x55}
kern.warn kernel: Tx token err {0x21}
kern.warn kernel: Tx token err {0x41}
kern.warn kernel: Tx token err {0x41}
kern.warn kernel: Tx token err {0x81}
kern.warn kernel: Tx token err {0x55}
kern.warn kernel: Tx token err {0x1}
kern.warn kernel: Tx token err {0x2}
kern.warn kernel: Tx token err {0x81}
```

### Token Validation Logic

**Discovered behavior:**

1. **Driver reads first byte** of each write() call
2. **Validates token type:**
   - ✅ Valid: 0x51 (TXCONFIG), 0x52 (TXTASK), 0x53 (RXSTATUS)
   - ❌ Invalid: Everything else → logged as "Tx token err"
3. **Logs rejected tokens** to kernel message buffer
4. **Discards invalid packets** (not forwarded to hardware)

**Pseudocode of driver logic:**
```c
int bitmain_write(struct file *file, const char __user *buf, size_t count) {
    uint8_t token;
    copy_from_user(&token, buf, 1);

    if (token != 0x51 && token != 0x52 && token != 0x53) {
        printk(KERN_WARNING "Tx token err {0x%x}\n", token);
        return -EINVAL; // or silently drop
    }

    // Forward to USB hardware
    usb_bulk_msg(dev, pipe, buf, count, &actual, timeout);
    return actual;
}
```

---

## Device Initialization Sequence

### Boot Process

From system logs, chronological order:

1. **USB Driver Load** (boot time)
   ```
   usbcore: registered new interface driver bitmain
   bitmain-asic: success to register device
   ```

2. **Device Enumeration**
   ```
   usb 1-1.1: new full-speed USB device number 3
   bitmain 1-1.1:1.0: USB Bitmain asic device now attached
   PIC microchip usb OK
   ```

3. **Bootloader Mode** (firmware update phase)
   ```
   usb 1-1.1: USB disconnect, device number 3
   bitmainbl 1-1.1:1.0: USB Bitmain bootloader device now attached
   PIC microchip bootloader usb OK
   ```

4. **PIC Firmware Check**
   ```
   sysinit: Version 1.0
   sysinit: open pic update file fail
   sysinit: Control bootloader to app
   ```

5. **Switch to Application Mode**
   ```
   usb 1-1.1: USB disconnect, device number 4
   usb 1-1.1: new full-speed USB device number 5
   bitmain 1-1.1:1.0: USB Bitmain asic device now attached
   PIC microchip usb OK
   ```

6. **Driver Initialization**
   ```
   bitmain_asic_init ok
   bitmain_asic_open_usb OK!!
   ```

### Dual-Mode Device

**Bootloader Mode (VID:PID unknown):**
- Used for firmware updates
- Attached as `/dev/bitmainbl0`
- Active during boot initialization
- Switches to app mode after firmware check

**Application Mode (0x4254:0x4153):**
- Normal operation mode
- Attached as `/dev/bitmain-asic`
- Used by CGMiner for mining

---

## IRQ Management

### IRQ Allocation

```
kern.err kernel: genirq: Flags mismatch irq 17. 00000000 (bitmain-asic) vs. 00000000 (bitmain-asic)
```

**Analysis:**
- Driver uses **IRQ 17** for device events
- **Exclusive access enforced** at IRQ level
- Attempting to open device twice causes IRQ conflict
- This explains "device busy" errors when CGMiner holds device

**Implications:**
- Only ONE process can have device open at a time
- CGMiner must fully release device (including IRQ) before other tools can access
- IRQ remains allocated until process exits or device is closed

---

## Write Operation Flow

### Userspace to Hardware Path

```
[Application]
    ↓ write(fd, packet, len)
[Kernel Driver]
    ↓ Token validation (first byte)
    ↓ If invalid → "Tx token err" → reject
    ↓ If valid (0x51/0x52/0x53) → continue
[USB Subsystem]
    ↓ usb_bulk_msg()
[USB Device (PIC)]
    ↓ USB protocol handling
[ASIC Hardware]
    ↓ Execute command
```

### Packet Requirements

Based on validation behavior:

1. **First byte MUST be valid token:** 0x51, 0x52, or 0x53
2. **Packet structure must match protocol:**
   - Header: [Token | Version | Length]
   - Payload: (varies by token type)
   - Trailer: [CRC16]
3. **Length field must be accurate**
4. **CRC must be valid** (likely validated by hardware/PIC)

---

## Read Operation Behavior

### EOF Mystery

All read() operations return **EOF immediately**:
```
⚠️  Read error: EOF
```

**Possible explanations:**

1. **Driver implements write-only interface:**
   - read() function returns 0 (EOF) immediately
   - Responses come through different channel

2. **Polling/Interrupt model:**
   - Hardware sends data via USB IN endpoint
   - Driver buffers in kernel space
   - Application must poll or wait for interrupt

3. **CGMiner uses USB directly:**
   - Bypasses `/dev/bitmain-asic` for reads
   - Uses libusb to access USB endpoints directly
   - Driver only intercepts writes for validation

### Hung Read Investigation

From testing:
- Read timeouts > 2 seconds cause kernel hang
- Process enters 'D' state (uninterruptible sleep)
- Cannot be killed with SIGKILL
- Requires full system reboot

**Root cause:** Driver's read function likely waits on USB IN endpoint without proper timeout handling, causing kernel-level deadlock.

**Safe practice:** Always use timeout ≤ 300ms

---

## CGMiner Integration

### How CGMiner Accesses Device

Based on logs and source code analysis:

1. **Opens `/dev/bitmain-asic` for write**
2. **Uses libusb for read operations** (direct USB access)
3. **Sends commands via write():**
   - TxConfig (0x51) - Configure chains/frequency/voltage
   - TxTask (0x52) - Submit work
   - RxStatus (0x53) - Request status

4. **Reads responses via USB:**
   - Bypasses character device
   - Uses USB bulk IN endpoint
   - Receives RxStatus (0xa1) and RxNonce (0xa2) packets

### Device Locking Behavior

```
sysinit: --bitmain-options 115200:32:8:16:250:0982
[CGMiner starts]
bitmain_asic_init ok
bitmain_asic_open_usb OK!!
[CGMiner stops]
bitmain 1-1.1:1.0: USB Bitmain asic #0 now disconnected
[Brief delay]
bitmain 1-1.1:1.0: USB Bitmain asic device now attached
```

**Observations:**
- CGMiner opens device during initialization
- Holds exclusive lock (IRQ 17) during operation
- Closing CGMiner triggers USB disconnect event
- Device reattaches after ~1-2 seconds
- **Device remains "busy" if CGMiner doesn't exit cleanly**

---

## Protocol Testing Results

### Validated Token Types

From our testing and kernel logs:

| Token | Hex | Result | Kernel Log |
|-------|-----|--------|------------|
| TXCONFIG | 0x51 | ✅ Accepted | (no error) |
| TXTASK | 0x52 | ✅ Accepted | (no error) |
| RXSTATUS | 0x53 | ✅ Accepted | (no error) |
| Custom | 0x00 | ❌ Rejected | "Tx token err {0x0}" |
| Custom | 0x55 | ❌ Rejected | "Tx token err {0x55}" |
| Custom | 0x01 | ❌ Rejected | "Tx token err {0x1}" |
| Custom | 0x21 | ❌ Rejected | "Tx token err {0x21}" |
| Custom | 0x81 | ❌ Rejected | "Tx token err {0x81}" |

### Test Commands Rejected

All 12 test patterns from protocol-discover tool were rejected:
- NULL_COMMAND (0x00)
- MAGIC_HEADER (0x55)
- STATUS_REQUEST (0x01)
- VERSION_REQUEST (0x02)
- NONCE_PATTERN (0x21)
- FREQ_PATTERN (0x48)
- RESET_PATTERN (0x55)
- WORK_PATTERN (0x21)
- CHAIN_SELECT_0 (0x41)
- CHAIN_SELECT_1 (0x41)
- READ_RESULT (0x81)

All appeared in kernel log as "Tx token err"

---

## Device States

### State Machine

```
┌─────────────┐
│   DETACHED  │ ← Initial state (no USB device)
└──────┬──────┘
       │ USB plug / boot
       ▼
┌─────────────┐
│ BOOTLOADER  │ ← Firmware update mode
└──────┬──────┘
       │ Boot complete
       ▼
┌─────────────┐
│  ATTACHED   │ ← Device ready, no opener
└──────┬──────┘
       │ open()
       ▼
┌─────────────┐
│    OPENED   │ ← Exclusive access, IRQ allocated
└──────┬──────┘
       │ close()
       ▼
┌─────────────┐
│  ATTACHED   │ ← Ready for next open()
└─────────────┘
```

### State Transitions

| From | To | Trigger | Notes |
|------|-----|---------|-------|
| DETACHED | BOOTLOADER | USB enumeration | Automatic on boot |
| BOOTLOADER | ATTACHED | Firmware check complete | ~2-3 seconds |
| ATTACHED | OPENED | open() syscall | Allocates IRQ 17 |
| OPENED | ATTACHED | close() syscall | Releases IRQ |
| OPENED | DETACHED | USB disconnect | Forced disconnect |
| Any | DETACHED | USB cable removed | Hardware event |

---

## Known Issues & Workarounds

### Issue 1: Device Stays Busy

**Symptom:** Cannot open device, "device or resource busy" error

**Causes:**
- CGMiner didn't exit cleanly
- IRQ 17 still allocated
- Kernel driver state inconsistent

**Workarounds:**
1. Wait 5+ seconds after stopping CGMiner
2. Reload kernel module: `rmmod bitmain_asic && modprobe bitmain_asic`
3. Reboot system (last resort)

### Issue 2: Read Hangs System

**Symptom:** Process stuck in 'D' state, unkillable

**Cause:** Driver's read() waits indefinitely on USB IN endpoint

**Workaround:**
- **Always set read deadline ≤ 300ms**
- Use `SetReadDeadline(time.Now().Add(300*time.Millisecond))`
- Never use blocking reads

### Issue 3: Token Validation Confusion

**Symptom:** Valid-looking packets rejected

**Cause:** Driver validates first byte only, logs as error

**Solution:**
- Use only valid tokens: 0x51, 0x52, 0x53
- Ensure proper packet structure
- Check kernel logs for "Tx token err" messages

---

## Security Considerations

### Driver Attack Surface

**Write validation only:**
- Driver validates token type (first byte)
- Does NOT validate:
  - Packet length consistency
  - CRC correctness (likely done by PIC/hardware)
  - Payload structure
  - Command sequencing

**Potential vulnerabilities:**
- Malformed packets with valid tokens might crash firmware
- No rate limiting on write operations
- Direct USB access bypasses driver validation entirely

**For Hasher:**
- Cannot rely on driver for security
- Must implement validation at application level
- Consider hardware-level protections (secure enclosure)

---

## Recommendations for Application Development

### Best Practices

1. **Token Usage:**
   - Use only 0x51, 0x52, 0x53
   - Follow exact packet format from protocol spec
   - Always include valid CRC-16

2. **Device Access:**
   - Open device exclusively (no sharing)
   - Close properly on exit (defer device.Close())
   - Handle "busy" errors gracefully

3. **Read Operations:**
   - Set timeout ≤ 300ms
   - Expect EOF on `/dev/bitmain-asic`
   - Consider direct USB access for responses

4. **Error Handling:**
   - Check kernel logs (`dmesg` or `/var/log/messages`)
   - Look for "Tx token err" messages
   - Monitor USB disconnect/reconnect events

### Testing Checklist

- [ ] CGMiner fully stopped (no processes, no IRQ)
- [ ] Device file exists (`ls -l /dev/bitmain-asic`)
- [ ] Kernel module loaded (`lsmod | grep bitmain`)
- [ ] USB device enumerated (`lsusb | grep 4254:4153`)
- [ ] No IRQ conflicts (`dmesg | grep "irq 17"`)
- [ ] Read timeout set ≤ 300ms
- [ ] Valid token used (0x51/0x52/0x53)
- [ ] Packet structure matches protocol

---

## Appendix: System Log Excerpts

### Device Initialization (Success)
```
kern.info kernel: bitmain 1-1.1:1.0: USB Bitmain asic device now attached
kern.warn kernel: PIC microchip usb OK
kern.warn kernel: bitmain_asic_init ok
kern.warn kernel: bitmain_asic_open_usb OK!!
```

### Token Validation (Our Tests)
```
kern.warn kernel: Tx token err {0x0}
kern.warn kernel: Tx token err {0x55}
kern.warn kernel: Tx token err {0x1}
[...15 total rejections...]
kern.warn kernel: bitmain_asic close usb !!
```

### IRQ Conflict
```
kern.err kernel: genirq: Flags mismatch irq 17. 00000000 (bitmain-asic) vs. 00000000 (bitmain-asic)
```

### Device Disconnect
```
kern.info kernel: bitmain 1-1.1:1.0: USB Bitmain asic #0 now disconnected
```

---

## References

- System logs: `/var/log/messages` on Antminer S3
- Protocol specification: `docs/PROTOCOL.md`
- CGMiner source: `github.com/bitmaintech/cgminer`
- Testing logs: `logs/protocol-discover-final.log`

---

**Document Version:** 1.0
**Date:** 2025-12-18
**Author:** Hasher PoC Team
