#!/bin/sh
# Shared by setup-init.sh and setup-users.sh.

# The old default password is public, so it is refused as an override
# and replaced on a user that still has it.
OLD_DEFAULT_PASSWORD='SomeSecurePassword!'
# shellcheck disable=SC2034 # Used by the scripts that source this file.
ADMIN_PASSWORD_FILE="/bootstrap/admin-password"
# shellcheck disable=SC2034
MEMBER_PASSWORD_FILE="/bootstrap/member-password"

# reject_old_default NAME VALUE
reject_old_default() {
	if [ "$2" = "$OLD_DEFAULT_PASSWORD" ]; then
		echo "$1 must not be the old default password. Unset it to use a generated one." >&2
		exit 1
	fi
}

# warn_password_unknown EMAIL
warn_password_unknown() {
	echo "This environment does not know the password of $1, so it cannot print it." >&2
}

# create_password_file FILE
# A stored password is never replaced, because a user was created with it.
create_password_file() {
	if [ -s "$1" ]; then
		return 0
	fi
	(
		umask 077
		LC_ALL=C tr -dc 'A-Za-z0-9' </dev/urandom | head -c 32 >"$1"
	)
}

# store_password FILE VALUE
store_password() {
	(
		umask 077
		printf '%s' "$2" >"$1"
	)
}

# replace_old_default USER FILE TOKEN
# Gives the user the stored password, or a generated one, when its
# password is still the old default. The server checks the old password,
# so a password that was changed by hand is kept. A stored password does
# not show that the user has it. Returns 0 only when the password was
# replaced.
replace_old_default() {
	if [ -s "$2" ] && [ "$(cat "$2")" = "$OLD_DEFAULT_PASSWORD" ]; then
		# The server refuses a new password that equals the current one.
		rm -f "$2"
	fi
	stored=0
	if [ -s "$2" ]; then
		stored=1
	fi
	create_password_file "$2"
	status=$(jq -n --arg old "$OLD_DEFAULT_PASSWORD" --arg new "$(cat "$2")" '{old_password: $old, password: $new}' |
		curl -s -o /dev/null -w "%{http_code}" -X PUT \
			-H "Coder-Session-Token: $3" \
			-H "Content-Type: application/json" \
			--data-binary @- \
			"http://coderd:3000/api/v2/users/$1/password")
	if [ "$status" = "204" ]; then
		return 0
	fi
	if [ "$stored" = 0 ]; then
		# A generated password that the server did not accept would be
		# printed as the login.
		rm -f "$2"
	fi
	if [ "$status" != "400" ]; then
		echo "Could not replace the password of $1: the server answered HTTP $status." >&2
	fi
	return 1
}
