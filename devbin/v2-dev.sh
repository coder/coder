#!/usr/bin/env bash

set -euo pipefail

PROJECT_ROOT="$(git rev-parse --show-toplevel)"
cd "$(PROJECT_ROOT)"

# Do initial build - a dev build for coderd that doesn't require front-end assets
make dev/go/coderd

(trap 'kill 0' SIGINT; ./build/main & yarn dev)