#!/usr/bin/env bash

# This script resolves the latest Coder release version suitable for Iron Bank
# by parsing the release calendar table in docs/install/releases/index.md.
#
# It identifies releases marked as "Stable" or "Extended Support Release" (ESR)
# and returns the highest version number among them. ESR releases that are also
# in an active channel (e.g. "Mainline (ESR)") are included as well.
#
# Usage:
#   ./resolve-latest-release.sh [--index-url URL]
#
# Options:
#   --index-url URL   Override the URL used to fetch the release index.md file.
#                     Defaults to the raw file from GitHub main branch.
#   --index-file FILE Read from a local file instead of fetching from a URL.
#
# Output:
#   Prints the resolved version string (e.g. "v2.34.0") to stdout.
#   Exits with code 1 if no suitable release is found.

set -euo pipefail

DEFAULT_INDEX_URL="https://raw.githubusercontent.com/coder/coder/main/docs/install/releases/index.md"

index_url="${DEFAULT_INDEX_URL}"
index_file=""

while [[ $# -gt 0 ]]; do
	case "$1" in
	--index-url)
		index_url="$2"
		shift 2
		;;
	--index-file)
		index_file="$2"
		shift 2
		;;
	*)
		echo "Unknown option: $1" >&2
		exit 1
		;;
	esac
done

# Fetch the index.md content.
if [[ -n "$index_file" ]]; then
	if [[ ! -f "$index_file" ]]; then
		echo "Error: file '$index_file' does not exist" >&2
		exit 1
	fi
	index_content="$(cat "$index_file")"
else
	index_content="$(curl -fsSL "$index_url")"
fi

# Parse the release calendar table between the markers.
# Extract rows that have a status containing "Stable" or "ESR"
# (covers "Extended Support Release", "Mainline (ESR)", "Stable (ESR)", etc.)
# Then extract the latest release version from the "Latest Release" column.
#
# Table format:
# | Release name | Release Date | Status | Latest Release |
# | [2.33](...) | May 05, 2026 | Stable | [v2.33.6](...) |
# | [2.34](...) | June 02, 2026 | Mainline (ESR) | [v2.34.0](...) |

best_version=""
best_major=0
best_minor=0
best_patch=0

# Process each table row between the calendar markers.
in_calendar=false
while IFS= read -r line; do
	if [[ "$line" == *"RELEASE_CALENDAR_START"* ]]; then
		in_calendar=true
		continue
	fi
	if [[ "$line" == *"RELEASE_CALENDAR_END"* ]]; then
		in_calendar=false
		continue
	fi

	if ! $in_calendar; then
		continue
	fi

	# Skip header and separator rows.
	if [[ "$line" == *"Release name"* ]] || [[ "$line" == *"---"* ]]; then
		continue
	fi

	# Skip rows that are not table rows.
	if [[ "$line" != "|"* ]]; then
		continue
	fi

	# Split the row into columns by pipe delimiter.
	# Column layout: | Release name | Release Date | Status | Latest Release |
	status="$(echo "$line" | awk -F'|' '{print $4}' | xargs)"
	latest_release="$(echo "$line" | awk -F'|' '{print $5}' | xargs)"

	# Check if this is a Stable or ESR release.
	if [[ "$status" != *"Stable"* ]] && [[ "$status" != *"ESR"* ]] && [[ "$status" != *"Extended Support"* ]]; then
		continue
	fi

	# Extract version from the latest release column.
	# Format: [v2.33.6](https://github.com/coder/coder/releases/tag/v2.33.6)
	# or just: N/A
	if [[ "$latest_release" == "N/A" ]] || [[ -z "$latest_release" ]]; then
		continue
	fi

	# Extract the version string (e.g. "v2.33.6") from markdown link.
	version="$(echo "$latest_release" | grep -oP 'v[0-9]+\.[0-9]+\.[0-9]+' | head -1)"
	if [[ -z "$version" ]]; then
		continue
	fi

	# Parse major.minor.patch for comparison.
	major="$(echo "$version" | sed 's/^v//' | cut -d. -f1)"
	minor="$(echo "$version" | sed 's/^v//' | cut -d. -f2)"
	patch="$(echo "$version" | sed 's/^v//' | cut -d. -f3)"

	# Compare versions: pick the largest.
	if [[ "$major" -gt "$best_major" ]] ||
		[[ "$major" -eq "$best_major" && "$minor" -gt "$best_minor" ]] ||
		[[ "$major" -eq "$best_major" && "$minor" -eq "$best_minor" && "$patch" -gt "$best_patch" ]]; then
		best_version="$version"
		best_major="$major"
		best_minor="$minor"
		best_patch="$patch"
	fi
done <<<"$index_content"

if [[ -z "$best_version" ]]; then
	echo "Error: no Stable or ESR release found in the release calendar" >&2
	exit 1
fi

echo "$best_version"
