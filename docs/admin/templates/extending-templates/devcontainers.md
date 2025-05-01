# Configure a template for dev containers

To enable dev containers in workspaces, configure your template with the dev containers
modules and configurations outlined in this doc.

## Install the Dev Containers CLI

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

## Configure Automatic Dev Container Startup

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

<!-- nolint:MD028/no-blanks-blockquote -->

> [!TIP]
>
> Consider using the [`git-clone`](https://registry.coder.com/modules/git-clone)
> module to ensure your repository is cloned into the workspace folder and ready
> for automatic startup.

## Enable Dev Containers Integration

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

## Complete Template Example

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

## Next Steps

- [Dev Containers Integration](../../../user-guides/devcontainers/index.md)
