#!/bin/bash

echo "Testing hasher CLI pipeline..."

# Set up environment
export TERM=xterm

# Run hasher with a timeout and capture output
timeout 10s ./hasher 2>&1 | tee test_output.log

echo "CLI output captured in test_output.log"