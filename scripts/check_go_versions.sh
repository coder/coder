#!/usr/bin/env bash

# This script ensures that the same version of Go is referenced in all of the
# following files:
# - go.mod
# - dogfood/coder/Dockerfile
# - flake.nix
# - .github/actions/setup-go/action.yml
# The version of Go in go.mod is considered the source of truth.

set -euo pipefail
source "$(dirname "${BASH_SOURCE[0]}")/lib.sh"
cdroot

# At the time of writing, Nix only has go 1.22.x.
# We don't want to fail the build for this reason.
IGNORE_NIX=${IGNORE_NIX:-false}

GO_VERSION_GO_MOD=$(grep -Eo 'go [0-9]+\.[0-9]+\.[0-9]+' ./go.mod | cut -d' ' -f2)
GO_VERSION_DOCKERFILE=$(grep -Eo 'ARG GO_VERSION=[0-9]+\.[0-9]+\.[0-9]+' ./dogfood/coder/Dockerfile | cut -d'=' -f2)
GO_VERSION_SETUP_GO=$(yq '.inputs.version.default' .github/actions/setup-go/action.yaml)
GO_VERSION_FLAKE_NIX=$(grep -Eo '\bgo_[0-9]+_[0-9]+\b' ./flake.nix)
# Convert to major.minor format.
GO_VERSION_FLAKE_NIX_MAJOR_MINOR=$(echo "$GO_VERSION_FLAKE_NIX" | cut -d '_' -f 2-3 | tr '_' '.')
log "INFO : go.mod                   : $GO_VERSION_GO_MOD"
log "INFO : dogfood/coder/Dockerfile : $GO_VERSION_DOCKERFILE"
log "INFO : setup-go/action.yaml     : $GO_VERSION_SETUP_GO"
log "INFO : flake.nix                : $GO_VERSION_FLAKE_NIX_MAJOR_MINOR"

if [ "$GO_VERSION_GO_MOD" != "$GO_VERSION_DOCKERFILE" ]; then
	error "Go version mismatch between go.mod and dogfood/coder/Dockerfile:"
fi

if [ "$GO_VERSION_GO_MOD" != "$GO_VERSION_SETUP_GO" ]; then
	error "Go version mismatch between go.mod and .github/actions/setup-go/action.yaml"
fi

# At the time of writing, Nix only constrains the major.minor version.
# We need to check that specifically.
if [ "$IGNORE_NIX" = "false" ]; then
	GO_VERSION_GO_MOD_MAJOR_MINOR=$(echo "$GO_VERSION_GO_MOD" | cut -d '.' -f 1-2)
	if [ "$GO_VERSION_FLAKE_NIX_MAJOR_MINOR" != "$GO_VERSION_GO_MOD_MAJOR_MINOR" ]; then
		error "Go version mismatch between go.mod and flake.nix"
	fi
else
	log "INFO : Ignoring flake.nix, as IGNORE_NIX=${IGNORE_NIX}"
fi

log "Go version check passed, all versions are $GO_VERSION_GO_MOD"
