---
display_name: DigitalOcean Droplet (Linux)
description: Provision DigitalOcean Droplets as Coder workspaces
icon: ../../../site/static/icon/do.png
maintainer_github: coder
verified: true
tags: [vm, linux, digitalocean]
---

# Remote Development on DigitalOcean Droplets

Provision DigitalOcean Droplets as [Coder workspaces](https://coder.com/docs/user-guides/workspace-management) with this example template.

<!-- prerequisites:start -->

## Prerequisites

To deploy workspaces as DigitalOcean Droplets, you'll need:

- DigitalOcean [personal access token (PAT)](https://docs.digitalocean.com/reference/api/create-personal-access-token)

- DigitalOcean project ID, which the template builder prompts for. You can get
  your project information via the `doctl` CLI by running `doctl projects list`.

- **Optional:** DigitalOcean SSH key ID, which the template builder prompts for
  (obtain via the `doctl` CLI by running `doctl compute ssh-key list`).

  - Note that this is only required for Fedora images to work. Leave it as `0`
    if you don't need it.

### Authentication

This template assumes that the Coder Provisioner is run in an environment that is authenticated with Digital Ocean.

Obtain a [Digital Ocean Personal Access Token](https://cloud.digitalocean.com/account/api/tokens) and set the `DIGITALOCEAN_TOKEN` environment variable to the access token.
For other ways to authenticate [consult the Terraform provider's docs](https://registry.terraform.io/providers/digitalocean/digitalocean/latest/docs).

<!-- prerequisites:end -->

## Architecture

This template provisions the following resources:

- DigitalOcean VM (ephemeral, deleted on stop)
- Managed disk (persistent, mounted to `/home/coder`)

This means, when the workspace restarts, any tools or files outside of the home directory are not persisted. To pre-bake tools into the workspace (e.g. `python3`), modify the VM image, or use a [startup script](https://registry.terraform.io/providers/coder/coder/latest/docs/resources/script).

> [!NOTE]
> This template is designed to be a starting point! Edit the Terraform to extend the template to support your use case.
