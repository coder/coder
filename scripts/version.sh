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

# If in Sapling, just print the commit since we don't have tags.
if [[ -d ".sl" ]]; then
	sl log -l 1 | awk '/changeset/ { printf "0.0.0+sl-%s\n", substr($2, 0, 16) }'
	exit 0
fi

if [[ -n "${CODER_FORCE_VERSION:-}" ]]; then
	echo "${CODER_FORCE_VERSION}"
	exit 0
fi

# To make contributing easier, if there are no tags, we'll use a default
# version.
tag_list=$(git tag)
if [[ -z ${tag_list} ]]; then
	log
	log "INFO(version.sh): It appears you've checked out a fork or shallow clone of Coder."
	log "INFO(version.sh): By default GitHub does not include tags when forking."
	log "INFO(version.sh): We will use the default version 2.0.0 for this build."
	log "INFO(version.sh): To pull tags from upstream, use the following commands:"
	log "INFO(version.sh):   - git remote add upstream https://github.com/coder/coder.git"
	log "INFO(version.sh):   - git fetch upstream"
	log
	last_tag="v2.0.0"
else
	current_commit=$(git rev-parse HEAD)
	# Try to find the last tag that contains the current commit
	last_tag=$(git tag --contains "$current_commit" --sort=version:refname | head -n 1)
	# If there is no tag that contains the current commit,
	# get the latest tag sorted by semver.
	if [[ -z "${last_tag}" ]]; then
		last_tag=$(git tag --sort=version:refname | tail -n 1)
	fi
fi

version="${last_tag}"

# If the HEAD has extra commits since the last tag then we are in a dev version.
#
# Dev versions are denoted by the "-devel+" suffix with a trailing commit short
# SHA.
if [[ "${CODER_RELEASE:-}" == *t* ]]; then
	# $last_tag will equal `git describe --always` if we currently have the tag
	# checked out.
	if [[ "${last_tag}" != "$(git describe --always)" ]]; then
		# make won't exit on $(shell cmd) failures, so we have to kill it :(
		if [[ "$(ps -o comm= "${PPID}" || true)" == *make* ]]; then
			log "ERROR: version.sh: the current commit is not tagged with an annotated tag"
			kill "${PPID}" || true
			exit 1
		fi

		error "version.sh: the current commit is not tagged with an annotated tag"
	fi
else
	version+="-devel+$(git rev-parse --short HEAD)"
fi

# Remove the "v" prefix.
echo "${version#v}"
