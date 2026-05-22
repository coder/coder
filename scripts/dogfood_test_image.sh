#!/usr/bin/env bash

# Runs the CI toolchain checks (gen, fmt, lint, build) inside a dogfood image.
# Mirrors the test_image job in .github/workflows/dogfood.yaml so it can be
# run locally to validate a freshly built dogfood image before deployment.
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

GITCONFIG=/tmp/coder-dogfood-gitconfig

log() {
	echo "==> $*" >&2
}

# --- setup -------------------------------------------------------------------

log "Preparing checkout for container user (UID 1000)"
chmod -R a+rwX .

# Create a minimal gitconfig so the container's coder user (UID 1000)
# recognises the runner-owned checkout as a safe directory. Without this,
# git rev-parse fails inside scripts/lib.sh when resolving PROJECT_ROOT.
printf '[safe]\n\tdirectory = /home/coder/coder\n' >"$GITCONFIG"

# Helper: run a make target inside the image.
run_make() {
	docker run --rm \
		--volume "$(pwd)":/home/coder/coder \
		--volume "${GITCONFIG}":/home/coder/.gitconfig:ro \
		--workdir /home/coder/coder \
		--network=host \
		--env GITHUB_TOKEN \
		--env GITHUB_BASE_REF \
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
		# Fat build validates the full toolchain: Go, Node, pnpm, TypeScript/React.
		log "make build (fat binary)"
		run_make -j build/coder_linux_amd64
		;;
	check-unstaged)
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
