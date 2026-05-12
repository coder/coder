---
display_name: Coder Quickstart
description: Get started with Coder by picking your languages, editors, and a repo
icon: ../../../site/static/icon/coder.svg
maintainer_github: coder
verified: true
tags: [docker, quickstart]
---

# Coder Quickstart

Get up and running with Coder in minutes. Choose your programming languages, pick your preferred editors, optionally clone a Git repository, and start coding.

## How It Works

When you create a workspace from this template, you select:

1. **Languages** to pre-install (Python, Node.js, Go, Rust, Java, C/C++)
2. **Editors** to connect (VS Code in the browser, VS Code Desktop, Cursor, JetBrains, Zed, Windsurf)
3. **A Git repository** to clone (optional)

Coder provisions a workspace with your selections and you can start developing immediately.

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

## Architecture

This template provisions:

- **Docker container** (ephemeral) running Ubuntu with the Coder agent
- **Docker volume** (persistent) mounted at `/home/coder`

Files in your home directory persist across workspace restarts. Selected languages are installed on first start and cached for subsequent starts.

## Presets

Select a preset to auto-fill languages and editors for common workflows:

| Preset              | Languages           | Editors                             |
|---------------------|---------------------|-------------------------------------|
| **Web Development** | Python, Node.js     | VS Code (Browser)                   |
| **Backend (Go)**    | Go                  | VS Code (Browser), JetBrains GoLand |
| **Data Science**    | Python              | VS Code (Browser)                   |
| **Full Stack**      | Python, Node.js, Go | VS Code (Browser), Cursor           |

## IDE Notes

- **VS Code (Browser)**: Opens directly in your browser with no local install required.
- **VS Code Desktop, Cursor, Windsurf**: Require the desktop application installed on your local machine. Coder opens them via protocol handler.
- **JetBrains IDEs**: Filtered by your language selection (e.g. PyCharm for Python, GoLand for Go). Requires JetBrains Toolbox or Gateway on your local machine.
- **Zed**: Connects over SSH. Requires Zed installed on your local machine.
