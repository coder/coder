#!/usr/bin/env bash
# Retry a command with exponential backoff.
# Usage: retry.sh <max_attempts> <command...>
#
# Example:
#   retry.sh 3 go install gotest.tools/gotestsum@latest
#
# This will retry the command up to 3 times with exponential backoff
# (2s, 4s, 8s delays between attempts).

set -euo pipefail

if [[ $# -lt 2 ]]; then
	echo "Usage: retry.sh <max_attempts> <command...>" >&2
	exit 1
fi

max_attempts=$1
shift

attempt=1
until "$@"; do
	if ((attempt >= max_attempts)); then
		echo "Command failed after $max_attempts attempts: $*" >&2
		exit 1
	fi
	delay=$((2 ** attempt))
	echo "Attempt $attempt/$max_attempts failed, retrying in ${delay}s..." >&2
	sleep "$delay"
	((attempt++))
done
