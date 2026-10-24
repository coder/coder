#!/usr/bin/env bash

# This script checks that database migrations follow the expand/contract
# pattern: the schema at HEAD must stay usable by the queries of the version
# that runs alongside it during a rolling upgrade.
#
# Two boundaries are checked:
#   - branch:  the merge-base of a PR (or the previous commit on push). Catches
#              expanding and contracting in the same change.
#   - release: the latest release tag of the previous minor version. Catches
#              contracting something that the prior release still uses.
#
# For each boundary, the base ref's sqlc queries are prepared against a
# database migrated to HEAD (SQLC_DATABASE_URL), and migrations added since
# the base ref are linted for NOT NULL changes that break old INSERT statements.
#
# Findings are reported as warnings. Set MIGRATION_COMPAT_BLOCKING=1 to exit
# non-zero on findings. Override the base refs with MIGRATION_COMPAT_BRANCH_BASE
# and MIGRATION_COMPAT_RELEASE_BASE.
#
# Usage: SQLC_DATABASE_URL=... check_migration_compat.sh

set -euo pipefail
# shellcheck source=scripts/lib.sh
source "$(dirname "${BASH_SOURCE[0]}")/lib.sh"
cdroot

requiredenvs SQLC_DATABASE_URL

findings=0

warn() {
	echo "::warning title=Migration compatibility ($1)::$2"
	findings=$((findings + 1))
}

branch_base() {
	if [[ -n "${MIGRATION_COMPAT_BRANCH_BASE:-}" ]]; then
		echo "$MIGRATION_COMPAT_BRANCH_BASE"
	elif [[ -n "${GITHUB_BASE_REF:-}" ]]; then
		git merge-base HEAD "origin/$GITHUB_BASE_REF"
	elif [[ -n "${GITHUB_ACTIONS:-}" ]]; then
		git rev-parse HEAD^
	else
		local base
		base=$(git merge-base HEAD origin/main)
		if [[ "$base" == "$(git rev-parse HEAD)" ]]; then
			base=$(git rev-parse HEAD^)
		fi
		echo "$base"
	fi
}

# Prints the newest stable tag older than the minor version being built. On
# release/X.Y branches and vX.Y.Z tags that is the latest vX.(Y-1) tag; on main
# it is the latest stable tag overall.
release_base() {
	if [[ -n "${MIGRATION_COMPAT_RELEASE_BASE:-}" ]]; then
		echo "$MIGRATION_COMPAT_RELEASE_BASE"
		return
	fi
	local ref major=999999 minor=0
	ref=${GITHUB_BASE_REF:-${GITHUB_REF_NAME:-$(git rev-parse --abbrev-ref HEAD)}}
	if [[ "$ref" =~ ^release/([0-9]+)\.([0-9]+)$ ]] || [[ "$ref" =~ ^v([0-9]+)\.([0-9]+)\.[0-9]+$ ]]; then
		major=${BASH_REMATCH[1]}
		minor=${BASH_REMATCH[2]}
	fi
	git tag -l 'v*' --sort=-v:refname |
		grep -E '^v[0-9]+\.[0-9]+\.[0-9]+$' |
		awk -F'[v.]' -v M="$major" -v m="$minor" '($2 < M) || ($2 == M && $3 < m) { print; exit }'
}

# Prepares every query from the base ref against the HEAD schema. A failure
# means HEAD dropped, renamed, or retyped something the base ref still uses.
vet_queries() {
	local label=$1 ref=$2 tmp out
	tmp=$(mktemp -d)
	# shellcheck disable=SC2064 # Expand tmp now.
	trap "rm -rf '$tmp'" RETURN
	git archive "$ref" coderd/database/sqlc.yaml coderd/database/dump.sql coderd/database/queries | tar -x -C "$tmp"
	# Run from the repo root so mise resolves the pinned sqlc version.
	if out=$(sqlc vet -f "$tmp/coderd/database/sqlc.yaml" 2>&1); then
		log "$label: queries at $ref prepare against the HEAD schema"
		return
	fi
	while IFS= read -r line; do
		[[ -n "$line" ]] && warn "$label" "Query at $ref fails against the HEAD schema: $line"
	done <<<"$out"
}

# Flags ALTER TABLE statements in migrations added since the base ref that
# make a column NOT NULL without a default. INSERT statements from the base ref
# omit the column and fail at execution time, which prepare does not catch.
lint_not_null() {
	local label=$1 ref=$2 file stmt
	while IFS= read -r file; do
		[[ -z "$file" ]] && continue
		while IFS= read -r stmt; do
			[[ -z "$stmt" ]] && continue
			warn "$label" "$file adds NOT NULL without DEFAULT, which breaks INSERT statements from $ref: $stmt"
		done < <(
			sed -E 's/--.*$//; s/IS[[:space:]]+NOT[[:space:]]+NULL/IS_NOT_NULL/gI' "$file" | tr '\n' ' ' | tr ';' '\n' |
				grep -iE 'ALTER[[:space:]]+TABLE' |
				grep -iE 'SET[[:space:]]+NOT[[:space:]]+NULL|ADD[[:space:]]+(COLUMN[[:space:]]+)?[^,]*NOT[[:space:]]+NULL' |
				grep -viE 'DEFAULT|DROP[[:space:]]+NOT[[:space:]]+NULL' |
				sed -E 's/[[:space:]]+/ /g; s/^ //' || true
		)
	done < <(git diff --name-only --diff-filter=A "$ref" HEAD -- 'coderd/database/migrations/*.up.sql')
}

check() {
	local label=$1 ref=$2
	if [[ -z "$ref" ]]; then
		log "$label: no base ref found, skipping"
		return
	fi
	log "$label: checking HEAD against $ref"
	vet_queries "$label" "$ref"
	lint_not_null "$label" "$ref"
}

check branch "$(branch_base)"
check release "$(release_base)"

if [[ $findings -eq 0 ]]; then
	log "Migration compatibility OK"
	exit 0
fi

log "Found $findings migration compatibility issue(s). Expand in one release, contract in a later one. See .claude/docs/DATABASE.md."
if [[ "${MIGRATION_COMPAT_BLOCKING:-0}" == "1" ]]; then
	exit 1
fi
