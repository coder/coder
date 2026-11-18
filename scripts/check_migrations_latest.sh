#!/usr/bin/env bash

# This script checks that coderd/database/migrations/LATEST names the newest
# migration and that no two migrations share a number. With --write, it
# rewrites LATEST to name the newest migration instead.
#
# LATEST turns concurrent migration PRs into a git conflict. Every PR that
# adds a migration rewrites the same line of LATEST, so once one of them
# merges, GitHub reports the others as conflicting and blocks their merge,
# even when their CI passed. Without it, two PRs that picked the same number
# both merge cleanly and leave main with a duplicate migration.
#
# Usage: check_migrations_latest.sh [--write]

set -euo pipefail
# shellcheck source=scripts/lib.sh
source "$(dirname "${BASH_SOURCE[0]}")/lib.sh"
cdroot

migrations_dir="coderd/database/migrations"
latest_file="${migrations_dir}/LATEST"
fix_hint="Run ./${migrations_dir}/fix_migration_numbers.sh to renumber your migrations and update LATEST."

write=0
if [[ $# -gt 0 ]]; then
	if [[ $# -eq 1 && $1 == "--write" ]]; then
		write=1
	else
		error "Usage: check_migrations_latest.sh [--write]"
	fi
fi

# A loop rather than an array literal, because shfmt parses a [0-9] glob
# inside an array literal as an index assignment.
migrations=()
shopt -s nullglob
for migration in "${migrations_dir}"/[0-9][0-9][0-9][0-9][0-9][0-9]_*.sql; do
	migrations+=("${migration##*/}")
done
shopt -u nullglob

mapfile -t ups < <(printf '%s\n' "${migrations[@]}" | grep '\.up\.sql$' | LC_ALL=C sort)
if [[ ${#ups[@]} -eq 0 ]]; then
	error "No migrations found in ${migrations_dir}"
fi
newest="${ups[${#ups[@]} - 1]%.up.sql}"

if [[ $write -eq 1 ]]; then
	cat >"$latest_file" <<-LATEST
		# The newest migration. Every PR that adds a migration changes the line
		# below, so two such PRs conflict in git instead of merging into a
		# duplicate migration number. create_migration.sh and
		# fix_migration_numbers.sh keep it up to date, and
		# scripts/check_migrations_latest.sh checks it.
		${newest}
	LATEST
	log "Wrote ${newest} to ${latest_file}"
	exit 0
fi

failed=0

# Two migrations with the same number break every database migration, so
# report them even though LATEST would also be wrong.
duplicates=$(
	printf '%s\n' "${migrations[@]}" |
		sed -nE 's/^([0-9]{6})_.*\.(up|down)\.sql$/\1 \2/p' |
		LC_ALL=C sort | uniq -d
)
if [[ -n "$duplicates" ]]; then
	log "ERROR: More than one migration uses the same number:"
	while read -r num direction; do
		printf '%s\n' "${migrations[@]}" | grep "^${num}_.*\.${direction}\.sql$" | sed "s|^|  ${migrations_dir}/|" >&2
	done <<<"$duplicates"
	failed=1
fi

if [[ ! -f "$latest_file" ]]; then
	log "ERROR: ${latest_file} is missing. Run ./scripts/check_migrations_latest.sh --write to create it."
	exit 1
fi

mapfile -t entries < <(grep -Ev '^[[:space:]]*(#|$)' "$latest_file" || true)
if [[ ${#entries[@]} -ne 1 ]]; then
	log "ERROR: ${latest_file} must name exactly one migration, found ${#entries[@]}."
	log "$fix_hint"
	exit 1
fi

# Migration names contain no whitespace, so strip all of it. This tolerates
# trailing spaces and CRLF line endings.
entry="${entries[0]//[[:space:]]/}"
if [[ "$entry" != "$newest" ]]; then
	log "ERROR: ${latest_file} names ${entry}, but the newest migration is ${newest}."
	log "$fix_hint"
	failed=1
fi

if [[ $failed -ne 0 ]]; then
	exit 1
fi

log "Migrations OK, ${latest_file} names ${newest}"
