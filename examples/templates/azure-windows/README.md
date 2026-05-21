---
display_name: Azure VM (Windows)
description: Provision Azure VMs as Coder workspaces
icon: ../../../site/static/icon/azure.png
maintainer_github: coder
verified: true
tags: [vm, windows, azure]
---

# Remote Development on Azure VMs (Windows)

Provision Azure Windows VMs as [Coder workspaces](https://coder.com/docs/workspaces) with this example template.

<!-- TODO: Add screenshot -->

## Prerequisites

### Authentication

This template assumes that coderd is run in an environment that is authenticated
with Azure. For example, run `az login` then `az account set --subscription=<id>`
to import credentials on the system and user running coderd. For other ways to
authenticate, [consult the Terraform docs](https://registry.terraform.io/providers/hashicorp/azurerm/latest/docs#authenticating-to-azure).

## Architecture

This template provisions the following resources:

- Azure VM (ephemeral, deleted on stop)
- Managed disk (persistent, mounted to `F:`)

This means, when the workspace restarts, any tools or files outside of the data directory are not persisted. To pre-bake tools into the workspace (e.g. `python3`), modify the VM image, or use a [startup script](https://registry.terraform.io/providers/coder/coder/latest/docs/resources/script).

> [!NOTE]
> This template is designed to be a starting point! Edit the Terraform to extend the template to support your use case.

### Persistent VM

> [!IMPORTANT]  
> This approach requires the [`az` CLI](https://learn.microsoft.com/en-us/cli/azure/install-azure-cli#install) to be present in the PATH of your Coder Provisioner.
> You will have to do this installation manually as it is not included in our official images.

It is possible to make the VM persistent (instead of ephemeral) by removing the `count` attribute in the `azurerm_windows_virtual_machine` resource block as well as adding the following snippet:

```hcl
# Stop the VM
resource "null_resource" "stop_vm" {
  count      = data.coder_workspace.me.transition == "stop" ? 1 : 0
  depends_on = [azurerm_windows_virtual_machine.main]
  provisioner "local-exec" {
    # Use deallocate so the VM is not charged
    command = "az vm deallocate --ids ${azurerm_windows_virtual_machine.main.id}"
  }
}

# Start the VM
resource "null_resource" "start" {
  count      = data.coder_workspace.me.transition == "start" ? 1 : 0
  depends_on = [azurerm_windows_virtual_machine.main]
  provisioner "local-exec" {
    command = "az vm start --ids ${azurerm_windows_virtual_machine.main.id}"
  }
}
```
