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

start_phase "Cleanup (${event})"
coder exp scaletest cleanup \
	--cleanup-job-timeout 15m \
	--cleanup-timeout 30m |
	tee "${SCALETEST_RESULTS_DIR}/cleanup-${event}.txt"
end_phase

if [[ $event = manual ]]; then
	echo 'Press any key to continue...'
	read -s -r -n 1
fi
