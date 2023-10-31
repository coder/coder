#!/usr/bin/env bash
set -euo pipefail

workspace=${1:-}
port=${2:-3000}

if [[ -z "${workspace}" ]]; then
  echo "Usage: $0 <workspace> [port]"
  exit 1
fi

# Go to site.
cd "$(dirname "$0")/../.."

echo "Running \"pnpm install\" to ensure local and remote are up-to-date..."
pnpm install

echo "Running \"playwright install\" for browser binaries..."
pnpm exec playwright install

playwright_out="$(mktemp -t playwright_server_out.XXXXXX)"

rm "$playwright_out"
mkfifo "$playwright_out"
exec 3<>"$playwright_out"

echo "Starting Playwright server..."
exec pnpm --silent exec node ./e2e/server/server.mjs 1>&3 &
playwright_pid=$!

trap '
kill $playwright_pid
exec 3>&-
rm "$playwright_out"
' EXIT

echo "Waiting for Playwright to start..."
read -r ws_endpoint <&3
if [[ ${ws_endpoint} != ws://* ]]; then
  echo "Playwright failed to start."
  echo "${ws_endpoint}"
  cat "$playwright_out"
  exit 1
fi
echo "Playwright started at ${ws_endpoint}"

ws_port=${ws_endpoint##*:}
ws_port=${ws_port%/*}

echo "Starting SSH tunnel, run test via \"pnpm run playwright:test\"..."

ssh -t -R "${ws_port}:127.0.0.1:${ws_port}" -L "${port}:127.0.0.1:${port}" coder."${workspace}" "export CODER_E2E_PORT='${port}'; export CODER_E2E_WS_ENDPOINT='${ws_endpoint}'; [[ -d site ]] && cd site; exec \"\$(grep \"\${USER}\": /etc/passwd | cut -d: -f7)\" -l"
