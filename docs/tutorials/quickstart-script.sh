#!/usr/bin/env bash

# Coder Quickstart Script
# Installs Docker (brew on macOS, get.docker.com on Linux)
# Installs Coder via official script
# Starts `coder server` — user follows the printed URL

set -euo pipefail

echo "🚀 Starting Coder Quickstart"
echo

# --- Utility ---
check_command() { command -v "$1" >/dev/null 2>&1; }

# --- Install Docker ---
install_docker() {
  echo "📦 Docker not found. Installing..."

  OS="$(uname -s | tr '[:upper:]' '[:lower:]')"

  if [[ "$OS" == "darwin" ]]; then
    echo "🍎 macOS detected."

    if ! check_command brew; then
      echo "❌ Homebrew not found. Please install Homebrew first:"
      echo "👉 https://brew.sh"
      exit 1
    fi

    echo "🍺 Installing Docker Desktop via Homebrew..."
    brew install --cask docker
    echo "✅ Docker Desktop installed."

    echo "🚀 Launching Docker Desktop..."
    open -a Docker

    echo "⏳ Waiting for Docker to start..."
    until docker info >/dev/null 2>&1; do
      sleep 2
    done

    echo "✅ Docker is running."

  elif [[ "$OS" == "linux" ]]; then
    echo "🐧 Installing Docker for Linux..."
    curl -fsSL https://get.docker.com | sh
    echo "✅ Docker installed on Linux."
  else
    echo "❌ Unsupported OS for Docker auto-install: $OS"
    exit 1
  fi
}

# --- Install Coder using the official installer ---
install_coder() {
  echo "📥 Installing Coder using official script..."
  curl -fsSL https://coder.com/install.sh | sh
  echo "✅ Coder installed."
}

# --- Start Coder server ---
start_coder() {
  echo
  echo "🚀 Starting Coder server..."
  echo "📣 Follow the URL printed below to finish setup in your browser."
  coder server
}

# --- Run ---
if ! check_command docker; then install_docker; fi
if ! check_command coder; then install_coder; fi

start_coder
