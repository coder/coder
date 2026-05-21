#!/usr/bin/env bash

SCRIPT_DIR=$(dirname "${BASH_SOURCE[0]}")
# shellcheck source=scripts/lib.sh
source "${SCRIPT_DIR}/lib.sh"

CODER_DEV_SHIM="${PROJECT_ROOT}/scripts/coder-dev.sh"

add_license() {
	CODER_DEV_LICENSE="${CODER_DEV_LICENSE:-}"
	if [[ -z "${CODER_DEV_LICENSE}" ]]; then
		echo "No license provided. Please set CODER_DEV_LICENSE environment variable."
		exit 1
	fi

	if [[ "${CODER_BUILD_AGPL:-0}" -gt "0" ]]; then
		echo "Not adding a license in AGPL build mode."
		exit 0
	fi

	NUM_LICENSES=$("${CODER_DEV_SHIM}" licenses list -o json | jq -r '. | length')
	if [[ "${NUM_LICENSES}" -gt "0" ]]; then
		echo "License already exists. Skipping addition."
		exit 0
	fi

	echo -n "${CODER_DEV_LICENSE}" | "${CODER_DEV_SHIM}" licenses add -f - || {
		echo "ERROR: failed to add license. Try adding one manually."
		exit 1
	}

	exit 0
}

main() {
	if [[ $# -eq 0 ]]; then
		echo "Available fixtures:"
		echo "  license: adds the license from CODER_DEV_LICENSE"
		exit 0
	fi

	[[ -n "${VERBOSE:-}" ]] && set -x
	set -euo pipefail

	case "$1" in
	"license")
		add_license
		;;
	*)
		echo "Unknown fixture: $1"
		exit 1
		;;
	esac
}

main "$@"
