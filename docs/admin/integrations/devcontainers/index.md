# Dev Containers

Dev containers allow developers to define their development environment
as code using the [Dev Container specification](https://containers.dev/).
Configuration lives in a `devcontainer.json` file alongside source code,
enabling consistent, reproducible environments.

By adopting dev containers, organizations can:

- **Standardize environments**: Eliminate "works on my machine" issues while
  still allowing developers to customize their tools within approved boundaries.
- **Scale efficiently**: Let developers maintain their own environment
  definitions, reducing the burden on platform teams.
- **Improve security**: Use hardened base images and controlled package
  registries to enforce security policies while enabling developer self-service.

Coder supports two approaches for running dev containers. Choose based on your
infrastructure and workflow requirements.

## Dev Containers Integration

The Dev Containers Integration uses the standard `@devcontainers/cli` and Docker
to run containers inside your workspace. This is the recommended approach for
most use cases.

**Best for:**

- Workspaces with Docker available (Docker-in-Docker or mounted socket)
- Dev container management in the Coder dashboard (discovery, status, rebuild)
- Multiple dev containers per workspace

[Configure Dev Containers Integration](./integration.md)

For user documentation, see the
[Dev Containers user guide](../../../user-guides/devcontainers/index.md).

## Envbuilder

Envbuilder transforms the workspace image itself from a `devcontainer.json`,
rather than running containers inside the workspace. It does not require
a Docker daemon.

**Best for:**

- Environments where Docker is unavailable or restricted
- Infrastructure-level control over image builds, caching, and security scanning
- Kubernetes-native deployments without privileged containers

[Configure Envbuilder](./envbuilder/index.md)
