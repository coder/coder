# Development containers on Coder

A development container (dev container or devcontainer) is an
[open source specification](https://containers.dev/implementors/spec/) for
defining containerized development environments.

Leverage Coder with dev containers and apply cloud-native security practices to
traditional ticket-ops and approval-ops workflows to help enable developers to
self-service.

## Benefits of devcontainers

There are several benefits to adding a devcontainer-compatible template to
Coder:

- Reliability and scalability
- Improved security
- Performance efficiency
- Cost Optimization

### Reliability and scalability

Envbuilder is an open source project independently packaged and versioned from
the centralized Coder open source project. This means that it can be used with
Coder, but it is not required. It also means that Dev Container builds can scale
independently of the Coder control plane and even run in CI/CD.

### Improved security

Since Coder and Envbuilder run on your own infrastructure, you can use firewalls
and cluster-level policies to ensure Envbuilder only downloads packages from
your secure registry powered by JFrog Artifactory or Sonatype Nexus.
Additionally, Envbuilder can be configured to push the full image back to your
registry for additional security scanning.

This means that Coder admins can still require hardened base images and
packages, while still allowing developer self service.

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

A development container

## Coder Envbuilder

Envbuilder is an open source project by Coder that runs dev containers via Coder
templates and your underlying infrastructure. It can run on Docker or
Kubernetes.

Envbuilder uses the Dev Container standard used in VS Code Local, Daytona,
DevPod, and Codespaces. This format is already familiar to developers and can
simplify migration. This allows developers to take control of their own
environments, while still following cloud-native security best practices. See
the
[Security section](./devcontainer-security-caching.md#devcontainer-security-and-caching)
for more information.

## Devcontainer Features

[Devcontainer Features](https://containers.dev/implementors/features/) allow
owners of a project to specify self-contained units of code and runtime
configuration that can be composed together on top of an existing base image.
This is a good place to install project-specific tools, such as
language-specific runtimes and compilers.

## Next steps

- [Add a devcontainer template](./add-devcontainer.md)
