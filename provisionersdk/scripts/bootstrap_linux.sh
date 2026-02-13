#!/usr/bin/env sh
set -eux

# Sleep for a good long while before exiting.
# This is to allow folks to exec into a failed workspace and poke around to
# troubleshoot.
waitonexit() {
	echo "=== Agent script exited with non-zero code ($?). Sleeping 24h to preserve logs..."
	sleep 86400
}
trap waitonexit EXIT

BINARY_DIR="${BINARY_DIR:-$(mktemp -d -t coder.XXXXXX)}"
BINARY_NAME=coder
BINARY_URL=${ACCESS_URL}bin/coder-linux-${ARCH}
cd "$BINARY_DIR"

# Attempt to download the coder agent.
# This could fail for a number of reasons, many of which are likely transient.
# So just keep trying!
while :; do
	# Try a number of different download tools, as we don not know what we
	# will have available.
	status=""
	if command -v curl >/dev/null 2>&1; then
		curl -fsSL --compressed "${BINARY_URL}" -o "${BINARY_NAME}" && break
		status=$?
	elif command -v wget >/dev/null 2>&1; then
		wget -q "${BINARY_URL}" -O "${BINARY_NAME}" && break
		status=$?
	elif command -v busybox >/dev/null 2>&1; then
		busybox wget -O "${BINARY_NAME}" "${BINARY_URL}" && break
		status=$?
	else
		echo "error: no download tool found, please install curl, wget or busybox"
		exit 127
	fi
	echo "error: failed to download coder agent"
	echo "       command returned non-zero exit code: ${status}"
	echo "Trying again in 30 seconds..."
	sleep 30
done

if ! chmod +x $BINARY_NAME; then
	echo "Failed to make $BINARY_NAME executable"
	exit 1
fi

export CODER_AGENT_AUTH="${AUTH_TYPE}"
export CODER_AGENT_URL="${ACCESS_URL}"

echo "Supervised? ${SUPERVISED}"

# If SUPERVISED is set to "true", install and run the agent as a systemd user
# unit instead of running it directly. This provides automatic restarts,
# proper lifecycle management, and journal logging.
if [ "${SUPERVISED}" = "true" ]; then
	# Remove the waitonexit trap since systemd manages the process lifecycle.
	trap - EXIT

	BINARY_ABS_PATH="$(pwd)/${BINARY_NAME}"

	# Ensure the systemd user unit directory exists.
	mkdir -p "${HOME}/.config/systemd/user"

	# Write the environment file so the unit picks up the agent token and URL.
	# This avoids baking secrets into the unit file itself.
	ENV_FILE="${HOME}/.config/coder-agent.env"
	cat >"${ENV_FILE}" <<-ENVEOF
		CODER_AGENT_AUTH=${CODER_AGENT_AUTH}
		CODER_AGENT_URL=${CODER_AGENT_URL}
	ENVEOF
	chmod 600 "${ENV_FILE}"

	# Write the ExecStopPost helper script. When the agent exits
	# abnormally, systemd runs this to record the kill signal so
	# the next agent instance can report it to coderd. On clean
	# exit ($SERVICE_RESULT=success) nothing is written because
	# the agent clears the file on graceful shutdown.
	#
	# This is a separate script file (rather than an inline
	# command) because the bootstrap script itself may be
	# embedded inside a single-quoted sh -c wrapper by
	# cloud-init userdata templates, and nested single
	# quotes break that wrapper.
	KILL_SIGNAL_FILE="/tmp/coder-agent-kill-signal.txt"
	STOP_POST_SCRIPT="${HOME}/.config/coder-agent-stop-post.sh"
	cat >"${STOP_POST_SCRIPT}" <<-STOPEOF
		#!/bin/sh
		if [ "\$SERVICE_RESULT" != "success" ]; then
		    echo "OOOPS \$EXIT_STATUS" > ${KILL_SIGNAL_FILE}
		fi
	STOPEOF
	chmod +x "${STOP_POST_SCRIPT}"

	# Write the systemd user unit. All restart state (other than
	# the kill signal file above) is managed by the agent binary
	# itself.
	cat >"${HOME}/.config/systemd/user/coder-agent.service" <<-UNITEOF
		[Unit]
		Description=Coder Agent
		Documentation=https://coder.com/docs
		StartLimitIntervalSec=10
		StartLimitBurst=3

		[Service]
		Type=exec
		EnvironmentFile=${ENV_FILE}
		CacheDirectory=coder
		KillSignal=SIGINT
		KillMode=mixed
		ExecStart=${BINARY_ABS_PATH} agent --no-reap
		ExecStopPost=${STOP_POST_SCRIPT}
		Restart=on-failure
		RestartSec=5
		TimeoutStopSec=90

		[Install]
		WantedBy=default.target
	UNITEOF

	# Set XDG_RUNTIME_DIR for systemctl --user to work outside of a
	# login session (e.g. when run via cloud-init or sudo).
	XDG_RUNTIME_DIR="/run/user/$(id -u)"
	export XDG_RUNTIME_DIR

	# Reload, enable, and start the unit.
	systemctl --user daemon-reload
	systemctl --user enable coder-agent.service
	systemctl --user start coder-agent.service

	echo "=== Coder agent started via systemd user unit ==="
	echo "    Logs: journalctl --user -u coder-agent.service -f"

	# Exit cleanly. The agent is now managed by systemd.
	exit 0
fi

# Default: run the agent directly (original behavior).
exec ./$BINARY_NAME agent
