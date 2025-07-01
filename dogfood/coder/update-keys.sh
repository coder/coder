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

pushd "$PROJECT_ROOT/dogfood/coder/files/usr/share/keyrings"

# Ansible PPA signing key
curl "${curl_flags[@]}" "https://keyserver.ubuntu.com/pks/lookup?op=get&search=0X6125E2A8C77F2818FB7BD15B93C4A3FD7BB9C367" |
	gpg "${gpg_flags[@]}" --output="ansible.gpg"

# Upstream Docker signing key
curl "${curl_flags[@]}" "https://download.docker.com/linux/ubuntu/gpg" |
	gpg "${gpg_flags[@]}" --output="docker.gpg"

# Fish signing key
curl "${curl_flags[@]}" "https://keyserver.ubuntu.com/pks/lookup?op=get&search=0x88421E703EDC7AF54967DED473C9FCC9E2BB48DA" |
	gpg "${gpg_flags[@]}" --output="fish-shell.gpg"

# Git-Core signing key
curl "${curl_flags[@]}" "https://keyserver.ubuntu.com/pks/lookup?op=get&search=0xE1DD270288B4E6030699E45FA1715D88E1DF1F24" |
	gpg "${gpg_flags[@]}" --output="git-core.gpg"

# GitHub CLI signing key
curl "${curl_flags[@]}" "https://cli.github.com/packages/githubcli-archive-keyring.gpg" |
	gpg "${gpg_flags[@]}" --output="github-cli.gpg"

# Google Linux Software repository signing key (Chrome)
curl "${curl_flags[@]}" "https://dl.google.com/linux/linux_signing_key.pub" |
	gpg "${gpg_flags[@]}" --output="google-chrome.gpg"

# Google Cloud signing key
curl "${curl_flags[@]}" "https://packages.cloud.google.com/apt/doc/apt-key.gpg" |
	gpg "${gpg_flags[@]}" --output="google-cloud.gpg"

# Hashicorp signing key
curl "${curl_flags[@]}" "https://apt.releases.hashicorp.com/gpg" |
	gpg "${gpg_flags[@]}" --output="hashicorp.gpg"

# Helix signing key
curl "${curl_flags[@]}" "https://keyserver.ubuntu.com/pks/lookup?op=get&search=0x27642B9FD7F1A161FC2524E3355A4FA515D7C855" |
	gpg "${gpg_flags[@]}" --output="helix.gpg"

# Microsoft repository signing key (Edge)
curl "${curl_flags[@]}" "https://packages.microsoft.com/keys/microsoft.asc" |
	gpg "${gpg_flags[@]}" --output="microsoft.gpg"

# Neovim signing key
curl "${curl_flags[@]}" "https://keyserver.ubuntu.com/pks/lookup?op=get&search=0x9DBB0BE9366964F134855E2255F96FCF8231B6DD" |
	gpg "${gpg_flags[@]}" --output="neovim.gpg"

# NodeSource signing key
curl "${curl_flags[@]}" "https://deb.nodesource.com/gpgkey/nodesource-repo.gpg.key" |
	gpg "${gpg_flags[@]}" --output="nodesource.gpg"

# Upstream PostgreSQL signing key
curl "${curl_flags[@]}" "https://www.postgresql.org/media/keys/ACCC4CF8.asc" |
	gpg "${gpg_flags[@]}" --output="postgresql.gpg"

# Yarnpkg signing key
curl "${curl_flags[@]}" "https://dl.yarnpkg.com/debian/pubkey.gpg" |
	gpg "${gpg_flags[@]}" --output="yarnpkg.gpg"

popd
