#!/usr/bin/env bash

# This script creates an archive containing the given binary renamed to
# `coder(.exe)?`, as well as the README.md and LICENSE files from the repo root.
#
# Usage: ./archive.sh --format tar.gz --os linux/darwin/windows [--output path/to/output.tar.gz] [--sign-darwin] [--agpl] path/to/binary
#
# The --format parameter must be set, and must either be "zip" or "tar.gz".
#
# If the --output parameter is not set, the default output path is the binary
# path (minus any .exe suffix) plus the format extension ".zip" or ".tar.gz".
#
# If --sign-darwin is specified, the zip file will be notarized using
# ./notarize_darwin.sh, which may take a while. Read that file for more details
# on the requirements.
#
# If the --agpl parameter is specified, only the AGPL license is included in the
# outputted archive.
#
# The absolute output path is printed on success.

set -euo pipefail
# shellcheck source=scripts/lib.sh
source "$(dirname "${BASH_SOURCE[0]}")/lib.sh"

format=""
output_path=""
sign_darwin="${CODER_SIGN_DARWIN:-0}"
os=""
agpl="${CODER_BUILD_AGPL:-0}"

args="$(getopt -o "" -l format:,output:,sign-darwin,os:,agpl -- "$@")"
eval set -- "$args"
while true; do
	case "$1" in
	--format)
		format="${2#.}"
		if [[ "$format" != "zip" ]] && [[ "$format" != "tar.gz" ]]; then
			error "Invalid --format parameter '$format', must be 'zip' or 'tar.gz'"
		fi
		shift 2
		;;
	--output)
		# realpath fails if the dir doesn't exist.
		mkdir -p "$(dirname "$2")"
		output_path="$(realpath "$2")"
		shift 2
		;;
	--os)
		os="$2"
		shift 2
		;;
	--sign-darwin)
		sign_darwin=1
		shift
		;;
	--agpl)
		agpl=1
		shift
		;;
	--)
		shift
		break
		;;
	*)
		error "Unrecognized option: $1"
		;;
	esac
done

if [[ "$format" == "" ]]; then
	error "--format is a required parameter"
fi

if [[ "$#" != 1 ]]; then
	error "Exactly one argument must be provided to this script, $# were supplied"
fi
if [[ ! -f "$1" ]]; then
	error "File '$1' does not exist or is not a regular file"
fi
input_file="$(realpath "$1")"

# Check dependencies
if [[ "$format" == "zip" ]]; then
	dependencies zip
fi
if [[ "$format" == "tar.gz" ]]; then
	dependencies tar
fi

sign_darwin="$([[ "$sign_darwin" == 1 ]] && [[ "$os" == "darwin" ]] && echo 1 || echo 0)"
if [[ "$sign_darwin" == 1 ]]; then
	dependencies rcodesign
	requiredenvs AC_APIKEY_ISSUER_ID AC_APIKEY_ID AC_APIKEY_FILE
fi

# Determine default output path.
if [[ "$output_path" == "" ]]; then
	output_path="${input_file%.exe}"
	output_path+=".$format"
fi

# Determine the filename of the binary inside the archive.
output_file="coder"
if [[ "$input_file" == *".exe" ]]; then
	output_file+=".exe"
fi

# Make temporary dir where all source files intended to be in the archive will
# be symlinked from.
cdroot
temp_dir="$(mktemp -d)"
ln -s "$input_file" "$temp_dir/$output_file"
ln -s "$(realpath README.md)" "$temp_dir/"
ln -s "$(realpath LICENSE)" "$temp_dir/"
if [[ "$agpl" == 0 ]]; then
	ln -s "$(realpath LICENSE.enterprise)" "$temp_dir/"
fi

# Ensure parent output dir and non-existent output file.
mkdir -p "$(dirname "$output_path")"
if [[ -e "$output_path" ]]; then
	rm "$output_path"
fi

cd "$temp_dir"
if [[ "$format" == "zip" ]]; then
	zip "$output_path" ./* 1>&2
else
	tar --dereference -czvf "$output_path" ./* 1>&2
fi

cdroot
rm -rf "$temp_dir"

if [[ "$sign_darwin" == 1 ]]; then
	log "Notarizing archive..."
	execrelative ./notarize_darwin.sh "$output_path"
fi

echo "$output_path"
