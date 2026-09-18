#!/usr/bin/env bash
# Configure an OpenCode Go Responses model on a running Coder instance.
#
# The script is idempotent. It creates or updates the provider, preserves an
# existing stored provider key when OPENCODE_GO_API_KEY is unset, and creates
# or updates the organization model configuration. It never prints secrets.
#
# Required for password login:
#   CODER_PASSWORD='...' \
#     ./scripts/setup-coder-opencode-go.sh
#
# Alternatively, provide CODER_SESSION_TOKEN. The API key is optional when the
# provider already has one stored in Coder:
#   OPENCODE_GO_API_KEY='...' ./scripts/setup-coder-opencode-go.sh

set -euo pipefail

: "${CODER_URL:=http://localhost:3140}"
: "${CODER_PROVIDER_NAME:=opencode-go}"
: "${CODER_PROVIDER_DISPLAY_NAME:=OpenCode Go}"
: "${CODER_PROVIDER_BASE_URL:=https://opencode.ai/zen/go/v1}"
: "${OPENCODE_MODEL:=muse-spark-1.3-contributor}"
: "${OPENCODE_MODEL_DISPLAY_NAME:=Muse Spark 1.3 Contributor}"
: "${OPENCODE_CONTEXT_LIMIT:=200000}"
: "${OPENCODE_COMPRESSION_THRESHOLD:=70}"
: "${OPENCODE_MAKE_DEFAULT:=true}"
: "${CODER_EMAIL:=admin@coder.com}"

for command in curl jq; do
	command -v "$command" >/dev/null 2>&1 || {
		echo "error: $command is required" >&2
		exit 1
	}
done

api_url() {
	printf '%s%s' "${CODER_URL%/}" "$1"
}

urlencode() {
	jq -rn --arg value "$1" '$value | @uri'
}

if [[ -n "${CODER_SESSION_TOKEN:-}" ]]; then
	token=$CODER_SESSION_TOKEN
else
	: "${CODER_PASSWORD:?set CODER_PASSWORD or CODER_SESSION_TOKEN}"
	token=$(curl --fail-with-body --silent --show-error \
		-X POST "$(api_url /api/v2/users/login)" \
		-H 'Content-Type: application/json' \
		--data "$(jq -nc --arg email "$CODER_EMAIL" --arg password "$CODER_PASSWORD" \
			'{email: $email, password: $password}')" | jq -er '.session_token')
fi

auth_header="Coder-Session-Token: $token"

api_get() {
	curl --fail-with-body --silent --show-error \
		-H "$auth_header" "$1"
}

api_json() {
	local method=$1
	local url=$2
	local body=$3
	curl --fail-with-body --silent --show-error \
		-X "$method" "$url" \
		-H "$auth_header" \
		-H 'Content-Type: application/json' \
		--data "$body"
}

provider_name_encoded=$(urlencode "$CODER_PROVIDER_NAME")
provider_id=$(api_get "$(api_url /api/v2/ai/providers)" | \
	jq -er --arg name "$CODER_PROVIDER_NAME" \
		'[.[] | select(.name == $name) | .id] | first // empty' || true)

settings=$(jq -nc \
	'{_type: "upstream-headers", _version: 1, headers: {"x-opencode-session": "{{chat_id}}"}}')

provider_payload=$(jq -nc \
	--arg type 'openai-compat' \
	--arg name "$CODER_PROVIDER_NAME" \
	--arg display_name "$CODER_PROVIDER_DISPLAY_NAME" \
	--arg base_url "$CODER_PROVIDER_BASE_URL" \
	--argjson settings "$settings" \
	--arg api_key "${OPENCODE_GO_API_KEY:-}" \
	'{type: $type, name: $name, display_name: $display_name, enabled: true,
	  base_url: $base_url, settings: $settings}
	 + if $api_key == "" then {} else {api_keys: [$api_key]} end')

if [[ -z "$provider_id" ]]; then
	provider=$(api_json POST "$(api_url /api/v2/ai/providers)" "$provider_payload")
	provider_id=$(jq -er '.id' <<<"$provider")
else
	update_payload=$(jq -nc \
		--arg display_name "$CODER_PROVIDER_DISPLAY_NAME" \
		--arg base_url "$CODER_PROVIDER_BASE_URL" \
		--argjson settings "$settings" \
		--arg api_key "${OPENCODE_GO_API_KEY:-}" \
		'{display_name: $display_name, enabled: true, base_url: $base_url,
		  settings: $settings}
		 + if $api_key == "" then {} else {api_keys: [{api_key: $api_key}]} end')
	api_json PATCH "$(api_url /api/v2/ai/providers/$provider_name_encoded)" "$update_payload" >/dev/null
fi

organization_id=${CODER_ORGANIZATION_ID:-}
if [[ -z "$organization_id" ]]; then
	organization_id=$(api_get "$(api_url /api/v2/organizations)" | jq -er '.[0].id')
fi

models=$(api_get "$(api_url "/api/v2/organizations/$organization_id/chats/models")")
model_id=$(jq -r --arg model "$OPENCODE_MODEL" \
	'[.models[] | select(.model == $model) | .id] | first // empty' <<<"$models")

model_payload=$(jq -nc \
	--arg provider_id "$provider_id" \
	--arg model "$OPENCODE_MODEL" \
	--arg display_name "$OPENCODE_MODEL_DISPLAY_NAME" \
	--argjson context_limit "$OPENCODE_CONTEXT_LIMIT" \
	--argjson compression_threshold "$OPENCODE_COMPRESSION_THRESHOLD" \
	--argjson is_default "$OPENCODE_MAKE_DEFAULT" \
	'{ai_provider_id: $provider_id, model: $model, display_name: $display_name,
	  enabled: true, is_default: $is_default, context_limit: $context_limit,
	  compression_threshold: $compression_threshold,
	  model_config: {openai_config: {use_responses_api: true}}}')

if [[ -z "$model_id" ]]; then
	api_json POST "$(api_url "/api/v2/organizations/$organization_id/chats/models")" "$model_payload" >/dev/null
else
	api_json PATCH "$(api_url "/api/v2/organizations/$organization_id/chats/models/$model_id")" "$model_payload" >/dev/null
fi

provider_summary=$(api_get "$(api_url "/api/v2/ai/providers/$provider_name_encoded")")
key_count=$(jq '.api_keys | length' <<<"$provider_summary")

printf 'Configured provider %q with Responses model %q for organization %s.\n' \
	"$CODER_PROVIDER_NAME" "$OPENCODE_MODEL" "$organization_id"
printf 'Upstream session header: x-opencode-session={{chat_id}}\n'
if [[ "$key_count" == 0 ]]; then
	printf 'Warning: no stored provider key is present. Set OPENCODE_GO_API_KEY and rerun, or add it in the Coder UI.\n' >&2
elif [[ -z "${OPENCODE_GO_API_KEY:-}" ]]; then
	printf 'Existing stored provider key preserved.\n'
else
	printf 'Provider key updated without printing its value.\n'
fi
