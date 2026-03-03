#!/usr/bin/env bash
# Build the Coder Docker image locally so you can test with Helm without relying on CI.
# Uses the repo's Dockerfile.base (Terraform 1.11.4) and produces a local image.
#
# Usage:
#   ./scripts/build_image_for_helm.sh [tag]
#
# Examples:
#   ./scripts/build_image_for_helm.sh
#     -> builds ghcr.io/coder/coder:local (or CODER_IMAGE_BASE:local)
#   ./scripts/build_image_for_helm.sh my-test
#     -> builds $CODER_IMAGE_BASE:my-test
#
# Then in Helm:
#   helm upgrade coder ... --set coder.image.tag=local
#   # or use your registry and tag

set -euo pipefail
# shellcheck source=scripts/lib.sh
source "$(dirname "${BASH_SOURCE[0]}")/lib.sh"
cdroot

tag="${1:-local}"
version="$(./scripts/version.sh)"
base_tag="${CODER_BASE_IMAGE_TAG:-coder-base:local}"
image_base="${CODER_IMAGE_BASE:-ghcr.io/coder/coder}"
image_tag="${image_base}:${tag}"

log "Building Coder image for Helm (Terraform 1.11.4 from Dockerfile.base)"
log "  Version: $version"
log "  Base tag: $base_tag"
log "  Image: $image_tag"

# 1. Site (needed for embedded frontend in fat binary)
if [[ ! -f site/out/index.html ]]; then
	log "--- Building site (frontend)"
	(cd site && pnpm install && pnpm build)
fi

# 2. Linux amd64 fat binary (cross-compile on Mac/Windows)
if [[ ! -f "build/coder_${version}_linux_amd64" ]]; then
	log "--- Building Linux amd64 binary"
	./scripts/build_go.sh \
		--os linux \
		--arch amd64 \
		--version "$version" \
		--output "build/coder_${version}_linux_amd64"
fi

# 3. Coder image (build base from Dockerfile.base with Terraform 1.11.4, then app image)
log "--- Building base image and Coder image $image_tag"
./scripts/build_docker.sh \
	--arch amd64 \
	--target "$image_tag" \
	--version "$version" \
	--build-base "$base_tag" \
	"build/coder_${version}_linux_amd64"

log "Done. Image: $image_tag"
log ""
log "Use with Helm (example):"
log "  export CODER_IMAGE=$image_tag"
log "  helm upgrade coder coder-v2/coder -n coder-dev -f your-values.yaml --set coder.image.repository=\$(echo $image_base | cut -d: -f1) --set coder.image.tag=$tag"
log ""
log "Or load into kind and point Helm at it:"
log "  docker save $image_tag | kind load docker-image --name kind $image_tag"
log "  helm upgrade ... --set coder.image.pullPolicy=IfNotPresent --set coder.image.tag=$tag"
