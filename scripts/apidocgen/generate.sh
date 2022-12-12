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

(
	cd "$SCRIPT_DIR"
	npm ci

	# Make sure that widdershins is installed correctly.
	node ./node_modules/widdershins/widdershins.js --version

	# Render the Markdown file.
	node ./node_modules/widdershins/widdershins.js \
		--user_templates "./markdown-template" \
		--search false \
		--omitHeader true \
		--language_tabs "shell:curl" \
		--summary "../../coderd/apidocs/swagger.json" \
		--outfile "../../docs/api.md"
)
