#!/usr/bin/env bash

# This script creates a Helm package for the given version. It will output a
# .tgz file at the specified path, and may optionally push it to the Coder OSS
# repo.
#
# ./helm.sh [--version 1.2.3] [--output path/to/coder.tgz] [--push]
#
# If no version is specified, defaults to the version from ./version.sh.
#
# If no output path is specified, defaults to
# "$repo_root/dist/coder_helm_$version.tgz".
#
# If the --push parameter is specified, the resulting artifact will be published
# to the Coder OSS repo. This requires `gsutil` to be installed and configured.

set -euo pipefail
# shellcheck source=scripts/lib.sh
source "$(dirname "${BASH_SOURCE[0]}")/lib.sh"

version=""
output_path=""
push=0

args="$(getopt -o "" -l version:,output:,push -- "$@")"
eval set -- "$args"
while true; do
	case "$1" in
	--version)
		version="$2"
		shift 2
		;;
	--output)
		output_path="$(realpath "$2")"
		shift 2
		;;
	--push)
		push="1"
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

# Remove the "v" prefix.
version="${version#v}"
if [[ "$version" == "" ]]; then
	version="$(execrelative ./version.sh)"
fi

if [[ "$output_path" == "" ]]; then
	cdroot
	mkdir -p dist
	output_path="$(realpath "dist/coder_helm_$version.tgz")"
fi

# Check dependencies
dependencies helm

# Make a destination temporary directory, as you cannot fully control the output
# path of `helm package` except for the directory name :/
cdroot
temp_dir="$(mktemp -d)"

cdroot
cd ./helm
log "--- Packaging helm chart for version $version ($output_path)"
helm package \
	--version "$version" \
	--app-version "$version" \
	--destination "$temp_dir" \
	. 1>&2

log "Moving helm chart to $output_path"
cp "$temp_dir"/*.tgz "$output_path"
rm -rf "$temp_dir"

if [[ "$push" == 1 ]]; then
	log "--- Publishing helm chart..."
	# TODO: figure out how/where we want to publish the helm chart
fi
