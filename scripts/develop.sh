#!/usr/bin/env bash

set -euo pipefail

SCRIPT_DIR=$(dirname "${BASH_SOURCE[0]}")
PROJECT_ROOT=$(cd "$SCRIPT_DIR" && git rev-parse --show-toplevel)

echo '== Run "make build" before running this command to build binaries.'
echo '== Without these binaries, workspaces will fail to start!'

# Run yarn install, to make sure node_modules are ready to go
"$PROJECT_ROOT/scripts/yarn_install.sh"

# Use static credentials for development
export CODER_DEV_ADMIN_EMAIL=admin@coder.com
export CODER_DEV_ADMIN_PASSWORD=password

# This is a way to run multiple processes in parallel, and have Ctrl-C work correctly
# to kill both at the same time. For more details, see:
# https://stackoverflow.com/questions/3004811/how-do-you-run-multiple-programs-in-parallel-from-a-bash-script
(
	cd "${PROJECT_ROOT}"

	trap 'kill 0' SIGINT
	CODERV2_HOST=http://127.0.0.1:3000 INSPECT_XSTATE=true yarn --cwd=./site dev &
	go run -tags embed cmd/coder/main.go server --dev --tunnel=true &
	wait
)
