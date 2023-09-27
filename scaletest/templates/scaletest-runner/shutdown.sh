#!/bin/bash
set -e

[[ $VERBOSE == 1 ]] && set -x

# shellcheck disable=SC2153 source=scaletest/templates/scaletest-runner/scripts/lib.sh
. "${SCRIPTS_DIR}/lib.sh"

cleanup() {
	coder tokens remove scaletest_runner >/dev/null 2>&1 || true
}
trap cleanup EXIT

annotate_grafana "workspace" "Agent stopping..."

"${SCRIPTS_DIR}/cleanup.sh" shutdown

annotate_grafana_end "workspace" "Agent running"
