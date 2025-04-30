# Dev Containers Integration (Early Access)

The dev containers integration is an early access feature that enables seamless
creation and management of dev containers in Coder workspaces. This feature
leverages the [`@devcontainers/cli`](https://github.com/devcontainers/cli) and
[Docker](https://www.docker.com) to provide a streamlined development
experience.

> [!NOTE]
>
> This implementation is different from the existing
> [Envbuilder-based dev containers](../admin/templates/managing-templates/devcontainers/index.md)
> offering.

## Contents

- [Features](#features)
- [Prerequisites](#prerequisites)
- [How It Works](#how-it-works)
- [Template Configuration](#template-configuration)
- [Working with Dev Containers](#working-with-dev-containers)
- [Dev Container Features](#dev-container-features)
- [Limitations during Early Access](#limitations-during-early-access)
- [Comparison with Envbuilder-based Dev Containers](#comparison-with-envbuilder-based-dev-containers)
- [Troubleshooting](#troubleshooting)
- [Next Steps](#next-steps)

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

## Prerequisites

- Coder version 2.22.0 or later
- Coder CLI version 2.22.0 or later
- Dev containers integration enabled in your template(s)
- Docker-compatible workspace image in the template(s)
- Appropriate permissions to execute Docker commands inside your workspace

## How It Works

The dev containers integration utilizes the `devcontainer` command from
[`@devcontainers/cli`](https://github.com/devcontainers/cli) to manage dev
containers within your Coder workspace. This command provides comprehensive
functionality for creating, starting, and managing dev containers.

Dev environments are configured through a standard `devcontainer.json` file,
which allows for extensive customization of your development setup.

When a workspace with the dev containers integration starts:

1. The workspace initializes the Docker environment
2. The integration detects repositories with a `.devcontainer` directory or a
   `devcontainer.json` file
3. The integration builds and starts the dev container based on the
   configuration
4. Your workspace automatically detects the running dev container

## Template Configuration

To enable the dev containers integration in your template, you need to add
specific configurations as shown below.

### Install the Dev Containers CLI

Use the
[devcontainers-cli](https://registry.coder.com/modules/devcontainers-cli) module
to ensure the `@devcontainers/cli` is installed in your workspace:

```terraform
module "devcontainers-cli" {
  count    = data.coder_workspace.me.start_count
  source   = "dev.registry.coder.com/modules/devcontainers-cli/coder"
  agent_id = coder_agent.dev.id
}
```

Alternatively, install the devcontainer CLI manually in your base image.

### Configure Automatic Dev Container Startup

The
[`coder_devcontainer`](https://registry.terraform.io/providers/coder/coder/latest/docs/resources/devcontainer)
resource automatically starts a dev container in your workspace, ensuring it's
ready when you access the workspace:

```terraform
resource "coder_devcontainer" "my-repository" {
  count            = data.coder_workspace.me.start_count
  agent_id         = coder_agent.dev.id
  workspace_folder = "/home/coder/my-repository"
}
```

> [!NOTE]
>
> The `workspace_folder` attribute must specify the location of the dev
> container's workspace and should point to a valid project folder containing a
> `devcontainer.json` file.

> [!TIP]
>
> Consider using the [`git-clone`](https://registry.coder.com/modules/git-clone)
> module to ensure your repository is cloned into the workspace folder and ready
> for automatic startup.

### Enable Dev Containers Integration

To enable the dev containers integration in your workspace, you must set the
`CODER_AGENT_DEVCONTAINERS_ENABLE` environment variable to `true` in your
workspace container:

```terraform
resource "docker_container" "workspace" {
  count = data.coder_workspace.me.start_count
  image = "codercom/oss-dogfood:latest"
  env = [
    "CODER_AGENT_DEVCONTAINERS_ENABLE=true",
    # ... Other environment variables.
  ]
  # ... Other container configuration.
}
```

This environment variable is required for the Coder agent to detect and manage
dev containers. Without it, the agent will not attempt to start or connect to
dev containers even if the `coder_devcontainer` resource is defined.

### Complete Template Example

Here's a simplified template example that enables the dev containers
integration:

```terraform
terraform {
  required_providers {
    coder  = { source = "coder/coder" }
    docker = { source = "kreuzwerker/docker" }
  }
}

provider "coder" {}
data "coder_workspace" "me" {}
data "coder_workspace_owner" "me" {}

resource "coder_agent" "dev" {
  arch                    = "amd64"
  os                      = "linux"
  startup_script_behavior = "blocking"
  startup_script          = "sudo service docker start"
  shutdown_script         = "sudo service docker stop"
  # ...
}

module "devcontainers-cli" {
  count    = data.coder_workspace.me.start_count
  source   = "dev.registry.coder.com/modules/devcontainers-cli/coder"
  agent_id = coder_agent.dev.id
}

resource "coder_devcontainer" "my-repository" {
  count            = data.coder_workspace.me.start_count
  agent_id         = coder_agent.dev.id
  workspace_folder = "/home/coder/my-repository"
}

resource "docker_container" "workspace" {
  count = data.coder_workspace.me.start_count
  image = "codercom/oss-dogfood:latest"
  env = [
    "CODER_AGENT_DEVCONTAINERS_ENABLE=true",
    # ... Other environment variables.
  ]
  # ... Other container configuration.
}
```

## Working with Dev Containers

### SSH Access

You can SSH into your dev container directly using the Coder CLI:

```console
coder ssh --container keen_dijkstra my-workspace
```

> [!NOTE]
>
> SSH access is not yet compatible with the `coder config-ssh` command for use
> with OpenSSH. You would need to manually modify your SSH config to include the
> `--container` flag in the `ProxyCommand`.

### Web Terminal Access

Once your workspace and dev container are running, you can use the web terminal
in the Coder interface to execute commands directly inside the dev container.

### IDE Integration (VS Code)

You can open your dev container directly in VS Code by:

1. Selecting "Open in VS Code Desktop" from the Coder web interface
2. Using the Coder CLI with the container flag:

```console
coder open vscode --container keen_dijkstra my-workspace
```

While optimized for VS Code, other IDEs with dev containers support may also
work.

### Port Forwarding

During the early access phase, port forwarding is limited to ports defined via
[`appPort`](https://containers.dev/implementors/json_reference/#image-specific)
in your `devcontainer.json` file.

> [!NOTE]
>
> Support for automatic port forwarding via the `forwardPorts` property in
> `devcontainer.json` is planned for a future release.

For example, with this `devcontainer.json` configuration:

```json
{
	"appPort": ["8080:8080", "4000:3000"]
}
```

You can forward these ports to your local machine using:

```console
coder port-forward my-workspace --tcp 8080,4000
```

This forwards port 8080 (local) -> 8080 (agent) -> 8080 (dev container) and port
4000 (local) -> 4000 (agent) -> 3000 (dev container).

## Dev Container Features

You can use standard dev container features in your `devcontainer.json` file.
Coder also maintains a
[repository of features](https://github.com/coder/devcontainer-features) to
enhance your development experience.

Currently available features include:

- [code-server](https://github.com/coder/devcontainer-features/blob/main/src/code-server)

To use the code-server feature, add the following to your `devcontainer.json`:

```json
{
	"features": {
		"ghcr.io/coder/devcontainer-features/code-server:1": {
			"port": 13337
		}
	},
	"appPort": [13337]
}
```

> [!NOTE]
>
> Remember to include the port in the `appPort` section to ensure proper port
> forwarding.

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

## Troubleshooting

### Dev Container Not Starting

If your dev container fails to start:

1. Check the agent logs for error messages:
   - `/tmp/coder-agent.log`
   - `/tmp/coder-startup-script.log`
   - `/tmp/coder-script-[script_id].log`
2. Verify that Docker is running in your workspace
3. Ensure the `devcontainer.json` file is valid
4. Check that the repository has been cloned correctly
5. Verify the resource limits in your workspace are sufficient

## Next Steps

- Explore the [dev container specification](https://containers.dev/) to learn
  more about advanced configuration options
- Read about [dev container features](https://containers.dev/features) to
  enhance your development environment
- Check the
  [VS Code dev containers documentation](https://code.visualstudio.com/docs/devcontainers/containers)
  for IDE-specific features
