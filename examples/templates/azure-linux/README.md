---
display_name: Azure VM (Linux)
description: Provision Azure VMs as Coder workspaces
icon: ../../../site/static/icon/azure.png
maintainer_github: coder
verified: true
tags: [vm, linux, azure]
---

# Remote Development on Azure VMs (Linux)

Provision Azure Linux VMs as [Coder workspaces](https://coder.com/docs/workspaces) with this example template.

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
- Managed disk (persistent, mounted to `/home/coder`)
- Resource group, virtual network, subnet, and network interface (persistent, required by the managed disk and VM)

### What happens on stop

When a workspace is **stopped**, only the VM is destroyed. The managed disk, resource group, virtual network, subnet, and network interface all persist. This is by design — the managed disk retains your `/home/coder` data across workspace restarts, and the other resources remain because the disk depends on them.

This means you will see these Azure resources in your subscription even when a workspace is stopped. This is expected behavior.

### What happens on delete

When a workspace is **deleted**, all resources are destroyed, including the resource group, networking resources, and managed disk.

### Workspace restarts

Since the VM is ephemeral, any tools or files outside of the home directory are not persisted across restarts. To pre-bake tools into the workspace (e.g. `python3`), modify the VM image, or use a [startup script](https://registry.terraform.io/providers/coder/coder/latest/docs/resources/script). Alternatively, individual developers can [personalize](https://coder.com/docs/dotfiles) their workspaces with dotfiles.

> [!NOTE]
> This template is designed to be a starting point! Edit the Terraform to extend the template to support your use case.


### Persistent VM

> [!IMPORTANT]  
> This approach requires the [`az` CLI](https://learn.microsoft.com/en-us/cli/azure/install-azure-cli#install) to be present in the PATH of your Coder Provisioner.
> You will have to do this installation manually as it is not included in our official images.

It is possible to make the VM persistent (instead of ephemeral) by removing the `count` attribute in the `azurerm_linux_virtual_machine` resource block as well as adding the following snippet:

```hcl
# Stop the VM
resource "null_resource" "stop_vm" {
  count      = data.coder_workspace.me.transition == "stop" ? 1 : 0
  depends_on = [azurerm_linux_virtual_machine.main]
  provisioner "local-exec" {
    # Use deallocate so the VM is not charged
    command = "az vm deallocate --ids ${azurerm_linux_virtual_machine.main.id}"
  }
}

# Start the VM
resource "null_resource" "start" {
  count      = data.coder_workspace.me.transition == "start" ? 1 : 0
  depends_on = [azurerm_linux_virtual_machine.main]
  provisioner "local-exec" {
    command = "az vm start --ids ${azurerm_linux_virtual_machine.main.id}"
  }
}
```
