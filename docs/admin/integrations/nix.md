---
title: Nix(OS)
---

> [!NOTE]
> Nix(OS) support is in alpha as an [early access](../../reference/feature-stages.md#early-access-features) feature.
> Expect breaking changes, and report issues on [GitHub](https://github.com/coder/registry/issues).

[Nix](https://nixos.org) defines development environments as code via [flakes](https://nix.dev/concepts/flakes) committed to Git.
Flakes declares the tools a project needs and their configuration, its `flake.lock` pins every dependency, and can even set OS-level settings on NixOS so every workspace builds the same environment.

Nix and NixOS solve different problems, and you can use either or both, in containers or VMs:

- **Nix develop shells for each repository's tech stack**: **developers** declare the languages and tools a project needs in a dev shell in the project's repository.
  Nix runs on your existing base image, such as Debian or Ubuntu, so the template doesn't change when a project's stack does.
  This is the closest equivalent to a `devcontainer.json`.
- **NixOS for the template's base image**: **template administrators** define the workspace operating system, including system packages, services, and users, in a flake instead of maintaining an image.

The two can combine: a NixOS template can also open each repository's dev shell.

## Why Nix instead of Dev Containers

- **Reproducible**: `flake.lock` pins every package, so workspaces resolve identical versions.
- **Flexible**: environments come from the flake and work in VMs, containers, or even locally, unlike Dev Containers, which require container-based workflows.
- **Configurability**: package options and settings live in your flake, so you can declaratively manage more than dependencies (for example, kernel versions and modules, partition schemes, VS Code settings and extensions, and desktop environment configuration).
- **Fast updates**: packages come from centralized binary caches (such as `cache.nixos.org`), and the Nix store can persist across workspace restarts.
- **Safe changes (NixOS)**: a configuration that fails to build leaves the running system unchanged, and you can roll back to a previous generation.

## Nix develop shells (repository/project level, for developers)

Add the [Nix(OS) module](https://registry.coder.com/modules/coder-labs/nix) to any template whose image already has Nix installed.
It adds a **Nix Develop** app and prebuilds the project's dev shell at workspace startup:

```tf
module "nix" {
  source   = "registry.coder.com/coder-labs/nix/coder"
  version  = "1.0.0"
  agent_id = coder_agent.main.id

  # This is the important Nix Develop-specific part
  develop  = { enabled = true, path = "~/project" }
}
```

> [!WARNING]
> The module does not install Nix and assumes it is available in the PATH; include it in your image in case you are not running NixOS,.
> 
> To install the `nix` CLI, you can use the official [Nix](https://nixos.org/download/) installer, or the installer for the [Lix](https://lix.systems/install/) Nix fork, for a more modern experience.

## NixOS (template level, for administrators)

A NixOS template boots each workspace from a NixOS VM image and applies the provided flake (`nixos.flake_ref`) via `nixos-rebuild switch` before the Coder agent starts.
It works on any infrastructure that can boot NixOS and run a script as root at boot, for example through cloud-init or instance user data. Specifics depend on your cloud provider.

To build a NixOS template:

1. Write or modify your existing flake with a specific `nixosConfigurations` entry for your Coder workspaces.
   The [example flake](https://github.com/coder/nixos-example-flake) is a good starting point.

   If you are building your own flake, import [`github:coder/nixos-modules`](https://github.com/coder/nixos-modules), which runs the Coder agent as a SystemD unit and creates the workspace user, then add your platform's hardware configuration.

1. Add the [Nix(OS) module](https://registry.coder.com/modules/coder-labs/nix) to your template and point it at your flake:

   ```tf
   module "nix" {
     source   = "registry.coder.com/coder-labs/nix/coder"
     version  = "1.0.0"
     agent_id = coder_agent.main.id

     # This is the important NixOS-specific part
     nixos = {
       flake_ref  = "https://github.com/example-org/workspace-flake"
       flake_attr = "my-workspace"
     }
   }
   ```

1. Pass the module's `boot_script` output (e.g `module.nix.boot_script`) to your platform's boot hook (for example, cloud-init) so it runs as root before the agent starts.
   The boot hook must also hand the agent token to the agent unit, as described in [the agent handoff](https://github.com/coder/nixos-modules#the-agent-handoff).

Rebuild logs are written to `/var/log/coder-nixos/rebuild-latest.log` in the workspace.

> [!TIP]
> This can be tricky for new Coder administrators.
> If you're stuck or want to share feedback, join our [community](https://coder.com/community) for help.

### Ready-made templates

These registry templates wire up NixOS for a specific platform.
Use them directly, or as a reference for your own template.

The Coder team is actively working on adding new templates to the registry, but feel free to [contribute](https://github.com/coder/registry).

#### AWS EC2

The [AWS EC2 (NixOS)](https://registry.coder.com/templates/coder-labs/aws-nixos) template runs each workspace as a NixOS virtual machine on EC2.
Rebuild progress streams to the **NixOS** log in the workspace dashboard.
Its `flake_attr` supports `$ARCH`, which the template replaces with `x86_64` or `aarch64` to match the instance type.

1. Fork the [example flake](https://github.com/coder/nixos-example-flake), edit `configuration.nix`, and commit your changes with `flake.lock`.
1. Create a template from [AWS EC2 (NixOS)](https://registry.coder.com/templates/coder-labs/aws-nixos) and set `flake_ref` to your fork's Git URL.
1. Create a workspace.

See the template README for prerequisites and limitations.

## Caching

### Binary caches

Nix downloads prebuilt packages from binary caches (`cache.nixos.org` by default) instead of building them in the workspace.
Public caches are shared infrastructure and don't have every package, so some packages still build locally.
We recommend setting up your own cache server in each region where you are deploying workspaces to ensure the fastest download speeds possible.

To add your own cache, set `extra-substituters` and `extra-trusted-public-keys` in the module's [Nix settings](https://registry.coder.com/modules/coder-labs/nix#nix-settings).
To replace the default caches instead, set `substituters` and `trusted-public-keys`.

On NixOS, also set these options in your flake's `nix.settings` so they apply beyond the boot rebuild.

To host your own cache for faster download speeds and full coverage of packages, follow the nix.dev [binary cache tutorial](https://nix.dev/tutorials/nixos/binary-cache-setup.html).
Its [Alternatives](https://nix.dev/tutorials/nixos/binary-cache-setup.html#alternatives) section lists ready-made options, such as hosted and S3-backed caches.

### Other

Other caching methods are available such as prebuilding your Nix image in CI (if you use containers) or using a base disk that you refresh frequently.

If you are looking for the most time-efficient way to improve startup times, set up a local binary cache server first, then combine it with other improvements later.

In most cases, having a binary cache server is preferable either way, because users will still be fetching their own packages (via `nix develop` or other).
