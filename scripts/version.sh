#!/usr/bin/env bash

# This script generates the version string used by Coder, including for dev
# versions. Note: the version returned by this script will NOT include the "v"
# prefix that is included in the Git tag.
#
# If $CODER_FORCE_DEV_VERSION is set to "true", the returned version will be a
# dev version even if the current commit is tagged.
#
# If $CODER_NO_DEV_VERSION is set to "true", the script will fail if the current
# commit is not tagged.

set -euo pipefail
# shellcheck source=scripts/lib.sh
source "$(dirname "${BASH_SOURCE[0]}")/lib.sh"
cdroot

# $current will equal $last_tag if we currently have the tag checked out.
last_tag="$(git describe --tags --abbrev=0)"
current="$(git describe --always)"

version="$last_tag"

# If the HEAD has extra commits since the last tag then we are in a dev version.
#
# Dev versions are denoted by the "-devel+" suffix with a trailing commit short
# SHA.
if [[ "${CODER_FORCE_DEV_VERSION:-}" == *t* ]] || [[ "$last_tag" != "$current" ]]; then
	if [[ "${CODER_NO_DEV_VERSION:-}" == *t* ]]; then
		# make won't exit on $(shell cmd) failures :(
		if [[ "$(ps -o comm= "$PPID" || true)" == *make* ]]; then
			log "ERROR: version.sh attemped to generate a dev version string when CODER_NO_DEV_VERSION was set"
			kill "$PPID" || true
			exit 1
		fi

		error "version.sh attemped to generate a dev version string when CODER_NO_DEV_VERSION was set"
	fi
	version+="-devel+$(git rev-parse --short HEAD)"
fi

# Remove the "v" prefix.
echo "${version#v}"
