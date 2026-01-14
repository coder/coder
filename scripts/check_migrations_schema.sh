#!/usr/bin/env bash

# This script checks that database migrations do not hardcode the "public" schema.
# Migrations should rely on search_path instead to support deployments using
# non-public schemas.
#
# Usage: check_migrations_schema.sh [files...]
# If no files are provided, it will check the default migration and fixture paths.

set -euo pipefail
# shellcheck source=scripts/lib.sh
source "$(dirname "${BASH_SOURCE[0]}")/lib.sh"
cdroot

failed=0

check_files() {
	local files=("$@")

	if [[ ${#files[@]} -eq 0 ]]; then
		return 0
	fi

	set +e
	local matches
	matches=$(grep -l 'public\.' "${files[@]}" 2>/dev/null)
	set -e

	if [[ -n "$matches" ]]; then
		error "SQL files must not hardcode the 'public' schema. Use unqualified table names instead."
		echo "The following files contain 'public.' references:"
		echo "$matches" | while read -r file; do
			echo "  $file"
			grep -n 'public\.' "$file" | sed 's/^/    /' | head -5
		done
		return 1
	fi
	return 0
}

if [[ $# -gt 0 ]]; then
	# Files provided as arguments
	check_files "$@" || failed=1
else
	# Default behavior: check migrations and fixtures
	migration_files=$(find coderd/database/migrations -maxdepth 1 -name '*.sql' -type f 2>/dev/null)
	if [[ -n "$migration_files" ]]; then
		# shellcheck disable=SC2086
		check_files $migration_files || failed=1
	fi

	fixture_files=$(find coderd/database/migrations/testdata/fixtures -name '*.sql' -type f 2>/dev/null)
	if [[ -n "$fixture_files" ]]; then
		# shellcheck disable=SC2086
		check_files $fixture_files || failed=1
	fi
fi

if [[ $failed -eq 0 ]]; then
	log "Migration schema references OK"
fi

exit "$failed"
