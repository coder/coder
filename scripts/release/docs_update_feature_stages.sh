#!/usr/bin/env bash

# Usage: ./docs_update_feature_stages.sh [file]
#
# Updates the generated sections of feature-stages.md in place. Defaults
# to docs/install/releases/feature-stages.md (relative to the repo root).
# The file must already exist and contain the BEGIN/END marker comments.

set -euo pipefail
# shellcheck source=scripts/lib.sh
source "$(dirname "${BASH_SOURCE[0]}")/../lib.sh"
cdroot

if isdarwin; then
	dependencies gsed gawk
	sed() { gsed "$@"; }
	awk() { gawk "$@"; }
fi

parse_beta_features() {
	jq -r '
		# Collect paths that live under any beta-marked subtree. We exclude
		# the beta node itself so a beta root still emits as a row; only its
		# descendants are suppressed.
		[
			.routes[] | recurse(.children[]?)
			| select((.state // []) | index("beta"))
			| .children[]? | recurse(.children[]?)
			| .path | select(. != null)
		] as $covered
		|
		# Emit every beta node whose path is not covered. A doc cross-listed
		# under both a beta and a non-beta parent is treated as beta-covered
		# and dropped from the table.
		.routes[] | recurse(.children[]?)
		| select((.state // []) | index("beta"))
		| select((.path // "") as $p | $covered | index($p) | not)
		| [.title, (.description // ""), (.path // "")]
		| join("|")
	' "${PROJECT_ROOT}/docs/manifest.json"
}

dest=${1:-docs/install/releases/feature-stages.md}

log "Updating generated feature-stages sections in ${dest}"

# The experiments table comes from docs/experiments.json, which
# scripts/experimentsdocgen generates from codersdk.ExperimentsKnown (every
# experiment this version knows about) and codersdk.ExperimentsSafe (the ones
# `--experiments=*` enables). Reading the generated file keeps the table and
# the machine-readable list identical by construction.
experiments_json="${EXPERIMENTS_JSON:-${PROJECT_ROOT}/docs/experiments.json}"

table="$(
	if [[ ! -f "${experiments_json}" ]] || [[ "$(jq '.experiments | length' "${experiments_json}")" -eq 0 ]]; then
		echo "Currently no experimental features are available."
		exit 0
	fi

	# The last column spells out the command: a flag in the opt-in set is
	# enabled by `--experiments=*` as well as by name; every other flag must be
	# named. Spelling it out keeps the table readable when the opt-in set is
	# empty, which it is today.
	echo "| Feature | Flag | Description | Enable with |"
	echo "| ------- | ---- | ----------- | ----------- |"
	jq -r '.experiments[] | "| \(.displayName) | `\(.id)` | \(.description) | \(if .safe then "`--experiments=*` or `--experiments=\(.id)`" else "`--experiments=\(.id)`" end) |"' "${experiments_json}"
)"

# Collect beta features from the current docs/manifest.json. Keying on the
# route path also dedupes routes that appear under more than one parent.
declare -A beta_features=() beta_feature_descriptions=()
while IFS='|' read -r title desc doc_path; do
	if [[ -z "${title}" ]]; then
		continue
	fi

	key="${doc_path}"
	if [[ -z "${key}" ]]; then
		key="${title}"
	fi

	if [[ ! -v beta_features[$key] ]]; then
		beta_features[$key]="${title}"
		beta_feature_descriptions[$key]="${desc}"
	fi
done < <(parse_beta_features)

beta_table="$(
	if [[ "${#beta_features[@]}" -eq 0 ]]; then
		echo "Currently no beta features are available."
		exit 0
	fi

	echo "| Feature | Description |"
	echo "| ------- | ----------- |"
	for key in "${!beta_features[@]}"; do
		title=${beta_features[$key]}
		desc=${beta_feature_descriptions[$key]}

		# Linkify when the target exists in this tree.
		if [[ "${key}" == ./* ]]; then
			rel="${key#./}"
			if [[ -f "${PROJECT_ROOT}/docs/${rel}" ]]; then
				title="[${title}](../../${rel})"
			fi
		fi

		echo "| ${title} | ${desc} |"
	done
)"

awk \
	-v table="${table}" \
	-v beta_table="${beta_table}" \
	'
	BEGIN{include=1}
	/BEGIN: available-experimental-features/{print; print table; include=0}
	/END: available-experimental-features/{include=1}
	/BEGIN: available-beta-features/{print; print beta_table; include=0}
	/END: available-beta-features/{include=1}
	include
	' \
	"${dest}" \
	>"${dest}".tmp
mv "${dest}".tmp "${dest}"
