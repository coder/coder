# Extending templates

There are a variety of Coder-native features to extend the configuration of your
development environments. Many of the following features are defined in your
templates using the
[Coder Terraform provider](https://registry.terraform.io/providers/coder/coder/latest/docs).
The provider docs will provide code examples for usage; alternatively, you can
view our
[example templates](https://github.com/coder/coder/tree/main/examples/templates)
to get started.

## Workspace agents

For users to connect to a workspace, the template must include a
[`coder_agent`](https://registry.terraform.io/providers/coder/coder/latest/docs/resources/agent).
The associated agent will facilitate
[workspace connections](../../../user-guides/workspace-access/index.md) via SSH,
port forwarding, and IDEs. The agent may also display real-time
[workspace metadata](./agent-metadata.md) like resource usage.

```tf
resource "coder_agent" "dev" {
  os   = "linux"
  arch = "amd64"
  dir  = "/workspace"
  display_apps {
    vscode = true
  }
}
```

You can also leverage [resource metadata](./resource-metadata.md) to display
static resource information from your template.

Templates must include some computational resource to start the agent. All
processes on the workspace are then spawned from the agent. It also provides all
information displayed in the dashboard's workspace view.

![A healthy workspace agent](../../../images/templates/healthy-workspace-agent.png)

Multiple agents may be used in a single template or even a single resource. Each
agent may have its own apps, startup script, and metadata. This can be used to
associate multiple containers or VMs with a workspace.

## Resource persistence

The resources you define in a template may be _ephemeral_ or _persistent_.
Persistent resources stay provisioned when workspaces are stopped, where as
ephemeral resources are destroyed and recreated on restart. All resources are
destroyed when a workspace is deleted.

You can read more about how resource behavior and workspace state in the [workspace lifecycle documentation](../../../user-guides/workspace-lifecycle.md).

Template resources follow the
[behavior of Terraform resources](https://developer.hashicorp.com/terraform/language/resources/behavior#how-terraform-applies-a-configuration)
and can be further configured  using the
[lifecycle argument](https://developer.hashicorp.com/terraform/language/meta-arguments/lifecycle).

A common configuration is a template whose only persistent resource is the home
directory. This allows the developer to retain their work while ensuring the
rest of their environment is consistently up-to-date on each workspace restart.

When a workspace is deleted, the Coder server essentially runs a
[terraform destroy](https://www.terraform.io/cli/commands/destroy) to remove all
resources associated with the workspace.

> [!TIP]
> Terraform's
> [prevent-destroy](https://www.terraform.io/language/meta-arguments/lifecycle#prevent_destroy)
> and
> [ignore-changes](https://www.terraform.io/language/meta-arguments/lifecycle#ignore_changes)
> meta-arguments can be used to prevent accidental data loss.

## Coder apps

Additional IDEs, documentation, or services can be associated to your workspace
using the
[`coder_app`](https://registry.terraform.io/providers/coder/coder/latest/docs/resources/app)
resource.

![Coder Apps in the dashboard](../../../images/admin/templates/coder-apps-ui.png)

Note that some apps are associated to the agent by default as
[`display_apps`](https://registry.terraform.io/providers/coder/coder/latest/docs/resources/agent#nested-schema-for-display_apps)
and can be hidden directly in the
[`coder_agent`](https://registry.terraform.io/providers/coder/coder/latest/docs/resources/agent)
resource. You can arrange the display orientation of Coder apps in your template
using [resource ordering](./resource-ordering.md).

### Coder app examples

<div class="tabs">

You can use these examples to add new Coder apps:

## code-server

```hcl
resource "coder_app" "code-server" {
  agent_id     = coder_agent.main.id
  slug         = "code-server"
  display_name = "code-server"
  url          = "http://localhost:13337/?folder=/home/${local.username}"
  icon         = "/icon/code.svg"
  subdomain    = false
  share        = "owner"
}
```

## Filebrowser

```hcl
resource "coder_app" "filebrowser" {
  agent_id     = coder_agent.main.id
  display_name = "file browser"
  slug         = "filebrowser"
  url          = "http://localhost:13339"
  icon         = "/icon/database.svg"
  subdomain    = true
  share        = "owner"
}
```

## Zed

```hcl
resource "coder_app" "zed" {
    agent_id = coder_agent.main.id
    slug          = "slug"
    display_name  = "Zed"
    external = true
    url      = "zed://ssh/coder.${data.coder_workspace.me.name}"
    icon     = "/icon/zed.svg"
}
```

</div>

Check out our [module registry](https://registry.coder.com/modules) for
additional Coder apps from the team and our OSS community.

## Running scripts on workspace lifecycle

The
[`coder_script`](https://registry.terraform.io/providers/coder/coder/latest/docs/resources/script)
resource runs scripts during workspace lifecycle events like startup, stop, or
on a scheduled basis. It provides more control than the deprecated
`startup_script` field in `coder_agent`.

### When to use coder_script

- **Initialization tasks**: Install dependencies, clone repositories, configure
  services
- **Cleanup tasks**: Stop services gracefully on workspace stop
- **Scheduled maintenance**: Run periodic tasks via cron schedules
- **Blocking startup**: Wait for critical services before allowing user login

### Basic example

```tf
resource "coder_script" "install_dependencies" {
  agent_id           = coder_agent.main.id
  display_name       = "Install Dependencies"
  icon               = "/icon/package.svg"
  script             = <<-EOF
    #!/bin/sh
    set -e
    apt-get update
    apt-get install -y git curl
  EOF
  run_on_start       = true
  start_blocks_login = true
}
```

### Key features

- **Lifecycle control**: Run on start (`run_on_start`), stop (`run_on_stop`),
  or cron schedule (`cron`)
- **Login blocking**: Use `start_blocks_login = true` to ensure critical setup
  completes before user access
- **Timeouts**: Configure `timeout` for long-running scripts
- **Custom icons**: Display meaningful icons with the `icon` parameter
- **Log capture**: Script output is automatically captured and visible in the
  workspace UI

### Advanced patterns

Many [Coder modules](https://registry.coder.com/modules) use `coder_script`
internally. For example:

- [`git-clone`](https://registry.coder.com/modules/coder/git-clone): Clones
  repositories on startup
- [`dotfiles`](https://registry.coder.com/modules/coder/dotfiles): Applies user
  dotfiles
- [`code-server`](https://registry.coder.com/modules/coder/code-server):
  Installs and configures VS Code

You can also reference external script files:

```tf
resource "coder_script" "init_docker" {
  agent_id     = coder_agent.main.id
  display_name = "Initialize Docker"
  script       = file("${path.module}/scripts/init-docker.sh")
  run_on_start = true
}
```

See the
[Coder Terraform provider documentation](https://registry.terraform.io/providers/coder/coder/latest/docs/resources/script)
for complete reference.

<children></children>
