#!/bin/bash
set -euo pipefail

[[ $VERBOSE == 1 ]] && set -x

# shellcheck disable=SC2153 source=scaletest/templates/scaletest-runner/scripts/lib.sh
. "${SCRIPTS_DIR}/lib.sh"

log "Running scaletest..."
set_status Running

start_phase "Creating workspaces"
coder exp scaletest create-workspaces \
	--count "${SCALETEST_NUM_WORKSPACES}" \
	--template "${SCALETEST_TEMPLATE}" \
	--concurrency "${SCALETEST_CREATE_CONCURRENCY}" \
	--job-timeout 15m \
	--no-cleanup \
	--output json:"${SCALETEST_RESULTS_DIR}/create-workspaces.json"
show_json "${SCALETEST_RESULTS_DIR}/create-workspaces.json"
end_phase

wait_baseline 5

start_phase "SSH traffic"
coder exp scaletest workspace-traffic \
	--ssh \
	--bytes-per-tick 10240 \
	--tick-interval 1s \
	--timeout 5m \
	--output json:"${SCALETEST_RESULTS_DIR}/traffic-ssh.json"
show_json "${SCALETEST_RESULTS_DIR}/traffic-ssh.json"
end_phase

wait_baseline 5

start_phase "ReconnectingPTY traffic"
coder exp scaletest workspace-traffic \
	--bytes-per-tick 10240 \
	--tick-interval 1s \
	--timeout 5m \
	--output json:"${SCALETEST_RESULTS_DIR}/traffic-reconnectingpty.json"
show_json "${SCALETEST_RESULTS_DIR}/traffic-reconnectingpty.json"
end_phase

wait_baseline 5

start_phase "Dashboard traffic"
coder exp scaletest dashboard \
	--count "${SCALETEST_NUM_WORKSPACES}" \
	--job-timeout 5m \
	--output json:"${SCALETEST_RESULTS_DIR}/traffic-dashboard.json"
show_json "${SCALETEST_RESULTS_DIR}/traffic-dashboard.json"
end_phase

wait_baseline 5

log "Scaletest complete!"
set_status Complete
