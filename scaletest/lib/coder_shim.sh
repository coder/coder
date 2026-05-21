#!/usr/bin/env bash

# This is a shim for easily executing Coder commands against a loadtest cluster
# without having to overwrite your own session/URL
PROJECT_ROOT="$(git rev-parse --show-toplevel)"
# shellcheck source=scripts/lib.sh
source "${PROJECT_ROOT}/scripts/lib.sh"
CONFIG_DIR="${PROJECT_ROOT}/scaletest/.coderv2"
CODER_BIN="${CONFIG_DIR}/coder"
DRY_RUN="${DRY_RUN:-0}"
maybedryrun "$DRY_RUN" exec "${CODER_BIN}" --global-config "${CONFIG_DIR}" "$@"
