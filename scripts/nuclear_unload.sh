#!/bin/sh
# Nuclear option: Force unload bitmain_asic module
# WARNING: Only use if other methods fail - may cause system instability

echo "=== NUCLEAR OPTION: Force Module Unload ==="
echo "WARNING: This may cause kernel instability or crash"
echo "Press Ctrl+C within 3 seconds to abort..."
sleep 3

echo "Attempting forced module removal..."

# Method 1: Using -f flag
echo "Method 1: rmmod -f"
rmmod -f bitmain_asic 2>&1
if [ $? -eq 0 ]; then
    echo "SUCCESS"
    exit 0
fi

# Method 2: Using /proc interface (if available)
echo "Method 2: /proc interface"
echo "bitmain_asic" > /proc/sysrq-trigger 2>/dev/null

# Method 3: Kill all processes and retry
echo "Method 3: Aggressive process termination"
for pid in $(ps | grep -v grep | grep -v PID | awk '{print $1}'); do
    if [ "$pid" != "1" ] && [ "$pid" != "$$" ]; then
        kill -9 $pid 2>/dev/null
    fi
done
sleep 2

rmmod bitmain_asic 2>&1
if [ $? -eq 0 ]; then
    echo "SUCCESS after process kill"
    exit 0
fi

echo ""
echo "=== All methods failed ==="
echo "Module reference count may be held by kernel thread"
echo "Reboot may be required: reboot -f"
