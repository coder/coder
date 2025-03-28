#!/usr/bin/env bash

# Coder Quickstart:
# Scripts the steps found at https://coder.com/docs/tutorials/quickstart
# Installs Docker and the latest Coder release, then runs coder server.
# User completes setup by following the URL printed by Coder itself.

set -euo pipefail

echo "🚀 Starting Coder Quickstart"
echo

# Utility
check_command() { command -v "$1" >/dev/null 2>&1; }

# Install Docker if needed
install_docker() {
	echo "📦 Docker not found. Installing..."
	curl -fsSL https://get.docker.com | sh
	echo "✅ Docker installed."
}

# Install latest Coder release
install_coder() {
	echo "📥 Fetching latest Coder release..."

	RELEASE_API="https://api.github.com/repos/coder/coder/releases/latest"
	RELEASE_JSON=$(curl -s "$RELEASE_API")

	LATEST_VERSION=$(echo "$RELEASE_JSON" | grep '"tag_name":' | head -n1 | cut -d '"' -f4)
	echo "🔖 Latest version: $LATEST_VERSION"

	ASSET_URL=$(echo "$RELEASE_JSON" | grep -o 'https://[^"]*coder[^"]*linux[^"]*amd64[^"]*\.tar\.gz' | head -n1)

	if [[ -z "$ASSET_URL" ]]; then
		echo "❌ Could not find Linux AMD64 release asset. Exiting." >&2
		exit 1
	fi

	echo "🌐 Downloading: $ASSET_URL"
	curl -sSL -o coder.tar.gz "$ASSET_URL"

	if ! file coder.tar.gz | grep -q 'gzip compressed'; then
		echo "❌ Downloaded file is not a valid .tar.gz archive. Exiting." >&2
		cat coder.tar.gz
		exit 1
	fi

	tar -xzf coder.tar.gz
	sudo mv coder /usr/local/bin/
	rm -f coder.tar.gz
	echo "✅ Coder $LATEST_VERSION installed."
}

# Start Coder
start_coder() {
	echo "🚀 Starting Coder server..."
	echo "📣 After this, follow the URL printed in the terminal to continue setup in your browser."
	echo
	coder server
}

# Run
if ! check_command docker; then install_docker; fi
if ! check_command coder; then install_coder; fi

start_coder
