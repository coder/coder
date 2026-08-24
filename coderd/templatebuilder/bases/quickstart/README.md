---
display_name: Coder Quickstart
description: Get started with Coder by picking your languages and a repo
icon: ../../../site/static/icon/coder.svg
maintainer_github: coder
verified: true
tags: [docker, quickstart]
---

# Coder Quickstart

Get up and running with Coder in minutes. Choose your programming languages, optionally clone a Git repository, and start coding.

## How It Works

When you create a workspace from this template, you select:

1. **Languages** to pre-install (Python, Node.js, Go, Rust, Java, C/C++)
2. **A Git repository** to clone (optional)

Coder provisions a workspace with your selections and you can start developing immediately.

<!-- prerequisites:start -->

## Prerequisites

The host running Coder must have a Docker daemon accessible to the `coder` user:

```sh
# Add coder user to Docker group
sudo adduser coder docker

# Restart Coder server
sudo systemctl restart coder

# Verify access
sudo -u coder docker ps
```

<!-- prerequisites:end -->

## Architecture

This template provisions:

- **Docker container** (ephemeral) running Ubuntu with the Coder agent
- **Docker volume** (persistent) mounted at `/home/coder`

Files in your home directory (`/home/coder`) persist across workspace restarts, and this template installs language toolchains there so they persist too. The language install script runs on every start and blocks login until it finishes. On the first start it downloads the selected toolchains that are not already in the base image and installs them under your home directory; on later starts it detects the persisted toolchains and reuses them instead of re-downloading. Python and C/C++ ship in the base image, while Go, Node.js, Java, and Rust are installed under `/home/coder` (Rust under `~/.cargo`, the others under `~/.local`). This template assumes the deployment has internet access, since the first install fetches toolchains from the network.

## Presets

Select a preset to auto-fill languages for common workflows:

| Preset              | Languages           |
| ------------------- | ------------------- |
| **Web Development** | Python, Node.js     |
| **Backend (Go)**    | Go                  |
| **Data Science**    | Python              |
| **Full Stack**      | Python, Node.js, Go |

## Editors

VS Code Desktop is available on every workspace by default (Coder enables the VS Code Desktop display app automatically). To add more editors (VS Code in the browser, Cursor, JetBrains, Zed, Windsurf) or other tools, add them as modules in the next step of the template builder.
