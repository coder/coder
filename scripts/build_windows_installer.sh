#!/usr/bin/env bash

# This script packages a Windows installer for Coder containing the given
# binary.
#
# Usage: ./build_windows_installer.sh --output "path/to/installer.exe" [--version 1.2.3] [--agpl] path/to/binary.exe
#
# Only amd64 binaries are supported.
#
# If no version is specified, defaults to the version from ./version.sh.
#
# If the --agpl parameter is specified, only the AGPL license is included in the
# installer.

set -euo pipefail
# shellcheck source=scripts/lib.sh
source "$(dirname "${BASH_SOURCE[0]}")/lib.sh"

agpl="${CODER_BUILD_AGPL:-0}"
output_path=""
version=""

args="$(getopt -o "" -l agpl,output:,version: -- "$@")"
eval set -- "$args"
while true; do
	case "$1" in
	--agpl)
		agpl=1
		shift
		;;
	--output)
		mkdir -p "$(dirname "$2")"
		output_path="$(realpath "$2")"
		shift 2
		;;
	--version)
		version="$2"
		shift 2
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

if [[ "$output_path" == "" ]]; then
	error "--output is a required parameter"
fi

if [[ "$#" != 1 ]]; then
	error "Exactly one argument must be provided to this script, $# were supplied"
fi
if [[ ! -f "$1" ]]; then
	error "File '$1' does not exist or is not a regular file"
fi
input_file="$(realpath "$1")"

version="${version#v}"
if [[ "$version" == "" ]]; then
	version="$(execrelative ./version.sh)"
fi

# Remove the "v" prefix and ensure the version is in the format X.X.X.X for
# makensis.
nsis_version="${version//-*/}"
# Each component of a version must be a 16 bit integer, so we can't store any
# useful information like build date or commit SHA in the 4th component.
nsis_version+=".0"

# Check dependencies
dependencies makensis

# Make a temporary dir where all source files intended to be in the installer
# will be hardlinked/copied to.
cdroot
temp_dir="$(TMPDIR="$(dirname "$input_file")" mktemp -d)"
mkdir -p "$temp_dir/bin"
ln "$input_file" "$temp_dir/bin/coder.exe"
cp "$(realpath scripts/win-installer/installer.nsi)" "$temp_dir/installer.nsi"
cp "$(realpath scripts/win-installer/path.nsh)" "$temp_dir/path.nsh"
cp "$(realpath scripts/win-installer/coder.ico)" "$temp_dir/coder.ico"
cp "$(realpath scripts/win-installer/banner.bmp)" "$temp_dir/banner.bmp"

# Craft a license document by combining the AGPL license and optionally the
# enterprise license.
license_path="$temp_dir/license.txt"

if [[ "$agpl" == 0 ]]; then
	cat <<-EOF >"$license_path"
		This distribution of Coder includes some enterprise-licensed code which is not
		licensed under the AGPL license:

		$(sed 's/^/  /' "$(realpath LICENSE.enterprise)")



		The non-enterprise code in this distribution is licensed under the AGPL license:

		$(sed 's/^/  /' "$(realpath LICENSE)")
	EOF
else
	cat <<-EOF >"$license_path"
		This distribution of Coder is free software and is licensed under the AGPL
		license:

		$(sed 's/^/  /' "$(realpath LICENSE)")
	EOF
fi

# Run makensis to build the installer.
pushd "$temp_dir"
makensis \
	-V4 \
	-DCODER_VERSION="$version" \
	-DCODER_NSIS_VERSION="$nsis_version" \
	-DCODER_YEAR="$(date +%Y)" \
	installer.nsi
popd

# Copy the installer to the output path.
cp "$temp_dir/installer.exe" "$output_path"

rm -rf "$temp_dir"
