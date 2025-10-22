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
cd "$BINARY_DIR"
# Attempt to download the coder agent.
# This could fail for a number of reasons, many of which are likely transient.
# So just keep trying!
while :; do
	curl -fsSL --compressed "${BINARY_URL}" -o "${BINARY_NAME}" && break
	status=$?
	echo "error: failed to download coder agent using curl"
	echo "curl exit code: ${status}"
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
	exit 2
fi

exec ./${BINARY_NAME} agent
