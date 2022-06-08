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

# The notarization process is very fragile and heavily dependent on Apple's
# notarization server not returning server errors, so we retry this step 5
# times with a delay of 30 seconds between each attempt.
rc=0
for i in $(seq 1 5); do
	gon "$config" && rc=0 && break || rc=$?
	echo "gon exit code: $rc"
	if [ "$i" -lt 5 ]; then
		echo
		echo "Retrying notarization in 30 seconds"
		echo
		sleep 30
	else
		echo
		echo "Giving up :("
	fi
done

exit $rc
