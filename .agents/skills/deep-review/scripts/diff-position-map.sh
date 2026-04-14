#!/usr/bin/env bash

# This script parses a unified diff and produces a JSON mapping of
# {file, line} → diff_position suitable for posting GitHub PR review
# comments.
#
# Usage:
#   cat diff.patch | ./diff-position-map.sh
#   ./diff-position-map.sh --diff path/to/diff
#   ./diff-position-map.sh --diff path/to/diff --file coderd/handler.go

set -euo pipefail
# shellcheck source=scripts/lib.sh
source "$(dirname "${BASH_SOURCE[0]}")/../../../../scripts/lib.sh"

dependencies jq

diff_path=""
file_filter=""

args="$(getopt -o "" -l diff:,file: -- "$@")"
eval set -- "$args"
while true; do
	case "$1" in
	--diff)
		diff_path="$2"
		shift 2
		;;
	--file)
		file_filter="$2"
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

# Awk script that emits file\tline\tposition rows from unified diff.
# Uses only POSIX awk features (no gawk-specific match() with arrays).
# shellcheck disable=SC2016
awk_script='
/^diff --git / {
	# Extract the new-side path (b/...).
	n = split($0, parts, " ")
	raw = parts[n]
	# Strip leading "b/".
	file = substr(raw, 3)
	file_position = 0
	in_file = 0
	next
}

/^@@/ {
	if (!in_file) {
		in_file = 1
		file_position = 1
	} else {
		file_position++
	}
	# Parse +start from the @@ header.
	# Find the field that starts with + and extract the number.
	for (i = 1; i <= NF; i++) {
		if ($i ~ /^\+[0-9]/) {
			val = $i
			sub(/^\+/, "", val)
			sub(/,.*/, "", val)
			new_line = val + 0
			break
		}
	}
	next
}

# Skip lines before the first hunk header (e.g. ---/+++ lines).
!in_file { next }

/^\\ No newline at end of file/ {
	file_position++
	next
}

/^\+/ {
	file_position++
	print file "\t" new_line "\t" file_position
	new_line++
	next
}

/^-/ {
	file_position++
	next
}

/^ / {
	file_position++
	print file "\t" new_line "\t" file_position
	new_line++
	next
}
'

# Assemble nested JSON from the TSV rows.
jq_script='
[inputs | split("\t") | {file: .[0], line: .[1], pos: .[2]}]
| group_by(.file)
| map({key: .[0].file, value: (map({key: .line, value: (.pos | tonumber)}) | from_entries)})
| from_entries
'

if [[ -n "$diff_path" ]]; then
	diff_input() { cat "$diff_path"; }
else
	diff_input() { cat; }
fi

if [[ -n "$file_filter" ]]; then
	diff_input | awk "$awk_script" | awk -F'\t' -v f="$file_filter" '$1 == f' | jq -Rn "$jq_script"
else
	diff_input | awk "$awk_script" | jq -Rn "$jq_script"
fi
