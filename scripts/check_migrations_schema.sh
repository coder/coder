#!/usr/bin/env bash

# This script checks that database migrations do not hardcode the "public" schema.
# Migrations should rely on search_path instead to support deployments using
# non-public schemas.

set -euo pipefail
# shellcheck source=scripts/lib.sh
source "$(dirname "${BASH_SOURCE[0]}")/lib.sh"
cdroot

# Only check actual migrations, not test fixtures or dumps
MIGRATIONS_DIR="coderd/database/migrations"

failed=0

# Search for hardcoded public. schema references in migration files
# Exclude testdata directory which contains fixtures that use public.
set +e
matches=$(
	find "$MIGRATIONS_DIR" -maxdepth 1 -name '*.sql' -type f -print0 |
		xargs -0 grep -l 'public\.' 2>/dev/null
)
set -e

if [[ -n "$matches" ]]; then
	error "Migrations must not hardcode the 'public' schema. Use unqualified table names instead."
	echo "The following migration files contain 'public.' references:"
	echo "$matches" | while read -r file; do
		echo "  $file"
		grep -n 'public\.' "$file" | sed 's/^/    /'
	done
	failed=1
fi

# Also check fixtures (testdata) for consistency
FIXTURES_DIR="coderd/database/migrations/testdata/fixtures"
if [[ -d "$FIXTURES_DIR" ]]; then
	set +e
	fixture_matches=$(
		find "$FIXTURES_DIR" -name '*.sql' -type f -print0 |
			xargs -0 grep -l 'public\.' 2>/dev/null
	)
	set -e

	if [[ -n "$fixture_matches" ]]; then
		error "Test fixtures should not hardcode the 'public' schema for consistency."
		echo "The following fixture files contain 'public.' references:"
		echo "$fixture_matches" | while read -r file; do
			echo "  $file"
			grep -n 'public\.' "$file" | sed 's/^/    /' | head -5
			echo "    ..."
		done
		failed=1
	fi
fi

if [[ $failed -eq 1 ]]; then
	exit 1
fi

log "Migration schema references OK"
