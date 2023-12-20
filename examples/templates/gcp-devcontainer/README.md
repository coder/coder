---
display_name: Google Compute Engine (Devcontainer)
description: Provision a Devcontainer on Google Compute Engine instances as Coder workspaces
icon: ../../../site/static/icon/gcp.png
maintainer_github: coder
verified: true
tags: [vm, linux, gcp, devcontainer]
---

# Remote Development in a Devcontainer on Google Compute Engine

![Architecture Diagram](./architecture.svg)

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

- GCP VM (persistent)
- GCP Disk (persistent, mounted to root)

Coder persists the root volume. The full filesystem is preserved when the workspace restarts.

> **Note**
> This template is designed to be a starting point! Edit the Terraform to extend the template to support your use case.

## code-server

`code-server` is installed via the [`code-server`](https://registry.coder.com/modules/code-server) registry module. Please check [Coder Registry](https://registry.coder.com) for a list of all modules and templates.
