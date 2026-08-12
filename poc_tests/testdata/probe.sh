#!/usr/bin/env bash
# Startup script for the proof of concept acceptance tests.
#
# This runs inside the workspace, launched by the workspace_agent from its
# manifest. It does nothing itself: it hands off to the probe executable so
# that the interesting behaviour lives in Go, where it can be read and changed
# without escaping shell quoting.
#
# The executable and everything it needs arrive through the environment:
#
#   CODER_POC_PROBE_BIN      the executable to run
#   CODER_AGENT_SOCKET_PATH  where the workspace_agent listens
#   CODER_WORKSPACE_ID       the workspace the AI agent would belong to
#   CODER_AGENT_TOKEN        the workspace_agent credential
#   CODER_POC_MARKER_PATH    where the executable records that it ran
#
# The exit status of the executable becomes the exit status of this script,
# which the agent reports back to the control plane, so the test can observe
# failure without relying on the marker alone.

set -euo pipefail

if [[ -z "${CODER_POC_PROBE_BIN:-}" ]]; then
	echo "probe.sh: CODER_POC_PROBE_BIN is not set" >&2
	exit 1
fi

exec "${CODER_POC_PROBE_BIN}"
