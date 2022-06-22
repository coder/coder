#!/usr/bin/env bash

# Allow toggling verbose output
[[ -n ${VERBOSE:-""} ]] && set -x
set -euo pipefail

SCRIPT_DIR=$(dirname "${BASH_SOURCE[0]}")
# shellcheck disable=SC1091
source "${SCRIPT_DIR}/lib.sh"
PROJECT_ROOT=$(cd "$SCRIPT_DIR" && git rev-parse --show-toplevel)
set +u
CODER_DEV_ADMIN_PASSWORD="${CODER_DEV_ADMIN_PASSWORD:-password}"
set -u

# Preflight checks: ensure we have our required dependencies, and make sure nothing is listening on port 3000 or 8080
dependencies curl git go make nc yarn
nc -z localhost 3000 && echo '== ERROR: something is listening on port 3000. Kill it and re-run this script.' && exit 1
nc -z localhost 8080 && echo '== ERROR: something is listening on port 8080. Kill it and re-run this script.' && exit 1

echo '== Run "make build" before running this command to build binaries.'
echo '== Without these binaries, workspaces will fail to start!'

# Run yarn install, to make sure node_modules are ready to go
"$PROJECT_ROOT/scripts/yarn_install.sh"

# This is a way to run multiple processes in parallel, and have Ctrl-C work correctly
# to kill both at the same time. For more details, see:
# https://stackoverflow.com/questions/3004811/how-do-you-run-multiple-programs-in-parallel-from-a-bash-script
(
	SCRIPT_PID=$$
	cd "${PROJECT_ROOT}"
	CODERV2_HOST=http://127.0.0.1:3000 INSPECT_XSTATE=true yarn --cwd=./site dev || kill -INT ${SCRIPT_PID} &
	go run -tags embed cmd/coder/main.go server --in-memory --tunnel || kill -INT ${SCRIPT_PID} &

	echo '== Waiting for Coder to become ready'
	timeout 60s bash -c 'until curl -s --fail http://localhost:3000 > /dev/null 2>&1; do sleep 0.5; done'

	#  create the first user, the admin
	go run cmd/coder/main.go login http://127.0.0.1:3000 --username=admin --email=admin@coder.com --password="${CODER_DEV_ADMIN_PASSWORD}" ||
		echo 'Failed to create admin user. To troubleshoot, try running this command manually.'

	# || true to always exit code 0. If this fails, whelp.
	go run cmd/coder/main.go users create --email=member@coder.com --username=member --password="${CODER_DEV_ADMIN_PASSWORD}" ||
		echo 'Failed to create regular user. To troubleshoot, try running this command manually.'
	wait
)
