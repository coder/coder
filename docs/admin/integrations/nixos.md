---
title: Nix(OS)
---

[Nix](https://nixos.org) defines development environments as code.
A [flake](https://nix.dev/concepts/flakes) committed to Git declares the tools a project needs, and its `flake.lock` pins every dependency, so every workspace builds the same environment.

You can use Nix in two ways, on any infrastructure, including containers and VMs:

- **Nix**: the package manager installed on an existing distribution, such as a Debian or Ubuntu image.
  Developers get a project's `nix develop` shell, and you keep your current base image and template.
- **NixOS**: an operating system built entirely from a flake, including services, users, and system settings.

## Why Nix instead of dev containers

- **Reproducible**: `flake.lock` pins every package, so workspaces resolve identical versions.
- **No image builds**: environments come from the flake, not from building a container image per project.
  You don't need a Docker daemon, Docker-in-Docker, or privileged containers to use them.
- **Fast updates**: packages come from binary caches such as `cache.nixos.org`, and the Nix store can persist across workspace restarts.
- **Safe changes (NixOS)**: a configuration that fails to build leaves the running system unchanged, and you can roll back to a previous generation.

## Nix on an existing distribution

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

## NixOS workspaces

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
