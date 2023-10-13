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
	--cleanup-job-timeout 2h \
	--cleanup-timeout 5h |
	tee "${SCALETEST_RESULTS_DIR}/cleanup-${event}.txt"
end_phase

if [[ $event != prepare ]]; then
	start_phase "Scaling down provisioners..."
	maybedryrun "$DRY_RUN" kubectl scale deployment/coder-provisioner --replicas 1
	maybedryrun "$DRY_RUN" kubectl rollout status deployment/coder-provisioner
fi

if [[ $event = manual ]]; then
	echo 'Press any key to continue...'
	read -s -r -n 1
fi
