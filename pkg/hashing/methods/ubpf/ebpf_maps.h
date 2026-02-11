#ifndef __HASHER_MAPS_H
#define __HASHER_MAPS_H

#include <stdint.h>

/* * Neural Frame structure: Camouflaged for BM1382 
 * This represents the "Problem" sent from Go to the eBPF kernel.
 */
struct neural_frame {
    // 12 slots * 4 bytes = 48 bytes of semantic embedding data
    uint32_t slots[12];
    // The target token ID the ASIC is hunting for
    uint32_t target_token_id;
    // Padding to ensure the struct aligns with hardware cache lines
    uint32_t padding[3]; 
};

/* * Seed Result structure: The "Solution"
 * This represents the "Answer" (Golden Nonce) found by the hardware.
 */
struct seed_result {
    // The 32-bit Golden Nonce (Seed) that satisfied the Double-SHA256 match
    uint32_t best_seed;
    // Flag to indicate a match was found during the epoch
    uint32_t match_found;
    // Metadata for the GRPO reward calculation (e.g., hash prefix/stability)
    uint32_t reward_metadata[6]; 
};

/* * Map Definitions
 * These must be compatible with the uBPF implementation used in the 
 * Data Trainer simulator.
 */

// Map 0: Input Map (Go -> eBPF)
// Stores the current training frame with camouflaged header data.
struct bpf_map_def {
    unsigned int type;
    unsigned int key_size;
    unsigned int value_size;
    unsigned int max_entries;
};

// Map configuration for the Data Trainer orchestrator
#define MAP_TYPE_ARRAY 2

// For uBPF compatibility, we'll define maps as static structures
// The SEC() macro is only used during eBPF compilation
#ifdef __BPF__
#define SEC(NAME) __attribute__((section(NAME), used))
#else
#define SEC(NAME)
#endif

static struct bpf_map_def SEC("maps") frame_map = {
    .type = MAP_TYPE_ARRAY,
    .key_size = sizeof(uint32_t),
    .value_size = sizeof(struct neural_frame),
    .max_entries = 1, // One frame per training worker
};

static struct bpf_map_def SEC("maps") result_map = {
    .type = MAP_TYPE_ARRAY,
    .key_size = sizeof(uint32_t),
    .value_size = sizeof(struct seed_result),
    .max_entries = 1,
};

/* * Helper functions for Bitcoin header processing
 */
static inline uint32_t extract_nonce_from_header(const uint8_t* header) {
    // Extract nonce from bytes 76-80 (Little-Endian)
    return ((uint32_t)header[76]) |
           ((uint32_t)header[77] << 8) |
           ((uint32_t)header[78] << 16) |
           ((uint32_t)header[79] << 24);
}

static inline void extract_slots_from_header(const uint8_t* header, uint32_t* slots) {
    // Extract slots 0-7 from PrevBlockHash (bytes 4-36, Big-Endian)
    for (int i = 0; i < 8; i++) {
        slots[i] = ((uint32_t)header[4 + i*4] << 24) |
                  ((uint32_t)header[5 + i*4] << 16) |
                  ((uint32_t)header[6 + i*4] << 8) |
                  ((uint32_t)header[7 + i*4]);
    }
    
    // Extract slots 8-11 from MerkleRoot (bytes 36-68, Big-Endian)
    for (int i = 0; i < 4; i++) {
        slots[i + 8] = ((uint32_t)header[36 + i*4] << 24) |
                       ((uint32_t)header[37 + i*4] << 16) |
                       ((uint32_t)header[38 + i*4] << 8) |
                       ((uint32_t)header[39 + i*4]);
    }
}

/* * Constants for Bitcoin header validation
 */
#define BITCOIN_VERSION 0x00000002
#define BITCOIN_BITS    0x1d00ffff

static inline int validate_bitcoin_header(const uint8_t* header) {
    // Check version (bytes 0-3, Little-Endian)
    uint32_t version = ((uint32_t)header[0]) |
                     ((uint32_t)header[1] << 8) |
                     ((uint32_t)header[2] << 16) |
                     ((uint32_t)header[3] << 24);
    
    if (version != BITCOIN_VERSION) {
        return 0;
    }
    
    // Check bits (bytes 72-76, Little-Endian)
    uint32_t bits = ((uint32_t)header[72]) |
                   ((uint32_t)header[73] << 8) |
                   ((uint32_t)header[74] << 16) |
                   ((uint32_t)header[75] << 24);
    
    return bits == BITCOIN_BITS;
}

#endif /* __HASHER_MAPS_H */