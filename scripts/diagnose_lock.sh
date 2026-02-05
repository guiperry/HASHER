#!/bin/sh
# Diagnose and fix bitmain_asic module lock
# Run on Antminer S3 (root@192.168.12.151)

echo "=== Bitmain ASIC Device Lock Diagnostics ==="
echo "Date: $(date)"
echo ""

echo "=== 1. Module Status ==="
lsmod | grep bitmain_asic
echo ""

echo "=== 2. Device Status ==="
ls -la /dev/bitmain-asic 2>/dev/null || echo "Device not found"
echo ""

echo "=== 3. Processes Using Module ==="
# Check which processes have the module loaded
for pid in $(ls /proc | grep -E '^[0-9]+$'); do
    if [ -d "/proc/$pid" ]; then
        if grep -q bitmain_asic /proc/$pid/maps 2>/dev/null || grep -q bitmain /proc/$pid/maps 2>/dev/null; then
            echo "PID $pid: $(cat /proc/$pid/cmdline 2>/dev/null | tr '\0' ' ')"
        fi
    fi
done
echo ""

echo "=== 4. Open File Descriptors ==="
# Find processes with device open
for pid in $(ls /proc | grep -E '^[0-9]+$'); do
    if [ -d "/proc/$pid/fd" ]; then
        for fd in /proc/$pid/fd/*; do
            if [ -L "$fd" ]; then
                link=$(readlink "$fd" 2>/dev/null)
                if echo "$link" | grep -q "bitmain"; then
                    echo "PID $pid, FD $(basename $fd): $link"
                    echo "  CMD: $(cat /proc/$pid/cmdline 2>/dev/null | tr '\0' ' ')"
                fi
            fi
        done
    fi
done
echo ""

echo "=== 5. CGMiner Processes ==="
ps | grep -i cgminer | grep -v grep
echo ""

echo "=== 6. Kernel Module Holders ==="
cat /proc/modules | grep bitmain_asic
echo ""

echo "=== 7. Dmesg Related Messages ==="
dmesg | grep -i bitmain | tail -20
echo ""

echo "=== Diagnosis Complete ==="
