#!/bin/sh
set -e

# shellcheck source=scripts/docker-dev/password.sh
. /scripts/password.sh

CODER="go run ./cmd/coder"
TOKEN_FILE="/bootstrap/token"
TOKEN_NAME="bootstrap"

echo "=== Coder Dev Environment Init ==="

reject_old_default CODER_DEV_ADMIN_PASSWORD "${CODER_DEV_ADMIN_PASSWORD:-}"

print_admin_login() {
	if [ -n "${CODER_DEV_ADMIN_PASSWORD:-}" ]; then
		echo "Admin login: admin@coder.com (password from CODER_DEV_ADMIN_PASSWORD)"
	elif [ -s "$ADMIN_PASSWORD_FILE" ]; then
		echo "Admin login: admin@coder.com / $(cat "$ADMIN_PASSWORD_FILE")"
	else
		echo "Admin login: admin@coder.com (password was not set by this environment)"
	fi
}

if curl -s -o /dev/null -w "%{http_code}" http://coderd:3000/api/v2/users/first | grep -q "200"; then
	echo "First user already exists, skipping setup"
	print_admin_login
	exit 0
fi

# setup-users mounts /bootstrap read-only, so the member password is
# created here too.
create_password_file "$MEMBER_PASSWORD_FILE"
if [ -n "${CODER_DEV_ADMIN_PASSWORD:-}" ]; then
	PASSWORD="$CODER_DEV_ADMIN_PASSWORD"
else
	create_password_file "$ADMIN_PASSWORD_FILE"
	PASSWORD="$(cat "$ADMIN_PASSWORD_FILE")"
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

print_admin_login
echo "=== Init complete ==="
