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
# Without duplicate numbers, a wrong LATEST only needs rewriting. This also
# covers branches that add no migration, where fix_migration_numbers.sh exits
# early without touching LATEST.
write_hint="Run ./scripts/check_migrations_latest.sh --write, then git add and commit ${latest_file}."
renumber_hint="Run ./${migrations_dir}/fix_migration_numbers.sh to renumber your migrations and update LATEST. If that script reports this error itself, rename your migration to the next free number with git mv and run it again."

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
		#
		# If this file conflicts, another migration merged first. Merge main into
		# your branch, then run this while the conflict is still open:
		#
		#   ./coderd/database/migrations/fix_migration_numbers.sh
		#
		# It renumbers your migrations if their numbers are taken, rewrites this
		# file, and stages the result.
		#
		# Do not run it at a rebase stop. On a branch with more than one migration
		# commit it can leave duplicate numbers or swap the migration order. At
		# each rebase stop, and for a cherry-pick, backport, or revert, run these
		# instead:
		#
		#   ./scripts/check_migrations_latest.sh --write
		#   git add coderd/database/migrations/LATEST
		#
		# After a rebase finishes, run fix_migration_numbers.sh once and commit
		# the result.
		${newest}
	LATEST
	log "Wrote ${newest} to ${latest_file}"
	exit 0
fi

# Two migrations with the same number break every database migration. Report
# them first, because rewriting LATEST alone does not fix them.
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
	log "$renumber_hint"
	exit 1
fi

if [[ ! -f "$latest_file" ]]; then
	log "ERROR: ${latest_file} is missing. Run ./scripts/check_migrations_latest.sh --write to create it."
	exit 1
fi

mapfile -t entries < <(grep -Ev '^[[:space:]]*(#|$)' "$latest_file" || true)
if [[ ${#entries[@]} -ne 1 ]]; then
	log "ERROR: ${latest_file} must name exactly one migration, found ${#entries[@]}."
	log "$write_hint"
	exit 1
fi

# Migration names contain no whitespace, so strip all of it. This tolerates
# trailing spaces and CRLF line endings.
entry="${entries[0]//[[:space:]]/}"
if [[ "$entry" != "$newest" ]]; then
	log "ERROR: ${latest_file} names ${entry}, but the newest migration is ${newest}."
	log "$write_hint"
	exit 1
fi

log "Migrations OK, ${latest_file} names ${newest}"
