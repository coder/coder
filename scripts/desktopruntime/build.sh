#!/usr/bin/env bash

# This script builds the minimal Linux desktop runtime (Xvnc, xkbcomp and a
# trimmed xkeyboard-config) for embedding into the Coder workspace agent, and
# packs it into a reproducible zstd compressed tarball.
#
# Usage: ./build.sh --arch <amd64|arm64> --output <path.tar.zst> [--builder <name>]
#
# The archive is created with a fixed sort order, timestamp and ownership so
# that repeated builds of the same sources produce byte identical output.
#
# The build command can be replaced through CODER_DESKTOP_RUNTIME_BUILDX, which
# defaults to "docker buildx build". CI sets it to a Depot invocation, which
# takes the same --platform and --output flags and builds arm64 on native nodes
# instead of under emulation.
#
# The final size is printed on success.

set -euo pipefail
# shellcheck source=scripts/lib.sh
source "$(dirname "${BASH_SOURCE[0]}")/../lib.sh"

arch=""
output_path=""
builder=""

args="$(getopt -o "" -l arch:,output:,builder: -- "$@")"
eval set -- "$args"
while true; do
	case "$1" in
	--arch)
		arch="$2"
		if [[ "$arch" != "amd64" ]] && [[ "$arch" != "arm64" ]]; then
			error "Invalid --arch parameter '$arch', must be 'amd64' or 'arm64'"
		fi
		shift 2
		;;
	--output)
		# realpath fails if the dir doesn't exist.
		mkdir -p "$(dirname "$2")"
		output_path="$(realpath "$2")"
		shift 2
		;;
	--builder)
		builder="$2"
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

if [[ "$#" != 0 ]]; then
	error "Unexpected argument(s): $*"
fi
if [[ "$arch" == "" ]]; then
	error "--arch is required"
fi
if [[ "$output_path" == "" ]]; then
	error "--output is required"
fi
if [[ "$output_path" != *.tar.zst ]]; then
	error "--output must end in '.tar.zst'"
fi

# The build command is word split on purpose so that it can carry flags, for
# example "depot build --project wl5hnrrkns".
build_cmd_str="${CODER_DESKTOP_RUNTIME_BUILDX:-docker buildx build}"
read -r -a build_cmd <<<"$build_cmd_str"
if [[ "${#build_cmd[@]}" == 0 ]]; then
	error "CODER_DESKTOP_RUNTIME_BUILDX must not be empty"
fi
is_default_buildx=0
if [[ "$build_cmd_str" == "docker buildx build" ]]; then
	is_default_buildx=1
fi

dependencies "${build_cmd[0]}" tar zstd

cdroot
context="$PROJECT_ROOT/scripts/desktopruntime"

temp_dir="$(mktemp -d)"
# shellcheck disable=SC2317
cleanup() {
	rm -rf "$temp_dir"
}
trap cleanup EXIT

# supports_local_output reports whether the current buildx builder can export a
# filesystem. Rather than guessing from the driver name, it exports an empty
# image, which needs no image pull and takes well under a second.
supports_local_output() {
	mkdir -p "$temp_dir/probe/context"
	echo "FROM scratch" >"$temp_dir/probe/context/Dockerfile"
	docker buildx build \
		--output "type=local,dest=$temp_dir/probe/out" \
		"$temp_dir/probe/context" >/dev/null 2>&1
}

# An explicitly selected builder always wins, and BUILDX_BUILDER is left for
# buildx itself to pick up. Only when neither is set, and only for the default
# docker buildx command, does this fall back to a local docker-container
# builder, because older Docker daemons cannot export a filesystem.
builder_args=()
if [[ -n "$builder" ]]; then
	builder_args+=(--builder "$builder")
elif [[ -z "${BUILDX_BUILDER:-}" ]] && [[ "$is_default_buildx" == 1 ]] && ! supports_local_output; then
	log "The current buildx builder cannot export type=local, using a docker-container builder instead."
	if ! docker buildx inspect coder-desktopruntime >/dev/null 2>&1; then
		logrun docker buildx create --name coder-desktopruntime --driver docker-container >/dev/null
	fi
	builder_args+=(--builder coder-desktopruntime)
fi

logrun "${build_cmd[@]}" \
	"${builder_args[@]}" \
	--platform "linux/$arch" \
	--output "type=local,dest=$temp_dir/root" \
	"$context"

if [[ ! -x "$temp_dir/root/bin/Xvnc" ]]; then
	error "Build did not produce bin/Xvnc"
fi

logrun tar \
	--create \
	--file "$temp_dir/runtime.tar" \
	--directory "$temp_dir/root" \
	--sort=name \
	--mtime='2000-01-01 00:00:00Z' \
	--owner=0 \
	--group=0 \
	--numeric-owner \
	.

logrun zstd -19 -T0 -q --force -o "$output_path" "$temp_dir/runtime.tar"

unpacked_bytes="$(du -sb "$temp_dir/root" | cut -f1)"
bytes="$(stat -c %s "$output_path")"
log "Unpacked size: $unpacked_bytes bytes ($((unpacked_bytes / 1024 / 1024)) MiB)"
log "Compressed size: $bytes bytes ($((bytes / 1024 / 1024)) MiB)"
echo "$output_path"
