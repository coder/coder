#!/usr/bin/env bash

# This script generates swagger description file and required Go docs files
# from the coderd API.

set -euo pipefail

SCRIPT_DIR=$(dirname "${BASH_SOURCE[0]}")
API_MD_TMP_FILE=$(mktemp /tmp/coder-apidocgen.XXXXXX)

cleanup() {
	rm -f "${API_MD_TMP_FILE}"
}
trap cleanup EXIT

echo "Use temporary file: ${API_MD_TMP_FILE}"

(
	cd "$SCRIPT_DIR/../.."

	go run github.com/swaggo/swag/cmd/swag@v1.8.6 init \
		--generalInfo="coderd.go" \
		--dir="./coderd,./codersdk" \
		--output="./coderd/apidocs" \
		--outputTypes="go,json" \
		--parseDependency=true
) || exit $?

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
		--outfile "${API_MD_TMP_FILE}"
) || exit $?

(
	cd "$SCRIPT_DIR"
	go run postprocess/main.go -in-md-file-single "${API_MD_TMP_FILE}"
) || exit $?
