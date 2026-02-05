#!/bin/bash
# Deploy and run ASIC monitor with USB direct access

set -e

# Load environment variables from .env file
if [ -f .env ]; then
  set -a
  source .env
  set +a
else
  echo "❌ ERROR: .env file not found"
  echo "Please create it with DEVICE_IP and DEVICE_PASSWORD variables"
  exit 1
fi

# Verify required variables are set
if [ -z "$DEVICE_IP" ] || [ -z "$DEVICE_PASSWORD" ]; then
  echo "❌ ERROR: DEVICE_IP and DEVICE_PASSWORD must be set in .env file"
  exit 1
fi

ANTMINER_IP="$DEVICE_IP"
ANTMINER_USER="root"
ANTMINER_PASSWORD="$DEVICE_PASSWORD"
PROJECT_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
SDK_ROOT="$PROJECT_ROOT/toolchain/openwrt-sdk-19.07.10-ar71xx-generic_gcc-7.5.0_musl.Linux-x86_64"

SSH_OPTS="-o KexAlgorithms=+diffie-hellman-group14-sha1 -o HostKeyAlgorithms=+ssh-rsa -o StrictHostKeyChecking=no"

echo "🛡️  Hasher USB Monitor Deployment"
echo "=========================================="
echo ""

# Check if binary exists
if [ ! -f "$PROJECT_ROOT/bin/monitor-mips" ]; then
    echo "❌ Binary not found. Building..."
    cd "$PROJECT_ROOT"
    ./scripts/build-mips-cgo.sh bin/monitor-mips cmd/monitor/main.go
    echo ""
fi

# Clean up old files on Antminer
echo "🧹 Cleaning up old files..."
sshpass -p "$ANTMINER_PASSWORD" ssh $SSH_OPTS ${ANTMINER_USER}@${ANTMINER_IP} 'killall -9 asic-monitor 2>/dev/null; rm -f /tmp/monitor /tmp/libusb-1.0.so.0; echo "Ready"' || true
echo ""

# Deploy libusb library
echo "📦 Deploying libusb-1.0..."
sshpass -p "$ANTMINER_PASSWORD" scp $SSH_OPTS \
    "$SDK_ROOT/staging_dir/target-mips_24kc_musl/usr/lib/libusb-1.0.so.0.1.0" \
    ${ANTMINER_USER}@${ANTMINER_IP}:/tmp/libusb-1.0.so.0
echo "✅ libusb deployed"
echo ""

# Deploy binary
echo "🚀 Deploying asic-monitor binary..."
sshpass -p "$ANTMINER_PASSWORD" scp $SSH_OPTS \
    "$PROJECT_ROOT/bin/monitor-mips" \
    ${ANTMINER_USER}@${ANTMINER_IP}:/tmp/monitor
echo "✅ Binary deployed"
echo ""

# Set permissions
echo "🔧 Setting permissions..."
sshpass -p "$ANTMINER_PASSWORD" ssh $SSH_OPTS ${ANTMINER_USER}@${ANTMINER_IP} 'chmod +x /tmp/monitor'
echo "✅ Permissions set"
echo ""

# Create log directory
mkdir -p "$PROJECT_ROOT/logs"
TIMESTAMP=$(date +%Y%m%d_%H%M%S)
OUTPUT_FILE="$PROJECT_ROOT/logs/monitor-usb_${TIMESTAMP}.log"

echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo "Running ASIC monitor on Antminer (USB mode)..."
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo ""

# Run monitor in dump-status mode with LD_LIBRARY_PATH pointing to /tmp for our libusb
sshpass -p "$ANTMINER_PASSWORD" ssh $SSH_OPTS ${ANTMINER_USER}@${ANTMINER_IP} \
    'LD_LIBRARY_PATH=/tmp:/usr/lib /tmp/monitor --dump-status --dump-interval 2' 2>&1 | tee "$OUTPUT_FILE"

echo ""
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo "✅ Monitor run complete!"
echo ""
echo "📄 Output saved to: $OUTPUT_FILE"
echo ""

# Quick analysis
echo "🔍 Analyzing results..."
echo ""

if grep -q "USB device opened" "$OUTPUT_FILE"; then
    echo "✅ USB device access: SUCCESS"

    if grep -q "Interface claimed" "$OUTPUT_FILE"; then
        echo "✅ USB interface: Claimed successfully"
    fi

    if grep -q "Sent.*bytes" "$OUTPUT_FILE"; then
        echo "✅ USB communication: Data sent"
    fi

    # Look for parsed status or JSON dump entries
    if grep -q "Parsed RxStatus" "$OUTPUT_FILE" || grep -q '"crc_valid":true' "$OUTPUT_FILE"; then
        echo "✅ USB communication: Data received and parsed (RxStatus)"
        echo ""
        echo "🎉 SUCCESS! ASIC responded and status was parsed!"
    elif grep -q "Received.*bytes" "$OUTPUT_FILE"; then
        echo "✅ USB communication: Data received (raw)"
    fi
else
    echo "❌ USB device access: FAILED"

    if grep -q "Could not open USB device" "$OUTPUT_FILE"; then
        echo ""
        echo "💡 Device not found. Check:"
        echo "   ssh antminer 'lsusb | grep 4254'"
        echo "   ssh antminer 'ls -la /dev/bus/usb/'"
    fi
fi

echo ""
echo "📖 View full output: cat $OUTPUT_FILE"
