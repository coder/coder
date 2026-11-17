#!/usr/bin/env bash

# Runs github.com/coder/gpgkms (a go.mod tool dependency) with the
# OpenPGP identity of the Cloud KMS release signing key.
#
# Usage: ./scripts/gpgkms.sh public-key|sign [flags...]
#
# The OpenPGP key is derived from the KMS public key and the identity below, so
# none of it may change: the creation time is part of the fingerprint, and the
# user ID and lifetime are covered by the self-signature. -fingerprint makes
# gpgkms fail if the KMS key or creation time ever change; it does not cover the
# user ID or lifetime. Credentials come from Application Default Credentials
# (GOOGLE_APPLICATION_CREDENTIALS).

set -euo pipefail
# shellcheck source=scripts/lib.sh
source "$(dirname "${BASH_SOURCE[0]}")/lib.sh"

if [[ $# -lt 1 ]]; then
	error "Usage: $0 public-key|sign [flags...]"
fi
cmd="$1"
shift

# go tool needs a working directory inside the module; it is not changed here
# so relative paths in the arguments keep working.
exec go tool github.com/coder/gpgkms/cmd/gpgkms "$cmd" \
	-name "Coder Release Signing Key" \
	-email "security@coder.com" \
	-created "2026-11-16T00:00:00Z" \
	-lifetime "$((4 * 365 * 24))h" \
	-fingerprint "0F685EFE0AE5A8182A473DD86E3A8E5613707520" \
	"$@"
