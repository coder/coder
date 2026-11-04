#!/usr/bin/env bash
# Prepare a private image and configuration before the agent starts.
set -euo pipefail
umask 077

root=${WORKLOAD_HOST_DIR:-/var/lib/coder-workload-host}
project=${WORKLOAD_PROJECT_DIR:-/opt/workloads/project}
docker=(docker --host=unix:///var/run/docker.sock)

fail() {
	printf '%s\n' "$1" >&2
	exit 1
}

# Keep these files out of both Terraform state and Docker build contexts.
write_private() {
	local path=$1 temp
	temp=$(mktemp "$root/.prepare.XXXXXX")
	cat >"$temp"
	mv -f -- "$temp" "$path"
}

workspace=${WORKLOAD_WORKSPACE_ID:?WORKLOAD_WORKSPACE_ID is required}
cpu=${WORKLOAD_CPUS:?WORKLOAD_CPUS is required}
memory_mib=${WORKLOAD_MEMORY_MIB:?WORKLOAD_MEMORY_MIB is required}
uuid_pattern='^[[:xdigit:]]{8}-[[:xdigit:]]{4}-[[:xdigit:]]{4}-[[:xdigit:]]{4}-[[:xdigit:]]{12}$'
[[ "$workspace" =~ $uuid_pattern ]] || fail 'Workspace ID must be a UUID.'
[[ "$cpu" =~ ^[1-9][0-9]{0,3}$ && "$memory_mib" =~ ^[1-9][0-9]{0,6}$ ]] || fail 'Capacity must use positive whole numbers.'
((cpu <= 1024 && memory_mib >= 80 && memory_mib <= 1048576)) || fail 'Capacity must be 1-1024 CPUs and 80-1048576 MiB.'
memory=$((memory_mib * 1024 * 1024))
recipe=$(sha256sum "$project/Dockerfile")
recipe=${recipe%% *}
identity=$(jq -cn --arg workspace "$workspace" --arg recipe "$recipe" --argjson cpu "$cpu" --argjson memory "$memory" \
	'{workspace_id: $workspace, recipe: $recipe, cpu: $cpu, memory: $memory}')
mkdir -p "$root"
chmod 700 "$root"
manifest="$root/prepared.json"

# Even stopped containers can represent interrupted, unobserved execution.
survivors=$("${docker[@]}" ps --all --quiet)
[[ -z "$survivors" ]] || fail 'Surviving containers require operator reconciliation; refusing preparation.'

if [[ -f "$manifest" ]]; then
	jq -e --argjson identity "$identity" '.identity == $identity' "$manifest" >/dev/null ||
		fail 'Prepared host configuration changed or is invalid; drain and create a new host workspace.'
	config=$(jq -ce '.config' "$manifest")
	image_id=$(jq -er '.image' <<<"$config")
	# Never rebuild a lost image or follow a mutable tag on restart.
	observed=$("${docker[@]}" image inspect "$image_id" --format '{{.Id}}')
	[[ "$observed" == "$image_id" ]] || fail 'Prepared image identity changed.'
else
	for name in state config.json registration.json; do
		[[ ! -e "$root/$name" ]] || fail 'Prepared manifest is missing but private state exists; refusing to reset it.'
	done
	image_tag=coder-workload-demo:coder
	"${docker[@]}" build --network=host --tag "$image_tag" "$project"
	image=$("${docker[@]}" image inspect "$image_tag" --format '{{json .}}')
	image_id=$(jq -er '.Id' <<<"$image")
	revision=$(jq -er '.Config.Labels["dev.coder.workload.revision"]' <<<"$image")
	repository=$(jq -er '.Config.Labels["dev.coder.workload.repository"]' <<<"$image")
	[[ "$repository" == https://github.com/coder/coder.git && "$revision" =~ ^[a-f0-9]{40}$ && "$image_id" =~ ^sha256:[a-f0-9]{64}$ ]] ||
		fail 'Prepared image must identify the Coder repository, its exact revision, and content ID.'
	token=$(openssl rand -hex 32)
	config=$(jq -cn --arg token "$token" --arg state "$root/state" --arg image "$image_id" --arg revision "$revision" \
		--argjson cpu "$cpu" --argjson memory "$memory" \
		'{token: $token, state_dir: $state, image: $image, workdir: "/work/coder", shell: "/bin/bash",
		  repositories: {"/work/coder": $revision}, cpu: $cpu, memory_bytes: $memory,
		  docker_host: "unix:///var/run/docker.sock"}')
	prepared=$(jq -cn --argjson identity "$identity" --argjson config "$config" '{identity: $identity, config: $config}')
	write_private "$manifest" <<<"$prepared"
fi

write_private "$root/config.json" <<<"$config"
# The administrator adds the current build's agent ID from the Coder API.
# Terraform's agent resource ID is not the agent's database ID.
registration=$(jq -cn --argjson config "$config" '{token: $config.token}')
write_private "$root/registration.json" <<<"$registration"
