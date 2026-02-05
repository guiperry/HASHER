#!/bin/bash
# Run device probe and capture output

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

echo "🔬 Hasher Device Probe Runner"
echo "===================================="
echo ""

# Clean up old binary on Antminer
echo "🧹 Cleaning up old binary..."
sshpass -p "$ANTMINER_PASSWORD" ssh $SSH_OPTS ${ANTMINER_USER}@${ANTMINER_IP} 'killall -9 device-probe 2>/dev/null; rm -f /tmp/device-probe; echo "Ready"' || true
echo ""

# Build and deploy
echo "🔨 Building probe tool..."
cd "$PROJECT_ROOT"
make build-probe

echo ""
echo "🚀 Deploying to Antminer..."
make deploy-probe

echo ""
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo "Running device probe on Antminer..."
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo ""

mkdir -p "$PROJECT_ROOT/logs"
TIMESTAMP=$(date +%Y%m%d_%H%M%S)
OUTPUT_FILE="$PROJECT_ROOT/logs/probe_${TIMESTAMP}.txt"

# Run probe and capture output
sshpass -p "$ANTMINER_PASSWORD" ssh $SSH_OPTS ${ANTMINER_USER}@${ANTMINER_IP} '/tmp/device-probe' 2>&1 | tee "$OUTPUT_FILE"

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
else
    echo "❌ Device access: FAILED"
fi

if grep -q "Read.*bytes:" "$OUTPUT_FILE"; then
    echo "✅ Read operations: Data received"
    grep "Read.*bytes:" "$OUTPUT_FILE" | head -3
else
    echo "⚠️  Read operations: No data or timeout"
fi

if grep -q "Wrote.*bytes" "$OUTPUT_FILE"; then
    echo "✅ Write operations: Success"
else
    echo "⚠️  Write operations: Failed or not tested"
fi

echo ""
echo "View full output: cat $OUTPUT_FILE"
