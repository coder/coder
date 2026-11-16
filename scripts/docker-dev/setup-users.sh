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
	# setup-init writes the password file only when it creates the first
	# user, so an environment created before passwords were generated has
	# the member but no file.
	echo "This environment did not generate the password of member@coder.com." >&2
	echo "If it is still the old public default, anyone who can reach this instance can sign in as that user." >&2
	echo "Remove this environment's volumes to start fresh, or change the password in the dashboard." >&2
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
