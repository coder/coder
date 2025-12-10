# Envbuilder

Envbuilder is an open-source tool that builds development environments from
[dev container](https://containers.dev/implementors/spec/) configuration files.
Unlike the [Dev Containers integration](../integration.md),
Envbuilder transforms the workspace image itself rather than running containers
inside the workspace.

Envbuilder is well-suited for Kubernetes-native deployments without privileged
containers, environments where Docker is unavailable or restricted, and
workflows where administrators need infrastructure-level control over image
builds, caching, and security scanning. For workspaces with Docker available,
the [Dev Containers Integration](../integration.md) offers container management
with dashboard visibility and multi-container support.

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
