#!/usr/bin/env bash
# Regenerate mise.lock with the helm URL workaround applied.
#
# mise's aqua plugin for `helm` emits `https://get.helm.sh/helm-X.Y.Z-...`
# in the lockfile, but the upstream tarball is published at
# `helm-vX.Y.Z-...` (with the `v` prefix). The 404 makes any mise install
# that consumes the lockfile fail. Until the upstream plugin is fixed,
# patch the URLs after regenerating.
#
# Usage: scripts/mise_lock.sh
set -euo pipefail

cd "$(git rev-parse --show-toplevel)"

mise lock

# Re-add the `v` prefix to helm tarball URLs.
sed -i.bak -E 's|/helm-([0-9]+\.[0-9]+\.[0-9]+)-|/helm-v\1-|g' mise.lock
rm -f mise.lock.bak

# Sanity check: no broken URL pattern remains.
if grep -qE '/helm-[0-9]+\.[0-9]+\.[0-9]+-' mise.lock; then
	echo "ERROR: helm URL workaround did not apply cleanly" >&2
	exit 1
fi

echo "mise.lock regenerated and patched."
