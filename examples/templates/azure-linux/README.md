---
display_name: Azure VM (Linux)
description: Provision Azure VMs as Coder workspaces
icon: ../../../site/static/icon/azure.png
maintainer_github: coder
verified: true
tags: [vm, linux, azure]
---

# azure-linux

To get started, run `coder templates init`. When prompted, select this template.
Follow the on-screen instructions to proceed.

## Authentication

This template assumes that coderd is run in an environment that is authenticated
with Azure. For example, run `az login` then `az account set --subscription=<id>`
to import credentials on the system and user running coderd. For other ways to
authenticate [consult the Terraform docs](https://registry.terraform.io/providers/hashicorp/azurerm/latest/docs#authenticating-to-azure).
