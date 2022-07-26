#!/usr/bin/env bash

# Allow toggling verbose output
[[ -n ${VERBOSE:-""} ]] && set -x
set -euo pipefail

SCRIPT_DIR=$(dirname "${BASH_SOURCE[0]}")
# shellcheck disable=SC1091,SC1090
source "${SCRIPT_DIR}/lib.sh"
PROJECT_ROOT=$(cd "$SCRIPT_DIR" && git rev-parse --show-toplevel)
CODER_DEV_BIN="${PROJECT_ROOT}/.coderv2/coder"
set +u
CODER_DEV_ADMIN_PASSWORD="${CODER_DEV_ADMIN_PASSWORD:-password}"
set -u

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

# Compile the CLI binary once just so we don't waste time compiling things multiple times
go build -tags embed -o "${CODER_DEV_BIN}" "${PROJECT_ROOT}/cmd/coder"
# Use the coder dev shim so we don't overwrite the user's existing Coder config.
CODER_DEV_SHIM="${PROJECT_ROOT}/scripts/coder-dev.sh"

# Run yarn install, to make sure node_modules are ready to go
"$PROJECT_ROOT/scripts/yarn_install.sh"

# This is a way to run multiple processes in parallel, and have Ctrl-C work correctly
# to kill both at the same time. For more details, see:
# https://stackoverflow.com/questions/3004811/how-do-you-run-multiple-programs-in-parallel-from-a-bash-script
(
	# If something goes wrong, just bail and tear everything down
	# rather than leaving things in an inconsistent state.
	trap 'kill -TERM -$$' ERR
	cdroot
	"${CODER_DEV_SHIM}" server --address 127.0.0.1:3000 --in-memory --tunnel || kill -INT -$$ &

	echo '== Waiting for Coder to become ready'
	timeout 60s bash -c 'until curl -s --fail http://localhost:3000 > /dev/null 2>&1; do sleep 0.5; done'

	#  create the first user, the admin
	"${CODER_DEV_SHIM}" login http://127.0.0.1:3000 --username=admin --email=admin@coder.com --password="${CODER_DEV_ADMIN_PASSWORD}" ||
		echo 'Failed to create admin user. To troubleshoot, try running this command manually.'

	# || true to always exit code 0. If this fails, whelp.
	"${CODER_DEV_SHIM}" users create --email=member@coder.com --username=member --password="${CODER_DEV_ADMIN_PASSWORD}" ||
		echo 'Failed to create regular user. To troubleshoot, try running this command manually.'

	# If we have docker available, then let's try to create a template!
	template_name=""
	if docker info >/dev/null 2>&1; then
		temp_template_dir=$(mktemp -d)
		echo code-server | "${CODER_DEV_SHIM}" templates init "${temp_template_dir}"
		# shellcheck disable=SC1090
		source <(go env | grep GOARCH)
		DOCKER_HOST=$(docker context inspect --format '{{.Endpoints.docker.Host}}')
		printf 'docker_arch: "%s"\ndocker_host: "%s"\n' "${GOARCH}" "${DOCKER_HOST}" | tee "${temp_template_dir}/params.yaml"
		template_name="docker-${GOARCH}"
		(
			"${CODER_DEV_SHIM}" templates create "${template_name}" --directory "${temp_template_dir}" --parameter-file "${temp_template_dir}/params.yaml" --yes &&
				rm -rfv "${temp_template_dir}" # Only delete template dir if template creation succeeds
		) || echo "Failed to create a template. The template files are in ${temp_template_dir}"
	fi

	# Start the frontend once we have a template up and running
	CODER_HOST=http://127.0.0.1:3000 INSPECT_XSTATE=true yarn --cwd=./site dev || kill -INT -$$ &
	log
	log "======================================================================="
	log "==                                                                   =="
	log "==               Coder is now running in development mode.           =="
	log "==                    API   : http://localhost:3000                  =="
	log "==                    Web UI: http://localhost:8080                  =="
	if [[ -n "${template_name}" ]]; then
		log "==                                                                   =="
		log "==            Docker template ${template_name} is ready to use!          =="
		log "==            Use ./scripts/coder-dev.sh to talk to this instance!   =="
		log "==                                                                   =="
	fi
	log "======================================================================="
	log
	# Wait for both frontend and backend to exit.
	wait
)
