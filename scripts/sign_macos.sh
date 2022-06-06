#!/usr/bin/env bash

set -euo pipefail

SCRIPT_DIR=$(dirname "${BASH_SOURCE[0]}")
PROJECT_ROOT=$(cd "$SCRIPT_DIR" && git rev-parse --show-toplevel)

cd "${PROJECT_ROOT}"

codesign -s "$AC_APPLICATION_IDENTITY" -f -v --timestamp --options runtime "$1"

config=$(mktemp -d)/gon.json
jq -r --null-input --arg path "$(pwd)/$1" '{
	"notarize": [
		{
			"path": $path,
			"bundle_id": "com.coder.cli"
		}
	]
}' >"$config"
gon "$config"
