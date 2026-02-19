# Dev Containers (via `@devcontainers/cli` CLI)

[Dev containers](https://containers.dev/) define your development environment
as code using a `devcontainer.json` file. Coder's Dev Containers integration
uses the [`@devcontainers/cli`](https://github.com/devcontainers/cli) and
[Docker](https://www.docker.com) to seamlessly build and run these containers,
with management in your dashboard.

This guide covers the Dev Containers CLI integration which requires Docker. For workspaces without Docker in them,
administrators can look into
[other options](../../admin/integrations/devcontainers/index.md#comparison) instead.

![Two Dev Containers running as sub-agents in a Coder workspace](../../images/user-guides/devcontainers/devcontainer-running.png)_Dev containers appear as sub-agents with their own apps, SSH access, and port forwarding_

## Prerequisites

- Coder version 2.24.0 or later
- Docker available inside your workspace (via Docker-in-Docker or a mounted socket, see [Docker in workspaces](../../admin/templates/extending-templates/docker-in-workspaces.md) for details on how to achieve this)
- The `devcontainer` CLI (`@devcontainers/cli` NPM package) installed in your workspace

The Dev Containers CLI integration is enabled by default in Coder.

Most templates with Dev Containers support include all these prerequisites. See
[Configure a template for Dev Containers](../../admin/integrations/devcontainers/integration.md)
for setup details.

## Features

- Automatic Dev Container detection from repositories
- Seamless container startup during workspace initialization
- Change detection with outdated status indicator
- On-demand container rebuild via dashboard button
- Integrated IDE experience with VS Code
- Direct SSH access to containers
- Automatic port detection

## Getting started

### Add a devcontainer.json

Add a `devcontainer.json` file to your repository. This file defines your
development environment. You can place it in:

- `.devcontainer/devcontainer.json` (recommended)
- `.devcontainer.json` (root of repository)
- `.devcontainer/<folder>/devcontainer.json` (for multiple configurations)

The third option allows monorepos to define multiple Dev Container
configurations in separate sub-folders. See the
[Dev Container specification](https://containers.dev/implementors/spec/#devcontainerjson)
for details.

Here's a minimal example:

```json
{
  "name": "My Dev Container",
  "image": "mcr.microsoft.com/devcontainers/base:ubuntu"
}
```

For more configuration options, see the
[Dev Container specification](https://containers.dev/).

### Start your Dev Container

Coder automatically discovers Dev Container configurations in your repositories
and displays them in your workspace dashboard. From there, you can start a dev
container with a single click.

![Discovered Dev Containers with Start buttons](../../images/user-guides/devcontainers/devcontainer-discovery.png)_Coder detects Dev Container configurations and displays them with a Start button_

If your template administrator has configured automatic startup (via the
`coder_devcontainer` Terraform resource or autostart settings), your dev
container will build and start automatically when the workspace starts.

### Connect to your Dev Container

Once running, your Dev Container appears as a sub-agent in your workspace
dashboard. You can connect via:

- **Web terminal** in the Coder dashboard
- **SSH** using `coder ssh <workspace>.<agent>`
- **VS Code** using the "Open in VS Code Desktop" button

See [Working with Dev Containers](./working-with-dev-containers.md) for detailed
connection instructions.

## How it works

The Dev Containers CLI integration uses the `devcontainer` command from
[`@devcontainers/cli`](https://github.com/devcontainers/cli) to manage
containers within your Coder workspace.

When a workspace with Dev Containers integration starts:

1. The workspace initializes the Docker environment.
1. The integration detects repositories with Dev Container configurations.
1. Detected Dev Containers appear in the Coder dashboard.
1. If auto-start is configured (via `coder_devcontainer` or autostart settings),
   the integration builds and starts the Dev Container automatically.
1. Coder creates a sub-agent for the running container, enabling direct access.

Without auto-start, users can manually start discovered Dev Containers from the
dashboard.

### Agent naming

Each Dev Container gets its own agent name, derived from the workspace folder
path. For example, a Dev Container with workspace folder `/home/coder/my-app`
will have an agent named `my-app`.

Agent names are sanitized to contain only lowercase alphanumeric characters and
hyphens. You can also set a
[custom agent name](./customizing-dev-containers.md#custom-agent-name)
in your `devcontainer.json`.

## Limitations

- **Linux only**: Dev Containers are currently not supported in Windows or
  macOS workspaces
- Changes to `devcontainer.json` require manual rebuild using the dashboard
  button
- The `forwardPorts` property in `devcontainer.json` with `host:port` syntax
  (e.g., `"db:5432"`) for Docker Compose sidecar containers is not yet
  supported. For single-container Dev Containers, use `coder port-forward` to
  access ports directly on the sub-agent.
- Some advanced Dev Container features may have limited support

## Next steps

- [Working with Dev Containers](./working-with-dev-containers.md) — SSH, IDE
  integration, and port forwarding
- [Customizing Dev Containers](./customizing-dev-containers.md) — Custom agent
  names, apps, and display options
- [Troubleshooting Dev Containers](./troubleshooting-dev-containers.md) —
  Diagnose common issues
- [Dev Container specification](https://containers.dev/) — Advanced
  configuration options
- [Dev Container features](https://containers.dev/features) — Enhance your
  environment with pre-built tools
