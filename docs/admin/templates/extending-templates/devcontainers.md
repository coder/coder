# Configure a Template for Dev Containers

Dev containers provide a consistent, reproducible development environment using the
[Development Containers specification](https://containers.dev/).
Coder's dev container support allows developers to work in fully configured environments with their preferred tools and extensions.

To enable dev containers in workspaces, [configure your template](../creating-templates.md) with the dev containers
modules and configurations outlined in this doc.

## Why Use Dev Containers

Dev containers improve consistency across environments by letting developers define their development setup.
When integrated with Coder templates, they provide:

- **Project-specific environments**: Each repository can define its own tools, extensions, and configuration.
- **Zero setup time**: Developers get fully configured environments without manual installation.
- **Consistency across teams**: Everyone works in identical environments regardless of their local machine.
- **Version control**: Development environment changes are tracked alongside code changes.

Visit [Choose an approach to Dev Containers](./dev-containers-envbuilder.md) for an in-depth comparison between
the Dev Container integration and the legacy Envbuilder integration.

## Prerequisites

- Dev containers require Docker to build and run containers inside the workspace.

  Ensure your workspace infrastructure has Docker configured with container creation permissions and sufficient resources.

  To confirm that Docker is configured correctly, create a test workspace and confirm that `docker ps` runs.
  If it doesn't, follow the steps in [Docker in workspaces](./docker-in-workspaces.md).

- The `devcontainers-cli` module requires npm.

  - You can use an image that already includes npm, such as `codercom/enterprise-node:ubuntu`.

## Install the Dev Containers CLI

Use the
[devcontainers-cli](https://registry.coder.com/modules/devcontainers-cli) module
to install `@devcontainers/cli` in your workspace:

```terraform
module "devcontainers-cli" {
  count    = data.coder_workspace.me.start_count
  source   = "dev.registry.coder.com/modules/devcontainers-cli/coder"
  agent_id = coder_agent.main.id
}
```

Alternatively, install `devcontainer/cli` manually in your base image:

```shell
RUN npm install -g @devcontainers/cli
```

## Define the Dev Container Resource

If you don't use [`git_clone`](#clone-the-repository), point the resource at the folder that contains `devcontainer.json`:

```terraform
resource "coder_devcontainer" "project" { # `project` in this example is how users will connect to the dev container: `ssh://project.<workspace>.me.coder`
  count            = data.coder_workspace.me.start_count
  agent_id         = coder_agent.main.id
  workspace_folder = "/home/coder/project"
}
```

## Clone the Repository

This step is optional, but it ensures that the project is present before the dev container starts.

Note that if you use the `git_clone` module, update or replace the `coder_devcontainer` resource
to point to `/home/coder/project/${module.git_clone[0].folder_name}` so that it is only defined once:

```terraform
module "git_clone" {
  count    = data.coder_workspace.me.start_count
  source   = "dev.registry.coder.com/modules/git-clone/coder"
  agent_id = coder_agent.main.id
  url      = "https://github.com/example/project.git"
  base_dir = "/home/coder"
}

resource "coder_devcontainer" "project" {
  count            = data.coder_workspace.me.start_count
  agent_id         = coder_agent.main.id
  workspace_folder = "/home/coder/${module.git_clone[0].folder_name}"
}
```

## Dev Container Features

Enhance your dev container experience with additional features.
For more advanced use cases, consult the [advanced dev containers doc](./advanced-dev-containers.md).

### Custom applications

```jsonc
{
  "customizations": {
    "coder": {
      "apps": [
        {
          "slug": "flask-app",
          "command": "python app.py",
          "icon": "/icon/flask.svg",
          "subdomain": true,
          "healthcheck": {
            "url": "http://localhost:5000/health",
            "interval": 10,
            "threshold": 10
          }
        }
      ]
    }
  }
}
```

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

## Example Docker Dev Container

<details><summary>Expand for the full file:</summary>

```terraform
terraform {
  required_providers {
    coder  = { source = "coder/coder" }
    docker = { source = "kreuzwerker/docker" }
  }
}

data "coder_workspace" "me" {}
data "coder_workspace_owner" "me" {}

resource "coder_agent" "main" {
  os   = "linux"
  arch = "amd64"

  startup_script_behavior = "blocking"
  startup_script  = "sudo service docker start"
  shutdown_script = "sudo service docker stop"
}

module "devcontainers-cli" {
  count    = data.coder_workspace.me.start_count
  source   = "dev.registry.coder.com/modules/devcontainers-cli/coder"
  agent_id = coder_agent.main.id
}

module "git_clone" {
  count    = data.coder_workspace.me.start_count
  source   = "dev.registry.coder.com/modules/git-clone/coder"
  agent_id = coder_agent.main.id
  url      = "https://github.com/coder/coder.git"
  base_dir = "/home/coder"
}

resource "coder_devcontainer" "project" {
  count            = data.coder_workspace.me.start_count
  agent_id         = coder_agent.main.id
  workspace_folder = "/home/coder/${module.git_clone[0].folder_name}"
}

resource "docker_container" "workspace" {
  count = data.coder_workspace.me.start_count
  image = "codercom/enterprise-node:ubuntu"
  name  = "coder-${data.coder_workspace_owner.me.name}-${lower(data.coder_workspace.me.name)}"

  runtime = "sysbox-runc"

  entrypoint = ["sh", "-c", coder_agent.main.init_script]

  env = [
    "CODER_AGENT_TOKEN=${coder_agent.main.token}",
    "CODER_AGENT_URL=${data.coder_workspace.me.access_url}",
    "CODER_AGENT_DEVCONTAINERS_ENABLE=true"
  ]
}
```

## Troubleshoot Common Issues

### Disable dev containers integration

To disable the dev containers integration in your workspace, set the `CODER_AGENT_DEVCONTAINERS_ENABLE= "false"` environment variable.

### Dev container does not start

1. Docker daemon not running inside the workspace.
1. `devcontainer.json` missing or is in the wrong place.
1. Build errors: check agent logs.

### Permission errors

- Docker socket not mounted or user lacks access.
- Workspace not `privileged` and no rootless runtime.

### Slow builds

- Allocate more CPU/RAM.
- Use image caching or pre-build common images.

## Next Steps

- [Advanced dev containers](./advanced-dev-containers.md)
- [Dev Containers Integration](../../../user-guides/devcontainers/index.md)
- [Working with Dev Containers](../../../user-guides/devcontainers/working-with-dev-containers.md)
- [Troubleshooting Dev Containers](../../../user-guides/devcontainers/troubleshooting-dev-containers.md)
