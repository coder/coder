#!/bin/bash

set -euo pipefail

SCRIPT_DIR=$(dirname "${BASH_SOURCE[0]}")
PROJECT_ROOT=$(cd "$SCRIPT_DIR" && git rev-parse --show-toplevel)

(
	cd "${PROJECT_ROOT}"

	FILES="$(git ls-files --other --modified --exclude-standard)"
	if [[ "$FILES" != "" ]]; then
		mapfile -t files <<<"$FILES"

		echo "The following files contain unstaged changes:"
		echo
		for file in "${files[@]}"; do
			echo "  - $file"
		done
		echo

		echo "These are the changes:"
		echo
		for file in "${files[@]}"; do
			git --no-pager diff "$file"
		done
		exit 1
	fi
)
exit 0
