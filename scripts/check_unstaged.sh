#!/bin/bash

set -euo pipefail

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

exit 0
