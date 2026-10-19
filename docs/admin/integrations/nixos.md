---
title: Nix(OS)
---

[NixOS](https://nixos.org) workspaces define the entire operating system as code, with no container images.
Developers commit a [flake](https://nix.dev/concepts/flakes) to Git, and every workspace builds the same pinned system from it.

The [AWS EC2 (NixOS) template](https://registry.coder.com/templates/coder-labs/aws-nixos) runs each workspace as a NixOS virtual machine configured from your flake.

## Why NixOS instead of dev containers

- **Reproducible**: `flake.lock` pins every package, so workspaces resolve identical versions.
- **No container tooling**: no image builds, Docker daemon, Docker-in-Docker, or privileged containers.
- **Fast updates**: packages come from binary caches such as `cache.nixos.org` instead of image rebuilds, and the Nix store persists across workspace restarts.
- **Safe changes**: a configuration that fails to build leaves the running system unchanged, and you can roll back to a previous generation.
- **Full-OS configuration**: services, users, and system settings live in the same flake as developer tools.

## How it works

On each boot, the template syncs your flake into `/etc/nixos` and runs `nixos-rebuild switch` before the Coder agent starts.
Rebuild progress streams to the **NixOS** log in the workspace dashboard.
Local edits and commits in `/etc/nixos` are preserved, so developers can iterate on the configuration inside the workspace:

```console
sudo nixos-rebuild switch --flake /etc/nixos#coder-workspace-ec2-x86_64
```

## Get started

1. Fork the [example flake](https://github.com/coder/nixos-example-flake), edit `configuration.nix`, and commit your changes with `flake.lock`.
1. Create a template from [AWS EC2 (NixOS)](https://registry.coder.com/templates/coder-labs/aws-nixos) and set `flake_ref` to your fork's Git URL.
1. Create a workspace.

To use an existing flake, import `github:coder/nixos-modules` and an EC2 hardware module.
See the template README for details, prerequisites, and limitations.

## Other infrastructure

The rebuild logic lives in the [Nix(OS) module](https://registry.coder.com/modules/coder-labs/nix), which you can wire into your own NixOS templates on other platforms.
The module can also add a `nix develop` shell to existing Docker, Kubernetes, or VM templates whose images already include Nix.
