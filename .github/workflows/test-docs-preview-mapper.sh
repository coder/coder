#!/bin/bash
# Regression tests for the logic-dense pieces of docs-preview.yaml: the
# path mapper (five case branches, all covered here), manifest path
# extraction, changed-file/image filtering, the eligible-set intersection,
# checkbox-state parsing, and the checked-state carryover. Where the
# workflow runs jq, these run the same jq against fixtures rather than a
# shell mirror. Keep in sync with docs-preview.yaml.

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

# normalize_manifest_path replicates the sed pipeline docs-preview.yaml runs
# over the manifest paths. Entries are written "./foo/bar.md" or "foo/bar.md"
# relative to docs/; both must normalize to "docs/foo/bar.md" to compare
# against the PR-files API filenames.
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

# parse_checkbox_line replicates the sed extraction docs-preview.yaml runs
# over the comment body to recover the live checked state a reviewer's clicks
# land in (GitHub persists a toggle as a comment-body edit). Emits
# "<x-or-space>\t<path>", the workflow's intermediate TSV.
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

# round_trip_state exercises the actual jq/grep/sed/base64 from
# docs-preview.yaml end to end: a path->sha map is encoded into the hidden
# marker, read back, the live checkbox glyphs parsed, and the carryover jq
# decides each page's final checked state.
STATE_PREFIX='docs-preview-state:'

# Recovers the {path: sha} map from the hidden marker, mirroring the guarded
# block in docs-preview.yaml: adopt the decode only if non-empty and a JSON
# object, else {}. The non-empty check keeps the outcome the same on
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

# A malformed marker must recover to {} with the run surviving. Feed markers
# that clear the charset grep but fail the decode or the object-type gate.
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
# docs-preview.yaml against manifest JSON on stdin, guarding the recursive
# path extraction that normalize_manifest_path above does not reach.
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
# docs-preview.yaml: keep only changed files whose path is in the manifest
# allowlist.
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

# filter_changed_images runs the real image filter jq from docs-preview.yaml:
# keep non-removed docs/ images outside docs/.style/, extensions matched
# case-insensitively (no sha; the image section is stateless).
filter_changed_images() {
	jq -r '.[] | select(.status != "removed") | select(.filename | test("^docs/.*\\.(png|jpe?g|gif|svg|webp|avif|bmp|ico)$"; "i")) | select((.filename | test("^docs/\\.style/")) | not) | .filename'
}

images_fixture='[
  {"filename":"docs/images/a.png","sha":"a1","status":"modified"},
  {"filename":"docs/images/user-guides/b.SVG","sha":"b1","status":"added"},
  {"filename":"docs/images/c.jpg","sha":"c1","status":"added"},
  {"filename":"docs/images/d.jpeg","sha":"d1","status":"added"},
  {"filename":"docs/images/e.gif","sha":"e1","status":"added"},
  {"filename":"docs/images/f.webp","sha":"f1","status":"added"},
  {"filename":"docs/images/g.avif","sha":"g2","status":"added"},
  {"filename":"docs/images/h.bmp","sha":"h1","status":"added"},
  {"filename":"docs/images/i.ico","sha":"i1","status":"added"},
  {"filename":"docs/images/old.gif","sha":"g1","status":"removed"},
  {"filename":"docs/.style/logo.png","sha":"s1","status":"modified"},
  {"filename":"docs/admin/notes.md","sha":"m1","status":"modified"},
  {"filename":"docs/images/clip.mp4","sha":"v1","status":"added"},
  {"filename":"site/logo.png","sha":"x1","status":"modified"}
]'
actual_images=$(printf '%s' "$images_fixture" | filter_changed_images | LC_ALL=C sort | tr '\n' '|')
# Every extension in the workflow regex (png, jpg, jpeg, gif, svg, webp,
# avif, bmp, ico) has a matching entry, so trimming any one from the regex
# fails this assertion. b.SVG also proves the match is case-insensitive.
expected_images="docs/images/a.png|docs/images/c.jpg|docs/images/d.jpeg|docs/images/e.gif|docs/images/f.webp|docs/images/g.avif|docs/images/h.bmp|docs/images/i.ico|docs/images/user-guides/b.SVG|"
if [ "$actual_images" = "$expected_images" ]; then
	echo "PASS: filter_changed_images (every extension matches; removed/.style/md/non-docs/video excluded, case-insensitive)"
else
	echo "FAIL: filter_changed_images -> $actual_images (expected $expected_images)"
	failures=$((failures + 1))
fi

# extract_ref_tokens, resolve_ref, and pages_for_image are spliced in
# verbatim from docs-preview.yaml so the mirror can't drift; the assertions
# below drive them against a throwaway docs tree. A page embeds an image only
# when a Markdown/HTML reference resolves to exactly that repo path, so prose
# mentions and same-basename collisions elsewhere don't match.
extract_ref_tokens() {
	local file="$1"
	# Markdown: capture up to the first space or ) so an optional
	# "title" after the path is dropped.
	grep -oE '!\[[^]]*\]\([^)[:space:]]+' "$file" 2>/dev/null |
		sed -E 's/^!\[[^]]*\]\(//' || true
	# HTML: single- or double-quoted src, case-insensitive tag/attr.
	grep -oiE '<img[^>]+src=("[^"]+"|'\''[^'\'']+'\'')' "$file" 2>/dev/null |
		sed -E 's/.*src=//I; s/^["'\'']//; s/["'\'']$//' || true
}

resolve_ref() {
	local file="$1" ref="$2" dir
	ref="${ref%%#*}"
	ref="${ref%%\?*}"
	ref="${ref#<}"
	ref="${ref%>}"
	[ -z "$ref" ] && return 0
	case "$ref" in
	*://* | //* | mailto:* | data:*) return 0 ;;
	esac
	dir=$(dirname "$file")
	realpath -m --relative-to="$PWD" "${PWD}/${dir}/${ref}" 2>/dev/null || true
}

pages_for_image() {
	local image="$1" base candidates file token
	base=$(basename "$image")
	candidates=$(grep -rlF "$base" docs --include='*.md' 2>/dev/null |
		grep -v '^docs/\.style/' || true)
	[ -z "$candidates" ] && return 0
	while IFS= read -r file; do
		[ -z "$file" ] && continue
		while IFS= read -r token; do
			[ -z "$token" ] && continue
			[ "$(basename "$token")" = "$base" ] || continue
			if [ "$(resolve_ref "$file" "$token")" = "$image" ]; then
				printf '%s\n' "$file"
				break
			fi
		done < <(extract_ref_tokens "$file")
	done <<<"$candidates"
}

img_fixture_root=$(mktemp -d)
img_orig_pwd=$PWD
mkdir -p \
	"$img_fixture_root/docs/user-guides" \
	"$img_fixture_root/docs/admin/sub" \
	"$img_fixture_root/docs/images/other" \
	"$img_fixture_root/docs/.style"
# Two pages embed docs/images/shared.png at different depths (one Markdown,
# one HTML): the nested-list case.
cat >"$img_fixture_root/docs/user-guides/page.md" <<'MD'
# UG

![shot](../images/shared.png)
MD
cat >"$img_fixture_root/docs/admin/sub/deep.md" <<'MD'
# Sub

<img src="../../images/shared.png" alt="x">
MD
# A different shared.png in another dir must not cross-match.
cat >"$img_fixture_root/docs/admin/collide.md" <<'MD'
# Other

![o](../images/other/shared.png)
MD
# Only a prose mention of the basename must not match.
cat >"$img_fixture_root/docs/admin/prose.md" <<'MD'
# Prose

We refreshed shared.png last week.
MD
# An external image with the same basename must be ignored.
cat >"$img_fixture_root/docs/admin/ext.md" <<'MD'
# Ext

![e](https://example.com/shared.png)
MD
# A .style page that references it is excluded by pages_for_image.
cat >"$img_fixture_root/docs/.style/tool.md" <<'MD'
# Style

![s](../images/shared.png)
MD

cd "$img_fixture_root"
actual_pfi=$(pages_for_image "docs/images/shared.png" | LC_ALL=C sort | tr '\n' '|')
expected_pfi="docs/admin/sub/deep.md|docs/user-guides/page.md|"
if [ "$actual_pfi" = "$expected_pfi" ]; then
	echo "PASS: pages_for_image (md+html refs match; prose/external/.style excluded)"
else
	echo "FAIL: pages_for_image -> $actual_pfi (expected $expected_pfi)"
	failures=$((failures + 1))
fi
actual_collide=$(pages_for_image "docs/images/other/shared.png" | LC_ALL=C sort | tr '\n' '|')
expected_collide="docs/admin/collide.md|"
if [ "$actual_collide" = "$expected_collide" ]; then
	echo "PASS: pages_for_image (same-basename images in different dirs don't cross-match)"
else
	echo "FAIL: pages_for_image collision -> $actual_collide (expected $expected_collide)"
	failures=$((failures + 1))
fi
cd "$img_orig_pwd"
rm -rf "$img_fixture_root"

# resolve_ref branch coverage: each normalization step (strip #fragment,
# ?query, <...> wrapper; reject external and protocol-relative refs) has a
# case, so deleting any one fails the suite. Path math is relative to $PWD,
# so these are independent of the real tree.
assert_resolves_ref() {
	local ref="$1" expected="$2" desc="$3" actual
	actual=$(resolve_ref "docs/user-guides/page.md" "$ref")
	if [ "$actual" = "$expected" ]; then
		echo "PASS: resolve_ref ($desc)"
	else
		echo "FAIL: resolve_ref ($desc) -> \"$actual\" (expected \"$expected\")"
		failures=$((failures + 1))
	fi
}
assert_resolves_ref "../images/shared.png" "docs/images/shared.png" "relative path"
assert_resolves_ref "../images/shared.png#top" "docs/images/shared.png" "strips #fragment"
assert_resolves_ref "../images/shared.png?v=1" "docs/images/shared.png" "strips ?query"
assert_resolves_ref "<../images/shared.png>" "docs/images/shared.png" "strips <...> wrapper"
assert_resolves_ref "https://example.com/shared.png" "" "external URL ignored"
assert_resolves_ref "//cdn.example.com/shared.png" "" "protocol-relative ignored"
assert_resolves_ref "mailto:docs@coder.com" "" "mailto ignored"
assert_resolves_ref "" "" "empty ref"

# extract_ref_tokens branch coverage: a titled Markdown ref (the space stops
# the path capture), an uppercase <IMG SRC="..."> (case-insensitive grep),
# and a single-quoted src.
ert_root=$(mktemp -d)
cat >"$ert_root/titled.md" <<'MD'
![shot](../images/shared.png "Optional title")
MD
cat >"$ert_root/upper.md" <<'MD'
<IMG SRC="../images/shared.png" alt="x">
MD
cat >"$ert_root/single.md" <<'MD'
<img src='../images/shared.png'>
MD
assert_extracts() {
	local file="$1" expected="$2" desc="$3" actual
	actual=$(extract_ref_tokens "$file" | tr '\n' '|')
	if [ "$actual" = "$expected" ]; then
		echo "PASS: extract_ref_tokens ($desc)"
	else
		echo "FAIL: extract_ref_tokens ($desc) -> \"$actual\" (expected \"$expected\")"
		failures=$((failures + 1))
	fi
}
assert_extracts "$ert_root/titled.md" "../images/shared.png|" "titled Markdown ref drops the title"
assert_extracts "$ert_root/upper.md" "../images/shared.png|" "uppercase <IMG SRC> matches case-insensitively"
assert_extracts "$ert_root/single.md" "../images/shared.png|" "single-quoted src"
rm -rf "$ert_root"

# build_image_section runs the real image_section_json jq from
# docs-preview.yaml: combine the changed-image list and "<image>\t<page>"
# pairs into [{image, pages:[...]}], images sorted, pages per image deduped
# and sorted. An image with no pair keeps pages:[], so an icon- or
# screenshot-only PR still renders.
build_image_section() {
	local images_list="$1" pairs_tsv="$2"
	jq -n \
		--argjson images "$(printf '%s\n' "$images_list" | jq -R -s '[splits("\n") | select(length > 0)]')" \
		--argjson pairs "$(printf '%s\n' "$pairs_tsv" | jq -R -s '[splits("\n") | select(length > 0) | split("\t") | {image: .[0], page: .[1]}]')" \
		'($pairs | group_by(.image) | map({key: .[0].image, value: (map(.page) | unique)}) | from_entries) as $by
		 | [$images[] | {image: ., pages: ($by[.] // [])}]
		 | sort_by(.image)'
}
# c.png is a changed image with no pair (the icon-only case); it must still
# appear, with pages: [].
images_list_fixture=$(printf 'docs/images/b.png\ndocs/images/a.png\ndocs/images/c.png')
pairs_fixture=$(printf 'docs/images/b.png\tdocs/z.md\ndocs/images/a.png\tdocs/p2.md\ndocs/images/a.png\tdocs/p1.md\ndocs/images/a.png\tdocs/p1.md')
actual_group=$(build_image_section "$images_list_fixture" "$pairs_fixture" | jq -c .)
expected_group='[{"image":"docs/images/a.png","pages":["docs/p1.md","docs/p2.md"]},{"image":"docs/images/b.png","pages":["docs/z.md"]},{"image":"docs/images/c.png","pages":[]}]'
if [ "$actual_group" = "$expected_group" ]; then
	echo "PASS: build_image_section (sorted images, de-duped sorted pages, unmapped image kept with empty pages)"
else
	echo "FAIL: build_image_section -> $actual_group (expected $expected_group)"
	failures=$((failures + 1))
fi

# build_comment_body and render_image_section mirror the body assembler in
# docs-preview.yaml, rendering the exact comment body the workflow posts so
# it can be sized by measuring real bytes not estimating. Read the
# $final_rows, $total_pages, $url_prefix, $image_section, $image_section_json,
# $DOCS_PREVIEW_MARKER, $STATE_PREFIX, and IMAGE_* globals set before each
# case below. Keep in sync with docs-preview.yaml.
DOCS_PREVIEW_MARKER='<!-- docs-preview -->'
STATE_PREFIX='docs-preview-state:'
# Representative values for the Files-tab link in the omitted-pages
# summary; the workflow supplies these from the GitHub Actions env.
REPO='owner/repo'
PR_NUMBER='123'
# Caps that bound the image section, mirrored from docs-preview.yaml.
IMAGE_SECTION_BUDGET=20000
IMAGE_PAGES_MAX=25
# Default so the page-only cases below can call build_comment_body under
# set -u; the image cases set it via render_image_section.
image_section=""

page_url() {
	local filename="$1" page_path url
	page_path=$(map_doc_path "$filename")
	url="$url_prefix"
	if [ -n "$page_path" ]; then
		url="${url}/${page_path}"
	fi
	printf '%s' "$url"
}

render_image_section() {
	local budget="$1" count intro header out="" shown=0 i=0
	local image page_count entry j page url dropped candidate
	count=$(printf '%s' "$image_section_json" | jq 'length')
	if [ "$count" -eq 0 ]; then
		return 0
	fi
	intro="These images changed. Each is listed with the navigable page(s) that embed it, so you can open the preview and see the new image in context. An image with no page listed isn't embedded by any navigable docs page. These links are informational and have no review checkboxes."
	header="#### Changed images"$'\n\n'"${intro}"$'\n\n'
	while [ "$i" -lt "$count" ]; do
		image=$(printf '%s' "$image_section_json" | jq -r --argjson i "$i" '.[$i].image')
		page_count=$(printf '%s' "$image_section_json" | jq --argjson i "$i" '.[$i].pages | length')
		# The backticks are literal Markdown code-span delimiters.
		entry="- \`${image}\`"$'\n'
		j=0
		while [ "$j" -lt "$page_count" ] && [ "$j" -lt "$IMAGE_PAGES_MAX" ]; do
			page=$(printf '%s' "$image_section_json" | jq -r --argjson i "$i" --argjson j "$j" '.[$i].pages[$j]')
			url=$(page_url "$page")
			entry="${entry}  - [\`${page}\`](${url})"$'\n'
			j=$((j + 1))
		done
		if [ "$page_count" -gt "$IMAGE_PAGES_MAX" ]; then
			entry="${entry}  - _and $((page_count - IMAGE_PAGES_MAX)) more page(s) embedding this image_"$'\n'
		fi
		# Keep the first image unconditionally (an empty section under a
		# header is self-contradicting); measure each later image's
		# would-be section and stop before it exceeds budget.
		if [ "$shown" -gt 0 ]; then
			candidate="${header}${out}${entry}"
			if [ "$(printf '%s' "$candidate" | LC_ALL=C wc -c)" -gt "$budget" ]; then
				break
			fi
		fi
		out="${out}${entry}"
		shown=$((shown + 1))
		i=$((i + 1))
	done
	dropped=$((count - shown))
	if [ "$dropped" -gt 0 ]; then
		out="${out}"$'\n'"_and ${dropped} more changed image(s) not listed to stay under GitHub's comment size limit. See the [Files tab](https://github.com/${REPO}/pull/${PR_NUMBER}/files) for the full list._"$'\n'
	fi
	printf '%s%s' "$header" "$out"
}

build_comment_body() {
	local n="$1" rows state_json state_b64 checklist="" intro page_block=""
	local filename checked url box omitted

	rows=$(printf '%s' "$final_rows" | jq -c --argjson n "$n" '.[:$n]')
	state_json=$(printf '%s' "$rows" | jq -c 'map({(.filename): .sha}) | add // {}')
	state_b64=$(printf '%s' "$state_json" | base64 -w0)

	if [ "$total_pages" -gt 0 ]; then
		while IFS=$'\t' read -r filename checked; do
			[ -z "$filename" ] && continue
			url=$(page_url "$filename")
			box=" "
			if [ "$checked" = "true" ]; then
				box="x"
			fi
			# The backticks are literal Markdown code-span delimiters.
			checklist="${checklist}- [${box}] [\`${filename}\`](${url})"$'\n'
		done < <(printf '%s' "$rows" | jq -r '.[] | [.filename, (.checked | tostring)] | @tsv')

		omitted=$((total_pages - n))
		if [ "$omitted" -gt 0 ]; then
			checklist="${checklist}"$'\n'"_and ${omitted} more changed page(s) not listed to stay under GitHub's comment size limit. See the [Files tab](https://github.com/${REPO}/pull/${PR_NUMBER}/files) for the full list._"$'\n'
		fi

		intro="Check off each page once it's been reviewed. If a page changes in a later push, its checkbox clears automatically so it gets a fresh look. Pages not yet wired into the docs navigation aren't listed here."
		page_block="${intro}"$'\n\n'"${checklist}"
	fi

	# The identity and state markers are always present so the comment
	# stays discoverable and round-trippable, even on an image-only PR
	# whose state map is {}.
	{
		printf '## Docs preview\n\n'
		if [ -n "$page_block" ]; then
			printf '%s\n' "$page_block"
		fi
		if [ -n "$image_section" ]; then
			printf '%s\n' "$image_section"
		fi
		printf '%s\n' "$DOCS_PREVIEW_MARKER"
		printf '<!-- %s%s -->' "$STATE_PREFIX" "$state_b64"
	}
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

# Repo-scale worst case: a 400-page docs migration on a long branch with
# ~60-char paths. The cap must keep the real body under GitHub's 65536-char
# limit while still listing as many pages as fit.
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

# render_image_section renders a nested list of each changed image with a
# preview link for every page that embeds it. A docs-root page
# (docs/README.md) links to the branch root with no trailing path.
url_prefix="https://coder.com/docs/@branch"
IMAGE_PAGES_MAX=25
image_section_json='[{"image":"docs/images/a.png","pages":["docs/user-guides/x.md","docs/y.md"]},{"image":"docs/images/b.png","pages":["docs/README.md"]}]'
section=$(render_image_section "$IMAGE_SECTION_BUDGET")
# shellcheck disable=SC2016 # backtick-quoted paths are literal Markdown.
if printf '%s' "$section" | grep -qF '#### Changed images' &&
	printf '%s' "$section" | grep -qF -- '- `docs/images/a.png`' &&
	printf '%s' "$section" | grep -qF -- '  - [`docs/user-guides/x.md`](https://coder.com/docs/@branch/user-guides/x)' &&
	printf '%s' "$section" | grep -qF -- '  - [`docs/README.md`](https://coder.com/docs/@branch)'; then
	echo "PASS: render_image_section (nested preview links; README maps to docs root)"
else
	echo "FAIL: render_image_section basic ->"
	printf '%s\n' "$section"
	failures=$((failures + 1))
fi

# Per-image page cap: at most IMAGE_PAGES_MAX links, then an "and N more"
# note. Page-link lines start with two spaces, a dash and a bracket; the
# note line starts with an underscore, so ^  - \[ counts only the links.
IMAGE_PAGES_MAX=2
many_pages=$(jq -nc '[range(5) | "docs/p\(.).md"]')
image_section_json=$(jq -nc --argjson p "$many_pages" '[{image: "docs/images/big.png", pages: $p}]')
section=$(render_image_section "$IMAGE_SECTION_BUDGET")
shown_links=$(printf '%s\n' "$section" | grep -cE '^  - \[')
if [ "$shown_links" -eq 2 ] && printf '%s' "$section" | grep -qF 'and 3 more page(s) embedding this image'; then
	echo "PASS: render_image_section (per-image page cap keeps 2 with an overflow note)"
else
	echo "FAIL: render_image_section page cap -> shown_links=$shown_links"
	printf '%s\n' "$section"
	failures=$((failures + 1))
fi
IMAGE_PAGES_MAX=25

# Byte-budget truncation across images: with a 1-byte budget only the
# first image (always kept) renders, and a trailing "and N more image(s)"
# note reports the rest.
image_section_json='[{"image":"docs/images/aaaaaaaaaa.png","pages":["docs/one.md"]},{"image":"docs/images/bbbbbbbbbb.png","pages":["docs/two.md"]},{"image":"docs/images/cccccccccc.png","pages":["docs/three.md"]}]'
section=$(render_image_section 1)
# shellcheck disable=SC2016 # backtick is a literal Markdown delimiter.
imgs_shown=$(printf '%s\n' "$section" | grep -cE '^- `docs/images/')
if [ "$imgs_shown" -eq 1 ] && printf '%s' "$section" | grep -qF 'and 2 more changed image(s) not listed'; then
	echo "PASS: render_image_section (byte-budget truncation keeps first image + note)"
else
	echo "FAIL: render_image_section truncation -> imgs_shown=$imgs_shown"
	printf '%s\n' "$section"
	failures=$((failures + 1))
fi

# build_comment_body on an image-only PR (zero changed pages): the
# checklist and its intro are omitted, the image section renders, both
# hidden markers are present, and the state map is {}.
final_rows='[]'
total_pages=0
url_prefix="https://coder.com/docs/@branch"
image_section_json='[{"image":"docs/images/a.png","pages":["docs/user-guides/x.md"]}]'
image_section=$(render_image_section "$IMAGE_SECTION_BUDGET")
image_only_body=$(build_comment_body 0)
if printf '%s' "$image_only_body" | grep -qF '#### Changed images' &&
	! printf '%s' "$image_only_body" | grep -qF 'Check off each page' &&
	printf '%s' "$image_only_body" | grep -qF 'These links are informational and have no review checkboxes.' &&
	! printf '%s' "$image_only_body" | grep -qF 'separate from the checklist above' &&
	printf '%s' "$image_only_body" | grep -qF '<!-- docs-preview -->' &&
	[ "$(recover_old_state "$image_only_body")" = '{}' ]; then
	echo "PASS: build_comment_body (image-only PR: image section, no checklist, empty state)"
else
	echo "FAIL: build_comment_body image-only ->"
	printf '%s\n' "$image_only_body"
	failures=$((failures + 1))
fi

# Icon-only PR: the only changed image embeds no navigable page, so
# image_section_json carries it with pages:[]. The comment must still render,
# listing the image on its own line with no sub-links.
final_rows='[]'
total_pages=0
url_prefix="https://coder.com/docs/@branch"
image_section_json='[{"image":"docs/images/icons/system.svg","pages":[]}]'
image_section=$(render_image_section "$IMAGE_SECTION_BUDGET")
icon_only_body=$(build_comment_body 0)
# shellcheck disable=SC2016 # backtick-quoted path is literal Markdown.
if printf '%s' "$icon_only_body" | grep -qF '#### Changed images' &&
	printf '%s' "$icon_only_body" | grep -qF -- '- `docs/images/icons/system.svg`' &&
	! printf '%s' "$icon_only_body" | grep -qF 'Check off each page' &&
	[ "$(printf '%s\n' "$icon_only_body" | grep -cE '^  - \[')" -eq 0 ]; then
	echo "PASS: build_comment_body (icon-only PR: unmapped image still produces a comment)"
else
	echo "FAIL: build_comment_body icon-only ->"
	printf '%s\n' "$icon_only_body"
	failures=$((failures + 1))
fi

# build_comment_body with both changed pages and images: both sections
# render. A page that is both changed and an image referrer appears in both,
# and the image sub-link (no checkbox glyph) never pollutes carried-over
# checkbox state.
final_rows='[{"filename":"docs/a.md","sha":"s1","checked":true},{"filename":"docs/b.md","sha":"s2","checked":false}]'
total_pages=2
image_section_json='[{"image":"docs/images/a.png","pages":["docs/a.md"]}]'
image_section=$(render_image_section "$IMAGE_SECTION_BUDGET")
both_body=$(build_comment_body 2)
# shellcheck disable=SC2016 # backtick-quoted paths are literal Markdown.
if printf '%s' "$both_body" | grep -qF 'Check off each page' &&
	printf '%s' "$both_body" | grep -qF -- '- [x] [`docs/a.md`]' &&
	printf '%s' "$both_body" | grep -qF '#### Changed images' &&
	printf '%s' "$both_body" | grep -qF -- '  - [`docs/a.md`](https://coder.com/docs/@branch/a)'; then
	echo "PASS: build_comment_body (pages and images both render)"
else
	echo "FAIL: build_comment_body pages+images ->"
	printf '%s\n' "$both_body"
	failures=$((failures + 1))
fi
if [ "$(recover_old_checked "$both_body" | jq -cS .)" = '{"docs/a.md":true,"docs/b.md":false}' ]; then
	echo "PASS: build_comment_body (image sub-links excluded from checkbox state)"
else
	echo "FAIL: image sub-links polluted checkbox state -> $(recover_old_checked "$both_body")"
	failures=$((failures + 1))
fi

if [ "$failures" -gt 0 ]; then
	echo ""
	echo "$failures test(s) failed."
	exit 1
fi

echo ""
echo "All tests passed."
