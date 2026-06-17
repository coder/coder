#!/usr/bin/env bash

# Known active ESR (Extended Support Release) minor versions.
# Update this list when new ESR versions are designated or old ones reach end
# of life. This file is sourced by the backport workflow and the release
# calendar generator.
# shellcheck disable=SC2034 # ESR_VERSIONS is used by the sourcing script.
ESR_VERSIONS=(29 34)
