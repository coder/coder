#!/usr/bin/env bash

set -euo pipefail

PROJECT_ROOT="$(git rev-parse --show-toplevel)"

curl_flags=(
	--silent
	--show-error
	--location
)

gpg_flags=(
	--dearmor
	--yes
)

pushd "$PROJECT_ROOT/dogfood/coder/ubuntu-26.04/files/usr/share/keyrings"

# Upstream Docker signing key
curl "${curl_flags[@]}" "https://download.docker.com/linux/ubuntu/gpg" |
	gpg "${gpg_flags[@]}" --output="docker.gpg"

# GitHub CLI signing key
curl "${curl_flags[@]}" "https://cli.github.com/packages/githubcli-archive-keyring.gpg" |
	gpg "${gpg_flags[@]}" --output="github-cli.gpg"

# Google Cloud signing key
curl "${curl_flags[@]}" "https://packages.cloud.google.com/apt/doc/apt-key.gpg" |
	gpg "${gpg_flags[@]}" --output="google-cloud.gpg"

# Hashicorp signing key
curl "${curl_flags[@]}" "https://apt.releases.hashicorp.com/gpg" |
	gpg "${gpg_flags[@]}" --output="hashicorp.gpg"

# Upstream PostgreSQL signing key
curl "${curl_flags[@]}" "https://www.postgresql.org/media/keys/ACCC4CF8.asc" |
	gpg "${gpg_flags[@]}" --output="postgresql.gpg"

popd
