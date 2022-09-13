#!/bin/bash

set -euo pipefail
# shellcheck source=scripts/lib.sh
source "$(dirname "${BASH_SOURCE[0]}")/lib.sh"
cdroot

FILES="$(git ls-files --other --modified --exclude-standard)"
if [[ "$FILES" != "" ]]; then
	mapfile -t files <<<"$FILES"

	log
	log "The following files contain unstaged changes:"
	log
	for file in "${files[@]}"; do
		log "  - $file"
	done

	log
	log "These are the changes:"
	log
	for file in "${files[@]}"; do
		git --no-pager diff "$file" 1>&2
	done

	log
	error "Unstaged changes, see above for details."
fi
