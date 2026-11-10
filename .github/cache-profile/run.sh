#!/usr/bin/env bash
set -euo pipefail
mode="$1"
profile_out="$CACHE_PROFILE_ROOT/$mode"
mkdir -p "$profile_out"
export CACHE_PROFILE_DIR="$profile_out"
# Every invocation uses an empty compiler-side cache. The per-job agent retains
# downloaded bundles and extracted outputs across invocations.
export GOCACHE="$RUNNER_TEMP/client-cache-$mode"
mkdir -p "$GOCACHE"
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
if [[ "$PROFILE_PROVIDER" == coder && -f "$CODER_CACHE_JOB_DIR/go-metrics.json" ]]; then
  cp "$CODER_CACHE_JOB_DIR/go-metrics.json" "$profile_out/bundle-metrics.json"
  cat "$profile_out/bundle-metrics.json"
fi
exit "$result"
