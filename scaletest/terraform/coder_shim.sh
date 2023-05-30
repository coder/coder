#!/usr/bin/env bash

# This is a shim for easily executing Coder commands against a loadtest cluster
# without having to overwrite your own session/URL
SCRIPT_DIR=$(dirname "${BASH_SOURCE[0]}")
CONFIG_DIR="${SCRIPT_DIR}/.coderv2"
CODER_BIN="${CONFIG_DIR}/coder"
exec "${CODER_BIN}" --global-config "${CONFIG_DIR}" "$@"
