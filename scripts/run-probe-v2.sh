#!/bin/bash
# Run enhanced device probe v2 and capture output

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

SSH_OPTS="-o KexAlgorithms=+diffie-hellman-group14-sha1 -o HostKeyAlgorithms=+ssh-rsa -o StrictHostKeyChecking=no"

echo "🔬 Hasher Enhanced Device Probe v2 Runner"
echo "================================================"
echo ""

# Clean up old binary on Antminer
echo "🧹 Cleaning up old binary..."
sshpass -p "$ANTMINER_PASSWORD" ssh $SSH_OPTS ${ANTMINER_USER}@${ANTMINER_IP} 'killall -9 device-probe-v2 2>/dev/null; rm -f /tmp/device-probe-v2; echo "Ready"' || true
echo ""

# Build and deploy
echo "🔨 Building enhanced probe tool v2..."
cd "$PROJECT_ROOT"
make build-probe-v2

echo ""
echo "🚀 Deploying to Antminer..."
make deploy-probe-v2

echo ""
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo "Running enhanced device probe v2 on Antminer..."
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo ""

mkdir -p "$PROJECT_ROOT/logs"
TIMESTAMP=$(date +%Y%m%d_%H%M%S)
OUTPUT_FILE="$PROJECT_ROOT/logs/probe_v2_${TIMESTAMP}.txt"

# Run probe and capture output
sshpass -p "$ANTMINER_PASSWORD" ssh $SSH_OPTS ${ANTMINER_USER}@${ANTMINER_IP} '/tmp/device-probe-v2' 2>&1 | tee "$OUTPUT_FILE"

echo ""
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo "✅ Probe complete!"
echo ""
echo "📄 Output saved to: $OUTPUT_FILE"
echo ""
echo "🔍 Analyzing results..."
echo ""

# Quick analysis
if grep -q "Device opened successfully" "$OUTPUT_FILE"; then
    echo "✅ Device access: SUCCESS"

    if grep -q "Read.*bytes:" "$OUTPUT_FILE"; then
        echo "✅ Read operations: Data received"
        echo ""
        echo "📊 Read data samples:"
        grep "Read.*bytes:" "$OUTPUT_FILE" | head -3
    else
        echo "⚠️  Read operations: No data or timeout"
    fi

    if grep -q "Wrote.*bytes" "$OUTPUT_FILE"; then
        echo "✅ Write operations: Success"
    else
        echo "⚠️  Write operations: Failed or not tested"
    fi

    if grep -q "IOCTL.*Success" "$OUTPUT_FILE"; then
        echo "✅ IOCTL discovery: Found working commands!"
        echo ""
        echo "📊 Working IOCTLs:"
        grep "IOCTL.*Success" "$OUTPUT_FILE" || grep "Testing.*Success" "$OUTPUT_FILE"
    fi
else
    echo "❌ Device access: FAILED"
    echo ""

    if grep -q "No processes have the device open" "$OUTPUT_FILE"; then
        echo "ℹ️  No processes have the device open"
    fi

    if grep -q "Processes with device open:" "$OUTPUT_FILE"; then
        echo "⚠️  Found processes with device open:"
        grep -A 5 "Processes with device open:" "$OUTPUT_FILE"
    fi

    if grep -q "CGMiner still running" "$OUTPUT_FILE"; then
        echo "⚠️  CGMiner is still running - needs SIGKILL"
    fi

    echo ""
    echo "💡 Next steps:"
    echo "   - Check kernel modules: lsmod | grep -i bitmain"
    echo "   - Try module reload: rmmod bitmain_asic && modprobe bitmain_asic"
    echo "   - Check dmesg: dmesg | tail -50"
fi

echo ""
echo "View full output: cat $OUTPUT_FILE"
