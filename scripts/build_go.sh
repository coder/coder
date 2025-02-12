#!/usr/bin/env bash

# This script builds a single Go binary of Coder with the given parameters.
#
# Usage: ./build_go.sh [--version 1.2.3-devel+abcdef] [--os linux] [--arch amd64] [--output path/to/output] [--slim] [--agpl] [--boringcrypto] [--dylib]
#
# Defaults to linux:amd64 with slim disabled, but can be controlled with GOOS,
# GOARCH and CODER_SLIM_BUILD=1. If no version is specified, defaults to the
# version from ./version.sh.
#
# GOARM can be controlled by suffixing any arm architecture (i.e. arm or arm64)
# with "vX" (e.g. "v7", "v8").
#
# Unless overridden via --output, the built binary will be dropped in
# "$repo_root/build/coder_$version_$os_$arch" (with a ".exe" suffix for windows
# builds) and the absolute path to the binary will be printed to stdout on
# completion.
#
# If the --sign-darwin parameter is specified and the OS is darwin, the output
# binary will be signed using ./sign_darwin.sh. Read that file for more details
# on the requirements.
#
# If the --agpl parameter is specified, builds only the AGPL-licensed code (no
# Coder enterprise features).
#
# If the --boringcrypto parameter is specified, builds use boringcrypto instead of
# the standard go crypto libraries.
#
# If the --dylib parameter is specified, the Coder Desktop `.dylib` is built
# instead of the standard binary. This is only supported on macOS arm64 & amd64.

set -euo pipefail
# shellcheck source=scripts/lib.sh
source "$(dirname "${BASH_SOURCE[0]}")/lib.sh"

version=""
os="${GOOS:-linux}"
arch="${GOARCH:-amd64}"
slim="${CODER_SLIM_BUILD:-0}"
sign_darwin="${CODER_SIGN_DARWIN:-0}"
sign_windows="${CODER_SIGN_WINDOWS:-0}"
bin_ident="com.coder.cli"
output_path=""
agpl="${CODER_BUILD_AGPL:-0}"
boringcrypto=${CODER_BUILD_BORINGCRYPTO:-0}
debug=0
dylib=0

args="$(getopt -o "" -l version:,os:,arch:,output:,slim,agpl,sign-darwin,boringcrypto,dylib,debug -- "$@")"
eval set -- "$args"
while true; do
	case "$1" in
	--version)
		version="$2"
		shift 2
		;;
	--os)
		os="$2"
		shift 2
		;;
	--arch)
		arch="$2"
		shift 2
		;;
	--output)
		mkdir -p "$(dirname "$2")"
		output_path="$(realpath "$2")"
		shift 2
		;;
	--slim)
		slim=1
		shift
		;;
	--agpl)
		agpl=1
		shift
		;;
	--sign-darwin)
		sign_darwin=1
		shift
		;;
	--boringcrypto)
		boringcrypto=1
		shift
		;;
	--dylib)
		dylib=1
		shift
		;;
	--debug)
		debug=1
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

cdroot

# Remove the "v" prefix.
version="${version#v}"
if [[ "$version" == "" ]]; then
	version="$(execrelative ./version.sh)"
fi

# Check dependencies
dependencies go
if [[ "$sign_darwin" == 1 ]]; then
	dependencies rcodesign
	requiredenvs AC_CERTIFICATE_FILE AC_CERTIFICATE_PASSWORD_FILE
fi

if [[ "$sign_windows" == 1 ]]; then
	dependencies java
	requiredenvs JSIGN_PATH EV_KEYSTORE EV_KEY EV_CERTIFICATE_PATH EV_TSA_URL GCLOUD_ACCESS_TOKEN
fi

ldflags=(
	-X "'github.com/coder/coder/v2/buildinfo.tag=$version'"
)
# Disable deubgger information if not building a binary for debuggers.
if [[ "$debug" == 0 ]]; then
	ldflags+=(-s -w)
fi

# We use ts_omit_aws here because on Linux it prevents Tailscale from importing
# github.com/aws/aws-sdk-go-v2/aws, which adds 7 MB to the binary.
TS_EXTRA_SMALL="ts_omit_aws,ts_omit_bird,ts_omit_tap,ts_omit_kube"
if [[ "$slim" == 0 ]]; then
	build_args+=(-tags "embed,$TS_EXTRA_SMALL")
else
	build_args+=(-tags "slim,$TS_EXTRA_SMALL")
fi
if [[ "$agpl" == 1 ]]; then
	# We don't use a tag to control AGPL because we don't want code to depend on
	# a flag to control AGPL vs. enterprise behavior.
	ldflags+=(-X "'github.com/coder/coder/v2/buildinfo.agpl=true'")
fi
cgo=0
if [[ "$dylib" == 1 ]]; then
	if [[ "$os" != "darwin" ]]; then
		error "dylib builds are not supported on $os"
	fi
	cgo=1
	build_args+=("-buildmode=c-shared")
	SDKROOT="$(xcrun --sdk macosx --show-sdk-path)"
	export SDKROOT
	bin_ident="com.coder.Coder-Desktop.VPN.dylib"

	plist_file=$(mktemp)
	trap 'rm -f "$plist_file"' EXIT
	# CFBundleShortVersionString must be in the format /[0-9]+.[0-9]+.[0-9]+/
	# CFBundleVersion can be in any format
	BUNDLE_IDENTIFIER="$bin_ident" VERSION_STRING="$version" SHORT_VERSION_STRING=$(echo "$version" | grep -oE '^[0-9]+\.[0-9]+\.[0-9]+') \
		execrelative envsubst <"$(realpath ./vpn/dylib/info.plist.tmpl)" >"$plist_file"
	ldflags+=("-extldflags '-sectcreate __TEXT __info_plist $plist_file'")
fi
build_args+=(-ldflags "${ldflags[*]}")

# Disable optimizations if building a binary for debuggers.
if [[ "$debug" == 1 ]]; then
	build_args+=(-gcflags "all=-N -l")
fi

# Compute default output path.
if [[ "$output_path" == "" ]]; then
	mkdir -p "build"
	output_path="build/coder_${version}_${os}_${arch}"
	if [[ "$os" == "windows" ]]; then
		output_path+=".exe"
	fi
	output_path="$(realpath "$output_path")"
fi
build_args+=(-o "$output_path")

# Determine GOARM.
arm_version=""
if [[ "$arch" == "arm" ]]; then
	arm_version="7"
elif [[ "$arch" == "armv"* ]] || [[ "$arch" == "arm64v"* ]]; then
	arm_version="${arch//*v/}"

	# Remove the v* suffix.
	arch="${arch//v*/}"
fi

cmd_path="./enterprise/cmd/coder"
if [[ "$agpl" == 1 ]]; then
	cmd_path="./cmd/coder"
fi
if [[ "$dylib" == 1 ]]; then
	cmd_path="./vpn/dylib/lib.go"
fi

goexp=""
if [[ "$boringcrypto" == 1 ]]; then
	cgo=1
	goexp="boringcrypto"
fi

GOEXPERIMENT="$goexp" CGO_ENABLED="$cgo" GOOS="$os" GOARCH="$arch" GOARM="$arm_version" \
	go build \
	"${build_args[@]}" \
	"$cmd_path" 1>&2

if [[ "$sign_darwin" == 1 ]] && [[ "$os" == "darwin" ]]; then
	execrelative ./sign_darwin.sh "$output_path" "$bin_ident" 1>&2
fi

if [[ "$sign_windows" == 1 ]] && [[ "$os" == "windows" ]]; then
	execrelative ./sign_windows.sh "$output_path" 1>&2
fi

echo "$output_path"
