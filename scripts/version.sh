#!/usr/bin/env bash

# This script generates the version string used by Coder, including for dev
# versions. Note: the version returned by this script will NOT include the "v"
# prefix that is included in the Git tag.
#
# If $CODER_RELEASE is set to "true", the returned version will equal the
# current git tag. If the current commit is not tagged, this will fail.
#
# If $CODER_RELEASE is not set, the returned version will always be a dev
# version.

set -euo pipefail
# shellcheck source=scripts/lib.sh
source "$(dirname "${BASH_SOURCE[0]}")/lib.sh"
cdroot

if [[ "${CODER_FORCE_VERSION:-}" != "" ]]; then
	echo "$CODER_FORCE_VERSION"
	exit 0
fi

last_tag="$(git describe --tags --abbrev=0)"
version="$last_tag"

# If the HEAD has extra commits since the last tag then we are in a dev version.
#
# Dev versions are denoted by the "-devel+" suffix with a trailing commit short
# SHA.
if [[ "${CODER_RELEASE:-}" == *t* ]]; then
	# $last_tag will equal `git describe --always` if we currently have the tag
	# checked out.
	if [[ "$last_tag" != "$(git describe --always)" ]]; then
		# make won't exit on $(shell cmd) failures, so we have to kill it :(
		if [[ "$(ps -o comm= "$PPID" || true)" == *make* ]]; then
			log "ERROR: version.sh: the current commit is not tagged with an annotated tag"
			kill "$PPID" || true
			exit 1
		fi

		error "version.sh: the current commit is not tagged with an annotated tag"
	fi
else
	version+="-devel+$(git rev-parse --short HEAD)"
fi

# Remove the "v" prefix.
echo "${version#v}"
