#!/bin/sh
set -e

# shellcheck source=scripts/docker-dev/password.sh
. /scripts/password.sh

CODER="go run ./cmd/coder"
TOKEN_FILE="/bootstrap/token"

echo "=== Setting up users ==="

reject_old_default CODER_DEV_MEMBER_PASSWORD "${CODER_DEV_MEMBER_PASSWORD:-}"

if [ -n "${CODER_DEV_MEMBER_PASSWORD:-}" ]; then
	PASSWORD="$CODER_DEV_MEMBER_PASSWORD"
elif [ -s "$MEMBER_PASSWORD_FILE" ]; then
	PASSWORD="$(cat "$MEMBER_PASSWORD_FILE")"
else
	echo "Member password file $MEMBER_PASSWORD_FILE is missing or empty." >&2
	exit 1
fi

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

echo "Member login: member@coder.com / $PASSWORD"
echo "=== Users setup complete ==="
