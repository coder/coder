#!/usr/bin/env bash

# This script notarizes the provided zip file using an Apple Developer account.
#
# Usage: ./notarize_darwin.sh path/to/zipfile.zip
#
# The provided zip file must contain a coder binary that has already been signed
# using ./sign_darwin.sh.
#
# On success, all of the contained binaries inside the input zip file will
# notarized. This does not make any changes to the zip or contained files
# itself, but GateKeeper checks will pass for the binaries inside the zip file
# as long as the device is connected to the internet to download the
# notarization ticket from Apple.
#
# You can check if a binary is notarized by running the following command on a
# Mac:
#   spctl --assess -vvv -t install path/to/binary
#
# Depends on the rcodesign utility. Requires the following environment variables
# to be set:
#  - $AC_APIKEY_ISSUER_ID: The issuer UUID of the Apple App Store Connect API
#    key.
#  - $AC_APIKEY_ID: The key ID of the Apple App Store Connect API key.
#  - $AC_APIKEY_FILE: The path to the private key P8 file of the Apple App Store
#    Connect API key.

set -euo pipefail
# shellcheck source=scripts/lib.sh
source "$(dirname "${BASH_SOURCE[0]}")/lib.sh"

# Check dependencies
dependencies rcodesign
requiredenvs AC_APIKEY_ISSUER_ID AC_APIKEY_ID AC_APIKEY_FILE

# Encode the notarization key components into a JSON file for easily calling
# `rcodesign notary-submit`.
key_file="$(mktemp)"
chmod 600 "$key_file"
trap 'rm -f "$key_file"' EXIT
rcodesign encode-app-store-connect-api-key \
	"$AC_APIKEY_ISSUER_ID" \
	"$AC_APIKEY_ID" \
	"$AC_APIKEY_FILE" \
	>"$key_file"

# The notarization process is very fragile and heavily dependent on Apple's
# notarization server not returning server errors, so we retry this step twice
# with a delay of 30 seconds between attempts.
NOTARY_SUBMIT_ATTEMPTS=2
rc=0
for i in $(seq 1 $NOTARY_SUBMIT_ATTEMPTS); do
	# -v is quite verbose, the default output is pretty good on it's own. Adding
	# -v makes it dump the credentials used for uploading to Apple's S3 bucket.
	rcodesign notary-submit \
		--api-key-path "$key_file" \
		--wait \
		"$@" \
		1>&2 && rc=0 && break || rc=$?

	log "rcodesign exit code: $rc"
	if [[ $i -lt $NOTARY_SUBMIT_ATTEMPTS ]]; then
		log
		log "Retrying notarization in 30 seconds"
		log
		sleep 30
	else
		log
		log "Giving up :("
	fi
done

exit $rc
