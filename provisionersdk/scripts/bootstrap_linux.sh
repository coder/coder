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
		busybox wget -q "${BINARY_URL}" -O "${BINARY_NAME}" && break
		status=$?
	else
		echo "error: no download tool found, please install curl, wget or busybox wget"
		exit 127
	fi
	echo "error: failed to download coder agent"
	echo "       command returned: ${status}"
	echo "Trying again in 30 seconds..."
	sleep 30
done

if ! chmod +x $BINARY_NAME; then
	echo "Failed to make $BINARY_NAME executable"
	exit 1
fi

haslibcap2() {
	command -v setcap /dev/null 2>&1
	command -v capsh /dev/null 2>&1
}
printnetadminmissing() {
	echo "The root user does not have CAP_NET_ADMIN permission. " + \
		"If running in Docker, add the capability to the container for " + \
		"improved network performance."
	echo "This has security implications. See https://man7.org/linux/man-pages/man7/capabilities.7.html"
}

# Attempt to add CAP_NET_ADMIN to the agent binary. This allows us to increase
# network buffers which improves network transfer speeds.
if [ -n "${USE_CAP_NET_ADMIN:-}" ]; then
	# If running as root, we do not need to do anything.
	if [ "$(id -u)" -eq 0 ]; then
		echo "Running as root, skipping setcap"
		# Warn the user if root does not have CAP_NET_ADMIN.
		if ! capsh --has-p=CAP_NET_ADMIN; then
			printnetadminmissing
		fi

	# If not running as root, make sure we have sudo perms and the "setcap" +
	# "capsh" binaries exist.
	elif sudo -nl && haslibcap2; then
		# Make sure the root user has CAP_NET_ADMIN.
		if sudo -n capsh --has-p=CAP_NET_ADMIN; then
			sudo -n setcap CAP_NET_ADMIN=+ep ./$BINARY_NAME || true
		else
			printnetadminmissing
		fi

	# If we are not running as root, cant sudo, and "setcap" does not exist, we
	# cannot do anything.
	else
		echo "Unable to setcap agent binary. To enable improved network performance, " + \
			"give the agent passwordless sudo permissions and the \"setcap\" + \"capsh\" binaries."
		echo "This has security implications. See https://man7.org/linux/man-pages/man7/capabilities.7.html"
	fi
fi

export CODER_AGENT_AUTH="${AUTH_TYPE}"
export CODER_AGENT_URL="${ACCESS_URL}"

output=$(./${BINARY_NAME} --version | head -n1)
if ! echo "${output}" | grep -q Coder; then
	echo >&2 "ERROR: Downloaded agent binary returned unexpected version output"
	echo >&2 "${BINARY_NAME} --version output: \"${output}\""
	exit 2
fi

exec ./${BINARY_NAME} agent
