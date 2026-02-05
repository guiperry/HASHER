#!/bin/bash
# Enhanced kernel module management for hasher-server deployment
echo "Stopping CGMiner processes..."
if pgrep cgminer > /dev/null 2>&1; then
    /etc/init.d/cgminer stop 2>/dev/null || true
    sleep 3
    killall -9 cgminer bmminer 2>/dev/null || true
    sleep 1
    if pgrep cgminer > /dev/null 2>&1; then
        echo "WARNING: CGMiner still running"
    else
        echo "SUCCESS: CGMiner stopped"
    fi
else
    echo "CGMiner was not running"
fi

echo "Releasing ASIC device..."
# Try multiple approaches to release device
echo "  - Attempting to unload kernel modules..."
rmmod bitmain_asic 2>/dev/null || echo "  - bitmain_asic unload failed (may be in use)"
rmmod usb_bitmain 2>/dev/null || echo "  - usb_bitmain unload failed"
sleep 2

# Try killall for any lingering processes
killall -9 cgminer bmminer hasher-server 2>/dev/null || true
sleep 1

# Reload kernel module with proper device setup
echo "  - Reloading kernel modules..."
modprobe bitmain_asic 2>/dev/null || echo "  - bitmain_asic reload failed"
sleep 3

# Ensure device nodes are created with correct permissions
echo "  - Creating device nodes..."
rm -f /dev/bitmain-asic /dev/bitmain0 2>/dev/null || true
# Check if module created device automatically
if [ -c /dev/bitmain-asic ]; then
    echo "  - Device /dev/bitmain-asic created automatically"
else
    echo "  - Creating device /dev/bitmain-asic (major 10, minor 60)..."
    mknod /dev/bitmain-asic c 10 60
fi

if [ -c /dev/bitmain0 ]; then
    echo "  - Device /dev/bitmain0 created automatically"
else
    echo "  - Creating device /dev/bitmain0 (major 180, minor 0)..."
    mknod /dev/bitmain0 c 180 0
fi

# Set proper permissions
chmod 666 /dev/bitmain-asic /dev/bitmain0 2>/dev/null || true

# Final check
echo "  - Verifying module status:"
if lsmod | grep -q bitmain_asic; then
    echo "    SUCCESS: bitmain_asic module loaded"
else
    echo "    ERROR: bitmain_asic module not loaded"
fi

if [ -c /dev/bitmain-asic ]; then
    echo "    SUCCESS: /dev/bitmain-asic device available"
    ls -la /dev/bitmain-asic
else
    echo "    ERROR: /dev/bitmain-asic device missing"
fi