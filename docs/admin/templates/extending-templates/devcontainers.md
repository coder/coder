# Configure a Template for Dev Containers

Dev containers provide consistent, reproducible development environments using the
[Development Containers specification](https://containers.dev/).
Coder's dev container support allows developers to work in fully configured environments with their preferred tools and extensions.

To enable dev containers in workspaces, [configure your template](../creating-templates.md) with the dev containers
modules and configurations outlined in this doc.

## Why Use Dev Containers

Dev containers improve consistency across environments by letting developers define their development setup within
the code repository.

When integrated with Coder templates, dev containers provide:

- **Project-specific environments**: Each repository can define its own tools, extensions, and configuration.
- **Zero setup time**: Developers start workspace with fully configured environments without additional installation.
- **Consistency across teams**: Everyone works in identical environments regardless of their local machine.
- **Version-controlled environments**: Development environment changes are tracked alongside code changes.

The Dev Container integration replaces the legacy Envbuilder integration.
Visit [Choose an approach to Dev Containers](./dev-containers-envbuilder.md) for more information about how they compare.

## Prerequisites

- Dev containers require Docker to build and run containers inside the workspace.

  Ensure your workspace infrastructure has Docker configured with container creation permissions and sufficient resources.

  If it doesn't, follow the steps in [Docker in workspaces](./docker-in-workspaces.md).

- The `devcontainers-cli` module requires npm.

  - You can use an image that already includes npm, such as `codercom/enterprise-node:ubuntu`.

## Install the Dev Containers CLI

Use the
[devcontainers-cli](https://registry.coder.com/modules/devcontainers-cli) module
to install `devcontainers/cli` in your workspace:

```terraform
module "devcontainers-cli" {
  count    = data.coder_workspace.me.start_count
  source   = "dev.registry.coder.com/modules/devcontainers-cli/coder"
  agent_id = coder_agent.main.id
}
```

Alternatively, install `@devcontainer/cli` manually in your base image:

```shell
RUN npm install -g @devcontainers/cli
```

## Define the Dev Container Resource

If you don't use the [`git-clone`](#clone-the-repository) module, point the resource at the folder that contains `devcontainer.json`:

```terraform
resource "coder_devcontainer" "my-repository" {
  count            = data.coder_workspace.me.start_count
  agent_id         = coder_agent.main.id
  workspace_folder = "/home/coder/project" # or /home/coder/project/.devcontainer
}
```

> [!NOTE]
> The `workspace_folder` attribute must specify the location of the dev
> container's workspace and should point to a valid project folder containing a
> `devcontainer.json` file.

## Add Dev Container Features

Enhance your dev container experience with additional features.
For more advanced use cases, consult the [advanced dev containers doc](./advanced-dev-containers.md).

### Custom applications

Add apps to the dev container workspace resource for one-click access.

```terraform
		"coder": {
			"apps": [
				{
					"slug": "cursor",
					"displayName": "Cursor",
					"url": "cursor://coder.coder-remote/openDevContainer?owner=${localEnv:CODER_WORKSPACE_OWNER_NAME}&workspace=${localEnv:CODER_WORKSPACE_NAME}&agent=${localEnv:CODER_WORKSPACE_PARENT_AGENT_NAME}&url=${localEnv:CODER_URL}&token=$SESSION_TOKEN&devContainerName=${localEnv:CONTAINER_ID}&devContainerFolder=${containerWorkspaceFolder}&localWorkspaceFolder=${localWorkspaceFolder}",
					"external": true,
					"icon": "/icon/cursor.svg",
					"order": 1
				},
```

<details><summary>Expand for a full example:</summary>

This is an excerpt from the [.devcontainer.json](https://github.com/coder/coder/blob/main/.devcontainer/devcontainer.json) in the Coder repository:

```terraform
resource "coder_devcontainer" "my-repository" {
...
{
  "customizations": {
		...
		"coder": {
			"apps": [
				{
					"slug": "cursor",
					"displayName": "Cursor",
					"url": "cursor://coder.coder-remote/openDevContainer?owner=${localEnv:CODER_WORKSPACE_OWNER_NAME}&workspace=${localEnv:CODER_WORKSPACE_NAME}&agent=${localEnv:CODER_WORKSPACE_PARENT_AGENT_NAME}&url=${localEnv:CODER_URL}&token=$SESSION_TOKEN&devContainerName=${localEnv:CONTAINER_ID}&devContainerFolder=${containerWorkspaceFolder}&localWorkspaceFolder=${localWorkspaceFolder}",
					"external": true,
					"icon": "/icon/cursor.svg",
					"order": 1
				},
				// Reproduce `code-server` app here from the code-server
				// feature so that we can set the correct folder and order.
				// Currently, the order cannot be specified via option because
				// we parse it as a number whereas variable interpolation
				// results in a string. Additionally we set health check which
				// is not yet set in the feature.
				{
					"slug": "code-server",
					"displayName": "code-server",
					"url": "http://${localEnv:FEATURE_CODE_SERVER_OPTION_HOST:127.0.0.1}:${localEnv:FEATURE_CODE_SERVER_OPTION_PORT:8080}/?folder=${containerWorkspaceFolder}",
					"openIn": "${localEnv:FEATURE_CODE_SERVER_OPTION_APPOPENIN:slim-window}",
					"share": "${localEnv:FEATURE_CODE_SERVER_OPTION_APPSHARE:owner}",
					"icon": "/icon/code.svg",
					"group": "${localEnv:FEATURE_CODE_SERVER_OPTION_APPGROUP:Web Editors}",
					"order": 3,
					"healthCheck": {
						"url": "http://${localEnv:FEATURE_CODE_SERVER_OPTION_HOST:127.0.0.1}:${localEnv:FEATURE_CODE_SERVER_OPTION_PORT:8080}/healthz",
						"interval": 5,
						"threshold": 2
					},
				{
					"slug": "windsurf",
					"displayName": "Windsurf Editor",
					"url": "windsurf://coder.coder-remote/openDevContainer?owner=${localEnv:CODER_WORKSPACE_OWNER_NAME}&workspace=${localEnv:CODER_WORKSPACE_NAME}&agent=${localEnv:CODER_WORKSPACE_PARENT_AGENT_NAME}&url=${localEnv:CODER_URL}&token=$SESSION_TOKEN&devContainerName=${localEnv:CONTAINER_ID}&devContainerFolder=${containerWorkspaceFolder}&localWorkspaceFolder=${localWorkspaceFolder}",
					"external": true,
					"icon": "/icon/windsurf.svg",
					"order": 3
				},
				{
					"slug": "zed",
					"displayName": "Zed Editor",
					"url": "zed://ssh/${localEnv:CODER_WORKSPACE_AGENT_NAME}.${localEnv:CODER_WORKSPACE_NAME}.${localEnv:CODER_WORKSPACE_OWNER_NAME}.coder${containerWorkspaceFolder}",
					"external": true,
					"icon": "/icon/zed.svg",
					"order": 4
				},
				}
			]
		}
	},
}
```

</details>

### Agent naming

Coder names dev container agents in this order:

1. `customizations.coder.name` in `devcontainer.json`
1. Project directory name (name of folder containing `devcontainer.json` or `.devcontainer` folder)
1. If the project directory name is already taken, the name is expanded to include the parent folder.

   For example, if the path is `/home/coder/some/project` and `project` is taken, then the agent is `some-project`.

### Multiple dev containers

```terraform
resource "coder_devcontainer" "frontend" {
  count            = data.coder_workspace.me.start_count
  agent_id         = coder_agent.main.id
  workspace_folder = "/home/coder/frontend"
}
resource "coder_devcontainer" "backend" {
  count            = data.coder_workspace.me.start_count
  agent_id         = coder_agent.main.id
  workspace_folder = "/home/coder/backend"
}
```

## Example Docker Dev Container Template

<details><summary>Expand for the full file:</summary>

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

resource "coder_agent" "main" {
  arch                    = "amd64"
  os                      = "linux"
  startup_script_behavior = "blocking"
  startup_script          = "sudo service docker start"
  shutdown_script         = "sudo service docker stop"
}

module "devcontainers-cli" {
  count    = data.coder_workspace.me.start_count
  source   = "registry.coder.com/modules/devcontainers-cli/coder"
  agent_id = coder_agent.main.id
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

## Next Steps

- [Dev Containers Integration](../../../user-guides/devcontainers/index.md)
- [Working with Dev Containers](../../../user-guides/devcontainers/working-with-dev-containers.md)
- [Troubleshooting Dev Containers](../../../user-guides/devcontainers/troubleshooting-dev-containers.md)
