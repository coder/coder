#!/usr/bin/env bash

# This is a shim for developing and dogfooding Coder so that we don't
# overwrite an existing session in ~/.config/coderv2
set -euo pipefail

SCRIPT_DIR=$(dirname "${BASH_SOURCE[0]}")
# shellcheck disable=SC1091,SC1090
source "${SCRIPT_DIR}/lib.sh"

GOOS="$(go env GOOS)"
GOARCH="$(go env GOARCH)"
CODER_DEV_BIN="build/coder_${GOOS}_${GOARCH}"

cdroot
mkdir -p ./.coderv2
CODER_DEV_DIR="$(realpath ./.coderv2)"

if [[ ! -x "${CODER_DEV_BIN}" ]]; then
	echo "Run this command first:"
	echo "  make $CODER_DEV_BIN"
	exit 1
fi

exec "${CODER_DEV_BIN}" --global-config "${CODER_DEV_DIR}" "$@"
