#!/usr/bin/env sh
set -eux pipefail
trap 'echo === Agent script exited with non-zero code. Sleeping 24h to preserve logs... && sleep 86400' EXIT
BINARY_DIR=$(mktemp -d -t coder.XXXXXX)
BINARY_NAME=coder
BINARY_URL=${ACCESS_URL}bin/coder-linux-${ARCH}
cd "$BINARY_DIR"
# In the below invocations, we sleep for 30 seconds before exiting.
# This is because some providers (e.g. kreuzwerker/docker) will
# automatically remove a Docker container that exits within 15
# seconds, making troubleshooting a failed workspace build
# extremely difficult.
if command -v curl >/dev/null 2>&1; then
	curl -fsSL --compressed "${BINARY_URL}" -o "${BINARY_NAME}" || (
		status=$?
		echo "error: failed to download coder agent using curl"
		sleep 30
		exit $status
	)
elif command -v wget >/dev/null 2>&1; then
	wget -q "${BINARY_URL}" -O "${BINARY_NAME}" || (
		status=$?
		echo "error: failed to download coder agent using wget"
		sleep 30
		exit $status
	)
elif command -v busybox >/dev/null 2>&1; then
	busybox wget -q "${BINARY_URL}" -O "${BINARY_NAME}" || (
		status=$?
		echo "error: failed to download coder agent using busybox wget"
		sleep 30
		exit $status
	)
else
	echo "error: no download tool found, please install curl, wget or busybox wget"
	exit 1
fi
chmod +x $BINARY_NAME || (
	echo "Failed to make $BINARY_NAME executable" && sleep 30 && exit 1
)
export CODER_AGENT_AUTH="${AUTH_TYPE}"
export CODER_AGENT_URL="${ACCESS_URL}"
exec ./$BINARY_NAME agent || (
	echo "Failed to exec ${BINARY_NAME}"
	sleep 30
	exit 126
)
