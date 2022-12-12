---
name: Develop code-server in Docker
description: Run code-server in a Docker development environment
tags: [local, docker]
icon: /icon/docker.png
---

# code-server in Docker

## Getting started

Run `coder templates init` and select this template. Follow the instructions that appear.

## Supported Parameters

You can create a file containing parameters and pass the argument
`--parameter-file` to `coder templates create`.
See `params.sample.yaml` for more information.

This template has the following predefined parameters:

- `docker_host`: Path to (or address of) the Docker socket.
  > You can determine the correct value for this by running
  > `docker context ls`.
- `docker_arch`: Architecture of the host running Docker.
  This can be `amd64`, `arm64`, or `armv7`.
