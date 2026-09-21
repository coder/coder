#!/usr/bin/env bash
# Registers the simulated chat model with a local Coder deployment so the
# workspace debugging prototype can run without a real LLM provider.
#
# Usage:
#   CODER_URL=http://127.0.0.1:3000 CODER_SESSION_TOKEN=... ./scripts/simulated-chat-model/register.sh [base_url]
#
# base_url defaults to http://127.0.0.1:18080/v1, matching the server's default
# listen address.
set -euo pipefail

CODER_URL="${CODER_URL:-http://127.0.0.1:3000}"
BASE_URL="${1:-http://127.0.0.1:18080/v1}"
: "${CODER_SESSION_TOKEN:?set CODER_SESSION_TOKEN to an admin token}"

api() {
	curl -fsS -H "Coder-Session-Token: ${CODER_SESSION_TOKEN}" -H "Content-Type: application/json" "$@"
}

org_id=$(api "${CODER_URL}/api/v2/organizations" | jq -r '.[] | select(.is_default) | .id')

provider_id=$(api "${CODER_URL}/api/v2/ai/providers" | jq -r '.[] | select(.name=="simulated") | .id' || true)
if [[ -z "${provider_id}" ]]; then
	provider_id=$(api -X POST "${CODER_URL}/api/v2/ai/providers" -d "$(jq -n --arg url "${BASE_URL}" '{
		type: "openai-compat",
		name: "simulated",
		display_name: "Simulated (prototype)",
		enabled: true,
		base_url: $url,
		api_keys: ["simulated-key"]
	}')" | jq -r .id)
	echo "created provider ${provider_id}"
else
	echo "provider exists ${provider_id}"
fi

model_id=$(api "${CODER_URL}/api/v2/organizations/${org_id}/chats/models" | jq -r '.models[] | select(.model=="simulated-debugger") | .id' || true)
if [[ -z "${model_id}" ]]; then
	model_id=$(api -X POST "${CODER_URL}/api/v2/organizations/${org_id}/chats/models" -d "$(jq -n --arg p "${provider_id}" '{
		ai_provider_id: $p,
		model: "simulated-debugger",
		display_name: "Simulated debugger",
		enabled: true,
		is_default: true,
		context_limit: 200000
	}')" | jq -r .id)
	echo "created model ${model_id}"
else
	echo "model exists ${model_id}"
fi
