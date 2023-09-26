---
display_name: Azure VM (Linux)
description: Provision Azure VMs as Coder workspaces
icon: ../../../site/static/icon/azure.png
maintainer_github: coder
verified: true
tags: [vm, linux, azure]
---

# Remote Development on Azure VMs (Linux)

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
- Managed disk (persistent, mounted to `/home/coder`)

This means, when the workspace restarts, any tools or files outside of the home directory are not persisted. To pre-bake tools into the workspace (e.g. `python3`), modify the VM image, or use a [startup script](https://registry.terraform.io/providers/coder/coder/latest/docs/resources/script). Alternatively, individual developers can [personalize](https://coder.com/docs/v2/latest/dotfiles) their workspaces with dotfiles.

> **Note**
> This template is designed to be a starting point! Edit the Terraform to extend the template to support your use case.

## code-server

`code-server` is installed via the `startup_script` argument in the `coder_agent`
resource block. The `coder_app` resource is defined to access `code-server` through
the dashboard UI over `localhost:13337`.
