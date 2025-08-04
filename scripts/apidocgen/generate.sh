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
# Create basic markdown structure that the postprocessor can work with
# The postprocessor expects sections separated by <!-- APIDOCGEN: BEGIN SECTION -->
echo "<!-- APIDOCGEN: BEGIN SECTION -->" > "${API_MD_TMP_FILE}"
echo "# General" >> "${API_MD_TMP_FILE}"
echo "" >> "${API_MD_TMP_FILE}"
echo "This documentation is generated from the OpenAPI specification using Redocly CLI." >> "${API_MD_TMP_FILE}"
echo "" >> "${API_MD_TMP_FILE}"
echo "The Coder API is organized around REST. Our API has predictable resource-oriented URLs, accepts form-encoded request bodies, returns JSON-encoded responses, and uses standard HTTP response codes, authentication, and verbs." >> "${API_MD_TMP_FILE}"
echo "" >> "${API_MD_TMP_FILE}"
# Validate the OpenAPI spec with redocly (suppress output to avoid cluttering)
pnpm exec -- redocly lint "../../coderd/apidoc/swagger.json" > /dev/null 2>&1 || true
# Perform the postprocessing
go run postprocess/main.go -in-md-file-single "${API_MD_TMP_FILE}"
popd
