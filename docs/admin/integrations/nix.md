---
title: Nix(OS)
---

> [!NOTE]
> Nix(OS) support is in alpha as an [early access](../../reference/feature-stages.md#early-access-features) feature.
> Expect breaking changes, and report issues on [GitHub](https://github.com/coder/registry/issues).

[Nix](https://nixos.org) defines development environments as code.
A [flake](https://nix.dev/concepts/flakes) committed to Git declares the tools a project needs, and its `flake.lock` pins every dependency, so every workspace builds the same environment.

Nix and NixOS solve different problems, and you can use either or both, in containers or VMs:

- **Nix dev shells for each repository's tech stack**: developers declare the languages and tools a project needs in a dev shell in the project's repository.
  Nix runs on your existing base image, such as Debian or Ubuntu, so the template doesn't change when a project's stack does.
  This is the closest equivalent to a `devcontainer.json`.
- **NixOS for the template's base image**: template administrators define the workspace operating system, including system packages, services, and users, in a flake instead of maintaining an image.

The two combine: a NixOS template can also open each repository's dev shell.

## Why Nix instead of dev containers

- **Reproducible**: `flake.lock` pins every package, so workspaces resolve identical versions.
- **Flexible**: environments come from the flake, can be used in VMs, containers or even locally, unlike Dev Containers which has a hard requirement for container-based workflows.
- **Fast updates**: packages come from binary caches such as `cache.nixos.org`, and the Nix store can persist across workspace restarts.
- **Safe changes (NixOS)**: a configuration that fails to build leaves the running system unchanged, and you can roll back to a previous generation.

## Nix develop shells (repository/project level, for developers)

Add the [Nix(OS) module](https://registry.coder.com/modules/coder-labs/nix) to any template whose image already has Nix installed.
It adds a **Nix Develop** app and prebuilds the project's dev shell at workspace startup:

```tf
module "nix" {
  source   = "registry.coder.com/coder-labs/nix/coder"
  version  = "1.0.0"
  agent_id = coder_agent.main.id
  develop  = { enabled = true, path = "~/project" }
}
```

> [!WARNING]
> The module does not install Nix; include it in your image.
> For example with the official [Nix](https://nixos.org/download/) installer, or with the [Lix](https://lix.systems/install/) distribution/fork's installer for a more modern experience.

## NixOS (template level, for administrators)

A NixOS template boots each workspace from a NixOS image and applies your flake with `nixos-rebuild switch` before the Coder agent starts.
It works on any infrastructure that can boot NixOS and run a script as root at boot, for example through cloud-init or instance user data.

To build a NixOS template:

1. Write a flake with a `nixosConfigurations` entry for each workspace host.
   Import [`github:coder/nixos-modules`](https://github.com/coder/nixos-modules), which runs the Coder agent as a systemd unit and creates the workspace user, then add your platform's hardware configuration.
   The [example flake](https://github.com/coder/nixos-example-flake) is a good starting point.
1. Add the [Nix(OS) module](https://registry.coder.com/modules/coder-labs/nix) to your template and point it at your flake:

   ```tf
   module "nix" {
     source   = "registry.coder.com/coder-labs/nix/coder"
     version  = "1.0.0"
     agent_id = coder_agent.main.id
     nixos = {
       flake_ref  = "https://github.com/example-org/workspace-flake"
       flake_attr = "my-workspace"
     }
   }
   ```

1. Pass the module's `boot_script` output to your platform's boot hook so it runs as root before the agent starts.
   The boot hook must also hand the agent token to the agent unit, as described in [the agent handoff](https://github.com/coder/nixos-modules#the-agent-handoff).

Rebuild logs are written to `/var/log/coder-nixos/rebuild-latest.log` in the workspace.

## Ready-made templates

These registry templates wire up NixOS for a specific platform.
Use them directly, or as a reference for your own template.

### AWS EC2

The [AWS EC2 (NixOS)](https://registry.coder.com/templates/coder-labs/aws-nixos) template runs each workspace as a NixOS virtual machine on EC2.
Rebuild progress streams to the **NixOS** log in the workspace dashboard.

1. Fork the [example flake](https://github.com/coder/nixos-example-flake), edit `configuration.nix`, and commit your changes with `flake.lock`.
1. Create a template from [AWS EC2 (NixOS)](https://registry.coder.com/templates/coder-labs/aws-nixos) and set `flake_ref` to your fork's Git URL.
1. Create a workspace.

See the template README for prerequisites and limitations.

## Caching

Nix downloads prebuilt packages from binary caches, `cache.nixos.org` by default, instead of building them in the workspace.
To add your own cache, set `extra-substituters` and `extra-trusted-public-keys` in the module's [Nix settings](https://registry.coder.com/modules/coder-labs/nix#nix-settings).
To replace the default caches instead, set `substituters` and `trusted-public-keys`.
On NixOS, also set them in your flake's `nix.settings` so they apply beyond the boot rebuild.

To host your own cache, follow the nix.dev [binary cache tutorial](https://nix.dev/tutorials/nixos/binary-cache-setup.html).
Its [Alternatives](https://nix.dev/tutorials/nixos/binary-cache-setup.html#alternatives) section lists ready-made options, such as hosted and S3-backed caches.
