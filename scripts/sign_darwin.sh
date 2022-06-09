#!/usr/bin/env bash

# This script notarizes the provided zip file.
#
# Usage: ./publish_release.sh [--version 1.2.3] [--dry-run] path/to/asset1 path/to/asset2 ...
#
# The provided zip file must contain a coder binary that has already been signed
# using the codesign tool.
#
# On success, the input file will be successfully signed and notarized.
#
# Depends on codesign and gon utilities. Requires the $AC_APPLICATION_IDENTITY
# environment variable to be set.

set -euo pipefail
# shellcheck source=lib.sh
source "$(dirname "${BASH_SOURCE[0]}")/lib.sh"

if [[ "${AC_APPLICATION_IDENTITY:-}" == "" ]]; then
	error "AC_APPLICATION_IDENTITY must be set for ./sign_darwin.sh"
fi

# Create the gon config.
config="$(mktemp -d)/gon.json"
jq -r --null-input --arg path "$(pwd)/$1" '{
	"notarize": [
		{
			"path": $path,
			"bundle_id": "com.coder.cli"
		}
	]
}' >"$config"

# Sign the zip file with our certificate.
codesign -s "$AC_APPLICATION_IDENTITY" -f -v --timestamp --options runtime "$1"

# Notarize the signed zip file.
#
# The notarization process is very fragile and heavily dependent on Apple's
# notarization server not returning server errors, so we retry this step 5
# times with a delay of 30 seconds between each attempt.
rc=0
for i in $(seq 1 5); do
	gon "$config" && rc=0 && break || rc=$?
	log "gon exit code: $rc"
	if [ "$i" -lt 5 ]; then
		log
		log "Retrying notarization in 30 seconds"
		log
		sleep 30
	else
		log
		log "Giving up :("
	fi
done

exit $rc
