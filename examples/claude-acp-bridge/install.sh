#!/bin/sh
set -eu

# Installation does not download dependencies or change Claude authentication.
source_dir=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
exec node "$source_dir/scripts/install.mjs" "$@"
