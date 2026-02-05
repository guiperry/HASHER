#!/bin/bash

# A script to find and kill running hasher-related processes.
# It identifies processes by name and by network ports they are listening on.

# --- Configuration ---
# Add more process names to this list as needed.
PROCESS_NAMES=(
    "hasher-cli"
    "hasher-server"
    "hasher-host"
)

# Add more ports to this list as needed.
PORTS=(
    "80" # gRPC hasher server
    "8008"  # Hasher host API
)
# --- End of Configuration ---

PIDS_TO_KILL=()

# --- Find processes by name ---
echo "Searching for hasher processes by name: ${PROCESS_NAMES[*]}..."
for name in "${PROCESS_NAMES[@]}"; do
    # pgrep finds processes by name. -f matches against the full command line.
    pids=$(pgrep -f "$name")
    if [ -n "$pids" ]; then
        for pid in $pids;
        do
            PIDS_TO_KILL+=($pid)
            echo "Found process '$name' with PID: $pid"
        done
    fi
done

# --- Find processes by port ---
echo "Searching for processes by port: ${PORTS[*]}..."
for port in "${PORTS[@]}"; do
    # ss is used to investigate sockets.
    # -l: display listening sockets. -n: don't resolve service names.
    # -t: display TCP sockets. -p: show process using socket.
    # The sed command extracts PID.
    pid=$(ss -lntp | grep ":$port" | sed -n 's/.*pid=\([0-9]*\).*/\1/p' | head -n 1)
    if [ -n "$pid" ]; then
        PIDS_TO_KILL+=($pid)
        process_name=$(ps -p $pid -o comm=)
        echo "Found process '$process_name' (PID: $pid) listening on port: $port"
    fi
done

# --- Terminate found processes ---
if [ ${#PIDS_TO_KILL[@]} -eq 0 ]; then
    echo "No running hasher processes found."
else
    # Get unique PIDs to avoid trying to kill the same process twice
    UNIQUE_PIDS=($(echo "${PIDS_TO_KILL[@]}" | tr ' ' '\n' | sort -u | tr '\n' ' '))

    echo "The following PIDs will be terminated: ${UNIQUE_PIDS[*]}"
    for pid in "${UNIQUE_PIDS[@]}"; do
        process_name=$(ps -p $pid -o comm=)
        echo "Killing process '$process_name' (PID: $pid)..."
        # Using kill -9 for a forceful stop.
        kill -9 $pid
        if [ $? -eq 0 ]; then
            echo "Process $pid terminated successfully."
        else
            echo "Failed to terminate process $pid. It may have already exited."
        fi
    done
    echo "Termination complete."
fi

exit 0