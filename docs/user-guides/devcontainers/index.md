# Dev Containers Integration

The Dev Containers integration enables seamless creation and management of Dev
Containers in Coder workspaces. This feature leverages the
[`@devcontainers/cli`](https://github.com/devcontainers/cli) and
[Docker](https://www.docker.com) to provide a streamlined development
experience.

This implementation is different from the existing
[Envbuilder-based Dev Containers](../../admin/templates/managing-templates/devcontainers/index.md)
offering.

## Prerequisites

- Coder version 2.24.0 or later
- Coder CLI version 2.24.0 or later
- **Linux or macOS workspace**, Dev Containers are not supported on Windows
- A template with:
  - Dev Containers integration enabled
  - A Docker-compatible workspace image
- Appropriate permissions to execute Docker commands inside your workspace

## How It Works

The Dev Containers integration utilizes the `devcontainer` command from
[`@devcontainers/cli`](https://github.com/devcontainers/cli) to manage Dev
Containers within your Coder workspace.
This command provides comprehensive functionality for creating, starting, and managing Dev Containers.

Dev environments are configured through a standard `devcontainer.json` file,
which allows for extensive customization of your development setup.

When a workspace with the Dev Containers integration starts:

1. The workspace initializes the Docker environment.
1. The integration detects repositories with a `.devcontainer` directory or a
   `devcontainer.json` file.
1. The integration builds and starts the Dev Container based on the
   configuration.
1. Your workspace automatically detects the running Dev Container.

## Features

### Available Now

- Automatic Dev Container detection from repositories
- Seamless Dev Container startup during workspace initialization
- Dev Container change detection and dirty state indicators
- On-demand Dev Container recreation via rebuild button
- Integrated IDE experience in Dev Containers with VS Code
- Direct service access in Dev Containers
- SSH access to Dev Containers
- Automatic port detection for container ports

## Limitations

The Dev Containers integration has the following limitations:

- **Not supported on Windows**
- Changes to the `devcontainer.json` file require manual container recreation
  using the rebuild button
- Some Dev Container features may not work as expected

## Comparison with Envbuilder-based Dev Containers

| Feature        | Dev Containers Integration             | Envbuilder Dev Containers                    |
|----------------|----------------------------------------|----------------------------------------------|
| Implementation | Direct `@devcontainers/cli` and Docker | Coder's Envbuilder                           |
| Target users   | Individual developers                  | Platform teams and administrators            |
| Configuration  | Standard `devcontainer.json`           | Terraform templates with Envbuilder          |
| Management     | User-controlled                        | Admin-controlled                             |
| Requirements   | Docker access in workspace             | Compatible with more restricted environments |

Choose the appropriate solution based on your team's needs and infrastructure
constraints. For additional details on Envbuilder's Dev Container support, see
the
[Envbuilder Dev Container spec support documentation](https://github.com/coder/envbuilder/blob/main/docs/devcontainer-spec-support.md).

## Next Steps

- Explore the [Dev Container specification](https://containers.dev/) to learn
  more about advanced configuration options
- Read about [Dev Container features](https://containers.dev/features) to
  enhance your development environment
- Check the
  [VS Code dev containers documentation](https://code.visualstudio.com/docs/devcontainers/containers)
  for IDE-specific features
