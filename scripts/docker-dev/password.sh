#!/bin/sh
# Shared by setup-init.sh and setup-users.sh.

# The old default password is public, so it is refused as an override.
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
