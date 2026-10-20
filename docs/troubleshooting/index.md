---
title: Troubleshooting
---

Use these guides to find and fix common problems with workspaces, templates, networking, and integrations.
Each guide is organized by the symptom or error message you see.

## Start with a quick check

- Run `coder ping <workspace>` to check the connection between your device and a workspace.
- Open `/health` on your Coder deployment to check its health, as described in [Deployment health](../admin/monitoring/health-check.md).

## Find your problem

| Symptom                                                                                      | Guide                                                              |
|----------------------------------------------------------------------------------------------|--------------------------------------------------------------------|
| A workspace agent does not connect, or a startup script fails or times out                   | [Templates](./templates.md)                                        |
| A workspace connection is slow, relayed through DERP, or does not connect directly           | [Networking](./networking.md)                                      |
| A dev container does not start, or you cannot connect to it with SSH or VS Code              | [Dev containers](./dev-containers.md)                              |
| `coder exp sync` commands fail, or startup scripts wait on each other indefinitely           | [Startup coordination](./startup-coordination.md)                  |
| An OAuth2 client receives an error such as `invalid_scope` or `invalid_grant` from Coder     | [OAuth2 provider](./oauth2-provider.md)                            |
| Coder cannot connect to the Docker daemon, or a Docker workspace is stuck in "Connecting..." | [Docker installation](../install/server/docker.md#troubleshooting) |

## Get help

If these guides do not solve your problem:

1. Generate a [support bundle](../support/support-bundle.md) with `coder support bundle <workspace>`.
1. Search or [open a GitHub issue](https://github.com/coder/coder/issues/new), or ask in [Discord](https://discord.gg/coder).
1. [Coder Premium](https://coder.com/pricing#compare-plans) includes dedicated support.

<children></children>
