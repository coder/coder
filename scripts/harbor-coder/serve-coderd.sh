#!/usr/bin/env bash
# Build coderd from the current checkout and serve it in the background for a
# Harbor eval run. Prints nothing secret; the bootstrap step emits credentials
# to the env file given as $1.
#
# Usage: scripts/harbor-coder/serve-coderd.sh <env-file> [port]
#
# Requires ANTHROPIC_API_KEY in the environment for the bootstrap step.
set -euo pipefail

ENV_FILE="${1:?env file path required}"
PORT="${2:-3000}"
ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
STATE_DIR="${HARBOR_CODER_STATE_DIR:-${ROOT}/.harbor-coder}"
mkdir -p "${STATE_DIR}"

BIN="${STATE_DIR}/coder"
if [[ -z "${HARBOR_CODER_SKIP_BUILD:-}" ]]; then
  echo "building coder (slim) -> ${BIN}"
  # The slim build has no embedded frontend; the eval only needs the API, the
  # built-in provisioner, and the agent binary (the CLI itself) for the image.
  # Static so it runs inside any task image regardless of its glibc version.
  (cd "${ROOT}" && CGO_ENABLED=0 go build -o "${BIN}" ./cmd/coder)
fi

ACCESS_URL="http://localhost:${PORT}"
LOG="${STATE_DIR}/coderd.log"
echo "starting coderd on ${ACCESS_URL} (log: ${LOG})"
nohup "${BIN}" server \
  --http-address "0.0.0.0:${PORT}" \
  --access-url "${ACCESS_URL}" \
  --telemetry=false \
  --cache-dir "${STATE_DIR}/cache" \
  --provisioner-daemons 4 \
  --global-config "${STATE_DIR}/config" \
  >"${LOG}" 2>&1 &
echo $! > "${STATE_DIR}/coderd.pid"

for _ in $(seq 1 90); do
  if curl -fsS "${ACCESS_URL}/healthz" >/dev/null 2>&1; then
    break
  fi
  sleep 2
done
curl -fsS "${ACCESS_URL}/healthz" >/dev/null || { echo "coderd did not become healthy"; tail -50 "${LOG}"; exit 1; }

export PATH="${STATE_DIR}:${PATH}"
export HARBOR_CODER_STATE_DIR="${STATE_DIR}"
# harbor_coder is installed alongside Harbor (uv tool install --with-editable),
# so run the bootstrap with Harbor's interpreter.
PY=(uv tool run --from harbor python)
command -v uv >/dev/null || PY=(python3)
"${PY[@]}" -m harbor_coder.bootstrap \
  --url "${ACCESS_URL}" \
  --template-dir "${ROOT}/scripts/harbor-coder/template" \
  --model "${HARBOR_CODER_MODEL:-claude-haiku-4-5}" \
  > "${ENV_FILE}"
chmod 600 "${ENV_FILE}"
echo "HARBOR_CODER_BIN=${BIN}" >> "${ENV_FILE}"
echo "bootstrap complete; env written to ${ENV_FILE}"
