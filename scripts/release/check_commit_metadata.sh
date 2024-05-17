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

if [[ -z ${from_ref} ]]; then
	error "No from_ref specified"
fi
if [[ -z ${to_ref} ]]; then
	error "No to_ref specified"
fi

range="${from_ref}..${to_ref}"

# Check dependencies.
dependencies gh

# Authenticate gh CLI
gh_auth

COMMIT_METADATA_BREAKING=0
declare -a COMMIT_METADATA_COMMITS
declare -A COMMIT_METADATA_TITLE COMMIT_METADATA_HUMAN_TITLE COMMIT_METADATA_CATEGORY COMMIT_METADATA_AUTHORS

# This environment variable can be set to 1 to ignore missing commit metadata,
# useful for dry-runs.
ignore_missing_metadata=${CODER_IGNORE_MISSING_COMMIT_METADATA:-0}

main() {
	log "Checking commit metadata for changes between ${from_ref} and ${to_ref}..."

	# Match a commit prefix pattern, e.g. feat: or feat(site):.
	prefix_pattern="^([a-z]+)(\([^)]+\))?:"

	# If a commit contains this title prefix or the source PR contains the
	# label, patch releases will not be allowed.
	# This regex matches both `feat!:` and `feat(site)!:`.
	breaking_title="^[a-z]+(\([^)]+\))?!:"
	breaking_label=release/breaking
	breaking_category=breaking
	experimental_label=release/experimental
	experimental_category=experimental

	# Security related changes are labeled `security`.
	security_label=security
	security_category=security

	# Order is important here, first partial match wins.
	declare -A humanized_areas=(
		["agent/agentssh"]="Agent SSH"
		["coderd/database"]="Database"
		["enterprise/audit"]="Auditing"
		["enterprise/cli"]="CLI"
		["enterprise/coderd"]="Server"
		["enterprise/dbcrypt"]="Database"
		["enterprise/derpmesh"]="Networking"
		["enterprise/provisionerd"]="Provisioner"
		["enterprise/tailnet"]="Networking"
		["enterprise/wsproxy"]="Workspace Proxy"
		[agent]="Agent"
		[cli]="CLI"
		[coderd]="Server"
		[codersdk]="SDK"
		[docs]="Documentation"
		[enterprise]="Enterprise"
		[examples]="Examples"
		[helm]="Helm"
		[install.sh]="Installer"
		[provisionersdk]="SDK"
		[provisionerd]="Provisioner"
		[provisioner]="Provisioner"
		[pty]="CLI"
		[scaletest]="Scale Testing"
		[site]="Dashboard"
		[support]="Support"
		[tailnet]="Networking"
	)

	# Get hashes for all cherry-picked commits between the selected ref
	# and main. These are sorted by commit title so that we can group
	# two cherry-picks together.
	declare -A cherry_pick_commits
	git_cherry_out=$(
		{
			git log --no-merges --cherry-mark --pretty=format:"%m %H %s" "${to_ref}...origin/main"
			echo
			git log --no-merges --cherry-mark --pretty=format:"%m %H %s" "${from_ref}...origin/main"
			echo
		} | { grep '^=' || true; } | sort -u | sort -k3
	)
	if [[ -n ${git_cherry_out} ]]; then
		mapfile -t cherry_picks <<<"${git_cherry_out}"
		# Iterate over the array in groups of two
		for ((i = 0; i < ${#cherry_picks[@]}; i += 2)); do
			mapfile -d ' ' -t parts1 <<<"${cherry_picks[i]}"
			mapfile -d ' ' -t parts2 <<<"${cherry_picks[i + 1]}"
			commit1=${parts1[1]}
			title1=${parts1[*]:2}
			commit2=${parts2[1]}
			title2=${parts2[*]:2}

			if [[ ${title1} != "${title2}" ]]; then
				error "Invariant failed, cherry-picked commits have different titles: ${title1} != ${title2}"
			fi

			cherry_pick_commits[${commit1}]=${commit2}
			cherry_pick_commits[${commit2}]=${commit1}
		done
	fi

	# Get abbreviated and full commit hashes and titles for each commit.
	git_log_out="$(git log --no-merges --left-right --pretty=format:"%m %h %H %s" "${range}")"
	if [[ -z ${git_log_out} ]]; then
		error "No commits found in range ${range}"
	fi
	mapfile -t commits <<<"${git_log_out}"

	# Get the lowest committer date of the commits so that we can fetch
	# the PRs that were merged.
	lookback_date=$(
		{
			# Check all included commits.
			for commit in "${commits[@]}"; do
				mapfile -d ' ' -t parts <<<"${commit}"
				sha_long=${parts[2]}
				git show --no-patch --date=short --format='%cd' "${sha_long}"
			done
			# Include cherry-picks and their original commits (the
			# original commit may be older than the cherry pick).
			for cherry_pick in "${cherry_picks[@]}"; do
				mapfile -d ' ' -t parts <<<"${cherry_pick}"
				sha_long=${parts[1]}
				git show --no-patch --date=short --format='%cd' "${sha_long}"
			done
		} | sort -t- -n | head -n 1
	)
	# Get the labels for all PRs merged since the last release, this is
	# inexact based on date, so a few PRs part of the previous release may
	# be included.
	#
	# Example output:
	#
	#   27386d49d08455b6f8fbf2c18f38244d03fda892 author:hello labels:label:security
	#   d9f2aaf3b430d8b6f3d5f24032ed6357adaab1f1 author:world
	#   fd54512858c906e66f04b0744d8715c2e0de97e6 author:bye labels:label:stale label:enhancement
	pr_list_out="$(
		gh pr list \
			--base main \
			--state merged \
			--limit 10000 \
			--search "merged:>=${lookback_date}" \
			--json mergeCommit,labels,author \
			--jq '.[] | "\( .mergeCommit.oid ) author:\( .author.login ) labels:\(["label:\( .labels[].name )"] | join(" "))"'
	)"

	declare -A authors labels
	if [[ -n ${pr_list_out} ]]; then
		mapfile -t pr_metadata_raw <<<"${pr_list_out}"

		for entry in "${pr_metadata_raw[@]}"; do
			commit_sha_long=${entry%% *}
			commit_author=${entry#* author:}
			commit_author=${commit_author%% *}
			authors[${commit_sha_long}]=${commit_author}
			all_labels=${entry#* labels:}
			labels[${commit_sha_long}]=${all_labels}
		done
	fi

	for commit in "${commits[@]}"; do
		mapfile -d ' ' -t parts <<<"${commit}"
		left_right=${parts[0]} # From `git log --left-right`, see `man git-log` for details.
		commit_sha_short=${parts[1]}
		commit_sha_long=${parts[2]}
		commit_prefix=${parts[3]}
		title=${parts[*]:3}
		title=${title%$'\n'}
		title_no_prefix=${parts[*]:4}
		title_no_prefix=${title_no_prefix%$'\n'}

		# For COMMIT_METADATA_COMMITS in case of cherry-pick override.
		commit_sha_long_orig=${commit_sha_long}

		# Check if this is a potential cherry-pick.
		if [[ -v cherry_pick_commits[${commit_sha_long}] ]]; then
			# Is this the cherry-picked or the original commit?
			if [[ ! -v authors[${commit_sha_long}] ]] || [[ ! -v labels[${commit_sha_long}] ]]; then
				log "Cherry-picked commit ${commit_sha_long}, checking original commit ${cherry_pick_commits[${commit_sha_long}]}"
				# Use the original commit's metadata from GitHub.
				commit_sha_long=${cherry_pick_commits[${commit_sha_long}]}
			else
				# Skip the cherry-picked commit, we only need the original.
				log "Skipping commit ${commit_sha_long} cherry-picked into ${from_ref} as ${cherry_pick_commits[${commit_sha_long}]} (${title})"
				continue
			fi
		fi

		author=
		if [[ -v authors[${commit_sha_long}] ]]; then
			author=${authors[${commit_sha_long}]}
			if [[ ${author} == "app/dependabot" ]]; then
				log "Skipping commit by app/dependabot ${commit_sha_short} (${commit_sha_long})"
				continue
			fi
		fi

		if [[ ${left_right} == "<" ]]; then
			# Skip commits that are already in main.
			log "Skipping commit ${commit_sha_short} from other branch (${commit_sha_long} ${title})"
			continue
		fi

		COMMIT_METADATA_COMMITS+=("${commit_sha_long_orig}")

		# Safety-check, guarantee all commits had their metadata fetched.
		if [[ -z ${author} ]] || [[ ! -v labels[${commit_sha_long}] ]]; then
			if [[ ${ignore_missing_metadata} != 1 ]]; then
				error "Metadata missing for commit ${commit_sha_short} (${commit_sha_long})"
			else
				log "WARNING: Metadata missing for commit ${commit_sha_short} (${commit_sha_long})"
			fi
		fi

		# Store the commit title for later use.
		COMMIT_METADATA_TITLE[${commit_sha_short}]=${title}
		if [[ -n ${author} ]]; then
			COMMIT_METADATA_AUTHORS[${commit_sha_short}]="@${author}"
		fi

		# Create humanized titles where possible, examples:
		#
		# 	"feat: add foo" -> "Add foo".
		# 	"feat(site): add bar" -> "Dashboard: Add bar".
		COMMIT_METADATA_HUMAN_TITLE[${commit_sha_short}]=${title}
		if [[ ${commit_prefix} =~ ${prefix_pattern} ]]; then
			sub=${BASH_REMATCH[2]}
			if [[ -z ${sub} ]]; then
				# No parenthesis found, simply drop the prefix.
				COMMIT_METADATA_HUMAN_TITLE[${commit_sha_short}]="${title_no_prefix^}"
			else
				# Drop the prefix and replace it with a humanized area,
				# leave as-is for unknown areas.
				sub=${sub#(}
				for area in "${!humanized_areas[@]}"; do
					if [[ ${sub} = "${area}"* ]]; then
						COMMIT_METADATA_HUMAN_TITLE[${commit_sha_short}]="${humanized_areas[${area}]}: ${title_no_prefix^}"
						break
					fi
				done
			fi
		fi

		# First, check the title for breaking changes. This avoids doing a
		# GH API request if there's a match.
		if [[ ${commit_prefix} =~ ${breaking_title} ]] || [[ ${labels[${commit_sha_long}]:-} = *"label:${breaking_label}"* ]]; then
			COMMIT_METADATA_CATEGORY[${commit_sha_short}]=${breaking_category}
			COMMIT_METADATA_BREAKING=1
			continue
		elif [[ ${labels[${commit_sha_long}]:-} = *"label:${security_label}"* ]]; then
			COMMIT_METADATA_CATEGORY[${commit_sha_short}]=${security_category}
			continue
		elif [[ ${labels[${commit_sha_long}]:-} = *"label:${experimental_label}"* ]]; then
			COMMIT_METADATA_CATEGORY[${commit_sha_short}]=${experimental_category}
			continue
		fi

		if [[ ${commit_prefix} =~ ${prefix_pattern} ]]; then
			commit_prefix=${BASH_REMATCH[1]}
		fi
		case ${commit_prefix} in
		# From: https://github.com/commitizen/conventional-commit-types
		feat | fix | docs | style | refactor | perf | test | build | ci | chore | revert)
			COMMIT_METADATA_CATEGORY[${commit_sha_short}]=${commit_prefix}
			;;
		*)
			COMMIT_METADATA_CATEGORY[${commit_sha_short}]=other
			;;
		esac
	done
}

declare_print_commit_metadata() {
	declare -p COMMIT_METADATA_COMMITS COMMIT_METADATA_BREAKING COMMIT_METADATA_TITLE COMMIT_METADATA_HUMAN_TITLE COMMIT_METADATA_CATEGORY COMMIT_METADATA_AUTHORS
}

export_commit_metadata() {
	_COMMIT_METADATA_CACHE="${range}:$(declare_print_commit_metadata)"
	export _COMMIT_METADATA_CACHE COMMIT_METADATA_COMMITS COMMIT_METADATA_BREAKING COMMIT_METADATA_TITLE COMMIT_METADATA_HUMAN_TITLE COMMIT_METADATA_CATEGORY COMMIT_METADATA_AUTHORS
}

# _COMMIT_METADATA_CACHE is used to cache the results of this script in
# the environment because bash arrays are not passed on to subscripts.
if [[ ${_COMMIT_METADATA_CACHE:-} == "${range}:"* ]]; then
	eval "${_COMMIT_METADATA_CACHE#*:}"
else
	if [[ ${ignore_missing_metadata} == 1 ]]; then
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
