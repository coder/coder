#!/usr/bin/env bash

# This script generates swagger description file and required Go docs files
# from the coderd API.

set -euo pipefail
# shellcheck source=scripts/lib.sh
source "$(dirname "$(dirname "${BASH_SOURCE[0]}")")/lib.sh"

APIDOCGEN_DIR=$(dirname "${BASH_SOURCE[0]}")
API_MD_TMP_FILE=$(mktemp /tmp/coder-apidocgen.XXXXXX)

cleanup() {
	rm -f "${API_MD_TMP_FILE}"
}
trap cleanup EXIT

log "Use temporary file: ${API_MD_TMP_FILE}"

pushd "${PROJECT_ROOT}"
go run github.com/swaggo/swag/cmd/swag@v1.8.9 init \
        --generalInfo="coderd.go" \
        --dir="./coderd,./codersdk,./enterprise/coderd,./enterprise/wsproxy/wsproxysdk" \
        --output="./coderd/apidoc" \
        --outputTypes="go,json" \
        --parseDependency=true
popd

pushd "${APIDOCGEN_DIR}"

# Make sure that redocly is installed correctly.
pnpm exec -- redocly --version
# Generate basic markdown structure (redocly doesn't have direct markdown output like widdershins)
# Generate markdown documentation using Redocly
# First validate the OpenAPI spec
log "Validating OpenAPI spec with Redocly..."
pnpm exec -- redocly lint "../../coderd/apidoc/swagger.json" > /dev/null 2>&1 || true

# Generate HTML documentation with Redocly
log "Generating HTML documentation with Redocly..."
HTML_TMP_FILE=$(mktemp /tmp/coder-apidoc-html.XXXXXX)
pnpm exec -- redocly build-docs "../../coderd/apidoc/swagger.json" --output "${HTML_TMP_FILE}"

# Convert HTML to markdown
log "Converting HTML to markdown..."
node html-to-markdown.js "${HTML_TMP_FILE}" "${API_MD_TMP_FILE}"

# Clean up HTML file
rm -f "${HTML_TMP_FILE}"
# Perform the postprocessing
go run postprocess/main.go -in-md-file-single "${API_MD_TMP_FILE}"
popd
