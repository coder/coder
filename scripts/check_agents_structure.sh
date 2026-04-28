#!/usr/bin/env bash
set -euo pipefail
# shellcheck source=scripts/lib.sh
# shellcheck disable=SC1091
source "$(dirname "${BASH_SOURCE[0]}")/lib.sh"
cdroot

echo "--- check agent docs structure"

required_docs=(
	".claude/docs/OBSERVABILITY.md"
	".claude/docs/DEV_ISOLATION.md"
	".claude/docs/AGENT_FAILURES.md"
)

fail=0

for doc in "${required_docs[@]}"; do
	if [[ ! -f "$doc" ]]; then
		echo "error: required harness doc is missing: $doc"
		fail=1
	fi
done

is_reference_path() {
	local ref="$1"
	case "$ref" in
	*/* | package.json | AGENTS.local.md)
		return 0
		;;
	*)
		return 1
		;;
	esac
}

# TODO: Add circular AGENTS.md include detection if nested agent docs begin
# referencing each other. Current checks validate file existence only.
mapfile -t agent_files < <(git ls-files '*AGENTS.md' | sort)

for agent_file in "${agent_files[@]}"; do
	agent_dir="$(dirname "$agent_file")"
	while IFS=$'\t' read -r line_number ref; do
		if [[ -z "${line_number:-}" || -z "${ref:-}" ]]; then
			continue
		fi
		if ! is_reference_path "$ref"; then
			continue
		fi

		candidate="$agent_dir/$ref"
		candidate="${candidate#./}"
		if [[ -e "$candidate" ]]; then
			continue
		fi

		if [[ "$(basename "$ref")" == "AGENTS.local.md" ]]; then
			echo "warning: $agent_file:$line_number: optional local agent file is not present: $ref"
			continue
		fi

		echo "error: $agent_file:$line_number: referenced file does not exist: $ref"
		fail=1
	done < <(
		awk '
			/^[[:space:]]*(-[[:space:]]+)?@/ {
				ref = $0
				sub(/^[[:space:]]*(-[[:space:]]+)?@/, "", ref)
				sub(/[[:space:]`)>].*$/, "", ref)
				sub(/[,:;)]+$/, "", ref)
				print FNR "\t" ref
			}
		' "$agent_file"
	)
done

if [[ -f AGENTS.md ]]; then
	root_agent_lines=$(wc -l <AGENTS.md)
	if ((root_agent_lines > 600)); then
		echo "warning: AGENTS.md is $root_agent_lines lines, consider keeping the root guide concise."
	fi
fi

if [[ "$fail" -ne 0 ]]; then
	exit 1
fi

echo "OK: agent docs structure looks valid."
