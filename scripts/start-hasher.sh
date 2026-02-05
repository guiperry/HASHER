#!/bin/bash
set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_DIR="$(dirname "$SCRIPT_DIR")"

# Load environment variables from .env file
if [ -f "$PROJECT_DIR/.env" ]; then
    set -a
    source "$PROJECT_DIR/.env"
    set +a
else
    echo "❌ ERROR: .env file not found at $PROJECT_DIR/.env"
    echo "Please create it with DEVICE_IP and DEVICE_PASSWORD variables"
    exit 1
fi

# Verify required variables are set
if [ -z "$DEVICE_IP" ] || [ -z "$DEVICE_PASSWORD" ]; then
    echo "❌ ERROR: DEVICE_IP and DEVICE_PASSWORD must be set in .env file"
    exit 1
fi

get_local_ip() {
    local ip=$(ip route get 1 2>/dev/null | awk '{print $7}' | head -1)
    if [ -z "$ip" ]; then
        ip=$(hostname -I 2>/dev/null | awk '{print $1}')
    fi
    if [ -z "$ip" ]; then
        ip="192.168.1.100"
    fi
    echo "$ip"
}

LOCAL_IP=$(get_local_ip)
echo "Local IP: $LOCAL_IP"

echo "Starting KNIRVHASHER Stratum Proxy..."
cd "$PROJECT_DIR"
./bin/stratum-proxy &
PROXY_PID=$!
echo "Stratum proxy started (PID: $PROXY_PID)"

echo ""
echo "Configuring antminer cgminer to use $LOCAL_IP:3333 as pool..."
sshpass -p "$DEVICE_PASSWORD" ssh -o KexAlgorithms=+diffie-hellman-group14-sha1 \
    -o HostKeyAlgorithms=+ssh-rsa -o StrictHostKeyChecking=no \
    root@$DEVICE_IP "uci set cgminer.default.pool1url='$LOCAL_IP:3333' && uci set cgminer.default.pool1user='knirv.hasher' && uci set cgminer.default.pool1pw='x' && uci commit cgminer && /etc/init.d/cgminer restart"

echo ""
echo "KNIRVHASHER started!"
echo "Shares will be logged to: $PROJECT_DIR/logs/stratum_shares.log"
echo "To view shares: tail -f $PROJECT_DIR/logs/stratum_shares.log"
echo ""
echo "To stop: kill $PROXY_PID && ssh root@$DEVICE_IP '/etc/init.d/cgminer stop'"
