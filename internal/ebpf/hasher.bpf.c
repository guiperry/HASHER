// SPDX-License-Identifier: GPL-2.0 OR BSD-3-Clause
#include <linux/bpf.h>
#include <linux/types.h>
#include <bpf/bpf_helpers.h>
#include <bpf/bpf_tracing.h>
#include <bpf/bpf_core_read.h>

#define MAX_ENTRIES 10240
#define TASK_COMM_LEN 16

// Event types
#define EVENT_COMPUTE_START 1
#define EVENT_COMPUTE_END 2
#define EVENT_BATCH_START 3
#define EVENT_BATCH_END 4
#define EVENT_ERROR 5

// Hash event structure
struct hash_event {
    __u64 timestamp;
    __u32 pid;
    __u32 tid;
    __u8 event_type;
    __u32 data_size;
    __u64 latency_ns;
    __u32 batch_size;
    char comm[TASK_COMM_LEN];
};

// Statistics structure
struct hash_stats {
    __u64 total_requests;
    __u64 total_bytes;
    __u64 total_latency_ns;
    __u64 peak_latency_ns;
    __u64 error_count;
};

// Maps
struct {
    __uint(type, BPF_MAP_TYPE_HASH);
    __uint(max_entries, MAX_ENTRIES);
    __type(key, __u64);  // tid
    __type(value, __u64); // start timestamp
} compute_start_times SEC(".maps");

struct {
    __uint(type, BPF_MAP_TYPE_RINGBUF);
    __uint(max_entries, 256 * 1024);
} events SEC(".maps");

struct {
    __uint(type, BPF_MAP_TYPE_ARRAY);
    __uint(max_entries, 1);
    __type(key, __u32);
    __type(value, struct hash_stats);
} stats SEC(".maps");

// Helper to get current stats
static inline struct hash_stats* get_stats(void) {
    __u32 key = 0;
    return bpf_map_lookup_elem(&stats, &key);
}

// Trace point when compute operation starts
SEC("uprobe/hasher_compute_start")
int trace_compute_start(struct pt_regs *ctx) {
    __u64 tid = bpf_get_current_pid_tgid();
    __u64 ts = bpf_ktime_get_ns();
    
    bpf_map_update_elem(&compute_start_times, &tid, &ts, BPF_ANY);
    
    // Emit start event
    struct hash_event *e;
    e = bpf_ringbuf_reserve(&events, sizeof(*e), 0);
    if (!e)
        return 0;
    
    e->timestamp = ts;
    e->pid = tid >> 32;
    e->tid = tid;
    e->event_type = EVENT_COMPUTE_START;
    e->data_size = 0;
    e->latency_ns = 0;
    e->batch_size = 1;
    bpf_get_current_comm(&e->comm, sizeof(e->comm));
    
    bpf_ringbuf_submit(e, 0);
    return 0;
}

// Trace point when compute operation ends
SEC("uprobe/hasher_compute_end")
int trace_compute_end(struct pt_regs *ctx) {
    __u64 tid = bpf_get_current_pid_tgid();
    __u64 ts = bpf_ktime_get_ns();
    
    __u64 *start_ts = bpf_map_lookup_elem(&compute_start_times, &tid);
    if (!start_ts)
        return 0;
    
    __u64 latency = ts - *start_ts;
    
    // Update stats
    struct hash_stats *s = get_stats();
    if (s) {
        __sync_fetch_and_add(&s->total_requests, 1);
        __sync_fetch_and_add(&s->total_latency_ns, latency);
        
        // Update peak latency atomically
        __u64 current_peak = s->peak_latency_ns;
        if (latency > current_peak) {
            __sync_val_compare_and_swap(&s->peak_latency_ns, current_peak, latency);
        }
    }
    
    // Emit end event
    struct hash_event *e;
    e = bpf_ringbuf_reserve(&events, sizeof(*e), 0);
    if (!e) {
        bpf_map_delete_elem(&compute_start_times, &tid);
        return 0;
    }
    
    e->timestamp = ts;
    e->pid = tid >> 32;
    e->tid = tid;
    e->event_type = EVENT_COMPUTE_END;
    e->data_size = 0;
    e->latency_ns = latency;
    e->batch_size = 1;
    bpf_get_current_comm(&e->comm, sizeof(e->comm));
    
    bpf_ringbuf_submit(e, 0);
    bpf_map_delete_elem(&compute_start_times, &tid);
    
    return 0;
}

// Trace batch operations
SEC("uprobe/hasher_batch_start")
int trace_batch_start(struct pt_regs *ctx) {
    __u64 tid = bpf_get_current_pid_tgid();
    __u64 ts = bpf_ktime_get_ns();
    
    bpf_map_update_elem(&compute_start_times, &tid, &ts, BPF_ANY);
    
    struct hash_event *e;
    e = bpf_ringbuf_reserve(&events, sizeof(*e), 0);
    if (!e)
        return 0;
    
    e->timestamp = ts;
    e->pid = tid >> 32;
    e->tid = tid;
    e->event_type = EVENT_BATCH_START;
    e->data_size = 0;
    e->latency_ns = 0;
    e->batch_size = 0; // Will be filled by userspace
    bpf_get_current_comm(&e->comm, sizeof(e->comm));
    
    bpf_ringbuf_submit(e, 0);
    return 0;
}

SEC("uprobe/hasher_batch_end")
int trace_batch_end(struct pt_regs *ctx) {
    __u64 tid = bpf_get_current_pid_tgid();
    __u64 ts = bpf_ktime_get_ns();
    
    __u64 *start_ts = bpf_map_lookup_elem(&compute_start_times, &tid);
    if (!start_ts)
        return 0;
    
    __u64 latency = ts - *start_ts;
    
    struct hash_event *e;
    e = bpf_ringbuf_reserve(&events, sizeof(*e), 0);
    if (!e) {
        bpf_map_delete_elem(&compute_start_times, &tid);
        return 0;
    }
    
    e->timestamp = ts;
    e->pid = tid >> 32;
    e->tid = tid;
    e->event_type = EVENT_BATCH_END;
    e->data_size = 0;
    e->latency_ns = latency;
    e->batch_size = 0;
    bpf_get_current_comm(&e->comm, sizeof(e->comm));
    
    bpf_ringbuf_submit(e, 0);
    bpf_map_delete_elem(&compute_start_times, &tid);
    
    return 0;
}

char LICENSE[] SEC("license") = "Dual BSD/GPL";
