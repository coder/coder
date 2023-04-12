# Resource Metadata

Expose key workspace information to your users via [`coder_metadata`](https://registry.terraform.io/providers/coder/coder/latest/docs/resources/metadata) resources in your template code.

![ui](../images/metadata-ui.png)

<blockquote class="info">
Coder automatically generates the <code>type</code> metadata.
</blockquote>

You can use `coder_metadata` to show

- Compute resources
- IP addresses
- [Secrets](../secrets.md#displaying-secrets)
- Important file paths

and any other Terraform resource attribute.

## Example

Expose the disk size, deployment name, and persistent
directory in a Kubernetes template with:

```hcl
resource "kubernetes_persistent_volume_claim" "root" {
    ...
}

resource "kubernetes_deployment" "coder" {
  # My deployment is ephemeral
  count = data.coder_workspace.me.start_count
  ...
}

resource "coder_metadata" "pvc" {
  resource_id = kubernetes_persistent_volume_claim.root.id
  item {
    key = "size"
    value = kubernetes_persistent_volume_claim.root.spec[0].resources[0].requests.storage
  }
  item {
    key = "dir"
    value = "/home/coder"
  }
}

resource "coder_metadata" "deployment" {
  count = data.coder_workspace.me.start_count
  resource_id = kubernetes_deployment.coder[0].id
  item {
    key = "name"
    value = kubernetes_deployment.coder[0].metadata[0].name
  }
}
```

## Hiding resources in the UI

Some resources don't need to be exposed in the UI; this helps keep the workspace view clean for developers. To hide a resource, use the `hide` attribute:

```hcl
resource "coder_metadata" "hide_serviceaccount" {
  count = data.coder_workspace.me.start_count
  resource_id = kubernetes_service_account.user_data.id
  hide = true
  item {
    key = "name"
    value = kubernetes_deployment.coder[0].metadata[0].name
  }
}
```

## Using custom resource icon

To use custom icons on your resources, use the `icon` attribute (must be a valid path or URL):

```hcl
resource "coder_metadata" "resource_with_icon" {
  count = data.coder_workspace.me.start_count
  resource_id = kubernetes_service_account.user_data.id
  icon = "/icon/database.svg"
  item {
    key = "name"
    value = kubernetes_deployment.coder[0].metadata[0].name
  }
}
```

To make easier for you to customize your resource we added some built-in icons:

- Folder `/icon/folder.svg`
- Memory `/icon/memory.svg`
- Image `/icon/image.svg`
- Widgets `/icon/widgets.svg`
- Database `/icon/database.svg`

We also have other icons related to the IDEs. You can see all the icons [here](https://github.com/coder/coder/tree/main/site/static/icon).

## Agent Metadata

In cases where you want to present automatically updating, dynamic values. You
can use the `metadata` block in the `coder_agent` resource. For example:

```hcl
resource "coder_agent" "dev" {
  os   = "linux"
  arch = "amd64"
  dir  = "/workspace"
  metadata {
    name = "Process Count"
    script = "ps aux | wc -l"
    interval = 1
    timeout = 3
  }
}
```

Read more [here](./agent-metadata.md).

## Up next

- Learn about [secrets](../secrets.md)
- Learn about [Agent Metadata](./agent-metadata.md)
