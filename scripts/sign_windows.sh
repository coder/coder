#!/usr/bin/env bash

# This script signs the provided windows binary with an Extended Validation
# code signing certificate.
#
# Usage: ./sign_windows.sh path/to/binary
#
# On success, the input file will be signed using the EV cert.
#
# Depends on the jsign utility (and thus Java). Requires the following environment variables
# to be set:
#  - $JSIGN_PATH: The path to the jsign jar.
#  - $EV_KEYSTORE: The name of the keyring containing the private key
#  - $EV_KEY: The name of the key.
#  - $EV_CERTIFICATE_PATH: The path to the certificate.
#  - $EV_TSA_URL: The url of the timestamp server to use.

set -euo pipefail
# shellcheck source=scripts/lib.sh
source "$(dirname "${BASH_SOURCE[0]}")/lib.sh"

# Check dependencies
dependencies java
requiredenvs JSIGN_PATH EV_KEYSTORE EV_KEY EV_CERTIFICATE_PATH EV_TSA_URL GCLOUD_ACCESS_TOKEN

java -jar "$JSIGN_PATH" \
	--storetype GOOGLECLOUD \
	--storepass "$GCLOUD_ACCESS_TOKEN" \
	--keystore "$EV_KEYSTORE" \
	--alias "$EV_KEY" \
	--certfile "$EV_CERTIFICATE_PATH" \
	--tsmode RFC3161 \
	--tsaurl "$EV_TSA_URL" \
	"$@" \
	1>&2
