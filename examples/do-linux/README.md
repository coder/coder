---
name: Develop in Linux on a Digital Ocean Droplet
description: Get started with Linux development on a Digital Ocean Droplet.
tags: [cloud, digitalocean]
---

# do-droplet-linux

This is an example for deploying workspaces on Digital Ocean Droplets.

## Requirements

- Digital Ocean Personal Access Token (PAT)
- Digital Ocean Project ID (e.g. `doctl projects list`)
  - Remove `variable "step2_do_project_id"` and `resource "digitalocean_project_resources" "project"` if you don't want project association.
