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

# shellcheck disable=SC1090
source <(go env)

# Preflight checks: ensure we have our required dependencies, and make sure nothing is listening on port 3000 or 8080
dependencies curl git go make yarn
curl --fail http://127.0.0.1:3000 >/dev/null 2>&1 && echo '== ERROR: something is listening on port 3000. Kill it and re-run this script.' && exit 1
curl --fail http://127.0.0.1:8080 >/dev/null 2>&1 && echo '== ERROR: something is listening on port 8080. Kill it and re-run this script.' && exit 1

if [[ ! -e ./site/out/bin/coder.sha1 && ! -e ./site/out/bin/coder.tar.zst ]]; then
	log
	log "======================================================================="
	log "==   Run 'make bin' before running this command to build binaries.   =="
	log "==       Without these binaries, workspaces will fail to start!      =="
	log "======================================================================="
	log
	exit 1
fi

# Run yarn install, to make sure node_modules are ready to go
"$PROJECT_ROOT/scripts/yarn_install.sh"

# This is a way to run multiple processes in parallel, and have Ctrl-C work correctly
# to kill both at the same time. For more details, see:
# https://stackoverflow.com/questions/3004811/how-do-you-run-multiple-programs-in-parallel-from-a-bash-script
(
	# If something goes wrong, just bail and tear everything down
	# rather than leaving things in an inconsistent state.
	trap 'kill -INT -$$' ERR
	cdroot
	CODER_HOST=http://127.0.0.1:3000 INSPECT_XSTATE=true yarn --cwd=./site dev || kill -INT -$$ &
	go run -tags embed cmd/coder/main.go server --address 127.0.0.1:3000 --in-memory --tunnel || kill -INT -$$ &

	echo '== Waiting for Coder to become ready'
	timeout 60s bash -c 'until curl -s --fail http://localhost:3000 > /dev/null 2>&1; do sleep 0.5; done'

	#  create the first user, the admin
	go run cmd/coder/main.go login http://127.0.0.1:3000 --username=admin --email=admin@coder.com --password="${CODER_DEV_ADMIN_PASSWORD}" ||
		echo 'Failed to create admin user. To troubleshoot, try running this command manually.'

	# || true to always exit code 0. If this fails, whelp.
	go run cmd/coder/main.go users create --email=member@coder.com --username=member --password="${CODER_DEV_ADMIN_PASSWORD}" ||
		echo 'Failed to create regular user. To troubleshoot, try running this command manually.'

	# If we have docker available, then let's try to create a template!
	if docker run --rm hello-world >/dev/null 2>&1; then
		temp_template_dir=$(mktemp -d)
		# cd "${temp_template_dir}"
		echo code-server | go run "${PROJECT_ROOT}/cmd/coder/main.go" templates init "${temp_template_dir}"
		if [[ "$GOARCH" = "arm64" ]]; then
			# MacOS sed expects an argument to -i.
			sed_ext_arg=""
			if [[ "$GOOS" = "darwin" ]]; then
				sed_ext_arg="''"
			fi
			sed -i "$sed_ext_arg" 's/arch.*=.*"amd64"/arch = "arm64"/' "${temp_template_dir}/main.tf"
		fi
		go run "${PROJECT_ROOT}/cmd/coder/main.go" templates create "docker-${GOARCH}" -d "${temp_template_dir}" -y
		rm -rfv "${temp_template_dir}"
	fi

	log
	log "======================================================================="
	log "==               Coder is now running in development mode.           =="
	log "==                    API   : http://localhost:3000                  =="
	log "==                    Web UI: http://localhost:8080                  =="
	log "======================================================================="
	log
	# Wait for both frontend and backend to exit.
	wait
)
