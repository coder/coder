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
