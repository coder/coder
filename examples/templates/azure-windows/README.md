---
display_name: Azure VM (Windows)
description: Provision Azure VMs as Coder workspaces
icon: ../../../site/static/icon/azure.png
maintainer_github: coder
verified: true
tags: [vm, windows, azure]
---

# Remote Development on Azure VMs (Windows)

Provision AWS EC2 VMs as [Coder workspaces](https://coder.com/docs/coder-v2/latest) with this example template.

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

> **Note**
> This template is designed to be a starting point! Edit the Terraform to extend the template to support your use case.
