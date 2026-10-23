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
TROUBLESHOOTING_URL=https://coder.com/docs/admin/templates/troubleshooting#agent-connection-issues
cd "$BINARY_DIR"

# Print a hint for a failed agent download based on the download tool and its
# exit code.
printdownloadhint() {
	tool=$1
	code=$2
	hint=""
	case "${tool}:${code}" in
	curl:6)
		hint="Could not resolve the host in ${ACCESS_URL}. Check the DNS configuration of this workspace, or ask your network administrator."
		;;
	curl:7)
		hint="Could not connect to ${ACCESS_URL}. Check that a firewall or network policy allows this workspace to reach the Coder server, and that the access URL is not a loopback address such as localhost."
		;;
	curl:28)
		hint="Timed out connecting to ${ACCESS_URL}. A firewall may be dropping traffic from this workspace to the Coder server."
		;;
	curl:35 | curl:60 | curl:77 | wget:5)
		hint="TLS verification failed for ${ACCESS_URL}. If the Coder server uses a self-signed or private certificate, add its CA certificate to this workspace image."
		;;
	curl:22 | wget:8)
		hint="The Coder server returned an HTTP error for ${BINARY_URL}. Check that the access URL is correct and that the server provides the agent binary for linux-${ARCH}."
		;;
	wget:4)
		hint="A network error occurred while connecting to ${ACCESS_URL}. Check the DNS and firewall configuration of this workspace."
		;;
	esac
	if [ -n "${hint}" ]; then
		echo "hint: ${hint}"
	fi
	echo "See ${TROUBLESHOOTING_URL}"
}

# Map the Go architecture of this script to the values that uname -m reports.
expectedmachine() {
	case "${ARCH}" in
	amd64) echo "x86_64" ;;
	arm64) echo "aarch64 arm64" ;;
	armv7) echo "armv7l armv8l" ;;
	*) echo "${ARCH}" ;;
	esac
}
# Attempt to download the coder agent.
# This could fail for a number of reasons, many of which are likely transient.
# So just keep trying!
while :; do
	# Try a number of different download tools, as we don not know what we
	# will have available.
	status=""
	tool=""
	if command -v curl >/dev/null 2>&1; then
		tool=curl
		curl -fsSL --compressed "${BINARY_URL}" -o "${BINARY_NAME}" && break
		status=$?
	elif command -v wget >/dev/null 2>&1; then
		tool=wget
		wget -q "${BINARY_URL}" -O "${BINARY_NAME}" && break
		status=$?
	elif command -v busybox >/dev/null 2>&1; then
		tool=busybox
		busybox wget -q "${BINARY_URL}" -O "${BINARY_NAME}" && break
		status=$?
	else
		echo "error: no download tool found, please install curl, wget or busybox wget"
		echo "hint: install curl in the workspace image, for example with apt-get install curl, apk add curl, or dnf install curl."
		echo "See ${TROUBLESHOOTING_URL}"
		exit 127
	fi
	echo "error: failed to download coder agent from ${BINARY_URL}"
	echo "       ${tool} returned: ${status}"
	printdownloadhint "${tool}" "${status}"
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
	machine=$(uname -m)
	if ! echo " $(expectedmachine) " | grep -q " ${machine} "; then
		echo >&2 "hint: this workspace reports architecture ${machine}, but the template requested the linux-${ARCH} agent. Set the arch of the coder_agent resource to match the workspace."
	else
		echo >&2 "hint: the download may have been replaced by a proxy or captive portal. Check that ${BINARY_URL} serves the Coder agent binary."
	fi
	echo >&2 "See ${TROUBLESHOOTING_URL}"
	exit 2
fi

exec ./${BINARY_NAME} agent
