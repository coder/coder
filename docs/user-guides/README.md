# Workspaces

### What is a Workspace?

At the highest level, a workspace is a set of cloud resources. These resources
can be VMs, Kubernetes clusters, storage buckets, or whatever else [Terraform](https://developer.hashicorp.com/terraform/docs)
lets you dream up.

The resources that run the agent are described as _computational resources_,
while those that don't are called _peripheral resources_.

Each resource may also be _persistent_ or _ephemeral_ depending on whether
they're destroyed on workspace stop.

Coder Workspaces are managed by the workspace agent, which facilitates [connections](./workspace-access.md).

## Viewing workspaces

You can manage your existing workspaces in the **Workspaces** tab. The name, associated template, and status are shown for each workspace. You can pin workspaces to the top of this UI by marking them as "favorite."

![Workspace listing UI](../images/user-guides/workspace-list-ui.png)

## Creating workspaces

You can create a workspace in the UI. Log in to your Coder instance, go to the
**Templates** tab, find the template you need, and select **Create Workspace**.

![Creating a workspace in the UI](./images/user-guides/create-workspace-ui.png)

When you create a workspace, you will be prompted to give it a name. You might
also be prompted to set some [parameters](#workspace-parameters) that the template provides.

You can also create a workspace from the command line:

Each Coder user has their own workspaces created from
[shared templates](./admin/templates/README.md):

```shell
# create a workspace from the template; specify any variables
coder create --template="<templateName>" <workspaceName>

# show the resources behind the workspace and how to connect
coder show <workspace-name>
```

## Updating workspaces

After updating the default version of the template that a workspace was created
from, you can update the workspace.

![Updating a workspace](../../images/workspace-update.png)

If the workspace is running, Coder stops it, updates it, then starts the
workspace again.

On the command line:

```shell
coder update <workspace-name>
```

### Automatic updates

It can be tedious to manually update a workspace everytime an update is pushed
to a template. Users can choose to opt-in to automatic updates to update to the
active template version whenever the workspace is started.

Note: If a template is updated such that new parameter inputs are required from
the user, autostart will be disabled for the workspace until the user has
manually updated the workspace.

![Automatic Updates](./images/workspace-automatic-updates.png)

### Update policies

Template admins can require workspaces be on the latest version before starting. When this is enabled, you will be presented with an "Update and Start" button in the UI. Workspaces that start on connect will automatically update on the first out-of-date connection.

## Bulk operations (enterprise)

Enterprise users may apply bulk operations (update, delete, start, stop) in the **Workspaces** tab.

Select the workspaces you'd like to modify with the checkboxes on the left, then use the top-right **Actions** dropdown to apply the operation.

![Bulk workspace actions](../images/user-guides/workspace-bulk-actions.png)

## Next steps

- [Access your workspace](./workspace-access/README.md)
- [Learn about templates](./admin/templates/README.md)
- [Try Coder](../start/coder-tour.md)
