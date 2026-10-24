---
display_name: GitHub Actions Runner
description: Run one GitHub Actions job in a disposable Docker workspace
icon: ../../../site/static/icon/github.svg
maintainer_github: coder
verified: false
tags: [docker, ci]
---

# GitHub Actions runner

Use this template with a Coder GitHub runner pool. The pool polls GitHub's
scale-set service, creates a workspace with a runner-specific JIT secret, and
deletes the workspace after its job completes. Creating this workspace manually
does not register a runner.

The Coder provisioner needs access to a Docker daemon. The container needs
outbound access to Coder and GitHub. For local development the template rewrites
localhost in the agent initialization script to the Docker host gateway.

Each allocation runs the official GitHub runner image as its non-root user.
The image version must be maintained as GitHub releases updates. Workspaces
share neither persistent volumes nor the host Docker socket. This example
supports Linux shell workflows; Docker actions and service containers require
an isolated Docker implementation and are not configured here. For untrusted
workflow code, use a template that provisions a separate VM per workspace.

Choose a dedicated workspace owner with no personal secrets, external-auth
tokens, or infrastructure privileges. The runner receives a JIT configuration
through an ephemeral workspace secret; the pool's GitHub PAT stays in coderd.

Create a pool through
`POST /api/v2/organizations/{organization}/github-runner-pools` with the
template ID, owner ID, pool name, GitHub repository or organization URL, a token
with runner administration permission, and `max_runners`. Configure a workflow
with `runs-on: <pool-name>`. Inspect pool status and allocations through that
API. Deleting a pool drains running jobs before removing its scale set.

Startup and job deadlines bound failed allocations. The job deadline forcibly
deletes a workspace even if its job is still running; set it above the expected
workflow duration. Configure the template's scheduling policies so developer
workspace autostop does not interrupt CI jobs.
