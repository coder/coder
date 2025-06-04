# Creating your first coder workspace

A workspace is the environment that a developer works in. Developers in a team
each work from their own workspace and can use
[multiple IDEs](../user-guides/workspace-access/index.md).

A developer creates a workspace from a
[shared template](../admin/templates/index.md). This lets an entire team work in
environments that are identically configured and provisioned with the same
resources.

## Before you begin

This guide will use the Docker template from the
[previous step](../tutorials/template-from-scratch.md) to create and connect to
a Coder workspace.

## 1. Create a workspace from your template through the GUI

You can create a workspace in the UI. Log in to your Coder instance, go to the
**Templates** tab, find the template you need, and select **Create Workspace**.

![Template Preview](../images/start/template-preview.png)

In **New workspace**, fill in **Name** then scroll down to select **Create
Workspace**.

![Create Workspace](../images/start/create-workspace.png)

Coder starts your new workspace from your template.

After a few seconds, your workspace is ready to use.

![Workspace is ready](../images/start/workspace-ready.png)

## 2. Try out your new workspace

The Docker starter template lets you connect to your workspace in a few ways:

- VS Code Desktop: Loads your workspace into
  [VS Code Desktop](https://code.visualstudio.com/Download) installed on your
  local computer.
- code-server: Opens
  [browser-based VS Code](../user-guides/workspace-access/web-ides.md#code-server)
  with your workspace.
- Terminal: Opens a browser-based terminal with a shell in the workspace's
  Docker instance.
- JetBrains Gateway: Opens JetBrains IDEs via JetBrains Gateway.
- SSH: Use SSH to log in to the workspace from your local machine. If you
  haven't already, you'll have to install Coder on your local machine to
  configure your SSH client.

> [!TIP]
> You can edit the template to let developers connect to a workspace in
> [a few more ways](../admin/templates/extending-templates/web-ides.md).

## 3. Modify your workspace settings

Developers can modify attributes of their workspace including update policy,
scheduling, and parameters which define their development environment.

Once you're finished, you can stop your workspace.

## Next Steps

- Creating workspaces with the [CLI](../reference/cli/create.md)
- Creating workspaces with the [API](../reference/api/workspaces.md)
