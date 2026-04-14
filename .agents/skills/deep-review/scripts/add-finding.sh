#!/usr/bin/env bash

# This script appends a structured JSON finding to a reviewer's output
# file. It handles JSON construction, escaping, and field validation so
# reviewer sub-agents can focus on content rather than formatting.
#
# Usage: ./add-finding.sh --output <path> --severity <P0|P1|P2|P3|P4|Obs|Nit> \
#          --file <path> --line <number> --summary <text> --evidence <text> \
#          --reviewer <role-name>

set -euo pipefail
# shellcheck source=scripts/lib.sh
source "$(dirname "${BASH_SOURCE[0]}")/../../../../scripts/lib.sh"

dependencies jq

output=""
severity=""
file=""
line=""
summary=""
evidence=""
reviewer=""

args="$(getopt -o "" -l output:,severity:,file:,line:,summary:,evidence:,reviewer: -- "$@")"
eval set -- "$args"
while true; do
	case "$1" in
	--output)
		output="$2"
		shift 2
		;;
	--severity)
		severity="$2"
		shift 2
		;;
	--file)
		file="$2"
		shift 2
		;;
	--line)
		line="$2"
		shift 2
		;;
	--summary)
		summary="$2"
		shift 2
		;;
	--evidence)
		evidence="$2"
		shift 2
		;;
	--reviewer)
		reviewer="$2"
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

# Validate severity against the allowed set.
case "$severity" in
P0 | P1 | P2 | P3 | P4 | Obs | Nit) ;;
"")
	error "--severity is a required parameter"
	;;
*)
	error "Invalid --severity '$severity', must be one of: P0, P1, P2, P3, P4, Obs, Nit"
	;;
esac

# Validate required fields.
if [[ -z "$output" ]]; then
	error "--output is a required parameter"
fi
if [[ -z "$reviewer" ]]; then
	error "--reviewer is a required parameter"
fi
if [[ -z "$summary" ]]; then
	error "--summary is a required parameter"
fi

# Severity-specific required fields.
case "$severity" in
P0 | P1 | P2 | P3 | P4)
	if [[ -z "$file" ]]; then
		error "--file is required for severity $severity"
	fi
	if [[ -z "$line" ]]; then
		error "--line is required for severity $severity"
	fi
	if [[ -z "$evidence" ]]; then
		error "--evidence is required for severity $severity"
	fi
	;;
Nit)
	if [[ -z "$file" ]]; then
		error "--file is required for severity Nit"
	fi
	;;
Obs)
	# Only --summary and --reviewer are required for observations.
	;;
esac

# Initialize the output file if it doesn't exist.
if [[ ! -f "$output" ]]; then
	echo '[]' >"$output"
fi

# Build jq arguments. Use --argjson for line so it becomes a number
# (or null), and --arg for strings. Null-valued optional fields are
# passed as jq null via --argjson.
jq_args=(
	--arg sev "$severity"
	--arg summary "$summary"
	--arg reviewer "$reviewer"
)

if [[ -n "$file" ]]; then
	jq_args+=(--arg file "$file")
else
	jq_args+=(--argjson file "null")
fi

if [[ -n "$line" ]]; then
	jq_args+=(--argjson line "$line")
else
	jq_args+=(--argjson line "null")
fi

if [[ -n "$evidence" ]]; then
	jq_args+=(--arg evidence "$evidence")
else
	jq_args+=(--argjson evidence "null")
fi

tmp="$(mktemp)"
jq "${jq_args[@]}" \
	'. += [{ severity: $sev, file: $file, line: $line, summary: $summary, evidence: $evidence, reviewer: $reviewer }]' \
	"$output" >"$tmp" && mv "$tmp" "$output"
