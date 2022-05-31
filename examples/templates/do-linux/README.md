---
name: Develop in Linux on a Digital Ocean Droplet
description: Get started with Linux development on a Digital Ocean Droplet.
tags: [cloud, digitalocean]
---

# do-linux

This is an example for deploying workspaces as Digital Ocean Droplets.

## Requirements

- Digital Ocean Personal Access Token (PAT)
- Digital Ocean Project ID (e.g. `doctl projects list`)
  - Remove `variable "step2_do_project_id"` and `resource "digitalocean_project_resources" "project"` if you don't want project association.
- (Optional) Digital Ocean SSH key ID (e.g. `doctl compute ssh-key list`)
  - Only required for Fedora images to work.
