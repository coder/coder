#!/usr/bin/env bash

# Usage: ./develop.sh [--agpl]
#
# If the --agpl parameter is specified, builds only the AGPL-licensed code (no
# Coder enterprise features).

SCRIPT_DIR=$(dirname "${BASH_SOURCE[0]}")
# shellcheck source=scripts/lib.sh
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

pids=()
exit_cleanup() {
	set +e
	# Set empty interrupt handler so cleanup isn't interrupted.
	trap '' INT
	# Send interrupt to the entire process group to start shutdown procedures.
	kill -INT -$$
	# Remove exit trap to avoid infinite loop.
	trap - EXIT

	# Just in case, send interrupts to our children.
	kill -INT "${pids[@]}" >/dev/null 2>&1
	# Use the hammer if things take too long.
	{ sleep 5 && kill -TERM -$$ >/dev/null 2>&1; } &

	# Wait for all children to exit (this can be aborted by hammer).
	wait_cmds
	exit 1
}
start_cmd() {
	echo "== CMD: $*" >&2
	"$@" || fatal "CMD: $*" &
	pids+=("$!")
}
wait_cmds() {
	wait "${pids[@]}" >/dev/null 2>&1
}
fatal() {
	echo "== FAIL: $*" >&2
	exit_cleanup
}

# This is a way to run multiple processes in parallel, and have Ctrl-C work correctly
# to kill both at the same time. For more details, see:
# https://stackoverflow.com/questions/3004811/how-do-you-run-multiple-programs-in-parallel-from-a-bash-script
(
	# If something goes wrong, just bail and tear everything down
	# rather than leaving things in an inconsistent state.
	trap 'exit_cleanup' INT EXIT
	trap 'fatal "Script encountered an error"' ERR

	cdroot
	start_cmd "${CODER_DEV_SHIM}" server --address 0.0.0.0:3000

	echo '== Waiting for Coder to become ready'
	timeout 60s bash -c 'until curl -s --fail http://localhost:3000/healthz > /dev/null 2>&1; do sleep 0.5; done' ||
		fatal 'Coder did not become ready in time'

	# Check if credentials are already set up to avoid setting up again.
	"${CODER_DEV_SHIM}" list >/dev/null 2>&1 && touch "${PROJECT_ROOT}/.coderv2/developsh-did-first-setup"

	if [ ! -f "${PROJECT_ROOT}/.coderv2/developsh-did-first-setup" ]; then
		# Try to create the initial admin user.
		if "${CODER_DEV_SHIM}" login http://127.0.0.1:3000 --first-user-username=admin --first-user-email=admin@coder.com --first-user-password="${password}"; then
			# Only create this file if an admin user was successfully
			# created, otherwise we won't retry on a later attempt.
			touch "${PROJECT_ROOT}/.coderv2/developsh-did-first-setup"
		else
			echo 'Failed to create admin user. To troubleshoot, try running this command manually.'
		fi

		# Try to create a regular user.
		"${CODER_DEV_SHIM}" users create --email=member@coder.com --username=member --password="${password}" ||
			echo 'Failed to create regular user. To troubleshoot, try running this command manually.'
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
	CODER_HOST=http://127.0.0.1:3000 start_cmd yarn --cwd=./site dev --host | {
		while read -r line; do
			echo "[SITE] $(date -Iseconds): $line"
		done
	}

	interfaces=(localhost)
	if which ip >/dev/null 2>&1; then
		# shellcheck disable=SC2207
		interfaces+=($(ip a | awk '/inet / {print $2}' | cut -d/ -f1))
	elif which ifconfig >/dev/null 2>&1; then
		# shellcheck disable=SC2207
		interfaces+=($(ifconfig | awk '/inet / {print $2}'))
	fi

	# Space padding used after the URLs to align "==".
	space_padding=26
	log
	log "===================================================================="
	log "==                                                                =="
	log "==            Coder is now running in development mode.           =="
	for iface in "${interfaces[@]}"; do
		log "$(printf "==                  API:    http://%s:3000%$((space_padding - ${#iface}))s==" "$iface" "")"
	done
	for iface in "${interfaces[@]}"; do
		log "$(printf "==                  Web UI: http://%s:8080%$((space_padding - ${#iface}))s==" "$iface" "")"
	done
	log "==                                                                =="
	log "==      Use ./scripts/coder-dev.sh to talk to this instance!      =="
	log "===================================================================="
	log

	# Wait for both frontend and backend to exit.
	wait_cmds
)
