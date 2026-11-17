#!/usr/bin/env bash

# Runs github.com/coder/gpgkms (pinned by the tool directive in go.mod) with the
# OpenPGP identity of the Cloud KMS release signing key.
#
# Usage: ./scripts/gpgkms.sh public-key|sign [flags...]
#
# The OpenPGP key is derived from the KMS public key and the identity below, so
# none of it may change: the creation time is part of the fingerprint, and the
# user ID and lifetime are covered by the self-signature. Credentials come from
# Application Default Credentials (GOOGLE_APPLICATION_CREDENTIALS).

set -euo pipefail
# shellcheck source=scripts/lib.sh
source "$(dirname "${BASH_SOURCE[0]}")/lib.sh"

if [[ $# -lt 1 ]]; then
	error "Usage: $0 public-key|sign [flags...]"
fi
cmd="$1"
shift

bin_dir="$(mktemp -d)"
trap 'rm -rf "$bin_dir"' EXIT
(cd "$PROJECT_ROOT" && go build -o "$bin_dir/gpgkms" github.com/coder/gpgkms/cmd/gpgkms)

"$bin_dir/gpgkms" "$cmd" \
	-name "Coder Release Signing Key" \
	-email "security@coder.com" \
	-created "2026-11-16T00:00:00Z" \
	-lifetime "$((4 * 365 * 24))h" \
	"$@"
