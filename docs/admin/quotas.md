# Quotas

Coder Enterprise admins may define deployment-level quotas to control costs
and ensure equitable access to cloud resources. The quota system currently controls
instanteneous cost. For example, the system can ensure that every user in your
deployment has a spend rate lower than $100/month at any given moment.

Enforcement occurs on workspace create, start, and stop operations. If a user
hits their quota, they can always unblock themselves by stopping or deleting
one of their workspaces.

Quotas are licensed with [Groups](./groups.md).

## Definitions

- **Credits** are the fundamental units of the quota system. They map to the
  smallest denomination of your preferred currency. For example, if you work with USD,
  think of each credit as a cent.
- **Budget** is the enforced, upper limit of credit spend

## Establishing Costs

Templates describe their daily cost through the `daily_cost` attribute in
[`resource_metadata`](https://registry.terraform.io/providers/coder/coder/latest/docs/resources/metadata).
Since costs are defined with each resource, an offline workspace may consume
less quota than an online workspace.

A common Coder use case is workspaces with persistent storage and ephemeral
compute. For example:

```hcl
resource "coder_metadata" "volume" {
    resource_id = "${docker_volume.home_volume.id}"
    cost = 10
}

resource "docker_volume" "home_volume" {
  name = "coder-${data.coder_workspace.me.owner}-${data.coder_workspace.me.name}-root"
}

resource "coder_metadata" "container" {
    resource_id = "${docker_container.workspace.id}"
    cost = 20
}

resource "docker_container" "workspace" {
  count = data.coder_workspace.me.start_count
  image = "codercom/code-server:latest"
  ...
  volumes {
    container_path = "/home/coder/"
    volume_name    = docker_volume.home_volume.name
    read_only      = false
  }
}
```

In that template, the workspace consumes 10 quota credits when it's offline, and
30 when it's online.

Coder checks for Quota availability on workspace create, start, and stop
operations.

When a user creates a workspace, the `resource_metadata.cost` fields are summed up,

Then, when users create workspaces they would see:

<img src="../images/admin/quotas.png"/>

## Quota Enforcement

## Up next

- [Enterprise](../enterprise.md)
- [Configuring](./configure.md)
