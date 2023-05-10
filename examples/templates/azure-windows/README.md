---
name: Develop in Windows on Azure
description: Get started with Windows development on Microsoft Azure.
tags: [cloud, azure, windows]
icon: /icon/azure.png
---

# azure-windows

To get started, run `coder templates init`. When prompted, select this template.
Follow the on-screen instructions to proceed.

## Authentication

This template assumes that coderd is run in an environment that is authenticated
with Azure. For example, run `az login` then `az account set --subscription=<id>`
to import credentials on the system and user running coderd. For other ways to
authenticate [consult the Terraform docs](https://registry.terraform.io/providers/hashicorp/azurerm/latest/docs#authenticating-to-azure).

## Dependencies

This template depends on the Azure CLI tool (`az`) to start and stop the Windows VM. Ensure this
tool is installed and available in the path on the machine that runs coderd.
