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

# Ensure GITHUB_TOKEN is available
if [[ -z "${GITHUB_TOKEN:-}" ]]; then
	if GITHUB_TOKEN="$(gh auth token 2>/dev/null)"; then
		export GITHUB_TOKEN
	else
		echo "Error: GitHub token not found. Please run 'gh auth login' to authenticate." >&2
		exit 1
	fi
fi

if isdarwin; then
	dependencies gsed gawk
	sed() { gsed "$@"; }
	awk() { gawk "$@"; }
fi

echo_latest_stable_version() {
	# Extract redirect URL to determine latest stable tag
	version="$(curl -fsSLI -o /dev/null -w "%{url_effective}" https://github.com/coder/coder/releases/latest)"
	version="${version#https://github.com/coder/coder/releases/tag/v}"
	echo "v${version}"
}

echo_latest_mainline_version() {
	# Use GitHub API to get latest release version, authenticated
	echo "v$(
		curl -fsSL -H "Authorization: token ${GITHUB_TOKEN}" https://api.github.com/repos/coder/coder/releases |
			awk -F'"' '/"tag_name"/ {print $4}' |
			tr -d v |
			tr . ' ' |
			sort -k1,1nr -k2,2nr -k3,3nr |
			head -n1 |
			tr ' ' .
	)"
}

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
	# Try ExperimentsSafe first, then fall back to ExperimentsAll if needed
	experiments_var="ExperimentsSafe"
	experiments_output=$(go doc -all -C "${dir}" ./codersdk "${experiments_var}" 2>/dev/null || true)

	if [[ -z "${experiments_output}" ]]; then
		# Fall back to ExperimentsAll if ExperimentsSafe is not found
		experiments_var="ExperimentsAll"
		experiments_output=$(go doc -all -C "${dir}" ./codersdk "${experiments_var}" 2>/dev/null || true)

		if [[ -z "${experiments_output}" ]]; then
			log "Warning: Neither ExperimentsSafe nor ExperimentsAll found in ${dir}"
			return
		fi
	fi

	echo "${experiments_output}" |
		tr -d $'\n\t ' |
		grep -E -o "${experiments_var}=Experiments\{[^}]*\}" |
		sed -e 's/.*{\(.*\)}.*/\1/' |
		tr ',' '\n'
}

parse_experiments() {
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
	if [[ -z "${tag}" || "${tag}" == "v" ]]; then
		echo "Error: Failed to retrieve valid ${channel} version tag. Check your GitHub token or rate limit." >&2
		exit 1
	fi

	dir="$(sparse_clone_codersdk "${workdir}" "${channel}" "${tag}")"

	declare -A all_experiments=()
	all_experiments_out="$(parse_all_experiments "${dir}")"
	if [[ -n "${all_experiments_out}" ]]; then
		readarray -t all_experiments_tmp <<<"${all_experiments_out}"
		for exp in "${all_experiments_tmp[@]}"; do
			all_experiments[$exp]=1
		done
	fi

	maybe_desc=

	while read -r line; do
		line=${line//$'\n'/}
		readarray -d '|' -t parts <<<"$line"

		if [[ -z ${parts[0]} ]]; then
			maybe_desc+="${parts[2]//$'\n'/ }"
			continue
		fi

		var="${parts[0]}"
		key="${parts[1]}"
		desc="${parts[2]}"
		desc=${desc//$'\n'/}

		if [[ -z "${desc}" ]]; then
			desc="${maybe_desc% }"
		fi
		maybe_desc=

		if [[ ! -v all_experiments[$var] ]]; then
			log "Skipping ${var}, not listed in experiments list"
			continue
		fi

		if [[ ! -v experiments[$key] ]]; then
			experiments[$key]="$desc"
		fi

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

awk \
	-v table="${table}" \
	'BEGIN{include=1} /BEGIN: available-experimental-features/{print; print table; include=0} /END: available-experimental-features/{include=1} include' \
	"${dest}" \
	>"${dest}".tmp
mv "${dest}".tmp "${dest}"

(cd site && pnpm exec prettier --cache --write ../"${dest}")
