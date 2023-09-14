#!/bin/bash
set -euo pipefail

[[ $VERBOSE == 1 ]] && set -x

# shellcheck disable=SC2153 source=scaletest/templates/scaletest-runner/scripts/lib.sh
. "${SCRIPTS_DIR}/lib.sh"

event=${1:-}

if [[ -z $event ]]; then
	event=manual
fi

if [[ $event = manual ]]; then
	echo -n 'WARNING: This will clean up all scaletest resources, continue? (y/n) '
	read -r -n 1
	if [[ $REPLY != [yY] ]]; then
		echo $'\nAborting...'
		exit 1
	fi
fi

phase_start "Cleanup (${event})"
coder exp scaletest cleanup \
	--cleanup-concurrency "${SCALETEST_CLEANUP_CONCURRENCY}" \
	--cleanup-job-timeout 15m \
	--cleanup-timeout 30m \
	| tee "result-cleanup-${event}.txt"
end_phase

if [[ $event = manual ]]; then
	echo 'Press any key to continue...'
	read -s -r -n 1
fi
