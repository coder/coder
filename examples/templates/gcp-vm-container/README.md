---
display_name: Google Compute Engine (VM Container)
description: Provision Google Compute Engine instances as Coder workspaces
icon: ../../../site/static/icon/gcp.png
maintainer_github: coder
verified: true
tags: [vm-container, linux, gcp]
---

# Remote Development on Google Compute Engine (VM Container)

## Prerequisites

### Authentication

This template assumes that coderd is run in an environment that is authenticated
with Google Cloud. For example, run `gcloud auth application-default login` to
import credentials on the system and user running coderd. For other ways to
authenticate [consult the Terraform
docs](https://registry.terraform.io/providers/hashicorp/google/latest/docs/guides/getting_started#adding-credentials).

Coder requires a Google Cloud Service Account to provision workspaces. To create
a service account:

1. Navigate to the [CGP
   console](https://console.cloud.google.com/projectselector/iam-admin/serviceaccounts/create),
   and select your Cloud project (if you have more than one project associated
   with your account)

1. Provide a service account name (this name is used to generate the service
   account ID)

1. Click **Create and continue**, and choose the following IAM roles to grant to
   the service account:

   - Compute Admin
   - Service Account User

   Click **Continue**.

1. Click on the created key, and navigate to the **Keys** tab.

1. Click **Add key** > **Create new key**.

1. Generate a **JSON private key**, which will be what you provide to Coder
   during the setup process.

## Architecture

This template provisions the following resources:

- GCP VM (ephemeral, deleted on stop)
  - Container in VM
- Managed disk (persistent, mounted to `/home/coder` in container)

This means, when the workspace restarts, any tools or files outside of the home directory are not persisted. To pre-bake tools into the workspace (e.g. `python3`), modify the container image, or use a [startup script](https://registry.terraform.io/providers/coder/coder/latest/docs/resources/script).

> **Note**
> This template is designed to be a starting point! Edit the Terraform to extend the template to support your use case.

## code-server

`code-server` is installed via the `startup_script` argument in the `coder_agent`
resource block. The `coder_app` resource is defined to access `code-server` through
the dashboard UI over `localhost:13337`.
