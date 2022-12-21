#!/usr/bin/env bash

# Usage: ./generate_release_notes.sh --old-version <old version> --new-version <new version> --ref <ref>
#
# Example: ./generate_release_notes.sh --old-version v0.13.0 --new-version v0.13.1 --ref 1e6b244c
#
# This script generates release notes for the given version. It will generate
# release notes for all commits between the old version and the new version.
#
# Ref must be set to the commit that the new version will be tagget at. This
# is used to determine the commits that are included in the release. If the
# commit is already tagged, ref can be set to the tag name.

set -euo pipefail
# shellcheck source=scripts/lib.sh
source "$(dirname "$(dirname "${BASH_SOURCE[0]}")")/lib.sh"

old_version=
new_version=
ref=

args="$(getopt -o '' -l old-version:,new-version:,ref: -- "$@")"
eval set -- "$args"
while true; do
	case "$1" in
	--old-version)
		old_version="$2"
		shift 2
		;;
	--new-version)
		new_version="$2"
		shift 2
		;;
	--ref)
		ref="$2"
		shift 2
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

if [[ -z $old_version ]]; then
	error "No old version specified"
fi
if [[ -z $new_version ]]; then
	error "No new version specified"
fi
if [[ -z $ref ]]; then
	error "No ref specified"
fi

# shellcheck source=scripts/release/check_commit_metadata.sh
source "$SCRIPT_DIR/release/check_commit_metadata.sh" "${old_version}" "${ref}"

# Sort commits by title prefix, then by date, only return sha at the end.
mapfile -t commits < <(git log --no-merges --pretty=format:"%ct %h %s" "${old_version}..${ref}" | sort -k3,3 -k1,1n | cut -d' ' -f2)

breaking_changelog=
feat_changelog=
fix_changelog=
other_changelog=

for commit in "${commits[@]}"; do
	line="- $commit ${COMMIT_METADATA_TITLE[$commit]}\n"

	case "${COMMIT_METADATA_CATEGORY[$commit]}" in
	breaking)
		breaking_changelog+="$line"
		;;
	feat)
		feat_changelog+="$line"
		;;
	fix)
		fix_changelog+="$line"
		;;
	*)
		other_changelog+="$line"
		;;
	esac
done

changelog="$(
	if ((${#breaking_changelog} > 0)); then
		echo -e "### BREAKING CHANGES\n"
		echo -e "$breaking_changelog"
	fi
	if ((${#feat_changelog} > 0)); then
		echo -e "### Features\n"
		echo -e "$feat_changelog"
	fi
	if ((${#fix_changelog} > 0)); then
		echo -e "### Bug fixes\n"
		echo -e "$fix_changelog"
	fi
	if ((${#other_changelog} > 0)); then
		echo -e "### Other changes\n"
		echo -e "$other_changelog"
	fi
)"

image_tag="$(execrelative ./image_tag.sh --version "$new_version")"

echo -e "## Changelog

$changelog

Compare: [\`${old_version}...${new_version}\`](https://github.com/coder/coder/compare/${old_version}...${new_version})

## Container image

- \`docker pull $image_tag\`
"
