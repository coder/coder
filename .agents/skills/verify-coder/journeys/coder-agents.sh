#!/usr/bin/env bash
# Journey for features/coder-agents.md: the admin adds an OpenAI-compatible
# provider that points at a local llm-mock, adds a default model, sends a chat
# message from /agents, and the UI and the API show the mock's reply.
# Usage: coder-agents.sh   (prints one line per check and a count line)
set -euo pipefail
scripts="$(dirname "${BASH_SOURCE[0]}")/../scripts"
# shellcheck source=.agents/skills/verify-coder/scripts/lib.sh
source "$scripts/lib.sh"
vc_load_run
"$scripts/doctor.sh" >/dev/null || vc_fail "doctor failed, run $scripts/doctor.sh"

out="$VC_EVIDENCE/$(date -u +%Y%m%dT%H%M%SZ)-coder-agents"
mkdir -p "$out"
VC_CHECK_LOG="$out/steps.log"
VC_RESULT_FILE="$out/result.txt"
vc_on_fail() {
	vc_capture_failure "$out" "$1"
}
stamp="$(date +%s)"
provider_name="vc-mock-$stamp"
provider_display="Verify mock $stamp"
model_display="Verify mock model $stamp"
message="verify chat $stamp"
reply="This is a mock response from OpenAI."

mock_port=19460
while vc_port_busy "$mock_port"; do
	mock_port=$((mock_port + 1))
done
mock_pid_file="$VC_RUN/llm-mock-$stamp.pid"
"$(vc_coder_bin)" exp scaletest llm-mock --address "127.0.0.1:$mock_port" >"$out/llm-mock.log" 2>&1 </dev/null &
echo "$!" >"$mock_pid_file"
stop_mock() {
	if [[ -f "$mock_pid_file" ]]; then
		kill "$(cat "$mock_pid_file")" 2>/dev/null || true
		rm -f "$mock_pid_file"
	fi
}
trap stop_mock EXIT

mock_ready() {
	for _ in $(seq 30); do
		if grep -q 'OpenAI endpoint' "$out/llm-mock.log"; then
			echo "llm-mock serves http://127.0.0.1:$mock_port/v1"
			return 0
		fi
		sleep 1
	done
	echo "llm-mock did not start, see $out/llm-mock.log"
	return 1
}

add_provider() {
	vc_browser_login
	vc_ab open "$VC_WEB_URL/ai/settings/providers" >/dev/null
	vc_ab wait --fn 'document.title === "AI Providers - Coder"' >/dev/null
	vc_ab find role button click --name "Add provider" >/dev/null
	vc_ab find role menuitem click --name "OpenAI-compatible" >/dev/null
	vc_ab wait --url "**/ai/settings/providers/add?type=openai-compat" >/dev/null
	vc_ab wait 'input[name=apiKey]' >/dev/null
	vc_ab fill 'input[name=name]' "$provider_name" >/dev/null
	vc_ab fill 'input[name=displayName]' "$provider_display" >/dev/null
	vc_ab fill 'input[name=baseUrl]' "http://127.0.0.1:$mock_port/v1" >/dev/null
	vc_ab screenshot "$out/01-provider-form.png" >/dev/null
	vc_ab fill 'input[name=apiKey]' "mock-key-not-a-secret" >/dev/null
	vc_ab find role button click --name "Add provider" >/dev/null
	vc_ab wait --text "Provider \"$provider_display\" added." >/dev/null
	vc_api /api/v2/ai/providers | jq --arg n "$provider_name" '[.[] | select(.name == $n) | {name, type, display_name, base_url, enabled}]' >"$out/02-api-provider.json"
	jq -e --arg u "http://127.0.0.1:$mock_port/v1" 'length == 1 and .[0].type == "openai-compat" and .[0].base_url == $u' "$out/02-api-provider.json" >/dev/null
	echo "toast shown, GET /api/v2/ai/providers has $provider_name at the mock URL"
}

add_model() {
	local org had_default
	org="$(vc_api /api/v2/organizations/default | jq -r '.id')"
	had_default="$(vc_api "/api/v2/organizations/$org/chats/models" | jq '[.models[] | select(.is_default)] | length')"
	vc_ab open "$VC_WEB_URL/ai/settings/models" >/dev/null
	vc_ab wait --fn 'document.title === "Models - AI Settings - Coder"' >/dev/null
	vc_ab find role button click --name "Add model" >/dev/null
	vc_ab find role menuitem click --name "$provider_display" >/dev/null
	vc_ab wait --url "**/ai/settings/models/add*" >/dev/null
	vc_ab wait '#contextLimit' >/dev/null
	vc_ab fill '#model' "mock-1" >/dev/null
	vc_ab fill '#displayName' "$model_display" >/dev/null
	vc_ab fill '#contextLimit' "128000" >/dev/null
	vc_ab find label "Set as Coder Agents default model" check >/dev/null
	vc_ab screenshot "$out/03-model-form.png" >/dev/null
	vc_ab find role button click --name "Add Model" >/dev/null
	# With a default already set, saving asks "Replace default model" first.
	if ((had_default > 0)); then
		vc_ab wait --text "Replace default model" >/dev/null
		vc_ab find role button click --name "Confirm" >/dev/null
	fi
	vc_ab wait --text "Model \"$model_display\" added." >/dev/null
	vc_api "/api/v2/organizations/$org/chats/models" | jq --arg d "$model_display" '[.models[] | select(.display_name == $d) | {id, display_name, model, is_default}]' >"$out/04-api-model.json"
	jq -e 'length == 1 and .[0].is_default == true' "$out/04-api-model.json" >/dev/null
	echo "toast shown, the model is the Coder Agents default"
}

chat() {
	vc_ab open "$VC_WEB_URL/agents" >/dev/null
	vc_ab wait --load networkidle >/dev/null
	vc_ab snapshot -i >"$out/05-agents-empty.snapshot.txt"
	grep -qF "combobox \"$model_display\"" "$out/05-agents-empty.snapshot.txt" || {
		echo "the composer does not select $model_display"
		return 1
	}
	vc_ab find label "Chat message" fill "$message" >/dev/null
	vc_ab find role button click --name "Send" >/dev/null
	vc_ab wait --url "**/agents/*-*" >/dev/null
	vc_ab wait --text "$reply" >/dev/null
	vc_ab screenshot "$out/06-chat-reply.png" >/dev/null
	vc_ab get url >"$out/06-chat-url.txt"
	echo "the reply streamed into $(cat "$out/06-chat-url.txt")"
}

chat_api() {
	local id status=""
	id="$(sed 's#.*/agents/##; s#[?].*##' "$out/06-chat-url.txt")"
	for _ in $(seq 30); do
		status="$(vc_api "/api/v2/chats/$id" | jq -r '.status')"
		if [[ "$status" == waiting ]]; then
			break
		fi
		sleep 1
	done
	[[ "$status" == waiting ]] || {
		echo "chat $id status is $status"
		return 1
	}
	vc_api "/api/v2/chats/$id/messages" |
		jq 'if type == "array" then . else .messages end | map({role, text: ([.content[]? | select(.type == "text") | .text] | join(""))})' >"$out/07-api-messages.json"
	jq -e --arg m "$message" --arg r "$reply" \
		'any(.[]; .role == "user" and .text == $m) and any(.[]; .role == "assistant" and .text == $r)' \
		"$out/07-api-messages.json" >/dev/null
	echo "chat $id is waiting with the user message and the assistant reply"
}

mock_stops() {
	stop_mock
	sleep 1
	if vc_port_busy "$mock_port"; then
		echo "port $mock_port still listens"
		return 1
	fi
	echo "llm-mock stopped, port $mock_port is free"
}

vc_check mock mock_ready
vc_check provider add_provider
vc_check model add_model
vc_check chat chat
vc_check chat-api chat_api
vc_check mock-stop mock_stops
echo "coder-agents: $VC_PASS/$VC_TOTAL checks passed, evidence in $out" | tee -a "$out/result.txt"
((VC_PASS == VC_TOTAL))
