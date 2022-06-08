#!/usr/bin/env bash

# This script builds multiple Go binaries for Coder with the given OS and
# architecture combinations.
#
# Usage: ./build_go_matrix.sh [--version 1.2.3+devel.abcdef] [--output dist/] [--slim] [--sign-darwin] os1:arch1,arch2 os2:arch1 os1:arch3
#
# If no OS:arch combinations are provided, nothing will happen and no error will
# be returned. Slim builds are disabled by default. If no version is specified,
# defaults to the version from ./version.sh
#
# The --output parameter must be a directory with a trailing slash where all
# files will be dropped with the default name scheme
# `coder_$version_$os_$arch(.exe)?`, or must contain the `{os}` and `{arch}`
# template variables. You may also use `{version}`. Note that for windows builds
# the `.exe` suffix will be appended automatically.
#
# Unless overridden via --output, the built binary will be dropped in
# "$repo_root/dist/coder_$version_$os_$arch" (with a ".exe" suffix for windows
# builds).
#
# If the --sign-darwin parameter is specified, all darwin binaries will be
# signed using the `codesign` utility. $AC_APPLICATION_IDENTITY must be set and
# the signing certificate must be imported for this to work.

set -euo pipefail
# shellcheck source=lib.sh
source "$(dirname "${BASH_SOURCE[0]}")/lib.sh"

version=""
output_path=""
slim=0
sign_darwin=0

args="$(getopt -o "" -l version:,output:,slim,sign-darwin -- "$@")"
eval set -- "$args"
while true; do
    case "$1" in
    --version)
        version="$2"
        shift 2
        ;;
    --output)
        # realpath fails if the dir doesn't exist.
        mkdir -p "$(dirname "$2")"
        output_path="$(realpath "$2")"
        shift 2
        ;;
    --slim)
        slim=1
        shift
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

# Verify the output path template.
if [[ "$output_path" == "" ]]; then
    # Input paths are relative, so we don't cdroot at the top, but for this case
    # we want it to be relative to the root.
    cdroot
    output_path="dist/coder_{version}_{os}_{arch}"
elif [[ "$output_path" == */ ]]; then
    output_path="${output_path}coder_{version_{os}_{arch}"
else
    # Verify that it contains {os} and {arch} at least.
    if [[ "$output_path" != *"{os}"* ]] || [[ "$output_path" != *"{arch}"* ]]; then
        error "Templated output path '$output_path' must contain {os} and {arch}"
    fi
fi

# Remove the "v" prefix.
version="${version#v}"
if [[ "$version" == "" ]]; then
    version="$(execrelative ./version.sh)"
fi

# Parse the os:arch specs into an array.
specs=()
for spec in "$@"; do
    spec_os="$(echo "$spec" | cut -d ":" -f 1)"
    if [[ "$spec_os" == "" ]] || [[ "$spec_os" == *" "* ]]; then
        error "Could not parse matrix build spec '$spec': invalid OS '$spec_os'"
    fi

    # No quoting is important here.
    for spec_arch in $(echo "$spec" | cut -d ":" -f 2 | tr "," "\n"); do
        if [[ "$spec_arch" == "" ]] || [[ "$spec_os" == *" "* ]]; then
            error "Could not parse matrix build spec '$spec': invalid architecture '$spec_arch'"
        fi

        specs+=("$spec_os $spec_arch")
    done
done

build_args=()
if [[ "$slim" == 1 ]]; then
    build_args+=(--slim)
fi
if [[ "$sign_darwin" == 1 ]]; then
    build_args+=(--sign-darwin)
fi

# Build each spec.
for spec in "${specs[@]}"; do
    spec_os="$(echo "$spec" | cut -d " " -f 1)"
    spec_arch="$(echo "$spec" | cut -d " " -f 2)"

    # Craft output path from the template.
    spec_output="$output_path"
    spec_output="${spec_output//\{os\}/"$spec_os"}"
    spec_output="${spec_output//\{arch\}/"$spec_arch"}"
    spec_output="${spec_output//\{version\}/"$version"}"

    spec_output_binary="$spec_output"
    if [[ "$spec_os" == "windows" ]]; then
        spec_output_binary+=".exe"
    fi

    # Ensure parent dir.
    mkdir -p "$(dirname "$spec_output")"

    echo "--- Building coder for $spec_os $spec_arch ($spec_output_binary)"
    execrelative ./build_go.sh \
        --version "$version" \
        --os "$spec_os" \
        --arch "$spec_arch" \
        --output "$spec_output_binary" \
        "${build_args[@]}"
    echo
    echo
done
