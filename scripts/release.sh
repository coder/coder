#!/usr/bin/env bash

set -euo pipefail
# shellcheck source=scripts/lib.sh
source "$(dirname "${BASH_SOURCE[0]}")/lib.sh"
cdroot

usage() {
	cat <<EOH
Usage: ./release.sh [--branch <name>] [--draft] [--dry-run] [--ref <ref>] [--major | --minor | --patch]

This script should be called to create a new release.

When run, this script will display the new version number and optionally a
preview of the release notes. The new version will be selected automatically
based on if the release contains breaking changes or not. If the release
contains breaking changes, a new minor version will be created. Otherwise, a
new patch version will be created.

Set --ref if you need to specify a specific commit that the new version will
be tagged at, otherwise the latest commit will be used.

Set --minor to force a minor version bump, even when there are no breaking
changes. Likewise for --major. By default a patch version will be created.

Set --dry-run to run the release workflow in CI as a dry-run (no release will
be created).

To mark a release as containing breaking changes, the commit title should
either contain a known prefix with an exclamation mark ("feat!:",
"feat(api)!:") or the PR that was merged can be tagged with the
"release/breaking" label.

To test changes to this script, you can set --branch <my-branch>, which will
run the release workflow in CI as a dry-run and use the latest commit on the
specified branch as the release commit. This will also set --dry-run.
EOH
}

# Warn if CODER_IGNORE_MISSING_COMMIT_METADATA is set any other way than via
# --branch.
if [[ ${CODER_IGNORE_MISSING_COMMIT_METADATA:-0} != 0 ]]; then
	log "WARNING: CODER_IGNORE_MISSING_COMMIT_METADATA is enabled externally, we will ignore missing commit metadata."
fi

branch=main
draft=0
dry_run=0
ref=
increment=

args="$(getopt -o h -l branch:,draft,dry-run,help,ref:,major,minor,patch -- "$@")"
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
mapfile -t versions < <(gh api -H "Accept: application/vnd.github+json" /repos/coder/coder/git/refs/tags -q '.[].ref | split("/") | .[2]' | grep '^v' | sort -r -V)
old_version=${versions[0]}

# shellcheck source=scripts/release/check_commit_metadata.sh
trap 'log "Check commit metadata failed, you can try to set \"export CODER_IGNORE_MISSING_COMMIT_METADATA=1\" and try again, if you know what you are doing."' EXIT
source "$SCRIPT_DIR/release/check_commit_metadata.sh" "$old_version" "$ref"
trap - EXIT

new_version="$(execrelative ./release/tag_version.sh --dry-run --ref "$ref" --"$increment")"
release_notes="$(execrelative ./release/generate_release_notes.sh --old-version "$old_version" --new-version "$new_version" --ref "$ref")"

log
read -p "Preview release notes? (y/n) " -n 1 -r show_reply
log
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
log
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
if [[ ${CODER_IGNORE_MISSING_COMMIT_METADATA:-0} == 1 ]]; then
	args+=(-F ignore_missing_commit_metadata=true)
fi

log
gh workflow run release.yaml \
	--ref "$branch" \
	-F increment="$increment" \
	-F snapshot=false \
	"${args[@]}"
log

read -p "Watch release? (y/n) " -n 1 -r watch
log
if ! [[ $watch =~ ^[Yy]$ ]]; then
	exit 0
fi

log 'Waiting for job to become "in_progress"...'

# Wait at most 3 minutes (3*60)/3 = 60 for the job to start.
for _ in $(seq 1 60); do
	mapfile -t run < <(
		# Output:
		# 3886828508
		# in_progress
		gh run list -w release.yaml \
			--limit 1 \
			--json status,databaseId \
			--jq '.[] | (.databaseId | tostring), .status'
	)
	if [[ ${run[1]} != "in_progress" ]]; then
		sleep 3
		continue
	fi
	gh run watch --exit-status "${run[0]}"
	exit 0
done

error "Waiting for job to start timed out."
