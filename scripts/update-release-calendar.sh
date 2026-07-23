#!/bin/bash

set -euo pipefail

# This script updates the release documentation to reflect the current release
# schedule across all supported channels. It is intended to run in CI after a
# release so the resulting docs PR is authored by the CI bot (and can be
# approved by whoever ran the release).
#
# It updates:
#   - docs/install/releases/index.md: the release calendar table and the
#     "latest ESR version" prose.
#   - docs/install/kubernetes.md: the Helm `--version` pins for each channel.
#   - docs/install/rancher.md: the per-channel version pins.
#
# Channels covered: mainline (n), stable (n-1), ESR, and maintenance ESR
# (ESR-1). The active ESR set is maintained in
# scripts/release_channels/esr_versions.txt (also consumed by
# .github/workflows/backport.yaml). Update that file when new ESR versions are
# designated or old ones reach end of life.

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

DOCS_FILE="docs/install/releases/index.md"
KUBERNETES_FILE="docs/install/kubernetes.md"
RANCHER_FILE="docs/install/rancher.md"
ESR_VERSIONS_FILE="${SCRIPT_DIR}/release_channels/esr_versions.txt"

CALENDAR_START_MARKER="<!-- RELEASE_CALENDAR_START -->"
CALENDAR_END_MARKER="<!-- RELEASE_CALENDAR_END -->"

VERSION_MAJOR=2

# Known active ESR (Extended Support Release) minor versions. The shared source
# of truth stores full major.minor versions; extract the minor component, since
# the release logic below is scoped to the 2.x line. Blank lines and '#'
# comments are ignored.
mapfile -t ESR_VERSIONS < <(grep -vE '^\s*(#|$)' "$ESR_VERSIONS_FILE" | cut -d. -f2)

# Check if a minor version is a known active ESR version.
is_esr_version() {
	local minor=$1
	for esr in "${ESR_VERSIONS[@]}"; do
		if [[ "$minor" -eq "$esr" ]]; then
			return 0
		fi
	done
	return 1
}

# Format date as "Month DD, YYYY"
format_date() {
	TZ=UTC date -d "$1" +"%B %d, %Y"
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
		TZ=UTC date -d "$tag_date" +"%Y-%m-%d"
	fi
}

# Generate a single release row for the calendar table.
# Arguments: version_major, rel_minor, status
generate_release_row() {
	local version_major=$1
	local rel_minor=$2
	local status=$3
	local version_name="$version_major.$rel_minor"
	local actual_release_date
	local formatted_date
	local latest_patch
	local patch_link
	local formatted_version_name

	# Get the actual release date from the first published tag
	if [[ "$status" != "Not Released" ]]; then
		actual_release_date=$(get_release_date "$version_major" "$rel_minor")

		if [ -n "$actual_release_date" ]; then
			formatted_date=$(format_date "$actual_release_date")
		else
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
		echo "| $formatted_version_name | | $status | $patch_link |"
	else
		formatted_version_name="[$version_name](https://coder.com/changelog/coder-$version_major-$rel_minor)"
		echo "| $formatted_version_name | $formatted_date | $status | $patch_link |"
	fi
}

# Generate releases table showing:
# - Active ESR releases (older than the standard window)
# - 3 previous unsupported releases
# - 1 security support release (n-2)
# - 1 stable release (n-1)
# - 1 mainline release (n)
# - 1 next release (n+1)
#
# ESR versions within the standard window that would otherwise show as
# "Not Supported" are marked as "Extended Support Release" instead.
generate_release_calendar() {
	local result=""
	local version_major=$VERSION_MAJOR
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

	# Add active ESR versions that fall before the standard window
	for esr_minor in "${ESR_VERSIONS[@]}"; do
		if [[ "$esr_minor" -lt "$start_minor" ]]; then
			result+="$(generate_release_row "$version_major" "$esr_minor" "Extended Support Release")\n"
		fi
	done

	# Generate rows for each release (7 total: 3 unsupported, 1 security, 1 stable, 1 mainline, 1 next)
	for i in {0..6}; do
		# Calculate release minor version
		local rel_minor=$((start_minor + i))
		local status

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

		# Mark ESR versions. An ESR that has aged out of support shows as a
		# full "Extended Support Release"; while it is still in an active
		# channel we append "(ESR)" to that channel, e.g. "Mainline (ESR)".
		if is_esr_version "$rel_minor"; then
			if [[ "$status" == "Not Supported" ]]; then
				status="Extended Support Release"
			elif [[ "$status" != "Not Released" ]]; then
				status="$status (ESR)"
			fi
		fi

		result+="$(generate_release_row "$version_major" "$rel_minor" "$status")\n"
	done

	echo -e "$result"
}

# regex_escape prints its argument with ERE metacharacters backslash-escaped so
# it can be embedded safely inside a bash regular expression.
regex_escape() {
	printf '%s' "$1" | sed 's/[][(){}.^$*+?|\\]/\\&/g'
}

# update_autoversion updates the semantic version that follows each
#   <!-- autoversion(<channel>): "<pattern>" -->
# pragma for the given channel. The pattern contains a literal [version]
# placeholder; the version on one of the next few lines is replaced. This
# mirrors the autoversion handling previously done by the release TUI.
# Arguments: file, channel, new_version
update_autoversion() {
	local file=$1 channel=$2 newver=$3
	local -a lines
	mapfile -t lines <"$file"
	local n=${#lines[@]}
	local i j changed=0
	local pragma_re='autoversion\(([^)]+)\): "([^"]*)"'

	for ((i = 0; i < n; i++)); do
		if [[ ${lines[i]} =~ $pragma_re ]]; then
			local pragma_channel=${BASH_REMATCH[1]}
			local pattern=${BASH_REMATCH[2]}
			[[ $pragma_channel == "$channel" ]] || continue

			local pre=${pattern%%\[version\]*}
			local post=${pattern##*\[version\]}
			local pre_re post_re line_re
			pre_re=$(regex_escape "$pre")
			post_re=$(regex_escape "$post")
			line_re="^(.*${pre_re})[0-9]+\.[0-9]+\.[0-9]+(${post_re}.*)\$"

			# Replace the version on one of the next few lines. The version can
			# sit several lines below the pragma (blank line, code fence, and a
			# few command lines), so search a generous window and stop at the
			# first match.
			for ((j = i + 1; j < n && j <= i + 10; j++)); do
				if [[ ${lines[j]} =~ $line_re ]]; then
					lines[j]="${BASH_REMATCH[1]}${newver}${BASH_REMATCH[2]}"
					changed=1
					break
				fi
			done
		fi
	done

	if [[ $changed -eq 1 ]]; then
		printf '%s\n' "${lines[@]}" >"$file"
	fi
}

# update_rancher_pin updates a `- **<label>**: `X.Y.Z`` pin in rancher.md.
# Arguments: file, label, new_version
update_rancher_pin() {
	local file=$1 label=$2 newver=$3
	local label_re
	label_re=$(regex_escape "$label")
	sed -i -E "s#(\\*\\*${label_re}\\*\\*: \`)[0-9]+\.[0-9]+\.[0-9]+(\`)#\\1${newver}\\2#" "$file"
}

# update_esr_prose updates the "latest ESR version" sentence in index.md.
# Arguments: file, display_version (e.g. 2.34), tag_version (e.g. 2.34.6)
update_esr_prose() {
	local file=$1 display=$2 tag=$3
	sed -i -E \
		"s#(The latest ESR version is \[Coder )[0-9]+\.[0-9]+(\]\(https://github.com/coder/coder/releases/tag/v)[0-9]+\.[0-9]+\.[0-9]+(\))#\\1${display}\\2${tag}\\3#" \
		"$file"
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

# --- Update per-channel version pins across the release docs ---

# Mainline is the highest released minor; stable is the one before it.
LATEST_TAG=$(cd "$(git rev-parse --show-toplevel)" && git tag | grep '^v[0-9]*\.[0-9]*\.[0-9]*$' | sort -V | tail -1)
MAINLINE_MINOR=$(echo "$LATEST_TAG" | cut -d. -f2)
STABLE_MINOR=$((MAINLINE_MINOR - 1))

# Of the active ESR minors, the highest is the current ESR and the next highest
# is the maintenance ESR (ESR-1).
mapfile -t ESR_SORTED < <(printf '%s\n' "${ESR_VERSIONS[@]}" | sort -rn)
ESR_MINOR=${ESR_SORTED[0]:-}
ESR_MAINT_MINOR=${ESR_SORTED[1]:-}

MAINLINE_VER=$(get_latest_patch "$VERSION_MAJOR" "$MAINLINE_MINOR")
STABLE_VER=$(get_latest_patch "$VERSION_MAJOR" "$STABLE_MINOR")
ESR_VER=""
[ -n "$ESR_MINOR" ] && ESR_VER=$(get_latest_patch "$VERSION_MAJOR" "$ESR_MINOR")
ESR_MAINT_VER=""
[ -n "$ESR_MAINT_MINOR" ] && ESR_MAINT_VER=$(get_latest_patch "$VERSION_MAJOR" "$ESR_MAINT_MINOR")

# index.md: latest ESR prose.
if [ -n "$ESR_VER" ]; then
	update_esr_prose "$DOCS_FILE" "${VERSION_MAJOR}.${ESR_MINOR}" "$ESR_VER"
fi

# kubernetes.md: Helm --version pins per channel.
[ -n "$MAINLINE_VER" ] && update_autoversion "$KUBERNETES_FILE" "mainline" "$MAINLINE_VER"
[ -n "$STABLE_VER" ] && update_autoversion "$KUBERNETES_FILE" "stable" "$STABLE_VER"
[ -n "$ESR_VER" ] && update_autoversion "$KUBERNETES_FILE" "esr" "$ESR_VER"
[ -n "$ESR_MAINT_VER" ] && update_autoversion "$KUBERNETES_FILE" "maintenance-esr" "$ESR_MAINT_VER"

# rancher.md: per-channel version pins.
[ -n "$MAINLINE_VER" ] && update_rancher_pin "$RANCHER_FILE" "Mainline" "$MAINLINE_VER"
[ -n "$STABLE_VER" ] && update_rancher_pin "$RANCHER_FILE" "Stable" "$STABLE_VER"
[ -n "$ESR_VER" ] && update_rancher_pin "$RANCHER_FILE" "ESR" "$ESR_VER"
[ -n "$ESR_MAINT_VER" ] && update_rancher_pin "$RANCHER_FILE" "Maintenance ESR" "$ESR_MAINT_VER"

# run make fmt/markdown
make fmt/markdown

echo "Successfully updated release docs (calendar, ESR prose, Kubernetes and Rancher pins)."
