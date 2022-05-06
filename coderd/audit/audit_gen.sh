#!/usr/bin/env bash

set -euo pipefail

cd "$(git rev-parse --show-toplevel)"
go run ./scripts/auditgen ./coderd/database "$@"
