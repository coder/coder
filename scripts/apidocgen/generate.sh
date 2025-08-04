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

# Make sure that OpenAPI Generator is installed correctly.
pnpm exec -- openapi-generator-cli version

# Generate markdown documentation using OpenAPI Generator
log "Generating markdown documentation with OpenAPI Generator..."
OUTPUT_TMP_DIR=$(mktemp -d /tmp/coder-apidoc-openapi.XXXXXX)
pnpm exec -- openapi-generator-cli generate \
	-i "../../coderd/apidoc/swagger.json" \
	-g markdown \
	-o "${OUTPUT_TMP_DIR}"

# Combine all markdown files into a single file
log "Combining markdown files..."
node concat-markdown.js "${OUTPUT_TMP_DIR}" "${API_MD_TMP_FILE}"

# Clean up temporary directory
rm -rf "${OUTPUT_TMP_DIR}"
# Perform the postprocessing
go run postprocess/main.go -in-md-file-single "${API_MD_TMP_FILE}"
popd
