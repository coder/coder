# Workspaces

Workspaces contain the dependencies, IDEs, and configuration information needed for software development.

## Create workspaces

Each Coder user has their own workspaces, created from a shared [template](./templates.md).

```sh
# create a workspace from template, specify any variables
coder workspaces create <workspace-name>

# show the resources behind the workspace, and how to connect
coder workspaces show <workspace-name>
```

## Connect with SSH

Once Coder workspaces are added to your SSH hosts, you can connect from any IDE with remote development support.

```sh
coder config-ssh

ssh coder.<workspace-name>
```

## Editors and IDEs

The following desktop IDEs have been tested with Coder. Any IDE with SSH support should work!

- VS Code (with [Remote - SSH](https://marketplace.visualstudio.com/items?itemName=ms-vscode-remote.remote-ssh) extension)
- JetBrains (with [Gateway](https://www.jetbrains.com/help/idea/remote-development-a.html#launch_gateway) installed)
  - IntelliJ IDEA
  - CLion
  - GoLand
  - PyCharm
  - Rider
  - RubyMine
  - WebStorm

## Workspace lifecycle

Workspaces in Coder are started and stopped, often based on activity or when a [template update](./templates.md#manage-templates) is available.

While the exact behavior depends on the template, resources are often destroyed and re-created when a workspace is restarted. For more details, see [persistent and ephemeral resources](./templates.md#persistent-and-ephemeral-resources).

> ⚠️ To avoid data loss, reference your template documentation to see where to store files, install software, etc. Default templates are documented in [../examples](../examples/).
>
> You can use `coder workspace show <workspace-name>` to see which resources are persistent vs ephemeral.

When a workspace is deleted, all of the workspace's resources are deleted.

## Updating workspaces

Use the following command to update a workspace to the latest version of a template. The workspace will be stopped and started.

```sh
coder workspaces update <workspace-name>
```
