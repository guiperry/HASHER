#!/bin/bash

# Script to kill all data-miner processes

set -e

echo "🔍 Checking for running data-miner processes..."

# Find data-miner PIDs
PIDS=$(pgrep -f "data-miner" || true)

if [ -z "$PIDS" ]; then
    echo "✅ No data-miner processes found running"
    exit 0
fi

echo "⚠️  Found data-miner processes:"
echo "$PIDS"
echo ""

# Try graceful shutdown first with SIGTERM
echo "🛑 Sending SIGTERM for graceful shutdown..."
echo "$PIDS" | xargs -r kill -TERM 2>/dev/null || true

# Wait a bit for graceful shutdown
echo "⏳ Waiting 5 seconds for graceful shutdown..."
sleep 5

# Check if processes are still running
STILL_RUNNING=$(pgrep -f "data-miner" || true)

if [ -n "$STILL_RUNNING" ]; then
    echo "⚠️  Processes still running, sending SIGKILL..."
    echo "$STILL_RUNNING" | xargs -r kill -KILL 2>/dev/null || true
    
    # Final check
    sleep 1
    REMAINING=$(pgrep -f "data-miner" || true)
    
    if [ -n "$REMAINING" ]; then
        echo "❌ Failed to kill some processes:"
        echo "$REMAINING"
        exit 1
    fi
fi

echo "✅ All data-miner processes terminated successfully"
exit 0
