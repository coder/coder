#!/usr/bin/env bash

# This script checks that database migrations do not hardcode the "public" schema.
# Migrations should rely on search_path instead to support deployments using
# non-public schemas.

set -euo pipefail
# shellcheck source=scripts/lib.sh
source "$(dirname "${BASH_SOURCE[0]}")/lib.sh"
cdroot

failed=0

# check_public_schema_references checks for hardcoded public. schema references
# in SQL files within a directory.
# Arguments:
#   $1 - directory to check
#   $2 - description for error message (e.g., "migration" or "fixture")
#   $3 - find maxdepth (optional, defaults to no limit)
check_public_schema_references() {
	local dir=$1
	local desc=$2
	local maxdepth=${3:-}

	if [[ ! -d "$dir" ]]; then
		return 0
	fi

	local find_args=("$dir")
	if [[ -n "$maxdepth" ]]; then
		find_args+=(-maxdepth "$maxdepth")
	fi
	find_args+=(-name '*.sql' -type f -print0)

	set +e
	local matches
	matches=$(
		find "${find_args[@]}" |
			xargs -0 grep -l 'public\.' 2>/dev/null
	)
	set -e

	if [[ -n "$matches" ]]; then
		error "${desc} files must not hardcode the 'public' schema. Use unqualified table names instead."
		echo "The following ${desc} files contain 'public.' references:"
		echo "$matches" | while read -r file; do
			echo "  $file"
			grep -n 'public\.' "$file" | sed 's/^/    /' | head -5
		done
		return 1
	fi
	return 0
}

# Check migrations (top-level only, not testdata)
if ! check_public_schema_references "coderd/database/migrations" "Migration" 1; then
	failed=1
fi

# NOTE: Fixtures (testdata) are not checked because they contain historical data
# that uses hardcoded public. schema references. Cleaning these up would be a
# separate effort.

if [[ $failed -eq 1 ]]; then
	exit 1
fi

log "Migration schema references OK"
