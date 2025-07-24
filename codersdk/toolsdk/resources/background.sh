#!/bin/bash

# This script is used to run a command in the background.

set -o errexit
set -o pipefail

set -o nounset

COMMAND="$ARG_COMMAND"
COMMAND_ID="$ARG_COMMAND_ID"

set +o nounset

LOG_DIR="/tmp/mcp-bg"
LOG_PATH="$LOG_DIR/$COMMAND_ID.log"
mkdir -p "$LOG_DIR"

nohup bash -c "$COMMAND" >"$LOG_PATH" 2>&1 &
COMMAND_PID="$!"

echo "Command started with PID: $COMMAND_PID"
echo "Log path: $LOG_PATH"
