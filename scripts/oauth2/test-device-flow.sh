#!/bin/bash
set -e

# OAuth2 Device Authorization Flow test using x/oauth2 library
# Usage: ./test-device-flow.sh

SESSION_TOKEN="${SESSION_TOKEN:-$(cat ./.coderv2/session 2>/dev/null || echo '')}"
BASE_URL="${BASE_URL:-http://localhost:3000}"
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

# Colors for output
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
RED='\033[0;31m'
# BLUE='\033[0;34m'  # Unused color
NC='\033[0m' # No Color

echo -e "${GREEN}=== OAuth2 Device Authorization Flow Test ===${NC}"
echo ""

# Check if app credentials are set
if [ -z "$CLIENT_ID" ] || [ -z "$CLIENT_SECRET" ]; then
	echo -e "${RED}ERROR: CLIENT_ID and CLIENT_SECRET must be set${NC}"
	echo "Run: eval \$(./setup-test-app.sh) first"
	exit 1
fi

# Check if Go is installed
if ! command -v go &>/dev/null; then
	echo -e "${RED}ERROR: Go is not installed${NC}"
	echo "Please install Go to use the device flow test"
	exit 1
fi

# Export required environment variables
export CLIENT_ID
export CLIENT_SECRET
export BASE_URL

echo -e "${YELLOW}Starting device authorization flow...${NC}"
echo ""

# Run the Go device flow client
go run "$SCRIPT_DIR/device/server.go"
