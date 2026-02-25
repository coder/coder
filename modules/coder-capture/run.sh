#!/bin/bash
set -euo pipefail

CAPTURE_BIN="$HOME/.coder-capture/bin/coder-capture"

# Install coder-capture if not present
if [ ! -f "$CAPTURE_BIN" ]; then
  mkdir -p "$HOME/.coder-capture/bin"
  if command -v coder-capture >/dev/null 2>&1; then
    cp "$(which coder-capture)" "$CAPTURE_BIN"
    chmod +x "$CAPTURE_BIN"
  else
    echo "WARNING: coder-capture binary not found in PATH."
    echo "Install it in your Docker image: COPY coder-capture /usr/local/bin/coder-capture"
    exit 0
  fi
fi

# Build enable command with optional flags
ENABLE_ARGS="enable"
%{ if no_trailer }
ENABLE_ARGS="$ENABLE_ARGS --no-trailer"
%{ endif }
%{ if log_dir != "" }
ENABLE_ARGS="$ENABLE_ARGS --log-dir ${log_dir}"
%{ endif }

exec "$CAPTURE_BIN" $ENABLE_ARGS
