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
# changes. Likewise for --major. By default a patch version will be created.
#
# Set --dry-run to run the release workflow in CI as a dry-run (no release will
# be created).
#
# To mark a release as containing breaking changes, the commit title should
# either contain a known prefix with an exclamation mark ("feat!:",
# "feat(api)!:") or the PR that was merged can be tagged with the
# "release/breaking" label.
#
# To test changes to this script, you can set `--branch <my-branch>`, which will
# run the release workflow in CI as a dry-run and use the latest commit on the
# specified branch as the release commit. This will also set --dry-run.
#
# Usage: ./release.sh [--branch <name>] [--draft] [--dry-run] [--ref <ref>] [--major | --minor | --patch]

set -euo pipefail
# shellcheck source=scripts/lib.sh
source "$(dirname "${BASH_SOURCE[0]}")/lib.sh"
cdroot

branch=main
draft=0
dry_run=0
ref=
increment=

args="$(getopt -o n -l branch:,draft,dry-run,ref:,major,minor,patch -- "$@")"
eval set -- "$args"
while true; do
	case "$1" in
	--branch)
		branch="$2"
		log "Using branch $branch, implies DRYRUN and CODER_IGNORE_MISSING_COMMIT_METADATA."
		dry_run=1
		export CODER_IGNORE_MISSING_COMMIT_METADATA=1
		shift 2
		;;
	--draft)
		draft=1
		shift
		;;
	--dry-run)
		dry_run=1
		shift
		;;
	--ref)
		ref="$2"
		shift 2
		;;
	--major | --minor | --patch)
		if [[ -n $increment ]]; then
			error "Cannot specify multiple version increments."
		fi
		increment=${1#--}
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

if [[ -z $increment ]]; then
	# Default to patch versions.
	increment="patch"
fi

# Make sure the repository is up-to-date before generating release notes.
log "Fetching $branch and tags from origin..."
git fetch --quiet --tags origin "$branch"

# Resolve to the latest ref on origin/main unless otherwise specified.
ref=$(git rev-parse --short "${ref:-origin/$branch}")

# Make sure that we're running the latest release script.
if [[ -n $(git diff --name-status origin/"$branch" -- ./scripts/release.sh) ]]; then
	error "Release script is out-of-date. Please check out the latest version and try again."
fi

# Check the current version tag from GitHub (by number) using the API to
# ensure no local tags are considered.
mapfile -t versions < <(gh api -H "Accept: application/vnd.github+json" /repos/coder/coder/git/refs/tags -q '.[].ref | split("/") | .[2]' | grep '^v' | sort -r -V)
old_version=${versions[0]}

# shellcheck source=scripts/release/check_commit_metadata.sh
source "$SCRIPT_DIR/release/check_commit_metadata.sh" "$old_version" "$ref"

new_version="$(execrelative ./release/increment_version_tag.sh --dry-run --ref "$ref" --"$increment")"
release_notes="$(execrelative ./release/generate_release_notes.sh --old-version "$old_version" --new-version "$new_version" --ref "$ref")"

echo
read -p "Preview release notes? (y/n) " -n 1 -r show_reply
echo
if [[ $show_reply =~ ^[Yy]$ ]]; then
	echo -e "$release_notes\n"
fi

create_message="Create release"
if ((draft)); then
	create_message="Create draft release"
fi
if ((dry_run)); then
	create_message+=" (DRYRUN)"
fi
read -p "$create_message? (y/n) " -n 1 -r create
echo
if ! [[ $create =~ ^[Yy]$ ]]; then
	exit 0
fi

args=()
if ((draft)); then
	args+=(-F draft=true)
fi
if ((dry_run)); then
	args+=(-F dry_run=true)
fi

gh workflow run release.yaml \
	--ref "$branch" \
	-F increment="$increment" \
	-F snapshot=false \
	"${args[@]}"

log "Release process started, you can watch the release via: gh run watch --exit-status <run-id>"
