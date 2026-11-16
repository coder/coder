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

- The home directory is a persistent volume, so cloned repositories, pulled images, and build cache survive workspace restarts.
  The volume must be ext4 or xfs for native overlay storage, which most default storage classes provide.
- If Podman can't start its API socket, for example because user namespaces are disabled on the node, the startup script fails and prints the Podman error to the startup logs.
- One level of container nesting is supported.
  For dev containers, use the `docker-outside-of-docker` feature.
  Refer to [Docker in workspaces](https://coder.com/docs/admin/templates/extending-templates/docker-in-workspaces#nested-container-engines).
