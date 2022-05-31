---
name: Develop in Windows on Google Cloud
description: Get started with Windows development on Google Cloud.
tags: [cloud, google]
---

# gcp-windows

## Getting Started

Run `coder templates init`, and when prompted, select this template. Follow the
on-screen instructions to proceed.

## Service account

Coder requires a Google Cloud Service Account to provision workspaces. To create
a service account:

1. Navigate to the [CGP console](https://console.cloud.google.com/projectselector/iam-admin/serviceaccounts/create).
2. Add the following roles:
   - Compute Admin
   - Service Account User
3. Click on the created key, and navigate to the **Keys** tab.
4. Click **Add key** > **Create new key**.
5. Generate a **JSON private key**, which will be what you provide to coder.
