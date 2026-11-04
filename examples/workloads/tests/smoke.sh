#!/usr/bin/env bash
# Opt-in bootstrap test. Creates and removes only its own Docker resources.
set -euo pipefail
umask 077

[[ ${CODER_WORKLOAD_HOST_DOCKER_TEST:-} == 1 ]] || {
	printf 'Set CODER_WORKLOAD_HOST_DOCKER_TEST=1 to run the privileged Docker test.\n' >&2
	exit 1
}
example=$(cd "$(dirname "$0")/.." && pwd)
id=$(cat /proc/sys/kernel/random/uuid)
name="coder-workload-template-test-$id"
image="$name:host"
temp=$(mktemp -d "${TMPDIR:-/tmp}/coder-workload-template.XXXXXX")
cleanup() {
	if [[ ${failed:-1} == 1 ]]; then
		docker logs "$name" >&2 || true
	fi
	docker rm --force "$name" >/dev/null 2>&1 || true
	docker volume rm "$name-docker" "$name-state" >/dev/null 2>&1 || true
	docker image rm "$image" >/dev/null 2>&1 || true
	rm -rf "$temp"
}
trap cleanup EXIT

cat >"$temp/agent-init.sh" <<'INIT'
set -euo pipefail
# Stand in for Coder's downloaded init script, after preparation has finished.
test -s "$CODER_AGENT_WORKLOAD_CONFIG"
test "$(stat -c %a "$CODER_AGENT_WORKLOAD_CONFIG")" = 600
image=$(jq -r .image "$CODER_AGENT_WORKLOAD_CONFIG")
docker image inspect "$image" >/dev/null
touch /tmp/agent-started
exec sleep infinity
INIT

docker build --tag "$image" "$example/host"
docker volume create "$name-docker" >/dev/null
docker volume create "$name-state" >/dev/null
docker create --name "$name" --privileged --init --cgroupns=private \
	--cpu-period=100000 --cpu-quota=800000 --memory=16g --memory-swap=16g \
	--mount "type=volume,source=$name-docker,target=/var/lib/docker" \
	--mount "type=volume,source=$name-state,target=/var/lib/coder-workload-host" \
	--env "WORKLOAD_WORKSPACE_ID=$id" \
	--env WORKLOAD_CPUS=4 --env WORKLOAD_MEMORY_MIB=8192 "$image" >/dev/null
docker cp "$example/Dockerfile" "$name:/opt/workloads/project/Dockerfile"
docker cp "$temp/agent-init.sh" "$name:/opt/workloads/agent-init.sh"

wait_for_agent() {
	# Poll an observable condition with a deadline, not a fixed startup delay.
	# A cold Coder toolchain and dependency build can take tens of minutes.
	local deadline=$((SECONDS + 3600))
	until docker exec "$name" test -f /tmp/agent-started 2>/dev/null; do
		[[ $(docker inspect --format '{{.State.Running}}' "$name") == true ]]
		((SECONDS < deadline))
		sleep 1
	done
}

docker start "$name" >/dev/null
wait_for_agent
docker exec "$name" cat /var/lib/coder-workload-host/config.json >"$temp/config.json"
prepared_image=$(jq -r .image "$temp/config.json")
docker exec "$name" docker run --rm --network=bridge --cap-drop=ALL --security-opt=no-new-privileges \
	--user=1000:1000 --cpus=4 --memory=8g --memory-swap=8g --pids-limit=256 "$prepared_image" \
	bash -euo pipefail -c '
		test "$(id -u)" = 1000
		test "$(git rev-parse HEAD)" = e26c1934a5f96c5209e7b054419db5f6c5c12323
		test "$(cat /sys/fs/cgroup/cpu.max)" = "400000 100000"
		test "$(cat /sys/fs/cgroup/memory.max)" = 8589934592
		test "$(cat /sys/fs/cgroup/memory.swap.max)" = 0
		test "$(cat /sys/fs/cgroup/pids.max)" = 256
		test ! -e /var/run/docker.sock
		GOPROXY=off GOTOOLCHAIN=local GOMAXPROCS=4 go test -count=1 -p=4 ./coderd/util/ptr ./coderd/util/slice
		pnpm --dir site test src/utils/fileSize.test.ts --maxWorkers=2
	'

# A clean stop must preserve the exact generation and supervisor credential.
docker exec "$name" rm /tmp/agent-started
docker stop --time 30 "$name" >/dev/null
docker start "$name" >/dev/null
wait_for_agent
docker exec "$name" cat /var/lib/coder-workload-host/config.json >"$temp/restarted.json"
cmp "$temp/config.json" "$temp/restarted.json"

# An unobserved container must prevent the next agent from starting.
docker exec "$name" docker create --name surviving-command "$prepared_image" true >/dev/null
docker exec "$name" rm /tmp/agent-started
docker stop --time 30 "$name" >/dev/null
docker start "$name" >/dev/null
exit_code=$(timeout 90 docker wait "$name")
[[ "$exit_code" != 0 ]]
docker cp "$name:/tmp/agent-started" "$temp/unexpected" 2>/dev/null && exit 1
failed=0
printf 'PASS: image preparation, project tests, cgroup limits, clean restart, and surviving-container fencing.\n'
