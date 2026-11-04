#!/usr/bin/env bats

setup() {
	export WORKLOAD_HOST_DIR="$BATS_TEST_TMPDIR/state"
	export WORKLOAD_PROJECT_DIR="$BATS_TEST_TMPDIR/project"
	export WORKLOAD_WORKSPACE_ID=11111111-1111-4111-8111-111111111111
	export WORKLOAD_CPUS=4
	export WORKLOAD_MEMORY_MIB=8192
	export FAKE_DOCKER_LOG="$BATS_TEST_TMPDIR/docker.log"
	mkdir -p "$BATS_TEST_TMPDIR/bin" "$WORKLOAD_PROJECT_DIR"
	install -m 755 "$BATS_TEST_DIRNAME/fake-docker.sh" "$BATS_TEST_TMPDIR/bin/docker"
	cp "$BATS_TEST_DIRNAME/../Dockerfile" "$WORKLOAD_PROJECT_DIR/Dockerfile"
	export PATH="$BATS_TEST_TMPDIR/bin:$PATH"
	prepare="$BATS_TEST_DIRNAME/../host/prepare-host.sh"
}

@test "first start builds before publishing private configuration" {
	run bash "$prepare"
	[ "$status" -eq 0 ]
	grep -Fx "build --network=host --tag coder-workload-demo:coder $WORKLOAD_PROJECT_DIR" "$FAKE_DOCKER_LOG"
	jq -e '.cpu == 4 and .memory_bytes == 8589934592 and (.token | length) == 64' "$WORKLOAD_HOST_DIR/config.json"
	jq -e '.workdir == "/work/coder" and .shell == "/bin/bash" and .repositories == {"/work/coder": "e26c1934a5f96c5209e7b054419db5f6c5c12323"}' "$WORKLOAD_HOST_DIR/config.json"
	jq -e --arg image "sha256:$(printf '%064d' 0)" '.image == $image' "$WORKLOAD_HOST_DIR/config.json"
	jq -e --arg token "$(jq -r .token "$WORKLOAD_HOST_DIR/config.json")" \
		'.token == $token and (has("agent_id") | not)' "$WORKLOAD_HOST_DIR/registration.json"
	[ "$(stat -c %a "$WORKLOAD_HOST_DIR")" = 700 ]
	for name in prepared.json config.json registration.json; do
		[ "$(stat -c %a "$WORKLOAD_HOST_DIR/$name")" = 600 ]
	done
	[[ "$output" != *"$(jq -r .token "$WORKLOAD_HOST_DIR/config.json")"* ]]
}

@test "restart reuses image and token and preserves execution state" {
	bash "$prepare"
	cp "$WORKLOAD_HOST_DIR/config.json" "$BATS_TEST_TMPDIR/original.json"
	mkdir "$WORKLOAD_HOST_DIR/state"
	printf 'retained dispatch guards' >"$WORKLOAD_HOST_DIR/state/state.json"
	run bash "$prepare"
	[ "$status" -eq 0 ]
	[ "$(grep -c '^build ' "$FAKE_DOCKER_LOG")" -eq 1 ]
	cmp "$BATS_TEST_TMPDIR/original.json" "$WORKLOAD_HOST_DIR/config.json"
	[ "$(cat "$WORKLOAD_HOST_DIR/state/state.json")" = 'retained dispatch guards' ]
	jq -e '(has("agent_id") | not)' "$WORKLOAD_HOST_DIR/registration.json"
	grep -F "image inspect sha256:" "$FAKE_DOCKER_LOG"
}

@test "build failure never publishes configuration" {
	export FAKE_BUILD_FAIL=1
	run bash "$prepare"
	[ "$status" -eq 42 ]
	[ ! -e "$WORKLOAD_HOST_DIR/config.json" ]
	[ ! -e "$WORKLOAD_HOST_DIR/prepared.json" ]
}

@test "invalid repository metadata never publishes configuration" {
	for metadata in FAKE_REPOSITORY=https://github.com/other/repo.git FAKE_REVISION=main; do
		run env "$metadata" bash "$prepare"
		[ "$status" -ne 0 ]
		[[ "$output" == *'must identify the Coder repository'* ]]
		[ ! -e "$WORKLOAD_HOST_DIR/config.json" ]
		[ ! -e "$WORKLOAD_HOST_DIR/prepared.json" ]
	done
}

@test "surviving containers prevent preparation without cleanup" {
	export FAKE_CONTAINERS=container-id
	run bash "$prepare"
	[ "$status" -ne 0 ]
	[[ "$output" == *'Surviving containers'* ]]
	[ "$(cat "$FAKE_DOCKER_LOG")" = 'ps --all --quiet' ]
	[ ! -e "$WORKLOAD_HOST_DIR/config.json" ]
}

@test "changed recipe cannot replace the prepared generation" {
	bash "$prepare"
	printf 'FROM different-image\n' >"$WORKLOAD_PROJECT_DIR/Dockerfile"
	run bash "$prepare"
	[ "$status" -ne 0 ]
	[[ "$output" == *'configuration changed'* ]]
	[ "$(grep -c '^build ' "$FAKE_DOCKER_LOG")" -eq 1 ]
}

@test "changed capacity or workspace cannot reuse prepared state" {
	bash "$prepare"
	run env WORKLOAD_CPUS=3 bash "$prepare"
	[ "$status" -ne 0 ]
	run env WORKLOAD_WORKSPACE_ID=44444444-4444-4444-8444-444444444444 bash "$prepare"
	[ "$status" -ne 0 ]
	[ "$(grep -c '^build ' "$FAKE_DOCKER_LOG")" -eq 1 ]
}

@test "missing image is not rebuilt" {
	bash "$prepare"
	export FAKE_MISSING_IMAGE=1
	run bash "$prepare"
	[ "$status" -ne 0 ]
	[ "$(grep -c '^build ' "$FAKE_DOCKER_LOG")" -eq 1 ]
}

@test "missing manifest does not reset existing private state" {
	bash "$prepare"
	rm "$WORKLOAD_HOST_DIR/prepared.json"
	run bash "$prepare"
	[ "$status" -ne 0 ]
	[[ "$output" == *'private state exists'* ]]
	[ "$(grep -c '^build ' "$FAKE_DOCKER_LOG")" -eq 1 ]
}

@test "corrupt manifest does not trigger another build" {
	bash "$prepare"
	printf 'not JSON' >"$WORKLOAD_HOST_DIR/prepared.json"
	run bash "$prepare"
	[ "$status" -ne 0 ]
	[ "$(grep -c '^build ' "$FAKE_DOCKER_LOG")" -eq 1 ]
}

@test "invalid capacity is rejected before using Docker" {
	for value in 0 1.5 -1 99999999999999999; do
		run env WORKLOAD_CPUS="$value" bash "$prepare"
		[ "$status" -ne 0 ]
	done
	run env WORKLOAD_MEMORY_MIB=32 bash "$prepare"
	[ "$status" -ne 0 ]
	[ ! -e "$FAKE_DOCKER_LOG" ]
}
