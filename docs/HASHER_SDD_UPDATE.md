
## HASHER SDD Update: ASIC Nonce-Mining for LSH

**Project HASHER: Adaptive Design for BM1382 Hardware Constraints**

---

### **1. Executive Summary: The Hardware Reality Pivot**

The ASIC is designed for Bitcoin mining (finding nonces where SHA256(SHA256(header+nonce)) < target), but the HASHER needs deterministic hashing (SHA256(input || seed) → fixed output). The original design assumed the BM1382 could perform arbitrary SHA-256 hashes. Testing has confirmed the ASIC is hard-wired for the Bitcoin mining loop: .

**Revised Core Innovation:** Instead of using the ASIC as a hash function, HASHER now uses it as a **deterministic bucket generator**. By setting a "Difficulty 1" target, we use the first valid **Nonce** discovered as the LSH signature. This maintains the 500 GH/s speed advantage by repurposing the mining hardware's natural state.

---

### **2. Updated Technical Architecture**

#### **2.1 From Hashing to Mining**

We must pack the 128-bit LSH projections into the standard 80-byte Bitcoin block header structure to be processed by the `0x52 (TXTASK)` protocol.

**Header Mapping:**

* **Version (4 bytes):** Used as a "Salt" or Seed for the LSH forest.
* **Previous Block Hash (32 bytes):** Stores the first 4 LSH projections (4x32-bit floats).
* **Merkle Root (32 bytes):** Stores the next 4 LSH projections.
* **Timestamp/Bits (8 bytes):** Fixed metadata to ensure determinism.
* **Nonce (4 bytes):** The output we seek from the ASIC.

#### **2.2 Updated ASIC Dispatcher (Go)**

The dispatcher no longer sends raw strings; it constructs binary headers that look like valid mining work.

```go
// pkg/asic/header_builder.go

func BuildMiningHeader(projections []float32, salt uint32) []byte {
    header := make([]byte, 80)
    
    // 1. Version (Salt)
    binary.LittleEndian.PutUint32(header[0:4], salt)
    
    // 2. Previous Block Hash (Projections 0-7)
    // We treat the 32-byte field as 8 float32s
    for i := 0; i < 8; i++ {
        val := math.Float32bits(projections[i])
        binary.LittleEndian.PutUint32(header[4+(i*4):8+(i*4)], val)
    }
    
    // 3. Merkle Root (Projections 8-15)
    for i := 0; i < 8; i++ {
        val := math.Float32bits(projections[8+i])
        binary.LittleEndian.PutUint32(header[36+(i*4):40+(i*4)], val)
    }

    // 4. Fixed Difficulty Bits (0x1d00ffff = Difficulty 1)
    binary.LittleEndian.PutUint32(header[72:76], 0x1d00ffff)
    
    return header
}

```

---

### **3. LSH Theory: Temporal Recursive Nonces**

To handle the **61MB RAM constraint**, we implement a **Temporal Recursive Algorithm**. If a bucket collision occurs, we "mine deeper" by using the previously found nonce as a seed for the next task.

#### **3.1 The Multi-Nonce Signature**

Instead of one 128-bit signature, we collect the first  nonces that satisfy the target.

* **LSH Bucket ID:** 
* **Rerank Filter:** 
* **Determinism Guarantee:** We specify a `Nonce Range` (e.g., 0 to 1,000,000). The ASIC will always find the same "Golden Nonce" for the same projection data within that range.

---

### **4. Protocol Implementation Updates**

#### **4.1 Revised TXTASK Packet (0x52)**

The payload must now strictly adhere to the 80-byte header format expected by the BM1382.

```go
// pkg/asic/hasher.go (Updated)

func (h *LSHHasher) ComputeNonceBucket(header []byte) (uint32, error) {
    // 1. Construct the 0x52 Token Packet
    packet := make([]byte, 4 + 80 + 2)
    packet[0] = 0x52 // TXTASK
    packet[1] = 0x01 // Version
    binary.LittleEndian.PutUint16(packet[2:4], 80) // Payload Length
    copy(packet[4:84], header)
    
    // 2. Write to /dev/bitmain-asic
    if err := h.device.Write(packet); err != nil {
        return 0, err
    }

    // 3. Read RXSTATUS (0x53) to find the Golden Nonce
    // The ASIC will return the first nonce that hits the Difficulty 1 target.
    nonce, err := h.pollForNonce() 
    return nonce, err
}

```

---

### **5. Performance & Constraints Re-evaluation**

| Metric | Original Design | Updated Design (Mining) |
| --- | --- | --- |
| **Hashing Latency** | 100µs (Direct) | ~1-2ms (Mining Search + USB) |
| **Determinism** | High (SHA-256) | High (First Nonce in Range) |
| **CPU Load** | Moderate | Low (Header packing is simple math) |
| **RAM Usage** | 61MB | 61MB (No change, uses mmap index) |

**Bottleneck Analysis Update:**
The primary bottleneck is no longer SHA-256 computation, but the **USB Bulk Transfer** and the **time-to-first-nonce**. At 500 GH/s, the hardware finds a Difficulty 1 nonce in nanoseconds, meaning the total search time remains dominated by the network call to the API server (~42ms).

---

### **6. Implementation Roadmap: Immediate Next Steps**

1. **Header Verification:** Flash a modified `cgminer` to the S3 to dump raw `0x52` packets and ensure our Go-built headers are accepted by the BM1382 PIC.
2. **Nonce Stability Test:** Run the same vector through 10 different Antminers to verify they all return the same "First Nonce" for a fixed range.
3. **Index Migration:** Update `LSHIndex` to use 32-bit Nonces as keys in the memory-mapped B-tree.

