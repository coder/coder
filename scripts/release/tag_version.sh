#!/usr/bin/env bash

set -euo pipefail
# shellcheck source=scripts/lib.sh
source "$(dirname "$(dirname "${BASH_SOURCE[0]}")")/lib.sh"
cdroot

usage() {
	cat <<EOH
Usage: ./version_tag.sh [--dry-run] [--old-version <version>] [--ref <ref>] <--major | --minor | --patch>

This script should be called to tag a new release. It will take the suggested
increment (major, minor, patch) and optionally promote e.g. patch -> minor if
there are breaking changes between the previous version and the given --ref
(or HEAD).

Pass --old-version optionally to ensure that the version is bumped from the
provided version instead of the latest tag (for use in release.sh).

This script will create a git tag, it should only be called by release.sh or in
CI.
EOH
}

dry_run=0
old_version=
ref=HEAD
increment=
force=0

args="$(getopt -o h -l dry-run,help,old-version:,ref:,major,minor,patch,force -- "$@")"
eval set -- "$args"
while true; do
	case "$1" in
	--dry-run)
		dry_run=1
		shift
		;;
	--old-version)
		old_version="$2"
		shift 2
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
	--force)
		force=1
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

ref_name=${ref:-HEAD}
ref=$(git rev-parse "${ref_name}")

if [[ -z $increment ]]; then
	error "No version increment provided."
fi

if [[ -z $old_version ]]; then
	old_version="$(git describe --abbrev=0 "$ref^1" --always)"
fi

# shellcheck source=scripts/release/check_commit_metadata.sh
source "$SCRIPT_DIR/check_commit_metadata.sh" "$old_version" "$ref"

if ((COMMIT_METADATA_BREAKING == 1)); then
	prev_increment=$increment
	if [[ $increment == patch ]]; then
		increment=minor
	fi
	if [[ $prev_increment != "$increment" ]]; then
		if ((force == 1)); then
			log "Breaking change detected but --force provided, would use \"$increment\" but keeping \"$prev_increment\"."
			increment=$prev_increment
		else
			log "Breaking change detected, changing version increment from \"$prev_increment\" to \"$increment\"."
		fi
	else
		log "Breaking change detected, provided increment is sufficient, using \"$increment\" increment."
	fi
else
	log "No breaking changes detected, using \"$increment\" increment."
fi

mapfile -d . -t version_parts <<<"${old_version#v}"
release_branch_prefix="release/"
release_ff=0
case "$increment" in
patch)
	release_branch="${release_branch_prefix}${version_parts[0]}.${version_parts[1]}"
	branch_contains_ref=$(git branch --contains "${ref}" --list "${release_branch}" --format='%(refname)')
	if [[ -z $branch_contains_ref ]]; then
		# Allow patch if we can fast-forward to ref, no need for dry-run here
		# since we're not checking out the branch and deleting it afterwards.
		git branch --no-track "${release_branch}-ff" "${release_branch}"
		# We're using git fetch here to perform a fast-forward on a
		# non-checked-out branch. The "." uses the local repo as remote (faster).
		if ! git fetch --quiet . "${ref}":"${release_branch}-ff"; then
			git branch --quiet --delete --force "${release_branch}-ff"
			error "Provided ref (${ref_name}) is not in the required release branch (${release_branch}) and cannot be fast-forwarded, unable to increment patch version. Please increment minor or major."
		fi
		git branch --quiet --delete --force "${release_branch}-ff"
		release_ff=1
	fi
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

release_branch="${release_branch_prefix}${version_parts[0]}.${version_parts[1]}"
new_version="v${version_parts[0]}.${version_parts[1]}.${version_parts[2]}"

log "Old version: $old_version"
log "New version: $new_version"
log "Release branch: $release_branch"

tag_exists=$(git tag --list "$new_version")
if [[ -n ${tag_exists} ]]; then
	error "Tag ${new_version} already exists."
fi

if [[ ${increment} = patch ]]; then
	if ((release_ff == 1)); then
		log "Fast-forwarding release branch"
		maybedryrun "$dry_run" git checkout "${release_branch}"
		maybedryrun "$dry_run" git merge --ff-only "${ref}"
	else
		log "Using existing release branch"
		maybedryrun "$dry_run" git checkout "${release_branch}"
	fi
else
	remote_branch_exists=$(git branch --remotes --list "*/${release_branch}" --format='%(refname)')
	local_branch_exists=$(git branch --list "${release_branch}" --format='%(refname)')
	if [[ -n ${remote_branch_exists} ]] || [[ -n ${local_branch_exists} ]]; then
		if [[ ${prev_increment} == patch ]]; then
			error "Release branch ${release_branch} already exists, impossible upgrade from \"${prev_increment}\" to \"${increment}\" detected. Please check your ref (${ref_name}) and that no incompatible commits were cherry-picked."
		fi
	fi

	if [[ -n ${remote_branch_exists} ]]; then
		error "Release branch ${release_branch} already exists on remote, please check your ref."
	fi

	if [[ -n ${local_branch_exists} ]]; then
		# If it exists, ensure that this release branch points to the provided ref.
		release_branch_ref=$(git rev-parse "${release_branch}")
		if [[ ${release_branch_ref} != "${ref}" ]]; then
			error "Local release branch ${release_branch} already exists, but does not point to the provided ref (${ref_name})."
		fi
		log "Using existing release branch"
		maybedryrun "$dry_run" git checkout "${release_branch}"
	else
		log "Creating new release branch"
		maybedryrun "$dry_run" git checkout -b "${release_branch}" "${ref}"
	fi
fi

# Ensure the ref is in the release branch.
branch_contains_ref=$(git branch --contains "${ref}" --list "${release_branch}" --format='%(refname)')
if [[ -z $branch_contains_ref ]]; then
	error "Provided ref (${ref_name}) is not in the required release branch (${release_branch})."
fi

maybedryrun "$dry_run" git tag -a "$new_version" -m "Release $new_version" "$ref"

echo "${release_branch} ${new_version}"
