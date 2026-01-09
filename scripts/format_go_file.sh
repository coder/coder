#!/usr/bin/env bash

# This script formats Go file(s) with our project-specific configuration.

# Usage: format_go_file <file>...

set -euo pipefail

if [[ "$#" -lt 1 ]]; then
	echo "Usage: $0 <file>..."
	exit 1
fi

go tool mvdan.cc/gofumpt -w -l "${@}"
go tool github.com/daixiang0/gci write -s standard -s default -s "Prefix(github.com/coder,cdr.dev/)" "${@}"
