#!/bin/bash
set -euo pipefail

[[ $VERBOSE == 1 ]] && set -x

# shellcheck source=scripts/lib.sh
. ~/coder/scripts/lib.sh

coder() {
	maybedryrun "$DRY_RUN" command coder "${@}"
}

show_json() {
	maybedryrun "$DRY_RUN" jq 'del(.. | .logs?)' "${1}"
}

wait_baseline() {
	s=${1:-2}
	echo "Waiting ${s}m (establishing baseline)..."
	echo "${s}" >/tmp/.scaletest_phase_wait_baseline
	sleep $((s * 60))
	rm /tmp/.scaletest_phase_wait_baseline
}

echo "Running scaletest..."
touch /tmp/.scaletest_running

touch /tmp/.scaletest_phase_creating_workspaces

coder exp scaletest create-workspaces \
	--count "${SCALETEST_NUM_WORKSPACES}" \
	--template="${SCALETEST_TEMPLATE}" \
	--concurrency "${SCALETEST_CREATE_CONCURRENCY}" \
	--job-timeout 15m \
	--no-cleanup \
	--output json:"${SCALETEST_RUN_DIR}/result-create-workspaces.json"
show_json "${SCALETEST_RUN_DIR}/result-create-workspaces.json"
rm /tmp/.scaletest_phase_creating_workspaces

wait_baseline 5

touch /tmp/.scaletest_phase_ssh
coder exp scaletest workspace-traffic \
	--ssh \
	--bytes-per-tick 10240 \
	--tick-interval 1s \
	--concurrency 0 \
	--timeout 5m \
	--output json:"${SCALETEST_RUN_DIR}/result-ssh.json"
show_json "${SCALETEST_RUN_DIR}/result-ssh.json"
rm /tmp/.scaletest_phase_ssh

wait_baseline 5

touch /tmp/.scaletest_phase_rpty
coder exp scaletest workspace-traffic \
	--bytes-per-tick 10240 \
	--tick-interval 1s \
	--concurrency 0 \
	--timeout 5m \
	--output json:"${SCALETEST_RUN_DIR}/result-rpty.json"
show_json "${SCALETEST_RUN_DIR}/result-rpty.json"
rm /tmp/.scaletest_phase_rpty

wait_baseline 5

touch /tmp/.scaletest_phase_dashboard
coder exp scaletest dashboard \
	--count "${SCALETEST_NUM_WORKSPACES}" \
	--concurrency 0 \
	--job-timeout 5m \
	--output json:"${SCALETEST_RUN_DIR}/result-dashboard.json"
show_json "${SCALETEST_RUN_DIR}/result-dashboard.json"
rm /tmp/.scaletest_phase_dashboard

wait_baseline 5

echo "Scaletest complete!"
touch /tmp/.scaletest_complete
