#!/usr/bin/env bash

set -euo pipefail

go install cmd/coder/main.go
echo "Coder CLI now installed at:"
echo "$(which coder)"
