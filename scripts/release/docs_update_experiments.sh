#!/usr/bin/env bash

# Usage: ./docs_update_experiments.sh
#
# This script updates the available experimental features in the documentation.
# It fetches the latest mainline and stable releases to extract the available
# experiments and their descriptions. The script will update the
# feature-stages.md file with a table of the latest experimental features.

set -euo pipefail
# shellcheck source=scripts/lib.sh
source "$(dirname "${BASH_SOURCE[0]}")/../lib.sh"
cdroot

if isdarwin; then
	dependencies gsed gawk
	sed() { gsed "$@"; }
	awk() { gawk "$@"; }
fi

# From install.sh
echo_latest_stable_version() {
	# https://gist.github.com/lukechilds/a83e1d7127b78fef38c2914c4ececc3c#gistcomment-2758860
	version="$(curl -fsSLI -o /dev/null -w "%{url_effective}" https://github.com/coder/coder/releases/latest)"
	version="${version#https://github.com/coder/coder/releases/tag/v}"
	echo "v${version}"
}

echo_latest_mainline_version() {
	# Fetch the releases from the GitHub API, sort by version number,
	# and take the first result. Note that we're sorting by space-
	# separated numbers and without utilizing the sort -V flag for the
	# best compatibility.
	echo "v$(
		curl -fsSL https://api.github.com/repos/coder/coder/releases |
			awk -F'"' '/"tag_name"/ {print $4}' |
			tr -d v |
			tr . ' ' |
			sort -k1,1nr -k2,2nr -k3,3nr |
			head -n1 |
			tr ' ' .
	)"
}

# For testing or including experiments from `main`.
echo_latest_main_version() {
	echo origin/main
}

sparse_clone_codersdk() {
	mkdir -p "${1}"
	cd "${1}"
	rm -rf "${2}"
	git clone --quiet --no-checkout "${PROJECT_ROOT}" "${2}"
	cd "${2}"
	git sparse-checkout set --no-cone codersdk
	git checkout "${3}" -- codersdk
	echo "${1}/${2}"
}

parse_all_experiments() {
	# Go doc doesn't include inline array comments, so this parsing should be
	# good enough. We remove all whitespaces so that we can extract a plain
	# string that looks like {}, {ExpA}, or {ExpA,ExpB,}.
	#
	# Example: ExperimentsAll=Experiments{ExperimentNotifications,ExperimentAutoFillParameters,}
	go doc -all -C "${dir}" ./codersdk ExperimentsAll |
		tr -d $'\n\t ' |
		grep -E -o 'ExperimentsAll=Experiments\{[^}]*\}' |
		sed -e 's/.*{\(.*\)}.*/\1/' |
		tr ',' '\n'
}

parse_experiments() {
	# Extracts the experiment name and description from the Go doc output.
	# The output is in the format:
	#
	# 	||Add new experiments here!
	# 	ExperimentExample|example|This isn't used for anything.
	# 	ExperimentAutoFillParameters|auto-fill-parameters|This should not be taken out of experiments until we have redesigned the feature.
	# 	ExperimentMultiOrganization|multi-organization|Requires organization context for interactions, default org is assumed.
	# 	ExperimentCustomRoles|custom-roles|Allows creating runtime custom roles.
	# 	ExperimentNotifications|notifications|Sends notifications via SMTP and webhooks following certain events.
	# 	ExperimentWorkspaceUsage|workspace-usage|Enables the new workspace usage tracking.
	# 	||ExperimentTest is an experiment with
	# 	||a preceding multi line comment!?
	# 	ExperimentTest|test|
	#
	go doc -all -C "${1}" ./codersdk Experiment |
		sed \
			-e 's/\t\(Experiment[^ ]*\)\ \ *Experiment = "\([^"]*\)"\(.*\/\/ \(.*\)\)\?/\1|\2|\4/' \
			-e 's/\t\/\/ \(.*\)/||\1/' |
		grep '|'
}

workdir=build/docs/experiments
dest=docs/install/releases/feature-stages.md

log "Updating available experimental features in ${dest}"

declare -A experiments=() experiment_tags=()

for channel in mainline stable; do
	log "Fetching experiments from ${channel}"

	tag=$(echo_latest_"${channel}"_version)
	dir="$(sparse_clone_codersdk "${workdir}" "${channel}" "${tag}")"

	declare -A all_experiments=()
	all_experiments_out="$(parse_all_experiments "${dir}")"
	if [[ -n "${all_experiments_out}" ]]; then
		readarray -t all_experiments_tmp <<<"${all_experiments_out}"
		for exp in "${all_experiments_tmp[@]}"; do
			all_experiments[$exp]=1
		done
	fi

	# Track preceding/multiline comments.
	maybe_desc=

	while read -r line; do
		line=${line//$'\n'/}
		readarray -d '|' -t parts <<<"$line"

		# Missing var/key, this is a comment or description.
		if [[ -z ${parts[0]} ]]; then
			maybe_desc+="${parts[2]//$'\n'/ }"
			continue
		fi

		var="${parts[0]}"
		key="${parts[1]}"
		desc="${parts[2]}"
		desc=${desc//$'\n'/}

		# If desc (trailing comment) is empty, use the preceding/multiline comment.
		if [[ -z "${desc}" ]]; then
			desc="${maybe_desc% }"
		fi
		maybe_desc=

		# Skip experiments not listed in ExperimentsAll.
		if [[ ! -v all_experiments[$var] ]]; then
			log "Skipping ${var}, not listed in ExperimentsAll"
			continue
		fi

		# Don't overwrite desc, prefer first come, first served (i.e. mainline > stable).
		if [[ ! -v experiments[$key] ]]; then
			experiments[$key]="$desc"
		fi

		# Track the release channels where the experiment is available.
		experiment_tags[$key]+="${channel}, "
	done < <(parse_experiments "${dir}")
done

table="$(
	if [[ "${#experiments[@]}" -eq 0 ]]; then
		echo "Currently no experimental features are available in the latest mainline or stable release."
		exit 0
	fi

	echo "| Feature | Description | Available in |"
	echo "|---------|-------------|--------------|"
	for key in "${!experiments[@]}"; do
		desc=${experiments[$key]}
		tags=${experiment_tags[$key]%, }
		echo "| \`$key\` | $desc | ${tags} |"
	done
)"

# Use awk to print everything outside the BEING/END block and insert the
# table in between.
awk \
	-v table="${table}" \
	'BEGIN{include=1} /BEGIN: available-experimental-features/{print; print table; include=0} /END: available-experimental-features/{include=1} include' \
	"${dest}" \
	>"${dest}".tmp
mv "${dest}".tmp "${dest}"

# Format the file for a pretty table (target single file for speed).
(cd site && pnpm exec prettier --cache --write ../"${dest}")
