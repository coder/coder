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
	--timeout 5h \
	--job-timeout 5h \
	--no-cleanup \
	--output json:"${SCALETEST_RESULTS_DIR}/create-workspaces.json"
show_json "${SCALETEST_RESULTS_DIR}/create-workspaces.json"
end_phase

wait_baseline "${SCALETEST_PARAM_LOAD_SCENARIO_BASELINE_DURATION}"

declare -A failed=()
for scenario in "${SCALETEST_PARAM_LOAD_SCENARIOS[@]}"; do
	start_phase "Load scenario: ${scenario}"

	set +e
	status=0
	case "${scenario}" in
	"SSH Traffic")
		coder exp scaletest workspace-traffic \
			--ssh \
			--bytes-per-tick "${SCALETEST_PARAM_LOAD_SCENARIO_SSH_TRAFFIC_BYTES_PER_TICK}" \
			--tick-interval "${SCALETEST_PARAM_LOAD_SCENARIO_SSH_TRAFFIC_TICK_INTERVAL}ms" \
			--timeout "${SCALETEST_PARAM_LOAD_SCENARIO_SSH_TRAFFIC_DURATION}m" \
			--job-timeout "${SCALETEST_PARAM_LOAD_SCENARIO_SSH_TRAFFIC_DURATION}m30s" \
			--output json:"${SCALETEST_RESULTS_DIR}/traffic-ssh.json"
		status=$?
		show_json "${SCALETEST_RESULTS_DIR}/traffic-ssh.json"
		;;
	"Web Terminal Traffic")
		coder exp scaletest workspace-traffic \
			--bytes-per-tick "${SCALETEST_PARAM_LOAD_SCENARIO_WEB_TERMINAL_TRAFFIC_BYTES_PER_TICK}" \
			--tick-interval "${SCALETEST_PARAM_LOAD_SCENARIO_WEB_TERMINAL_TRAFFIC_TICK_INTERVAL}ms" \
			--timeout "${SCALETEST_PARAM_LOAD_SCENARIO_WEB_TERMINAL_TRAFFIC_DURATION}m" \
			--job-timeout "${SCALETEST_PARAM_LOAD_SCENARIO_WEB_TERMINAL_TRAFFIC_DURATION}m30s" \
			--output json:"${SCALETEST_RESULTS_DIR}/traffic-web-terminal.json"
		status=$?
		show_json "${SCALETEST_RESULTS_DIR}/traffic-web-terminal.json"
		;;
	"Dashboard Traffic")
		coder exp scaletest dashboard \
			--timeout "${SCALETEST_PARAM_LOAD_SCENARIO_DASHBOARD_TRAFFIC_DURATION}m" \
			--job-timeout "${SCALETEST_PARAM_LOAD_SCENARIO_DASHBOARD_TRAFFIC_DURATION}m30s" \
			--output json:"${SCALETEST_RESULTS_DIR}/traffic-dashboard.json" \
			>"${SCALETEST_RESULTS_DIR}/traffic-dashboard-output.log"
		status=$?
		show_json "${SCALETEST_RESULTS_DIR}/traffic-dashboard.json"
		;;

	# Debug scenarios, for testing the runner.
	"debug:success")
		maybedryrun "$DRY_RUN" sleep 10
		status=0
		;;
	"debug:error")
		maybedryrun "$DRY_RUN" sleep 10
		status=1
		;;
	esac
	set -e
	if ((status > 0)); then
		log "Load scenario failed: ${scenario} (exit=${status})"
		failed+=(["${scenario}"]="$status")
		PHASE_ADD_TAGS=error end_phase
	else
		end_phase
	fi

	wait_baseline "${SCALETEST_PARAM_LOAD_SCENARIO_BASELINE_DURATION}"
done

if ((${#failed[@]} > 0)); then
	log "Load scenarios failed: ${!failed[*]}"
	for scenario in "${!failed[@]}"; do
		log "  ${scenario}: exit=${failed[$scenario]}"
	done
	exit 1
fi

log "Scaletest complete!"
set_status Complete
