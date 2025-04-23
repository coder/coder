#!/bin/bash

set -euo pipefail

# This script automatically updates the release calendar in docs/install/releases/index.md
# It calculates the releases based on the first Tuesday of each month rule
# and updates the status of each release (Not Supported, Security Support, Stable, Mainline, Not Released)

DOCS_FILE="docs/install/releases/index.md"

# Define unique markdown comments as anchors
CALENDAR_START_MARKER="<!-- RELEASE_CALENDAR_START -->"
CALENDAR_END_MARKER="<!-- RELEASE_CALENDAR_END -->"

# Get current date
current_date=$(date +"%Y-%m-%d")
current_month=$(date +"%m")
current_year=$(date +"%Y")

# Function to get the first Tuesday of a given month and year
get_first_tuesday() {
    local year=$1
    local month=$2
    
    # Find the first day of the month
    local first_day=$(date -d "$year-$month-01" +"%u")
    
    # Calculate days until first Tuesday (if day 1 is Tuesday, first_day=2)
    local days_until_tuesday=$((first_day == 2 ? 0 : (9 - first_day) % 7))
    
    # Get the date of the first Tuesday
    local first_tuesday=$(date -d "$year-$month-01 +$days_until_tuesday days" +"%Y-%m-%d")
    
    echo "$first_tuesday"
}

# Function to format date as "Month DD, YYYY"
format_date() {
    date -d "$1" +"%B %d, %Y"
}

# Function to get the latest patch version for a minor release
get_latest_patch() {
    local version_major=$1
    local version_minor=$2
    
    # Get all tags for this minor version
    local tags=$(cd "$(git rev-parse --show-toplevel)" && git tag | grep "^v$version_major\\.$version_minor\\." | sort -V)
    
    # Get the latest one
    local latest=$(echo "$tags" | tail -1)
    
    if [ -z "$latest" ]; then
        # If no tags found, return empty
        echo ""
    else
        # Return without the v prefix
        echo "${latest#v}"
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
    
    # Find the current minor version by looking at the last mainline release tag
    local latest_version=$(cd "$(git rev-parse --show-toplevel)" && git tag | grep '^v[0-9]*\.[0-9]*\.[0-9]*$' | sort -V | tail -1)
    local version_minor=$(echo "$latest_version" | cut -d. -f2)
    
    # Start with 3 unsupported releases back
    local start_minor=$((version_minor - 5))
    
    # Initialize the calendar table with an additional column for latest release
    result="| Release name | Release Date       | Status           | Latest Release |\n"
    result+="|-------------|-------------------|------------------|----------------|\n"
    
    # Generate rows for each release (7 total: 3 unsupported, 1 security, 1 stable, 1 mainline, 1 next)
    for i in {0..6}; do
        # Calculate release minor version
        local rel_minor=$((start_minor + i))
        # Format release name without the .x
        local version_name="$version_major.$rel_minor"
        
        # Calculate release month and year based on release pattern
        # This is a simplified calculation assuming monthly releases
        local rel_month=$(( (current_month - (5 - i) + 12) % 12 ))
        [[ $rel_month -eq 0 ]] && rel_month=12
        local rel_year=$current_year
        if [[ $rel_month -gt $current_month ]]; then
            rel_year=$((rel_year - 1))
        fi
        if [[ $rel_month -lt $current_month && $i -gt 5 ]]; then
            rel_year=$((rel_year + 1))
        fi
        
        # Skip January releases starting from 2025
        if [[ $rel_month -eq 1 && $rel_year -ge 2025 ]]; then
            rel_month=2
            rel_year=$rel_year
        fi
        
        # Get release date (first Tuesday of the month)
        local release_date=$(get_first_tuesday $rel_year $(printf "%02d" $rel_month))
        local formatted_date=$(format_date "$release_date")
        
        # Get latest patch version
        local latest_patch=$(get_latest_patch $version_major $rel_minor)
        local patch_link=""
        if [ -n "$latest_patch" ]; then
            patch_link="[v${latest_patch}](https://github.com/coder/coder/releases/tag/v${latest_patch})"
        else
            patch_link="N/A"
        fi
        
        # Determine status
        local status
        if [[ "$release_date" > "$current_date" ]]; then
            status="Not Released"
        elif [[ $i -eq 6 ]]; then
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
        
        # Format version name and patch link based on release status
        # No links for unreleased versions
        local formatted_version_name
        if [[ "$status" == "Not Released" ]]; then
            formatted_version_name="$version_name"
            patch_link="N/A"
        else
            formatted_version_name="[$version_name](https://coder.com/changelog/coder-$version_major-$rel_minor)"
        fi
        
        # Add row to table
        result+="| $formatted_version_name | $formatted_date | $status | $patch_link |\n"
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
    ' "$DOCS_FILE" > "${DOCS_FILE}.new"

# Replace the original file with the updated version
mv "${DOCS_FILE}.new" "$DOCS_FILE"

echo "Successfully updated release calendar in $DOCS_FILE"