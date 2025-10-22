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
if [[ ${SCALETEST_PARAM_SKIP_CREATE_WORKSPACES} == 0 ]]; then
	# Note that we allow up to 5 failures to bring up the workspace, since
	# we're creating a lot of workspaces at once and some of them may fail
	# due to network issues or other transient errors.
	coder exp scaletest create-workspaces \
		--retry 5 \
		--count "${SCALETEST_PARAM_NUM_WORKSPACES}" \
		--template "${SCALETEST_PARAM_TEMPLATE}" \
		--concurrency "${SCALETEST_PARAM_CREATE_CONCURRENCY}" \
		--timeout 5h \
		--job-timeout 5h \
		--no-cleanup \
		--output json:"${SCALETEST_RESULTS_DIR}/create-workspaces.json"
	show_json "${SCALETEST_RESULTS_DIR}/create-workspaces.json"
fi
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

		return "${status}"
	}
fi

run_scenario_cmd() {
	local scenario=${1}
	shift
	local command=("$@")

	set +e
	if [[ ${SCALETEST_PARAM_LOAD_SCENARIO_RUN_CONCURRENTLY} == 1 ]]; then
		annotate_grafana scenario "Load scenario: ${scenario}"
	fi
	"${command[@]}"
	status=${?}
	if [[ ${SCALETEST_PARAM_LOAD_SCENARIO_RUN_CONCURRENTLY} == 1 ]]; then
		export GRAFANA_ADD_TAGS=
		if [[ ${status} != 0 ]]; then
			GRAFANA_ADD_TAGS=error
		fi
		annotate_grafana_end scenario "Load scenario: ${scenario}"
	fi
	exit "${status}"
}

declare -a pids=()
declare -A pid_to_scenario=()
declare -A failed=()
target_start=0
target_end=-1

if [[ ${SCALETEST_PARAM_LOAD_SCENARIO_RUN_CONCURRENTLY} == 1 ]]; then
	start_phase "Load scenarios: ${SCALETEST_PARAM_LOAD_SCENARIOS[*]}"
fi
for scenario in "${SCALETEST_PARAM_LOAD_SCENARIOS[@]}"; do
	if [[ ${SCALETEST_PARAM_LOAD_SCENARIO_RUN_CONCURRENTLY} == 0 ]]; then
		start_phase "Load scenario: ${scenario}"
	fi

	set +e
	status=0
	case "${scenario}" in
	"SSH Traffic")
		greedy_agent_traffic "${SCALETEST_PARAM_LOAD_SCENARIO_SSH_TRAFFIC_DURATION}" "${scenario}" &
		greedy_agent_traffic_pid=$!

		target_count=$(jq -n --argjson percentage "${SCALETEST_PARAM_LOAD_SCENARIO_SSH_TRAFFIC_PERCENTAGE}" --argjson num_workspaces "${SCALETEST_PARAM_NUM_WORKSPACES}" '$percentage / 100 * $num_workspaces | floor')
		target_end=$((target_start + target_count))
		if [[ ${target_end} -gt ${SCALETEST_PARAM_NUM_WORKSPACES} ]]; then
			log "WARNING: Target count ${target_end} exceeds number of workspaces ${SCALETEST_PARAM_NUM_WORKSPACES}, using ${SCALETEST_PARAM_NUM_WORKSPACES} instead."
			target_start=0
			target_end=${target_count}
		fi
		run_scenario_cmd "${scenario}" coder exp scaletest workspace-traffic \
			--template "${SCALETEST_PARAM_TEMPLATE}" \
			--ssh \
			--bytes-per-tick "${SCALETEST_PARAM_LOAD_SCENARIO_SSH_TRAFFIC_BYTES_PER_TICK}" \
			--tick-interval "${SCALETEST_PARAM_LOAD_SCENARIO_SSH_TRAFFIC_TICK_INTERVAL}ms" \
			--timeout "${SCALETEST_PARAM_LOAD_SCENARIO_SSH_TRAFFIC_DURATION}m" \
			--job-timeout "${SCALETEST_PARAM_LOAD_SCENARIO_SSH_TRAFFIC_DURATION}m30s" \
			--output json:"${SCALETEST_RESULTS_DIR}/traffic-ssh.json" \
			--scaletest-prometheus-address "0.0.0.0:${SCALETEST_PROMETHEUS_START_PORT}" \
			--target-workspaces "${target_start}:${target_end}" \
			"${non_greedy_agent_traffic_args[@]}" &
		pids+=($!)
		if [[ ${SCALETEST_PARAM_LOAD_SCENARIO_RUN_CONCURRENTLY} == 0 ]]; then
			wait "${pids[-1]}"
			status=$?
			show_json "${SCALETEST_RESULTS_DIR}/traffic-ssh.json"
		else
			SCALETEST_PROMETHEUS_START_PORT=$((SCALETEST_PROMETHEUS_START_PORT + 1))
		fi
		wait "${greedy_agent_traffic_pid}"
		status2=$?
		if [[ ${status} == 0 ]]; then
			status=${status2}
		fi
		;;
	"Web Terminal Traffic")
		greedy_agent_traffic "${SCALETEST_PARAM_LOAD_SCENARIO_WEB_TERMINAL_TRAFFIC_DURATION}" "${scenario}" &
		greedy_agent_traffic_pid=$!

		target_count=$(jq -n --argjson percentage "${SCALETEST_PARAM_LOAD_SCENARIO_WEB_TERMINAL_TRAFFIC_PERCENTAGE}" --argjson num_workspaces "${SCALETEST_PARAM_NUM_WORKSPACES}" '$percentage / 100 * $num_workspaces | floor')
		target_end=$((target_start + target_count))
		if [[ ${target_end} -gt ${SCALETEST_PARAM_NUM_WORKSPACES} ]]; then
			log "WARNING: Target count ${target_end} exceeds number of workspaces ${SCALETEST_PARAM_NUM_WORKSPACES}, using ${SCALETEST_PARAM_NUM_WORKSPACES} instead."
			target_start=0
			target_end=${target_count}
		fi
		run_scenario_cmd "${scenario}" coder exp scaletest workspace-traffic \
			--template "${SCALETEST_PARAM_TEMPLATE}" \
			--bytes-per-tick "${SCALETEST_PARAM_LOAD_SCENARIO_WEB_TERMINAL_TRAFFIC_BYTES_PER_TICK}" \
			--tick-interval "${SCALETEST_PARAM_LOAD_SCENARIO_WEB_TERMINAL_TRAFFIC_TICK_INTERVAL}ms" \
			--timeout "${SCALETEST_PARAM_LOAD_SCENARIO_WEB_TERMINAL_TRAFFIC_DURATION}m" \
			--job-timeout "${SCALETEST_PARAM_LOAD_SCENARIO_WEB_TERMINAL_TRAFFIC_DURATION}m30s" \
			--output json:"${SCALETEST_RESULTS_DIR}/traffic-web-terminal.json" \
			--scaletest-prometheus-address "0.0.0.0:${SCALETEST_PROMETHEUS_START_PORT}" \
			--target-workspaces "${target_start}:${target_end}" \
			"${non_greedy_agent_traffic_args[@]}" &
		pids+=($!)
		if [[ ${SCALETEST_PARAM_LOAD_SCENARIO_RUN_CONCURRENTLY} == 0 ]]; then
			wait "${pids[-1]}"
			status=$?
			show_json "${SCALETEST_RESULTS_DIR}/traffic-web-terminal.json"
		else
			SCALETEST_PROMETHEUS_START_PORT=$((SCALETEST_PROMETHEUS_START_PORT + 1))
		fi
		wait "${greedy_agent_traffic_pid}"
		status2=$?
		if [[ ${status} == 0 ]]; then
			status=${status2}
		fi
		;;
	"App Traffic")
		greedy_agent_traffic "${SCALETEST_PARAM_LOAD_SCENARIO_APP_TRAFFIC_DURATION}" "${scenario}" &
		greedy_agent_traffic_pid=$!

		target_count=$(jq -n --argjson percentage "${SCALETEST_PARAM_LOAD_SCENARIO_APP_TRAFFIC_PERCENTAGE}" --argjson num_workspaces "${SCALETEST_PARAM_NUM_WORKSPACES}" '$percentage / 100 * $num_workspaces | floor')
		target_end=$((target_start + target_count))
		if [[ ${target_end} -gt ${SCALETEST_PARAM_NUM_WORKSPACES} ]]; then
			log "WARNING: Target count ${target_end} exceeds number of workspaces ${SCALETEST_PARAM_NUM_WORKSPACES}, using ${SCALETEST_PARAM_NUM_WORKSPACES} instead."
			target_start=0
			target_end=${target_count}
		fi
		run_scenario_cmd "${scenario}" coder exp scaletest workspace-traffic \
			--template "${SCALETEST_PARAM_TEMPLATE}" \
			--bytes-per-tick "${SCALETEST_PARAM_LOAD_SCENARIO_APP_TRAFFIC_BYTES_PER_TICK}" \
			--tick-interval "${SCALETEST_PARAM_LOAD_SCENARIO_APP_TRAFFIC_TICK_INTERVAL}ms" \
			--timeout "${SCALETEST_PARAM_LOAD_SCENARIO_APP_TRAFFIC_DURATION}m" \
			--job-timeout "${SCALETEST_PARAM_LOAD_SCENARIO_APP_TRAFFIC_DURATION}m30s" \
			--output json:"${SCALETEST_RESULTS_DIR}/traffic-app.json" \
			--scaletest-prometheus-address "0.0.0.0:${SCALETEST_PROMETHEUS_START_PORT}" \
			--app "${SCALETEST_PARAM_LOAD_SCENARIO_APP_TRAFFIC_MODE}" \
			--target-workspaces "${target_start}:${target_end}" \
			"${non_greedy_agent_traffic_args[@]}" &
		pids+=($!)
		if [[ ${SCALETEST_PARAM_LOAD_SCENARIO_RUN_CONCURRENTLY} == 0 ]]; then
			wait "${pids[-1]}"
			status=$?
			show_json "${SCALETEST_RESULTS_DIR}/traffic-app.json"
		else
			SCALETEST_PROMETHEUS_START_PORT=$((SCALETEST_PROMETHEUS_START_PORT + 1))
		fi
		wait "${greedy_agent_traffic_pid}"
		status2=$?
		if [[ ${status} == 0 ]]; then
			status=${status2}
		fi
		;;
	"Dashboard Traffic")
		target_count=$(jq -n --argjson percentage "${SCALETEST_PARAM_LOAD_SCENARIO_DASHBOARD_TRAFFIC_PERCENTAGE}" --argjson num_workspaces "${SCALETEST_PARAM_NUM_WORKSPACES}" '$percentage / 100 * $num_workspaces | floor')
		target_end=$((target_start + target_count))
		if [[ ${target_end} -gt ${SCALETEST_PARAM_NUM_WORKSPACES} ]]; then
			log "WARNING: Target count ${target_end} exceeds number of workspaces ${SCALETEST_PARAM_NUM_WORKSPACES}, using ${SCALETEST_PARAM_NUM_WORKSPACES} instead."
			target_start=0
			target_end=${target_count}
		fi
		# TODO: Remove this once the dashboard traffic command is fixed,
		# (i.e. once images are no longer dumped into PWD).
		mkdir -p dashboard
		pushd dashboard
		run_scenario_cmd "${scenario}" coder exp scaletest dashboard \
			--timeout "${SCALETEST_PARAM_LOAD_SCENARIO_DASHBOARD_TRAFFIC_DURATION}m" \
			--job-timeout "${SCALETEST_PARAM_LOAD_SCENARIO_DASHBOARD_TRAFFIC_DURATION}m30s" \
			--output json:"${SCALETEST_RESULTS_DIR}/traffic-dashboard.json" \
			--scaletest-prometheus-address "0.0.0.0:${SCALETEST_PROMETHEUS_START_PORT}" \
			--target-users "${target_start}:${target_end}" \
			>"${SCALETEST_RESULTS_DIR}/traffic-dashboard-output.log" &
		pids+=($!)
		popd
		if [[ ${SCALETEST_PARAM_LOAD_SCENARIO_RUN_CONCURRENTLY} == 0 ]]; then
			wait "${pids[-1]}"
			status=$?
			show_json "${SCALETEST_RESULTS_DIR}/traffic-dashboard.json"
		else
			SCALETEST_PROMETHEUS_START_PORT=$((SCALETEST_PROMETHEUS_START_PORT + 1))
		fi
		;;

	# Debug scenarios, for testing the runner.
	"debug:greedy_agent_traffic")
		greedy_agent_traffic 10 "${scenario}" &
		pids+=($!)
		if [[ ${SCALETEST_PARAM_LOAD_SCENARIO_RUN_CONCURRENTLY} == 0 ]]; then
			wait "${pids[-1]}"
			status=$?
		else
			SCALETEST_PROMETHEUS_START_PORT=$((SCALETEST_PROMETHEUS_START_PORT + 1))
		fi
		;;
	"debug:success")
		{
			maybedryrun "$DRY_RUN" sleep 10
			true
		} &
		pids+=($!)
		if [[ ${SCALETEST_PARAM_LOAD_SCENARIO_RUN_CONCURRENTLY} == 0 ]]; then
			wait "${pids[-1]}"
			status=$?
		else
			SCALETEST_PROMETHEUS_START_PORT=$((SCALETEST_PROMETHEUS_START_PORT + 1))
		fi
		;;
	"debug:error")
		{
			maybedryrun "$DRY_RUN" sleep 10
			false
		} &
		pids+=($!)
		if [[ ${SCALETEST_PARAM_LOAD_SCENARIO_RUN_CONCURRENTLY} == 0 ]]; then
			wait "${pids[-1]}"
			status=$?
		else
			SCALETEST_PROMETHEUS_START_PORT=$((SCALETEST_PROMETHEUS_START_PORT + 1))
		fi
		;;

	*)
		log "WARNING: Unknown load scenario: ${scenario}, skipping..."
		;;
	esac
	set -e

	# Allow targeting to be distributed evenly across workspaces when each
	# scenario is run concurrently and all percentages add up to 100.
	target_start=${target_end}

	if [[ ${SCALETEST_PARAM_LOAD_SCENARIO_RUN_CONCURRENTLY} == 1 ]]; then
		pid_to_scenario+=(["${pids[-1]}"]="${scenario}")
		# Stagger the start of each scenario to avoid a burst of load and deted
		# problematic scenarios.
		sleep $((SCALETEST_PARAM_LOAD_SCENARIO_CONCURRENCY_STAGGER_DELAY_MINS * 60))
		continue
	fi

	if ((status > 0)); then
		log "Load scenario failed: ${scenario} (exit=${status})"
		failed+=(["${scenario}"]="${status}")
		PHASE_ADD_TAGS=error end_phase
	else
		end_phase
	fi

	wait_baseline "${SCALETEST_PARAM_LOAD_SCENARIO_BASELINE_DURATION}"
done
if [[ ${SCALETEST_PARAM_LOAD_SCENARIO_RUN_CONCURRENTLY} == 1 ]]; then
	wait "${pids[@]}"
	# Wait on all pids will wait until all have exited, but we need to
	# check their individual exit codes.
	for pid in "${pids[@]}"; do
		wait "${pid}"
		status=${?}
		scenario=${pid_to_scenario[${pid}]}
		if ((status > 0)); then
			log "Load scenario failed: ${scenario} (exit=${status})"
			failed+=(["${scenario}"]="${status}")
		fi
	done
	if ((${#failed[@]} > 0)); then
		PHASE_ADD_TAGS=error end_phase
	else
		end_phase
	fi
fi

if ((${#failed[@]} > 0)); then
	log "Load scenarios failed: ${!failed[*]}"
	for scenario in "${!failed[@]}"; do
		log "  ${scenario}: exit=${failed[$scenario]}"
	done
	exit 1
fi

log "Scaletest complete!"
set_status Complete
