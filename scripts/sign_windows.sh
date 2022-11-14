#!/usr/bin/env bash

set -x
# This script signs the provided windows binary with a X.509 certificate and
# it's associated private key.
#
# Usage: ./sign_windows.sh path/to/binary
#
# On success, the input file will be signed using the X.509 certificate.
#
# You can check if a binary is signed by running the following command:
#   osslsigncode verify path/to/binary
#
# Depends on the osslsigncode utility. Requires the following environment variables
# to be set:
#  - $AUTHENTICODE_CERTIFICATE_FILE: The path to the X5.09 certificate file.
#  - $AUTHENTICODE_CERTIFICATE_PASSWORD_FILE: The path to the file containing the password
#    for the X5.09 certificate.

set -euo pipefail
# shellcheck source=scripts/lib.sh
source "$(dirname "${BASH_SOURCE[0]}")/lib.sh"

# Check dependencies
dependencies osslsigncode
requiredenvs AUTHENTICODE_CERTIFICATE_FILE AUTHENTICODE_CERTIFICATE_PASSWORD_FILE

osslsigncode sign \
	-pkcs12 "$AUTHENTICODE_CERTIFICATE_FILE" \
	-readpass "$AUTHENTICODE_CERTIFICATE_PASSWORD_FILE" \
	-n "Coder" \
	-i "https://coder.com" \
	-t "http://timestamp.sectigo.com"
	-in "$@" \
	-out "$@" \
	1>&2

osslsigncodeosslsigncode verify "$@" 1>&2
