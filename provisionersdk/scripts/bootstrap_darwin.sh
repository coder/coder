#!/usr/bin/env sh
set -eux pipefail
trap "echo === Agent script exited with non-zero code. Sleeping 24h to preserve logs... && sleep 86400" EXIT
BINARY_DIR=$(mktemp -d -t coder.XXXXXX)
BINARY_NAME=coder
cd "$BINARY_DIR"
curl -fsSL --compressed "${ACCESS_URL}bin/coder-darwin-${ARCH}" -o "${BINARY_NAME}"
chmod +x $BINARY_NAME
export CODER_AGENT_AUTH="${AUTH_TYPE}"
export CODER_AGENT_URL="${ACCESS_URL}"
exec ./$BINARY_NAME agent
