#!/usr/bin/env bash

set -euo pipefail
# shellcheck source=scripts/lib.sh
source "$(dirname "${BASH_SOURCE[0]}")/lib.sh"
cdroot

usage() {
	cat <<EOH
Usage: ./release.sh [--dry-run] [-h | --help] [--ref <ref>] [--major | --minor | --patch]

This script should be called to create a new release.

When run, this script will display the new version number and optionally a
preview of the release notes. The new version will be selected automatically
based on if the release contains breaking changes or not. If the release
contains breaking changes, a new minor version will be created. Otherwise, a
new patch version will be created.

To mark a release as containing breaking changes, the commit title should
either contain a known prefix with an exclamation mark ("feat!:",
"feat(api)!:") or the PR that was merged can be tagged with the
"release/breaking" label.

GitHub labels that affect release notes:

- release/breaking: Shown under BREAKING CHANGES, prevents patch release.
- release/experimental: Shown at the bottom under Experimental.
- security: Shown under SECURITY.

Flags:

Set --major or --minor to force a larger version bump, even when there are no
breaking changes. By default a patch version will be created, --patch is no-op.

Set --ref if you need to specify a specific commit that the new version will
be tagged at, otherwise the latest commit will be used.

Set --dry-run to see what this script would do without making actual changes.
EOH
}

branch=main
dry_run=0
ref=
increment=

args="$(getopt -o h -l dry-run,help,ref:,major,minor,patch -- "$@")"
eval set -- "$args"
while true; do
	case "$1" in
	--dry-run)
		dry_run=1
		shift
		;;
	-h | --help)
		usage
		exit 0
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
log "Checking GitHub for latest release..."
versions_out="$(gh api -H "Accept: application/vnd.github+json" /repos/coder/coder/git/refs/tags -q '.[].ref | split("/") | .[2]' | grep '^v' | sort -r -V)"
mapfile -t versions <<<"$versions_out"
old_version=${versions[0]}
log "Latest release: $old_version"
log

trap 'log "Check commit metadata failed, you can try to set \"export CODER_IGNORE_MISSING_COMMIT_METADATA=1\" and try again, if you know what you are doing."' EXIT
# shellcheck source=scripts/release/check_commit_metadata.sh
source "$SCRIPT_DIR/release/check_commit_metadata.sh" "$old_version" "$ref"
trap - EXIT

log "Executing DRYRUN of release tagging..."
new_version="$(execrelative ./release/tag_version.sh --old-version "$old_version" --ref "$ref" --"$increment" --dry-run)"
log
read -p "Continue? (y/n) " -n 1 -r continue_release
log
if ! [[ $continue_release =~ ^[Yy]$ ]]; then
	exit 0
fi

release_notes="$(execrelative ./release/generate_release_notes.sh --old-version "$old_version" --new-version "$new_version" --ref "$ref")"

read -p "Preview release notes? (y/n) " -n 1 -r show_reply
log
if [[ $show_reply =~ ^[Yy]$ ]]; then
	log
	echo -e "$release_notes\n"
fi

read -p "Create release? (y/n) " -n 1 -r create
log
if ! [[ $create =~ ^[Yy]$ ]]; then
	exit 0
fi

log
# Run without dry-run to actually create the tag, note we don't update the
# new_version variable here to ensure we're pushing what we showed before.
maybedryrun "$dry_run" execrelative ./release/tag_version.sh --old-version "$old_version" --ref "$ref" --"$increment" >/dev/null
maybedryrun "$dry_run" git push --tags -u origin "$new_version"

if ((dry_run)); then
	# We can't watch the release.yaml workflow if we're in dry-run mode.
	exit 0
fi

log
read -p "Watch release? (y/n) " -n 1 -r watch
log
if ! [[ $watch =~ ^[Yy]$ ]]; then
	exit 0
fi

log 'Waiting for job to become "in_progress"...'

# Wait at most 3 minutes (3*60)/3 = 60 for the job to start.
for _ in $(seq 1 60); do
	output="$(
		# Output:
		# 3886828508
		# in_progress
		gh run list -w release.yaml \
			--limit 1 \
			--json status,databaseId \
			--jq '.[] | (.databaseId | tostring), .status'
	)"
	mapfile -t run <<<"$output"
	if [[ ${run[1]} != "in_progress" ]]; then
		sleep 3
		continue
	fi
	gh run watch --exit-status "${run[0]}"
	exit 0
done

error "Waiting for job to start timed out."
