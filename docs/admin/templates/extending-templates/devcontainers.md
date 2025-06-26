# Configure a template for dev containers

Dev containers provide a consistent, reproducible development environment using the
[Development Containers specification](https://containers.dev/).
Coder's dev container support allows developers to work in fully configured environments with their preferred tools and extensions.

To enable dev containers in workspaces, [configure your template](../creating-templates.md) with the dev containers
modules and configurations outlined in this doc.

## Why use dev containers

Dev containers improve consistency across environments by letting developers define their development setup.
When integrated with Coder templates, they provide:

- **Project-specific environments**: Each repository can define its own tools, extensions, and configuration.
- **Zero setup time**: Developers get fully configured environments without manual installation.
- **Consistency across teams**: Everyone works in identical environments regardless of their local machine.
- **Version control**: Development environment changes are tracked alongside code changes.

## Prerequisites

- Dev containers require Docker to build and run containers inside the workspace.

  Ensure your workspace infrastructure has Docker configured with container creation permissions and sufficient resources.

  To confirm that Docker is configured correctly, create a test workspace and confirm that `docker ps` runs.
  If it doesn't, follow the steps in [Docker in workspaces](./docker-in-workspaces.md).

- The `devcontainers-cli` module requires npm.

  - Use an image that already includes npm, such as `codercom/enterprise-node:ubuntu`
  - <details><summary>If your template doesn't already include npm, install it at runtime with the `nodejs` module:</summary>

    1. This block should be before the `devcontainers-cli` block in `main.tf`:

       ```terraform
        module "nodejs" {
          count    = data.coder_workspace.me.start_count
          source   = "dev.registry.coder.com/modules/nodejs/coder"
          agent_id = coder_agent.main.id
        }
        ```

    1. In the `devcontainers-cli` module block, add:

       ```terraform
       depends_on       = [module.nodejs]
       ```

   </details>

## Enable Dev Containers Integration

To enable the dev containers integration in your workspace, add the `CODER_AGENT_DEVCONTAINERS_ENABLE` environment variable to your existing `coder_agent` block:

```terraform
env = {
  CODER_AGENT_DEVCONTAINERS_ENABLE = "true"
  # existing variables ...
}
```

This environment variable is required for the Coder agent to detect and manage dev containers.
Without it, the agent will not attempt to start or connect to dev containers even if the
`coder_devcontainer` resource is defined.

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

Alternatively, install the devcontainer CLI manually in your base image:

```shell
RUN npm install -g @devcontainers/cli
```

## Define the dev container resource

If you don't use [`git_clone`](#clone-the-repository), point the resource at the folder that contains `devcontainer.json`:

```terraform
resource "coder_devcontainer" "project" {
  count            = data.coder_workspace.me.start_count
  agent_id         = coder_agent.main.id
  workspace_folder = "/home/coder/project"
}
```

## Clone the repository

This step is optional, but it ensures that the project is present before the dev container starts.

Note that if you use the `git_clone` module, place it before the `coder_devcontainer` resource
and update or replace that resource to point at `/home/coder/project/${module.git_clone[0].folder_name}` so that it is only defined once:

```terraform
module "git_clone" {
  count    = data.coder_workspace.me.start_count
  source   = "dev.registry.coder.com/modules/git-clone/coder"
  agent_id = coder_agent.main.id
  url      = "https://github.com/example/project.git"
  base_dir = "/home/coder/project"
}

resource "coder_devcontainer" "project" {
  count            = data.coder_workspace.me.start_count
  agent_id         = coder_agent.main.id
  workspace_folder = "/home/coder/project/${module.git_clone[0].folder_name}"
  depends_on       = [module.git_clone]
}
```

## Dev container features

Enhance your dev container experience with additional features.
For more advanced use cases, consult the [advanced dev containers doc](./advanced-dev-containers.md).

### Custom applications

```jsonc
{
  "customizations": {
    "coder": {
      "apps": {
        "flask-app": {
          "command": "python app.py",
          "icon": "/icon/flask.svg",
          "subdomain": true,
          "healthcheck": {
            "url": "http://localhost:5000/health",
            "interval": 10,
            "threshold": 10
          }
        }
      }
    }
  }
}
```

### Agent naming

Coder names dev container agents in this order:

1. `customizations.coder.agent.name` in `devcontainer.json`
1. `name` in `devcontainer.json`
1. Directory name that contains the config
1. `devcontainer` (default)

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

## Complete Template Examples

You can test the Coder dev container integration and features with these starter templates.

<details><summary>Docker-based template (privileged)</summary>

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
  env  = { CODER_AGENT_DEVCONTAINERS_ENABLE = "true" }

  startup_script_behavior = "blocking"
  startup_script  = "sudo service docker start"
  shutdown_script = "sudo service docker stop"
}

module "devcontainers_cli" {
  count    = data.coder_workspace.me.start_count
  source   = "dev.registry.coder.com/modules/devcontainers-cli/coder"
  agent_id = coder_agent.main.id
}

module "git_clone" {
  count    = data.coder_workspace.me.start_count
  source   = "dev.registry.coder.com/modules/git-clone/coder"
  agent_id = coder_agent.main.id
  url      = "https://github.com/example/project.git"
  base_dir     = "/home/coder/project"
}

resource "coder_devcontainer" "project" {
  count            = data.coder_workspace.me.start_count
  agent_id         = coder_agent.main.id
  workspace_folder = "/home/coder/project/${module.git_clone[0].folder_name}"
  depends_on       = [module.git_clone]
}

resource "docker_container" "workspace" {
  count      = data.coder_workspace.me.start_count
  image      = "codercom/enterprise-node:ubuntu"
  name       = "coder-${data.coder_workspace_owner.me.name}-${lower(data.coder_workspace.me.name)}"
  privileged = true   # or mount /var/run/docker.sock
}
```

</details>

<details><summary>Kubernetes-based template (Sysbox runtime)</summary>

```terraform
terraform {
  required_providers {
    coder      = { source = "coder/coder" }
    kubernetes = { source = "hashicorp/kubernetes" }
  }
}

data "coder_workspace" "me" {}
data "coder_workspace_owner" "me" {}

resource "coder_agent" "main" {
  os   = "linux"
  arch = "amd64"
  env  = { CODER_AGENT_DEVCONTAINERS_ENABLE = "true" }

  startup_script_behavior = "blocking"
  startup_script = "sudo service docker start"
}

module "devcontainers_cli" {
  count    = data.coder_workspace.me.start_count
  source   = "dev.registry.coder.com/modules/devcontainers-cli/coder"
  agent_id = coder_agent.main.id
}

module "git_clone" {
  count    = data.coder_workspace.me.start_count
  source   = "dev.registry.coder.com/modules/git-clone/coder"
  agent_id = coder_agent.main.id
  url      = "https://github.com/example/project.git"
  base_dir     = "/home/coder/project"
}

resource "coder_devcontainer" "project" {
  count            = data.coder_workspace.me.start_count
  agent_id         = coder_agent.main.id
  workspace_folder = "/home/coder/project/${module.git_clone[0].folder_name}"
  depends_on       = [module.git_clone]
}

resource "kubernetes_pod" "workspace" {
  count = data.coder_workspace.me.start_count

  metadata {
    name       = "coder-${data.coder_workspace_owner.me.name}-${lower(data.coder_workspace.me.name)}"
    namespace = "coder-workspaces"
  }

  spec {
    container {
      name  = "main"
      image = "codercom/enterprise-base:ubuntu"

      security_context { privileged = true }  # or use Sysbox / rootless
      env { name = "CODER_AGENT_TOKEN" value = coder_agent.main.token }
    }
  }
}
```

</details>

## Troubleshoot common issues

### Dev container does not start

1. `CODER_AGENT_DEVCONTAINERS_ENABLE=true` missing.
1. Docker daemon not running inside the workspace.
1. `devcontainer.json` missing or mislocated.
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
