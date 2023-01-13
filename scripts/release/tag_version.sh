#!/usr/bin/env bash

set -euo pipefail
# shellcheck source=scripts/lib.sh
source "$(dirname "$(dirname "${BASH_SOURCE[0]}")")/lib.sh"
cdroot

usage() {
	cat <<EOH
Usage: ./version_tag.sh [--dry-run] [--ref <ref>] <--major | --minor | --patch>

This script should be called to tag a new release. It will take the suggested
increment (major, minor, patch) and optionally promote e.g. patch -> minor if
there are breaking changes between the previous version and the given --ref
(or HEAD).

This script will create a git tag, so it should only be run in CI (or via
--dry-run).
EOH
}

dry_run=0
ref=HEAD
increment=

args="$(getopt -o h -l dry-run,help,ref:,major,minor,patch -- "$@")"
eval set -- "$args"
while true; do
	case "$1" in
	--dry-run)
		dry_run=1
		shift
		;;
	--ref)
		ref="$2"
		shift 2
		;;
	-h | --help)
		usage
		exit 0
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
dependencies git

if [[ -z $increment ]]; then
	error "No version increment provided."
fi

if [[ $dry_run != 1 ]] && [[ ${CI:-} == "" ]]; then
	error "This script must be run in CI or with --dry-run."
fi

old_version="$(git describe --abbrev=0 "$ref^1")"
cur_tag="$(git describe --abbrev=0 "$ref")"
if [[ $old_version != "$cur_tag" ]]; then
	message="Ref \"$ref\" is already tagged with a release ($cur_tag)"
	if ! ((dry_run)); then
		error "$message."
	fi
	log "DRYRUN: $message, echoing current tag."
	echo "$cur_tag"
	exit 0
fi
ref=$(git rev-parse --short "$ref")

log "Checking commit metadata for changes since $old_version..."
# shellcheck source=scripts/release/check_commit_metadata.sh
source "$SCRIPT_DIR/release/check_commit_metadata.sh" "$old_version" "$ref"

if ((COMMIT_METADATA_BREAKING == 1)); then
	prev_increment=$increment
	if [[ $increment == patch ]]; then
		increment=minor
	fi
	if [[ $prev_increment != "$increment" ]]; then
		log "Breaking change detected, changing version increment from \"$prev_increment\" to \"$increment\"."
	else
		log "Breaking change detected, provided increment is sufficient, using \"$increment\" increment."
	fi
else
	log "No breaking changes detected, using \"$increment\" increment."
fi

mapfile -d . -t version_parts <<<"${old_version#v}"
case "$increment" in
patch)
	version_parts[2]=$((version_parts[2] + 1))
	;;
minor)
	version_parts[1]=$((version_parts[1] + 1))
	version_parts[2]=0
	;;
major)
	version_parts[0]=$((version_parts[0] + 1))
	version_parts[1]=0
	version_parts[2]=0
	;;
*)
	error "Unrecognized version increment."
	;;
esac

new_version="v${version_parts[0]}.${version_parts[1]}.${version_parts[2]}"

log "Old version: $old_version"
log "New version: $new_version"
maybedryrun "$dry_run" git tag -a "$new_version" -m "Release $new_version" "$ref"

echo "$new_version"
