#!/bin/bash
set -euo pipefail

[[ $VERBOSE == 1 ]] && set -x

# Unzip scripts and add to path.
# shellcheck disable=SC2153
echo "Extracting scaletest scripts into ${SCRIPTS_DIR}..."
base64 -d <<<"${SCRIPTS_ZIP}" >/tmp/scripts.zip
rm -rf "${SCRIPTS_DIR}" || true
mkdir -p "${SCRIPTS_DIR}"
unzip -o /tmp/scripts.zip -d "${SCRIPTS_DIR}"
rm /tmp/scripts.zip

echo "Cloning coder/coder repo..."
if [[ ! -d "${HOME}/coder" ]]; then
	git clone https://github.com/coder/coder.git "${HOME}/coder"
fi
(cd "${HOME}/coder" && git pull)

# shellcheck disable=SC2153 source=scaletest/templates/scaletest-runner/scripts/lib.sh
. "${SCRIPTS_DIR}/lib.sh"

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
		sleep 285 # ~300 when accounting for profile and trace.
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

# Show failure in the UI if script exits with error.
failed_status=Failed
on_exit() {
	code=${?}
	trap - ERR EXIT
	set +e

	kill -INT "${pprof_pid}"

	case "${SCALETEST_PARAM_CLEANUP_STRATEGY}" in
	on_stop)
		# Handled by shutdown script.
		;;
	on_success)
		if ((code == 0)); then
			"${SCRIPTS_DIR}/cleanup.sh" "${SCALETEST_PARAM_CLEANUP_STRATEGY}"
		fi
		;;
	on_error)
		if ((code > 0)); then
			"${SCRIPTS_DIR}/cleanup.sh" "${SCALETEST_PARAM_CLEANUP_STRATEGY}"
		fi
		;;
	*)
		"${SCRIPTS_DIR}/cleanup.sh" "${SCALETEST_PARAM_CLEANUP_STRATEGY}"
		;;
	esac

	annotate_grafana_end "" "Start scaletest"
}
trap on_exit EXIT

on_err() {
	code=${?}
	trap - ERR
	set +e

	log "Scaletest failed!"
	GRAFANA_EXTRA_TAGS=error set_status "${failed_status} (exit=${code})"
	"${SCRIPTS_DIR}/report.sh" failed
	lock_status # Ensure we never rewrite the status after a failure.

	exit "${code}"
}
trap on_err ERR

# Pass session token since `prepare.sh` has not yet run.
CODER_SESSION_TOKEN=$CODER_USER_TOKEN "${SCRIPTS_DIR}/report.sh" started
annotate_grafana "" "Start scaletest"

"${SCRIPTS_DIR}/prepare.sh"

"${SCRIPTS_DIR}/run.sh"

"${SCRIPTS_DIR}/report.sh" completed
