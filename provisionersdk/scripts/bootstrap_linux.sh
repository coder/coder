
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

# If SUPERVISED is set to "true", install and run the agent as a systemd user
# unit instead of exec'ing it directly. This provides automatic restarts,
# proper lifecycle management, and journal logging.
if [ "${SUPERVISED:-false}" = "true" ]; then
	# Remove the waitonexit trap since systemd manages the process lifecycle.
	trap - EXIT

	BINARY_ABS_PATH="$(pwd)/${BINARY_NAME}"

	# Ensure the systemd user unit directory exists.
	mkdir -p "${HOME}/.config/systemd/user"

	# Write the environment file so the unit picks up the agent token and URL.
	# This avoids baking secrets into the unit file itself.
	ENV_FILE="${HOME}/.config/coder-agent.env"
	cat > "${ENV_FILE}" <<-ENVEOF
	CODER_AGENT_AUTH=${CODER_AGENT_AUTH}
	CODER_AGENT_URL=${CODER_AGENT_URL}
	CODER_AGENT_TOKEN=${CODER_AGENT_TOKEN}
	ENVEOF
	chmod 600 "${ENV_FILE}"

	# Write the systemd user unit.
	cat > "${HOME}/.config/systemd/user/coder-agent.service" <<-UNITEOF
	[Unit]
	Description=Coder Agent
	Documentation=https://coder.com/docs
	StartLimitIntervalSec=10
	StartLimitBurst=3

	[Service]
	Type=notify
	EnvironmentFile=${ENV_FILE}
	CacheDirectory=coder
	KillSignal=SIGINT
	KillMode=mixed
	ExecStart=${BINARY_ABS_PATH} agent --no-reap
	Restart=on-failure
	RestartSec=5
	TimeoutStopSec=90

	[Install]
	WantedBy=default.target
	UNITEOF

	# Enable lingering so the user manager (and this unit) starts at boot
	# and persists without an active login session.
	# This requires either:
	#   1. The user has permission (polkit policy or root pre-enabled it), OR
	#   2. It was already enabled via cloud-init (preferred, see template changes).
	# We attempt it here but don't fail if it's denied - the template's
	# cloud-init should have already handled it.
	loginctl enable-linger "$(whoami)" 2>/dev/null || true

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
