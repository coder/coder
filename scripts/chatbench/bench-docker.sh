#!/usr/bin/env bash
# Runs bench.mjs inside the official Playwright image so WebKit has its
# system libraries (the host OS is newer than Playwright supports).
# Uses the host network to reach the benchmark stack on 127.0.0.1:18080.
#
#   scripts/chatbench/bench-docker.sh --browser=webkit --scenario=compare --label=before
set -euo pipefail
REPO=$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)
ROOT=${CHATBENCH_ROOT:-${XDG_CACHE_HOME:-$HOME/.cache}/chatbench}
IMAGE=mcr.microsoft.com/playwright:v1.55.1-noble
exec docker run --rm --network host --ipc=host --init \
	--user "$(id -u):$(id -g)" \
	-e HOME=/tmp \
	-e CHATBENCH_ROOT="$ROOT" \
	-v "$REPO:$REPO" \
	-v "$ROOT:$ROOT" \
	-w "$REPO" \
	"$IMAGE" node scripts/chatbench/bench.mjs "$@"
