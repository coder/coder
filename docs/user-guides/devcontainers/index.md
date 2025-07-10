# Dev Containers Integration

The dev containers integration enables seamless creation and management of dev containers in Coder workspaces.
This feature leverages the [`@devcontainers/cli`](https://github.com/devcontainers/cli) and [Docker](https://www.docker.com)
to provide a streamlined development experience.

## Prerequisites

- Coder version 2.24.0 or later
- Coder CLI version 2.24.0 or later
- A template with:
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
1. The integration detects repositories with a `.devcontainer` directory or a `devcontainer.json` file.
1. The integration builds (or rebuilds) and starts the dev container based on the configuration.
1. Your workspace automatically detects the running dev container.
1. If the configuration changes, the workspace prompts you to rebuild the dev container.

## Features

### Detection & Lifecycle

- Automatic discovery of `.devcontainer` configs.
- Change detection with rebuild prompts.
- Rebuild containers with one click from the Coder dashboard.

### Connectivity

- Full SSH access directly into dev containers (`coder ssh <agent>.<workspace>.me.coder`).
- Automatic port forwarding.

## Known Limitations

Currently, dev containers are not compatible with [prebuilt workspaces](../../admin/templates/extending-templates/prebuilt-workspaces.md).

If your template allows for prebuilt workspaces, do not select a prebuilt workspace if you plan to use a dev container.

## Next Steps

- [Dev Container specification](https://containers.dev/)
- [VS Code dev containers documentation](https://code.visualstudio.com/docs/devcontainers/containers)
- [Choose an approach to Dev Containers](../../admin/templates/extending-templates/dev-containers-envbuilder.md)
