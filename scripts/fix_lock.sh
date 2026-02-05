#!/bin/sh
# Fix bitmain_asic module lock - run AFTER diagnose_lock.sh
# Run on Antminer S3 (root@192.168.12.151)

echo "=== Attempting to Free bitmain_asic Device ==="
echo ""

# Step 1: Kill any remaining cgminer processes
echo "=== Step 1: Killing CGMiner processes ==="
killall -9 cgminer 2>/dev/null
killall -9 cgminer.exe 2>/dev/null
sleep 2
ps | grep -i cgminer | grep -v grep
echo ""

# Step 2: Find and kill processes with device open
echo "=== Step 2: Killing processes with device open ==="
for pid in $(ls /proc | grep -E '^[0-9]+$'); do
    if [ -d "/proc/$pid/fd" ]; then
        for fd in /proc/$pid/fd/*; do
            if [ -L "$fd" ]; then
                link=$(readlink "$fd" 2>/dev/null)
                if echo "$link" | grep -q "bitmain"; then
                    cmd=$(cat /proc/$pid/cmdline 2>/dev/null | tr '\0' ' ')
                    echo "Killing PID $pid using $link: $cmd"
                    kill -9 $pid 2>/dev/null
                fi
            fi
        done
    fi
done
sleep 2
echo ""

# Step 3: Check if any kernel threads are using it
echo "=== Step 3: Checking kernel threads ==="
ps | grep -E '\[.*\]' | head -10
echo ""

# Step 4: Attempt module unload
echo "=== Step 4: Attempting module unload ==="
rmmod bitmain_asic 2>&1
if [ $? -eq 0 ]; then
    echo "SUCCESS: Module unloaded"
else
    echo "FAILED: Module still in use"
    echo "Trying forced unload..."
    rmmod -f bitmain_asic 2>&1
fi
echo ""

# Step 5: Verify
echo "=== Step 5: Verification ==="
lsmod | grep bitmain_asic && echo "Module still loaded" || echo "Module unloaded"
ls -la /dev/bitmain-asic 2>/dev/null || echo "Device node removed"
echo ""

# Step 6: Alternative - reload if needed
echo "=== Step 6: Reload module (optional) ==="
if ! lsmod | grep -q bitmain_asic; then
    echo "Module unloaded. To reload: modprobe bitmain_asic"
    echo "Or start hasher-server which should load it"
fi
echo ""

echo "=== Fix Attempt Complete ==="
echo "Check above output for SUCCESS/FAILED messages"
