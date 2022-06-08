#!/usr/bin/env bash

# This script creates an archive containing the given binary, as well as the
# README.md and LICENSE files.
#
# Usage: ./archive.sh --format tar.gz [--output path/to/output.tar.gz] [--sign-darwin] path/to/binary
#
# The --format parameter must be set, and must either be "zip" or "tar.gz".
#
# If the --output parameter is not set, the default output path is the binary
# path (minus any .exe suffix) plus the format extension ".zip" or ".tar.gz".
#
# If --sign-darwin is specified, the zip file is signed with the `codesign`
# utility and then notarized using the `gon` utility, which may take a while.
# $AC_APPLICATION_IDENTITY must be set and the signing certificate must be
# imported for this to work. Also, the input binary must already be signed with
# the `codesign` tool.

set -euo pipefail
# shellcheck source=lib.sh
source "$(dirname "${BASH_SOURCE[0]}")/lib.sh"

format=""
output_path=""
sign_darwin=0

args="$(getopt -o "" -l version:,output:,slim,sign-darwin -- "$@")"
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
    --sign-darwin)
        if [[ "${AC_APPLICATION_IDENTITY:-}" == "" ]]; then
            error "AC_APPLICATION_IDENTITY must be set when --sign-darwin is supplied"
        fi
        sign_darwin=1
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

if [[ "$#" != 1 ]]; then
    error "Exactly one argument must be provided to this script"
fi
if [[ ! -f "$1" ]]; then
    error "File '$1' does not exist or is not a regular file"
fi
input_file="$(realpath "$1")"

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
ln -s "$input_file" "$temp_dir/$output_path"
ln -s README.md "$temp_dir/"
ln -s LICENSE "$temp_dir/"

# Ensure parent output dir.
mkdir -p "$(dirname "$output_path")"

cd "$temp_dir"
if [[ "$format" == "zip" ]]; then
    zip "$output_path" ./*
else
    tar -czvf "$output_path" ./*
fi

rm -rf "$temp_dir"

if [[ "$sign_darwin" == 1 ]]; then
    echo "Notarizing binary..."
    execrelative ./sign_darwin.sh "$output_path"
fi
