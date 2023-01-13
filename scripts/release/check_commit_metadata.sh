#!/usr/bin/env bash

# Usage: source ./check_commit_metadata.sh <from revision> <to revision>
# Usage: ./check_commit_metadata.sh <from revision> <to revision>
#
# Example: ./check_commit_metadata.sh v0.13.1 971e3678
#
# When sourced, this script will populate the COMMIT_METADATA_* variables
# with the commit metadata for each commit in the revision range.
#
# Because this script does some expensive lookups via the GitHub API, its
# results will be cached in the environment and restored if this script is
# sourced a second time with the same arguments.

set -euo pipefail
# shellcheck source=scripts/lib.sh
source "$(dirname "$(dirname "${BASH_SOURCE[0]}")")/lib.sh"

from_ref=${1:-}
to_ref=${2:-}

if [[ -z $from_ref ]]; then
	error "No from_ref specified"
fi
if [[ -z $to_ref ]]; then
	error "No to_ref specified"
fi

range="$from_ref..$to_ref"

# Check dependencies.
dependencies gh

COMMIT_METADATA_BREAKING=0
declare -A COMMIT_METADATA_TITLE COMMIT_METADATA_CATEGORY

# This environment variable can be set to 1 to ignore missing commit metadata,
# useful for dry-runs.
ignore_missing_metadata=${CODER_IGNORE_MISSING_COMMIT_METADATA:-0}

main() {
	# Match a commit prefix pattern, e.g. feat: or feat(site):.
	prefix_pattern="^([a-z]+)(\([a-z]*\))?:"

	# If a commit contains this title prefix or the source PR contains the
	# label, patch releases will not be allowed.
	# This regex matches both `feat!:` and `feat(site)!:`.
	breaking_title="^[a-z]+(\([a-z]*\))?!:"
	breaking_label=release/breaking
	breaking_category=breaking

	# Security related changes are labeled `security`.
	security_label=security

	# Get abbreviated and full commit hashes and titles for each commit.
	mapfile -t commits < <(git log --no-merges --pretty=format:"%h %H %s" "$range")

	# If this is a tag, use rev-list to find the commit it points to.
	from_commit=$(git rev-list -n 1 "$from_ref")
	# Get the committer date of the commit so that we can list PRs merged.
	from_commit_date=$(git show --no-patch --date=short --format=%cd "$from_commit")

	# Get the labels for all PRs merged since the last release, this is
	# inexact based on date, so a few PRs part of the previous release may
	# be included.
	#
	# Example output:
	#
	#   27386d49d08455b6f8fbf2c18f38244d03fda892 label:security
	#   d9f2aaf3b430d8b6f3d5f24032ed6357adaab1f1
	#   fd54512858c906e66f04b0744d8715c2e0de97e6 label:stale label:enhancement
	mapfile -t pr_labels_raw < <(
		gh pr list \
			--base main \
			--state merged \
			--limit 10000 \
			--search "merged:>=$from_commit_date" \
			--json mergeCommit,labels \
			--jq '.[] | .mergeCommit.oid + " " + (["label:" + .labels[].name] | join(" "))'
	)
	declare -A labels
	for entry in "${pr_labels_raw[@]}"; do
		commit_sha_long=${entry%% *}
		all_labels=${entry#* }
		labels[$commit_sha_long]=$all_labels
	done

	for commit in "${commits[@]}"; do
		mapfile -d ' ' -t parts <<<"$commit"
		commit_sha_short=${parts[0]}
		commit_sha_long=${parts[1]}
		commit_prefix=${parts[2]}

		if [[ $ignore_missing_metadata != 1 ]]; then
			# Safety-check, guarantee all commits had their metadata fetched.
			if [[ ! -v labels[$commit_sha_long] ]]; then
				error "Metadata missing for commit $commit_sha_short"
			fi
		fi

		# Store the commit title for later use.
		title=${parts[*]:2}
		title=${title%$'\n'}
		COMMIT_METADATA_TITLE[$commit_sha_short]=$title

		# First, check the title for breaking changes. This avoids doing a
		# GH API request if there's a match.
		if [[ $commit_prefix =~ $breaking_title ]] || [[ ${labels[$commit_sha_long]:-} = *"label:$breaking_label"* ]]; then
			COMMIT_METADATA_CATEGORY[$commit_sha_short]=$breaking_category
			COMMIT_METADATA_BREAKING=1
			continue
		elif [[ ${labels[$commit_sha_long]:-} = *"label:$security_label"* ]]; then
			COMMIT_METADATA_CATEGORY[$commit_sha_short]=$security_label
			continue
		fi

		if [[ $commit_prefix =~ $prefix_pattern ]]; then
			commit_prefix=${BASH_REMATCH[1]}
		fi
		case $commit_prefix in
		# From: https://github.com/commitizen/conventional-commit-types
		feat | fix | docs | style | refactor | perf | test | build | ci | chore | revert)
			COMMIT_METADATA_CATEGORY[$commit_sha_short]=$commit_prefix
			;;
		*)
			COMMIT_METADATA_CATEGORY[$commit_sha_short]=other
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
	if [[ $ignore_missing_metadata == 1 ]]; then
		log "WARNING: Ignoring missing commit metadata, breaking changes may be missed."
	fi
	main
fi

export_commit_metadata

# Make it easier to debug this script by printing the associative array
# when it's not sourced.
if ! issourced; then
	declare_print_commit_metadata
fi
