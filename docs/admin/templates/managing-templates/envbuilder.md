---
title: Envbuilder
---

> [!WARNING]
> Envbuilder is in **maintenance mode** with no new features planned.
>
> The recommended way to use dev containers in Coder is the built-in [Dev Containers Integration](../../integrations/devcontainers/integration.md).
> To find the best approach for your deployment, join our [community](https://coder.com/community) to be guided towards the best approach.

Envbuilder shifts environment definition from template administrators to developers.
Instead of baking tools into template images, developers define their environments via `devcontainer.json` files in their repositories.

Envbuilder transforms the workspace image itself from a dev container configuration, without requiring a Docker daemon.

For setup instructions, see [Envbuilder documentation](../../integrations/devcontainers/envbuilder/index.md).

> [!TIP]
> Nix flakes are an alternate way to handle dependency management which lets developers and/or template administrators define environments and manage dependencies at a much deeper level, without requiring a container runtime.
>
> To use them in Coder with or without NixOS, consult the [Nix(OS)](../../integrations/nix.md) page.
