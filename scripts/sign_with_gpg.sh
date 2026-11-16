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
# KMS (see scripts/gpgkms). Both signatures go in the same .asc file, with the
# $CODER_GPG_RELEASE_KEY_BASE64 signature first. Set both of these to enable it:
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
# Remove the temporary keyring (which holds the private key) and any
# intermediate signature even if a step below fails.
trap 'rm -rf "$gnupg_home_temp" "${FILE_TO_SIGN}.sig"' EXIT

# Ensure GPG uses the temporary directory
echo "$CODER_GPG_RELEASE_KEY_BASE64" | base64 -d | gpg --homedir "$gnupg_home_temp" --import 1>&2

# Mark the imported key as ultimately trusted so GPG does not emit an
# "untrusted key" warning during signature verification. We derive the
# fingerprint from the keyring rather than hard-coding it so this works
# regardless of which key is supplied.
fingerprint="$(gpg --homedir "$gnupg_home_temp" --with-colons --fingerprint | awk -F: '/^fpr/ { print $10; exit }')"
echo "${fingerprint}:6:" | gpg --homedir "$gnupg_home_temp" --import-ownertrust 1>&2

kms_key="${CODER_GPG_KMS_KEY:-}"
if [[ "$kms_key" != "" ]]; then
	requiredenvs CODER_GPG_KMS_CREDENTIALS_FILE
	gpgkms_bin="$gnupg_home_temp/gpgkms"
	(cd "$PROJECT_ROOT" && go build -o "$gpgkms_bin" ./scripts/gpgkms)
	gpgkms() {
		GOOGLE_APPLICATION_CREDENTIALS="$CODER_GPG_KMS_CREDENTIALS_FILE" "$gpgkms_bin" "$@"
	}
	kms_args=(-key "$kms_key")
fi

# Sign the binary. This generates a file in the same directory and
# with the same name as the binary but ending in ".asc".
#
# We pipe `true` into `gpg` so that it never tries to be interactive (i.e.
# ask for a passphrase). The key we import above is not password protected.
expected_signatures=1
if [[ "$kms_key" == "" ]]; then
	true | gpg --homedir "$gnupg_home_temp" --detach-sign --armor "$FILE_TO_SIGN" 1>&2
else
	# Sign with the GPG key first, unarmored, then append the KMS signature.
	# The old key's signature must stay first: released VS Code extensions
	# only verify the first signature in the file.
	true | gpg --homedir "$gnupg_home_temp" --batch --yes --detach-sign --output "${FILE_TO_SIGN}.sig" "$FILE_TO_SIGN" 1>&2
	gpgkms sign "${kms_args[@]}" -prepend "${FILE_TO_SIGN}.sig" -out "${FILE_TO_SIGN}.asc" "$FILE_TO_SIGN" 1>&2
	rm -f "${FILE_TO_SIGN}.sig"

	# Import the KMS public key so both signatures can be verified below.
	gpgkms public-key "${kms_args[@]}" | gpg --homedir "$gnupg_home_temp" --import 1>&2
	expected_signatures=2
fi

# Verify the signature(s) and capture the exit status. gpg exits non-zero if
# any signature is bad, and we also require one good signature per key.
verification_result=0
good_signatures="$(gpg --homedir "$gnupg_home_temp" --status-fd 1 --verify "${FILE_TO_SIGN}.asc" "$FILE_TO_SIGN" | grep -c '^\[GNUPG:\] GOODSIG ')" || verification_result=$?
if [[ "$verification_result" -eq 0 ]] && [[ "$good_signatures" -ne "$expected_signatures" ]]; then
	log "Expected $expected_signatures good signatures, got $good_signatures"
	verification_result=1
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
