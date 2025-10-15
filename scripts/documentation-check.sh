#!/usr/bin/env bash

SCRIPT_DIR=$(dirname "${BASH_SOURCE[0]}")
# shellcheck source=scripts/lib.sh
source "${SCRIPT_DIR}/lib.sh"

CODER_BIN=${CODER_BIN:-"$(which coder)"}

TEMPDIR=$(mktemp -d)
trap 'rm -rf "${TEMPDIR}"' EXIT

[[ -n ${VERBOSE:-} ]] && set -x
set -euo pipefail

usage() {
	echo "Usage: $0 <options>"
	echo "Commands:"
	echo "  create      - Create a new documentation check task"
	echo "  wait        - Wait for task to complete"
	echo "  summary     - Get task output summary"
	echo "  delete      - Delete the task"
	exit 1
}

create() {
	requiredenvs CODER_URL CODER_SESSION_TOKEN CODER_USERNAME TASK_NAME TEMPLATE_NAME TEMPLATE_PRESET PROMPT

	# Check if a task already exists
	set +e
	task_json=$("${CODER_BIN}" \
		--url "${CODER_URL}" \
		--token "${CODER_SESSION_TOKEN}" \
		exp tasks status "${CODER_USERNAME}/${TASK_NAME}" \
		--output json 2>/dev/null)
	set -e

	if [[ "${TASK_NAME}" == $(jq -r '.name' <<<"${task_json}" 2>/dev/null) ]]; then
		echo "Task \"${CODER_USERNAME}/${TASK_NAME}\" already exists. Sending prompt to existing task."
		prompt
		exit 0
	fi

	"${CODER_BIN}" \
		--url "${CODER_URL}" \
		--token "${CODER_SESSION_TOKEN}" \
		exp tasks create \
		--name "${TASK_NAME}" \
		--template "${TEMPLATE_NAME}" \
		--preset "${TEMPLATE_PRESET}" \
		--org coder \
		--owner "${CODER_USERNAME}" \
		--stdin <<<"${PROMPT}"
	exit 0
}

prompt() {
	requiredenvs CODER_URL CODER_SESSION_TOKEN TASK_NAME PROMPT

	${CODER_BIN} \
		--url "${CODER_URL}" \
		--token "${CODER_SESSION_TOKEN}" \
		exp tasks status "${TASK_NAME}" \
		--watch >/dev/null

	${CODER_BIN} \
		--url "${CODER_URL}" \
		--token "${CODER_SESSION_TOKEN}" \
		exp tasks send "${TASK_NAME}" \
		--stdin \
		<<<"${PROMPT}"
}

wait_for_completion() {
	requiredenvs CODER_URL CODER_SESSION_TOKEN TASK_NAME

	${CODER_BIN} \
		--url "${CODER_URL}" \
		--token "${CODER_SESSION_TOKEN}" \
		exp tasks status "${TASK_NAME}" \
		--watch >/dev/null
}

summary() {
	requiredenvs CODER_URL CODER_SESSION_TOKEN TASK_NAME

	last_msg_json=$(
		${CODER_BIN} \
			--url "${CODER_URL}" \
			--token "${CODER_SESSION_TOKEN}" \
			exp tasks logs "${TASK_NAME}" \
			--output json
	)

	# Extract the last output message from the task
	last_output_msg=$(jq -r 'last(.[] | select(.type=="output")) | .content' <<<"${last_msg_json}")

	# Clean up the output (remove bullet points and tool markers)
	last_msg=$(echo "${last_output_msg}" | sed 's/^● //' | sed 's/●//g')

	echo "${last_msg}"
}

delete() {
	requiredenvs CODER_URL CODER_SESSION_TOKEN TASK_NAME

	"${CODER_BIN}" \
		--url "${CODER_URL}" \
		--token "${CODER_SESSION_TOKEN}" \
		delete \
		"${TASK_NAME}" \
		--yes
	exit 0
}

main() {
	dependencies coder

	if [[ $# -eq 0 ]]; then
		usage
	fi

	case "$1" in
	create)
		create
		;;
	wait)
		wait_for_completion
		;;
	summary)
		summary
		;;
	delete)
		delete
		;;
	*)
		echo "Unknown option: $1"
		usage
		;;
	esac
}

main "$@"
