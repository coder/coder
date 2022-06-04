---
name: Develop in Windows on Google Cloud
description: Get started with Windows development on Google Cloud.
tags: [cloud, google]
---

# gcp-windows

## Getting started

Pick this template in `coder templates init` and follow instructions.

## Authentication

This template assumes that coderd is run in an environment that is authenticated
with Google Cloud. For example, run `gcloud auth application-default login` to import
credentials on the system and user running coderd.  For other ways to authenticate
[consult the Terraform docs](https://registry.terraform.io/providers/hashicorp/google/latest/docs/guides/getting_started#adding-credentials).

## Required permissions / policy

The user or service account used by the Terraform provisioner should have the following roles

- Compute Admin
