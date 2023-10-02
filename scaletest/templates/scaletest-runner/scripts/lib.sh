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
SCALETEST_PPROF_DIR="${SCALETEST_RUN_DIR}/pprof"

mkdir -p "${SCALETEST_STATE_DIR}" "${SCALETEST_RESULTS_DIR}" "${SCALETEST_PPROF_DIR}"

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
	prev_status=$(get_status)
	if [[ ${prev_status} != *"Not started"* ]]; then
		annotate_grafana_end "status" "Status: ${prev_status}"
	fi
	echo "$(date -Ins) ${*}${dry_run}" >>"${SCALETEST_STATE_DIR}/status"

	annotate_grafana "status" "Status: ${*}"
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
		phase_num=$(grep -c START: "${SCALETEST_PHASE_FILE}")
	fi
	phase_num=$((phase_num + 1))
	log "Start phase ${phase_num}: ${*}"
	echo "$(date -Ins) START:${phase_num}: ${*}" >>"${SCALETEST_PHASE_FILE}"

	GRAFANA_EXTRA_TAGS="${PHASE_TYPE:-phase-default}" annotate_grafana "phase" "Phase ${phase_num}: ${*}"
}
end_phase() {
	phase=$(tail -n 1 "${SCALETEST_PHASE_FILE}" | grep "START:${phase_num}:" | cut -d' ' -f3-)
	if [[ -z ${phase} ]]; then
		log "BUG: Could not find start phase ${phase_num} in ${SCALETEST_PHASE_FILE}"
		exit 1
	fi
	log "End phase ${phase_num}: ${phase}"
	echo "$(date -Ins) END:${phase_num}: ${phase}" >>"${SCALETEST_PHASE_FILE}"

	GRAFANA_EXTRA_TAGS="${PHASE_TYPE:-phase-default}" annotate_grafana_end "phase" "Phase ${phase_num}: ${phase}"
}
get_phase() {
	if [[ -f "${SCALETEST_PHASE_FILE}" ]]; then
		phase_raw=$(tail -n1 "${SCALETEST_PHASE_FILE}")
		phase=$(echo "${phase_raw}" | cut -d' ' -f3-)
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

annotate_grafana() {
	local tags=${1} text=${2} start=${3:-$(($(date +%s) * 1000))}
	local json resp id

	if [[ -z $tags ]]; then
		tags="scaletest,runner"
	else
		tags="scaletest,runner,${tags}"
	fi
	if [[ -n ${GRAFANA_EXTRA_TAGS:-} ]]; then
		tags="${tags},${GRAFANA_EXTRA_TAGS}"
	fi

	log "Annotating Grafana (start=${start}): ${text} [${tags}]"

	json="$(
		jq \
			--argjson time "${start}" \
			--arg text "${text}" \
			--arg tags "${tags}" \
			'{time: $time, tags: $tags | split(","), text: $text}' <<<'{}'
	)"
	if [[ ${DRY_RUN} == 1 ]]; then
		log "Would have annotated Grafana, data=${json}"
		return 0
	fi
	if ! resp="$(
		curl -sSL \
			--insecure \
			-H "Authorization: Bearer ${GRAFANA_API_TOKEN}" \
			-H "Content-Type: application/json" \
			-d "${json}" \
			"${GRAFANA_URL}/api/annotations"
	)"; then
		# Don't abort scaletest just because we couldn't annotate Grafana.
		log "Failed to annotate Grafana: ${resp}"
		return 0
	fi

	if [[ $(jq -r '.message' <<<"${resp}") != "Annotation added" ]]; then
		log "Failed to annotate Grafana: ${resp}"
		return 0
	fi

	log "Grafana annotation added!"

	id="$(jq -r '.id' <<<"${resp}")"
	echo "${id}:${tags}:${text}:${start}" >>"${SCALETEST_STATE_DIR}/grafana-annotations"
}
annotate_grafana_end() {
	local tags=${1} text=${2} start=${3:-} end=${4:-$(($(date +%s) * 1000))}
	local id json resp

	if [[ -z $tags ]]; then
		tags="scaletest,runner"
	else
		tags="scaletest,runner,${tags}"
	fi
	if [[ -n ${GRAFANA_EXTRA_TAGS:-} ]]; then
		tags="${tags},${GRAFANA_EXTRA_TAGS}"
	fi

	if [[ ${DRY_RUN} == 1 ]]; then
		log "Would have updated Grafana annotation (end=${end}): ${text} [${tags}]"
		return 0
	fi

	if ! id=$(grep ":${tags}:${text}:${start}" "${SCALETEST_STATE_DIR}/grafana-annotations" | sort -n | tail -n1 | cut -d: -f1); then
		log "NOTICE: Could not find Grafana annotation to end: '${tags}:${text}:${start}', skipping..."
		return 0
	fi

	log "Annotating Grafana (end=${end}): ${text} [${tags}]"

	json="$(
		jq \
			--argjson timeEnd "${end}" \
			'{timeEnd: $timeEnd}' <<<'{}'
	)"
	if [[ ${DRY_RUN} == 1 ]]; then
		log "Would have patched Grafana annotation: id=${id}, data=${json}"
		return 0
	fi
	if ! resp="$(
		curl -sSL \
			--insecure \
			-H "Authorization: Bearer ${GRAFANA_API_TOKEN}" \
			-H "Content-Type: application/json" \
			-X PATCH \
			-d "${json}" \
			"${GRAFANA_URL}/api/annotations/${id}"
	)"; then
		# Don't abort scaletest just because we couldn't annotate Grafana.
		log "Failed to annotate Grafana end: ${resp}"
		return 0
	fi

	if [[ $(jq -r '.message' <<<"${resp}") != "Annotation patched" ]]; then
		log "Failed to annotate Grafana end: ${resp}"
		return 0
	fi

	log "Grafana annotation patched!"
}

wait_baseline() {
	s=${1:-2}
	PHASE_TYPE="phase-wait" start_phase "Waiting ${s}m to establish baseline"
	maybedryrun "$DRY_RUN" sleep $((s * 60))
	PHASE_TYPE="phase-wait" end_phase
}

get_appearance() {
	session_token=$CODER_USER_TOKEN
	if [[ -f "${CODER_CONFIG_DIR}/session" ]]; then
		session_token="$(<"${CODER_CONFIG_DIR}/session")"
	fi
	curl -sSL \
		-H "Coder-Session-Token: ${session_token}" \
		"${CODER_URL}/api/v2/appearance"
}
set_appearance() {
	local json=$1 color=$2 message=$3

	session_token=$CODER_USER_TOKEN
	if [[ -f "${CODER_CONFIG_DIR}/session" ]]; then
		session_token="$(<"${CODER_CONFIG_DIR}/session")"
	fi
	newjson="$(
		jq \
			--arg color "${color}" \
			--arg message "${message}" \
			'. | .service_banner.message |= $message | .service_banner.background_color |= $color' <<<"${json}"
	)"
	maybedryrun "${DRY_RUN}" curl -sSL \
		-X PUT \
		-H 'Content-Type: application/json' \
		-H "Coder-Session-Token: ${session_token}" \
		--data "${newjson}" \
		"${CODER_URL}/api/v2/appearance"
}
