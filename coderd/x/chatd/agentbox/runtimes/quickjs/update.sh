#!/usr/bin/env bash
# Downloads the quickjs-ng WASI build recorded in VERSION and verifies
# its checksum. Edit VERSION first to move to a new release.

set -euo pipefail

cd "$(dirname "${BASH_SOURCE[0]}")"

url="$(sed -n 's/^url=//p' VERSION)"
sha256="$(sed -n 's/^sha256=//p' VERSION)"

if [[ -z ${url} || -z ${sha256} ]]; then
	echo "VERSION must define url and sha256" >&2
	exit 1
fi

tmp="$(mktemp)"
trap 'rm -f "${tmp}"' EXIT

curl -fsSL "${url}" -o "${tmp}"
echo "${sha256}  ${tmp}" | sha256sum -c -
mv "${tmp}" qjs-wasi.wasm
trap - EXIT
echo "updated qjs-wasi.wasm to ${url}"
