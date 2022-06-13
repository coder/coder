#!/usr/bin/env bash

# This script builds multiple Go binaries for Coder with the given OS and
# architecture combinations.
#
# Usage: ./build_go_matrix.sh [--version 1.2.3+devel.abcdef] [--output dist/] [--slim] [--sign-darwin] [--archive] [--package-linux] os1:arch1,arch2 os2:arch1 os1:arch3
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
#
# If the --archive parameter is specified, all binaries will be archived using
# ./archive.sh. The --sign-darwin parameter will be carried through, and all
# archive files will be dropped in the output directory with the same name as
# the binary and the .zip (for windows and darwin) or .tar.gz extension.
#
# If the --package-linux parameter is specified, all linux binaries will be
# packaged using ./package.sh. Requires the nfpm binary.

set -euo pipefail
# shellcheck source=scripts/lib.sh
source "$(dirname "${BASH_SOURCE[0]}")/lib.sh"

version=""
output_path=""
slim=0
sign_darwin=0
archive=0
package_linux=0

args="$(getopt -o "" -l version:,output:,slim,sign-darwin,archive,package-linux -- "$@")"
eval set -- "$args"
while true; do
	case "$1" in
	--version)
		version="$2"
		shift 2
		;;
	--output)
		output_path="$2"
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
	--archive)
		archive=1
		shift
		;;
	--package-linux)
		package_linux=1
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
	mkdir -p dist
	output_path="$(realpath "dist/coder_{version}_{os}_{arch}")"
elif [[ "$output_path" == */ ]]; then
	output_path="${output_path}coder_{version}_{os}_{arch}"
elif [[ "$output_path" != *"{os}"* ]] || [[ "$output_path" != *"{arch}"* ]]; then
	# If the output path isn't a directory (ends with /) then it must have
	# template variables.
	error "Templated output path '$output_path' must contain {os} and {arch}"
fi

mkdir -p "$(dirname "$output_path")"
output_path="$(realpath "$output_path")"

# Remove the "v" prefix.
version="${version#v}"
if [[ "$version" == "" ]]; then
	version="$(execrelative ./version.sh)"
fi

# Parse the os:arch specs into an array.
specs=()
may_zip=0
may_tar=0
for spec in "$@"; do
	spec_os="$(echo "$spec" | cut -d ":" -f 1)"
	if [[ "$spec_os" == "" ]] || [[ "$spec_os" == *" "* ]]; then
		error "Could not parse matrix build spec '$spec': invalid OS '$spec_os'"
	fi

	# Determine which dependencies we need.
	if [[ "$spec_os" == "windows" ]] || [[ "$spec_os" == "darwin" ]]; then
		may_zip=1
	else
		may_tar=1
	fi

	# No quoting is important here.
	for spec_arch in $(echo "$spec" | cut -d ":" -f 2 | tr "," "\n"); do
		if [[ "$spec_arch" == "" ]] || [[ "$spec_os" == *" "* ]]; then
			error "Could not parse matrix build spec '$spec': invalid architecture '$spec_arch'"
		fi

		specs+=("$spec_os:$spec_arch")
	done
done

# Remove duplicate specs while maintaining the same order.
specs_str="${specs[*]}"
specs=()
for s in $(echo "$specs_str" | tr " " "\n" | awk '!a[$0]++'); do
	specs+=("$s")
done

# Check dependencies
if ! command -v go; then
	error "The 'go' binary is required."
fi
if [[ "$sign_darwin" == 1 ]]; then
	if ! command -v jq; then
		error "The 'jq' binary is required."
	fi
	if ! command -v codesign; then
		error "The 'codesign' binary is required."
	fi
	if ! command -v gon; then
		error "The 'gon' binary is required."
	fi
fi
if [[ "$archive" == 1 ]]; then
	if [[ "$may_zip" == 1 ]] && ! command -v zip; then
		error "The 'zip' binary is required."
	fi
	if [[ "$may_tar" == 1 ]] && ! command -v tar; then
		error "The 'zip' binary is required."
	fi
fi
if [[ "$package_linux" == 1 ]] && ! command -v nfpm; then
	error "The 'nfpm' binary is required."
fi

build_args=()
if [[ "$slim" == 1 ]]; then
	build_args+=(--slim)
fi
if [[ "$sign_darwin" == 1 ]]; then
	build_args+=(--sign-darwin)
fi

# Build each spec.
for spec in "${specs[@]}"; do
	spec_os="$(echo "$spec" | cut -d ":" -f 1)"
	spec_arch="$(echo "$spec" | cut -d ":" -f 2)"

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

	log "--- Building coder for $spec_os $spec_arch ($spec_output_binary)"
	execrelative ./build_go.sh \
		--version "$version" \
		--os "$spec_os" \
		--arch "$spec_arch" \
		--output "$spec_output_binary" \
		"${build_args[@]}"
	log
	log

	if [[ "$archive" == 1 ]]; then
		spec_archive_format="tar.gz"
		if [[ "$spec_os" == "windows" ]] || [[ "$spec_os" == "darwin" ]]; then
			spec_archive_format="zip"
		fi
		spec_output_archive="$spec_output.$spec_archive_format"

		archive_args=()
		if [[ "$sign_darwin" == 1 ]] && [[ "$spec_os" == "darwin" ]]; then
			archive_args+=(--sign-darwin)
		fi

		log "--- Creating archive for $spec_os $spec_arch ($spec_output_archive)"
		execrelative ./archive.sh \
			--format "$spec_archive_format" \
			--output "$spec_output_archive" \
			"${archive_args[@]}" \
			"$spec_output_binary"
		log
		log
	fi

	if [[ "$package_linux" == 1 ]] && [[ "$spec_os" == "linux" ]]; then
		execrelative ./package.sh \
			--arch "$spec_arch" \
			--version "$version" \
			"$spec_output_binary"
		log
	fi
done
