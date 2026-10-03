#!/usr/bin/env bash
# Retry a command with exponential backoff.
#
# Usage: retry.sh [--max-attempts N] -- <command...>
#
# Example:
#   retry.sh --max-attempts 3 -- go install gotest.tools/gotestsum@latest
#
# This will retry the command up to 3 times with exponential backoff
# (2s, 4s, 8s delays between attempts).

set -euo pipefail

# shellcheck source=scripts/lib.sh
source "$(dirname "${BASH_SOURCE[0]}")/../../scripts/lib.sh"

max_attempts=3

args="$(getopt -o "" -l max-attempts: -- "$@")"
eval set -- "$args"
while true; do
	case "$1" in
	--max-attempts)
		max_attempts="$2"
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

if [[ $# -lt 1 ]]; then
	error "Usage: retry.sh [--max-attempts N] -- <command...>"
fi

attempt=1
until "$@"; do
	if ((attempt >= max_attempts)); then
		error "Command failed after $max_attempts attempts: $*"
	fi
	delay=$((2 ** attempt))
	log "Attempt $attempt/$max_attempts failed, retrying in ${delay}s..."
	sleep "$delay"
	((attempt++))
done
