#!/bin/bash
# Deploy and run lock diagnostics/fix on Antminer S3

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

HOST="root@$DEVICE_IP"
PASSWORD="$DEVICE_PASSWORD"
SSH_OPTS="-o KexAlgorithms=+diffie-hellman-group14-sha1 -o HostKeyAlgorithms=+ssh-rsa -o StrictHostKeyChecking=no"

echo "=== Deploying lock fix scripts to Antminer ==="

# Create remote directory
sshpass -p "$PASSWORD" ssh $SSH_OPTS $HOST "mkdir -p /tmp/hasher-fix"

# Copy scripts
sshpass -p "$PASSWORD" scp $SSH_OPTS scripts/diagnose_lock.sh scripts/fix_lock.sh scripts/nuclear_unload.sh $HOST:/tmp/hasher-fix/

# Make executable
sshpass -p "$PASSWORD" ssh $SSH_OPTS $HOST "chmod +x /tmp/hasher-fix/*.sh"

echo ""
echo "=== Step 1: Running Diagnostics ==="
sshpass -p "$PASSWORD" ssh $SSH_OPTS $HOST "/tmp/hasher-fix/diagnose_lock.sh" | tee /tmp/diagnose_output.txt

echo ""
echo "=== Step 2: Attempting Fix ==="
sshpass -p "$PASSWORD" ssh $SSH_OPTS $HOST "/tmp/hasher-fix/fix_lock.sh" | tee /tmp/fix_output.txt

echo ""
echo "=== Checking Results ==="
sshpass -p "$PASSWORD" ssh $SSH_OPTS $HOST "lsmod | grep bitmain; ls -la /dev/bitmain-asic"

echo ""
echo "=== Done ==="
echo "If module is still loaded, you can try the nuclear option:"
echo "  ssh $SSH_OPTS $HOST '/tmp/hasher-fix/nuclear_unload.sh'"
echo "Or reboot:"
echo "  ssh $SSH_OPTS $HOST 'reboot -f'"
