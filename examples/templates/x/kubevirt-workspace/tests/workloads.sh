#!/usr/bin/env bash
# Run inside the workspace through Coder, not against the runner's Docker daemon.
set -euo pipefail

fixtures=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)
proof="$HOME/kubevirt-prototype-proof"
phase=${1:?Usage: workloads.sh prepare|verify}

wait_for() {
	local deadline=$((SECONDS + 120))
	until "$@"; do
		if ((SECONDS >= deadline)); then
			echo "Timed out waiting for: $*" >&2
			return 1
		fi
		sleep 1
	done
}

http_matches() {
	local address response
	address=$(docker compose port app 8080)
	response=$(curl -fsS --max-time 5 "http://$address") || return 1
	[[ $response == "$1" ]]
}

backend_matches() {
	local response
	response=$(docker compose exec -T app wget -T 5 -qO- http://backend:8080/value) || return 1
	[[ $response == "$marker" ]]
}

case "$phase" in
prepare)
	if [[ -e $proof ]]; then
		echo "Refusing to overwrite existing proof directory: $proof" >&2
		exit 1
	fi
	umask 077
	mkdir "$proof"
	cp "$fixtures/Dockerfile" "$fixtures/compose.yaml" "$proof/"
	cd "$proof"
	marker=$(python3 -c 'import uuid; print(uuid.uuid4().hex)')
	printf '%s\n' "$marker" >marker
	printf 'COMPOSE_PROJECT_NAME=kvproof%s\nPROTOTYPE_IMAGE=coder-kv-proof:%s\n' "$marker" "$marker" >.env
	image="coder-kv-proof:$marker"
	mkdir project
	printf '%s\n' "$marker" >project/index.html
	cat /proc/sys/kernel/random/boot_id >boot-id-before

	docker info --format 'Docker server: {{.Name}} {{.ServerVersion}} {{.Driver}}'
	docker compose version
	docker buildx build --load --progress=plain \
		--build-arg "BUILD_MARKER=$marker" \
		--label "com.coder.kubevirt-prototype=$marker" -t "$image" .
	[[ $(docker run --rm "$image" cat /image-marker) == "$marker" ]]
	docker image inspect --format '{{.Id}}' "$image" >image-id-before
	echo 'PASS image build and execution'

	docker compose up -d
	wait_for http_matches "$marker"
	printf '%s\n' "$marker-edited" >project/index.html
	wait_for http_matches "$marker-edited"
	docker compose exec -T app sh -c 'printf "%s\n" "$1" > /workspace/generated.txt' sh "$marker-generated"
	[[ $(<project/generated.txt) == "$marker-generated" ]]
	echo 'PASS live bind mounts, generated files, and a published port'

	if docker run --rm -v "$proof/project:/workspace:ro" "$image" sh -c 'echo unexpected > /workspace/index.html' 2>readonly-error; then
		echo 'FAIL read-only bind mount accepted a write' >&2
		exit 1
	fi
	grep -q 'Read-only file system' readonly-error
	[[ $(<project/index.html) == "$marker-edited" ]]
	echo 'PASS read-only bind mount negative control'

	docker compose exec -T backend sh -c 'printf "%s\n" "$1" > /data/value' sh "$marker"
	wait_for backend_matches
	echo 'PASS Compose service discovery and service-to-service networking'

	docker compose stop backend
	# Prove the app container still executes commands; only its backend is gone.
	docker compose exec -T app true
	if docker compose exec -T app wget -T 5 -qO- http://backend:8080/value 2>backend-error; then
		echo 'FAIL backend request succeeded with the backend stopped' >&2
		exit 1
	fi
	grep -Eq 'bad address|Connection refused|timed out|No route to host' backend-error
	docker compose start backend
	wait_for backend_matches
	echo 'PASS stopped-backend negative control and recovery'

	agent_pid=$(systemctl show --property=MainPID --value coder-agent)
	[[ $agent_pid != 0 ]]
	sudo systemctl restart docker
	[[ $(systemctl show --property=MainPID --value coder-agent) == "$agent_pid" ]]
	wait_for http_matches "$marker-edited"
	wait_for backend_matches
	echo 'PASS Docker daemon restart without restarting the Coder agent'
	printf '%s\n' complete >prepared
	;;
verify)
	cd "$proof"
	[[ $(<prepared) == complete ]]
	marker=$(<marker)
	image="coder-kv-proof:$marker"
	[[ $(<boot-id-before) != "$(cat /proc/sys/kernel/random/boot_id)" ]]
	[[ $(<project/index.html) == "$marker-edited" ]]
	[[ $(<project/generated.txt) == "$marker-generated" ]]
	[[ $(<image-id-before) == "$(docker image inspect --format '{{.Id}}' "$image")" ]]
	wait_for http_matches "$marker-edited"
	wait_for backend_matches
	echo 'PASS new guest boot with persistent workspace files, Docker image, named volume, and running Compose services'
	printf '%s\n' complete >verified
	;;
*)
	echo "Unknown phase: $phase" >&2
	exit 2
	;;
esac
