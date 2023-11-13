# Workspaces

A workspace is the environment that a developer works in. Developers in a team
each work from their own workspace and can use [multiple IDEs](./ides.md).

A developer creates a workspace from a [shared template](./templates/index.md).
This lets an entire team work in environments that are identically configured
and provisioned with the same resources.

## Creating workspaces

You can create a workspace in the UI. Log in to your Coder instance, go to the
**Templates** tab, find the template you need, and select **Create Workspace**.

![Creating a workspace in the UI](./images/creating-workspace-ui.png)

When you create a workspace, you will be prompted to give it a name. You might
also be prompted to set some parameters that the template provides.

You can manage your existing templates in the **Workspaces** tab.

You can also create a workspace from the command line:

Each Coder user has their own workspaces created from
[shared templates](./templates/index.md):

```shell
# create a workspace from the template; specify any variables
coder create --template="<templateName>" <workspaceName>

# show the resources behind the workspace and how to connect
coder show <workspace-name>
```

## Workspace filtering

In the Coder UI, you can filter your workspaces using pre-defined filters or
Coder's filter query. For example, you can find the workspaces that you own or
that are currently running.

The following filters are supported:

- `owner` - Represents the `username` of the owner. You can also use `me` as a
  convenient alias for the logged-in user.
- `template` - Specifies the name of the template.
- `status` - Indicates the status of the workspace. For a list of supported
  statuses, see
  [WorkspaceStatus documentation](https://pkg.go.dev/github.com/coder/coder/codersdk#WorkspaceStatus).

## Starting and stopping workspaces

By default, you manually start and stop workspaces as you need. You can also
schedule a workspace to start and stop automatically.

To set a workspace's schedule, go to the workspace, then **Settings** >
**Schedule**.

![Scheduling UI](./images/schedule.png)

Coder might also stop a workspace automatically if there is a
[template update](./templates/index.md#Start/stop) available.

### Autostart and autostop

Use autostart to start a workspace at a specified time and which days of the
week. Also, you can choose your preferred timezone.

![Autostart UI](./images/autostart.png)

Use autostop to stop a workspace after a number of hours. Autostop won't stop a
workspace if you're still using it. It waits for another hour before checking
again. Coder checks for active connections in the IDE, SSH, Port Forwarding, and
coder_app.

![Autostop UI](./images/autostop.png)

### Max lifetime

Max lifetime is a template setting that determines the number of hours a
workspace will run before Coder automatically stops it, regardless of any active
connections. Use this setting to ensure that workspaces do not run in perpetuity
when connections are left open inadvertently.

## Updating workspaces

After updating the default version of the template that a workspace was created
from, you can update the workspace.

![Updating a workspace](./images/workspace-update.png)

If the workspace is running, Coder stops it, updates it, then starts the
workspace again.

On the command line:

```shell
coder update <workspace-name>
```

## Workspace resources

Workspaces in Coder are started and stopped, often based on whether there was
any activity or if there was a
[template update](./templates/index.md#Start/stop) available.

Resources are often destroyed and re-created when a workspace is restarted,
though the exact behavior depends on the template. For more information, see
[Resource Persistence](./templates/resource-persistence.md).

> ⚠️ To avoid data loss, refer to your template documentation for information on
> where to store files, install software, etc., so that they persist. Default
> templates are documented in
> [../examples/templates](https://github.com/coder/coder/tree/main/examples/templates).
>
> You can use `coder show <workspace-name>` to see which resources are
> persistent and which are ephemeral.

Typically, when a workspace is deleted, all of the workspace's resources are
deleted along with it. Rarely, one may wish to delete a workspace without
deleting its resources, e.g. a workspace in a broken state. Users with the
Template Admin role have the option to do so both in the UI, and also in the CLI
by running the `delete` command with the `--orphan` flag. This option should be
considered cautiously as orphaning may lead to unaccounted cloud resources.

## Repairing workspaces

Use the following command to re-enter template input variables in an existing
workspace. This command is useful when a workspace fails to build because its
state is out of sync with the template.

```shell
coder update <your workspace name> --always-prompt
```

First, try re-entering parameters from a workspace. In the Coder UI, you can
filter your workspaces using pre-defined filters or employing the Coder's filter
query. Take a look at the following examples to understand how to use the
Coder's filter query:

- To find the workspaces that you own, use the filter `owner:me`.
- To find workspaces that are currently running, use the filter
  `status:running`.

![Re-entering template variables](./images/template-variables.png)

You can also do this in the CLI with the following command:

```shell
coder update <your workspace name> --always-prompt
```

If that does not work, a Coder admin can manually push and pull the Terraform
state for a given workspace. This can lead to state corruption or deleted
resources if you do not know what you are doing.

```shell
coder state pull <username>/<workspace name>
# Make changes
coder state push <username>/<workspace name>
```

## Logging

Coder stores macOS and Linux logs at the following locations:

| Service           | Location                         |
| ----------------- | -------------------------------- |
| `startup_script`  | `/tmp/coder-startup-script.log`  |
| `shutdown_script` | `/tmp/coder-shutdown-script.log` |
| Agent             | `/tmp/coder-agent.log`           |

> Note: Logs are truncated once they reach 5MB in size.

## Up next

- Learn about how to personalize your workspace with [Dotfiles](./dotfiles.md)
- Learn about using [IDEs](./ides.md)
