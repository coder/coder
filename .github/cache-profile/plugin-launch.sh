#!/usr/bin/env bash
set -euo pipefail
args=()
for arg in "$@"; do
  if [[ "$arg" == --cache-dir=* && -n "${CACHE_PROFILE_LOCAL_DIR:-}" ]]; then
    args+=("--cache-dir=$CACHE_PROFILE_LOCAL_DIR")
  else
    args+=("$arg")
  fi
done
if [[ -n "${CACHE_PROFILE_CONCURRENCY:-}" ]]; then
  args+=("-c=$CACHE_PROFILE_CONCURRENCY")
fi
exec "$CACHE_PROFILE_PLUGIN" "${args[@]}"
