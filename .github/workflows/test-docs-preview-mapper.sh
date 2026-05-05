#!/bin/bash
# Regression tests for the path-mapping logic in docs-preview.yaml.
# The mapper converts a repo-relative docs path into the URL path
# used by the docs site preview. Five distinct branches exist in the
# case block; every branch must be covered here.

set -euo pipefail

# map_doc_path replicates the case block from docs-preview.yaml so
# we can exercise it without running the full workflow.
map_doc_path() {
	local first_doc="$1"
	local rel="${first_doc#docs/}"
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

if [ "$failures" -gt 0 ]; then
	echo ""
	echo "$failures test(s) failed."
	exit 1
fi

echo ""
echo "All tests passed."
