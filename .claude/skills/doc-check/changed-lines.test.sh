#!/bin/bash
# Regression tests for changed-lines.awk. Builds a throwaway repository,
# makes a commit with the cases the parser must handle, and compares the
# output with the expected lines. Run it from anywhere:
#
#   bash .claude/skills/doc-check/changed-lines.test.sh

set -euo pipefail

awk_file="$(cd "$(dirname "$0")" && pwd)/changed-lines.awk"
repo="$(mktemp -d)"
trap 'rm -rf "$repo"' EXIT
cd "$repo"

git init -q
git config user.email test@example.com
git config user.name test
git config core.autocrlf false

mkdir -p "docs/sub dir" docs/reference/cli docs/.style/style-guide
printf 'line one\nline two\nline three\n' >docs/a.md
printf 'crlf one\r\ncrlf two\r\n' >docs/crlf.md
printf 'keep\n' >"docs/sub dir/spaced.md"
printf 'gone\n' >docs/deleted.md
printf 'generated\n' >docs/reference/cli/x.md
printf 'rule\n' >docs/.style/style-guide/r.md
git add -A
git commit -qm base
base=$(git rev-parse HEAD)

# Edits: an added line that starts with "++" and one that starts with "--",
# which look like diff headers; CRLF line endings; a path with a space; a
# deleted page; a new page; and files outside the review scope.
printf 'line one\n++ starts with plus plus\nline two edited\nline three\n-- starts with dashes\n' >docs/a.md
printf 'crlf one\r\ncrlf two changed\r\ncrlf three\r\n' >docs/crlf.md
printf 'keep\nadded in spaced path\n' >"docs/sub dir/spaced.md"
git rm -q docs/deleted.md
printf 'brand new\npage\n' >docs/new.md
printf 'generated, changed\n' >docs/reference/cli/x.md
printf 'rule, changed\n' >docs/.style/style-guide/r.md
printf 'not docs\n' >README.md
git add -A
git commit -qm head
head=$(git rev-parse HEAD)

actual=$(git diff --no-color --no-ext-diff --unified=0 "$base" "$head" -- 'docs/*.md' \
	':(exclude)docs/reference/api' ':(exclude)docs/reference/cli' ':(exclude)docs/.style/style-guide' |
	awk -f "$awk_file")

expected='docs/a.md:2: ++ starts with plus plus
docs/a.md:3: line two edited
docs/a.md:5: -- starts with dashes
docs/crlf.md:2: crlf two changed
docs/crlf.md:3: crlf three
docs/new.md:1: brand new
docs/new.md:2: page
docs/sub dir/spaced.md:2: added in spaced path'

if [ "$actual" != "$expected" ]; then
	echo "changed-lines.awk output does not match." >&2
	diff <(printf '%s\n' "$expected") <(printf '%s\n' "$actual") >&2 || true
	exit 1
fi
echo "changed-lines.awk: all cases pass."
