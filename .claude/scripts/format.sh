#!/bin/bash

# Claude Code hook script for file formatting
# This script integrates with the centralized Makefile formatting targets
# and supports the Claude Code hooks system for automatic file formatting.

set -euo pipefail

# A variable to memoize the command for canonicalizing paths.
_CANONICALIZE_CMD=""

# canonicalize_path resolves a path to its absolute, canonical form.
# It tries 'realpath' and 'readlink -f' in order.
# The chosen command is memoized to avoid repeated checks.
# If none of these are available, it returns an empty string.
canonicalize_path() {
	local path_to_resolve="$1"

	# If we haven't determined a command yet, find one.
	if [[ -z "$_CANONICALIZE_CMD" ]]; then
		if command -v realpath >/dev/null 2>&1; then
			_CANONICALIZE_CMD="realpath"
		elif command -v readlink >/dev/null 2>&1 && readlink -f . >/dev/null 2>&1; then
			_CANONICALIZE_CMD="readlink"
		else
			# No command found, so we can't resolve.
			# We set a "none" value to prevent re-checking.
			_CANONICALIZE_CMD="none"
		fi
	fi

	# Now, execute the command.
	case "$_CANONICALIZE_CMD" in
	realpath)
		realpath "$path_to_resolve" 2>/dev/null
		;;
	readlink)
		readlink -f "$path_to_resolve" 2>/dev/null
		;;
	*)
		# This handles the "none" case or any unexpected error.
		echo ""
		;;
	esac
}

# Read JSON input from stdin
input=$(cat)

# Extract the file path from the JSON input
# Expected format: {"tool_input": {"file_path": "/absolute/path/to/file"}} or {"tool_response": {"filePath": "/absolute/path/to/file"}}
file_path=$(echo "$input" | jq -r '.tool_input.file_path // .tool_response.filePath // empty')

# Secure path canonicalization to prevent path traversal attacks
# Resolve repo root to an absolute, canonical path.
repo_root_raw="$(cd "$(dirname "$0")/../.." && pwd)"
repo_root="$(canonicalize_path "$repo_root_raw")"
if [[ -z "$repo_root" ]]; then
	# Fallback if canonicalization fails
	repo_root="$repo_root_raw"
fi

# Resolve the input path to an absolute path
if [[ "$file_path" = /* ]]; then
	# Already absolute
	abs_file_path="$file_path"
else
	# Make relative paths absolute from repo root
	abs_file_path="$repo_root/$file_path"
fi

# Canonicalize the path (resolve symlinks and ".." segments)
canonical_file_path="$(canonicalize_path "$abs_file_path")"

# Check if canonicalization failed or if the resolved path is outside the repo
if [[ -z "$canonical_file_path" ]] || { [[ "$canonical_file_path" != "$repo_root" ]] && [[ "$canonical_file_path" != "$repo_root"/* ]]; }; then
	echo "Error: File path is outside repository or invalid: $file_path" >&2
	exit 1
fi

# Handle the case where the file path is the repository root itself.
if [[ "$canonical_file_path" == "$repo_root" ]]; then
	echo "Warning: Formatting the repository root is not a supported operation. Skipping." >&2
	exit 0
fi

# Convert back to relative path from repo root for consistency
file_path="${canonical_file_path#"$repo_root"/}"

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
