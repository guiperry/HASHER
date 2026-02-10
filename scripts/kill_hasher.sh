#!/bin/bash

# kill_hasher.sh - Safely terminate all hasher processes
# This script performs graceful shutdown with fallback to force kill
# It also stops the hasher-server on the ASIC device and removes the binary

set -uo pipefail

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

# Load environment variables from .env file
if [ -f .env ]; then
  set -a
  source .env
  set +a
else
  echo -e "${RED}ERROR: .env file not found${NC}"
  echo "Please create it with DEVICE_IP and DEVICE_PASSWORD variables"
  exit 1
fi

# Verify required variables are set
if [ -z "$DEVICE_IP" ] || [ -z "$DEVICE_PASSWORD" ]; then
  echo -e "${RED}ERROR: DEVICE_IP and DEVICE_PASSWORD must be set in .env file${NC}"
  exit 1
fi

# ASIC Device Configuration
ANTMINER_IP="$DEVICE_IP"
ANTMINER_USER="${DEVICE_USER:-root}"
ANTMINER_PASSWORD="$DEVICE_PASSWORD"
SERVER_BINARY_PATH="/tmp/hasher-server"

# SSH options for legacy algorithms
SSH_OPTS="-o KexAlgorithms=+diffie-hellman-group14-sha1 -o HostKeyAlgorithms=+ssh-rsa -o StrictHostKeyChecking=no -o ConnectTimeout=5"

# Process patterns to match
PATTERNS=(
    "hasher"
    "asic-test"
    "device-probe"
    "device-monitor"
    "asic-monitor"
    "dataminer"
    "data-encoder"
    "data-trainer"
)

# Timeout for graceful kill (seconds)
GRACEFUL_TIMEOUT=5

# Function to check if sshpass is available
check_sshpass() {
    if ! command -v sshpass &> /dev/null; then
        echo -e "${RED}Error: sshpass is not installed${NC}"
        echo "Install with: sudo apt-get install sshpass"
        return 1
    fi
    return 0
}

# Function to stop hasher-server on ASIC device
stop_asic_server() {
    echo -e "\n${YELLOW}Stopping hasher-server on ASIC device (${ANTMINER_IP})...${NC}"
    
    if ! check_sshpass; then
        return 1
    fi
    
    # Check if we can connect to the device
    if ! sshpass -p "$ANTMINER_PASSWORD" ssh $SSH_OPTS "$ANTMINER_USER@$ANTMINER_IP" "echo 'Connected'" &>/dev/null; then
        echo -e "${YELLOW}Warning: Cannot connect to ASIC device (may be offline)${NC}"
        return 1
    fi
    
    # Find and kill hasher-server process on device
    echo "  Searching for hasher-server process on device..."
    local device_pids
    device_pids=$(sshpass -p "$ANTMINER_PASSWORD" ssh $SSH_OPTS "$ANTMINER_USER@$ANTMINER_IP" "pgrep -f 'hasher-server'" 2>/dev/null || true)
    
    if [ -n "$device_pids" ]; then
        echo "  Found hasher-server PID(s): $device_pids"
        echo "  Sending SIGTERM to device process(es)..."
        sshpass -p "$ANTMINER_PASSWORD" ssh $SSH_OPTS "$ANTMINER_USER@$ANTMINER_IP" "kill -TERM $device_pids" 2>/dev/null || true
        sleep 2
        
        # Check if still running and force kill if needed
        local still_running
        still_running=$(sshpass -p "$ANTMINER_PASSWORD" ssh $SSH_OPTS "$ANTMINER_USER@$ANTMINER_IP" "pgrep -f 'hasher-server'" 2>/dev/null || true)
        if [ -n "$still_running" ]; then
            echo "  Force killing with SIGKILL..."
            sshpass -p "$ANTMINER_PASSWORD" ssh $SSH_OPTS "$ANTMINER_USER@$ANTMINER_IP" "kill -KILL $still_running" 2>/dev/null || true
            sleep 1
        fi
        
        # Verify process stopped
        local final_check
        final_check=$(sshpass -p "$ANTMINER_PASSWORD" ssh $SSH_OPTS "$ANTMINER_USER@$ANTMINER_IP" "pgrep -f 'hasher-server'" 2>/dev/null || true)
        if [ -z "$final_check" ]; then
            echo -e "${GREEN}  ✓ hasher-server stopped on device${NC}"
        else
            echo -e "${RED}  ✗ Failed to stop hasher-server on device${NC}"
        fi
    else
        echo "  No hasher-server process found on device"
    fi
    
    # Remove binary from device
    echo "  Removing hasher-server binary from device..."
    if sshpass -p "$ANTMINER_PASSWORD" ssh $SSH_OPTS "$ANTMINER_USER@$ANTMINER_IP" "rm -f $SERVER_BINARY_PATH" 2>/dev/null; then
        echo -e "${GREEN}  ✓ Binary removed from device${NC}"
    else
        echo -e "${YELLOW}  Binary may not exist or could not be removed${NC}"
    fi
    
    return 0
}

# Function to stop local processes
stop_local_processes() {
    echo -e "${YELLOW}Searching for local hasher processes...${NC}"
    
    # Collect all matching PIDs
    local PIDS=()
    for pattern in "${PATTERNS[@]}"; do
        while IFS= read -r pid; do
            if [ -n "$pid" ]; then
                PIDS+=("$pid")
            fi
        done < <(pgrep -f "$pattern" 2>/dev/null || true)
    done
    
    # Remove duplicates
    if [ ${#PIDS[@]} -gt 0 ]; then
        PIDS=($(printf "%s\n" "${PIDS[@]}" | sort -u))
    fi
    
    if [ ${#PIDS[@]} -eq 0 ]; then
        echo -e "${GREEN}No local hasher processes found.${NC}"
        return 0
    fi
    
    echo -e "${YELLOW}Found ${#PIDS[@]} local process(es) to terminate:${NC}"
    for pid in "${PIDS[@]}"; do
        if ps -p "$pid" > /dev/null 2>&1; then
            local cmd
            cmd=$(ps -p "$pid" -o comm= 2>/dev/null || echo "unknown")
            echo "  PID $pid: $cmd"
        fi
    done
    
    # Attempt graceful termination first
    echo -e "\n${YELLOW}Attempting graceful shutdown (SIGTERM)...${NC}"
    for pid in "${PIDS[@]}"; do
        if ps -p "$pid" > /dev/null 2>&1; then
            echo "  Sending SIGTERM to PID $pid..."
            kill -TERM "$pid" 2>/dev/null || true
        fi
    done
    
    # Wait for processes to exit
    sleep 1
    local still_running=()
    for pid in "${PIDS[@]}"; do
        if ps -p "$pid" > /dev/null 2>&1; then
            still_running+=("$pid")
        fi
    done
    
    # Force kill if any still running after timeout
    if [ ${#still_running[@]} -gt 0 ]; then
        echo -e "\n${YELLOW}Waiting up to ${GRACEFUL_TIMEOUT}s for graceful shutdown...${NC}"
        
        local waited=0
        while [ $waited -lt $GRACEFUL_TIMEOUT ] && [ ${#still_running[@]} -gt 0 ]; do
            sleep 1
            ((waited++))
            
            local remaining=()
            for pid in "${still_running[@]}"; do
                if ps -p "$pid" > /dev/null 2>&1; then
                    remaining+=("$pid")
                fi
            done
            still_running=("${remaining[@]}")
        done
        
        if [ ${#still_running[@]} -gt 0 ]; then
            echo -e "\n${RED}Force killing remaining processes (SIGKILL)...${NC}"
            for pid in "${still_running[@]}"; do
                echo "  Sending SIGKILL to PID $pid..."
                kill -KILL "$pid" 2>/dev/null || true
            done
        fi
    fi
    
    # Final verification
    echo -e "\n${YELLOW}Verifying local termination...${NC}"
    local remaining_count=0
    for pattern in "${PATTERNS[@]}"; do
        local count
        count=$(pgrep -f "$pattern" 2>/dev/null | wc -l)
        remaining_count=$((remaining_count + count))
    done
    
    if [ $remaining_count -eq 0 ]; then
        echo -e "${GREEN}All local processes terminated successfully.${NC}"
        return 0
    else
        echo -e "${RED}Warning: $remaining_count local process(es) may still be running.${NC}"
        return 1
    fi
}

# Main execution
main() {
    echo -e "${YELLOW}========================================${NC}"
    echo -e "${YELLOW}  Hasher Shutdown Script${NC}"
    echo -e "${YELLOW}========================================${NC}"
    
    local exit_code=0
    
    # Stop ASIC server first
    stop_asic_server || exit_code=1
    
    # Stop local processes
    stop_local_processes || exit_code=1
    
    echo -e "\n${YELLOW}========================================${NC}"
    if [ $exit_code -eq 0 ]; then
        echo -e "${GREEN}Shutdown completed successfully${NC}"
    else
        echo -e "${RED}Shutdown completed with warnings${NC}"
    fi
    echo -e "${YELLOW}========================================${NC}"
    
    exit $exit_code
}

# Run main function
main
