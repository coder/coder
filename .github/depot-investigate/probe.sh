#!/usr/bin/env bash
set -euo pipefail
root="$RUNNER_TEMP/depot-investigation"
mkdir -p "$root" "$GOTMPDIR"
python3 "$DEPOT_PROBE_SCRIPTS/inspect-agent.py"
export DEPOT_ORIGINAL_GOCACHEPROG="$GOCACHEPROG"
export GOCACHEPROG="python3 $DEPOT_PROBE_SCRIPTS/protocol-proxy.py"
# Only this ephemeral job's temporary directories are changed.
if ! mountpoint -q "$GOTMPDIR"; then sudo mount -t tmpfs -o size=16g tmpfs "$GOTMPDIR"; fi
agent_pid=$(python3 -c 'import json,sys;print(json.load(open(sys.argv[1]))["agents"][0]["pid"])' "$root/agent.json")
sudo timeout 900 strace -f -qq -e trace=connect -s 0 -p "$agent_pid" -o "$root/agent-connect.log" 2> "$root/strace-status.txt" &
trace_pid=$!
(sudo timeout 900 tcpdump -i any -n -tt -q 'tcp and (port 443 or port 980)' 2> "$root/tcpdump-status.txt" > "$root/packet-metadata.log") &
packet_pid=$!
(sleep 2; while true; do date -u +%FT%TZ; sudo ss -Hntp; sleep 2; done) > "$root/connections.log" &
ss_pid=$!
trap 'kill "$trace_pid" "$packet_pid" "$ss_pid" 2>/dev/null || true; sudo pkill -INT -P "$packet_pid" 2>/dev/null || true' EXIT
for phase in cold-plugin repeat-cold-plugin; do
 export DEPOT_PROBE_PHASE_DIR="$root/$phase"
 mkdir -p "$DEPOT_PROBE_PHASE_DIR"
 export GOCACHE="$RUNNER_TEMP/depot-probe-cache-$phase"
 mkdir -p "$GOCACHE"
 find "$GOCACHE" -type f | wc -l > "$DEPOT_PROBE_PHASE_DIR/files-before.txt"
 date -u +%FT%TZ > "$DEPOT_PROBE_PHASE_DIR/start.txt"
 vmstat -wt 1 > "$DEPOT_PROBE_PHASE_DIR/vmstat.log" &
 sampler=$!
 GIT_CONFIG_GLOBAL=/dev/null GIT_CONFIG_SYSTEM=/dev/null /usr/bin/time -v -o "$DEPOT_PROBE_PHASE_DIR/time.txt" \
 go test -debug-actiongraph="$DEPOT_PROBE_PHASE_DIR/actiongraph.json" -tags=testsmallbatch -timeout=25m -p=8 -parallel=8 -count=1 -run='^$' -exec=/bin/true \
 ./coderd ./enterprise/coderd ./cli ./... > "$DEPOT_PROBE_PHASE_DIR/build.log" 2>&1
 kill "$sampler" 2>/dev/null || true
 date -u +%FT%TZ > "$DEPOT_PROBE_PHASE_DIR/end.txt"
 cat "$DEPOT_PROBE_PHASE_DIR/time.txt"
 python3 "$DEPOT_PROBE_SCRIPTS/summarize.py" "$DEPOT_PROBE_PHASE_DIR"
done
python3 "$DEPOT_PROBE_SCRIPTS/http-probe.py"
