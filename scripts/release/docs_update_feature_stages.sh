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

parse_all_experiments() {
	# Try ExperimentsSafe first, then fall back to ExperimentsAll if needed.
	experiments_var="ExperimentsSafe"
	experiments_output=$(go doc -all ./codersdk "${experiments_var}" 2>/dev/null || true)

	if [[ -z "${experiments_output}" ]]; then
		experiments_var="ExperimentsAll"
		experiments_output=$(go doc -all ./codersdk "${experiments_var}" 2>/dev/null || true)

		if [[ -z "${experiments_output}" ]]; then
			log "Warning: Neither ExperimentsSafe nor ExperimentsAll found in ./codersdk"
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
	go doc -all ./codersdk Experiment |
		sed \
			-e 's/\t\(Experiment[^ ]*\)\ \ *Experiment = "\([^"]*\)"\(.*\/\/ \(.*\)\)\?/\1|\2|\4/' \
			-e 's/\t\/\/ \(.*\)/||\1/' |
		grep '|'
}

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

# Collect experiments from the current codersdk package.
declare -A experiments=()
declare -A all_experiments=()
all_experiments_out="$(parse_all_experiments)"
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

	experiments[$key]="$desc"
done < <(parse_experiments)

table="$(
	if [[ "${#experiments[@]}" -eq 0 ]]; then
		echo "Currently no experimental features are available."
		exit 0
	fi

	echo "| Feature | Description |"
	echo "| ------- | ----------- |"
	for key in "${!experiments[@]}"; do
		desc=${experiments[$key]}
		echo "| \`$key\` | $desc |"
	done
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
