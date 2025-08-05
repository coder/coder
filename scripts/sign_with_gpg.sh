#!/usr/bin/env bash

# This script signs a given binary using GPG.
# It expects the binary to be signed as the first argument.
#
# Usage: ./sign_with_gpg.sh path/to/binary
#
# On success, the input file will be signed using the GPG key and the signature output file will moved to /site/out/bin/ (happens in the Makefile)
#
# Depends on the GPG utility. Requires the following environment variables to be set:
#  - $CODER_GPG_RELEASE_KEY_BASE64: The base64 encoded private key to use.

set -euo pipefail
# shellcheck source=scripts/lib.sh
source "$(dirname "${BASH_SOURCE[0]}")/lib.sh"

requiredenvs CODER_GPG_RELEASE_KEY_BASE64

FILE_TO_SIGN="$1"

if [[ -z "$FILE_TO_SIGN" ]]; then
	error "Usage: $0 <file_to_sign>"
fi

if [[ ! -f "$FILE_TO_SIGN" ]]; then
	error "File not found: $FILE_TO_SIGN"
fi

# Import the GPG key.
old_gnupg_home="${GNUPGHOME:-}"
gnupg_home_temp="$(mktemp -d)"
export GNUPGHOME="$gnupg_home_temp"

# Ensure GPG uses the temporary directory
echo "$CODER_GPG_RELEASE_KEY_BASE64" | base64 -d | gpg --homedir "$gnupg_home_temp" --import 1>&2

# Sign the binary. This generates a file in the same directory and
# with the same name as the binary but ending in ".asc".
#
# We pipe `true` into `gpg` so that it never tries to be interactive (i.e.
# ask for a passphrase). The key we import above is not password protected.
true | gpg --homedir "$gnupg_home_temp" --detach-sign --armor "$FILE_TO_SIGN" 1>&2

# Verify the signature and capture the exit status
gpg --homedir "$gnupg_home_temp" --verify "${FILE_TO_SIGN}.asc" "$FILE_TO_SIGN" 1>&2
verification_result=$?

# Clean up the temporary GPG home
rm -rf "$gnupg_home_temp"
unset GNUPGHOME
if [[ "$old_gnupg_home" != "" ]]; then
	export GNUPGHOME="$old_gnupg_home"
fi

if [[ $verification_result -eq 0 ]]; then
	echo "${FILE_TO_SIGN}.asc"
else
	error "Signature verification failed!"
fi
