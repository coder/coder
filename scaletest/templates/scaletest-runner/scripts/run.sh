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

non_greedy_agent_traffic_args=()
if [[ ${SCALETEST_PARAM_GREEDY_AGENT} != 1 ]]; then
	greedy_agent_traffic() { :; }
else
	echo "WARNING: Greedy agent enabled, this may cause the load tests to fail." >&2
	non_greedy_agent_traffic_args=(
		# Let the greedy agent traffic command be scraped.
		# --scaletest-prometheus-address 0.0.0.0:21113
		# --trace=false
	)

	annotate_grafana greedy_agent "Create greedy agent"

	coder exp scaletest create-workspaces \
		--count 1 \
		--template "${SCALETEST_PARAM_GREEDY_AGENT_TEMPLATE}" \
		--concurrency 1 \
		--timeout 5h \
		--job-timeout 5h \
		--no-cleanup \
		--output json:"${SCALETEST_RESULTS_DIR}/create-workspaces-greedy-agent.json"

	wait_baseline "${SCALETEST_PARAM_LOAD_SCENARIO_BASELINE_DURATION}"

	greedy_agent_traffic() {
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
		annotate_grafana greedy_agent "${scenario}: Greedy agent traffic"

		# Produce load at about 1000MB/s (25MB/40ms).
		set +e
		coder exp scaletest workspace-traffic \
			--template "${SCALETEST_PARAM_GREEDY_AGENT_TEMPLATE}" \
			--bytes-per-tick $((1024 * 1024 * 25)) \
			--tick-interval 40ms \
			--timeout "$((delay))s" \
			--job-timeout "$((delay))s" \
			--output json:"${SCALETEST_RESULTS_DIR}/traffic-${type}-greedy-agent.json" \
			--scaletest-prometheus-address 0.0.0.0:21113 \
			--trace=false \
			"${args[@]}"
		status=${?}
		show_json "${SCALETEST_RESULTS_DIR}/traffic-${type}-greedy-agent.json"

		export GRAFANA_ADD_TAGS=
		if [[ ${status} != 0 ]]; then
			GRAFANA_ADD_TAGS=error
		fi
		annotate_grafana_end greedy_agent "${scenario}: Greedy agent traffic"

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
		greedy_agent_traffic "${SCALETEST_PARAM_LOAD_SCENARIO_SSH_TRAFFIC_DURATION}" "${scenario}" &
		coder exp scaletest workspace-traffic \
			--template "${SCALETEST_PARAM_TEMPLATE}" \
			--ssh \
			--bytes-per-tick "${SCALETEST_PARAM_LOAD_SCENARIO_SSH_TRAFFIC_BYTES_PER_TICK}" \
			--tick-interval "${SCALETEST_PARAM_LOAD_SCENARIO_SSH_TRAFFIC_TICK_INTERVAL}ms" \
			--timeout "${SCALETEST_PARAM_LOAD_SCENARIO_SSH_TRAFFIC_DURATION}m" \
			--job-timeout "${SCALETEST_PARAM_LOAD_SCENARIO_SSH_TRAFFIC_DURATION}m30s" \
			--output json:"${SCALETEST_RESULTS_DIR}/traffic-ssh.json" \
			"${non_greedy_agent_traffic_args[@]}"
		status=$?
		wait
		status2=$?
		if [[ ${status} == 0 ]]; then
			status=${status2}
		fi
		show_json "${SCALETEST_RESULTS_DIR}/traffic-ssh.json"
		;;
	"Web Terminal Traffic")
		greedy_agent_traffic "${SCALETEST_PARAM_LOAD_SCENARIO_WEB_TERMINAL_TRAFFIC_DURATION}" "${scenario}" &
		coder exp scaletest workspace-traffic \
			--template "${SCALETEST_PARAM_TEMPLATE}" \
			--bytes-per-tick "${SCALETEST_PARAM_LOAD_SCENARIO_WEB_TERMINAL_TRAFFIC_BYTES_PER_TICK}" \
			--tick-interval "${SCALETEST_PARAM_LOAD_SCENARIO_WEB_TERMINAL_TRAFFIC_TICK_INTERVAL}ms" \
			--timeout "${SCALETEST_PARAM_LOAD_SCENARIO_WEB_TERMINAL_TRAFFIC_DURATION}m" \
			--job-timeout "${SCALETEST_PARAM_LOAD_SCENARIO_WEB_TERMINAL_TRAFFIC_DURATION}m30s" \
			--output json:"${SCALETEST_RESULTS_DIR}/traffic-web-terminal.json" \
			"${non_greedy_agent_traffic_args[@]}"
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
	"debug:greedy_agent_traffic")
		greedy_agent_traffic 10 "${scenario}"
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
