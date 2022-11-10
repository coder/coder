# Quotas

Coder Enterprise admins may define deployment-level quotas to control costs
and ensure equitable access to cloud resources.

Quotas are available to any license with support for [Groups](./groups.md).

Templates describe their quota cost through [`resource_metadata`](../templates/resource-metadata.md).

Coder checks for Quota availability on workspace create, start, and stop
operations.

When a user creates a workspace, the `resource_metadata.cost` fields are summed up,

Then, when users create workspaces they would see:

<img src="../images/admin/quotas.png"/>

## Enabling this feature

This feature is only available with an enterprise license. [Learn more](../enterprise.md)

## Up next

- [Enterprise](../enterprise.md)
- [Configuring](./configure.md)
