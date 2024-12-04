# Dev containers

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

## Prerequisites

An administrator should construct or choose a base image and create a template
that includes a `devcontainer_builder` image before a developer team configures
dev containers.

## Benefits of devcontainers

There are several benefits to adding a dev container-compatible template to
Coder:

- Reliability through standardization
- Scalability for growing teams
- Improved security
- Performance efficiency
- Cost Optimization

### Reliability through standardization

Use dev containers to empower development teams to personalize their own
environments while maintaining consistency and security through an approved and
hardened base image.

Standardized environments ensure uniform behavior across machines and team
members, eliminating "it works on my machine" issues and creating a stable
foundation for development and testing. Containerized setups reduce dependency
conflicts and misconfigurations, enhancing build stability.

### Scalability for growing teams

Dev containers allow organizations to handle multiple projects and teams
efficiently.

You can leverage platforms like Kubernetes to allocate resources on demand,
optimizing costs and ensuring fair distribution of quotas. Developer teams can
use efficient custom images and independently configure the contents of their
version-controlled dev containers.

This approach allows organizations to scale seamlessly, reducing the maintenance
burden on the administrators that support diverse projects while allowing
development teams to maintain their own images and onboard new users quickly.

### Improved security

Since Coder and Envbuilder run on your own infrastructure, you can use firewalls
and cluster-level policies to ensure Envbuilder only downloads packages from
your secure registry powered by JFrog Artifactory or Sonatype Nexus.
Additionally, Envbuilder can be configured to push the full image back to your
registry for additional security scanning.

This means that Coder admins can require hardened base images and packages,
while still allowing developer self-service.

Envbuilder runs inside a small container image but does not require a Docker
daemon in order to build a dev container. This is useful in environments where
you may not have access to a Docker socket for security reasons, but still need
to work with a container.

### Performance efficiency

Create a unique image for each project to reduce the dependency size of any
given project.

Envbuilder has various caching modes to ensure workspaces start as fast as
possible, such as layer caching and even full image caching and fetching via the
[Envbuilder Terraform provider](https://registry.terraform.io/providers/coder/envbuilder/latest/docs).

### Cost optimization

By creating unique images per-project, you remove unnecessary dependencies and
reduce the workspace size and resource consumption of any given project. Full
image caching ensures optimal start and stop times.

## When to use a dev container

Dev containers are a good fit for developer teams who are familiar with Docker
and are already using containerized development environments. If you have a
large number of projects with different toolchains, dependencies, or that depend
on a particular Linux distribution, dev containers make it easier to quickly
switch between projects.

They may also be a great fit for more restricted environments where you may not
have access to a Docker daemon since it doesn't need one to work.

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

- [Add a dev container template](./add-devcontainer.md)
