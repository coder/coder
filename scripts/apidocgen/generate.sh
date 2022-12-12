#!/usr/bin/env bash

# This script generates swagger description file and required Go docs files
# from the coderd API.

set -euo pipefail

SCRIPT_DIR=$(dirname "${BASH_SOURCE[0]}")

(
	cd "$SCRIPT_DIR/../.."

	go run github.com/swaggo/swag/cmd/swag@v1.8.6 init \
		--generalInfo="coderd.go" \
		--dir="./coderd,./codersdk" \
		--output="./coderd/apidocs" \
		--outputTypes="go,json" \
		--parseDependency=true
)
