# Devcontainers (alpha)

[Devcontainers](https://containers.dev) are an open source specification for defining development environments. With [envbuilder](https://github.com/coder/coder), an open source project by Coder, you can support devcontainers in your Coder templates. There are several benefits to this:

- Drop-in migration from Codespaces or any repositories that use devcontainers
- Developer teams can manage their own images/dependencies in the project repository without relying on an image registry, CI pipelines, or manual effort from platform teams

## How it works

- Coder admins add a devcontainer-compatible template to Coder (this can run on VMs, Docker, or Kubernetes)

- Developers enter their repository URL as a [parameter](./parameters.md) when creating workspaces. [envbuilder](https://github.com/coder/envbuilder) clones the repo and builds a container from the `devcontainer.json` specified in the repo.

- Developers can edit the `devcontainer.json` in their workspace and rebuild to iterate on their development environment.

## Caching
