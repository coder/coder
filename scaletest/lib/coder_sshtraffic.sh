#!/usr/bin/env bash

if [[ -n "${VERBOSE}" ]]; then
	set -x
fi

BYTES_PER_TICK="${BYTES_PER_TICK:-1024}"
CODER_TOKEN="${CODER_TOKEN:-}"
CODER_URL="${CODER_URL:-}"
TICK_INTERVAL="${TICK_INTERVAL:-1}"
SSH_VERBOSE="${SSH_VERBOSE:-}"
VERBOSE="${VERBOSE:-}"

usage() {
	echo "This script connects to all running Coder workspaces and generates SSH traffic by reading data from /dev/urandom."
	echo "You must have the Owner role in order for this to work."
	echo "Usage: ${SCRIPT_NAME} --coder-url <coder_url> --coder-token <coder_token> [--bytes-per-tick <bytes-per-tick>] [--tick-interval <tick-interval>] [--ssh-verbose] [--verbose]"
	exit 1
}

SCRIPT_NAME=$(basename "${0}")
ARGS="$(getopt -o "" -l bytes-per-tick:,coder-token:,coder-url:,help,tick-interval:,ssh-verbose,verbose, -- "$@")"
eval set -- "$ARGS"
while true; do
	case "$1" in
	--bytes-per-tick)
		BYTES_PER_TICK="$2"
		shift 2
		;;
	--coder-token)
		CODER_TOKEN="$2"
		shift 2
		;;
	--coder-url)
		CODER_URL="$2"
		shift 2
		;;
	--help)
		usage
		;;
	--tick-interval)
		TICK_INTERVAL="$2"
		shift 2
		;;
	--ssh-verbose)
		SSH_VERBOSE="-vvv"
		shift
		;;
	--verbose)
		VERBOSE="1"
		shift
		;;
	--)
		shift
		break
		;;
	*)
		echo "Unrecognized option: $1"
		exit 1
		;;
	esac
done

if [[ -n "${VERBOSE}" ]]; then
	set -x
fi

if [[ -z "${CODER_URL}" ]]; then
	usage
fi

if [[ -z "${CODER_TOKEN}" ]]; then
	usage
fi

set -euo pipefail
trap 'trap - SIGTERM && kill -- -$$' SIGINT SIGTERM EXIT

ACTIVE_WORKSPACES=$(coder --url "${CODER_URL}" --token "${CODER_TOKEN}" list --search status:running -o table -c workspace | tail -n +2)
# This is not guaranteed to work in all cases but it does work with the default Kubernetes template.
CURRENT_WORKSPACE_NAME_GUESS=$(hostname | sed 's/^coder-//g' | sed 's#-#/#g')
for ws in ${ACTIVE_WORKSPACES}; do
	if [[ "${ws}" == "${CURRENT_WORKSPACE_NAME_GUESS}" ]]; then
		# Don't bother sending traffic to the current workspace
		continue
	fi
	# shellcheck disable=SC2086,SC2087
	ssh ${SSH_VERBOSE} localhost \
		-o ConnectTimeout=0 \
		-o StrictHostKeyChecking=no \
		-o UserKnownHostsFile=/dev/null \
		-o LogLevel=ERROR \
		-o ProxyCommand="coder --url $CODER_URL --token $CODER_TOKEN ssh --wait=no --stdio ${ws}" \
		/bin/bash <<EOF 2>&1 &
while true
do
        tr -dc a-zA-Z0-9 < /dev/urandom | head -c ${BYTES_PER_TICK}
        sleep ${TICK_INTERVAL}
done
EOF
done

wait
