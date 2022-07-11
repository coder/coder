#!/usr/bin/env sh
set -eux pipefail
trap 'echo === Agent script exited with non-zero code. Sleeping 24h to preserve logs... && sleep 86400' EXIT
BINARY_DIR=$(mktemp -d -t coder.XXXXXX)
BINARY_NAME=coder
BINARY_URL=${ACCESS_URL}bin/coder-linux-${ARCH}
cd "$BINARY_DIR"
if command -v curl >/dev/null 2>&1; then
	curl -fsSL --compressed "${BINARY_URL}" -o "${BINARY_NAME}"
elif command -v wget >/dev/null 2>&1; then
	wget -q "${BINARY_URL}" -O "${BINARY_NAME}"
elif command -v busybox >/dev/null 2>&1; then
	busybox wget -q "${BINARY_URL}" -O "${BINARY_NAME}"
else
	echo "error: no download tool found, please install curl, wget or busybox wget"
	exit 1
fi
chmod +x $BINARY_NAME
export CODER_AGENT_AUTH="${AUTH_TYPE}"
export CODER_AGENT_URL="${ACCESS_URL}"
exec ./$BINARY_NAME agent
