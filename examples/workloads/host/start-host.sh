#!/usr/bin/env bash
set -euo pipefail
umask 077

export DOCKER_HOST=unix:///var/run/docker.sock
export CODER_AGENT_WORKLOAD_CONFIG=/var/lib/coder-workload-host/config.json
unset DOCKER_TLS_VERIFY DOCKER_CERT_PATH

docker_pid=
agent_pid=
cleanup() {
	trap - EXIT INT TERM
	if [[ -n "$agent_pid" ]]; then
		kill -TERM "$agent_pid" 2>/dev/null || true
		wait "$agent_pid" || true
	fi
	if [[ -n "$docker_pid" ]]; then
		kill -TERM "$docker_pid" 2>/dev/null || true
		wait "$docker_pid" || true
	fi
}
trap cleanup EXIT
trap 'exit 130' INT
trap 'exit 143' TERM

# The daemon is private to this workspace. Workloads use Docker's default bridge;
# firewall policy is managed outside the workload supervisor.
dockerd-entrypoint.sh dockerd --host="$DOCKER_HOST" --storage-driver=overlay2 &
docker_pid=$!
for ((attempt = 0; attempt < 60; attempt++)); do
	if docker info >/dev/null 2>&1; then
		break
	fi
	if ! kill -0 "$docker_pid" 2>/dev/null; then
		wait "$docker_pid"
		exit 1
	fi
	sleep 1
done
docker info >/dev/null

bash /opt/workloads/prepare-host.sh
printf 'Prepared workload image is ready; starting the Coder agent.\n'
bash /opt/workloads/agent-init.sh &
agent_pid=$!

# Failure of either process stops the host; never automatically restart an
# agent against an uncertain workload or silently lose Docker supervision.
wait -n "$docker_pid" "$agent_pid"
