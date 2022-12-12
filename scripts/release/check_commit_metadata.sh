#!/usr/bin/env bash

# Usage: source ./check_commit_tags.sh <revision range>
# Usage: ./check_commit_tags.sh <revision range>
#
# Example: ./check_commit_tags.sh v0.13.1..971e3678
#
# When sourced, this script will populate the COMMIT_METADATA_* variables
# with the commit metadata for each commit in the revision range.
#
# Because this script does some expensive lookups via the GitHub API, its
# results will be cached in the environment and restored if this script is
# sourced a second time with the same arguments.

set -euo pipefail
# shellcheck source=scripts/lib.sh
source "$(dirname "${BASH_SOURCE[0]}")/../lib.sh"

range=${1:-}

if [[ -z $range ]]; then
	error "No revision range specified"
fi

# Check dependencies.
dependencies gh

COMMIT_METADATA_BREAKING=0
declare -A COMMIT_METADATA_TITLE COMMIT_METADATA_CATEGORY

main() {
	# Match a commit prefix pattern, e.g. feat: or feat(site):.
	prefix_pattern="^([a-z]+)(\([a-z]*\))?:"

	# If a commit contains this title prefix or the source PR contains the
	# label, patch releases will not be allowed.
	# This regex matches both `feat!:` and `feat(site)!:`.
	breaking_title="^[a-z]+(\([a-z]*\))?!:"
	breaking_label=release/breaking
	breaking_category=breaking

	mapfile -t commits < <(git log --no-merges --pretty=format:"%h %s" "$range")

	for commit in "${commits[@]}"; do
		mapfile -d ' ' -t parts <<<"$commit"
		commit_sha=${parts[0]}
		commit_prefix=${parts[1]}

		# Store the commit title for later use.
		title=${parts[*]:1}
		title=${title%$'\n'}
		COMMIT_METADATA_TITLE[$commit_sha]=$title

		# First, check the title for breaking changes. This avoids doing a
		# GH API request if there's a match.
		if [[ $commit_prefix =~ $breaking_title ]]; then
			COMMIT_METADATA_CATEGORY[$commit_sha]=$breaking_category
			COMMIT_METADATA_BREAKING=1
			continue
		fi

		# Get the labels for the PR associated with this commit.
		mapfile -t labels < <(gh api -H "Accept: application/vnd.github+json" "/repos/coder/coder/commits/${commit_sha}/pulls" -q '.[].labels[].name')

		if [[ " ${labels[*]} " = *" ${breaking_label} "* ]]; then
			COMMIT_METADATA_CATEGORY[$commit_sha]=$breaking_category
			COMMIT_METADATA_BREAKING=1
			continue
		fi

		if [[ $commit_prefix =~ $prefix_pattern ]]; then
			commit_prefix=${BASH_REMATCH[1]}
		fi
		case $commit_prefix in
		feat | fix)
			COMMIT_METADATA_CATEGORY[$commit_sha]=$commit_prefix
			;;
		*)
			COMMIT_METADATA_CATEGORY[$commit_sha]=other
			;;
		esac
	done
}

declare_print_commit_metadata() {
	declare -p COMMIT_METADATA_BREAKING COMMIT_METADATA_TITLE COMMIT_METADATA_CATEGORY
}

export_commit_metadata() {
	_COMMIT_METADATA_CACHE="${range}:$(declare_print_commit_metadata)"
	export _COMMIT_METADATA_CACHE COMMIT_METADATA_BREAKING COMMIT_METADATA_TITLE COMMIT_METADATA_CATEGORY
}

# _COMMIT_METADATA_CACHE is used to cache the results of this script in
# the environment because bash arrays are not passed on to subscripts.
if [[ ${_COMMIT_METADATA_CACHE:-} == "${range}:"* ]]; then
	eval "${_COMMIT_METADATA_CACHE#*:}"
else
	main
fi

export_commit_metadata

# Make it easier to debug this script by printing the associative array
# when it's not sourced.
if ! issourced; then
	declare_print_commit_metadata
fi
