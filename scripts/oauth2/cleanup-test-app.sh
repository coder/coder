#!/bin/bash
set -e

# Cleanup OAuth2 test app
# Usage: ./cleanup-test-app.sh [CLIENT_ID]

CLIENT_ID="${1:-$CLIENT_ID}"
SESSION_TOKEN="${SESSION_TOKEN:-$(cat ./.coderv2/session 2>/dev/null || echo '')}"
BASE_URL="${BASE_URL:-http://localhost:3000}"

if [ -z "$CLIENT_ID" ]; then
	echo "ERROR: CLIENT_ID must be provided as argument or environment variable"
	echo "Usage: ./cleanup-test-app.sh <CLIENT_ID>"
	echo "Or set CLIENT_ID environment variable"
	exit 1
fi

if [ -z "$SESSION_TOKEN" ]; then
	echo "ERROR: SESSION_TOKEN must be set or ./.coderv2/session must exist"
	exit 1
fi

AUTH_HEADER="Coder-Session-Token: $SESSION_TOKEN"

echo "Deleting OAuth2 app: $CLIENT_ID"

RESPONSE=$(curl -s -w "\n%{http_code}" -X DELETE "$BASE_URL/api/v2/oauth2-provider/apps/$CLIENT_ID" \
	-H "$AUTH_HEADER")

HTTP_CODE=$(echo "$RESPONSE" | tail -n1)
BODY=$(echo "$RESPONSE" | head -n -1)

if [ "$HTTP_CODE" = "204" ]; then
	echo "✓ Successfully deleted OAuth2 app: $CLIENT_ID"
else
	echo "✗ Failed to delete OAuth2 app: $CLIENT_ID"
	echo "HTTP $HTTP_CODE"
	if [ -n "$BODY" ]; then
		echo "$BODY" | jq . 2>/dev/null || echo "$BODY"
	fi
	exit 1
fi
