# Configure a Template for Dev Containers

Dev containers provide consistent, reproducible development environments using the
[Development Containers specification](https://containers.dev/).
Coder's dev container support allows developers to work in fully configured environments with their preferred tools and extensions.

To add dev container support to a workspace, [configure your template](../creating-templates.md) with the dev containers
modules and configurations outlined in this doc.

## Why Use Dev Containers

Dev containers improve consistency across environments by letting developers define their development setup within
the code repository.

When integrated with Coder templates, dev containers provide:

- **Project-specific environments**: Each repository can define its own tools, extensions, and configuration.
- **Faster start times**: Developers start workspace with fully configured environments without additional installation.
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
  count = data.coder_workspace.me.start_count
  source = "registry.coder.com/modules/devcontainers-cli/coder"
  agent_id = coder_agent.main.id
}
```

Alternatively, you can install `@devcontainer/cli` manually in your base image:

```shell
RUN npm install -g @devcontainers/cli
```

## Configure the Agent for Docker Support

Your Coder agent needs proper configuration to support Docker and dev containers:

```terraform
resource "coder_agent" "main" {
  os = "linux"
  arch = "amd64"

  # Ensure Docker starts before the agent considers the workspace ready
  startup_script_behavior = "blocking"
  startup_script = "sudo service docker start"

  # Properly shut down Docker when the workspace stops
  shutdown_script = "sudo service docker stop"
}
```

The `blocking` behavior ensures Docker is fully started before the workspace is considered ready, which is critical for dev containers to function correctly.

## Define the Dev Container Resource

Use the git-clone module to provide repository access and define the dev container:

```terraform
module "git-clone" {
  count = data.coder_workspace.me.start_count
  source = "registry.coder.com/modules/git-clone/coder"
  agent_id = coder_agent.main.id
  url = "https://github.com/example/repository.git"
  base_dir = "" # defaults to $HOME
}

resource "coder_devcontainer" "repository" {
  count = data.coder_workspace.me.start_count
  agent_id = coder_agent.main.id
  workspace_folder = module.git-clone[0].repo_dir  # Points to the cloned repository
}
```

The `repo_dir` output from the git-clone module provides the path to the cloned repository.

### If you're not using the git-clone module

If you need to point to a pre-existing directory or have another way of provisioning repositories, specify the directory that contains the dev container configuration:

```terraform
resource "coder_devcontainer" "my-repository" {
  count = data.coder_workspace.me.start_count
  agent_id = coder_agent.main.id
  workspace_folder = "/home/coder/project" # Path to a folder with devcontainer.json
}
```

> [!NOTE]
> The `workspace_folder` attribute must specify the location of the dev container's workspace and should point to a
> valid project directory that contains a `devcontainer.json` file or `.devcontainer` directory.

## Create the Docker Container Resource

Configure the Docker container to run the Coder agent and enable Docker-in-Docker capabilities required for dev containers.

This example uses the `sysbox-runc` runtime for secure container isolation.
If sysbox isn't available in your environment, consult [Docker in workspaces](./docker-in-workspaces.md) for alternatives.

```terraform
resource "docker_container" "workspace" {
  count = data.coder_workspace.me.start_count

  # Base image - this example uses one with npm pre-installed
  image = "codercom/enterprise-node:ubuntu"

  # Create a unique container name to avoid conflicts
  name = "coder-${data.coder_workspace_owner.me.name}-${lower(data.coder_workspace.me.name)}"

  # Use sysbox-runc for secure Docker-in-Docker
  runtime = "sysbox-runc"

  # Start the Coder agent when the container starts
  entrypoint = ["sh", "-c", coder_agent.main.init_script]

  # Configure agent authentication
  env = [
    "CODER_AGENT_TOKEN=${coder_agent.main.token}",
    "CODER_AGENT_URL=${data.coder_workspace.me.access_url}"
  ]
}
```

### Agent naming

Coder names dev container agents in this order:

1. `customizations.coder.name` in `devcontainer.json`
1. Project directory name (name of folder containing `devcontainer.json` or `.devcontainer` folder)
1. If the project directory name is already taken, the name is expanded to include the parent folder.

   For example, if the path is `/home/coder/some/project` and `project` is taken, then the agent is `some-project`.

## Add Dev Container Features

Enhance your dev container experience with additional features.
For more advanced use cases, consult the [advanced dev containers doc](./advanced-dev-containers.md).

### Custom applications

Add apps to the dev container workspace resource for one-click access:

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

### Multiple dev containers

```terraform
resource "coder_devcontainer" "frontend" {
  count = data.coder_workspace.me.start_count
  agent_id = coder_agent.main.id
  workspace_folder = "/home/coder/frontend"
}
resource "coder_devcontainer" "backend" {
  count = data.coder_workspace.me.start_count
  agent_id = coder_agent.main.id
  workspace_folder = "/home/coder/backend"
}
```

## Example Docker Dev Container Template

<details><summary>Expand for the full file:</summary>

```terraform
terraform {
  required_providers {
    coder = { source = "coder/coder" }
    docker = { source = "kreuzwerker/docker" }
  }
}

data "coder_workspace" "me" {}
data "coder_workspace_owner" "me" {}

resource "coder_agent" "main" {
  os = "linux"
  arch = "amd64"
  startup_script_behavior = "blocking"
  startup_script = "sudo service docker start"
  shutdown_script = "sudo service docker stop"
}

module "devcontainers-cli" {
  count = data.coder_workspace.me.start_count
  source = "registry.coder.com/modules/devcontainers-cli/coder"
  agent_id = coder_agent.main.id
}

module "git-clone" {
  count = data.coder_workspace.me.start_count
  source = "registry.coder.com/modules/git-clone/coder"
  agent_id = coder_agent.main.id
  url = "https://github.com/coder/coder.git"
  base_dir = "" # defaults to $HOME
  depth = 1 # Use a shallow clone for faster startup
}

resource "coder_devcontainer" "my-repository" {
  count = data.coder_workspace.me.start_count
  agent_id = coder_agent.main.id
  workspace_folder = module.git-clone[0].repo_dir
}

resource "docker_container" "workspace" {
  count = data.coder_workspace.me.start_count
  image = "codercom/enterprise-node:ubuntu"
  name = "coder-${data.coder_workspace_owner.me.name}-${lower(data.coder_workspace.me.name)}"
  runtime = "sysbox-runc"
  entrypoint = ["sh", "-c", coder_agent.main.init_script]
  env = [
    "CODER_AGENT_TOKEN=${coder_agent.main.token}",
    "CODER_AGENT_URL=${data.coder_workspace.me.access_url}"
  ]
}
```

## Troubleshoot Common Issues

### Disable dev containers integration

To disable the dev containers integration in your workspace, set the `CODER_AGENT_DEVCONTAINERS_ENABLE=false`
environment variable before starting the agent.

### Dev container does not start

1. Confirm that the Docker daemon is running inside the workspace:

   ```shell
     sudo service docker start && \
     docker ps
     ```

1. Confirm the location of `devcontainer.json`.

1. Check the agent logs for errors.

## Next Steps

- [Advanced dev containers](./advanced-dev-containers.md)
- [Dev Containers Integration](../../../user-guides/devcontainers/index.md)
- [Working with Dev Containers](../../../user-guides/devcontainers/working-with-dev-containers.md)
- [Troubleshooting Dev Containers](../../../user-guides/devcontainers/troubleshooting-dev-containers.md)
