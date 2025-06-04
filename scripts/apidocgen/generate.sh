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
