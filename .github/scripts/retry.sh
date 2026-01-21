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

# shellcheck source=scripts/lib.sh
source "$(dirname "${BASH_SOURCE[0]}")/../../scripts/lib.sh"

if [[ $# -lt 2 ]]; then
	error "Usage: retry.sh <max_attempts> <command...>"
fi

max_attempts=$1
shift

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
