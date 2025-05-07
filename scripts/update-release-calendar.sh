#!/bin/bash

set -euo pipefail

# This script automatically updates the release calendar in docs/install/releases/index.md
# It updates the status of each release (Not Supported, Security Support, Stable, Mainline, Not Released)
# and gets the release dates from the first published tag for each minor release.

DOCS_FILE="docs/install/releases/index.md"

CALENDAR_START_MARKER="<!-- RELEASE_CALENDAR_START -->"
CALENDAR_END_MARKER="<!-- RELEASE_CALENDAR_END -->"

current_date=$(date +"%Y-%m-%d")
current_month=$(date +"%m")
current_year=$(date +"%Y")

# Format date as "Month DD, YYYY"
format_date() {
	date -d "$1" +"%B %d, %Y"
}

get_latest_patch() {
	local version_major=$1
	local version_minor=$2
	local tags
	local latest

	# Get all tags for this minor version
	tags=$(cd "$(git rev-parse --show-toplevel)" && git tag | grep "^v$version_major\\.$version_minor\\." | sort -V)

	latest=$(echo "$tags" | tail -1)

	if [ -z "$latest" ]; then
		echo ""
	else
		echo "${latest#v}"
	fi
}

get_first_patch() {
	local version_major=$1
	local version_minor=$2
	local tags
	local first

	# Get all tags for this minor version
	tags=$(cd "$(git rev-parse --show-toplevel)" && git tag | grep "^v$version_major\\.$version_minor\\." | sort -V)

	first=$(echo "$tags" | head -1)

	if [ -z "$first" ]; then
		echo ""
	else
		echo "${first#v}"
	fi
}

get_release_date() {
	local version_major=$1
	local version_minor=$2
	local first_patch
	local tag_date

	# Get the first patch release
	first_patch=$(get_first_patch "$version_major" "$version_minor")
	
	if [ -z "$first_patch" ]; then
		# No release found
		echo ""
		return
	fi
	
	# Get the tag date from git
	tag_date=$(cd "$(git rev-parse --show-toplevel)" && git log -1 --format=%ai "v$first_patch" 2>/dev/null || echo "")
	
	if [ -z "$tag_date" ]; then
		echo ""
	else
		# Extract date in YYYY-MM-DD format
		date -d "$tag_date" +"%Y-%m-%d"
	fi
}

# Generate releases table showing:
# - 3 previous unsupported releases
# - 1 security support release (n-2)
# - 1 stable release (n-1)
# - 1 mainline release (n)
# - 1 next release (n+1)
generate_release_calendar() {
	local result=""
	local version_major=2
	local latest_version
	local version_minor
	local start_minor

	# Find the current minor version by looking at the last mainline release tag
	latest_version=$(cd "$(git rev-parse --show-toplevel)" && git tag | grep '^v[0-9]*\.[0-9]*\.[0-9]*$' | sort -V | tail -1)
	version_minor=$(echo "$latest_version" | cut -d. -f2)

	# Start with 3 unsupported releases back
	start_minor=$((version_minor - 5))

	result="| Release name | Release Date | Status | Latest Release |\n"
	result+="|--------------|--------------|--------|----------------|\n"

	# Generate rows for each release (7 total: 3 unsupported, 1 security, 1 stable, 1 mainline, 1 next)
	for i in {0..6}; do
		# Calculate release minor version
		local rel_minor=$((start_minor + i))
		local version_name="$version_major.$rel_minor"
		local actual_release_date
		local formatted_date
		local latest_patch
		local patch_link
		local status
		local formatted_version_name

		# Determine status based on position
		if [[ $i -eq 6 ]]; then
			status="Not Released"
		elif [[ $i -eq 5 ]]; then
			status="Mainline"
		elif [[ $i -eq 4 ]]; then
			status="Stable"
		elif [[ $i -eq 3 ]]; then
			status="Security Support"
		else
			status="Not Supported"
		fi

		# Get the actual release date from the first published tag
		if [[ "$status" != "Not Released" ]]; then
			actual_release_date=$(get_release_date "$version_major" "$rel_minor")
			
			# Format the release date if we have one
			if [ -n "$actual_release_date" ]; then
				formatted_date=$(format_date "$actual_release_date")
			else
				# If no release date found, just display TBD
				formatted_date="TBD"
			fi
		fi

		# Get latest patch version
		latest_patch=$(get_latest_patch "$version_major" "$rel_minor")
		if [ -n "$latest_patch" ]; then
			patch_link="[v${latest_patch}](https://github.com/coder/coder/releases/tag/v${latest_patch})"
		else
			patch_link="N/A"
		fi

		# Format version name and patch link based on release status
		if [[ "$status" == "Not Released" ]]; then
			formatted_version_name="$version_name"
			patch_link="N/A"
			# Add row to table without a date for "Not Released"
			result+="| $formatted_version_name | | $status | $patch_link |\n"
		else
			formatted_version_name="[$version_name](https://coder.com/changelog/coder-$version_major-$rel_minor)"
			# Add row to table with date for released versions
			result+="| $formatted_version_name | $formatted_date | $status | $patch_link |\n"
		fi
	done

	echo -e "$result"
}

# Check if the markdown comments exist in the file
if ! grep -q "$CALENDAR_START_MARKER" "$DOCS_FILE" || ! grep -q "$CALENDAR_END_MARKER" "$DOCS_FILE"; then
	echo "Error: Markdown comment anchors not found in $DOCS_FILE"
	echo "Please add the following anchors around the release calendar table:"
	echo "  $CALENDAR_START_MARKER"
	echo "  $CALENDAR_END_MARKER"
	exit 1
fi

# Generate the new calendar table content
NEW_CALENDAR=$(generate_release_calendar)

# Update the file while preserving the rest of the content
awk -v start_marker="$CALENDAR_START_MARKER" \
	-v end_marker="$CALENDAR_END_MARKER" \
	-v new_calendar="$NEW_CALENDAR" \
	'
    BEGIN { found_start = 0; found_end = 0; print_line = 1; }
    $0 ~ start_marker {
        print;
        print new_calendar;
        found_start = 1;
        print_line = 0;
        next;
    }
    $0 ~ end_marker {
        found_end = 1;
        print_line = 1;
        print;
        next;
    }
    print_line || !found_start || found_end { print }
    ' "$DOCS_FILE" >"${DOCS_FILE}.new"

# Replace the original file with the updated version
mv "${DOCS_FILE}.new" "$DOCS_FILE"

# run make fmt/markdown
make fmt/markdown

echo "Successfully updated release calendar in $DOCS_FILE"