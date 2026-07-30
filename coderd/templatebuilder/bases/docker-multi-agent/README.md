# Docker (Multiple Agents)

Provision two Coder workspace agents in separate Docker containers: a
primary `main` agent and a secondary `dev` agent. Modules can attach to
either agent depending on where the tooling should run.

<!-- prerequisites:start -->

## Prerequisites

Docker must be installed and running on the Coder host, and the Coder user
must have permission to use the Docker socket.

<!-- prerequisites:end -->

## Architecture

This template provisions the following resources:

- Two persistent Docker volumes (one per agent's `/home/coder`)
- Two Docker containers, each running its own Coder agent

Both containers are ephemeral and recreated on start; the home volumes
persist across restarts.
