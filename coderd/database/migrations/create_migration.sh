#!/usr/bin/env bash

# Usage:
# ./create_migration name of migration
# ./create_migration "name of migration"
# ./create_migration name_of_migration

set -euo pipefail

cat <<EOF

WARNING: Migrations now all run in a single transaction. This makes upgrades
safer, but means that 'ALTER TYPE resource_type ADD VALUE' cannot be used if the
enum value needs to be referenced in another migration.

This also means you should not use "BEGIN;" and "COMMIT;" in your migrations, as
everything is already in a migration.

An example way of the proper way to add an enum value:

CREATE TYPE new_logintype AS ENUM (
	'password',
	'github',
	'oidc',
	'token' -- this is our new value
);

ALTER TABLE users
	ALTER COLUMN login_type DROP DEFAULT, -- if the column has a default, it must be dropped first
	ALTER COLUMN login_type TYPE new_logintype USING (login_type::text::new_logintype), -- converts the old enum until the new enum using text as an intermediary
	ALTER COLUMN login_type SET DEFAULT 'password'::new_logintype; -- re-add the default using the new enum

DROP TYPE login_type;
ALTER TYPE new_logintype RENAME TO login_type;

EOF

SCRIPT_DIR=$(dirname "${BASH_SOURCE[0]}")
(
	cd "$SCRIPT_DIR"

	# if migration name is an empty string exit
	[[ -z "${*}" ]] && (echo "Must provide a migration name" && exit 1)

	# " " && "-" -> "_"
	title="$(echo "${@}" | tr "[:upper:]" "[:lower:]" | sed -E -e "s/( |-)/_/g")"

	migrate create -ext sql -dir . -seq "$title"

	echo "Run \"make gen\" to generate models."
)
