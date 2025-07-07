#!/bin/bash

# Claude Code hook script for file formatting
# This script integrates with the centralized Makefile formatting targets
# and supports the Claude Code hooks system for automatic file formatting.

set -euo pipefail

# Read JSON input from stdin
input=$(cat)

# Extract the file path from the JSON input
# Expected format: {"tool_input": {"file_path": "/absolute/path/to/file"}} or {"tool_response": {"filePath": "/absolute/path/to/file"}}
file_path=$(echo "$input" | jq -r '.tool_input.file_path // .tool_response.filePath // empty')

if [[ -z "$file_path" ]]; then
	echo "Error: No file path provided in input" >&2
	exit 1
fi

# Check if file exists
if [[ ! -f "$file_path" ]]; then
	echo "Error: File does not exist: $file_path" >&2
	exit 1
fi

# Get the file extension to determine the appropriate formatter
file_ext="${file_path##*.}"

# Change to the project root directory (where the Makefile is located)
cd "$(dirname "$0")/../.."

# Call the appropriate Makefile target based on file extension
case "$file_ext" in
go)
	make fmt/go FILE="$file_path"
	echo "✓ Formatted Go file: $file_path"
	;;
js | jsx | ts | tsx)
	make fmt/ts FILE="$file_path"
	echo "✓ Formatted TypeScript/JavaScript file: $file_path"
	;;
tf | tfvars)
	make fmt/terraform FILE="$file_path"
	echo "✓ Formatted Terraform file: $file_path"
	;;
sh)
	make fmt/shfmt FILE="$file_path"
	echo "✓ Formatted shell script: $file_path"
	;;
md)
	make fmt/markdown FILE="$file_path"
	echo "✓ Formatted Markdown file: $file_path"
	;;
*)
	echo "No formatter available for file extension: $file_ext"
	exit 0
	;;
esac
