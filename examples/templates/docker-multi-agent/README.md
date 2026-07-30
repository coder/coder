---
display_name: Docker (Multiple Agents)
description: Provision two Coder agents in separate Docker containers
icon: ../../../site/static/icon/docker.png
maintainer_github: coder
verified: true
tags: [docker, container, multi-agent]
---

# Remote Development on Docker Containers with Multiple Agents

Provision two [Coder](https://coder.com/docs/user-guides/workspace-management)
workspace agents in separate Docker containers with this example template: a
primary `main` agent and a secondary `dev` agent.

This layout is useful when different tools should run in isolated
environments while remaining part of a single workspace. Modules and IDEs
can attach to whichever agent is appropriate.

## Prerequisites

Docker must be installed and running on the Coder host, and the Coder user
must have permission to use the Docker socket.

## Architecture

This template provisions the following resources:

- Two persistent Docker volumes (one per agent's `/home/coder`)
- Two Docker containers, each running its own Coder agent

Both containers are ephemeral and recreated on start; the home volumes
persist across restarts.
