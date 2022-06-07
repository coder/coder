#!/usr/bin/env bash

# This script generates the version string used by Coder, including for dev
# versions. Note: the version returned by this script will NOT include the "v"
# prefix that is included in the Git tag.

set -euo pipefail
# shellcheck source=lib.sh
source "$(dirname "${BASH_SOURCE[0]}")/lib.sh"
cdroot

# $current will equal $last_tag if we're currently checked out on the tag's
# commit.
last_tag="$(git describe --tags --abbrev=0)"
current="$(git describe --always)"

version="$last_tag"

# If the HEAD has extra commits since the last tag then we are in a dev version.
#
# Dev versions are denoted by the "+dev." suffix with a trailing commit short
# SHA.
if [[ "$last_tag" != "$current" ]]; then
    if [[ "${CODER_NO_DEV_VERSION:-}" != "" ]]; then
        error "version.sh attemped to generate a dev version string when CODER_NO_DEV_VERSION was set"
    fi
    version+="+dev.$(git rev-parse --short HEAD)"
fi

# Remove the "v" prefix.
echo -n "${version#v}"
