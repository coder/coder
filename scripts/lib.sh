#!/usr/bin/env bash

# This script is meant to be sourced by other scripts. To source this script:
#     source "$(dirname "${BASH_SOURCE[0]}")/lib.sh"

set -euo pipefail

SCRIPT_DIR=$(dirname "${BASH_SOURCE[0]}")
PROJECT_ROOT=$(cd "$SCRIPT_DIR" && git rev-parse --show-toplevel)

cdself() {
    cd "$SCRIPT_DIR" || error "Could not change directory to '$SCRIPT_DIR'"
}

cdroot() {
    cd "$PROJECT_ROOT" || error "Could not change directory to '$PROJECT_ROOT'"
}

# Taken from https://stackoverflow.com/a/3915420 (CC-BY-SA 4.0)
# Fails if the directory doesn't exist.
realpath() {
    dir="$(dirname "$1")"
    base="$(basename "$1")"
    echo "$(
        cd "$dir" || error "Could not change directory to '$dir'"
        pwd -P
    )"/"$base"
}

error() {
    echo "ERROR: $*" 1>&2
    exit 1
}
