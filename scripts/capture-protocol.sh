#!/bin/sh
# Protocol Capture Tool
# Monitors CGMiner's communication with /dev/bitmain-asic using strace

DEVICE="/dev/bitmain-asic"
CAPTURE_DURATION=${1:-60}  # Default 60 seconds
OUTPUT_DIR="/tmp/sentinel-capture"

echo "🔍 Hasher Protocol Capture Tool"
echo "======================================"
echo ""
echo "Device: $DEVICE"
echo "Capture Duration: ${CAPTURE_DURATION}s"
echo "Output Directory: $OUTPUT_DIR"
echo ""

# Create output directory
mkdir -p "$OUTPUT_DIR"

# Stop any running cgminer
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo "Phase 1: Stopping existing CGMiner"
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo ""

if pgrep cgminer > /dev/null; then
    echo "Stopping CGMiner..."
    /etc/init.d/cgminer stop
    sleep 2

    # Force kill if still running
    if pgrep cgminer > /dev/null; then
        killall -9 cgminer
        sleep 1
    fi
fi

if pgrep cgminer > /dev/null; then
    echo "❌ Failed to stop CGMiner"
    exit 1
else
    echo "✅ CGMiner stopped"
fi
echo ""

# Start CGMiner under strace
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo "Phase 2: Starting CGMiner with strace"
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo ""

STRACE_LOG="$OUTPUT_DIR/strace.log"
CGMINER_LOG="$OUTPUT_DIR/cgminer.log"

echo "Starting CGMiner with strace monitoring..."
echo "  strace output: $STRACE_LOG"
echo "  cgminer output: $CGMINER_LOG"
echo ""

# Start cgminer under strace
# -e trace=read,write,ioctl,open,close - trace relevant syscalls
# -v - verbose mode (show full data)
# -s 1024 - show first 1024 bytes of strings
# -xx - show strings in hex
strace -e trace=read,write,ioctl,open,close -v -s 1024 -xx -o "$STRACE_LOG" \
    /usr/bin/cgminer --api-listen --bitmain-options 115200:32:8:16:250:0982 \
    > "$CGMINER_LOG" 2>&1 &

CGMINER_PID=$!
echo "✅ CGMiner started (PID: $CGMINER_PID)"
echo ""

# Wait for initialization
echo "Waiting 5 seconds for initialization..."
sleep 5

# Check if cgminer is still running
if ! kill -0 $CGMINER_PID 2>/dev/null; then
    echo "❌ CGMiner failed to start or crashed"
    echo ""
    echo "CGMiner log:"
    cat "$CGMINER_LOG"
    exit 1
fi

echo "✅ CGMiner running"
echo ""

# Capture for specified duration
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo "Phase 3: Capturing Protocol Data"
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo ""
echo "⏱️  Capturing for ${CAPTURE_DURATION} seconds..."
echo "   (You can press Ctrl+C to stop early)"
echo ""

# Show live statistics
for i in $(seq 1 $CAPTURE_DURATION); do
    sleep 1

    # Count operations every 10 seconds
    if [ $((i % 10)) -eq 0 ]; then
        WRITES=$(grep -c "write.*$DEVICE" "$STRACE_LOG" 2>/dev/null || echo 0)
        READS=$(grep -c "read.*$DEVICE" "$STRACE_LOG" 2>/dev/null || echo 0)
        echo "  [${i}s] Writes: $WRITES, Reads: $READS"
    fi
done

echo ""
echo "✅ Capture complete"
echo ""

# Stop cgminer
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo "Phase 4: Stopping CGMiner"
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo ""

if kill $CGMINER_PID 2>/dev/null; then
    echo "Sent SIGTERM to CGMiner..."
    sleep 2

    # Force kill if still running
    if kill -0 $CGMINER_PID 2>/dev/null; then
        kill -9 $CGMINER_PID 2>/dev/null
        sleep 1
    fi
fi

echo "✅ CGMiner stopped"
echo ""

# Analyze captured data
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo "Phase 5: Analyzing Captured Data"
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo ""

# Extract device-specific operations
DEVICE_LOG="$OUTPUT_DIR/device-operations.log"
grep "$DEVICE" "$STRACE_LOG" > "$DEVICE_LOG"

# Count operations
OPEN_COUNT=$(grep -c "open.*$DEVICE" "$DEVICE_LOG" 2>/dev/null || echo 0)
CLOSE_COUNT=$(grep -c "close.*$DEVICE" "$DEVICE_LOG" 2>/dev/null || echo 0)
WRITE_COUNT=$(grep -c "write.*$DEVICE" "$DEVICE_LOG" 2>/dev/null || echo 0)
READ_COUNT=$(grep -c "read.*$DEVICE" "$DEVICE_LOG" 2>/dev/null || echo 0)
IOCTL_COUNT=$(grep -c "ioctl.*$DEVICE" "$DEVICE_LOG" 2>/dev/null || echo 0)

echo "📊 Operation Summary:"
echo "   Opens:  $OPEN_COUNT"
echo "   Closes: $CLOSE_COUNT"
echo "   Writes: $WRITE_COUNT"
echo "   Reads:  $READ_COUNT"
echo "   IOCTLs: $IOCTL_COUNT"
echo ""

# Extract write operations
WRITES_LOG="$OUTPUT_DIR/write-operations.log"
grep "write.*$DEVICE" "$DEVICE_LOG" > "$WRITES_LOG" 2>/dev/null || touch "$WRITES_LOG"

if [ -s "$WRITES_LOG" ]; then
    WRITE_COUNT=$(wc -l < "$WRITES_LOG")
    echo "✅ Captured $WRITE_COUNT write operations"

    # Show first few writes
    echo ""
    echo "📝 Sample Write Operations (first 5):"
    head -5 "$WRITES_LOG" | while read line; do
        echo "   $line"
    done
else
    echo "⚠️  No write operations captured"
fi
echo ""

# Extract read operations
READS_LOG="$OUTPUT_DIR/read-operations.log"
grep "read.*$DEVICE" "$DEVICE_LOG" > "$READS_LOG" 2>/dev/null || touch "$READS_LOG"

if [ -s "$READS_LOG" ]; then
    READ_COUNT=$(wc -l < "$READS_LOG")
    echo "✅ Captured $READ_COUNT read operations"

    # Show first few reads
    echo ""
    echo "📖 Sample Read Operations (first 5):"
    head -5 "$READS_LOG" | while read line; do
        echo "   $line"
    done
else
    echo "⚠️  No read operations captured"
fi
echo ""

# Create summary
SUMMARY_LOG="$OUTPUT_DIR/summary.txt"
cat > "$SUMMARY_LOG" <<EOF
Hasher Protocol Capture Summary
======================================
Capture Date: $(date)
Duration: ${CAPTURE_DURATION}s
Device: $DEVICE

Operation Counts:
- Opens:  $OPEN_COUNT
- Closes: $CLOSE_COUNT
- Writes: $WRITE_COUNT
- Reads:  $READ_COUNT
- IOCTLs: $IOCTL_COUNT

Files Generated:
- strace.log           - Full strace output
- cgminer.log          - CGMiner stdout/stderr
- device-operations.log - All device operations
- write-operations.log - Write operations only
- read-operations.log  - Read operations only
- summary.txt          - This file

Next Steps:
1. Analyze write-operations.log to understand command format
2. Analyze read-operations.log to understand response format
3. Look for patterns in the hex data
4. Extract unique command sequences
5. Build protocol specification
EOF

echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo "✅ Capture Complete!"
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo ""
echo "📁 Output files in: $OUTPUT_DIR"
echo ""
echo "📄 Key files:"
echo "   - device-operations.log (all device I/O)"
echo "   - write-operations.log  (commands sent)"
echo "   - read-operations.log   (responses received)"
echo "   - summary.txt           (overview)"
echo ""
echo "🔍 To download files, run on your machine:"
echo "   scp -r root@192.168.12.151:$OUTPUT_DIR ./captured-protocol"
echo ""
