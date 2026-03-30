#!/bin/bash
# bwrap-agent.sh: Start the Coder agent inside a bubblewrap sandbox.
#
# This script wraps the agent binary and all its children in a bwrap
# mount namespace with almost all capabilities dropped.
#
# Sandbox policy:
#   - Root filesystem is read-only (prevents system modification)
#   - /home/coder is read-write (project files, shared with dev agent)
#   - /tmp is read-write (scratch space, bind from container /tmp)
#   - /proc is bind-mounted from host (needed by CLI tools)
#   - /dev is bind-mounted from host (devices)
#   - Network is shared (agent must reach coderd)
#   - All capabilities dropped except DAC_OVERRIDE
#
# DAC_OVERRIDE is retained so the sandbox process (running as root)
# can read and write files owned by uid 1000 (coder) on the shared
# home volume without chowning them. This preserves correct
# ownership for the dev agent, which runs as the coder user.
#
# The container must run as root with CAP_SYS_ADMIN so bwrap can
# create the mount namespace. bwrap then drops all caps except
# DAC_OVERRIDE before exec'ing the child process.

set -e

if ! command -v bwrap >/dev/null 2>&1; then
	echo "WARNING: bubblewrap not found, running unsandboxed" >&2
	exec "$@"
fi

exec bwrap \
	--ro-bind / / \
	--bind /home/coder /home/coder \
	--bind /tmp /tmp \
	--bind /proc /proc \
	--dev-bind /dev /dev \
	--die-with-parent \
	--cap-drop ALL \
	--cap-add cap_dac_override \
	"$@"
