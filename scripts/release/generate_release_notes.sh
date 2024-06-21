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
mainline=1

args="$(getopt -o '' -l old-version:,new-version:,ref:,mainline,stable -- "$@")"
eval set -- "${args}"
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
	--mainline)
		mainline=1
		shift
		;;
	--stable)
		mainline=0
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

if [[ -z ${old_version} ]]; then
	error "No old version specified"
fi
if [[ -z ${new_version} ]]; then
	error "No new version specified"
fi
if [[ ${new_version} != v* ]]; then
	error "New version must start with a v"
fi
if [[ -z ${ref} ]]; then
	error "No ref specified"
fi

# shellcheck source=scripts/release/check_commit_metadata.sh
source "${SCRIPT_DIR}/check_commit_metadata.sh" "${old_version}" "${ref}"

# Sort commits by title prefix, then by date, only return sha at the end.
git_show_out="$(
	{
		echo "${COMMIT_METADATA_COMMITS[@]}" |
			tr ' ' '\n' |
			xargs git show --no-patch --pretty=format:"%ct %h %s"
	} | sort -k3,3 -k1,1n | cut -d' ' -f2
)"
mapfile -t commits <<<"${git_show_out}"

# From: https://github.com/commitizen/conventional-commit-types
# NOTE(mafredri): These need to be supported in check_commit_metadata.sh as well.
declare -a section_order=(
	breaking
	security
	feat
	fix
	docs
	refactor
	perf
	test
	build
	ci
	chore
	revert
	other
	experimental
)

declare -A section_titles=(
	[breaking]='BREAKING CHANGES'
	[security]='SECURITY'
	[feat]='Features'
	[fix]='Bug fixes'
	[docs]='Documentation'
	[refactor]='Code refactoring'
	[perf]='Performance improvements'
	[test]='Tests'
	[build]='Builds'
	[ci]='Continuous integration'
	[chore]='Chores'
	[revert]='Reverts'
	[other]='Other changes'
	[experimental]='Experimental changes'
)

# Verify that all items in section_order exist as keys in section_titles and
# vice-versa.
for cat in "${section_order[@]}"; do
	if [[ " ${!section_titles[*]} " != *" ${cat} "* ]]; then
		error "BUG: category ${cat} does not exist in section_titles"
	fi
done
for cat in "${!section_titles[@]}"; do
	if [[ " ${section_order[*]} " != *" ${cat} "* ]]; then
		error "BUG: Category ${cat} does not exist in section_order"
	fi
done

for commit in "${commits[@]}"; do
	title=${COMMIT_METADATA_TITLE[${commit}]}
	if [[ -v COMMIT_METADATA_HUMAN_TITLE[${commit}] ]]; then
		title=${COMMIT_METADATA_HUMAN_TITLE[${commit}]}
	fi

	if [[ ${title} =~ \(#[0-9]*\)$ ]]; then
		title="${title%)}, ${commit})"
	else
		title="${title} (${commit})"
	fi
	line="- ${title}"
	if [[ -v COMMIT_METADATA_AUTHORS[${commit}] ]]; then
		line+=" (${COMMIT_METADATA_AUTHORS[${commit}]})"
	fi

	# Default to "other" category.
	cat=other
	for c in "${!section_titles[@]}"; do
		if [[ ${c} == "${COMMIT_METADATA_CATEGORY[${commit}]}" ]]; then
			cat=${c}
			break
		fi
	done
	declare "${cat}"_changelog+="${line}"$'\n'
done

changelog="$(
	for cat in "${section_order[@]}"; do
		changes="$(eval "echo -e \"\${${cat}_changelog:-}\"")"
		if ((${#changes} > 0)); then
			echo -e "\n### ${section_titles["${cat}"]}\n"
			if [[ ${cat} == experimental ]]; then
				echo -e "These changes are feature-flagged and can be enabled with the \`--experiments\` server flag. They may change or be removed in future releases.\n"
			fi
			echo -e "${changes}"
		fi
	done
)"

image_tag="$(execrelative ../image_tag.sh --version "${new_version}")"

blurb=
stable_since=
if ((mainline)); then
	blurb="
> [!NOTE]
> This is a mainline Coder release. We advise enterprise customers without a staging environment to install our [latest stable release](https://github.com/coder/coder/releases/latest) while we refine this version. Learn more about our [Release Schedule](https://coder.com/docs/v2/latest/install/releases).
"
else
	# Date format: April 23, 2024
	d=$(date +'%B %d, %Y')
	stable_since="> ## Stable (since ${d})"$'\n\n'
fi

echo -e "${stable_since}## Changelog
${blurb}${changelog}

Compare: [\`${old_version}...${new_version}\`](https://github.com/coder/coder/compare/${old_version}...${new_version})

## Container image

- \`docker pull ${image_tag}\`

## Install/upgrade

Refer to our docs to [install](https://coder.com/docs/v2/latest/install) or [upgrade](https://coder.com/docs/v2/latest/admin/upgrade) Coder, or use a release asset below.
"
