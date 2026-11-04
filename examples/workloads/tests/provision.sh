#!/usr/bin/env bash
# Exercise the actual Terraform resources with Docker and a substitute Coder agent.
set -euo pipefail
umask 077

[[ ${CODER_WORKLOAD_HOST_DOCKER_TEST:-} == 1 ]] || {
	printf 'Set CODER_WORKLOAD_HOST_DOCKER_TEST=1 to run the privileged Terraform test.\n' >&2
	exit 1
}
example=$(cd "$(dirname "$0")/.." && pwd)
id=$(cat /proc/sys/kernel/random/uuid)
name="coder-workloads-$id"
# Use the same daemon even when the shell has a different Docker context.
export TF_VAR_docker_socket=${TF_VAR_docker_socket:-unix:///var/run/docker.sock}
docker=(docker --host "$TF_VAR_docker_socket")
temp=$(mktemp -d "${TMPDIR:-/tmp}/coder-workload-provision.XXXXXX")
cleanup() {
	local status=$? cleanup_failed=0 existing volume
	# Terraform normally destroys these. Also clean up partial test failures.
	existing=$("${docker[@]}" container ls --all --quiet --filter "name=^/$name$") || cleanup_failed=1
	if [[ -n "$existing" ]]; then
		if [[ ${failed:-1} == 1 ]]; then
			"${docker[@]}" logs "$name" >&2 || true
		fi
		"${docker[@]}" container rm --force "$name" >/dev/null || cleanup_failed=1
	fi
	for volume in "$name-docker" "$name-state"; do
		existing=$("${docker[@]}" volume ls --quiet --filter "name=^$volume$") || cleanup_failed=1
		if [[ -n "$existing" ]]; then
			"${docker[@]}" volume rm "$volume" >/dev/null || cleanup_failed=1
		fi
	done
	existing=$("${docker[@]}" image ls --quiet --filter "reference=coder-workload-host:$id") || cleanup_failed=1
	if [[ -n "$existing" ]]; then
		"${docker[@]}" image rm "coder-workload-host:$id" >/dev/null || cleanup_failed=1
	fi
	if ((cleanup_failed)); then
		printf 'Cleanup failed for %s on %s; retained test files at %s.\n' "$name" "$TF_VAR_docker_socket" "$temp" >&2
		exit 1
	fi
	rm -rf "$temp"
	exit "$status"
}
trap cleanup EXIT

cp "$example/main.tf" "$example/Dockerfile" "$example/workload-metadata.sh" "$temp/"
cp -R "$example/host" "$temp/host"
if [[ -f "$example/.terraform.lock.hcl" ]]; then
	cp "$example/.terraform.lock.hcl" "$temp/"
fi
mkdir "$temp/tests"
cat >"$temp/tests/provision.tftest.hcl" <<EOF
mock_provider "coder" {
  mock_data "coder_provisioner" {
    defaults = { arch = "$(uname -m | sed 's/x86_64/amd64/; s/aarch64/arm64/')" }
  }
  mock_data "coder_workspace" {
    defaults = { id = "$id", start_count = 1 }
  }
  mock_data "coder_workspace_owner" {
    defaults = { id = "$id" }
  }
  mock_resource "coder_agent" {
    defaults = { id = "$id", token = "test-only", init_script = "exec sleep infinity" }
  }
}

run "provision_host" {
  command = apply

  assert {
    condition = join(" ", docker_container.host[0].entrypoint) == "bash /opt/workloads/start-host.sh"
    error_message = "The host must use the bootstrap entrypoint, not the project image."
  }
}
EOF
# Do not use shared TF_DATA_DIR or local provider development overrides.
export TF_DATA_DIR="$temp/.terraform"
export TF_CLI_CONFIG_FILE=/dev/null
terraform -chdir="$temp" init -input=false -no-color
terraform -chdir="$temp" test -no-color
failed=0
printf 'PASS: the Terraform Docker provider builds and starts the host image.\n'
