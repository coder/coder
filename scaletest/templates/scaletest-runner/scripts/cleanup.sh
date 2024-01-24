#!/bin/bash
set -euo pipefail

[[ $VERBOSE == 1 ]] && set -x

# shellcheck disable=SC2153 source=scaletest/templates/scaletest-runner/scripts/lib.sh
. "${SCRIPTS_DIR}/lib.sh"

event=${1:-}

if [[ -z $event ]]; then
	event=manual
fi

do_cleanup() {
	start_phase "Cleanup (${event})"
	coder exp scaletest cleanup \
		--cleanup-job-timeout 2h \
		--cleanup-timeout 5h |
		tee "${SCALETEST_RESULTS_DIR}/cleanup-${event}.txt"
	end_phase
}

do_scaledown() {
	start_phase "Scale down provisioners (${event})"
	maybedryrun "$DRY_RUN" kubectl scale deployment/coder-provisioner --replicas 1
	maybedryrun "$DRY_RUN" kubectl rollout status deployment/coder-provisioner
	end_phase
}

case "${event}" in
manual)
	echo -n 'WARNING: This will clean up all scaletest resources, continue? (y/n) '
	read -r -n 1
	if [[ $REPLY != [yY] ]]; then
		echo $'\nAborting...'
		exit 1
	fi
	echo

	do_cleanup
	do_scaledown

	echo 'Press any key to continue...'
	read -s -r -n 1
	;;
prepare)
	do_cleanup
	;;
on_stop) ;; # Do nothing, handled by "shutdown".
always | on_success | on_error | shutdown)
	do_cleanup
	do_scaledown
	;;
shutdown_scale_down_only)
	do_scaledown
	;;
*)
	echo "Unknown event: ${event}" >&2
	exit 1
	;;
esac
