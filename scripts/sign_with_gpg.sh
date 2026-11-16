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
#
# Optionally, the file is also signed with a release key held in Google Cloud
# KMS (see scripts/gpgkms). That signature goes in a separate .v2.asc file, so
# the .asc file and every client that verifies it stay unchanged. Set both of
# these to enable it:
#  - $CODER_GPG_KMS_KEY: The Cloud KMS CryptoKeyVersion resource name.
#  - $CODER_GPG_KMS_CREDENTIALS_FILE: Google credentials for the KMS signer.
#    Only gpgkms sees them, so they don't replace the job's default credentials.

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
# Remove the temporary keyring (which holds the private key) even if a step
# below fails.
trap 'rm -rf "$gnupg_home_temp"' EXIT

# Ensure GPG uses the temporary directory
echo "$CODER_GPG_RELEASE_KEY_BASE64" | base64 -d | gpg --homedir "$gnupg_home_temp" --import 1>&2

# Mark the imported key as ultimately trusted so GPG does not emit an
# "untrusted key" warning during signature verification. We derive the
# fingerprint from the keyring rather than hard-coding it so this works
# regardless of which key is supplied.
fingerprint="$(gpg --homedir "$gnupg_home_temp" --with-colons --fingerprint | awk -F: '/^fpr/ { print $10; exit }')"
echo "${fingerprint}:6:" | gpg --homedir "$gnupg_home_temp" --import-ownertrust 1>&2

# Sign the binary. This generates a file in the same directory and
# with the same name as the binary but ending in ".asc".
#
# We pipe `true` into `gpg` so that it never tries to be interactive (i.e.
# ask for a passphrase). The key we import above is not password protected.
true | gpg --homedir "$gnupg_home_temp" --detach-sign --armor "$FILE_TO_SIGN" 1>&2

# Verify the signature and capture the exit status
gpg --homedir "$gnupg_home_temp" --verify "${FILE_TO_SIGN}.asc" "$FILE_TO_SIGN" 1>&2
verification_result=$?

# Never leave a .v2.asc from an earlier build next to a newly built file.
rm -f "${FILE_TO_SIGN}.v2.asc"
if [[ "${CODER_GPG_KMS_KEY:-}" != "" ]]; then
	requiredenvs CODER_GPG_KMS_CREDENTIALS_FILE
	gpgkms_bin="$gnupg_home_temp/gpgkms"
	(cd "$PROJECT_ROOT" && go build -o "$gpgkms_bin" ./scripts/gpgkms)
	gpgkms() {
		GOOGLE_APPLICATION_CREDENTIALS="$CODER_GPG_KMS_CREDENTIALS_FILE" "$gpgkms_bin" "$@"
	}

	kms_signature="$gnupg_home_temp/kms.asc"
	gpgkms sign -key "$CODER_GPG_KMS_KEY" -out "$kms_signature" "$FILE_TO_SIGN" 1>&2

	# Verify against a keyring holding only the KMS key, so the file must
	# contain exactly one signature, made by that key.
	kms_gnupg_home="$gnupg_home_temp/kms"
	mkdir -m 700 "$kms_gnupg_home"
	gpgkms public-key -key "$CODER_GPG_KMS_KEY" | gpg --homedir "$kms_gnupg_home" --import 1>&2
	gpg --homedir "$kms_gnupg_home" --verify "$kms_signature" "$FILE_TO_SIGN" 1>&2 || verification_result=$?
	if [[ $verification_result -eq 0 ]]; then
		mv "$kms_signature" "${FILE_TO_SIGN}.v2.asc"
	fi
fi

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
