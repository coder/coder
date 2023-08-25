#!/usr/bin/env bash

# This script creates a Helm package for the given version. It will output a
# .tgz file at the specified path, and may optionally push it to the Coder OSS
# repo.
#
# ./helm.sh [--version 1.2.3] [--chart coder|provisioner] [--output path/to/coder.tgz]
#
# If no version is specified, defaults to the version from ./version.sh.
#
# If no chart is specified, defaults to 'coder'
#
# If no output path is specified, defaults to
# "$repo_root/build/$chart_helm_$version.tgz".

set -euo pipefail
# shellcheck source=scripts/lib.sh
source "$(dirname "${BASH_SOURCE[0]}")/lib.sh"

version=""
output_path=""
chart=""

args="$(getopt -o "" -l version:,chart:,output:,push -- "$@")"
eval set -- "$args"
while true; do
	case "$1" in
	--version)
		version="$2"
		shift 2
		;;
	--chart)
		chart="$2"
		shift 2
		;;
	--output)
		output_path="$(realpath "$2")"
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

# Remove the "v" prefix.
version="${version#v}"
if [[ "$version" == "" ]]; then
	version="$(execrelative ./version.sh)"
fi

if [[ "$chart" == "" ]]; then
	chart="coder"
fi
if ! [[ "$chart" =~ ^(coder|provisioner)$ ]]; then
	error "--chart value must be one of (coder, provisioner)"
fi

if [[ "$output_path" == "" ]]; then
	cdroot
	mkdir -p build
	output_path="$(realpath "build/${chart}_helm_${version}.tgz")"
fi

# Check dependencies
dependencies helm

# Make a destination temporary directory, as you cannot fully control the output
# path of `helm package` except for the directory name :/
cdroot
temp_dir="$(mktemp -d)"

cdroot
cd "./helm/${chart}"
log "--- Updating dependencies"
helm dependency update .
log "--- Packaging helm chart $chart for version $version ($output_path)"
helm package \
	--version "$version" \
	--app-version "$version" \
	--destination "$temp_dir" \
	. 1>&2

log "Moving helm chart to $output_path"
cp "$temp_dir"/*.tgz "$output_path"
rm -rf "$temp_dir"
