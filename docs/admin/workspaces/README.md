# Administering workspaces

<!-- 
Layout of admin/workspaces/

README.md
lifecycle.md
update-policies.md
multiple-agents.md
auditing.md
 -->

Coder manages Workspaces, which are user-facing virtualized development environments. Each workspace is defined by a [template](../templates/README.md), owned by a single user, and can be individually modified with parameters and scheduling.

Coder allows workpsaces to be hosted on either VMs or containers, and is unopionated on which compute you choose to maximize flexibility.

![Example workspace view](../../images/user-guides/workspace-list-ui.png)

> If you are an end-user of Coder looking to learn more about how to use and manage the workspaces you own, see our [user guides](../../user-guides/README.md).

## Viewing and Filtering workspaces

Admins have visibility for every workspace in a deployment under the **Workspaces** tab. The name, associated template, and status are shown for each workspace.

![Workspace listing UI](../images/user-guides/workspace-list-ui.png)

You can filter these workspaces using pre-defined filters or
Coder's filter query. For example, you can find the workspaces that you own or
that are currently running.

The following filters are supported:

- `owner` - Represents the `username` of the owner. You can also use `me` as a
  convenient alias for the logged-in user.
- `template` - Specifies the name of the template.
- `status` - Indicates the status of the workspace. For a list of supported
  statuses, see
  [WorkspaceStatus documentation](https://pkg.go.dev/github.com/coder/coder/codersdk#WorkspaceStatus).


## Updating workspaces

### Bulk updates

### Workspace update policies

## Multiple workspace agents

## Workspace scheduling



TODO: Write index

