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
BINARY_DIR=$(mktemp -d -t coder.XXXXXX)
BINARY_NAME=coder
BINARY_URL=${ACCESS_URL}bin/coder-darwin-${ARCH}
TROUBLESHOOTING_URL=https://coder.com/docs/admin/templates/troubleshooting#agent-connection-issues
cd "$BINARY_DIR"

# Print a hint for a failed agent download based on the curl exit code.
printdownloadhint() {
	code=$1
	hint=""
	case "${code}" in
	6)
		hint="Could not resolve the host in ${ACCESS_URL}. Check the DNS configuration of this workspace, or ask your network administrator."
		;;
	7)
		hint="Could not connect to ${ACCESS_URL}. Check that a firewall or network policy allows this workspace to reach the Coder server, and that the access URL is not a loopback address such as localhost."
		;;
	28)
		hint="Timed out connecting to ${ACCESS_URL}. A firewall may be dropping traffic from this workspace to the Coder server."
		;;
	35 | 60 | 77)
		hint="TLS verification failed for ${ACCESS_URL}. If the Coder server uses a self-signed or private certificate, trust its CA certificate on this machine."
		;;
	22)
		hint="The Coder server returned an HTTP error for ${BINARY_URL}. Check that the access URL is correct and that the server provides the agent binary for darwin-${ARCH}."
		;;
	esac
	if [ -n "${hint}" ]; then
		echo "hint: ${hint}"
	fi
	echo "See ${TROUBLESHOOTING_URL}"
}
# Attempt to download the coder agent.
# This could fail for a number of reasons, many of which are likely transient.
# So just keep trying!
while :; do
	curl -fsSL --compressed "${BINARY_URL}" -o "${BINARY_NAME}" && break
	status=$?
	echo "error: failed to download coder agent from ${BINARY_URL} using curl"
	echo "curl exit code: ${status}"
	printdownloadhint "${status}"
	echo "Trying again in 30 seconds..."
	sleep 30
done

if ! chmod +x $BINARY_NAME; then
	echo "Failed to make $BINARY_NAME executable"
	exit 1
fi

export CODER_AGENT_AUTH="${AUTH_TYPE}"
export CODER_AGENT_URL="${ACCESS_URL}"

output=$(./${BINARY_NAME} --version | head -n1)
if ! echo "${output}" | grep -q Coder; then
	echo >&2 "ERROR: Downloaded agent binary returned unexpected version output"
	echo >&2 "${BINARY_NAME} --version output: \"${output}\""
	echo >&2 "hint: check that ${BINARY_URL} serves the Coder agent binary for darwin-${ARCH}, and that the arch of the coder_agent resource matches this machine ($(uname -m))."
	echo >&2 "See ${TROUBLESHOOTING_URL}"
	exit 2
fi

exec ./${BINARY_NAME} agent
