#!/usr/bin/env bash

set -euo pipefail

PROJECT_ROOT="$(git rev-parse --show-toplevel)"
cd "${PROJECT_ROOT}"

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
  trap 'kill 0' SIGINT
  CODERV2_HOST=http://127.0.0.1:3000 INSPECT_XSTATE=true yarn --cwd=./site dev &
  go run -tags embed cmd/coder/main.go server --dev --tunnel=true &
  wait
)
