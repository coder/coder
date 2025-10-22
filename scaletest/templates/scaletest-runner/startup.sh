#!/bin/bash
set -euo pipefail

[[ $VERBOSE == 1 ]] && set -x

if [[ ${SCALETEST_PARAM_GREEDY_AGENT_TEMPLATE} == "${SCALETEST_PARAM_TEMPLATE}" ]]; then
	echo "ERROR: Greedy agent template must be different from the scaletest template." >&2
	exit 1
fi

if [[ ${SCALETEST_PARAM_LOAD_SCENARIO_RUN_CONCURRENTLY} == 1 ]] && [[ ${SCALETEST_PARAM_GREEDY_AGENT} == 1 ]]; then
	echo "ERROR: Load scenario concurrency and greedy agent test cannot be enabled at the same time." >&2
	exit 1
fi

# Unzip scripts and add to path.
# shellcheck disable=SC2153
echo "Extracting scaletest scripts into ${SCRIPTS_DIR}..."
base64 -d <<<"${SCRIPTS_ZIP}" >/tmp/scripts.zip
rm -rf "${SCRIPTS_DIR}" || true
mkdir -p "${SCRIPTS_DIR}"
unzip -o /tmp/scripts.zip -d "${SCRIPTS_DIR}"
# Chmod to work around https://github.com/coder/coder/issues/10034
chmod +x "${SCRIPTS_DIR}"/*.sh
rm /tmp/scripts.zip

echo "Cloning coder/coder repo..."
if [[ ! -d "${HOME}/coder" ]]; then
	git clone https://github.com/coder/coder.git "${HOME}/coder"
fi
(cd "${HOME}/coder" && git fetch -a && git checkout "${SCALETEST_PARAM_REPO_BRANCH}" && git pull)

# Store the input parameters (for debugging).
env | grep "^SCALETEST_" | sort >"${SCALETEST_RUN_DIR}/environ.txt"

# shellcheck disable=SC2153 source=scaletest/templates/scaletest-runner/scripts/lib.sh
. "${SCRIPTS_DIR}/lib.sh"

appearance_json="$(get_appearance)"
service_banner_message=$(jq -r '.service_banner.message' <<<"${appearance_json}")
service_banner_message="${service_banner_message/% | */}"
service_banner_color="#D65D0F" # Orange.

annotate_grafana "workspace" "Agent running" # Ended in shutdown.sh.

{
	pids=()
	ports=()
	declare -A pods=()
	next_port=6061
	for pod in $(kubectl get pods -l app.kubernetes.io/name=coder -o jsonpath='{.items[*].metadata.name}'); do
		maybedryrun "${DRY_RUN}" kubectl -n coder-big port-forward "${pod}" "${next_port}:6060" &
		pids+=($!)
		ports+=("${next_port}")
		pods[${next_port}]="${pod}"
		next_port=$((next_port + 1))
	done

	trap 'trap - EXIT; kill -INT "${pids[@]}"; exit 1' INT EXIT

	while :; do
		# Sleep for short periods of time so that we can exit quickly.
		# This adds up to ~300 when accounting for profile and trace.
		for ((i = 0; i < 285; i++)); do
			sleep 1
		done
		log "Grabbing pprof dumps"
		start="$(date +%s)"
		annotate_grafana "pprof" "Grab pprof dumps (start=${start})"
		for type in allocs block heap goroutine mutex 'profile?seconds=10' 'trace?seconds=5'; do
			for port in "${ports[@]}"; do
				tidy_type="${type//\?/_}"
				tidy_type="${tidy_type//=/_}"
				maybedryrun "${DRY_RUN}" curl -sSL --output "${SCALETEST_PPROF_DIR}/pprof-${tidy_type}-${pods[${port}]}-${start}.gz" "http://localhost:${port}/debug/pprof/${type}"
			done
		done
		annotate_grafana_end "pprof" "Grab pprof dumps (start=${start})"
	done
} &
pprof_pid=$!

logs_gathered=0
gather_logs() {
	if ((logs_gathered == 1)); then
		return
	fi
	logs_gathered=1

	# Gather logs from all coderd and provisioner instances, and all workspaces.
	annotate_grafana "logs" "Gather logs"
	podsraw="$(
		kubectl -n coder-big get pods -l app.kubernetes.io/name=coder -o name
		kubectl -n coder-big get pods -l app.kubernetes.io/name=coder-provisioner -o name || true
		kubectl -n coder-big get pods -l app.kubernetes.io/name=coder-workspace -o name | grep "^pod/scaletest-" || true
	)"
	mapfile -t pods <<<"${podsraw}"
	for pod in "${pods[@]}"; do
		pod_name="${pod#pod/}"
		kubectl -n coder-big logs "${pod}" --since-time="${SCALETEST_RUN_START_TIME}" >"${SCALETEST_LOGS_DIR}/${pod_name}.txt"
	done
	annotate_grafana_end "logs" "Gather logs"
}

set_appearance "${appearance_json}" "${service_banner_color}" "${service_banner_message} | Scaletest running: [${CODER_USER}/${CODER_WORKSPACE}](${CODER_URL}/@${CODER_USER}/${CODER_WORKSPACE})!"

# Show failure in the UI if script exits with error.
on_exit() {
	code=${?}
	trap - ERR EXIT
	set +e

	kill -INT "${pprof_pid}"

	message_color="#4CD473" # Green.
	message_status=COMPLETE
	if ((code > 0)); then
		message_color="#D94A5D" # Red.
		message_status=FAILED
	fi

	# In case the test failed before gathering logs, gather them before
	# cleaning up, whilst the workspaces are still present.
	gather_logs

	case "${SCALETEST_PARAM_CLEANUP_STRATEGY}" in
	on_stop)
		# Handled by shutdown script.
		;;
	on_success)
		if ((code == 0)); then
			set_appearance "${appearance_json}" "${message_color}" "${service_banner_message} | Scaletest ${message_status}: [${CODER_USER}/${CODER_WORKSPACE}](${CODER_URL}/@${CODER_USER}/${CODER_WORKSPACE}), cleaning up..."
			"${SCRIPTS_DIR}/cleanup.sh" "${SCALETEST_PARAM_CLEANUP_STRATEGY}"
		fi
		;;
	on_error)
		if ((code > 0)); then
			set_appearance "${appearance_json}" "${message_color}" "${service_banner_message} | Scaletest ${message_status}: [${CODER_USER}/${CODER_WORKSPACE}](${CODER_URL}/@${CODER_USER}/${CODER_WORKSPACE}), cleaning up..."
			"${SCRIPTS_DIR}/cleanup.sh" "${SCALETEST_PARAM_CLEANUP_STRATEGY}"
		fi
		;;
	*)
		set_appearance "${appearance_json}" "${message_color}" "${service_banner_message} | Scaletest ${message_status}: [${CODER_USER}/${CODER_WORKSPACE}](${CODER_URL}/@${CODER_USER}/${CODER_WORKSPACE}), cleaning up..."
		"${SCRIPTS_DIR}/cleanup.sh" "${SCALETEST_PARAM_CLEANUP_STRATEGY}"
		;;
	esac

	set_appearance "${appearance_json}" "${message_color}" "${service_banner_message} | Scaletest ${message_status}: [${CODER_USER}/${CODER_WORKSPACE}](${CODER_URL}/@${CODER_USER}/${CODER_WORKSPACE})!"

	annotate_grafana_end "" "Start scaletest: ${SCALETEST_COMMENT}"

	wait "${pprof_pid}"
	exit "${code}"
}
trap on_exit EXIT

on_err() {
	code=${?}
	trap - ERR
	set +e

	log "Scaletest failed!"
	GRAFANA_EXTRA_TAGS=error set_status "Failed (exit=${code})"
	"${SCRIPTS_DIR}/report.sh" failed
	lock_status # Ensure we never rewrite the status after a failure.

	exit "${code}"
}
trap on_err ERR

# Pass session token since `prepare.sh` has not yet run.
CODER_SESSION_TOKEN=$CODER_USER_TOKEN "${SCRIPTS_DIR}/report.sh" started
annotate_grafana "" "Start scaletest: ${SCALETEST_COMMENT}"

"${SCRIPTS_DIR}/prepare.sh"

"${SCRIPTS_DIR}/run.sh"

# Gather logs before ending the test.
gather_logs

"${SCRIPTS_DIR}/report.sh" completed
