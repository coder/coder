#!/usr/bin/env bash

# Usage: ./develop.sh [--agpl]
#
# If the --agpl parameter is specified, builds only the AGPL-licensed code (no
# Coder enterprise features).

SCRIPT_DIR=$(dirname "${BASH_SOURCE[0]}")
# shellcheck disable=SC1091,SC1090
source "${SCRIPT_DIR}/lib.sh"

# Allow toggling verbose output
[[ -n ${VERBOSE:-} ]] && set -x
set -euo pipefail

password="${CODER_DEV_ADMIN_PASSWORD:-password}"

args="$(getopt -o "" -l agpl,password: -- "$@")"
eval set -- "$args"
while true; do
	case "$1" in
	--agpl)
		export CODER_BUILD_AGPL=1
		shift
		;;
	--password)
		password="$2"
		shift 2
		;;
	--)
		shift
		break
		;;
	*)
		error "Unrecognized option: $1"
		;;
	esac
done

# Preflight checks: ensure we have our required dependencies, and make sure nothing is listening on port 3000 or 8080
dependencies curl git go make yarn
curl --fail http://127.0.0.1:3000 >/dev/null 2>&1 && echo '== ERROR: something is listening on port 3000. Kill it and re-run this script.' && exit 1
curl --fail http://127.0.0.1:8080 >/dev/null 2>&1 && echo '== ERROR: something is listening on port 8080. Kill it and re-run this script.' && exit 1

# Compile the CLI binary. This should also compile the frontend and refresh
# node_modules if necessary.
GOOS="$(go env GOOS)"
GOARCH="$(go env GOARCH)"
make -j "build/coder_${GOOS}_${GOARCH}"

# Use the coder dev shim so we don't overwrite the user's existing Coder config.
CODER_DEV_SHIM="${PROJECT_ROOT}/scripts/coder-dev.sh"

# This is a way to run multiple processes in parallel, and have Ctrl-C work correctly
# to kill both at the same time. For more details, see:
# https://stackoverflow.com/questions/3004811/how-do-you-run-multiple-programs-in-parallel-from-a-bash-script
(
	# If something goes wrong, just bail and tear everything down
	# rather than leaving things in an inconsistent state.
	trap 'kill -TERM -$$' ERR
	cdroot
	"${CODER_DEV_SHIM}" server --address 0.0.0.0:3000 || kill -INT -$$ &

	echo '== Waiting for Coder to become ready'
	timeout 60s bash -c 'until curl -s --fail http://localhost:3000 > /dev/null 2>&1; do sleep 0.5; done'

	if [ ! -f "${PROJECT_ROOT}/.coderv2/developsh-did-first-setup" ]; then
		# Try to create the initial admin user.
		"${CODER_DEV_SHIM}" login http://127.0.0.1:3000 --username=admin --email=admin@coder.com --password="${password}" ||
			echo 'Failed to create admin user. To troubleshoot, try running this command manually.'

		# Try to create a regular user.
		"${CODER_DEV_SHIM}" users create --email=member@coder.com --username=member --password="${password}" ||
			echo 'Failed to create regular user. To troubleshoot, try running this command manually.'

		touch "${PROJECT_ROOT}/.coderv2/developsh-did-first-setup"
	fi

	# If we have docker available and the "docker" template doesn't already
	# exist, then let's try to create a template!
	example_template="code-server"
	template_name="docker"
	if docker info >/dev/null 2>&1 && ! "${CODER_DEV_SHIM}" templates versions list "${template_name}" >/dev/null 2>&1; then
		# sometimes terraform isn't installed yet when we go to create the
		# template
		sleep 5

		temp_template_dir="$(mktemp -d)"
		echo "${example_template}" | "${CODER_DEV_SHIM}" templates init "${temp_template_dir}"

		DOCKER_HOST="$(docker context inspect --format '{{ .Endpoints.docker.Host }}')"
		printf 'docker_arch: "%s"\ndocker_host: "%s"\n' "${GOARCH}" "${DOCKER_HOST}" >"${temp_template_dir}/params.yaml"
		(
			"${CODER_DEV_SHIM}" templates create "${template_name}" --directory "${temp_template_dir}" --parameter-file "${temp_template_dir}/params.yaml" --yes
			rm -rfv "${temp_template_dir}" # Only delete template dir if template creation succeeds
		) || echo "Failed to create a template. The template files are in ${temp_template_dir}"
	fi

	# Start the frontend once we have a template up and running
	CODER_HOST=http://127.0.0.1:3000 yarn --cwd=./site dev || kill -INT -$$ &

	log
	log "===================================================================="
	log "==                                                                =="
	log "==            Coder is now running in development mode.           =="
	log "==                  API:    http://localhost:3000                 =="
	log "==                  Web UI: http://localhost:8080                 =="
	log "==                                                                =="
	log "==      Use ./scripts/coder-dev.sh to talk to this instance!      =="
	log "===================================================================="
	log

	# Wait for both frontend and backend to exit.
	wait
)
