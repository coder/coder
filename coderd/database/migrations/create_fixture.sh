#!/usr/bin/env bash

# Naming the fixture is optional, if missing, the name of the latest
# migration will be used.
#
# Usage:
# ./create_fixture
# ./create_fixture name of fixture
# ./create_fixture "name of fixture"
# ./create_fixture name_of_fixture

set -euo pipefail

SCRIPT_DIR=$(dirname "${BASH_SOURCE[0]}")
(
	cd "$SCRIPT_DIR"

	latest_migration=$(basename "$(find . -maxdepth 1 -name "*.up.sql" | sort -n | tail -n 1)")
	if [[ -n "${*}" ]]; then
		name=$*
		name=${name// /_}
		num=${latest_migration%%_*}
		latest_migration="${num}_${name}.up.sql"
	fi

	filename="$(pwd)/testdata/fixtures/$latest_migration"
	touch "$filename"
	echo "$filename"
	echo "Edit fixture and commit it."
)
