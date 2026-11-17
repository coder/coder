#!/usr/bin/env bash

# This script signs the provided darwin binary with an Apple Developer
# certificate.
#
# Usage: ./sign_darwin.sh path/to/binary binary_identifier
#
# On success, the input file will be signed using the Apple Developer
# certificate.
#
# For the Coder CLI, the binary_identifier should be "com.coder.cli".
#
# You can check if a binary is signed by running the following command on a Mac:
#   codesign -dvv path/to/binary
#
# You can also run the following command to verify the signature on other
# systems, but it may be less accurate:
#   rcodesign verify path/to/binary
#
# Depends on the rcodesign utility. The signing key either lives in Google
# Cloud KMS (preferred) or in a local PKCS#12 certificate file.
#
# To sign with a key in Google Cloud KMS via the Cloud KMS PKCS#11 library
# (https://cloud.google.com/kms/docs/reference/pkcs11-library), the following
# environment variables must be set:
#  - $AC_PKCS11_LIBRARY: The path to the Cloud KMS PKCS#11 library
#    (libkmsp11.so). rcodesign must be built with the "pkcs11" feature.
#  - $AC_PKCS11_KEY_LABEL: The name of the Cloud KMS crypto key to sign with.
#    The key must use algorithm RSA_SIGN_PKCS1_2048_SHA256.
#  - $AC_CERTIFICATE_FILE: The path to the Apple Developer certificate issued
#    for the key (public part only, PEM or DER).
#  - $KMS_PKCS11_CONFIG: The path to the Cloud KMS PKCS#11 library YAML
#    configuration file naming the key ring that contains the key. Read by the
#    library itself.
# Additionally, Google credentials with permission to sign with the key must be
# available. Set $AC_PKCS11_CREDENTIALS_FILE to pass a credentials file to
# rcodesign only, so it doesn't replace the caller's default credentials.
# Otherwise Application Default Credentials are used.
#
# To sign with a local PKCS#12 certificate file instead, the following
# environment variables must be set:
#  - $AC_CERTIFICATE_FILE: The path to the Apple Developer P12 certificate file.
#  - $AC_CERTIFICATE_PASSWORD_FILE: The path to the file containing the password
#    for the Apple Developer certificate.

set -euo pipefail
# shellcheck source=scripts/lib.sh
source "$(dirname "${BASH_SOURCE[0]}")/lib.sh"

if [[ "$#" -lt 2 ]]; then
	echo "Usage: $0 path/to/binary binary_identifier"
	exit 1
fi

BINARY_PATH="$1"
BINARY_IDENTIFIER="$2"

# Check dependencies
dependencies rcodesign

if [[ "${AC_PKCS11_LIBRARY:-}" != "" ]]; then
	requiredenvs AC_PKCS11_LIBRARY AC_PKCS11_KEY_LABEL AC_CERTIFICATE_FILE KMS_PKCS11_CONFIG

	if [[ "${AC_PKCS11_CREDENTIALS_FILE:-}" != "" ]]; then
		export GOOGLE_APPLICATION_CREDENTIALS="$AC_PKCS11_CREDENTIALS_FILE"
	fi

	# -v is quite verbose, the default output is pretty good on it's own.
	rcodesign sign \
		--binary-identifier "$BINARY_IDENTIFIER" \
		--pkcs11-library "$AC_PKCS11_LIBRARY" \
		--pkcs11-key-label "$AC_PKCS11_KEY_LABEL" \
		--pkcs11-certificate-file "$AC_CERTIFICATE_FILE" \
		--code-signature-flags runtime \
		"$BINARY_PATH" \
		1>&2
else
	requiredenvs AC_CERTIFICATE_FILE AC_CERTIFICATE_PASSWORD_FILE

	# -v is quite verbose, the default output is pretty good on it's own.
	rcodesign sign \
		--binary-identifier "$BINARY_IDENTIFIER" \
		--p12-file "$AC_CERTIFICATE_FILE" \
		--p12-password-file "$AC_CERTIFICATE_PASSWORD_FILE" \
		--code-signature-flags runtime \
		"$BINARY_PATH" \
		1>&2
fi
