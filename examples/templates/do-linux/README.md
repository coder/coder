---
name: Develop in Linux on a Digital Ocean Droplet
description: Get started with Linux development on a Digital Ocean Droplet.
tags: [cloud, digitalocean]
---

# do-linux

This is an example for deploying workspaces as Digital Ocean Droplets.

## Requirements

- Digital Ocean Project ID (e.g. `doctl projects list`)
  - Remove `variable "step2_do_project_id"` and `resource "digitalocean_project_resources" "project"` if you don't want project association.
- (Optional) Digital Ocean SSH key ID (e.g. `doctl compute ssh-key list`)
  - Only required for Fedora images to work.

## Authentication

This template assumes that coderd is run in an environment that is authenticated
with Digital Ocean. Obtain a
[Digital Ocean Personal Access Token](https://cloud.digitalocean.com/account/api/tokens) and set
the environment variable `DIGITALOCEAN_TOKEN` to the access token before starting coderd. For
other ways to authenticate
[consult the Terraform docs](https://registry.terraform.io/providers/digitalocean/digitalocean/latest/docs).
