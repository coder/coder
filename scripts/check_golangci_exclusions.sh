#!/usr/bin/env bash

# Fails when a .golangci.yaml exclusions.paths entry matches a tracked Go
# file. Entries are regexes, so an unanchored ".git" once hid every path
# containing "<any char>git" from all linters.
set -euo pipefail
# shellcheck source=scripts/lib.sh
source "$(dirname "${BASH_SOURCE[0]}")/lib.sh"
cdroot

patterns=$(awk '/^    paths:$/ { p = 1; next } p && /^      - / { sub(/^      - /, ""); gsub(/^'\''|'\''$/, ""); print; next } { p = 0 }' .golangci.yaml | sort -u)
grep -qxF 'scripts/rules.go' <<<"$patterns" || error "found no exclusions.paths entries in .golangci.yaml"

status=0
while read -r pattern; do
	if matches=$(git ls-files '*.go' | grep -E -- "$pattern"); then
		log "ERROR: .golangci.yaml exclusion path '$pattern' hides tracked Go files:"
		log "$(head -n 5 <<<"$matches")"
		status=1
	fi
done < <(grep -vxF 'scripts/rules.go' <<<"$patterns")
exit "$status"
