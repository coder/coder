#!/bin/bash
set -euo pipefail

# Only source this script once, this env comes from sourcing
# scripts/lib.sh from coder/coder below.
if [[ ${SCRIPTS_LIB_IS_SOURCED:-0} == 1 ]]; then
	return 0
fi

# Source scripts/lib.sh from coder/coder for common functions.
# shellcheck source=scripts/lib.sh
. ~/coder/scripts/lib.sh

# Environment variables shared between scripts.
SCALETEST_STATE_DIR="${SCALETEST_RUN_DIR}/state"
SCALETEST_PHASE_FILE="${SCALETEST_STATE_DIR}/phase"
SCALETEST_RESULTS_DIR="${SCALETEST_RUN_DIR}/results"

coder() {
	maybedryrun "$DRY_RUN" command coder "${@}"
}

show_json() {
	maybedryrun "$DRY_RUN" jq 'del(.. | .logs?)' "${1}"
}

set_status() {
	echo "$(date -Iseconds) $*" >>"${SCALETEST_STATE_DIR}/status"
}
lock_status() {
	chmod 0440 "${SCALETEST_STATE_DIR}/status"
}
get_status() {
	# Order of importance (reverse of creation).
	if [[ -f "${SCALETEST_STATE_DIR}/status" ]]; then
		tail -n1 "${SCALETEST_STATE_DIR}/status" | cut -d' ' -f2-
	else
		echo "Not started"
	fi
}

phase_num=0
start_phase() {
	phase_num=$((phase_num + 1))
	log "Start phase ${phase_num}: ${*}"
	echo "$(date -Iseconds) START:${phase_num}: ${*}" >>"${SCALETEST_PHASE_FILE}"
}
end_phase() {
	phase="$(tail -n 1 "${SCALETEST_PHASE_FILE}" | grep "START:${phase_num}:" | cut -d' ' -f3-)"
	if [[ -z ${phase} ]]; then
		log "BUG: Could not find start phase ${phase_num} in ${SCALETEST_PHASE_FILE}"
		exit 1
	fi
	log "End phase ${phase_num}: ${phase}"
	echo "$(date -Iseconds) END:${phase_num}: ${phase}" >>"${SCALETEST_PHASE_FILE}"
}
get_phase() {
	if [[ -f "${SCALETEST_PHASE_FILE}" ]]; then
		phase_raw="$(tail -n1 "${SCALETEST_PHASE_FILE}")"
		phase="$(echo "${phase_raw}" | cut -d' ' -f3-)"
		if [[ ${phase_raw} == *"END:"* ]]; then
			phase+=" [done]"
		fi
		echo "${phase}"
	else
		echo "None"
	fi
}

wait_baseline() {
	s=${1:-2}
	start_phase "Waiting ${s}m to establish baseline"
	sleep $((s * 60))
	end_phase
}
