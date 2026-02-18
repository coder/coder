#!/bin/sh
set -e

CODER="go run ./cmd/coder"
PASSWORD="${CODER_DEV_ADMIN_PASSWORD:-SomeSecurePassword!}"
TOKEN_FILE="/bootstrap/token"
TOKEN_NAME="bootstrap"

echo "=== Coder Dev Environment Init ==="

if curl -s -o /dev/null -w "%{http_code}" http://coderd:3000/api/v2/users/first | grep -q "200"; then
    echo "First user already exists, skipping setup"
    exit 0
fi

# Step 1: Create first user (idempotent - creates OR logs in)
echo "Creating/logging in first user..."
$CODER login http://coderd:3000 \
	--first-user-username=admin \
	--first-user-email=admin@coder.com \
	--first-user-password="$PASSWORD" \
	--first-user-full-name="Admin User" \
	--first-user-trial=false

# Step 2: Create or retrieve bootstrap token
if [ -f "$TOKEN_FILE" ] && [ -s "$TOKEN_FILE" ]; then
	echo "Bootstrap token already exists."
else
	echo "Creating bootstrap token..."
	# Delete existing token if it exists (in case file was lost but token exists)
	$CODER tokens delete "$TOKEN_NAME" 2>/dev/null || true
	# Create new token with no expiry
	TOKEN=$($CODER tokens create --name "$TOKEN_NAME" --lifetime 0)
	echo "$TOKEN" >"$TOKEN_FILE"
	echo "Bootstrap token created and saved."
fi

echo "=== Init complete ==="
