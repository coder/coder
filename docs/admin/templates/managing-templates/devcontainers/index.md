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

Envbuilder is a legacy implementation of dev containers.

For the Docker-based Dev Containers integration, follow the [Configure a template for dev containers](../../extending-templates/devcontainers.md) documentation instead.

## Prerequisites

An administrator should construct or choose a base image and create a template
that includes an Envbuilder container image `coder/envbuilder` before a developer team configures dev containers.

Compare the differences between [Envbuilder and the Dev Containers integration](../../extending-templates/dev-containers-envbuilder.md).

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

## Next Steps

- [Add an Envbuilder dev container template](./add-devcontainer.md)
- [Choose an approach to Dev Containers](../../extending-templates/dev-containers-envbuilder.md)
