#!/bin/bash

set -euo pipefail
# shellcheck source=scripts/lib.sh
source "$(dirname "${BASH_SOURCE[0]}")/lib.sh"
cdroot

cd site/static/icon

# These exceptions are here for backwards compatibility. All new icons should
# be SVG to minimize the size of our repo and our bundle.
exceptions=(
	"aws.png"
	"azure.png"
	"docker.png"
	"do.png"
	"gcp.png"
	"k8s.png"
	"ruby.png"
)

function is_exception() {
	local value="$1"
	shift
	for item; do
		[[ "$item" == "$value" ]] && return 0
	done
	return 1
}

for file in *; do
	# Extract filename
	filename=$(basename -- "$file")

	# Check if the file is in the exception list
	if is_exception "$filename" "${exceptions[@]}"; then
		continue
	fi

	# If not an exception, check if it's an svg file
	if [[ "$file" != *.svg ]]; then
		echo "Found a non-svg file not in exception list: $file"
		exit 1
	fi
done
