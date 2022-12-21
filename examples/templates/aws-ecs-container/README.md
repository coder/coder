---
name: Develop in an ECS-hosted container
description: Get started with Linux development on AWS ECS.
tags: [cloud, aws]
icon: /icon/aws.png
---

# aws-ecs

This is a sample template for running a Coder workspace on ECS. It assumes there
is a pre-existing ECS cluster with EC2-based compute to host the workspace.

## Architecture

This workspace is built using the following AWS resources:

- Task definition - the container definition, includes the image, command, volume(s)
- ECS service - manages the task definition

## code-server

`code-server` is installed via the `startup_script` argument in the `coder_agent`
resource block. The `coder_app` resource is defined to access `code-server` through
the dashboard UI over `localhost:13337`.
