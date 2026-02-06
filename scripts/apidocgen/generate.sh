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
# Use our custom wrapper instead of "go tool swag init" to enable
# Strict mode, which turns duplicate-route warnings into hard errors.
# The upstream swag CLI does not expose a --strict flag.
go run "${APIDOCGEN_DIR}/swaginit/main.go"
popd

pushd "${APIDOCGEN_DIR}"

# Make sure that widdershins is installed correctly.
pnpm exec -- widdershins --version
# Render the Markdown file.
pnpm exec -- widdershins \
	--user_templates "./markdown-template" \
	--search false \
	--omitHeader true \
	--language_tabs "shell:curl" \
	--summary "../../coderd/apidoc/swagger.json" \
	--outfile "${API_MD_TMP_FILE}"
# Perform the postprocessing
go run postprocess/main.go -in-md-file-single "${API_MD_TMP_FILE}"
popd
