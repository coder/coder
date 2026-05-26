#!/usr/bin/env bash

# Validates dogfood image tooling by running gen, fmt, lint, and build inside
# the image. Can be run locally or in CI (mirrors the test_image workflow job).
#
# Usage: ./scripts/dogfood_test_image.sh <image>
#
# Arguments:
#   image   Docker image to test, e.g. dogfood-test:22.04 or
#           ghcr.io/coder/dogfood:latest
#
# Environment:
#   GITHUB_TOKEN      Passed into the container for authenticated API calls
#                     (optional for local runs).
#   GITHUB_BASE_REF   Base branch for diff-only lint checks (e.g. emdash).
#                     Set automatically by GitHub Actions for PRs.
#   CI                When set, fmt targets run in check-mode and actionlint
#                     is excluded from make lint (it runs separately in CI).
#   STEPS             Space-separated list of steps to run. Defaults to all.
#                     Valid values: gen fmt lint build check-unstaged
#
# Example:
#   ./scripts/dogfood_test_image.sh dogfood-test:22.04
#   STEPS="gen fmt" ./scripts/dogfood_test_image.sh dogfood-test:26.04

set -euo pipefail
# shellcheck source=scripts/lib.sh
source "$(dirname "${BASH_SOURCE[0]}")/lib.sh"
cdroot

if [[ $# -lt 1 ]]; then
	echo "Usage: $0 <image>" >&2
	exit 1
fi

IMAGE="$1"
STEPS="${STEPS:-gen fmt lint build check-unstaged}"

log() {
	echo "==> $*" >&2
}

# --- setup -------------------------------------------------------------------

if [[ -n "${CI:-}" ]]; then
	log "Preparing checkout for container user (UID 1000)"
	chmod -R a+rwX .
else
	log "NOTE: if the container cannot write to the checkout, run: chmod -R a+rwX ."
fi

# Helper: run a make target inside the image.
#
# Mounts /home/coder/ as a single named volume to mirror the dogfood
# workspace template (dogfood/coder/main.tf), so caches (Go modules,
# Go build, pnpm store, mise data, etc.) persist the same way they do
# in real workspaces. Per-cache subpath volumes would come up
# root-owned on first mount because Docker creates non-existent
# subpaths root-owned; the home-level volume inherits coder:coder
# from the image's existing /home/coder (`useradd --create-home`).
run_make() {
	docker run --rm \
		--volume coder-dogfood-home:/home/coder \
		--volume "$(pwd)":/home/coder/coder \
		--env GIT_CONFIG_COUNT=1 \
		--env GIT_CONFIG_KEY_0=safe.directory \
		--env GIT_CONFIG_VALUE_0=/home/coder/coder \
		--workdir /home/coder/coder \
		--network=host \
		--env GITHUB_TOKEN \
		--env GITHUB_BASE_REF \
		--env CI \
		"$IMAGE" \
		make "$@"
}

# --- steps -------------------------------------------------------------------

for step in $STEPS; do
	case "$step" in
	gen)
		log "make gen (GEN_SKIP_GOLDEN=1, skips tests that need Docker/testcontainers)"
		run_make --output-sync=line -j gen GEN_SKIP_GOLDEN=1
		;;
	fmt)
		log "make fmt"
		run_make --output-sync=line -j fmt
		;;
	lint)
		log "make lint"
		run_make --output-sync=line -j lint
		;;
	build)
		log "make build (fat binary)"
		run_make -j build/coder_linux_amd64
		;;
	check-unstaged)
		# Runs on the host: inspects git state after container steps wrote
		# generated/formatted files back via the volume mount.
		log "Checking for unstaged files"
		./scripts/check_unstaged.sh
		;;
	*)
		echo "Unknown step: $step" >&2
		echo "Valid steps: gen fmt lint build check-unstaged" >&2
		exit 1
		;;
	esac
done

log "All steps passed."
