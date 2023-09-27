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

# Show failure in the UI if script exits with error.
failed_status=Failed
on_exit() {
	trap - ERR EXIT

	case "${SCALETEST_PARAM_CLEANUP_STRATEGY}" in
	on_stop)
		# Handled by shutdown script.
		;;
	on_success)
		if [[ $(get_status) != "${failed_status}" ]]; then
			"${SCRIPTS_DIR}/cleanup.sh" "${SCALETEST_PARAM_CLEANUP_STRATEGY}"
		fi
		;;
	on_error)
		if [[ $(get_status) = "${failed_status}" ]]; then
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
}
trap on_err ERR

# Pass session token since `prepare.sh` has not yet run.
CODER_SESSION_TOKEN=$CODER_USER_TOKEN "${SCRIPTS_DIR}/report.sh" started
annotate_grafana "" "Start scaletest"

"${SCRIPTS_DIR}/prepare.sh"

"${SCRIPTS_DIR}/run.sh"

"${SCRIPTS_DIR}/report.sh" completed
