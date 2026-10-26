#!/usr/bin/env bash
# Build coderd from this checkout, serve it with embedded Postgres, and
# bootstrap it for a Harbor eval. Writes CODER_URL/CODER_SESSION_TOKEN/
# HARBOR_CODER_TEMPLATE to the env file given as $1.
#
# Requires HARBOR_CODER_UPSTREAM_TOKEN (a Coder API key for the AI gateway).
set -euo pipefail

ENV_FILE="${1:?env file path required}"
PORT="${2:-3000}"
ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
STATE="${ROOT}/.harbor-coder"
mkdir -p "${STATE}"

# Static so it runs inside any task image regardless of that image's glibc.
[[ -n "${HARBOR_CODER_SKIP_BUILD:-}" ]] || (cd "${ROOT}" && CGO_ENABLED=0 go build -o "${STATE}/coder" ./cmd/coder)

URL="http://localhost:${PORT}"
# Otherwise the new server fails to bind inside nohup and bootstrap happily
# talks to the old one, so a local A/B would evaluate the same build twice.
if curl -fs -o /dev/null --max-time 2 "${URL}/healthz"; then
	echo "something is already serving ${URL}; stop it (see ${STATE}/coderd.pid) or pass another port" >&2
	exit 1
fi
nohup "${STATE}/coder" server --http-address "0.0.0.0:${PORT}" --access-url "${URL}" \
	--telemetry=false --cache-dir "${STATE}/cache" --global-config "${STATE}/config" \
	--provisioner-daemons 4 >"${STATE}/coderd.log" 2>&1 &
echo $! >"${STATE}/coderd.pid"

export PATH="${STATE}:${PATH}"
uv tool run --from harbor python -m harbor_coder.bootstrap \
	--url "${URL}" --template-dir "${ROOT}/scripts/harbor-coder/template" \
	--state-dir "${STATE}" --model "${HARBOR_CODER_MODEL:-claude-haiku-4-5}" >"${ENV_FILE}"
chmod 600 "${ENV_FILE}"
echo "HARBOR_CODER_BIN=${STATE}/coder" >>"${ENV_FILE}"
