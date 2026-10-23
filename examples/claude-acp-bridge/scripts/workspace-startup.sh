#!/bin/sh
set -eu

# Run once from a workspace startup script, not from each SSH connection.
# Installation must already have placed the launcher at this path.
: "${CODER_ACP_BIN:=$HOME/.local/bin/coder-acp}"
exec "$CODER_ACP_BIN" serve --detach "$@"
