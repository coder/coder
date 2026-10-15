---
display_name: Kubernetes (Rootless Podman)
description: Provision rootless Podman pods as Coder workspaces
icon: ../../../site/static/icon/k8s.png
maintainer_github: coder
verified: true
tags: [container, kubernetes, podman]
---

# Remote development on Kubernetes with rootless Podman

Provision workspaces that run containers with rootless Podman, without privileged pods or a custom container runtime.

The workspace pod runs as a non-root user.
Podman uses native kernel overlay storage, so the `/dev/fuse` device plugin is not required on nodes with kernel 5.13 or later.
The agent starts a Docker-compatible API socket for tools such as Testcontainers and Docker Compose.

## Prerequisites

- Nodes with `user.max_user_namespaces` set above `0`.
  Refer to [Docker in workspaces](https://coder.com/docs/admin/templates/extending-templates/docker-in-workspaces#rootless-podman) for per-platform instructions, including EKS Auto Mode.
  If only some nodes qualify, set the `node_selector` variable to schedule workspaces onto them.
- The Coder provisioner needs permission to create pods in the target namespace.

## Notes

- The Podman graph root is an `emptyDir` volume, so pulled images are lost when the workspace stops.
  Mount a persistent volume at `/home/coder` to keep them instead.
- One level of container nesting is supported.
  For dev containers, use the `docker-outside-of-docker` feature.
  Refer to [Docker in workspaces](https://coder.com/docs/admin/templates/extending-templates/docker-in-workspaces#nested-container-engines).
