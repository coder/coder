---
display_name: DigitalOcean Droplet (Linux)
description: Provision DigitalOcean Droplets as Coder workspaces
icon: ../../../site/static/icon/do.png
maintainer_github: coder
verified: true
tags: [vm, linux, digitalocean]
---

# Remote Development on DigitalOcean Droplets

Provision DigitalOcean Droplets as [Coder workspaces](https://coder.com/docs/coder-v2/latest) with this example template.

<!-- TODO: Add screenshot -->

## Prerequisites

To deploy workspaces as DigitalOcean Droplets, you'll need:

- DigitalOcean [personal access token (PAT)](https://docs.digitalocean.com/reference/api/create-personal-access-token/)

- DigitalOcean project ID (you can get your project information via the `doctl`
  CLI by running `doctl projects list`)

- Remove the following sections from the `main.tf` file if you don't want to
  associate your workspaces with a project:

  - `variable "step2_do_project_id"`
  - `resource "digitalocean_project_resources" "project"`

- **Optional:** DigitalOcean SSH key ID (obtain via the `doctl` CLI by running
  `doctl compute ssh-key list`)

- Note that this is only required for Fedora images to work.

### Authentication

This template assumes that coderd is run in an environment that is authenticated
with Digital Ocean. Obtain a [Digital Ocean Personal Access
Token](https://cloud.digitalocean.com/account/api/tokens) and set the
environment variable `DIGITALOCEAN_TOKEN` to the access token before starting
coderd. For other ways to authenticate [consult the Terraform docs](https://registry.terraform.io/providers/digitalocean/digitalocean/latest/docs).

## Architecture

This template provisions the following resources:

- Azure VM (ephemeral, deleted on stop)
- Managed disk (persistent, mounted to `/home/coder`)

This means, when the workspace restarts, any tools or files outside of the home directory are not persisted. To pre-bake tools into the workspace (e.g. `python3`), modify the VM image, or use a [startup script](https://registry.terraform.io/providers/coder/coder/latest/docs/resources/script).

> **Note**
> This template is designed to be a starting point! Edit the Terraform to extend the template to support your use case.
