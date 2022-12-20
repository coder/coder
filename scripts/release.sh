#!/usr/bin/env bash

# This script should be called to create a new release.
#
# When run, this script will display the new version number and optionally a
# preview of the release notes. The new version will be selected automatically
# based on if the release contains breaking changes or not. If the release
# contains breaking changes, a new minor version will be created. Otherwise, a
# new patch version will be created.
#
# Set --ref if you need to specify a specific commit that the new version will
# be tagged at, otherwise the latest commit will be used.
#
# Set --minor to force a minor version bump, even when there are no breaking
# changes.
#
# To mark a release as containing breaking changes, the commit title should
# either contain a known prefix with an exclamation mark ("feat!:",
# "feat(api)!:") or the PR that was merged can be tagged with the
# "release/breaking" label.
#
# Usage: ./release.sh [--ref <ref>] [--minor]

set -euo pipefail
# shellcheck source=scripts/lib.sh
source "$(dirname "${BASH_SOURCE[0]}")/lib.sh"
cdroot

ref=
minor=0

args="$(getopt -o n -l ref:,minor -- "$@")"
eval set -- "$args"
while true; do
	case "$1" in
	--ref)
		ref="$2"
		shift 2
		;;
	--minor)
		minor=1
		shift
		;;
	--)
		shift
		break
		;;
	*)
		error "Unrecognized option: $1"
		;;
	esac
done

# Check dependencies.
dependencies gh sort

# Make sure the repository is up-to-date before generating release notes.
log "Fetching main and tags from origin..."
git fetch --quiet --tags origin main

# Resolve to the latest ref on origin/main unless otherwise specified.
ref=$(git rev-parse --short "${ref:-origin/main}")

# Make sure that we're running the latest release script.
if [[ -n $(git diff --name-status origin/main -- ./scripts/release.sh) ]]; then
	error "Release script is out-of-date. Please check out the latest version and try again."
fi

# Check the current version tag from GitHub (by number) using the API to
# ensure no local tags are considered.
mapfile -t versions < <(gh api -H "Accept: application/vnd.github+json" /repos/coder/coder/git/refs/tags -q '.[].ref | split("/") | .[2]' | grep '^v' | sort -r -V)
old_version=${versions[0]}

log "Checking commit metadata for changes since $old_version..."
# shellcheck source=scripts/release/check_commit_metadata.sh
source "$SCRIPT_DIR/release/check_commit_metadata.sh" "$old_version" "$ref"

mapfile -d . -t version_parts <<<"$old_version"
if [[ $minor == 1 ]] || [[ $COMMIT_METADATA_BREAKING == 1 ]]; then
	if [[ $COMMIT_METADATA_BREAKING == 1 ]]; then
		log "Breaking change detected, incrementing minor version..."
	else
		log "Forcing minor version bump..."
	fi
	version_parts[1]=$((version_parts[1] + 1))
	version_parts[2]=0
else
	log "No breaking changes detected, incrementing patch version..."
	version_parts[2]=$((version_parts[2] + 1))
fi
new_version="${version_parts[0]}.${version_parts[1]}.${version_parts[2]}"

log "Old version: ${old_version}"
log "New version: ${new_version}"

release_notes="$(execrelative ./release/generate_release_notes.sh --old-version "$old_version" --new-version "$new_version" --ref "$ref")"

echo
read -p "Preview release notes? (y/n) " -n 1 -r show_reply
echo
if [[ $show_reply =~ ^[Yy]$ ]]; then
	echo -e "$release_notes\n"
fi

read -p "Create release? (y/n) " -n 1 -r create
echo
if [[ $create =~ ^[Yy]$ ]]; then
	log "Tagging commit $ref as $new_version..."
	git tag -a "$new_version" -m "$new_version" "$ref"
	log "Pushing tag to origin..."
	git push -u origin "$new_version"
fi
