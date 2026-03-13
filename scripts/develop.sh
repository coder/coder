#!/usr/bin/env bash

# Usage: ./develop.sh [--agpl] [--port <port>]
#
# If the --agpl parameter is specified, builds only the AGPL-licensed code (no
# Coder enterprise features). The --port parameter changes the API port. The
# frontend dev server still listens on port 8080.

SCRIPT_DIR=$(dirname "${BASH_SOURCE[0]}")
# shellcheck source=scripts/lib.sh
source "${SCRIPT_DIR}/lib.sh"

# Allow toggling verbose output
[[ -n ${VERBOSE:-} ]] && set -x
set -euo pipefail

api_port=3000
web_port=8080
proxy_port=3010
CODER_DEV_ACCESS_URL="${CODER_DEV_ACCESS_URL:-}"
access_url_set=0
if [ -n "${CODER_DEV_ACCESS_URL}" ]; then
	access_url_set=1
fi
DEVELOP_IN_CODER="${DEVELOP_IN_CODER:-0}"
debug=0
DEFAULT_PASSWORD="SomeSecurePassword!"
password="${CODER_DEV_ADMIN_PASSWORD:-${DEFAULT_PASSWORD}}"
use_proxy=0
multi_org=0

# Ensure that extant environment variables do not override
# the config dir we use to override auth for dev.coder.com.
unset CODER_SESSION_TOKEN
unset CODER_URL

args="$(getopt -o "" -l access-url:,use-proxy,agpl,debug,password:,multi-organization,port: -- "$@")"
eval set -- "$args"
while true; do
	case "$1" in
	--access-url)
		CODER_DEV_ACCESS_URL="$2"
		access_url_set=1
		shift 2
		;;
	--port)
		api_port="$2"
		shift 2
		;;
	--agpl)
		export CODER_BUILD_AGPL=1
		shift
		;;
	--password)
		password="$2"
		shift 2
		;;
	--use-proxy)
		use_proxy=1
		shift
		;;
	--multi-organization)
		multi_org=1
		shift
		;;
	--debug)
		debug=1
		shift
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

if [ "${CODER_BUILD_AGPL:-0}" -gt "0" ] && [ "${use_proxy}" -gt "0" ]; then
	echo '== ERROR: cannot use both external proxies and APGL build.' && exit 1
fi

if [ "${CODER_BUILD_AGPL:-0}" -gt "0" ] && [ "${multi_org}" -gt "0" ]; then
	echo '== ERROR: cannot use both multi-organizations and APGL build.' && exit 1
fi

validate_port() {
	local port=$1
	local flag=$2

	if ! [[ "${port}" =~ ^[0-9]+$ ]]; then
		error "${flag} must be an integer between 1 and 65535"
	fi
	if [ "${#port}" -gt 5 ]; then
		error "${flag} must be an integer between 1 and 65535"
	fi
	if ((10#${port} < 1 || 10#${port} > 65535)); then
		error "${flag} must be an integer between 1 and 65535"
	fi
}

validate_port "${api_port}" "--port"
if [ "${api_port}" -eq "${web_port}" ]; then
	error "--port cannot use ${web_port} because the frontend dev server uses that port"
fi
if [ "${use_proxy}" -gt "0" ] && [ "${api_port}" -eq "${proxy_port}" ]; then
	error "--port cannot use ${proxy_port} when --use-proxy is enabled because the workspace proxy uses that port"
fi
if [ "${access_url_set}" -eq 0 ]; then
	CODER_DEV_ACCESS_URL="http://127.0.0.1:${api_port}"
fi

api_url="http://127.0.0.1:${api_port}"
api_local_url="http://localhost:${api_port}"
web_url="http://127.0.0.1:${web_port}"

if [ -n "${CODER_AGENT_URL:-}" ]; then
	DEVELOP_IN_CODER=1
fi

# Preflight checks: ensure we have our required dependencies, and make sure
# nothing is listening on the configured API or frontend ports.
dependencies curl git go jq make pnpm

if curl --silent --fail "${api_url}" >/dev/null 2>&1; then
	# Check if this is the Coder development server.
	if curl --silent --fail "${api_url}/api/v2/buildinfo" 2>&1 | jq -r '.version' >/dev/null 2>&1; then
		echo "== INFO: Coder development server is already running on port ${api_port}!" && exit 0
	else
		echo "== ERROR: something is listening on port ${api_port}. Kill it and re-run this script." && exit 1
	fi
fi

if curl --fail "${web_url}" >/dev/null 2>&1; then
	# Check if this is the Coder development frontend.
	if curl --silent --fail "${web_url}/api/v2/buildinfo" 2>&1 | jq -r '.version' >/dev/null 2>&1; then
		echo "== ERROR: Coder development frontend is already running on port ${web_port}, but the requested API on port ${api_port} is not already running. Stop the frontend and re-run this script." && exit 1
	else
		echo "== ERROR: something is listening on port ${web_port}. Kill it and re-run this script." && exit 1
	fi
fi

# Compile the CLI binary. This should also compile the frontend and refresh
# node_modules if necessary.
GOOS="$(go env GOOS)"
GOARCH="$(go env GOARCH)"
DEVELOP_IN_CODER="${DEVELOP_IN_CODER}" make -j "build/coder_${GOOS}_${GOARCH}"

# Use the coder dev shim so we don't overwrite the user's existing Coder config.
CODER_DEV_SHIM="${PROJECT_ROOT}/scripts/coder-dev.sh"

# Stores the pid of the subshell that runs our main routine.
ppid=0
# Tracks pids of commands we've started.
pids=()
exit_cleanup() {
	set +e
	# Set empty interrupt handler so cleanup isn't interrupted.
	# HUP is included in case SSH drops while cleanup is already
	# in progress from another signal.
	trap '' INT TERM HUP
	# Remove exit trap to avoid infinite loop.
	trap - EXIT

	# Send INT for graceful shutdown. On SIGHUP, the Go server
	# re-registers via signal.Notify and handles it, but other
	# commands may not. INT covers all cases uniformly.
	kill -INT "${pids[@]}" >/dev/null 2>&1
	# Use the hammer if things take too long. Stdout/stderr are
	# closed so the background job can't hold the shell open.
	{ sleep 15 && kill -TERM "${pids[@]}"; } >/dev/null 2>&1 &

	# Wait for all children to exit (this can be aborted by hammer).
	wait_cmds

	# Add a sleep to allow any final output to be flushed to keep the
	# terminal clean.
	sleep 0.5

	exit 1
}
start_cmd() {
	name=$1
	prefix=$2
	shift 2

	echo "== CMD: $*" >&2

	# Shield the command from direct SIGHUP via SIG_IGN, which
	# is inherited across exec. Go's signal.Notify resets it to
	# caught once registered, enabling graceful shutdown.
	(
		trap '' HUP
		FORCE_COLOR=1 exec "$@"
	) > >(
		# Keep draining output until the command's stdout closes.
		# Errexit is off so EIO from a dead terminal doesn't kill
		# this reader and break the pipe (causing SIGPIPE).
		set +e
		trap '' INT HUP

		while read -r line; do
			if [[ $prefix == date ]]; then
				echo "[$name] $(date '+%Y-%m-%d %H:%M:%S') $line"
			else
				echo "[$name] $line"
			fi
		done
		echo "== CMD EXIT: $*" >&2
		# Let parent know the command exited.
		kill -INT $ppid >/dev/null 2>&1
	) 2>&1 &
	pids+=("$!")
}
wait_cmds() {
	wait "${pids[@]}" >/dev/null 2>&1
}
fatal() {
	echo "== FAIL: $*" >&2
	exit_cleanup
	exit 1
}

# This is a way to run multiple processes in parallel, and have Ctrl-C work correctly
# to kill both at the same time. For more details, see:
# https://stackoverflow.com/questions/3004811/how-do-you-run-multiple-programs-in-parallel-from-a-bash-script
(
	ppid=$BASHPID
	# If something goes wrong, just bail and tear everything down
	# rather than leaving things in an inconsistent state.
	trap 'exit_cleanup' INT TERM HUP EXIT
	trap 'fatal "Script encountered an error"' ERR

	cdroot
	DEBUG_DELVE="${debug}" DEVELOP_IN_CODER="${DEVELOP_IN_CODER}" start_cmd API "" "${CODER_DEV_SHIM}" server --http-address "0.0.0.0:${api_port}" --swagger-enable --access-url "${CODER_DEV_ACCESS_URL}" --dangerous-allow-cors-requests=true --enable-terraform-debug-mode "$@"

	echo '== Waiting for Coder to become ready'
	# Start the timeout in the background so interrupting this script
	# doesn't hang for 60s.
	timeout 60s bash -c "until curl -s --fail ${api_local_url}/healthz > /dev/null 2>&1; do sleep 0.5; done" ||
		fatal 'Coder did not become ready in time' &
	wait $!

	# Check if credentials are already set up to avoid setting up again.
	"${CODER_DEV_SHIM}" list >/dev/null 2>&1 && touch "${PROJECT_ROOT}/.coderv2/developsh-did-first-setup"

	if ! "${CODER_DEV_SHIM}" whoami >/dev/null 2>&1; then
		# Try to create the initial admin user.
		echo "Login required; use admin@coder.com and password '${password}'" >&2

		if "${CODER_DEV_SHIM}" login "${api_url}" --first-user-username=admin --first-user-email=admin@coder.com --first-user-password="${password}" --first-user-full-name="Admin User" --first-user-trial=false; then
			# Only create this file if an admin user was successfully
			# created, otherwise we won't retry on a later attempt.
			touch "${PROJECT_ROOT}/.coderv2/developsh-did-first-setup"
		else
			echo 'Failed to create admin user. To troubleshoot, try running this command manually.'
		fi

		# Try to create a regular user.
		"${CODER_DEV_SHIM}" users create --email=member@coder.com --username=member --full-name "Regular User" --password="${password}" ||
			echo 'Failed to create regular user. To troubleshoot, try running this command manually.'
	fi

	# Create a new organization and add the member user to it.
	if [ "${multi_org}" -gt "0" ]; then
		another_org="second-organization"
		if ! "${CODER_DEV_SHIM}" organizations show selected --org "${another_org}" >/dev/null 2>&1; then
			echo "Creating organization '${another_org}'..."
			(
				"${CODER_DEV_SHIM}" organizations create -y "${another_org}"
			) || echo "Failed to create organization '${another_org}'"
		fi

		if ! "${CODER_DEV_SHIM}" org members list --org ${another_org} | grep "^member" >/dev/null 2>&1; then
			echo "Adding member user to organization '${another_org}'..."
			(
				"${CODER_DEV_SHIM}" organizations members add member --org "${another_org}"
			) || echo "Failed to add member user to organization '${another_org}'"
		fi

		echo "Starting external provisioner for '${another_org}'..."
		(
			start_cmd EXT_PROVISIONER "" "${CODER_DEV_SHIM}" provisionerd start --tag "scope=organization" --name second-org-daemon --org "${another_org}"
		) || echo "Failed to start external provisioner. No external provisioner started."
	fi

	# If we have docker available and the "docker" template doesn't already
	# exist, then let's try to create a template!
	template_name="docker"
	# Determine the name of the default org with some jq hacks!
	first_org_name=$("${CODER_DEV_SHIM}" organizations show me -o json | jq -r '.[] | select(.is_default) | .name')
	if docker info >/dev/null 2>&1 && ! "${CODER_DEV_SHIM}" templates versions list "${template_name}" >/dev/null 2>&1; then
		# sometimes terraform isn't installed yet when we go to create the
		# template
		echo "Waiting for terraform to be installed..."
		sleep 5

		echo "Initializing docker template..."
		temp_template_dir="$(mktemp -d)"
		"${CODER_DEV_SHIM}" templates init --id "${template_name}" "${temp_template_dir}"
		# Run terraform init so we get a terraform.lock.hcl
		pushd "${temp_template_dir}" && terraform init && popd

		DOCKER_HOST="$(docker context inspect --format '{{ .Endpoints.docker.Host }}')"
		printf 'docker_arch: "%s"\ndocker_host: "%s"\n' "${GOARCH}" "${DOCKER_HOST}" >"${temp_template_dir}/params.yaml"
		(
			echo "Pushing docker template to '${first_org_name}'..."
			"${CODER_DEV_SHIM}" templates push "${template_name}" --directory "${temp_template_dir}" --variables-file "${temp_template_dir}/params.yaml" --yes --org "${first_org_name}"
			if [ "${multi_org}" -gt "0" ]; then
				echo "Pushing docker template to '${another_org}'..."
				"${CODER_DEV_SHIM}" templates push "${template_name}" --directory "${temp_template_dir}" --variables-file "${temp_template_dir}/params.yaml" --yes --org "${another_org}"
			fi
			rm -rfv "${temp_template_dir}" # Only delete template dir if template creation succeeds
		) || echo "Failed to create a template. The template files are in ${temp_template_dir}"
	fi

	if [ "${use_proxy}" -gt "0" ]; then
		log "Using external workspace proxy"
		(
			# Attempt to delete the proxy first, in case it already exists.
			"${CODER_DEV_SHIM}" wsproxy delete local-proxy --yes || true
			# Create the proxy
			proxy_session_token=$("${CODER_DEV_SHIM}" wsproxy create --name=local-proxy --display-name="Local Proxy" --icon="/emojis/1f4bb.png" --only-token)
			# Start the proxy
			start_cmd PROXY "" "${CODER_DEV_SHIM}" wsproxy server --dangerous-allow-cors-requests=true --http-address="localhost:${proxy_port}" --proxy-session-token="${proxy_session_token}" --primary-access-url="${api_local_url}"
		) || echo "Failed to create workspace proxy. No workspace proxy created."
	fi

	# Start the frontend once we have a template up and running. We pin the
	# port because some environments export PORT for unrelated services.
	PORT="${web_port}" CODER_HOST="${api_url}" start_cmd SITE date pnpm --dir ./site dev --host

	interfaces=(localhost)
	if command -v ip >/dev/null; then
		# shellcheck disable=SC2207
		interfaces+=($(ip a | awk '/inet / {print $2}' | cut -d/ -f1))
	elif command -v ifconfig >/dev/null; then
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
		log "$(printf "==                  API:    http://%s:${api_port}%$((space_padding - ${#iface}))s==" "$iface" "")"
	done
	for iface in "${interfaces[@]}"; do
		log "$(printf "==                  Web UI: http://%s:${web_port}%$((space_padding - ${#iface}))s==" "$iface" "")"
	done
	if [ "${use_proxy}" -gt "0" ]; then
		for iface in "${interfaces[@]}"; do
			log "$(printf "==                  Proxy:  http://%s:${proxy_port}%$((space_padding - ${#iface}))s==" "$iface" "")"
		done
	fi
	log "==                                                                =="
	log "==      Use ./scripts/coder-dev.sh to talk to this instance!      =="
	log "$(printf "==       alias cdr=%s/scripts/coder-dev.sh%$((space_padding - ${#PWD}))s==" "$PWD" "")"
	log "===================================================================="
	log

	# Wait for both frontend and backend to exit.
	wait_cmds
)
