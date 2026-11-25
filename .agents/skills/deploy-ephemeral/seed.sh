#!/usr/bin/env bash
# Seeds a full-mode coder-ephemeral deployment with AI providers and models.
# The workspace supervisor runs it inside eph-dev after every successful
# start. It only creates what is missing, so a tester's changes survive
# restarts.
set -euo pipefail

url=${CODER_URL:-http://127.0.0.1:3100}
email=${EPH_ADMIN_EMAIL:-admin@coder.com}
password=${EPH_ADMIN_PASSWORD:-SomeSecurePassword!}
# eph-proxy on the VM listens here. It replaces the placeholder key with the
# coder-ephemeral service account token and forwards to dogfood's AI Gateway.
gateway=${EPH_GATEWAY_URL:-http://host.docker.internal:7080}
placeholder_key=set-by-eph-proxy

token=

api() {
	local method=$1 path=$2 body=${3:-} out status
	out=$(mktemp)
	local args=(-sS -o "$out" -w '%{http_code}' -X "$method" "$url$path")
	if [[ -n $token ]]; then
		args+=(-H "Coder-Session-Token: $token")
	fi
	if [[ -n $body ]]; then
		args+=(-H 'Content-Type: application/json' --data-binary "$body")
	fi
	if ! status=$(curl "${args[@]}"); then
		rm -f "$out"
		echo "seed: $method $path failed" >&2
		return 1
	fi
	if [[ $status != 2* ]]; then
		echo "seed: $method $path returned HTTP $status: $(head -c 500 "$out")" >&2
		rm -f "$out"
		return 1
	fi
	cat "$out"
	rm -f "$out"
}

token=$(api POST /api/v2/users/login "$(jq -cn --arg email "$email" --arg password "$password" \
	'{email: $email, password: $password}')" | jq -er .session_token)

ensure_provider() {
	local name=$1 type=$2 display_name=$3 base_url=$4 id
	id=$(api GET /api/v2/ai/providers | jq -r --arg name "$name" '(. // [])[] | select(.name == $name) | .id' | head -n1)
	if [[ -z $id ]]; then
		id=$(api POST /api/v2/ai/providers "$(jq -cn --arg type "$type" --arg name "$name" \
			--arg display_name "$display_name" --arg base_url "$base_url" --arg key "$placeholder_key" \
			'{type: $type, name: $name, display_name: $display_name, enabled: true, base_url: $base_url, api_keys: [$key]}')" |
			jq -er .id)
		echo "seed: created provider $name" >&2
	fi
	printf '%s' "$id"
}

models_json=$(api GET /api/v2/organizations/default/chats/models)
first_model=true
if [[ $(jq '(.models // []) | length' <<<"$models_json") -gt 0 ]]; then
	first_model=false
fi

ensure_model() {
	local provider_id=$1 model=$2 display_name=$3 context_limit=$4 model_config=${5:-null} body
	if jq -e --arg model "$model" '(.models // [])[] | select(.model == $model)' <<<"$models_json" >/dev/null; then
		return 0
	fi
	body=$(jq -cn --arg provider "$provider_id" --arg model "$model" --arg display_name "$display_name" \
		--argjson context_limit "$context_limit" --argjson is_default "$first_model" \
		--argjson model_config "$model_config" \
		'{ai_provider_id: $provider, model: $model, display_name: $display_name, enabled: true,
		  is_default: $is_default, context_limit: $context_limit}
		 + (if $model_config == null then {} else {model_config: $model_config} end)')
	api POST /api/v2/organizations/default/chats/models "$body" >/dev/null
	first_model=false
	echo "seed: created model $model" >&2
}

anthropic=$(ensure_provider anthropic anthropic Anthropic "$gateway/anthropic")
openai=$(ensure_provider openai openai OpenAI "$gateway/openai/v1")

# Display names and context limits follow dogfood's own model list. The first
# model created in an empty organization becomes the default.
ensure_model "$anthropic" claude-opus-5-5 "Opus 5.5" 1000000
ensure_model "$anthropic" claude-sonnet-5-5 "Sonnet 5.5" 1000000
ensure_model "$anthropic" claude-haiku-4-5 "Haiku 4.5" 200000
# Astra's function calling only works over the Responses API.
ensure_model "$openai" gpt-6-astra "GPT 6 Astra" 272000 '{"openai_config": {"use_responses_api": true}}'

overrides=$(api GET /api/v2/organizations/default/chats/model-overrides)
if ! jq -e '(.overrides // [])[] | select(.context == "title_generation" and .model_config_id != "")' <<<"$overrides" >/dev/null; then
	haiku=$(api GET /api/v2/organizations/default/chats/models |
		jq -r '(.models // [])[] | select(.model == "claude-haiku-4-5") | .id' | head -n1)
	if [[ -n $haiku ]]; then
		api PUT /api/v2/organizations/default/chats/model-overrides/title_generation \
			"$(jq -cn --arg id "$haiku" '{model_config_id: $id}')" >/dev/null
		echo "seed: set the title generation model to claude-haiku-4-5" >&2
	fi
fi

echo "seed: done" >&2
