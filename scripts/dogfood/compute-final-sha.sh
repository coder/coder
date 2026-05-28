#!/usr/bin/env bash
# Deterministic 12-char content hash of (base inputs + mise inputs) for
# a distro. Used as the primary tag for the dogfood image produced by
# `mise oci build`, so re-running CI on an unchanged commit reuses the
# previous tag. Same cache-key (not strict content address) semantics
# as `compute-base-sha.sh`.
set -euo pipefail

# 12 hex chars; see comment in compute-base-sha.sh.
HASH_LEN=12

distro="${1:?usage: $0 <22.04|26.04>}"

repo_root="$(git rev-parse --show-toplevel)"
cd "$repo_root"

base_sha="$("$repo_root/scripts/dogfood/compute-base-sha.sh" "$distro")"
mise_hash="$(sha256sum mise.toml mise.lock | sha256sum | cut -c"1-$HASH_LEN")"

printf '%s\n' "$base_sha-$mise_hash" | sha256sum | cut -c"1-$HASH_LEN"
