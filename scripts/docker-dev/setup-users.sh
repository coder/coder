#!/bin/sh
set -e

CODER="go run ./cmd/coder"
PASSWORD="${CODER_DEV_MEMBER_PASSWORD:-SomeSecurePassword!}"
TOKEN_FILE="/bootstrap/token"

echo "=== Setting up users ==="

# Load bootstrap token
CODER_SESSION_TOKEN=$(cat "$TOKEN_FILE")
if [ -z "${CODER_SESSION_TOKEN}" ]; then
	echo "Bootstrap token not found in ${TOKEN_FILE}"
	exit 1
fi
export CODER_SESSION_TOKEN

# Create member user (idempotent)
echo "Creating member user..."
$CODER users create \
	--email=member@coder.com \
	--username=member \
	--full-name="Regular User" \
	--password="$PASSWORD" 2>/dev/null || echo "Member user already exists."

echo "=== Users setup complete ==="
