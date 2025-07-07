#!/usr/bin/env bash

# This script signs a given binary using GPG.
# It expects the binary to be signed as the first argument.
#
# Usage: ./sign_with_gpg.sh path/to/binary
#
# On success, the input file will be signed using the GPG key.
#
# Depends on the GPG utility. Requires the following environment variables to be set:
#  - $CODER_GPG_RELEASE_KEY_BASE64: The base64 encoded private key to use.

set -euo pipefail

requiredenvs CODER_GPG_RELEASE_KEY_BASE64

FILE_TO_SIGN="$1"

if [[ -z "$FILE_TO_SIGN" ]]; then
  echo "Usage: $0 <file_to_sign>"
  exit 1
fi

if [[ ! -f "$FILE_TO_SIGN" ]]; then
  echo "File not found: $FILE_TO_SIGN"
  exit 1
fi

# Import the private key.
echo "$CODER_GPG_RELEASE_KEY_BASE64" | base64 --decode | gpg --import 1>&2

# Sign the binary.
gpg --detach-sign --armor "$FILE_TO_SIGN" 1>&2

# Verify the signature.
gpg --verify "${FILE_TO_SIGN}.sig" "$FILE_TO_SIGN" 1>&2
  
if [[ $? -eq 0 ]]; then
  echo "${FILE_TO_SIGN}.sig"
else
  echo "Signature verification failed!" >&2
  exit 1
fi
