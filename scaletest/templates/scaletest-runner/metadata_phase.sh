#!/bin/bash

# shellcheck disable=SC2153 source=scaletest/templates/scaletest-runner/scripts/lib.sh
. "${SCRIPTS_DIR}/lib.sh"

if [[ -f "${SCALETEST_PHASE_FILE}" ]]; then
	phase_raw="$(tail -n1 "${SCALETEST_PHASE_FILE}")"
	phase="$(echo "${phase_raw}" | cut -d' ' -f3-)"
	if [[ ${phase_raw} == *"END:"* ]]; then
		phase+=" (done)"
	fi
	echo "${phase}"
else
	echo "None"
fi
