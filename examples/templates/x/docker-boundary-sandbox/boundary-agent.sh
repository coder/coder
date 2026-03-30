#!/bin/bash
# boundary-agent.sh: Start the Coder agent inside an Agent Boundary jail.
#
# Agent Boundaries enforce a network allowlist on all outbound HTTP/HTTPS
# traffic. The agent and every tool it spawns inherit the restricted policy,
# which keeps the hidden chat agent from reaching unapproved domains.

set -e

# Install the policy into the user config location because coder boundary
# auto-discovers config there without extra flags.
mkdir -p ~/.config/coder_boundary
cp /etc/coder_boundary/config.yaml ~/.config/coder_boundary/config.yaml
chmod 600 ~/.config/coder_boundary/config.yaml

# Fall back gracefully when the CLI is unavailable so the template still starts
# on deployments that do not expose Agent Boundaries.
if ! command -v coder >/dev/null 2>&1; then
	echo "WARNING: coder CLI not found, running without Agent Boundaries" >&2
	exec "$@"
fi

# Launch the target process in an nsjail-backed boundary so all descendant
# processes inherit the same outbound network restrictions.
exec coder boundary -- "$@"
