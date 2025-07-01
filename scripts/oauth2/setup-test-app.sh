#!/bin/bash
set -e

# Setup OAuth2 test app and return credentials
# Usage: eval $(./setup-test-app.sh)

SESSION_TOKEN="${SESSION_TOKEN:-$(tr -d '\n' <./.coderv2/session || echo '')}"
BASE_URL="${BASE_URL:-http://localhost:3000}"

if [ -z "$SESSION_TOKEN" ]; then
	echo "ERROR: SESSION_TOKEN must be set or ./.coderv2/session must exist" >&2
	echo "Run: ./scripts/coder-dev.sh login" >&2
	exit 1
fi

AUTH_HEADER="Coder-Session-Token: $SESSION_TOKEN"

# Create OAuth2 App
APP_NAME="test-mcp-$(date +%s)"
APP_RESPONSE=$(curl -s -X POST "$BASE_URL/api/v2/oauth2-provider/apps" \
	-H "$AUTH_HEADER" \
	-H "Content-Type: application/json" \
	-d "{
    \"name\": \"$APP_NAME\",
    \"callback_url\": \"http://localhost:9876/callback\"
  }")

CLIENT_ID=$(echo "$APP_RESPONSE" | jq -r '.id')
if [ "$CLIENT_ID" = "null" ] || [ -z "$CLIENT_ID" ]; then
	echo "ERROR: Failed to create OAuth2 app" >&2
	echo "$APP_RESPONSE" | jq . >&2
	exit 1
fi

# Create Client Secret
SECRET_RESPONSE=$(curl -s -X POST "$BASE_URL/api/v2/oauth2-provider/apps/$CLIENT_ID/secrets" \
	-H "$AUTH_HEADER")

CLIENT_SECRET=$(echo "$SECRET_RESPONSE" | jq -r '.client_secret_full')
if [ "$CLIENT_SECRET" = "null" ] || [ -z "$CLIENT_SECRET" ]; then
	echo "ERROR: Failed to create client secret" >&2
	echo "$SECRET_RESPONSE" | jq . >&2
	exit 1
fi

# Output environment variable exports
echo "export CLIENT_ID=\"$CLIENT_ID\""
echo "export CLIENT_SECRET=\"$CLIENT_SECRET\""
echo "export APP_NAME=\"$APP_NAME\""
echo "export BASE_URL=\"$BASE_URL\""
echo "export SESSION_TOKEN=\"$SESSION_TOKEN\""

echo "# OAuth2 app created successfully:" >&2
echo "# App Name: $APP_NAME" >&2
echo "# Client ID: $CLIENT_ID" >&2
echo "# Run: eval \$(./setup-test-app.sh) to set environment variables" >&2
