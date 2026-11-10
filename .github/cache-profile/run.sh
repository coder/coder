#!/usr/bin/env bash
set -euo pipefail
mode="$1"
profile_out="$CACHE_PROFILE_ROOT/$mode"
mkdir -p "$profile_out"
export CACHE_PROFILE_DIR="$profile_out"
case "$mode" in
  baseline|warm) ;;
  c64) export CACHE_PROFILE_CONCURRENCY=64 ;;
  ram) export CACHE_PROFILE_LOCAL_DIR="$RUNNER_TEMP/profile-cache-ram" ;;
  ram-c64) export CACHE_PROFILE_LOCAL_DIR="$RUNNER_TEMP/profile-cache-ram" CACHE_PROFILE_CONCURRENCY=64 ;;
esac
if [[ "$mode" == ram* ]]; then
  mkdir -p "$CACHE_PROFILE_LOCAL_DIR"
  if ! mountpoint -q "$CACHE_PROFILE_LOCAL_DIR"; then sudo mount -t tmpfs -o size=8g tmpfs "$CACHE_PROFILE_LOCAL_DIR"; fi
  rm -rf "$CACHE_PROFILE_LOCAL_DIR"/*
elif [[ "$PROFILE_PROVIDER" == coder && "$mode" != warm && "$mode" != warm-local ]]; then
  # Remove only this job's plugin cache, without touching its runtime token.
  [[ "$CODER_CACHE_JOB_DIR" == /tmp/coder-cache-job-* ]]
  rm -rf "$CODER_CACHE_JOB_DIR/go-plugin"
fi
mkdir -p "$GOTMPDIR"
if ! mountpoint -q "$GOTMPDIR"; then sudo mount -t tmpfs -o size=16g tmpfs "$GOTMPDIR"; fi
for sampler in vmstat iostat pidstat; do
  case "$sampler" in
    vmstat) vmstat -wt 1 > "$profile_out/vmstat.log" & ;;
    iostat) iostat -dxt 1 > "$profile_out/iostat.log" & ;;
    pidstat) pidstat -h -u -r -d -C 'go|compile|link|go-cache-plugin|coder-cache' 1 > "$profile_out/pidstat.log" & ;;
  esac
  echo "$!" >> "$profile_out/samplers.pid"
done
trap 'xargs -r kill < "$profile_out/samplers.pid" 2>/dev/null || true' EXIT
date -u +%FT%TZ > "$profile_out/start.txt"
set +e
if [[ "$mode" == baseline ]]; then
  GIT_CONFIG_GLOBAL=/dev/null GIT_CONFIG_SYSTEM=/dev/null /usr/bin/time -v -o "$profile_out/time.txt" \
    gotestsum --format standard-quiet --jsonfile "$profile_out/tests.json" \
    --packages="./coderd ./enterprise/coderd ./cli ./..." -- \
    -debug-actiongraph="$profile_out/actiongraph.json" -tags=testsmallbatch -v -timeout=25m -p=8 -parallel=8 -count=1 \
    2>&1 | tee "$profile_out/test.log"
  result=${PIPESTATUS[0]}
else
  GIT_CONFIG_GLOBAL=/dev/null GIT_CONFIG_SYSTEM=/dev/null /usr/bin/time -v -o "$profile_out/time.txt" \
    go test -debug-actiongraph="$profile_out/actiongraph.json" -tags=testsmallbatch -timeout=25m -p=8 -parallel=8 -count=1 -run='^$' -exec=/bin/true \
    ./coderd ./enterprise/coderd ./cli ./... 2>&1 | tee "$profile_out/compile.log"
  result=${PIPESTATUS[0]}
fi
set -e
date -u +%FT%TZ > "$profile_out/end.txt"
printf '%s\n' "$result" > "$profile_out/exit-code.txt"
cat "$profile_out/time.txt"
if [[ "$mode" == ram* ]]; then sudo umount "$CACHE_PROFILE_LOCAL_DIR"; fi
if [[ "$mode" == baseline-repeat && "$result" == 0 ]]; then
  xargs -r kill < "$profile_out/samplers.pid" 2>/dev/null || true
  trap - EXIT
  echo 'Profiling control: reuse the local cache populated by the preceding default retrieval run'
  bash "$PROFILE_SCRIPTS/run.sh" warm-local
fi
exit "$result"
