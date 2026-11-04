#!/usr/bin/env bash
# Report only aggregate counts, never commands or supervisor credentials.
set -euo pipefail

field=${1:?A counter field is required}
case "$field" in
total | running) ;;
*)
	printf 'Unsupported workload counter.\n' >&2
	exit 1
	;;
esac
url=${WORKLOAD_CONTROL_PLANE_URL:?WORKLOAD_CONTROL_PLANE_URL is required}
: "${CODER_AGENT_TOKEN:?CODER_AGENT_TOKEN is required}"

# Use the current agent credential, not an administrator's session. Keep it out
# of command arguments and never print a failed API response as metadata.
if ! response=$(curl --fail --silent --show-error --max-time 8 \
	--header @<(printf 'Coder-Session-Token: %s\n' "$CODER_AGENT_TOKEN") \
	"${url%/}/api/v2/workspaceagents/me/workload-stats"); then
	printf 'Workload counts unavailable; ensure this host is registered.\n' >&2
	exit 1
fi
jq -er --arg field "$field" '.[$field] | select(type == "number" and . >= 0 and floor == .)' <<<"$response"
