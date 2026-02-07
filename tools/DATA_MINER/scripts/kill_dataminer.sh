#!/bin/bash

# Script to kill all dataminer processes

set -e

echo "🔍 Checking for running dataminer processes..."

# Find dataminer PIDs
PIDS=$(pgrep -f "dataminer" || true)

if [ -z "$PIDS" ]; then
    echo "✅ No dataminer processes found running"
    exit 0
fi

echo "⚠️  Found dataminer processes:"
echo "$PIDS"
echo ""

# Try graceful shutdown first with SIGTERM
echo "🛑 Sending SIGTERM for graceful shutdown..."
echo "$PIDS" | xargs -r kill -TERM 2>/dev/null || true

# Wait a bit for graceful shutdown
echo "⏳ Waiting 5 seconds for graceful shutdown..."
sleep 5

# Check if processes are still running
STILL_RUNNING=$(pgrep -f "dataminer" || true)

if [ -n "$STILL_RUNNING" ]; then
    echo "⚠️  Processes still running, sending SIGKILL..."
    echo "$STILL_RUNNING" | xargs -r kill -KILL 2>/dev/null || true
    
    # Final check
    sleep 1
    REMAINING=$(pgrep -f "dataminer" || true)
    
    if [ -n "$REMAINING" ]; then
        echo "❌ Failed to kill some processes:"
        echo "$REMAINING"
        exit 1
    fi
fi

echo "✅ All dataminer processes terminated successfully"
exit 0
