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
		warn_password_unknown admin@coder.com
		echo "Admin login: admin@coder.com (password was not set by this environment)"
	fi
}

create_bootstrap_token() {
	echo "Creating bootstrap token..."
	# Delete existing token if it exists (in case file was lost but token exists).
	# Without --delete the token is only expired and keeps its name.
	$CODER tokens delete --delete "$TOKEN_NAME" 2>/dev/null || true
	# A lifetime of 0 gives the default token lifetime of the server,
	# which is 7 days unless it is configured.
	TOKEN=$($CODER tokens create --name "$TOKEN_NAME" --lifetime 0)
	echo "$TOKEN" >"$TOKEN_FILE"
	echo "Bootstrap token created and saved."
}

# An environment created before passwords were generated has both users
# on the old default. setup-users mounts /bootstrap read-only, so the
# member is handled here too.
replace_old_default_passwords() {
	if [ ! -s "$TOKEN_FILE" ]; then
		return 0
	fi
	token="$(cat "$TOKEN_FILE")"
	if replace_old_default member "$MEMBER_PASSWORD_FILE" "$token"; then
		echo "Replaced the old default password of member@coder.com."
	fi
	if [ -n "${CODER_DEV_ADMIN_PASSWORD:-}" ]; then
		return 0
	fi
	if replace_old_default me "$ADMIN_PASSWORD_FILE" "$token"; then
		echo "Replaced the old default password of admin@coder.com."
	fi
}

# admin_sign_in PASSWORD
# Sets CODER_SESSION_TOKEN, and status for the message of the caller.
admin_sign_in() {
	response=$(jq -n --arg password "$1" '{email: "admin@coder.com", password: $password}' |
		curl -s -w '\n%{http_code}' -H "Content-Type: application/json" --data-binary @- http://coderd:3000/api/v2/users/login)
	status=$(printf '%s\n' "$response" | tail -n 1)
	CODER_SESSION_TOKEN=$(printf '%s\n' "$response" | sed '$d' | jq -r '.session_token // empty' 2>/dev/null || true)
	[ -n "$CODER_SESSION_TOKEN" ]
}

# Changing the admin password deletes every token of the admin, and the
# bootstrap token expires, so the stored token can stop working while
# the password is known.
restore_bootstrap_token() {
	if [ -s "$TOKEN_FILE" ] && curl -s -o /dev/null -w "%{http_code}" \
		-H "Coder-Session-Token: $(cat "$TOKEN_FILE")" \
		http://coderd:3000/api/v2/users/me | grep -q "200"; then
		return 0
	fi
	source=""
	if [ -n "${CODER_DEV_ADMIN_PASSWORD:-}" ]; then
		password="$CODER_DEV_ADMIN_PASSWORD"
		source="CODER_DEV_ADMIN_PASSWORD"
	elif [ -s "$ADMIN_PASSWORD_FILE" ]; then
		password="$(cat "$ADMIN_PASSWORD_FILE")"
		source="$ADMIN_PASSWORD_FILE"
	fi
	if [ -n "$source" ] && admin_sign_in "$password"; then
		if [ "$source" = "CODER_DEV_ADMIN_PASSWORD" ] && [ "$(cat "$ADMIN_PASSWORD_FILE" 2>/dev/null)" != "$password" ]; then
			# The server just accepted the override, so it is stored for
			# the starts after the variable is unset.
			store_password "$ADMIN_PASSWORD_FILE" "$password"
		fi
	else
		reason="this environment does not know the admin password"
		if [ -n "$source" ]; then
			reason="the server answered HTTP $status to the password from $source"
		fi
		# An environment created before passwords were generated can only
		# be signed in to with the old default once its token has expired.
		# An override says what the admin password is, so the old default
		# is not tried against it.
		if [ "$source" = "CODER_DEV_ADMIN_PASSWORD" ] || ! admin_sign_in "$OLD_DEFAULT_PASSWORD"; then
			echo "Could not sign in as admin@coder.com to create a new bootstrap token: $reason." >&2
			echo "Set CODER_DEV_ADMIN_PASSWORD to the current admin password and start again, or remove the volumes of this environment to start fresh." >&2
			if [ "$source" = "CODER_DEV_ADMIN_PASSWORD" ]; then
				echo "If the admin still has the old default password, unset CODER_DEV_ADMIN_PASSWORD and start again." >&2
			fi
			exit 1
		fi
		echo "Signed in as admin@coder.com with the old default password to replace it."
	fi
	export CODER_SESSION_TOKEN
	create_bootstrap_token
}

if curl -s -o /dev/null -w "%{http_code}" http://coderd:3000/api/v2/users/first | grep -q "200"; then
	echo "First user already exists, skipping setup"
	restore_bootstrap_token
	replace_old_default_passwords
	restore_bootstrap_token
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
if [ -n "${CODER_DEV_ADMIN_PASSWORD:-}" ]; then
	# The server just accepted the override, so it is stored for the
	# starts after the variable is unset.
	store_password "$ADMIN_PASSWORD_FILE" "$PASSWORD"
fi

# Step 2: Create or retrieve bootstrap token
if [ -f "$TOKEN_FILE" ] && [ -s "$TOKEN_FILE" ]; then
	echo "Bootstrap token already exists."
else
	create_bootstrap_token
fi

print_admin_login
echo "=== Init complete ==="
