#!/bin/bash
set -euo pipefail

EXAMPLES_DIR=$(dirname "${BASH_SOURCE[0]}")
PROJECT_ROOT=$(cd "$EXAMPLES_DIR" && git rev-parse --show-toplevel)

# shellcheck source=scripts/lib.sh
source "$PROJECT_ROOT/scripts/lib.sh"

dependencies curl jq sed

sed_args=(-i)
if isdarwin; then
	sed_args=(-i '')
fi

main() {
	pushd "$EXAMPLES_DIR/templates"

	# Fetch the latest release of terraform-provider-coder from GitHub.
	latest_provider_coder="$(curl --fail -sSL https://api.github.com/repos/coder/terraform-provider-coder/releases/latest | jq -r .tag_name)"
	latest_provider_coder=${latest_provider_coder#v}

	# Update all terraform files that contain ~ the following lines:
	#   source  = "coder/coder"
	#   version = "[version]"
	find . -type f -name "*.tf" -print0 | while read -r -d $'\0' f; do
		current_version_raw="$(grep -n -A 1 'source *= *"coder/coder"' "$f" | tail -n 1)"
		if [[ $current_version_raw = *version* ]]; then
			line="${current_version_raw%%-*}"
			sed "${sed_args[@]}" "$line s/\".*\"/\"$latest_provider_coder\"/" "$f"
		fi
	done
}

# Wrap the main function in a subshell to restore the working directory.
(main)
