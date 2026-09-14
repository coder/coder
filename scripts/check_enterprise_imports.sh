#!/usr/bin/env bash

# This file checks all our AGPL licensed source files to be sure they don't
# import any enterprise licensed packages (the inverse is fine).

set -euo pipefail
# shellcheck source=scripts/lib.sh
source "$(dirname "${BASH_SOURCE[0]}")/lib.sh"
cdroot

set +e
find . -regex ".*\.go" |
	grep -v "./enterprise" |
	grep -v ./scripts/auditdocgen/ --include="*.go" |
	grep -v ./scripts/clidocgen/ --include="*.go" |
	grep -v ./scripts/rules.go |
	xargs grep -n "github.com/coder/coder/v2/enterprise"
# reverse the exit code because we want this script to fail if grep finds anything.
status=$?
set -e
if [ $status -eq 0 ]; then
	error "AGPL code cannot import enterprise!"
fi
log "AGPL imports OK"
