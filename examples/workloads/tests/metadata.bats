#!/usr/bin/env bats
# Bats intentionally isolates each test's environment in a subshell.
# shellcheck disable=SC2030,SC2031

setup() {
	export WORKLOAD_CONTROL_PLANE_URL=http://control-plane.test/
	export CODER_AGENT_TOKEN=private-agent-token
	export FAKE_RESPONSE='{"total":12,"running":2,"command":"must-not-appear"}'
	mkdir -p "$BATS_TEST_TMPDIR/bin"
	cat >"$BATS_TEST_TMPDIR/bin/curl" <<'SCRIPT'
#!/usr/bin/env bash
set -euo pipefail
[[ "$*" != *"$CODER_AGENT_TOKEN"* ]]
[[ "${!#}" == http://control-plane.test/api/v2/workspaceagents/me/workload-stats ]]
while (($#)); do
	if [[ "$1" == --header ]]; then
		shift
		[[ "$(cat "${1#@}")" == "Coder-Session-Token: $CODER_AGENT_TOKEN" ]]
	fi
	shift
done
printf '%s\n' "$FAKE_RESPONSE"
exit "${FAKE_EXIT:-0}"
SCRIPT
	chmod +x "$BATS_TEST_TMPDIR/bin/curl"
	export PATH="$BATS_TEST_TMPDIR/bin:$PATH"
	metadata="$BATS_TEST_DIRNAME/../workload-metadata.sh"
}

@test "metadata uses current agent authentication and prints only the selected count" {
	run bash "$metadata" total
	[ "$status" -eq 0 ]
	[ "$output" = 12 ]
	run bash "$metadata" running
	[ "$status" -eq 0 ]
	[ "$output" = 2 ]
}

@test "metadata reports zero but not a fabricated count on API failure" {
	export FAKE_RESPONSE='{"total":0,"running":0}'
	run bash "$metadata" total
	[ "$status" -eq 0 ]
	[ "$output" = 0 ]
	export FAKE_EXIT=22 FAKE_RESPONSE='private failure detail'
	run bash "$metadata" total
	[ "$status" -ne 0 ]
	[[ "$output" == *'ensure this host is registered'* ]]
	[[ "$output" != *'private failure detail'* ]]
}

@test "metadata rejects missing or invalid counters and arbitrary field selection" {
	for response in '{}' '{"total":-1}' '{"total":0.5}' '{"total":"command-text"}'; do
		export FAKE_RESPONSE="$response"
		run bash "$metadata" total
		[ "$status" -ne 0 ]
		[[ "$output" != *command-text* ]]
	done
	run bash "$metadata" command
	[ "$status" -ne 0 ]
	[[ "$output" != *must-not-appear* ]]
}
