---
display_name: Fly.io Machines
description: Provision Fly.io Machines as Coder workspaces
icon: ../../../site/static/icon/fly.io.svg
maintainer_github: coder
verified: true
tags: [fly, container]
---

# Remote Development on Fly.io Machines

Provision Fly.io Machines as [Coder workspaces](https://coder.com/docs/coder-v2/latest) with this example template.

<!-- TODO: Add screenshot -->

## Prerequisites

- [flyctl](https://fly.io/docs/getting-started/installing-flyctl/) installed.
- [Coder](https://coder.com/) already setup and running with coder-cli installed locally.

### Authentication

1. Run `coder templates init` and select this template. Follow the instructions that appear.
2. cd into the directory that was created. (e.g. `cd fly-docker-image`)
3. Create the new template by running the following command from the `fly-docker-image` directory:

```bash
coder templates create fly-docker-image \
  --var fly_api_token=$(flyctl auth token) \
  --var fly_org=personal
```

> If the Coder server is also running as a fly.io app, then instead of setting variable `fly_api_token` you can also set a fly.io secret with the name `FLY_API_TOKEN`
>
> ```bash
> flyctl secrets set FLY_API_TOKEN=$(flyctl auth token) --app <your-coder-app-name>
> ```

4. Navigate to the Coder dashboard and create a new workspace using the template.

## Architecture

This template provisions the following resources:

- Fly.io Machine (ephemeral, deleted on stop)
- Fly.io Volume (persistent, mounted to `/home/coder`)

This means, when the workspace restarts, any tools or files outside of the home directory are not persisted. To pre-bake tools into the workspace (e.g. `python3`), modify the VM image, or use a [startup script](https://registry.terraform.io/providers/coder/coder/latest/docs/resources/script).

> **Note**
> This template is designed to be a starting point! Edit the Terraform to extend the template to support your use case.

---

Read our blog [post](coder.com/blog/deploying-coder-on-fly-io) to learn more about how to deploy Coder on fly.io.
