#!/usr/bin/env bash
# Journey for features/coder-agents.md: the admin adds an OpenAI-compatible
# provider that points at a local llm-mock, adds a default model, sends a chat
# message from /agents, and the UI and the API show the mock's reply. It then
# puts the instance back: chat archived, old default model, no mock provider.
# Usage: coder-agents.sh   (prints one line per check and a count line)
set -euo pipefail
scripts="$(dirname "${BASH_SOURCE[0]}")/../scripts"
# shellcheck source=.agents/skills/verify-coder/scripts/lib.sh
source "$scripts/lib.sh"
vc_load_run
"$scripts/doctor.sh" >/dev/null || vc_fail "doctor failed, run $scripts/doctor.sh"

mkdir -p "$VC_EVIDENCE"
# mktemp keeps two runs that start in the same second apart.
out="$(mktemp -d "$VC_EVIDENCE/$(date -u +%Y%m%dT%H%M%SZ)-coder-agents-XXXX")"
chmod 755 "$out"
VC_CHECK_LOG="$out/steps.log"
VC_RESULT_FILE="$out/result.txt"
vc_on_fail() {
	vc_capture_failure "$out" "$1"
}
# The random suffix keeps two runs that start in the same second apart.
stamp="$(date +%s)-$RANDOM"
provider_name="vc-mock-$stamp"
provider_display="Verify mock $stamp"
model_display="Verify mock model $stamp"
message="verify chat $stamp"
reply="This is a mock response from OpenAI."
# A reused .coderv2 can hold a developer's own default model. The journey
# replaces it for the chat and puts it back in the restore check.
org="$(vc_api /api/v2/organizations/default | jq -r '.id')"
old_default="$(vc_api "/api/v2/organizations/$org/chats/models" | jq -r '[.models[] | select(.is_default)][0].id // empty')"

mock_port=19460
while vc_port_busy "$mock_port"; do
	mock_port=$((mock_port + 1))
done
mock_pid_file="$VC_RUN/llm-mock-$stamp.pid"
"$(vc_coder_bin)" exp scaletest llm-mock --address "127.0.0.1:$mock_port" >"$out/llm-mock.log" 2>&1 </dev/null &
echo "$!" >"$mock_pid_file"
# stop_mock signals the pid only while it still runs this checkout's coder
# binary, so a recycled pid is left alone.
stop_mock() {
	local pid
	if [[ -f "$mock_pid_file" ]]; then
		pid="$(cat "$mock_pid_file")"
		if [[ "$(readlink "/proc/$pid/exe" 2>/dev/null)" == "$(vc_coder_bin)"* ]]; then
			kill "$pid" 2>/dev/null || true
		fi
		rm -f "$mock_pid_file"
	fi
}
# An interrupted or failed run still puts the default model back and
# deletes what it added. restore is defined below and runs here unless its
# check passed.
restored=0
on_exit() {
	if ((!restored)) && declare -F restore >/dev/null; then
		(
			set -e
			restore
		) >>"$VC_CHECK_LOG" 2>&1 || true
	fi
	stop_mock
}
trap on_exit EXIT
trap 'exit 130' INT TERM

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
	vc_browser_login "$VC_ADMIN_EMAIL" "$VC_ADMIN_PASSWORD"
	vc_ab open "$VC_WEB_URL/ai/settings/providers" >/dev/null
	vc_ab wait --fn 'document.title === "AI Providers - Coder"' >/dev/null
	vc_ab find role button click --name "Add provider" >/dev/null
	vc_ab wait '[role=menu]' >/dev/null
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
	vc_ab open "$VC_WEB_URL/ai/settings/models" >/dev/null
	vc_ab wait --fn 'document.title === "Models - AI Settings - Coder"' >/dev/null
	vc_ab find role button click --name "Add model" >/dev/null
	vc_ab wait '[role=menu]' >/dev/null
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
	if [[ -n "$old_default" ]]; then
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
	vc_ab wait '[data-testid=chat-message-input]' >/dev/null
	vc_ab wait --text "$model_display" >/dev/null
	vc_ab snapshot -i >"$out/05-agents-composer.snapshot.txt"
	grep -qF "combobox \"$model_display\"" "$out/05-agents-composer.snapshot.txt" || {
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

vc_send() {
	local method="$1" path="$2" body="${3:-}"
	if [[ -n "$body" ]]; then
		vc_curl -fsS --max-time 10 -X "$method" -H 'Content-Type: application/json' --data "$body" "$VC_API_URL$path" >/dev/null
	else
		vc_curl -fsS --max-time 10 -X "$method" "$VC_API_URL$path" >/dev/null
	fi
}

# restore archives the chat, gives the default back to the model that had it
# before while the journey's model still has it, and deletes the journey's
# model and provider.
restore() {
	local model_id chat_id want_default
	if [[ -f "$out/06-chat-url.txt" ]]; then
		chat_id="$(sed 's#.*/agents/##; s#[?].*##' "$out/06-chat-url.txt")"
		# The API refuses to archive an archived chat, so a retry skips it.
		if [[ "$(vc_api "/api/v2/chats/$chat_id" | jq -r '.archived')" != true ]]; then
			vc_send PATCH "/api/v2/chats/$chat_id" '{"archived": true}'
		fi
		vc_api "/api/v2/chats/$chat_id" | jq -e '.archived == true' >/dev/null
	fi
	model_id="$(vc_api "/api/v2/organizations/$org/chats/models" | jq -r --arg d "$model_display" '[.models[] | select(.display_name == $d)][0].id // empty')"
	# Another admin can have picked a newer default meanwhile, so the old
	# default comes back only while the journey's model still has it, and a
	# newer default stays.
	want_default="$(vc_api "/api/v2/organizations/$org/chats/models" | jq -r '[.models[] | select(.is_default)][0].id // empty')"
	if [[ -n "$model_id" && "$want_default" == "$model_id" ]]; then
		want_default="$old_default"
		if [[ -n "$old_default" ]]; then
			vc_send PATCH "/api/v2/organizations/$org/chats/models/$old_default" '{"is_default": true}'
		fi
	fi
	if [[ -n "$model_id" ]]; then
		vc_send DELETE "/api/v2/organizations/$org/chats/models/$model_id"
	fi
	if [[ "$(vc_api_status "/api/v2/ai/providers/$provider_name")" == 200 ]]; then
		vc_send DELETE "/api/v2/ai/providers/$provider_name"
	fi
	vc_api "/api/v2/organizations/$org/chats/models" >"$out/08-api-models-after.json"
	jq -e --arg d "$model_display" --arg w "$want_default" \
		'([.models[] | select(.display_name == $d)] | length == 0) and ([.models[] | select(.is_default)][0].id // "") == $w' \
		"$out/08-api-models-after.json" >/dev/null || {
		echo "the model list still has the journey's model or the wrong default"
		return 1
	}
	[[ "$(vc_api_status "/api/v2/ai/providers/$provider_name")" == 404 ]] || {
		echo "provider $provider_name still exists"
		return 1
	}
	echo "chat archived, default model is ${want_default:-none}, journey model and provider deleted"
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
passed_before_restore=$VC_PASS
vc_check restore restore
if ((VC_PASS > passed_before_restore)); then
	restored=1
fi
vc_check mock-stop mock_stops
echo "coder-agents: $VC_PASS/$VC_TOTAL checks passed, evidence in $out" | tee -a "$out/result.txt"
((VC_PASS == VC_TOTAL))
