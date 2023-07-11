# Devcontainers (alpha)

[Devcontainers](https://containers.dev) are an open source specification for defining development environments. [envbuilder](https://github.com/coder/envbuilder) is an open source project by Coder that runs devcontainers via Coder templates and your underlying infrastructure.

There are several benefits to adding a devcontainer-compatible template to Coder:

- Drop-in migration from Codespaces (or any existing repositories that use devcontainers)
- Easier to start projects from Coder (new workspace, pick starter devcontainer)
- Developer teams can "bring their own image." No need for platform teams to manage complex images, registries, and CI pipelines.

## How it works

- Coder admins add a devcontainer-compatible template to Coder (envbuilder can run on Docker or Kubernetes)

- Developers enter their repository URL as a [parameter](./parameters.md) when they create their workspace. [envbuilder](https://github.com/coder/envbuilder) clones the repo and builds a container from the `devcontainer.json` specified in the repo.

- Developers can edit the `devcontainer.json` in their workspace to rebuild to iterate on their development environments.

## Example templates

- [Docker](https://github.com/coder/coder/tree/main/examples/templates/devcontainer-docker)
- [Kubernetes](https://github.com/coder/coder/tree/main/examples/templates/devcontainer-kubernetes)

![Devcontainer parameter screen](../images/templates/devcontainers.png)

[Parameters](./parameters.md) can be used to prompt the user for a repo URL when they are creating a workspace.

## Authentication

You may need to authenticate to your container registry (e.g. Artifactory) or git provider (e.g. GitLab) to use envbuilder. Refer to the [envbuilder documentation](https://github.com/coder/envbuilder/) for more information.

## Caching

To improve build times, devcontainers can be cached. Refer to the [envbuilder documentation](https://github.com/coder/envbuilder/) for more information.

## Other features & known issues

Envbuilder is still under active development. Refer to the [envbuilder GitHub repo](https://github.com/coder/envbuilder/) for more information and to submit feature requests.
