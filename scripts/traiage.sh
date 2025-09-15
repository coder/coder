#!/usr/bin/env bash

SCRIPT_DIR=$(dirname "${BASH_SOURCE[0]}")
# shellcheck source=scripts/lib.sh
source "${SCRIPT_DIR}/lib.sh"

CODER_BIN=${CODER_BIN:-"$(which coder)"}

[[ -n ${VERBOSE:-} ]] && set -x
set -euo pipefail

usage() {
	echo "Usage: $0 <options>"
	exit 1
}

create() {
	requiredenvs CODER_URL CODER_SESSION_TOKEN WORKSPACE_NAME TEMPLATE_NAME
	"${CODER_BIN}" \
		--url "${CODER_URL}" \
		--token "${CODER_SESSION_TOKEN}" \
		create \
		--template "${TEMPLATE_NAME}" \
		--yes \
		"${WORKSPACE_NAME}"

	exit 0
}

prompt() {
	requiredenvs CODER_URL CODER_SESSION_TOKEN WORKSPACE_NAME PROMPT
	"${CODER_BIN}" \
		--url "${CODER_URL}" \
		--token "${CODER_SESSION_TOKEN}" \
		ssh "${WORKSPACE_NAME}" -- /bin/bash -lc "echo -n '${PROMPT}' | ~/.local/bin/claude --print --verbose --output-format=stream-json -"
}

delete() {
	requiredenvs CODER_URL CODER_SESSION_TOKEN WORKSPACE_NAME
	"${CODER_BIN}" \
		--url "${CODER_URL}" \
		--token "${CODER_SESSION_TOKEN}" \
		delete \
		"${WORKSPACE_NAME}" \
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
		prompt)
			prompt
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
