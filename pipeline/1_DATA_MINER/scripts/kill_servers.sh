#!/bin/bash

# Script to kill all opencode and ollama background processes

echo "🔍 Checking for running opencode and ollama processes..."

# List of targets to kill
TARGETS=("opencode" "ollama")

for TARGET in "${TARGETS[@]}"; do
    echo "--- Targeting: $TARGET ---"
    
    # Find PIDs
    PIDS=$(pgrep -f "$TARGET" || true)

    if [ -z "$PIDS" ]; then
        echo "✅ No $TARGET processes found running"
        continue
    fi

    echo "⚠️  Found $TARGET processes:"
    echo "$PIDS"
    echo ""

    # Try graceful shutdown first with SIGTERM
    echo "🛑 Sending SIGTERM to $TARGET for graceful shutdown..."
    echo "$PIDS" | xargs -r kill -TERM 2>/dev/null || true

    # Wait a bit for graceful shutdown
    echo "⏳ Waiting 3 seconds..."
    sleep 3

    # Check if processes are still running
    STILL_RUNNING=$(pgrep -f "$TARGET" || true)

    if [ -n "$STILL_RUNNING" ]; then
        echo "⚠️  $TARGET processes still running, sending SIGKILL..."
        echo "$STILL_RUNNING" | xargs -r kill -KILL 2>/dev/null || true
        
        # Final check
        sleep 1
        REMAINING=$(pgrep -f "$TARGET" || true)
        
        if [ -n "$REMAINING" ]; then
            echo "❌ Failed to kill some $TARGET processes:"
            echo "$REMAINING"
        else
            echo "✅ All $TARGET processes terminated"
        fi
    else
        echo "✅ All $TARGET processes terminated successfully"
    fi
done

echo ""
echo "✨ Cleanup complete"
exit 0
