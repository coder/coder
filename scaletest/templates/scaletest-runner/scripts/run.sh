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

if [[ ${SCALETEST_PARAM_GREEDY_AGENT} != 1 ]]; then
	greedy_agent() { :; }
else
	echo "WARNING: Greedy agent enabled, this may cause the load tests to fail." >&2

	coder exp scaletest create-workspaces \
		--count 1 \
		--template "${SCALETEST_PARAM_GREEDY_AGENT_TEMPLATE}" \
		--concurrency 1 \
		--timeout 5h \
		--job-timeout 5h \
		--no-cleanup \
		--output json:"${SCALETEST_RESULTS_DIR}/create-workspaces-greedy-agent.json"

	greedy_agent() {
		local timeout=${1} scenario=${2}
		# Run the greedy test for ~1/3 of the timeout.
		delay=$((timeout * 60 / 3))

		local type=web-terminal
		args=()
		if [[ ${scenario} == "SSH Traffic" ]]; then
			type=ssh
			args+=(--ssh)
		fi

		sleep "${delay}"
		annotate_grafana greedy_agent "${scenario}: Greedy agent"

		# Produce load at about 1000MB/s.
		set +e
		coder exp scaletest workspace-traffic \
			--template "${SCALETEST_PARAM_GREEDY_AGENT_TEMPLATE}" \
			--timeout "$((delay))s" \
			--job-timeout "$((delay))s" \
			--output json:"${SCALETEST_RESULTS_DIR}/traffic-${type}-greedy-agent.json" \
			--bytes-per-tick $((1024 * 1000)) \
			--tick-interval 1ms \
			"${args[@]}"
		status=${?}

		annotate_grafana_end greedy_agent "${scenario}: Greedy agent"

		return ${status}
	}
fi

declare -A failed=()
for scenario in "${SCALETEST_PARAM_LOAD_SCENARIOS[@]}"; do
	start_phase "Load scenario: ${scenario}"

	set +e
	status=0
	case "${scenario}" in
	"SSH Traffic")
		greedy_agent "${SCALETEST_PARAM_LOAD_SCENARIO_SSH_TRAFFIC_DURATION}" "${scenario}" &
		coder exp scaletest workspace-traffic \
			--template "${SCALETEST_PARAM_TEMPLATE}" \
			--ssh \
			--bytes-per-tick "${SCALETEST_PARAM_LOAD_SCENARIO_SSH_TRAFFIC_BYTES_PER_TICK}" \
			--tick-interval "${SCALETEST_PARAM_LOAD_SCENARIO_SSH_TRAFFIC_TICK_INTERVAL}ms" \
			--timeout "${SCALETEST_PARAM_LOAD_SCENARIO_SSH_TRAFFIC_DURATION}m" \
			--job-timeout "${SCALETEST_PARAM_LOAD_SCENARIO_SSH_TRAFFIC_DURATION}m30s" \
			--output json:"${SCALETEST_RESULTS_DIR}/traffic-ssh.json"
		status=$?
		wait
		status2=$?
		if [[ ${status} == 0 ]]; then
			status=${status2}
		fi
		show_json "${SCALETEST_RESULTS_DIR}/traffic-ssh.json"
		;;
	"Web Terminal Traffic")
		greedy_agent "${SCALETEST_PARAM_LOAD_SCENARIO_WEB_TERMINAL_TRAFFIC_DURATION}" "${scenario}" &
		coder exp scaletest workspace-traffic \
			--template "${SCALETEST_PARAM_TEMPLATE}" \
			--bytes-per-tick "${SCALETEST_PARAM_LOAD_SCENARIO_WEB_TERMINAL_TRAFFIC_BYTES_PER_TICK}" \
			--tick-interval "${SCALETEST_PARAM_LOAD_SCENARIO_WEB_TERMINAL_TRAFFIC_TICK_INTERVAL}ms" \
			--timeout "${SCALETEST_PARAM_LOAD_SCENARIO_WEB_TERMINAL_TRAFFIC_DURATION}m" \
			--job-timeout "${SCALETEST_PARAM_LOAD_SCENARIO_WEB_TERMINAL_TRAFFIC_DURATION}m30s" \
			--output json:"${SCALETEST_RESULTS_DIR}/traffic-web-terminal.json"
		status=$?
		wait
		status2=$?
		if [[ ${status} == 0 ]]; then
			status=${status2}
		fi
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
	"debug:greedy_agent")
		greedy_agent 10 "${scenario}"
		status=$?
		;;
	"debug:success")
		maybedryrun "$DRY_RUN" sleep 10
		status=0
		;;
	"debug:error")
		maybedryrun "$DRY_RUN" sleep 10
		status=1
		;;

	*)
		log "WARNING: Unknown load scenario: ${scenario}, skipping..."
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
