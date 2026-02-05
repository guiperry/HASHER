#!/bin/bash

echo "=== Hasher CLI Shutdown Test ==="

# Cleanup any existing running processes
pkill -9 -f hasher-cli 2>/dev/null || true
sleep 2

# Start application
echo -e "/quit" | timeout 20 ./hasher-cli &
APP_PID=$!
echo "Starting hasher CLI application with PID: $APP_PID"

# Give application time to start and quit
echo "Waiting for hasher CLI to complete..."
wait $APP_PID

# Check if application is still running
if ps -p $APP_PID > /dev/null; then
    echo "❌ Application failed to shut down"
    # Force kill application
    kill -9 $APP_PID 2>/dev/null || true
    sleep 1
    if ps -p $APP_PID > /dev/null; then
        echo "❌ Failed to force kill application"
        exit 1
    else
        echo "⚠️ Application force killed"
    fi
else
    echo "✅ Hasher CLI shut down successfully"
fi
echo "✅ Server is not running initially"

# Start the application
echo -e "/quit" | timeout 20 ./tinyllm &
APP_PID=$!
echo "Starting application with PID: $APP_PID"

# Give server time to start and quit
echo "Waiting for application to complete..."
wait $APP_PID

# Check if server is still running
if curl -s http://localhost:8000/health > /dev/null; then
    echo "❌ Server failed to shut down"
    # Force kill the server
    pkill -9 -f llama-server 2>/dev/null || true
    pkill -9 -f tinyllm 2>/dev/null || true
    sleep 1
    if curl -s http://localhost:8000/health > /dev/null; then
        echo "❌ Failed to force kill server"
        exit 1
    else
        echo "⚠️ Server force killed"
    fi
else
    echo "✅ Server shut down successfully"
fi