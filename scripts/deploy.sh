#!/bin/bash
# Hasher Deploy Script
# Builds and deploys the diagnostic tool to the Antminer

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

# Configuration
ANTMINER_IP="$DEVICE_IP"
ANTMINER_USER="root"
ANTMINER_PASSWORD="$DEVICE_PASSWORD"
PROJECT_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"

# SSH options for legacy Antminer
SSH_OPTS="-o KexAlgorithms=+diffie-hellman-group14-sha1 -o HostKeyAlgorithms=+ssh-rsa -o StrictHostKeyChecking=no"

# Check if sshpass is available
if ! command -v sshpass &> /dev/null; then
    echo "⚠️  Warning: sshpass not found. You'll need to enter password manually."
    echo "   To install: sudo apt-get install sshpass"
    SSHPASS_CMD=""
else
    SSHPASS_CMD="sshpass -p '$ANTMINER_PASSWORD'"
fi

echo "🛡️  Hasher Deployment Tool"
echo "===================================="
echo ""
echo "Project: $PROJECT_ROOT"
echo "Target:  $ANTMINER_USER@$ANTMINER_IP"
echo ""

# Build
echo "🔨 Building for MIPS architecture..."
cd "$PROJECT_ROOT"
mkdir -p bin
GOOS=linux GOARCH=mips GOMIPS=softfloat go build -o bin/asic-test-mips cmd/asic-test/main.go

if [ $? -ne 0 ]; then
    echo "❌ Build failed!"
    exit 1
fi

echo "✅ Build successful"
echo "📦 Binary size: $(du -h bin/asic-test-mips | cut -f1)"
echo ""

# Deploy
echo "🚀 Deploying to Antminer..."
eval "$SSHPASS_CMD scp $SSH_OPTS bin/asic-test-mips ${ANTMINER_USER}@${ANTMINER_IP}:/tmp/asic-test"

if [ $? -ne 0 ]; then
    echo "❌ Deployment failed!"
    echo ""
    echo "Troubleshooting:"
    echo "  1. Check if Antminer is reachable: ping $ANTMINER_IP"
    echo "  2. Verify SSH access: ssh $SSH_OPTS ${ANTMINER_USER}@${ANTMINER_IP}"
    echo "  3. Check if SSH config is set up (see ~/.ssh/config)"
    exit 1
fi

echo "✅ Deployment successful"
echo ""

# Make executable
echo "🔧 Making executable on remote..."
eval "$SSHPASS_CMD ssh $SSH_OPTS ${ANTMINER_USER}@${ANTMINER_IP} 'chmod +x /tmp/asic-test'"

echo "✅ Ready to run"
echo ""

# Ask to run
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo "Run diagnostics now? (y/n)"
read -r response

if [[ "$response" =~ ^[Yy]$ ]]; then
    echo ""
    echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
    echo "Running diagnostics on Antminer..."
    echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
    echo ""

    # Run and save output
    eval "$SSHPASS_CMD ssh $SSH_OPTS ${ANTMINER_USER}@${ANTMINER_IP} '/tmp/asic-test 2>&1 | tee /tmp/sentinel-diagnostics.txt'"

    echo ""
    echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
    echo "📄 Downloading diagnostics report..."

    mkdir -p "$PROJECT_ROOT/logs"
    TIMESTAMP=$(date +%Y%m%d_%H%M%S)
    OUTPUT_FILE="$PROJECT_ROOT/logs/diagnostics_${TIMESTAMP}.txt"

    eval "$SSHPASS_CMD scp $SSH_OPTS ${ANTMINER_USER}@${ANTMINER_IP}:/tmp/sentinel-diagnostics.txt '$OUTPUT_FILE'"

    echo "✅ Saved to: $OUTPUT_FILE"
    echo ""
    echo "View with: cat $OUTPUT_FILE"
else
    echo ""
    echo "To run manually:"
    if [ -n "$SSHPASS_CMD" ]; then
        echo "  $SSHPASS_CMD ssh $SSH_OPTS ${ANTMINER_USER}@${ANTMINER_IP} '/tmp/asic-test'"
    else
        echo "  ssh $SSH_OPTS ${ANTMINER_USER}@${ANTMINER_IP} '/tmp/asic-test'"
    fi
fi

echo ""
echo "✅ Done!"
