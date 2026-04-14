#!/usr/bin/env bash

# This script reads all reviewer JSON files from a review directory,
# deduplicates findings at the same location, identifies convergence
# (multiple reviewers at the same file+line), computes max severity,
# and outputs a consolidated findings inventory.
#
# Usage: ./compile-findings.sh --dir <review-dir> [--output <path>]

set -euo pipefail
# shellcheck source=scripts/lib.sh
source "$(dirname "${BASH_SOURCE[0]}")/../../../../scripts/lib.sh"

dependencies jq

dir=""
output=""

args="$(getopt -o "" -l dir:,output: -- "$@")"
eval set -- "$args"
while true; do
	case "$1" in
	--dir)
		dir="$2"
		shift 2
		;;
	--output)
		output="$2"
		shift 2
		;;
	--)
		shift
		break
		;;
	*)
		error "Unexpected argument: $1"
		;;
	esac
done

if [[ -z "$dir" ]]; then
	error "--dir is required"
fi

if [[ ! -d "$dir" ]]; then
	error "Directory does not exist: $dir"
fi

# Collect all findings from JSON array files in the directory.
all_findings="[]"
for f in "$dir"/*.json; do
	[[ -f "$f" ]] || continue

	# Skip files that aren't valid JSON or aren't arrays.
	file_type="$(jq -r 'type' "$f" 2>/dev/null)" || continue
	if [[ "$file_type" != "array" ]]; then
		log "Skipping non-array JSON file: $f"
		continue
	fi

	# Skip empty arrays.
	count="$(jq 'length' "$f")"
	if [[ "$count" -eq 0 ]]; then
		log "Skipping empty array: $f"
		continue
	fi

	all_findings="$(echo "$all_findings" | jq --slurpfile new "$f" '. + $new[0]')"
done

# Process all findings with jq: group, compute severity, detect
# convergence, and produce final output.
result="$(echo "$all_findings" | jq '
# Severity ordering: lower number = higher severity.
def sev_rank:
	if . == "P0" then 0
	elif . == "P1" then 1
	elif . == "P2" then 2
	elif . == "P3" then 3
	elif . == "P4" then 4
	elif . == "Obs" then 5
	elif . == "Nit" then 6
	else 99
	end;

def rank_to_sev:
	if . == 0 then "P0"
	elif . == 1 then "P1"
	elif . == 2 then "P2"
	elif . == 3 then "P3"
	elif . == 4 then "P4"
	elif . == 5 then "Obs"
	elif . == 6 then "Nit"
	else "Unknown"
	end;

# Group findings by file+line. Findings without file or line get
# unique keys so they are never grouped together.
def group_key:
	if (.file // "") == "" or (.line // null) == null then
		"__ungrouped__\(.)| " + (. | @json | @base64)
	else
		"\(.file):\(.line)"
	end;

# Build grouped findings.
(group_by(group_key)) |
map(
	. as $group |
	# Compute max severity (lowest rank number).
	($group | map(.severity | sev_rank) | min) as $max_rank |
	($max_rank | rank_to_sev) as $max_sev |
	# Pick summary from the reviewer with max severity (first match).
	($group | map(select((.severity | sev_rank) == $max_rank)) | .[0].summary) as $top_summary |
	# Build reviewer list.
	($group | map({
		role: .reviewer,
		severity: .severity,
		summary: .summary,
		evidence: .evidence
	})) as $reviewers |
	# Convergent if 2+ distinct reviewers.
	([$reviewers[].role] | unique | length >= 2) as $convergent |
	{
		file: $group[0].file,
		line: $group[0].line,
		summary: $top_summary,
		reviewers: $reviewers,
		max_severity: $max_sev,
		convergent: $convergent
	}
) as $findings |

# Compute stats.
{
	findings: $findings,
	stats: {
		total_findings: ($findings | length),
		by_severity: (
			{"P0": 0, "P1": 0, "P2": 0, "P3": 0, "P4": 0, "Obs": 0, "Nit": 0} as $init |
			reduce $findings[] as $f ($init;
				.[$f.max_severity] += 1
			)
		),
		convergent_count: ([$findings[] | select(.convergent)] | length),
		reviewers_reporting: ([$findings[].reviewers[].role] | unique)
	}
}
')"

if [[ -n "$output" ]]; then
	mkdir -p "$(dirname "$output")"
	echo "$result" >"$output"
	log "Findings written to $output"
else
	echo "$result"
fi
