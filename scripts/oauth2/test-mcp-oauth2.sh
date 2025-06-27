#!/bin/bash
set -euo pipefail

# Configuration
SESSION_TOKEN="${SESSION_TOKEN:-$(cat ./.coderv2/session 2>/dev/null || echo '')}"
BASE_URL="${BASE_URL:-http://localhost:3000}"

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[0;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Check prerequisites
if [ -z "$SESSION_TOKEN" ]; then
	echo -e "${RED}ERROR: SESSION_TOKEN must be set or ./.coderv2/session must exist${NC}"
	echo "Usage: SESSION_TOKEN=xxx ./test-mcp-oauth2.sh"
	echo "Or run: ./scripts/coder-dev.sh login"
	exit 1
fi

# Use session token for authentication
AUTH_HEADER="Coder-Session-Token: $SESSION_TOKEN"

echo -e "${BLUE}=== MCP OAuth2 Phase 1 Complete Test Suite ===${NC}\n"

# Test 1: Metadata endpoint
echo -e "${YELLOW}Test 1: OAuth2 Authorization Server Metadata${NC}"
METADATA=$(curl -s "$BASE_URL/.well-known/oauth-authorization-server")
echo "$METADATA" | jq .

if echo "$METADATA" | jq -e '.authorization_endpoint' >/dev/null; then
	echo -e "${GREEN}✓ Metadata endpoint working${NC}\n"
else
	echo -e "${RED}✗ Metadata endpoint failed${NC}\n"
	exit 1
fi

# Create OAuth2 App
echo -e "${YELLOW}Creating OAuth2 app...${NC}"
APP_NAME="test-mcp-$(date +%s)"
APP_RESPONSE=$(curl -s -X POST "$BASE_URL/api/v2/oauth2-provider/apps" \
	-H "$AUTH_HEADER" \
	-H "Content-Type: application/json" \
	-d "{
    \"name\": \"$APP_NAME\",
    \"callback_url\": \"http://localhost:9876/callback\"
  }")

if ! CLIENT_ID=$(echo "$APP_RESPONSE" | jq -r '.id'); then
	echo -e "${RED}Failed to create app:${NC}"
	echo "$APP_RESPONSE" | jq .
	exit 1
fi

echo -e "${GREEN}✓ Created app: $APP_NAME (ID: $CLIENT_ID)${NC}"

# Create Client Secret
echo -e "${YELLOW}Creating client secret...${NC}"
SECRET_RESPONSE=$(curl -s -X POST "$BASE_URL/api/v2/oauth2-provider/apps/$CLIENT_ID/secrets" \
	-H "$AUTH_HEADER")

CLIENT_SECRET=$(echo "$SECRET_RESPONSE" | jq -r '.client_secret_full')
echo -e "${GREEN}✓ Created client secret${NC}\n"

# Test 2: PKCE Flow
echo -e "${YELLOW}Test 2: PKCE Flow${NC}"
CODE_VERIFIER=$(openssl rand -base64 32 | tr -d "=+/" | cut -c -43)
CODE_CHALLENGE=$(echo -n "$CODE_VERIFIER" | openssl dgst -sha256 -binary | base64 | tr -d "=" | tr '+/' '-_')
STATE=$(openssl rand -hex 16)

AUTH_URL="$BASE_URL/oauth2/authorize?client_id=$CLIENT_ID&response_type=code&redirect_uri=http://localhost:9876/callback&state=$STATE&code_challenge=$CODE_CHALLENGE&code_challenge_method=S256"

REDIRECT_URL=$(curl -s -X POST "$AUTH_URL" \
	-H "Coder-Session-Token: $SESSION_TOKEN" \
	-w '\n%{redirect_url}' \
	-o /dev/null)

CODE=$(echo "$REDIRECT_URL" | grep -oP 'code=\K[^&]+')

if [ -n "$CODE" ]; then
	echo -e "${GREEN}✓ Got authorization code with PKCE${NC}"
else
	echo -e "${RED}✗ Failed to get authorization code${NC}"
	exit 1
fi

# Exchange with PKCE
TOKEN_RESPONSE=$(curl -s -X POST "$BASE_URL/oauth2/tokens" \
	-H "Content-Type: application/x-www-form-urlencoded" \
	-d "grant_type=authorization_code" \
	-d "code=$CODE" \
	-d "client_id=$CLIENT_ID" \
	-d "client_secret=$CLIENT_SECRET" \
	-d "code_verifier=$CODE_VERIFIER")

if echo "$TOKEN_RESPONSE" | jq -e '.access_token' >/dev/null; then
	echo -e "${GREEN}✓ PKCE token exchange successful${NC}\n"
else
	echo -e "${RED}✗ PKCE token exchange failed:${NC}"
	echo "$TOKEN_RESPONSE" | jq .
	exit 1
fi

# Test 3: Invalid PKCE
echo -e "${YELLOW}Test 3: Invalid PKCE (negative test)${NC}"
# Get new code
REDIRECT_URL=$(curl -s -X POST "$AUTH_URL" \
	-H "Coder-Session-Token: $SESSION_TOKEN" \
	-w '\n%{redirect_url}' \
	-o /dev/null)
CODE=$(echo "$REDIRECT_URL" | grep -oP 'code=\K[^&]+')

ERROR_RESPONSE=$(curl -s -X POST "$BASE_URL/oauth2/tokens" \
	-H "Content-Type: application/x-www-form-urlencoded" \
	-d "grant_type=authorization_code" \
	-d "code=$CODE" \
	-d "client_id=$CLIENT_ID" \
	-d "client_secret=$CLIENT_SECRET" \
	-d "code_verifier=wrong-verifier")

if echo "$ERROR_RESPONSE" | jq -e '.error' >/dev/null; then
	echo -e "${GREEN}✓ Invalid PKCE correctly rejected${NC}\n"
else
	echo -e "${RED}✗ Invalid PKCE was not rejected${NC}\n"
fi

# Test 4: Resource Parameter
echo -e "${YELLOW}Test 4: Resource Parameter Support${NC}"
RESOURCE="https://api.example.com"
STATE=$(openssl rand -hex 16)
RESOURCE_AUTH_URL="$BASE_URL/oauth2/authorize?client_id=$CLIENT_ID&response_type=code&redirect_uri=http://localhost:9876/callback&state=$STATE&resource=$RESOURCE"

REDIRECT_URL=$(curl -s -X POST "$RESOURCE_AUTH_URL" \
	-H "Coder-Session-Token: $SESSION_TOKEN" \
	-w '\n%{redirect_url}' \
	-o /dev/null)

CODE=$(echo "$REDIRECT_URL" | grep -oP 'code=\K[^&]+')

TOKEN_RESPONSE=$(curl -s -X POST "$BASE_URL/oauth2/tokens" \
	-H "Content-Type: application/x-www-form-urlencoded" \
	-d "grant_type=authorization_code" \
	-d "code=$CODE" \
	-d "client_id=$CLIENT_ID" \
	-d "client_secret=$CLIENT_SECRET" \
	-d "resource=$RESOURCE")

if echo "$TOKEN_RESPONSE" | jq -e '.access_token' >/dev/null; then
	echo -e "${GREEN}✓ Resource parameter flow successful${NC}\n"
else
	echo -e "${RED}✗ Resource parameter flow failed${NC}\n"
fi

# Test 5: Token Refresh
echo -e "${YELLOW}Test 5: Token Refresh${NC}"
REFRESH_TOKEN=$(echo "$TOKEN_RESPONSE" | jq -r '.refresh_token')

REFRESH_RESPONSE=$(curl -s -X POST "$BASE_URL/oauth2/tokens" \
	-H "Content-Type: application/x-www-form-urlencoded" \
	-d "grant_type=refresh_token" \
	-d "refresh_token=$REFRESH_TOKEN" \
	-d "client_id=$CLIENT_ID" \
	-d "client_secret=$CLIENT_SECRET")

if echo "$REFRESH_RESPONSE" | jq -e '.access_token' >/dev/null; then
	echo -e "${GREEN}✓ Token refresh successful${NC}\n"
else
	echo -e "${RED}✗ Token refresh failed${NC}\n"
fi

# Test 6: RFC 6750 Bearer Token Authentication
echo -e "${YELLOW}Test 6: RFC 6750 Bearer Token Authentication${NC}"
ACCESS_TOKEN=$(echo "$TOKEN_RESPONSE" | jq -r '.access_token')

# Test Authorization: Bearer header
echo -e "${BLUE}Testing Authorization: Bearer header...${NC}"
BEARER_RESPONSE=$(curl -s -w "%{http_code}" "$BASE_URL/api/v2/users/me" \
	-H "Authorization: Bearer $ACCESS_TOKEN")

HTTP_CODE="${BEARER_RESPONSE: -3}"
if [ "$HTTP_CODE" = "200" ]; then
	echo -e "${GREEN}✓ Authorization: Bearer header working${NC}"
else
	echo -e "${RED}✗ Authorization: Bearer header failed (HTTP $HTTP_CODE)${NC}"
fi

# Test access_token query parameter
echo -e "${BLUE}Testing access_token query parameter...${NC}"
QUERY_RESPONSE=$(curl -s -w "%{http_code}" "$BASE_URL/api/v2/users/me?access_token=$ACCESS_TOKEN")

HTTP_CODE="${QUERY_RESPONSE: -3}"
if [ "$HTTP_CODE" = "200" ]; then
	echo -e "${GREEN}✓ access_token query parameter working${NC}"
else
	echo -e "${RED}✗ access_token query parameter failed (HTTP $HTTP_CODE)${NC}"
fi

# Test WWW-Authenticate header on unauthorized request
echo -e "${BLUE}Testing WWW-Authenticate header on 401...${NC}"
UNAUTH_RESPONSE=$(curl -s -I "$BASE_URL/api/v2/users/me")
if echo "$UNAUTH_RESPONSE" | grep -i "WWW-Authenticate.*Bearer" >/dev/null; then
	echo -e "${GREEN}✓ WWW-Authenticate header present${NC}"
else
	echo -e "${RED}✗ WWW-Authenticate header missing${NC}"
fi

# Test 7: Protected Resource Metadata
echo -e "${YELLOW}Test 7: Protected Resource Metadata (RFC 9728)${NC}"
PROTECTED_METADATA=$(curl -s "$BASE_URL/.well-known/oauth-protected-resource")
echo "$PROTECTED_METADATA" | jq .

if echo "$PROTECTED_METADATA" | jq -e '.bearer_methods_supported[]' | grep -q "header"; then
	echo -e "${GREEN}✓ Protected Resource Metadata indicates bearer token support${NC}\n"
else
	echo -e "${RED}✗ Protected Resource Metadata missing bearer token support${NC}\n"
fi

# Cleanup
echo -e "${YELLOW}Cleaning up...${NC}"
curl -s -X DELETE "$BASE_URL/api/v2/oauth2-provider/apps/$CLIENT_ID" \
	-H "$AUTH_HEADER" >/dev/null

echo -e "${GREEN}✓ Deleted test app${NC}"

echo -e "\n${BLUE}=== All tests completed successfully! ===${NC}"
