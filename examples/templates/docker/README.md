---
name: Develop in Docker
description: Run workspaces on a Docker host using registry images
tags: [local, docker]
icon: /icon/docker.png
---

# docker

## Prerequisites

1. install jq on the coder host
2. If you want to use a remote docker host, add and set it using the `docker context` command

```shell
docker context create coder --docker "host=ssh://user@host"
docker context use coder
```

> Note: Your coder host must be able to connect to the remote docker host with a valid SSH key pair.

## Getting started

To get started, run `coder templates init`. When prompted, select this template.
Follow the on-screen instructions to proceed.

## Editing the image

Edit the `Dockerfile` and run `coder templates push` to update workspaces.

## code-server

`code-server` is installed via the `startup_script` argument in the `coder_agent`
resource block. The `coder_app` resource is defined to access `code-server` through
the dashboard UI over `localhost:13337`.

## Extending this template

See the [kreuzwerker/docker](https://registry.terraform.io/providers/kreuzwerker/docker) Terraform provider documentation to
add the following features to your Coder template:

- SSH/TCP docker host
- Registry authentication
- Build args
- Volume mounts
- Custom container spec
- More

We also welcome contributions!
