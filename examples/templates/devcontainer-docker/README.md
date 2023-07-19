---
name: Devcontainers in Docker
description: Develop using devcontainers in Docker
tags: [local, docker]
icon: /icon/docker.png
---

# devcontainer-docker

Develop using [devcontainers](https://containers.dev) in Docker.

To get started, run `coder templates init`. When prompted, select this template.
Follow the on-screen instructions to proceed.

## How it works

Coder supports devcontainers with [envbuilder](https://github.com/coder/envbuilder), an open source project. Read more about this in [Coder's documentation](https://coder.com/docs/v2/latest/templates/devcontainers).

## code-server

`code-server` is installed via the `startup_script` argument in the `coder_agent`
resource block. The `coder_app` resource is defined to access `code-server` through
the dashboard UI over `localhost:13337`.

## Extending this template

See the [kreuzwerker/docker](https://registry.terraform.io/providers/kreuzwerker/docker) Terraform provider documentation to add the following features to your Coder template:

- SSH/TCP docker host
- Registry authentication
- Build args
- Volume mounts
- Custom container spec
- More

We also welcome contributions!
