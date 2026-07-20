---
display_name: Docker Containers
description: Provision Docker containers as Coder workspaces
icon: ../../../site/static/icon/docker.png
maintainer_github: coder
verified: true
tags: [docker, container]
---

# Remote Development on Docker Containers

Provision Docker containers as [Coder workspaces](https://coder.com/docs/user-guides/workspace-management) with this example template.

<!-- TODO: Add screenshot -->

<!-- prerequisites:start -->

## Prerequisites

### Workspace image

The container image determines what tools, languages, and runtimes are available in the workspace out of the box, so it has a major impact on the developer experience.

Some options to consider:

- [`codercom/example-base:ubuntu`](https://github.com/coder/images/tree/main/images/base) (default): minimal and lightweight, but may not include many tools developers expect by default
- [`codercom/example-universal:ubuntu`](https://github.com/coder/images/tree/main/images/universal): catch-all image with many languages and tools available, but larger and slower to pull

More language-specific images (Go, Java, Node.js, and more) are available in [coder/images](https://github.com/coder/images), and the [devcontainers/images](https://github.com/devcontainers/images) collection is another good source of ready-made development images.
You can also build your own image to pre-bake the exact tools your team needs.
Visit [Coder's image management docs](https://coder.com/docs/admin/templates/managing-templates/image-management) for additional guidance.

### Infrastructure

The VM you run Coder on must have a running Docker socket and the `coder` user must be added to the Docker group:

```sh
# Add coder user to Docker group
sudo adduser coder docker

# Restart Coder server
sudo systemctl restart coder

# Test Docker
sudo -u coder docker ps
```

<!-- prerequisites:end -->

## Architecture

This template provisions the following resources:

- Docker image (built by Docker socket and kept locally)
- Docker container pod (ephemeral)
- Docker volume (persistent on `/home/coder`)

This means, when the workspace restarts, any tools or files outside of the home directory are not persisted. To pre-bake tools into the workspace (e.g. `python3`), modify the container image. Alternatively, individual developers can [personalize](https://coder.com/docs/user-guides/workspace-dotfiles) their workspaces with dotfiles.

> **Note**
> This template is designed to be a starting point! Edit the Terraform to extend the template to support your use case.

### Editing the image

Edit the `Dockerfile` and run `coder templates push` to update workspaces.
