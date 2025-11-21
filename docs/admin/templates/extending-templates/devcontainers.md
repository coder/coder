# Configure a template for dev containers

To enable dev containers in workspaces, configure your template with the dev containers
modules and configurations outlined in this doc.

> [!NOTE]
>
> Dev containers require a **Linux or macOS workspace**. Windows is not supported.

## Configuration Modes

There are two approaches to configuring dev containers in Coder:

### Manual Configuration

Use the [`coder_devcontainer`](https://registry.terraform.io/providers/coder/coder/latest/docs/resources/devcontainer) Terraform resource to explicitly define which dev
containers should be started in your workspace. This approach provides:

- Predictable behavior and explicit control
- Clear template configuration
- Easier troubleshooting
- Better for production environments

This is the recommended approach for most use cases.

### Project Discovery

Enable automatic discovery of dev containers in git repositories. Project discovery automatically scans git repositories for `.devcontainer/devcontainer.json` or `.devcontainer.json` files and surfaces them in the Coder UI. See the [Environment Variables](#environment-variables) section for detailed configuration options.

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

Dev containers integration is **enabled by default** in Coder 2.24.0 and later.
You don't need to set any environment variables unless you want to change the
default behavior.

If you need to explicitly disable dev containers, set the
`CODER_AGENT_DEVCONTAINERS_ENABLE` environment variable to `false`:

```terraform
resource "docker_container" "workspace" {
  count = data.coder_workspace.me.start_count
  image = "codercom/oss-dogfood:latest"
  env = [
    "CODER_AGENT_DEVCONTAINERS_ENABLE=false",  # Explicitly disable
    # ... Other environment variables.
  ]
  # ... Other container configuration.
}
```

See the [Environment Variables](#environment-variables) section below for more
details on available configuration options.

## Environment Variables

The following environment variables control dev container behavior in your
workspace. Both `CODER_AGENT_DEVCONTAINERS_ENABLE` and
`CODER_AGENT_DEVCONTAINERS_PROJECT_DISCOVERY_ENABLE` are **enabled by default**,
so you typically don't need to set them unless you want to explicitly disable
the feature.

### CODER_AGENT_DEVCONTAINERS_ENABLE

**Default: `true`** • **Added in: v2.24.0**

Enables the dev containers integration in the Coder agent.

The dev containers feature is enabled by default. You can explicitly disable it
by setting this to `false`.

### CODER_AGENT_DEVCONTAINERS_PROJECT_DISCOVERY_ENABLE

**Default: `true`** • **Added in: v2.25.0**

Enables automatic discovery of dev containers in git repositories.

When enabled (the default), the agent will:

- Scan the agent directory for git repositories
- Look for `.devcontainer/devcontainer.json` or `.devcontainer.json` files
- Register discovered dev containers automatically
- Respect `.gitignore` patterns during discovery

You can disable automatic discovery by setting this to `false` if you prefer to
use only the `coder_devcontainer` resource for explicit configuration.

### CODER_AGENT_DEVCONTAINERS_DISCOVERY_AUTOSTART_ENABLE

**Default: `false`** • **Added in: v2.25.0**

Automatically starts dev containers discovered via project discovery.

When enabled, discovered dev containers will be automatically built and started
during workspace initialization. This only applies to dev containers found via
project discovery. Dev containers defined with the `coder_devcontainer` resource
always auto-start regardless of this setting.

## Complete Template Example

Here's a simplified template example that uses dev containers with manual
configuration:

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
```

### Alternative: Project Discovery Mode

You can enable automatic starting of discovered dev containers:

```terraform
resource "docker_container" "workspace" {
  count = data.coder_workspace.me.start_count
  image = "codercom/oss-dogfood:latest"
  env = [
    # Project discovery is enabled by default, but autostart is not.
    # Enable autostart to automatically build and start discovered containers:
    "CODER_AGENT_DEVCONTAINERS_DISCOVERY_AUTOSTART_ENABLE=true",
    # ... Other environment variables.
  ]
  # ... Other container configuration.
}
```

With this configuration:

- Project discovery is enabled (default behavior)
- Discovered containers are automatically started (via the env var)
- The `coder_devcontainer` resource is **not** required
- Developers can work with multiple projects seamlessly

> [!NOTE]
>
> When using project discovery, you still need to install the devcontainers CLI
> using the module or in your base image.

## Next Steps

- [Dev Containers Integration](../../../user-guides/devcontainers/index.md)
- [Working with Dev Containers](../../../user-guides/devcontainers/working-with-dev-containers.md)
- [Troubleshooting Dev Containers](../../../user-guides/devcontainers/troubleshooting-dev-containers.md)
