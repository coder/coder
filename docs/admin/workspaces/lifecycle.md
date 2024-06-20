# Workspace lifecycle

<!-- TODO: Make a sexier opener -->
Workspaces are flexible, reproducible, and isolated units of compute. Workspaces are created via Terraform, managed through the Coder control plane, accessed through the Coder agent, then stopped and deleted again by Terraform. 

This page covers how workspaces move through this lifecycle. To learn about automating workspace schedules for cost control, read the [workspace scheduling docs](./schedule.md).

## Resource persistence

In Coder, your workspaces are composed of ephemeral and persistent resources. Persistent resources stay provisioned when the workspace is stopped, where as ephemeral resources are destroyed and recreated on restart. All resources are destroyed when a workspace is deleted.

A common example is to have a workspace whose only persistent resource is the home directory. This allows the developer to retain their work while ensuring the rest of their environment is consistently up-to-date on each workspace restart.

The persistence of resources in your workspace is determined by Terraform in your template. Read more from the official documentation on [Terraform resource behavior](https://developer.hashicorp.com/terraform/language/resources/behavior#how-terraform-applies-a-configuration) and how to configure it using the [lifecycle argument](https://developer.hashicorp.com/terraform/language/meta-arguments/lifecycle).

## Workspace States

Generally, there are 3 states that a workspace may fall into:
- Running: Started and ready for connections
- Stopped: Ephemeral resources destroyed, persistent resources idle
- Deleted: All resources destroyed, workspace records removed from database

If some error occurs during the above, a workspace may fall into one of the following broken states:
- Failed: Failure during provisioning, no resource consumption
- Unhealthy: Resources have been provisioned, but the agent can't facilitate connections

## Workspace creation

Workspaces can be created from [templates](../templates/README.md) via the CLI, API, or dashboard. To learn how, read our [user guides](../../user-guides/README.md). 

By default, there is no limit on the number of workspaces a user may create, regardless of the template's resource demands. Enterprise administrators may limit the number of workspaces per template or group using quotas to prevent over provisioning and control costs.

<!-- TODO: Quota link -->

When a user creates a workspace, they're sending a build request to the control plane. Coder takes this and uses [Terraform](https://www.terraform.io/) to provision a workspace defined by your [template](../templates/README.md). Generally, templates define the resources and environment of a workspace.  

Once the workspace is provisioned, the agent process starts and opens connections to your workspace via SSH, the terminal, and IDES like [JetBrains](../../user-guides/workspace-access/jetbrains.md) or [VSCode](../../user-guides/workspace-access/vscode.md). 

The agent is responsible for running your workspace startup scripts. These may configure tools, service connections, or personalization like [dotfiles](../../user-guides/workspace-dotfiles.md).

## Stopping workspaces

Workspaces may be stopped by a number of mechanisms. 

## Workspace deletion


## Ephemeral workspaces


### Dormant workspaces



## Next steps
<!--
TODO:
- connecting to your workspace
- writing templates
- workspace scheduling
-->
