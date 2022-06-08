#!/usr/bin/env bash

# This script is meant to be sourced by other scripts. To source this script:
#     source "$(dirname "${BASH_SOURCE[0]}")/lib.sh"

set -euo pipefail

SCRIPT_DIR=$(dirname "${BASH_SOURCE[0]}")
PROJECT_ROOT=$(cd "$SCRIPT_DIR" && git rev-parse --show-toplevel)

# pushd is a silent alternative to the real pushd shell command.
pushd() {
    command pushd "$@" >/dev/null
}

# popd is a silent alternative to the real popd shell command.
# shellcheck disable=SC2120
popd() {
    command popd "$@" >/dev/null
}

# cdself changes directory to the directory of the current script. This should
# not be used in scripts that may be sourced by other scripts.
cdself() {
    cd "$SCRIPT_DIR" || error "Could not change directory to '$SCRIPT_DIR'"
}

# cdroot changes directory to the root of the repository.
cdroot() {
    cd "$PROJECT_ROOT" || error "Could not change directory to '$PROJECT_ROOT'"
}

# execrelative can be used to execute scripts as if you were in the parent
# directory of the current script. This should not be used in scripts that may
# be sourced by other scripts.
execrelative() {
    pushd "$SCRIPT_DIR" || error "Could not change directory to '$SCRIPT_DIR'"
    "$@"
    popd
}

# realpath returns an absolute path to the given relative path. It will fail if
# the parent directory of the path does not exist. Make sure you are in the
# expected directory before running this to avoid errors.
#
# GNU realpath relies on coreutils, which are not installed or the default on
# Macs out of the box, so we have this mostly working bash alternative instead.
#
# Taken from https://stackoverflow.com/a/3915420 (CC-BY-SA 4.0)
realpath() {
    dir="$(dirname "$1")"
    base="$(basename "$1")"
    if [[ ! -d "$dir" ]]; then
        error "Could not change directory to '$dir': directory does not exist"
    fi
    echo "$(
        cd "$dir" || error "Could not change directory to '$dir'"
        pwd -P
    )"/"$base"
}

error() {
    echo "ERROR: $*" 1>&2
    exit 1
}
