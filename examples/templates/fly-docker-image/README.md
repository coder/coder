---
name: Develop on a Fly.io container
description: Run workspaces as Firecracker VMs on Fly.io
tags: [docker, fly.io]
icon: /icon/fly.io.svg
---

# Coder Fly.io Template

This template provisions a [code-server](https://github.com/coder/code-server) instance on [fly.io](https://fly.io) using the [codercom/code-server](https://hub.docker.com/r/codercom/code-server) image.

## Prerequisites

- [flyctl](https://fly.io/docs/getting-started/installing-flyctl/) installed.
- [Coder](https://coder.com/) already setup and running with coder-cli installed locally.

## Getting started

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

> Read our blog [post](coder.com/blog/deploying-coder-on-fly-io) to learn more about how to deploy Coder on fly.io.

4. Navigate to the Coder dashboard and create a new workspace using the template.

This is all. You should now have a code-server instance running on fly.io.
