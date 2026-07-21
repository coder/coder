#!/bin/bash
# Regression tests for the path-mapping logic in docs-preview.yaml.
# The mapper converts a repo-relative docs path into the URL path
# used by the docs site preview. Five distinct branches exist in the
# case block; every branch must be covered here.
#
# Also covers the other logic-dense pieces of docs-preview.yaml:
# extracting page paths from docs/manifest.json, filtering the PR's
# changed files, intersecting the two into the eligible set, parsing
# checkbox state out of the rendered checklist, and the checked-state
# carryover. Where the workflow runs jq, these tests run the same jq
# against fixtures rather than a shell mirror. Keep them in sync with
# docs-preview.yaml.

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
	printf '%s\n' "$1" | grep -oE '^[[:space:]]*- \[[ xX]\] \[`[^`]+`\]' | sed -E 's/^[[:space:]]*- \[([ xX])\] \[`([^`]+)`\]/\1\t\2/' || true
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

# decide_checked removed: round_trip_state below covers the carryover
# rule through the workflow's real jq, so the hand-written shell mirror
# only added a green check that guarded nothing.

# round_trip_state exercises the *actual* jq/grep/sed/base64 expressions
# from docs-preview.yaml end to end, which a hand-written shell mirror of
# the carryover rule could not: it drives the jq null-coalescing
# (// false, // null) and the base64 state marker directly. A path->sha
# map is encoded into the hidden marker, read back, the live checkbox
# glyphs are parsed, and the carryover jq decides each page's final
# checked state.
STATE_PREFIX='docs-preview-state:'

# Recovers the {path: sha} state map from the hidden marker, a faithful
# copy of the guarded block in docs-preview.yaml: decode under
# `2>/dev/null || true` and adopt the result only if it is non-empty and
# parses as a JSON object, else degrade to {} so a corrupt marker can't
# kill the run. The non-empty check keeps the guard's outcome the same on
# jq < 1.7, where `jq -e` exits 0 on empty input.
recover_old_state() {
	local body="$1" b64 decoded
	b64=$(printf '%s\n' "$body" | grep -oE "${STATE_PREFIX}[A-Za-z0-9+/=]+" | sed "s/^${STATE_PREFIX}//") || true
	if [ -n "$b64" ]; then
		decoded=$(printf '%s' "$b64" | base64 -d 2>/dev/null || true)
		if [ -n "$decoded" ] && printf '%s' "$decoded" | jq -e 'type == "object"' >/dev/null 2>&1; then
			printf '%s' "$decoded"
			return
		fi
	fi
	printf '{}'
}

# Recovers the {path: checked} map from the rendered checklist,
# replicating the grep|sed|jq pipeline in docs-preview.yaml.
recover_old_checked() {
	# shellcheck disable=SC2016 # backticks are literal Markdown code-span delimiters, not command substitution.
	printf '%s\n' "$1" |
		grep -oE '^[[:space:]]*- \[[ xX]\] \[`[^`]+`\]' |
		sed -E 's/^[[:space:]]*- \[([ xX])\] \[`([^`]+)`\]/\1\t\2/' |
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
	local old_state_json='{"docs/a.md":"sha1","docs/b.md":"sha1","docs/c.md":"sha1","docs/e.md":"sha1"}'
	local state_b64
	state_b64=$(printf '%s' "$old_state_json" | base64 -w0)

	# A rendered comment body with the hidden state marker: a.md checked,
	# b.md and c.md unchecked, and no checklist line for e.md (it is in
	# the state marker but absent from the list).
	local body
	# shellcheck disable=SC2016 # backtick-quoted paths are literal Markdown.
	body=$(printf '%s\n' \
		'## Docs preview' \
		'' \
		'- [x] [`docs/a.md`](https://coder.com/docs/@b/a)' \
		'- [ ] [`docs/b.md`](https://coder.com/docs/@b/b)' \
		'- [x] [`docs/c.md`](https://coder.com/docs/@b/c)' \
		'<!-- docs-preview -->' \
		"<!-- ${STATE_PREFIX}${state_b64} -->")

	# a.md: sha unchanged, was checked      -> stays checked.
	# b.md: sha unchanged, was unchecked    -> stays unchecked.
	# c.md: sha changed, was checked        -> resets to unchecked.
	# d.md: brand-new, absent from state    -> // null -> unchecked.
	# e.md: sha unchanged, absent from list -> // false -> unchecked.
	local eligible_json='[{"filename":"docs/a.md","sha":"sha1"},{"filename":"docs/b.md","sha":"sha1"},{"filename":"docs/c.md","sha":"sha2"},{"filename":"docs/d.md","sha":"sha9"},{"filename":"docs/e.md","sha":"sha1"}]'

	local rec_state rec_checked actual expected
	rec_state=$(recover_old_state "$body")
	rec_checked=$(recover_old_checked "$body")
	actual=$(decide_rows "$eligible_json" "$rec_state" "$rec_checked")
	expected='[{"filename":"docs/a.md","sha":"sha1","checked":true},{"filename":"docs/b.md","sha":"sha1","checked":false},{"filename":"docs/c.md","sha":"sha2","checked":false},{"filename":"docs/d.md","sha":"sha9","checked":false},{"filename":"docs/e.md","sha":"sha1","checked":false}]'

	if [ "$actual" = "$expected" ]; then
		echo "PASS: round_trip_state carryover"
	else
		echo "FAIL: round_trip_state carryover -> $actual (expected $expected)"
		failures=$((failures + 1))
	fi
}

assert_round_trip_state

# The malformed-marker path the decode guard added must recover to {} with
# the run surviving. Feed markers that clear the charset grep but fail the
# decode or the object-type gate.
assert_marker_recovers() {
	local marker="$1" expected="$2" desc="$3" body actual
	body=$(printf '## Docs preview\n<!-- docs-preview -->\n<!-- %s%s -->' "$STATE_PREFIX" "$marker")
	actual=$(recover_old_state "$body")
	if [ "$actual" = "$expected" ]; then
		echo "PASS: recover_old_state ($desc) -> $expected"
	else
		echo "FAIL: recover_old_state ($desc) -> $actual (expected $expected)"
		failures=$((failures + 1))
	fi
}

# A valid object marker recovers to the object verbatim.
assert_marker_recovers "$(printf '{"docs/a.md":"sha1"}' | base64 -w0)" '{"docs/a.md":"sha1"}' "valid object"
# Charset-valid but undecodable base64 (odd length) degrades to {}.
assert_marker_recovers "A" "{}" "undecodable base64"
# Valid base64 of a non-object (a JSON string) fails the type gate -> {}.
assert_marker_recovers "$(printf '"hello"' | base64 -w0)" "{}" "valid base64 non-object"
# Valid base64 that decodes to non-JSON bytes fails the parse gate -> {}.
assert_marker_recovers "$(printf '\xff\xfe\xfd' | base64 -w0)" "{}" "valid base64 non-JSON bytes"

# extract_manifest_paths runs the real jq + sed pipeline from
# docs-preview.yaml against manifest JSON on stdin, emitting one
# normalized repo-relative path per line. Guards the recursive
# `[.. | objects | select(has("path")) | .path]` extraction that
# normalize_manifest_path above does not reach.
extract_manifest_paths() {
	jq -r '[.. | objects | select(has("path")) | .path] | .[]' |
		sed -E 's#^\./##; s#^#docs/#'
}

# Manifest fixture in the real schema: "./"-prefixed and bare paths, a
# nested child, and an object with only icon_path (no "path" key) that
# must not be collected.
manifest_fixture='{"versions":["main"],"routes":[
  {"title":"Home","path":"./README.md","icon_path":"./images/home.svg"},
  {"title":"Install","path":"./install/index.md","children":[
    {"title":"CLI","path":"reference/cli/whoami.md"}
  ]},
  {"title":"IconOnly","icon_path":"./images/x.svg"}
]}'
actual_paths=$(printf '%s' "$manifest_fixture" | extract_manifest_paths | LC_ALL=C sort | tr '\n' ' ')
expected_paths="docs/README.md docs/install/index.md docs/reference/cli/whoami.md "
if [ "$actual_paths" = "$expected_paths" ]; then
	echo "PASS: extract_manifest_paths (icon_path-only object excluded)"
else
	echo "FAIL: extract_manifest_paths -> \"$actual_paths\" (expected \"$expected_paths\")"
	failures=$((failures + 1))
fi

# filter_changed_files runs the real pulls/files filter jq from
# docs-preview.yaml: keep non-removed docs/*.md outside docs/.style/,
# emitting <filename>\t<sha>.
filter_changed_files() {
	jq -r '.[] | select(.status != "removed") | select(.filename | test("^docs/.*\\.md$")) | select((.filename | test("^docs/\\.style/")) | not) | [.filename, .sha] | @tsv'
}

files_fixture='[
  {"filename":"docs/admin/index.md","sha":"aaa","status":"modified"},
  {"filename":"docs/ai-coder/tasks.md","sha":"bbb","status":"added"},
  {"filename":"docs/old.md","sha":"ccc","status":"removed"},
  {"filename":"docs/.style/word-list.txt","sha":"ddd","status":"modified"},
  {"filename":"docs/images/diagram.png","sha":"eee","status":"added"},
  {"filename":"site/README.md","sha":"fff","status":"modified"},
  {"filename":"docs/.style/rules.md","sha":"ggg","status":"modified"}
]'
actual_changed=$(printf '%s' "$files_fixture" | filter_changed_files | LC_ALL=C sort | tr '\n' '|')
expected_changed="$(printf 'docs/admin/index.md\taaa\ndocs/ai-coder/tasks.md\tbbb\n' | tr '\n' '|')"
if [ "$actual_changed" = "$expected_changed" ]; then
	echo "PASS: filter_changed_files (removed/.style/non-md/non-docs excluded)"
else
	echo "FAIL: filter_changed_files -> \"$actual_changed\" (expected \"$expected_changed\")"
	failures=$((failures + 1))
fi

# intersect_eligible replicates the grep -qxF intersection from
# docs-preview.yaml: keep only changed files whose path is in the
# manifest allowlist. This is the single decision the feature exists to
# make, so cover it directly.
intersect_eligible() {
	local changed="$1" allowed="$2"
	printf '%s\n' "$changed" | while IFS=$'\t' read -r filename sha; do
		[ -z "$filename" ] && continue
		if printf '%s\n' "$allowed" | grep -qxF "$filename"; then
			printf '%s\t%s\n' "$filename" "$sha"
		fi
	done
}

changed_tsv_fixture="$(printf 'docs/admin/index.md\taaa\ndocs/ai-coder/tasks.md\tbbb\ndocs/not-in-manifest.md\tccc')"
allowed_fixture="$(printf 'docs/admin/index.md\ndocs/ai-coder/tasks.md\ndocs/install/index.md')"
actual_eligible=$(intersect_eligible "$changed_tsv_fixture" "$allowed_fixture" | LC_ALL=C sort | tr '\n' '|')
expected_eligible="$(printf 'docs/admin/index.md\taaa\ndocs/ai-coder/tasks.md\tbbb\n' | tr '\n' '|')"
if [ "$actual_eligible" = "$expected_eligible" ]; then
	echo "PASS: intersect_eligible (drops paths not in the manifest)"
else
	echo "FAIL: intersect_eligible -> \"$actual_eligible\" (expected \"$expected_eligible\")"
	failures=$((failures + 1))
fi

# build_comment_body mirrors the body assembler in docs-preview.yaml:
# it renders the first N pages of $final_rows into the exact
# comment body the workflow posts, so the comment can be sized by
# measuring the real bytes instead of estimating a per-page cost. Reads
# the $final_rows, $total_pages, $url_prefix, $DOCS_PREVIEW_MARKER, and
# $STATE_PREFIX globals set before each case below. Keep in sync with
# docs-preview.yaml.
DOCS_PREVIEW_MARKER='<!-- docs-preview -->'
STATE_PREFIX='docs-preview-state:'
# Representative values for the Files-tab link in the omitted-pages
# summary; the workflow supplies these from the GitHub Actions env.
REPO='owner/repo'
PR_NUMBER='123'
build_comment_body() {
	local n="$1" rows state_json state_b64 checklist="" intro
	local filename checked page_path url box omitted

	rows=$(printf '%s' "$final_rows" | jq -c --argjson n "$n" '.[:$n]')
	state_json=$(printf '%s' "$rows" | jq -c 'map({(.filename): .sha}) | add // {}')
	state_b64=$(printf '%s' "$state_json" | base64 -w0)

	while IFS=$'\t' read -r filename checked; do
		[ -z "$filename" ] && continue
		page_path=$(map_doc_path "$filename")
		url="$url_prefix"
		if [ -n "$page_path" ]; then
			url="${url}/${page_path}"
		fi
		box=" "
		if [ "$checked" = "true" ]; then
			box="x"
		fi
		checklist="${checklist}- [${box}] [\`${filename}\`](${url})"$'\n'
	done < <(printf '%s' "$rows" | jq -r '.[] | [.filename, (.checked | tostring)] | @tsv')

	omitted=$((total_pages - n))
	if [ "$omitted" -gt 0 ]; then
		checklist="${checklist}"$'\n'"_and ${omitted} more changed page(s) not listed to stay under GitHub's comment size limit. See the [Files tab](https://github.com/${REPO}/pull/${PR_NUMBER}/files) for the full list._"$'\n'
	fi

	intro="Check off each page once it's been reviewed. If a page changes in a later push, its checkbox clears automatically so it gets a fresh look. Pages not yet wired into the docs navigation aren't listed here."

	printf '## Docs preview\n\n%s\n\n%s\n%s\n<!-- %s%s -->' \
		"$intro" "$checklist" "$DOCS_PREVIEW_MARKER" "$STATE_PREFIX" "$state_b64"
}

# cap_pages mirrors the measure-and-binary-search cap in docs-preview.yaml:
# keep every page if the whole body fits, else the largest leading prefix
# whose rendered body stays under $budget.
cap_pages() {
	local budget="$1" keep lo hi mid
	if [ "$(build_comment_body "$total_pages" | LC_ALL=C wc -c)" -le "$budget" ]; then
		printf '%s' "$total_pages"
		return
	fi
	lo=0
	hi=$((total_pages - 1))
	keep=0
	while [ "$lo" -le "$hi" ]; do
		mid=$(((lo + hi) / 2))
		if [ "$(build_comment_body "$mid" | LC_ALL=C wc -c)" -le "$budget" ]; then
			keep=$mid
			lo=$((mid + 1))
		else
			hi=$((mid - 1))
		fi
	done
	printf '%s' "$keep"
}

budget=65000
# GitHub's hard comment-body limit; the budget above leaves headroom under it.
github_comment_limit=65536

# Repo-scale worst case: a docs migration touching 400 pages on a long
# ticket-prefixed branch, ~60-char paths, the shape reviewers measured
# overflowing the old per-page estimate. The cap must keep the real body
# under GitHub's 65536-char limit while still listing as many pages as fit.
url_prefix="https://coder.com/docs/@feature-team-very-long-branch-name-docs-migration-2024"
final_rows=$(jq -nc '[range(400) | {
	filename: ("docs/reference/generated/section-\(. + 1000)/really-long-page-name-\(. + 1000).md"),
	sha: ("0123456789abcdef0123456789abcdef" + (. + 100000 | tostring)),
	checked: false
}]')
total_pages=$(printf '%s' "$final_rows" | jq 'length')

keep=$(cap_pages "$budget")
final_body_bytes=$(build_comment_body "$keep" | LC_ALL=C wc -c)
if [ "$keep" -lt "$total_pages" ] && [ "$final_body_bytes" -le "$github_comment_limit" ]; then
	echo "PASS: repo-scale cap keeps $keep/$total_pages pages, body ${final_body_bytes}B <= ${github_comment_limit}"
else
	echo "FAIL: repo-scale cap keeps $keep/$total_pages pages, body ${final_body_bytes}B (want < total and <= ${github_comment_limit})"
	failures=$((failures + 1))
fi

# Tightness: one page past the cap must exceed the budget, proving the
# cap doesn't leave usable space on the table.
over_body_bytes=$(build_comment_body "$((keep + 1))" | LC_ALL=C wc -c)
if [ "$over_body_bytes" -gt "$budget" ]; then
	echo "PASS: cap is tight (keep+1 body ${over_body_bytes}B > ${budget})"
else
	echo "FAIL: cap is not tight (keep+1 body ${over_body_bytes}B <= ${budget})"
	failures=$((failures + 1))
fi

# A small PR keeps every page and renders no omitted-pages summary line.
url_prefix="https://coder.com/docs/@short-branch"
final_rows=$(jq -nc '[range(5) | {filename: ("docs/page-\(.).md"), sha: "abc", checked: false}]')
total_pages=$(printf '%s' "$final_rows" | jq 'length')
keep=$(cap_pages "$budget")
small_body=$(build_comment_body "$keep")
if [ "$keep" -eq 5 ] && ! printf '%s' "$small_body" | grep -q "more changed page"; then
	echo "PASS: small PR keeps all 5 pages with no summary line"
else
	echo "FAIL: small PR keep=$keep (expected 5) or unexpected summary line"
	failures=$((failures + 1))
fi

# Round-trip build_comment_body's *own emitted* marker back through
# recover_old_state, proving the producer and consumer marker formats agree
# (a drift would silently reset every checkbox on every push).
emitted_state=$(recover_old_state "$small_body")
expected_state=$(printf '%s' "$final_rows" | jq -c 'map({(.filename): .sha}) | add // {}')
if [ "$emitted_state" = "$expected_state" ]; then
	echo "PASS: emitted marker round-trips through recovery"
else
	echo "FAIL: emitted marker round-trip -> $emitted_state (expected $expected_state)"
	failures=$((failures + 1))
fi

if [ "$failures" -gt 0 ]; then
	echo ""
	echo "$failures test(s) failed."
	exit 1
fi

echo ""
echo "All tests passed."
