---
name: Develop in Linux on a Digital Ocean Droplet
description: Get started with Linux development on a Digital Ocean Droplet.
tags: [cloud, digitalocean]
---

# do-linux

To deploy workspaces as DigitalOcean Droplets, you'll need:

- DigitalOcean [personal access token (PAT)](https://docs.digitalocean.com/reference/api/create-personal-access-token/)

- DigitalOcean project ID (you can get your project information via the `doctl` CLI by running `doctl projects list`)

  - Remove the following sections from the `main.tf` file if you don't want to
    associate your workspaces with a project:

    - `variable "step2_do_project_id"`
    - `resource "digitalocean_project_resources" "project"`

- **Optional:** DigitalOcean SSH key ID (obtain via the `doctl` CLI by running `doctl compute ssh-key list`)

  - Note that this is only required for Fedora images to work.
