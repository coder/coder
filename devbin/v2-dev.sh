#!/usr/bin/env bash

set -euo pipefail

PROJECT_ROOT="$(git rev-parse --show-toplevel)"
cd "$(PROJECT_ROOT)"

# Do initial build
make build


(trap 'kill 0' SIGINT; ./build/main & yarn dev)

