#!/usr/bin/env bash

# Usage: ./zizmor.sh [args...]
#
# This script is a wrapper around the zizmor Docker image. Zizmor lints GitHub
# actions workflows.
#
# We use Docker to run zizmor since it's written in Rust and is difficult to
# install on Ubuntu runners without building it with a Rust toolchain, which
# takes a long time.
#
# The repo is mounted at /repo and the working directory is set to /repo.

set -euo pipefail
# shellcheck source=scripts/lib.sh
source "$(dirname "${BASH_SOURCE[0]}")/lib.sh"

cdroot

image_tag="ghcr.io/zizmorcore/zizmor:1.11.0"
docker_args=(
	"--rm"
	"--volume" "$(pwd):/repo"
	"--workdir" "/repo"
	"--network" "host"
)

if [[ -t 0 ]]; then
	docker_args+=("-it")
fi

# If no GH_TOKEN is set, try to get one from `gh auth token`.
if [[ "${GH_TOKEN:-}" == "" ]] && command -v gh &>/dev/null; then
	set +e
	GH_TOKEN="$(gh auth token)"
	export GH_TOKEN
	set -e
fi

# Pass through the GitHub token if it's set, which allows zizmor to scan
# imported workflows too.
if [[ "${GH_TOKEN:-}" != "" ]]; then
	docker_args+=("--env" "GH_TOKEN")
fi

logrun exec docker run "${docker_args[@]}" "$image_tag" "$@"
