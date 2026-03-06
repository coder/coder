#!/usr/bin/env bash

# This script builds the Coder Docker image for scaletest use and tags it with
# the branch name and short commit hash.
#
# Usage: ./scripts/build_scaletest_image.sh [--push]
#
# The image is tagged as:
#   us-docker.pkg.dev/coder-scaletest/coder-delta/coder:<branch>-<short-hash>
#
# If --push is supplied, the image will be pushed after building.

set -euo pipefail
# shellcheck source=scripts/lib.sh
source "$(dirname "${BASH_SOURCE[0]}")/lib.sh"
cdroot

push=0
if [[ "${1:-}" == "--push" ]]; then
	push=1
fi

REGISTRY="us-docker.pkg.dev/coder-scaletest/coder-delta/coder"

branch="$(git rev-parse --abbrev-ref HEAD)"
short_hash="$(git rev-parse --short HEAD)"
# Sanitize branch name: replace characters invalid in Docker tags with dashes.
branch_safe="$(echo "$branch" | sed 's/[^a-zA-Z0-9._-]/-/g')"

image_tag="${REGISTRY}:${branch_safe}-${short_hash}"

log "=== Building Coder scaletest image ==="
log "  Branch:    $branch"
log "  Commit:    $short_hash"
log "  Image tag: $image_tag"

# Determine host architecture.
arch="$(go env GOARCH)"

log "--- Building Linux binary (${arch})..."
make -j "build/coder_linux_${arch}"

log "--- Building Docker image..."
./scripts/build_docker.sh \
	--arch "$arch" \
	--target "$image_tag" \
	"build/coder_linux_${arch}"

log "=== Successfully built: $image_tag ==="

if [[ "$push" == 1 ]]; then
	log "--- Pushing image..."
	docker push "$image_tag"
	log "=== Successfully pushed: $image_tag ==="
fi

echo "$image_tag"
