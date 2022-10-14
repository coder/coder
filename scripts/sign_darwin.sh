#!/usr/bin/env bash

# This script signs the provided darwin binary with an Apple Developer
# certificate.
#
# Usage: ./sign_darwin.sh path/to/binary
#
# On success, the input file will be signed using the Apple Developer
# certificate.
#
# You can check if a binary is signed by running the following command on a Mac:
#   codesign -dvv path/to/binary
#
# You can also run the following command to verify the signature on other
# systems, but it may be less accurate:
#   rcodesign verify path/to/binary
#
# Depends on the rcodesign utility. Requires the following environment variables
# to be set:
#  - $AC_CERTIFICATE_FILE: The path to the Apple Developer P12 certificate file.
#  - $AC_CERTIFICATE_PASSWORD_FILE: The path to the file containing the password
#    for the Apple Developer certificate.

set -euo pipefail
# shellcheck source=scripts/lib.sh
source "$(dirname "${BASH_SOURCE[0]}")/lib.sh"

# Check dependencies
dependencies rcodesign
requiredenvs AC_CERTIFICATE_FILE AC_CERTIFICATE_PASSWORD_FILE

# -v is quite verbose, the default output is pretty good on it's own.
rcodesign sign \
	--binary-identifier "com.coder.cli" \
	--p12-file "$AC_CERTIFICATE_FILE" \
	--p12-password-file "$AC_CERTIFICATE_PASSWORD_FILE" \
	--code-signature-flags runtime \
	"$@" \
	1>&2
