#!/bin/bash
set -euo pipefail

[[ $VERBOSE == 1 ]] && set -x

# shellcheck disable=SC2153 source=scaletest/templates/scaletest-runner/scripts/lib.sh
. "${SCRIPTS_DIR}/lib.sh"

mapfile -t scaletest_load_scenarios < <(jq -r '. | join ("\n")' <<<"${SCALETEST_PARAM_LOAD_SCENARIOS}")
export SCALETEST_PARAM_LOAD_SCENARIOS=("${scaletest_load_scenarios[@]}")

log "Running scaletest..."
set_status Running

start_phase "Creating workspaces"
coder exp scaletest create-workspaces \
	--count "${SCALETEST_PARAM_NUM_WORKSPACES}" \
	--template "${SCALETEST_PARAM_TEMPLATE}" \
	--concurrency "${SCALETEST_PARAM_CREATE_CONCURRENCY}" \
	--job-timeout 2h \
	--no-cleanup \
	--output json:"${SCALETEST_RESULTS_DIR}/create-workspaces.json"
show_json "${SCALETEST_RESULTS_DIR}/create-workspaces.json"
end_phase

wait_baseline "${SCALETEST_PARAM_LOAD_SCENARIO_BASELINE_DURATION}"

for scenario in "${SCALETEST_PARAM_LOAD_SCENARIOS[@]}"; do
	start_phase "Load scenario: ${scenario}"
	case "${scenario}" in
	"SSH Traffic")
		coder exp scaletest workspace-traffic \
			--ssh \
			--bytes-per-tick "${SCALETEST_PARAM_LOAD_SCENARIO_SSH_TRAFFIC_BYTES_PER_TICK}" \
			--tick-interval "${SCALETEST_PARAM_LOAD_SCENARIO_SSH_TRAFFIC_TICK_INTERVAL}ms" \
			--timeout "${SCALETEST_PARAM_LOAD_SCENARIO_SSH_TRAFFIC_DURATION}m" \
			--job-timeout "${SCALETEST_PARAM_LOAD_SCENARIO_SSH_TRAFFIC_DURATION}m30s" \
			--output json:"${SCALETEST_RESULTS_DIR}/traffic-ssh.json"
		show_json "${SCALETEST_RESULTS_DIR}/traffic-ssh.json"
		;;
	"Web Terminal Traffic")
		coder exp scaletest workspace-traffic \
			--bytes-per-tick "${SCALETEST_PARAM_LOAD_SCENARIO_WEB_TERMINAL_TRAFFIC_BYTES_PER_TICK}" \
			--tick-interval "${SCALETEST_PARAM_LOAD_SCENARIO_WEB_TERMINAL_TRAFFIC_TICK_INTERVAL}ms" \
			--timeout "${SCALETEST_PARAM_LOAD_SCENARIO_WEB_TERMINAL_TRAFFIC_DURATION}m" \
			--job-timeout "${SCALETEST_PARAM_LOAD_SCENARIO_WEB_TERMINAL_TRAFFIC_DURATION}m30s" \
			--output json:"${SCALETEST_RESULTS_DIR}/traffic-web-terminal.json"
		show_json "${SCALETEST_RESULTS_DIR}/traffic-web-terminal.json"
		;;
	"Dashboard Traffic")
		coder exp scaletest dashboard \
			--count "${SCALETEST_PARAM_NUM_WORKSPACES}" \
			--timeout "${SCALETEST_PARAM_LOAD_SCENARIO_DASHBOARD_TRAFFIC_DURATION}m" \
			--job-timeout "${SCALETEST_PARAM_LOAD_SCENARIO_DASHBOARD_TRAFFIC_DURATION}m30s" \
			--output json:"${SCALETEST_RESULTS_DIR}/traffic-dashboard.json" \
			>"${SCALETEST_RESULTS_DIR}/traffic-dashboard-output.log"
		show_json "${SCALETEST_RESULTS_DIR}/traffic-dashboard.json"
		;;
	esac
	end_phase

	wait_baseline "${SCALETEST_PARAM_LOAD_SCENARIO_BASELINE_DURATION}"
done

log "Scaletest complete!"
set_status Complete
