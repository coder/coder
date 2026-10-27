---
title: Nix(OS)
---

> [!NOTE]
> Nix(OS) support is in alpha as an [early access](../../reference/feature-stages.md#early-access-features) feature.
> Expect breaking changes, and report issues on [GitHub](https://github.com/coder/registry/issues).

[Nix](https://nixos.org) defines development environments as code.
A [flake](https://nix.dev/concepts/flakes) committed to Git declares the tools a project needs, and its `flake.lock` pins every dependency, so every workspace builds the same environment.

Nix and NixOS solve different problems, and you can use either or both, in containers or VMs:

- **Nix for each repository's tech stack**: developers declare the languages and tools a project needs in a dev shell in the project's repository.
  Nix runs on your existing base image, such as Debian or Ubuntu, so the template doesn't change when a project's stack does.
  This is the closest equivalent to a `devcontainer.json`.
- **NixOS for the template's base image**: template administrators define the workspace operating system, including system packages, services, and users, in a flake instead of maintaining an image.

The two combine: a NixOS template can also open each repository's dev shell.

## Why Nix instead of dev containers

- **Reproducible**: `flake.lock` pins every package, so workspaces resolve identical versions.
- **No image builds**: environments come from the flake, not from building a container image per project.
  You don't need a Docker daemon, Docker-in-Docker, or privileged containers to use them.
- **Fast updates**: packages come from binary caches such as `cache.nixos.org`, and the Nix store can persist across workspace restarts.
- **Safe changes (NixOS)**: a configuration that fails to build leaves the running system unchanged, and you can roll back to a previous generation.

## Nix for each repository's tech stack

Add the [Nix(OS) module](https://registry.coder.com/modules/coder-labs/nix) to any template whose image already has Nix installed.
It adds a **Nix Develop** app and prebuilds the project's dev shell at workspace startup:

```tf
module "nix" {
  source   = "registry.coder.com/coder-labs/nix/coder"
  version  = "1.0.0"
  agent_id = coder_agent.main.id
  nixos    = { enabled = false }
  develop  = { enabled = true, path = "~/project" }
}
```

The module does not install Nix; include it in your image, for example with the [Nix installer](https://nixos.org/download/).

## NixOS for the template's base image

The [AWS EC2 (NixOS)](https://registry.coder.com/templates/coder-labs/aws-nixos) template runs each workspace as a NixOS virtual machine configured from your flake.
On each boot, it syncs the flake into `/etc/nixos` and runs `nixos-rebuild switch` before the Coder agent starts.
Rebuild progress streams to the **NixOS** log in the workspace dashboard.
Local edits and commits in `/etc/nixos` are preserved, so developers can iterate on the configuration inside the workspace:

```console
sudo nixos-rebuild switch --flake /etc/nixos#coder-workspace-ec2-x86_64
```

To get started:

1. Fork the [example flake](https://github.com/coder/nixos-example-flake), edit `configuration.nix`, and commit your changes with `flake.lock`.
1. Create a template from [AWS EC2 (NixOS)](https://registry.coder.com/templates/coder-labs/aws-nixos) and set `flake_ref` to your fork's Git URL.
1. Create a workspace.

To use an existing flake, import `github:coder/nixos-modules` and an EC2 hardware module.
See the template README for prerequisites and limitations.

For NixOS on other infrastructure, use the template as a reference and wire the [Nix(OS) module](https://registry.coder.com/modules/coder-labs/nix)'s boot script into your own template.
