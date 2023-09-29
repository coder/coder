#!/bin/bash
set -e

[[ $VERBOSE == 1 ]] && set -x

# shellcheck disable=SC2153 source=scaletest/templates/scaletest-runner/scripts/lib.sh
. "${SCRIPTS_DIR}/lib.sh"

cleanup() {
	coder tokens remove scaletest_runner >/dev/null 2>&1 || true
	rm -f "${CODER_CONFIG_DIR}/session"
}
trap cleanup EXIT

annotate_grafana "workspace" "Agent stopping..."

"${SCRIPTS_DIR}/cleanup.sh" shutdown

annotate_grafana_end "workspace" "Agent running"

appearance_json="$(get_appearance)"
service_banner_message=$(jq -r '.service_banner.message' <<<"${appearance_json}")
service_banner_message="${service_banner_message/% | */}"
service_banner_color="#4CD473" # Green.

set_appearance "${appearance_json}" "${service_banner_color}" "${service_banner_message}"
