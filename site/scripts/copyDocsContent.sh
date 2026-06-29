#!/usr/bin/env bash
# Copies the documentation markdown and manifest from docs/ into
# site/static/docs-content/ so the dashboard can fetch and render them
# at /docs-content/*. Vite copies static/ into site/out/, which fat
# binaries embed and serve. The destination is gitignored and rebuilt
# from scratch on every run. Images are intentionally NOT copied; the
# dashboard loads them from GitHub raw URLs pinned to the build version.
#
# The Vite dev server does not watch docs/; re-run pnpm dev to pick up
# documentation changes during development.
set -euo pipefail

cd "$(dirname "${BASH_SOURCE[0]}")/.."

# Source builds that vendor only site/ (for example the nix flake) have
# no docs directory; skip the copy so the build still succeeds, just
# without embedded docs content.
if [[ ! -d ../docs ]]; then
	echo "copyDocsContent.sh: ../docs not found, skipping docs content copy" >&2
	exit 0
fi

if ! command -v rsync >/dev/null; then
	echo "copyDocsContent.sh: rsync is required but not installed" >&2
	exit 1
fi

dest="static/docs-content"
rm -rf "$dest"
mkdir -p "$dest"

echo "Copying docs content to $dest..."
rsync -a \
	--include='*/' \
	--include='*.md' \
	--include='manifest.json' \
	--exclude='*' \
	--prune-empty-dirs \
	../docs/ "$dest/"
