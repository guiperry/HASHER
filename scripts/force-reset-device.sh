#!/bin/bash
# Force reset ASIC device when stuck in usbfs claim
echo "=== FORCE RESET OF ASIC DEVICE ==="

# Kill any remaining hasher-server processes
echo "Killing hasher-server processes..."
killall -9 hasher-server 2>/dev/null || true
sleep 3

# Try to unload modules (will likely fail due to usbfs claim)
echo "Attempting to unload modules..."
rmmod bitmain_asic 2>/dev/null || echo "Module busy (expected)"
rmmod usb_bitmain 2>/dev/null || echo "USB module busy (expected)"

# Force USB reset by writing to sysfs
echo "Forcing USB reset via sysfs..."
if [ -d /sys/bus/usb/devices/1-1.1 ]; then
    echo '0' > /sys/bus/usb/devices/1-1.1/bConfiguration 2>/dev/null || true
    sleep 1
fi

# Alternative: Use usbreset if available
which usbreset >/dev/null 2>&1
if [ $? -eq 0 ]; then
    echo "Using usbreset to force device reset..."
    usbreset /dev/bus/usb/001/004 2>/dev/null || echo "usbreset failed"
    sleep 2
fi

# Final attempt: Remove device from usbfs
echo "Attempting to break usbfs claim..."
if [ -f /sys/bus/usb/drivers/usb ]; then
    ls /sys/bus/usb/devices/1-1.1/*/driver 2>/dev/null | while read driver; do
        if [ -n "$driver" ]; then
            echo "$driver" > /sys/bus/usb/drivers/usb/unbind 2>/dev/null || true
            sleep 1
        fi
    done
fi

# Now try to reload module
echo "Reloading kernel module..."
sleep 2
modprobe -r bitmain_asic 2>/dev/null || true
sleep 2
modprobe bitmain_asic 2>/dev/null || echo "Module reload failed"
sleep 3

# Create device nodes
echo "Creating device nodes..."
rm -f /dev/bitmain-asic /dev/bitmain0 2>/dev/null || true
mknod /dev/bitmain-asic c 10 60 2>/dev/null || true
mknod /dev/bitmain0 c 180 0 2>/dev/null || true
chmod 666 /dev/bitmain-asic /dev/bitmain0 2>/dev/null || true

echo "=== FORCE RESET COMPLETE ==="
echo "Final device status:"
ls -la /dev/bitmain* 2>/dev/null || echo "No devices found"