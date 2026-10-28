#!/usr/bin/env bash
# Build coderd from a source tree, serve it with embedded Postgres, and
# bootstrap it for a Harbor eval. Writes CODER_URL/CODER_SESSION_TOKEN/
# HARBOR_CODER_TEMPLATE/HARBOR_CODER_BIN to the env file given as $1.
#
# Usage: serve-coderd.sh <env-file> [port] [source-dir]
#
# Requires HARBOR_CODER_UPSTREAM_TOKEN (a Coder API key for the AI gateway).
# State (binary, Postgres, logs) lives under .harbor-coder/ next to this
# script's checkout regardless of which source tree is built.
set -euo pipefail

ENV_FILE="${1:?env file path required}"
PORT="${2:-3000}"
HERE="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT="$(cd "${HERE}/../.." && pwd)"
SRC="$(cd "${3:-${ROOT}}" && pwd)"
STATE="${ROOT}/.harbor-coder"
mkdir -p "${STATE}"

# Static so it runs inside any task image regardless of that image's glibc.
# Stamp the commit explicitly: plain `go build` only picks it up from .git,
# which a detached worktree lacks, and compare.py reports it per arm.
if [[ -z "${HARBOR_CODER_SKIP_BUILD:-}" ]]; then
	sha="$(git -C "${SRC}" rev-parse --short HEAD)"
	(cd "${SRC}" && CGO_ENABLED=0 go build \
		-ldflags "-X 'github.com/coder/coder/v2/buildinfo.tag=0.0.0-devel+${sha}'" \
		-o "${STATE}/coder" ./cmd/coder)
fi

URL="http://localhost:${PORT}"
# Otherwise the new server fails to bind inside nohup and bootstrap happily
# talks to the old one, so a local A/B would evaluate the same build twice.
if curl -fs -o /dev/null --max-time 2 "${URL}/healthz"; then
	echo "something is already serving ${URL}; stop it (see ${STATE}/coderd.pid) or pass another port" >&2
	exit 1
fi
# A fresh database per build: the provider key and default model are
# re-applied by bootstrap, and nothing from the previous arm should leak.
rm -rf "${STATE}/config" "${STATE}/first-user-password"
nohup "${STATE}/coder" server --http-address "0.0.0.0:${PORT}" --access-url "${URL}" \
	--telemetry=false --cache-dir "${STATE}/cache" --global-config "${STATE}/config" \
	--provisioner-daemons 4 >"${STATE}/coderd.log" 2>&1 &
echo $! >"${STATE}/coderd.pid"

export PATH="${STATE}:${PATH}"
# python resolves to the hash-verified venv from install.sh when it is on PATH.
python -m harbor_coder.bootstrap \
	--url "${URL}" --template-dir "${HERE}/template" \
	--state-dir "${STATE}" --model "${HARBOR_CODER_MODEL:-claude-haiku-4-5}" >"${ENV_FILE}"
chmod 600 "${ENV_FILE}"
echo "HARBOR_CODER_BIN=${STATE}/coder" >>"${ENV_FILE}"
