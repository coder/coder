#!/usr/bin/env bash
# Install Harbor and harbor_coder into a venv from the hash-pinned lock.
#
# Every transitive dependency (including litellm) is pinned by version and
# sha256 in requirements.lock, and --require-hashes refuses anything that
# does not match, so a compromised release on PyPI cannot be pulled in.
#
# Usage: scripts/harbor-coder/install.sh [venv-dir]   (default: .harbor-coder/venv)
# Afterwards: export PATH="<venv-dir>/bin:$PATH"
set -euo pipefail

HERE="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
VENV="${1:-${HERE}/../../.harbor-coder/venv}"

uv venv -q --python 3.12 "${VENV}"
uv pip install -q --python "${VENV}/bin/python" --require-hashes -r "${HERE}/requirements.lock"
# The harness itself is local source, so it has no hash; install it without
# touching the already-verified dependency set.
uv pip install -q --python "${VENV}/bin/python" --no-deps -e "${HERE}"
echo "installed harbor into ${VENV} (litellm $("${VENV}/bin/python" -c 'import importlib.metadata as m; print(m.version("litellm"))'))"
