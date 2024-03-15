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

pushd "$PROJECT_ROOT/dogfood/files/usr/share/keyrings"
# Upstream Docker signing key
curl "${curl_flags[@]}" "https://download.docker.com/linux/ubuntu/gpg" |
	gpg "${gpg_flags[@]}" --output="docker.gpg"

# Google Cloud signing key
curl "${curl_flags[@]}" "https://packages.cloud.google.com/apt/doc/apt-key.gpg" |
	gpg "${gpg_flags[@]}" --output="google-cloud.gpg"

# Google Linux Software repository signing key (Chrome)
curl "${curl_flags[@]}" "https://dl.google.com/linux/linux_signing_key.pub" |
	gpg "${gpg_flags[@]}" --output="google-chrome.gpg"

# Microsoft repository signing key (Edge)
curl "${curl_flags[@]}" "https://packages.microsoft.com/keys/microsoft.asc" |
	gpg "${gpg_flags[@]}" --output="microsoft.gpg"

# Upstream PostgreSQL signing key
curl "${curl_flags[@]}" "https://www.postgresql.org/media/keys/ACCC4CF8.asc" |
	gpg "${gpg_flags[@]}" --output="postgresql.gpg"

# NodeSource signing key
curl "${curl_flags[@]}" "https://deb.nodesource.com/gpgkey/nodesource-repo.gpg.key" |
	gpg "${gpg_flags[@]}" --output="nodesource.gpg"

# Yarnpkg signing key
curl "${curl_flags[@]}" "https://dl.yarnpkg.com/debian/pubkey.gpg" |
	gpg "${gpg_flags[@]}" --output="yarnpkg.gpg"

# Ansible PPA signing key
curl "${curl_flags[@]}" "https://keyserver.ubuntu.com/pks/lookup?op=get&search=0x6125e2a8c77f2818fb7bd15b93c4a3fd7bb9c367" |
	gpg "${gpg_flags[@]}" --output="ansible.gpg"

# Neovim signing key
curl "${curl_flags[@]}" "https://keyserver.ubuntu.com/pks/lookup?op=get&search=0x9dbb0be9366964f134855e2255f96fcf8231b6dd" |
	gpg "${gpg_flags[@]}" --output="neovim.gpg"

# Hashicorp signing key
curl "${curl_flags[@]}" "https://apt.releases.hashicorp.com/gpg" |
	gpg "${gpg_flags[@]}" --output="hashicorp.gpg"

# GitHub CLI signing key
curl "${curl_flags[@]}" "https://cli.github.com/packages/githubcli-archive-keyring.gpg" |
	gpg "${gpg_flags[@]}" --output="github-cli.gpg"
popd
