#!/usr/bin/env bash
set -euo pipefail
root="$RUNNER_TEMP/depot-investigation"
sudo python3 "$DEPOT_PROBE_SCRIPTS/agent-details.py" "$root" before
sudo timeout 180 tcpdump -i any -n -tt -q 'tcp and port 443' > "$root/replay-packet-metadata.log" 2> "$root/replay-tcpdump-status.txt" &
packet_pid=$!
trap 'kill "$packet_pid" 2>/dev/null || true; sudo pkill -INT -P "$packet_pid" 2>/dev/null || true' EXIT
sleep 1
python3 "$DEPOT_PROBE_SCRIPTS/replay.py"
sleep 2
sudo python3 "$DEPOT_PROBE_SCRIPTS/agent-details.py" "$root" after
sudo chown -R "$(id -u):$(id -g)" "$root"
