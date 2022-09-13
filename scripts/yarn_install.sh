#!/usr/bin/env bash
#
# Run "yarn install" with flags appropriate to the environment
# (local development vs build system)
#
# Usage: yarn_install.sh [optional extra flags]

set -euo pipefail
# shellcheck source=scripts/lib.sh
source "$(dirname "${BASH_SOURCE[0]}")/lib.sh"

cdroot
cd site

yarn_flags=(
	# Do not execute install scripts
	# TODO: check if build works properly with this enabled
	# --ignore-scripts

	# Check if existing node_modules are valid
	# TODO: determine if this is necessary
	# --check-files
)

if [[ -n ${CI:-} ]]; then
	yarn_flags+=(
		# Install dependencies from lockfile, ensuring builds are fully
		# reproducible
		--frozen-lockfile
		# Suppress progress information
		--silent
		# Disable interactive prompts for build
		--non-interactive
	)
fi

# Append whatever is specified on the command line
yarn_flags+=("$@")

echo "+ yarn install ${yarn_flags[*]}"
yarn install "${yarn_flags[@]}"
