#!/bin/bash
# Deploy and run protocol capture on Antminer

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

CAPTURE_DURATION=${1:-30}  # Default 30 seconds

echo "🔍 Hasher Protocol Capture Runner"
echo "========================================"
echo ""
echo "Target: $ANTMINER_USER@$ANTMINER_IP"
echo "Capture Duration: ${CAPTURE_DURATION}s"
echo ""

# Clean up old binary on Antminer
echo "🧹 Cleaning up old binary..."
sshpass -p "$ANTMINER_PASSWORD" ssh $SSH_OPTS ${ANTMINER_USER}@${ANTMINER_IP} 'killall -9 protocol-discover 2>/dev/null; rm -f /tmp/protocol-discover; echo "Ready"' || true
echo ""

# Deploy capture script
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo "Phase 1: Deploying Capture Tool"
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo ""

echo "Uploading capture-protocol.sh..."
sshpass -p "$ANTMINER_PASSWORD" scp $SSH_OPTS \
    "$PROJECT_ROOT/scripts/capture-protocol.sh" \
    ${ANTMINER_USER}@${ANTMINER_IP}:/tmp/capture-protocol.sh

sshpass -p "$ANTMINER_PASSWORD" ssh $SSH_OPTS ${ANTMINER_USER}@${ANTMINER_IP} \
    'chmod +x /tmp/capture-protocol.sh'

echo "✅ Deployed"
echo ""

# Run capture
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo "Phase 2: Running Protocol Capture"
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo ""
echo "⏱️  This will take approximately ${CAPTURE_DURATION} seconds..."
echo ""

sshpass -p "$ANTMINER_PASSWORD" ssh $SSH_OPTS ${ANTMINER_USER}@${ANTMINER_IP} \
    "/tmp/capture-protocol.sh $CAPTURE_DURATION"

echo ""
echo "✅ Capture complete on Antminer"
echo ""

# Download captured data
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo "Phase 3: Downloading Captured Data"
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo ""

mkdir -p "$PROJECT_ROOT/logs"
TIMESTAMP=$(date +%Y%m%d_%H%M%S)
LOCAL_OUTPUT="$PROJECT_ROOT/logs/protocol-capture-${TIMESTAMP}"

echo "Downloading to: $LOCAL_OUTPUT"
echo ""

sshpass -p "$ANTMINER_PASSWORD" scp -r $SSH_OPTS \
    ${ANTMINER_USER}@${ANTMINER_IP}:/tmp/sentinel-capture \
    "$LOCAL_OUTPUT"

echo "✅ Downloaded"
echo ""

# Quick analysis
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo "Phase 4: Quick Analysis"
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo ""

if [ -f "$LOCAL_OUTPUT/summary.txt" ]; then
    cat "$LOCAL_OUTPUT/summary.txt"
else
    echo "⚠️  No summary file found"
fi

echo ""
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo "✅ All Done!"
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo ""
echo "📁 Captured data: $LOCAL_OUTPUT"
echo ""
echo "📄 Files to analyze:"
echo "   $LOCAL_OUTPUT/write-operations.log  - Commands sent to ASICs"
echo "   $LOCAL_OUTPUT/read-operations.log   - Responses from ASICs"
echo "   $LOCAL_OUTPUT/device-operations.log - All device I/O"
echo ""
echo "🔍 Next: Analyze write-operations.log to understand the protocol"
echo ""
