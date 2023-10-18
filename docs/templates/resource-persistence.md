# Resource persistence

By default, all Coder resources are persistent, but production templates
**must** use the practices laid out in this document to prevent accidental
deletion.

Coder templates have full control over workspace ephemerality. In a completely
ephemeral workspace, there are zero resources in the Off state. In a completely
persistent workspace, there is no difference between the Off and On states.

The needs of most workspaces fall somewhere in the middle, persisting user data
like filesystem volumes, but deleting expensive, reproducible resources such as
compute instances.

## Disabling persistence

The Terraform
[`coder_workspace` data source](https://registry.terraform.io/providers/coder/coder/latest/docs/data-sources/workspace)
exposes the `start_count = [0 | 1]` attribute. To make a resource ephemeral, you
can assign the `start_count` attribute to resource's
[`count`](https://developer.hashicorp.com/terraform/language/meta-arguments/count)
meta-argument.

In this example, Coder will provision or tear down the `docker_container`
resource:

```hcl
data "coder_workspace" "me" {
}

resource "docker_container" "workspace" {
  # When `start_count` is 0, `count` is 0, so no `docker_container` is created.
  count = data.coder_workspace.me.start_count # 0 (stopped), 1 (started)
  # ... other config
}
```

## ⚠️ Persistence pitfalls

Take this example resource:

```hcl
data "coder_workspace" "me" {
}

resource "docker_volume" "home_volume" {
  name = "coder-${data.coder_workspace.me.owner}-home"
}
```

Because we depend on `coder_workspace.me.owner`, if the owner changes their
username, Terraform will recreate the volume (wiping its data!) the next time
that Coder starts the workspace.

To prevent this, use immutable IDs:

- `coder_workspace.me.owner_id`
- `coder_workspace.me.id`

```hcl
data "coder_workspace" "me" {
}

resource "docker_volume" "home_volume" {
  # This volume will survive until the Workspace is deleted or the template
  # admin changes this resource block.
  name = "coder-${data.coder_workspace.id}-home"
}
```

## 🛡 Bulletproofing

Even if your persistent resource depends exclusively on immutable IDs, a change
to the `name` format or other attributes would cause Terraform to rebuild the
resource.

You can prevent Terraform from recreating a resource under any circumstance by
setting the
[`ignore_changes = all` directive in the `lifecycle` block](https://developer.hashicorp.com/terraform/language/meta-arguments/lifecycle#ignore_changes).

```hcl
data "coder_workspace" "me" {
}

resource "docker_volume" "home_volume" {
  # This resource will survive until either the entire block is deleted
  # or the workspace is.
  name = "coder-${data.coder_workspace.me.id}-home"
  lifecycle {
    ignore_changes = all
  }
}
```
