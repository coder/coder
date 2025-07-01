# Dev Containers Integration

> [!NOTE]
>
> The Coder dev containers integration is an [early access](../../install/releases/feature-stages.md) feature.
>
> While functional for testing and feedback, it may change significantly before general availability.

The dev containers integration is an early access feature that enables seamless
creation and management of dev containers in Coder workspaces. This feature
leverages the [`@devcontainers/cli`](https://github.com/devcontainers/cli) and
[Docker](https://www.docker.com) to provide a streamlined development
experience.

This implementation is different from the existing
[Envbuilder-based dev containers](../../admin/templates/managing-templates/devcontainers/index.md)
offering.

## Prerequisites

- Coder version 2.22.0 or later
- Coder CLI version 2.22.0 or later
- A template with:
  - Dev containers integration enabled
  - A Docker-compatible workspace image
- Appropriate permissions to execute Docker commands inside your workspace

## How It Works

The dev containers integration utilizes the `devcontainer` command from
[`@devcontainers/cli`](https://github.com/devcontainers/cli) to manage dev
containers within your Coder workspace.
This command provides comprehensive functionality for creating, starting, and managing dev containers.

Dev environments are configured through a standard `devcontainer.json` file,
which allows for extensive customization of your development setup.

When a workspace with the dev containers integration starts:

1. The workspace initializes the Docker environment.
1. The integration detects repositories with a `.devcontainer` directory or a
   `devcontainer.json` file.
1. The integration builds and starts the dev container based on the
   configuration.
1. Your workspace automatically detects the running dev container.

## Features

### Available Now

- Automatic dev container detection from repositories
- Seamless dev container startup during workspace initialization
- Integrated IDE experience in dev containers with VS Code
- Direct service access in dev containers
- Limited SSH access to dev containers

### Coming Soon

- Dev container change detection
- On-demand dev container recreation
- Support for automatic port forwarding inside the container
- Full native SSH support to dev containers

## Limitations during Early Access

During the early access phase, the dev containers integration has the following
limitations:

- Changes to the `devcontainer.json` file require manual container recreation
- Automatic port forwarding only works for ports specified in `appPort`
- SSH access requires using the `--container` flag
- Some devcontainer features may not work as expected

These limitations will be addressed in future updates as the feature matures.

## Comparison with Envbuilder-based Dev Containers

| Feature        | Dev Containers (Early Access)          | Envbuilder Dev Containers                    |
|----------------|----------------------------------------|----------------------------------------------|
| Implementation | Direct `@devcontainers/cli` and Docker | Coder's Envbuilder                           |
| Target users   | Individual developers                  | Platform teams and administrators            |
| Configuration  | Standard `devcontainer.json`           | Terraform templates with Envbuilder          |
| Management     | User-controlled                        | Admin-controlled                             |
| Requirements   | Docker access in workspace             | Compatible with more restricted environments |

Choose the appropriate solution based on your team's needs and infrastructure
constraints. For additional details on Envbuilder's dev container support, see
the
[Envbuilder devcontainer spec support documentation](https://github.com/coder/envbuilder/blob/main/docs/devcontainer-spec-support.md).

## Next Steps

- Explore the [dev container specification](https://containers.dev/) to learn
  more about advanced configuration options
- Read about [dev container features](https://containers.dev/features) to
  enhance your development environment
- Check the
  [VS Code dev containers documentation](https://code.visualstudio.com/docs/devcontainers/containers)
  for IDE-specific features
