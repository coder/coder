#!/usr/bin/env bash

# This script fetches starter templates from coder/registry and places them
# into examples/templates/ for embedding into the Coder binary.
#
# Usage: ./scripts/fetch-registry-templates.sh

set -euo pipefail

cdroot() {
	cd "$(git rev-parse --show-toplevel)"
}

cdroot

REGISTRY_REPO="coder/registry"
REGISTRY_BRANCH="main"
REGISTRY_BASE_URL="https://github.com/${REGISTRY_REPO}"
REGISTRY_TEMPLATES_PATH="registry/coder/templates"
EXAMPLES_DIR="examples/templates"

# Templates to fetch from the registry. This list should be a superset of
# the //go:embed directives in examples/examples.go (embedded templates),
# plus any additional templates that exist in the directory for linting.
TEMPLATES=(
	aws-devcontainer
	aws-linux
	aws-windows
	azure-linux
	azure-windows
	digitalocean-linux
	docker
	docker-devcontainer
	docker-envbuilder
	gcp-devcontainer
	gcp-linux
	gcp-vm-container
	gcp-windows
	incus
	kubernetes
	kubernetes-devcontainer
	kubernetes-envbox
	nomad-docker
	scratch
	tasks-docker
)

log() {
	echo "==> $*" >&2
}

# Create a temporary directory for the registry clone.
tmpdir=$(mktemp -d)
trap 'rm -rf "$tmpdir"' EXIT

log "Downloading registry archive from ${REGISTRY_BASE_URL}..."
curl -fsSL "${REGISTRY_BASE_URL}/archive/refs/heads/${REGISTRY_BRANCH}.tar.gz" | tar -xz -C "$tmpdir"

# The archive extracts to a directory named "registry-<branch>".
registry_root="${tmpdir}/registry-${REGISTRY_BRANCH}"

if [[ ! -d "$registry_root" ]]; then
	echo "ERROR: Expected directory ${registry_root} not found" >&2
	ls -la "$tmpdir" >&2
	exit 1
fi

# Fetch each template from the registry.
fetched=0
missing=0
for template in "${TEMPLATES[@]}"; do
	src_dir="${registry_root}/${REGISTRY_TEMPLATES_PATH}/${template}"
	dst_dir="${EXAMPLES_DIR}/${template}"

	if [[ ! -d "$src_dir" ]]; then
		log "WARNING: Template ${template} not found in registry, keeping local version"
		((missing++)) || true
		continue
	fi

	log "Fetching template: ${template}"

	# Remove existing template directory and replace with registry version.
	rm -rf "$dst_dir"
	cp -r "$src_dir" "$dst_dir"

	# Rewrite the icon path in README.md front matter.
	# Registry format:  icon: ../../../../.icons/foo.svg
	# Required format:  icon: ../../../site/static/icon/foo.svg
	if [[ -f "${dst_dir}/README.md" ]]; then
		sed -i 's|icon: ../../../../\.icons/|icon: ../../../site/static/icon/|' "${dst_dir}/README.md"
		# Format markdown tables to match project style.
		pnpm exec markdown-table-formatter "${dst_dir}/README.md" 2>/dev/null || true
	fi
	((fetched++)) || true
done

log "Done. Fetched ${fetched} templates from registry (${missing} not yet available)."
