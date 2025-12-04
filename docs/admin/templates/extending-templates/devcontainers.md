# Configure a template for Dev Containers

To enable Dev Containers in workspaces, configure your template with the Dev Containers
modules and configurations outlined in this doc.

> [!NOTE]
>
> Dev Containers require a **Linux or macOS workspace**. Windows is not supported.

## Configuration Modes

There are two approaches to configuring Dev Containers in Coder:

### Manual Configuration

Use the [`coder_devcontainer`](https://registry.terraform.io/providers/coder/coder/latest/docs/resources/devcontainer) Terraform resource to explicitly define which Dev
Containers should be started in your workspace. This approach provides:

- Predictable behavior and explicit control
- Clear template configuration
- Easier troubleshooting
- Better for production environments

This is the recommended approach for most use cases.

### Project Discovery

Alternatively, enable automatic discovery of Dev Containers in Git repositories.
The agent scans for `devcontainer.json` files and surfaces them in the Coder UI.
See [Environment Variables](#environment-variables) for configuration options.

## Install the Dev Containers CLI

Use the
[devcontainers-cli](https://registry.coder.com/modules/devcontainers-cli) module
to ensure the `@devcontainers/cli` is installed in your workspace:

```terraform
module "devcontainers-cli" {
  count    = data.coder_workspace.me.start_count
  source   = "registry.coder.com/coder/devcontainers-cli/coder"
  agent_id = coder_agent.dev.id
}
```

Alternatively, install the devcontainer CLI manually in your base image.

## Configure Automatic Dev Container Startup

The
[`coder_devcontainer`](https://registry.terraform.io/providers/coder/coder/latest/docs/resources/devcontainer)
resource automatically starts a Dev Container in your workspace, ensuring it's
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

Dev Containers integration is **enabled by default** in Coder 2.24.0 and later.
You don't need to set any environment variables unless you want to change the
default behavior.

If you need to explicitly disable Dev Containers, set the
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

The following environment variables control Dev Container behavior in your
workspace. Both `CODER_AGENT_DEVCONTAINERS_ENABLE` and
`CODER_AGENT_DEVCONTAINERS_PROJECT_DISCOVERY_ENABLE` are **enabled by default**,
so you typically don't need to set them unless you want to explicitly disable
the feature.

### CODER_AGENT_DEVCONTAINERS_ENABLE

**Default: `true`** • **Added in: v2.24.0**

Enables the Dev Containers integration in the Coder agent.

The Dev Containers feature is enabled by default. You can explicitly disable it
by setting this to `false`.

### CODER_AGENT_DEVCONTAINERS_PROJECT_DISCOVERY_ENABLE

**Default: `true`** • **Added in: v2.25.0**

Enables automatic discovery of Dev Containers in Git repositories.

When enabled, the agent scans the configured working directory (set via the
`directory` attribute in `coder_agent`, typically the user's home directory) for
Git repositories. If the directory itself is a Git repository, it searches that
project. Otherwise, it searches immediate subdirectories for Git repositories.

For each repository found, the agent looks for `devcontainer.json` files in the
[standard locations](../../../user-guides/devcontainers/index.md#1-add-a-devcontainerjson)
and surfaces discovered Dev Containers in the Coder UI. Discovery respects
`.gitignore` patterns.

Set to `false` if you prefer explicit configuration via `coder_devcontainer`.

### CODER_AGENT_DEVCONTAINERS_DISCOVERY_AUTOSTART_ENABLE

**Default: `false`** • **Added in: v2.25.0**

Automatically starts Dev Containers discovered via project discovery.

When enabled, discovered Dev Containers will be automatically built and started
during workspace initialization. This only applies to Dev Containers found via
project discovery. Dev Containers defined with the `coder_devcontainer` resource
always auto-start regardless of this setting.

## Per-Container Customizations

Developers can customize individual dev containers using the `customizations.coder`
block in their `devcontainer.json` file. Available options include:

- `ignore` — Hide a dev container from Coder completely
- `autoStart` — Control whether the container starts automatically (requires
  `CODER_AGENT_DEVCONTAINERS_DISCOVERY_AUTOSTART_ENABLE` to be enabled)
- `name` — Set a custom agent name
- `displayApps` — Control which built-in apps appear
- `apps` — Define custom applications

For the full reference, see
[Customizing dev containers](../../../user-guides/devcontainers/customizing-dev-containers.md).

## Complete Template Example

Here's a simplified template example that uses Dev Containers with manual
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
  source   = "registry.coder.com/coder/devcontainers-cli/coder"
  agent_id = coder_agent.dev.id
}

resource "coder_devcontainer" "my-repository" {
  count            = data.coder_workspace.me.start_count
  agent_id         = coder_agent.dev.id
  workspace_folder = "/home/coder/my-repository"
}
```

### Alternative: Project Discovery with Autostart

By default, discovered containers appear in the dashboard but developers must
manually start them. To have them start automatically, enable autostart:

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

With autostart enabled:

- Discovered containers automatically build and start during workspace
  initialization
- The `coder_devcontainer` resource is not required
- Developers can work with multiple projects seamlessly

> [!NOTE]
>
> When using project discovery, you still need to install the devcontainers CLI
> using the module or in your base image.

## Example Template

The [Docker (Dev Containers)](https://github.com/coder/coder/tree/main/examples/templates/docker-devcontainer)
starter template demonstrates Dev Containers integration using Docker-in-Docker.
It includes the `devcontainers-cli` module, `git-clone` module, and the
`coder_devcontainer` resource.

## Next Steps

- [Dev Containers Integration](../../../user-guides/devcontainers/index.md)
- [Customizing Dev Containers](../../../user-guides/devcontainers/customizing-dev-containers.md)
- [Working with Dev Containers](../../../user-guides/devcontainers/working-with-dev-containers.md)
- [Troubleshooting Dev Containers](../../../user-guides/devcontainers/troubleshooting-dev-containers.md)
