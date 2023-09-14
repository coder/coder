#!/bin/bash
set -euo pipefail

[[ $VERBOSE == 1 ]] && set -x

# shellcheck disable=SC2153 source=scaletest/templates/scaletest-runner/scripts/lib.sh
. "${SCRIPTS_DIR}/lib.sh"

mkdir -p "${SCALETEST_STATE_DIR}"
mkdir -p "${SCALETEST_RESULTS_DIR}"

echo "Preparing scaletest workspace environment..."
set_status Preparing

echo "Cloning coder/coder repo..."

if [[ ! -d ~/coder ]]; then
	git clone https://github.com/coder/coder.git ~/coder
fi
(cd ~/coder && git pull)

echo "Creating coder CLI token (needed for cleanup during shutdown)..."

mkdir -p "${CODER_CONFIG_DIR}"
echo -n "${CODER_URL}" >"${CODER_CONFIG_DIR}/url"

set +x # Avoid logging the token.
# Persist configuration for shutdown script too since the
# owner token is invalidated immediately on workspace stop.
export CODER_SESSION_TOKEN=$CODER_USER_TOKEN
coder tokens delete scaletest_runner >/dev/null 2>&1 || true
# TODO(mafredri): Set TTL? This could interfere with delayed stop though.
token=$(coder tokens create --name scaletest_runner)
unset CODER_SESSION_TOKEN
echo -n "${token}" >"${CODER_CONFIG_DIR}/session"
[[ $VERBOSE == 1 ]] && set -x # Restore logging (if enabled).

log "Cleaning up from previous runs (if applicable)..."
"${SCRIPTS_DIR}/cleanup.sh" "prepare"

echo "Preparation complete!"
