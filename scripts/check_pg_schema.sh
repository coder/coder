#!/usr/bin/env bash

# This script checks that SQL files do not hardcode the "public" schema;
# they should rely on search_path instead to support deployments using
# non-public schemas.
#
# Usage: check_pg_schema.sh <label> [files...]
# Example: check_pg_schema.sh "Migrations" file1.sql file2.sql

set -euo pipefail
# shellcheck source=scripts/lib.sh
source "$(dirname "${BASH_SOURCE[0]}")/lib.sh"
cdroot

if [[ $# -lt 1 ]]; then
	error "Usage: check_pg_schema.sh <label> [files...]"
	exit 1
fi

label=$1
shift

# No files provided, nothing to check.
if [[ $# -eq 0 ]]; then
	log "$label schema references OK (no files to check)"
	exit 0
fi

files=("$@")

set +e
matches=$(grep -l 'public\.' "${files[@]}" 2>/dev/null)
set -e

if [[ -n "$matches" ]]; then
	log "ERROR: $label must not hardcode the 'public' schema. Use unqualified table names instead."
	echo "The following files contain 'public.' references:" >&2
	while read -r file; do
		echo "  $file" >&2
		grep -n 'public\.' "$file" | head -5 | sed 's/^/    /' >&2
	done <<<"$matches"
	exit 1
fi

log "$label schema references OK"
