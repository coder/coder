#!/bin/bash
# Regression tests for the path-mapping logic in docs-preview.yaml.
# The mapper converts a repo-relative docs path into the URL path
# used by the docs site preview. Five distinct branches exist in the
# case block; every branch must be covered here.
#
# Also covers three other pieces of docs-preview.yaml logic (DOCS-541):
# normalizing docs/manifest.json "path" values to repo-relative paths,
# parsing checked/unchecked state back out of the rendered checklist,
# and deciding whether a page's checkbox carries forward or resets.
# Keep these in sync with the corresponding shell in docs-preview.yaml.

set -euo pipefail

# map_doc_path replicates the case block from docs-preview.yaml so
# we can exercise it without running the full workflow.
map_doc_path() {
	local doc_path="$1"
	local rel="${doc_path#docs/}"
	local page_path

	case "$rel" in
	README.md)
		page_path=""
		;;
	*)
		local base dir stripped
		base="$(basename "$rel")"
		dir="$(dirname "$rel")"
		if [ "$dir" = "." ]; then
			dir=""
		fi
		case "$base" in
		index.md | README.md)
			page_path="$dir"
			;;
		*)
			stripped="${base%.md}"
			if [ -z "$dir" ]; then
				page_path="$stripped"
			else
				page_path="${dir}/${stripped}"
			fi
			;;
		esac
		;;
	esac

	printf '%s' "$page_path"
}

failures=0

assert_maps_to() {
	local input="$1"
	local expected="$2"
	local actual
	actual="$(map_doc_path "$input")"
	if [ "$actual" = "$expected" ]; then
		echo "PASS: $input -> \"$expected\""
	else
		echo "FAIL: $input -> \"$actual\" (expected \"$expected\")"
		failures=$((failures + 1))
	fi
}

# Branch 1: top-level README maps to the docs root.
assert_maps_to "docs/README.md" ""

# Branch 2: nested index.md strips the filename, leaving the dir.
assert_maps_to "docs/install/index.md" "install"

# Branch 3: nested README.md behaves the same as index.md.
assert_maps_to "docs/admin/README.md" "admin"

# Branch 4: nested regular file strips .md and keeps the dir prefix.
assert_maps_to "docs/ai-coder/tasks.md" "ai-coder/tasks"

# Branch 5: top-level non-README file strips .md with no dir prefix.
assert_maps_to "docs/CHANGELOG.md" "CHANGELOG"

# Additional coverage for edge cases and deeper nesting.
assert_maps_to "docs/index.md" ""
assert_maps_to "docs/about/contributing/CONTRIBUTING.md" "about/contributing/CONTRIBUTING"
assert_maps_to "docs/admin/groups.md" "admin/groups"
assert_maps_to "docs/tutorials/best-practices/index.md" "tutorials/best-practices"

# normalize_manifest_path replicates the sed pipeline docs-preview.yaml
# runs over `jq -r '[.. | objects | select(has("path")) | .path]'`
# output. manifest.json paths are written either "./foo/bar.md" or
# "foo/bar.md" relative to docs/; both forms must normalize to the
# same "docs/foo/bar.md" so they compare directly against the
# filenames returned by the PR-files API.
normalize_manifest_path() {
	printf '%s' "$1" | sed -E 's#^\./##; s#^#docs/#'
}

assert_normalizes_to() {
	local input="$1"
	local expected="$2"
	local actual
	actual="$(normalize_manifest_path "$input")"
	if [ "$actual" = "$expected" ]; then
		echo "PASS: normalize($input) -> \"$expected\""
	else
		echo "FAIL: normalize($input) -> \"$actual\" (expected \"$expected\")"
		failures=$((failures + 1))
	fi
}

# Branch A: manifest path with the "./" prefix most entries use.
assert_normalizes_to "./about/screenshots.md" "docs/about/screenshots.md"

# Branch B: manifest path with no prefix, as some entries have (for
# example everything under reference/cli/ in the real manifest).
assert_normalizes_to "reference/cli/whoami.md" "docs/reference/cli/whoami.md"

# Branch C: top-level README, no subdirectory.
assert_normalizes_to "./README.md" "docs/README.md"

# parse_checkbox_line replicates the sed extraction docs-preview.yaml
# runs over the existing comment body to recover the *live* checked
# state a reviewer's clicks land in (GitHub persists a checkbox toggle
# as a comment-body edit). Emits "<x-or-space>\t<path>", matching the
# workflow's intermediate TSV format.
parse_checkbox_line() {
	# shellcheck disable=SC2016 # backticks are literal Markdown code-span delimiters, not command substitution.
	printf '%s\n' "$1" | grep -oE '^- \[[ xX]\] \[`[^`]+`\]' | sed -E 's/^- \[([ xX])\] \[`([^`]+)`\]/\1\t\2/' || true
}

assert_checkbox_parses_to() {
	local input="$1"
	local expected="$2"
	local actual
	actual="$(parse_checkbox_line "$input")"
	if [ "$actual" = "$expected" ]; then
		echo "PASS: parse_checkbox($input) -> \"$expected\""
	else
		echo "FAIL: parse_checkbox($input) -> \"$actual\" (expected \"$expected\")"
		failures=$((failures + 1))
	fi
}

# Branch A: a checked page.
# shellcheck disable=SC2016 # backtick-quoted path in the fixture is literal Markdown, not command substitution.
assert_checkbox_parses_to '- [x] [`docs/foo/bar.md`](https://coder.com/docs/@b/foo/bar)' "$(printf 'x\tdocs/foo/bar.md')"

# Branch B: an unchecked page.
# shellcheck disable=SC2016
assert_checkbox_parses_to '- [ ] [`docs/foo/baz.md`](https://coder.com/docs/@b/foo/baz)' "$(printf ' \tdocs/foo/baz.md')"

# Branch C: an uppercase X, which GitHub also renders as checked.
# shellcheck disable=SC2016
assert_checkbox_parses_to '- [X] [`docs/foo/qux.md`](https://coder.com/docs/@b/foo/qux)' "$(printf 'X\tdocs/foo/qux.md')"

# Branch D: a non-checklist line (prose, a header, the hidden markers)
# must not match at all.
assert_checkbox_parses_to '## Docs preview' ""

# decide_checked replicates the jq carryover rule in docs-preview.yaml:
# a page's checkbox only carries its previous checked value forward
# when the page's blob sha is unchanged from the last time the
# workflow wrote the state marker. A brand-new page (no previous sha)
# or a page whose sha moved always starts unchecked.
decide_checked() {
	local prev_sha="$1"
	local new_sha="$2"
	local old_checked="$3"
	if [ -n "$prev_sha" ] && [ "$prev_sha" = "$new_sha" ]; then
		printf '%s' "$old_checked"
	else
		printf 'false'
	fi
}

assert_decides_checked() {
	local prev_sha="$1" new_sha="$2" old_checked="$3" expected="$4"
	local actual
	actual="$(decide_checked "$prev_sha" "$new_sha" "$old_checked")"
	if [ "$actual" = "$expected" ]; then
		echo "PASS: decide_checked($prev_sha, $new_sha, $old_checked) -> \"$expected\""
	else
		echo "FAIL: decide_checked($prev_sha, $new_sha, $old_checked) -> \"$actual\" (expected \"$expected\")"
		failures=$((failures + 1))
	fi
}

# Branch A: unchanged sha, previously checked -> stays checked.
assert_decides_checked "sha1" "sha1" "true" "true"

# Branch B: unchanged sha, previously unchecked -> stays unchecked.
assert_decides_checked "sha1" "sha1" "false" "false"

# Branch C: sha changed (page edited again) -> resets to unchecked
# even though it was previously checked.
assert_decides_checked "sha1" "sha2" "true" "false"

# Branch D: no previous sha (brand-new page in the list) -> unchecked.
assert_decides_checked "" "sha1" "true" "false"

# round_trip_state exercises the *actual* jq/grep/sed/base64 expressions
# from docs-preview.yaml end to end, which the scalar decide_checked test
# above cannot reach: it never touches the jq null-coalescing (// false,
# // null) or the base64 state marker. A path->sha map is encoded into
# the hidden marker, read back, the live checkbox glyphs are parsed, and
# the carryover jq decides each page's final checked state.
STATE_PREFIX='docs-preview-state:'

# Recovers the {path: sha} state map from the hidden marker, replicating
# the grep|sed|base64 pipeline in docs-preview.yaml.
recover_old_state() {
	local body="$1" b64
	b64=$(printf '%s\n' "$body" | grep -oE "${STATE_PREFIX}[A-Za-z0-9+/=]+" | sed "s/^${STATE_PREFIX}//") || true
	if [ -n "$b64" ]; then
		printf '%s' "$b64" | base64 -d
	else
		printf '{}'
	fi
}

# Recovers the {path: checked} map from the rendered checklist,
# replicating the grep|sed|jq pipeline in docs-preview.yaml.
recover_old_checked() {
	# shellcheck disable=SC2016 # backticks are literal Markdown code-span delimiters, not command substitution.
	printf '%s\n' "$1" |
		grep -oE '^- \[[ xX]\] \[`[^`]+`\]' |
		sed -E 's/^- \[([ xX])\] \[`([^`]+)`\]/\1\t\2/' |
		jq -R -s '[splits("\n") | select(length > 0) | split("\t") | {(.[1]): (.[0] | test("x"; "i"))}] | add // {}'
}

# Runs the carryover jq from docs-preview.yaml over the recovered maps.
decide_rows() {
	jq -n \
		--argjson eligible "$1" \
		--argjson old_state "$2" \
		--argjson old_checked "$3" \
		'[
			$eligible[] | . as $f |
			($old_state[$f.filename] // null) as $prev_sha |
			(if $prev_sha != null and $prev_sha == $f.sha
				then ($old_checked[$f.filename] // false)
				else false
			end) as $checked |
			{filename: $f.filename, sha: $f.sha, checked: $checked}
		] | sort_by(.filename)' | jq -c .
}

assert_round_trip_state() {
	local old_state_json='{"docs/a.md":"sha1","docs/b.md":"sha1","docs/d.md":"sha1"}'
	local state_b64
	state_b64=$(printf '%s' "$old_state_json" | base64 -w0)

	# A rendered comment body: a.md checked, b.md unchecked, no line for
	# d.md (present in the state marker but absent from the checklist),
	# plus the hidden state marker.
	local body
	# shellcheck disable=SC2016 # backtick-quoted paths are literal Markdown.
	body=$(printf '%s\n' \
		'## Docs preview' \
		'' \
		'- [x] [`docs/a.md`](https://coder.com/docs/@b/a)' \
		'- [ ] [`docs/b.md`](https://coder.com/docs/@b/b)' \
		'<!-- docs-preview -->' \
		"<!-- ${STATE_PREFIX}${state_b64} -->")

	# a.md: sha unchanged, was checked -> stays checked.
	# b.md: sha changed -> resets to unchecked (else branch).
	# c.md: brand-new, absent from old_state -> // null -> unchecked.
	# d.md: sha unchanged but absent from old_checked -> // false.
	local eligible_json='[{"filename":"docs/a.md","sha":"sha1"},{"filename":"docs/b.md","sha":"sha2"},{"filename":"docs/c.md","sha":"sha3"},{"filename":"docs/d.md","sha":"sha1"}]'

	local rec_state rec_checked actual expected
	rec_state=$(recover_old_state "$body")
	rec_checked=$(recover_old_checked "$body")
	actual=$(decide_rows "$eligible_json" "$rec_state" "$rec_checked")
	expected='[{"filename":"docs/a.md","sha":"sha1","checked":true},{"filename":"docs/b.md","sha":"sha2","checked":false},{"filename":"docs/c.md","sha":"sha3","checked":false},{"filename":"docs/d.md","sha":"sha1","checked":false}]'

	if [ "$actual" = "$expected" ]; then
		echo "PASS: round_trip_state carryover"
	else
		echo "FAIL: round_trip_state carryover -> $actual (expected $expected)"
		failures=$((failures + 1))
	fi
}

assert_round_trip_state

if [ "$failures" -gt 0 ]; then
	echo ""
	echo "$failures test(s) failed."
	exit 1
fi

echo ""
echo "All tests passed."
