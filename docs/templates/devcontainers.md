# Devcontainers (alpha)

[Devcontainers](https://containers.dev) are an open source specification for defining development environments. [envbuilder](https://github.com/coder/coder) is an open source project by Coder that runs devcontainers via Coder templates and your underlying infrastructure.

There are several benefits to adding a devcontainer-compatible template to Coder:

- Drop-in migration from Codespaces (or any existing repositories that use devcontainers)
- Easier to start projects from Coder (new workspace, pick starter devcontainer)
- Developer teams can "bring their own image." No need for platform teams to manage complex images, registries, and CI pipelines.

## How it works

- Coder admins add a devcontainer-compatible template to Coder (envbuilder can run on Docker or Kubernetes)

- Developers enter their repository URL as a [parameter](./parameters.md) when creating workspaces. [envbuilder](https://github.com/coder/envbuilder) clones the repo and builds a container from the `devcontainer.json` specified in the repo.

- Developers can edit the `devcontainer.json` in their workspace to rebuild to iterate on their development environments.



## Caching
