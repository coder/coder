#!/bin/sh
set -e

# shellcheck source=scripts/docker-dev/password.sh
. /scripts/password.sh

CODER="go run ./cmd/coder"
TOKEN_FILE="/bootstrap/token"

echo "=== Setting up users ==="

reject_old_default CODER_DEV_MEMBER_PASSWORD "${CODER_DEV_MEMBER_PASSWORD:-}"

# Load bootstrap token
CODER_SESSION_TOKEN=$(cat "$TOKEN_FILE")
if [ -z "${CODER_SESSION_TOKEN}" ]; then
	echo "Bootstrap token not found in ${TOKEN_FILE}"
	exit 1
fi
export CODER_SESSION_TOKEN

if [ -n "${CODER_DEV_MEMBER_PASSWORD:-}" ]; then
	PASSWORD="$CODER_DEV_MEMBER_PASSWORD"
elif [ -s "$MEMBER_PASSWORD_FILE" ]; then
	PASSWORD="$(cat "$MEMBER_PASSWORD_FILE")"
elif $CODER users show member >/dev/null 2>&1; then
	# setup-init writes the password file when it creates the first user
	# or replaces the old default, so a member without a file has a
	# password that was set some other way.
	warn_password_unknown member@coder.com
	echo "Member login: member@coder.com (password was not set by this environment)"
	echo "=== Users setup complete ==="
	exit 0
else
	echo "Member password file $MEMBER_PASSWORD_FILE is missing or empty." >&2
	exit 1
fi

# Create member user (idempotent)
echo "Creating member user..."
$CODER users create \
	--email=member@coder.com \
	--username=member \
	--full-name="Regular User" \
	--password="$PASSWORD" 2>/dev/null || echo "Member user already exists."

echo "Member login: member@coder.com / $PASSWORD"
echo "=== Users setup complete ==="
