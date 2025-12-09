# Envbuilder

Envbuilder is an open-source tool that builds development environments from
[dev container](https://containers.dev/implementors/spec/) configuration files.
Unlike the [Dev Containers integration](../integration.md),
Envbuilder transforms the workspace image itself rather than running containers
inside the workspace.

> [!NOTE]
>
> For most use cases, we recommend the
> [Dev Containers integration](../integration.md),
> which uses the standard `@devcontainers/cli` and Docker. Envbuilder is an
> alternative for environments where Docker is not available or for
> administrator-controlled dev container workflows.

Dev containers provide developers with increased autonomy and control over their
Coder cloud development environments.

By using dev containers, developers can customize their workspaces with tools
pre-approved by platform teams in registries like
[JFrog Artifactory](../../jfrog-artifactory.md). This simplifies
workflows, reduces the need for tickets and approvals, and promotes greater
independence for developers.

## Prerequisites

An administrator should construct or choose a base image and create a template
that includes a `devcontainer_builder` image before a developer team configures
dev containers.

## Devcontainer Features

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

- [Add an Envbuilder template](./add-envbuilder.md)
