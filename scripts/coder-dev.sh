#!/usr/bin/env bash

# This is a shim for developing and dogfooding Coder so that we don't
# overwrite an existing session in ~/.config/coderv2
set -euo pipefail

SCRIPT_DIR=$(dirname "${BASH_SOURCE[0]}")
# shellcheck disable=SC1091,SC1090
source "${SCRIPT_DIR}/lib.sh"

GOOS="$(go env GOOS)"
GOARCH="$(go env GOARCH)"
DEBUG_DELVE="${DEBUG_DELVE:-0}"
BINARY_TYPE=coder-slim
if [[ ${1:-} == server ]]; then
	BINARY_TYPE=coder
fi
if [[ ${1:-} == wsproxy ]] && [[ ${2:-} == server ]]; then
	BINARY_TYPE=coder
fi
if [[ ${1:-} == exp ]] && [[ ${2:-} == scaletest ]]; then
	BINARY_TYPE=coder
fi
if [[ ${1:-} == provisionerd ]]; then
	BINARY_TYPE=coder
fi
RELATIVE_BINARY_PATH="build/${BINARY_TYPE}_${GOOS}_${GOARCH}"

# To preserve the CWD when running the binary, we need to use pushd and popd to
# get absolute paths to everything.
pushd "$PROJECT_ROOT"
mkdir -p ./.coderv2
CODER_DEV_BIN="$(realpath "$RELATIVE_BINARY_PATH")"
CODER_DEV_DIR="$(realpath ./.coderv2)"
CODER_DELVE_DEBUG_BIN=$(realpath "./build/coder_debug_${GOOS}_${GOARCH}")
popd

case $BINARY_TYPE in
coder-slim)
	# Ensure the coder slim binary is always up-to-date with local
	# changes, this simplifies usage of this script for development.
	# NOTE: we send all output of `make` to /dev/null so that we do not break
	# scripts that read the output of this command.
	if [[ -t 1 ]]; then
		make -j "${RELATIVE_BINARY_PATH}"
	else
		make -j "${RELATIVE_BINARY_PATH}" >/dev/null 2>&1
	fi
	;;
coder)
	if [[ ! -x "${CODER_DEV_BIN}" ]]; then
		# A feature requiring the full binary was requested and the
		# binary is missing, normally it's built by `develop.sh`, but
		# it's an expensive operation, so we require manual action here.
		echo "Running \"coder $1\" requires the full binary, please run \"develop.sh\" first or build the binary manually:" 1>&2
		echo "  make $RELATIVE_BINARY_PATH" 1>&2
		exit 1
	fi
	;;
*)
	echo "Unknown binary type: $BINARY_TYPE"
	exit 1
	;;
esac

runcmd=("${CODER_DEV_BIN}")
if [[ "${DEBUG_DELVE}" == 1 ]]; then
	set -x
	build_flags=(
		--os "$GOOS"
		--arch "$GOARCH"
		--output "$CODER_DELVE_DEBUG_BIN"
		--debug
	)
	if [[ "$BINARY_TYPE" == "coder-slim" ]]; then
		build_flags+=(--slim)
	fi
	# All the prerequisites should be built above when we refreshed the regular
	# binary, so we can just build the debug binary here without having to worry
	# about/use the makefile.
	./scripts/build_go.sh "${build_flags[@]}"
	runcmd=(dlv exec --headless --continue --listen 127.0.0.1:12345 --accept-multiclient "$CODER_DELVE_DEBUG_BIN" --)
fi

exec "${runcmd[@]}" --global-config "${CODER_DEV_DIR}" "$@"
