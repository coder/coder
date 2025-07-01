#!/bin/bash
set -e

# Manual OAuth2 flow test with automatic callback handling
# Usage: ./test-manual-flow.sh

SESSION_TOKEN="${SESSION_TOKEN:-$(cat ./.coderv2/session 2>/dev/null || echo '')}"
BASE_URL="${BASE_URL:-http://localhost:3000}"
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

# Colors for output
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
RED='\033[0;31m'
NC='\033[0m' # No Color

# Cleanup function
cleanup() {
	if [ -n "$SERVER_PID" ]; then
		echo -e "\n${YELLOW}Stopping OAuth2 test server...${NC}"
		kill "$SERVER_PID" 2>/dev/null || true
	fi
}

trap cleanup EXIT

# Check if app credentials are set
if [ -z "$CLIENT_ID" ] || [ -z "$CLIENT_SECRET" ]; then
	echo -e "${RED}ERROR: CLIENT_ID and CLIENT_SECRET must be set${NC}"
	echo "Run: eval \$(./setup-test-app.sh) first"
	exit 1
fi

# Check if Go is installed
if ! command -v go &>/dev/null; then
	echo -e "${RED}ERROR: Go is not installed${NC}"
	echo "Please install Go to use the OAuth2 test server"
	exit 1
fi

# Generate PKCE parameters
CODE_VERIFIER=$(openssl rand -base64 32 | tr -d "=+/" | cut -c -43)
export CODE_VERIFIER
CODE_CHALLENGE=$(echo -n "$CODE_VERIFIER" | openssl dgst -sha256 -binary | base64 | tr -d "=" | tr '+/' '-_')
export CODE_CHALLENGE

# Generate state parameter
STATE=$(openssl rand -hex 16)
export STATE

# Export required environment variables
export CLIENT_ID
export CLIENT_SECRET
export BASE_URL

# Start the OAuth2 test server
echo -e "${YELLOW}Starting OAuth2 test server on http://localhost:9876${NC}"
go run "$SCRIPT_DIR/oauth2-test-server.go" &
SERVER_PID=$!

# Wait for server to start
sleep 1

# Build authorization URL
AUTH_URL="$BASE_URL/oauth2/authorize?client_id=$CLIENT_ID&response_type=code&redirect_uri=http://localhost:9876/callback&state=$STATE&code_challenge=$CODE_CHALLENGE&code_challenge_method=S256"

echo ""
echo -e "${GREEN}=== Manual OAuth2 Flow Test ===${NC}"
echo ""
echo "1. Open this URL in your browser:"
echo -e "${YELLOW}$AUTH_URL${NC}"
echo ""
echo "2. Log in if required, then click 'Allow' to authorize the application"
echo ""
echo "3. You'll be automatically redirected to the test server"
echo "   The server will handle the token exchange and display the results"
echo ""
echo -e "${YELLOW}Waiting for OAuth2 callback...${NC}"
echo "Press Ctrl+C to cancel"
echo ""

# Wait for the server process
wait $SERVER_PID
