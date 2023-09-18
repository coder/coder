#!/bin/bash
set -euo pipefail

# Only source this script once, this env comes from sourcing
# scripts/lib.sh from coder/coder below.
if [[ ${SCRIPTS_LIB_IS_SOURCED:-0} == 1 ]]; then
	return 0
fi

# Source scripts/lib.sh from coder/coder for common functions.
# shellcheck source=scripts/lib.sh
. "${HOME}/coder/scripts/lib.sh"

# Make shellcheck happy.
DRY_RUN=${DRY_RUN:-0}

# Environment variables shared between scripts.
SCALETEST_STATE_DIR="${SCALETEST_RUN_DIR}/state"
SCALETEST_PHASE_FILE="${SCALETEST_STATE_DIR}/phase"
# shellcheck disable=SC2034
SCALETEST_RESULTS_DIR="${SCALETEST_RUN_DIR}/results"

coder() {
	maybedryrun "${DRY_RUN}" command coder "${@}"
}

show_json() {
	maybedryrun "${DRY_RUN}" jq 'del(.. | .logs?)' "${1}"
}

set_status() {
	dry_run=
	if [[ ${DRY_RUN} == 1 ]]; then
		dry_run=" (dry-ryn)"
	fi
	echo "$(date -Ins) ${*}${dry_run}" >>"${SCALETEST_STATE_DIR}/status"
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
	# This may be incremented from another script, so we read it every time.
	if [[ -f "${SCALETEST_PHASE_FILE}" ]]; then
		phase_num="$(grep -c START: "${SCALETEST_PHASE_FILE}")"
	fi
	phase_num=$((phase_num + 1))
	log "Start phase ${phase_num}: ${*}"
	echo "$(date -Ins) START:${phase_num}: ${*}" >>"${SCALETEST_PHASE_FILE}"
}
end_phase() {
	phase="$(tail -n 1 "${SCALETEST_PHASE_FILE}" | grep "START:${phase_num}:" | cut -d' ' -f3-)"
	if [[ -z ${phase} ]]; then
		log "BUG: Could not find start phase ${phase_num} in ${SCALETEST_PHASE_FILE}"
		exit 1
	fi
	log "End phase ${phase_num}: ${phase}"
	echo "$(date -Ins) END:${phase_num}: ${phase}" >>"${SCALETEST_PHASE_FILE}"
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
get_previous_phase() {
	if [[ -f "${SCALETEST_PHASE_FILE}" ]] && [[ $(grep -c START: "${SCALETEST_PHASE_FILE}") -gt 1 ]]; then
		grep START: "${SCALETEST_PHASE_FILE}" | tail -n2 | head -n1 | cut -d' ' -f3-
	else
		echo "None"
	fi
}

wait_baseline() {
	s=${1:-2}
	start_phase "Waiting ${s}m to establish baseline"
	maybedryrun "$DRY_RUN" sleep $((s * 60))
	end_phase
}
