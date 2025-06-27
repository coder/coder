# Envbuilder Dev Containers

A Development Container is an
[open-source specification](https://containers.dev/implementors/spec/) for
defining containerized development environments which are also called
development containers (dev containers).

Dev containers provide developers with increased autonomy and control over their
Coder cloud development environments.

By using dev containers, developers can customize their workspaces with tools
pre-approved by platform teams in registries like
[JFrog Artifactory](../../../integrations/jfrog-artifactory.md). This simplifies
workflows, reduces the need for tickets and approvals, and promotes greater
independence for developers.

This doc explains how to use Envbuilder to integrate dev containers in a template.

For the Docker-based Dev Containers integration, follow the [Configure a template for dev containers](../../extending-templates/devcontainers.md) documentation.

## Prerequisites

An administrator should construct or choose a base image and create a template
that includes an Envbuilder container image `coder/envbuilder` before a developer team configures dev containers.

## Benefits of Envbuilder

Key differences compared with the [Docker-based integration](../../extending-templates/devcontainers.md):

| Capability / Trait                       | Dev Containers integration (CLI-based)   | Envbuilder Dev Containers                 |
|------------------------------------------|------------------------------------------|-------------------------------------------|
| Build engine                             | `@devcontainers/cli` + Docker            | Envbuilder transforms the workspace image |
| Runs separate Docker container           | Yes (parent workspace + child container) | No (modifies the parent container)        |
| Multiple Dev Containers per workspace    | Yes                                      | No                                        |
| Rebuild when `devcontainer.json` changes | Yes (auto-prompt)                        | Limited (requires full workspace rebuild) |
| Docker required in workspace             | Yes                                      | No (works in restricted envs)             |
| Admin vs. developer control              | Developer decides per repo               | Platform admin manages via template       |
| Templates                                | Standard `devcontainer.json`             | Terraform + Envbuilder blocks             |
| Suitable for CI / AI agents              | Yes. Deterministic, composable           | Less ideal. No isolated container         |

Consult the full comparison at [Choose an approach to Dev Containers](../../extending-templates/dev-containers-envbuilder.md).

## Dev container Features

[Dev container Features](https://containers.dev/implementors/features/) allow
owners of a project to specify self-contained units of code and runtime
configuration that can be composed together on top of an existing base image.
This is a good place to install project-specific tools, such as
language-specific runtimes and compilers.

## Coder Envbuilder

[Envbuilder](https://github.com/coder/envbuilder/) is an open-source project
maintained by Coder that runs dev containers via Coder templates and your
underlying infrastructure. Envbuilder can run on Docker or Kubernetes.

It is independently packaged and versioned from the centralized Coder
open-source project. This means that Envbuilder can be used with Coder, but it
is not required. It also means that dev container builds can scale independently
of the Coder control plane and even run within a CI/CD pipeline.

## Next steps

- [Add an Envbuilder dev container template](./add-devcontainer.md)
- [Choose an approach to Dev Containers](../../extending-templates/dev-containers-envbuilder.md)
