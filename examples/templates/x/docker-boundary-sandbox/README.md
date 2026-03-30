---
display_name: Docker + Boundary Sandbox
description: Two-agent Docker template with an Agent Boundaries network-sandboxed chat agent
icon: ../../../../site/static/icon/docker.png
maintainer_github: coder
tags: [docker, container, chat, boundary]
---

# Docker + Boundary Sandbox

> [!WARNING]
> This template is experimental.
> It depends on the `-coderd-chat` agent naming convention, which is an
> internal proof-of-concept integration detail.
> Do not rely on this template shape for production deployments.
> Agent Boundaries is also a Coder Enterprise feature, so the hidden chat
> agent will not be network-sandboxed on unlicensed deployments.

This example provisions two Docker-backed agents that share the same
`/home/coder` volume but run in separate containers. The visible `dev`
agent behaves like a normal development workspace, while the hidden
`dev-coderd-chat` agent runs behind Agent Boundaries.

## How it works

The workspace has two agents:

- **`dev`**: Standard development agent with code-server. It is visible in the
  dashboard and has unrestricted networking.
- **`dev-coderd-chat`**: Hidden chat-only agent. The `-coderd-chat` suffix tells
  chat routing to target this agent and keeps it out of the normal UI. Its
  process tree is wrapped by `coder boundary`.

Both agents mount the same persistent `/home/coder` volume, so they can share
source code and workspace state while still using different runtime policies.

## Agent Boundaries vs bubblewrap

The `docker-chat-sandbox` example uses bubblewrap for **filesystem isolation**,
which keeps the chat agent on a read-only root filesystem with a writable home
mount. This template uses Agent Boundaries for **network policy isolation**,
which allowlists outbound HTTP and HTTPS destinations and denies everything
else by default. These controls are complementary: Boundary limits what the
agent can talk to, and bubblewrap limits what it can read or write.

## Network policy

| Traffic                               | Policy                                 |
| ------------------------------------- | -------------------------------------- |
| Coder server (`host.docker.internal`) | Allowed                                |
| Anthropic API                         | Allowed                                |
| All other HTTP/HTTPS                  | Denied and logged                      |
| Non-HTTP traffic (DNS, raw TCP)       | Controlled by nsjail network namespace |
| Visible dev agent                     | Unrestricted                           |

## How the sandbox works

1. `boundary-agent.sh` copies `boundary-config.yaml` into
   `~/.config/coder_boundary/config.yaml` and launches
   `coder boundary -- sh /tmp/coder-init.sh`.
2. `nsjail` creates a network namespace for the hidden chat agent.
3. Boundary routes outbound HTTP and HTTPS traffic through a local proxy.
4. The proxy checks each request against the allowlist. Matching destinations
   pass through, and everything else is denied.
5. Boundary records local logs in `/tmp/boundary_logs/` and streams audit
   decisions to the Coder control plane.

## Docker runtime requirements

| Requirement          | Why                                                                         |
| -------------------- | --------------------------------------------------------------------------- |
| `CAP_NET_ADMIN`      | `nsjail` needs it to create network namespaces                              |
| `seccomp=unconfined` | Docker's default seccomp profile blocks the `clone` patterns `nsjail` needs |

The visible `dev` container runs without extra capabilities.

## Customizing the allowlist

Edit `boundary-config.yaml` to add or remove permitted domains for the hidden
chat agent. Keep comments next to each rule so future readers know why a domain
is allowed. For more advanced match rules, including method and path filters,
see the Agent Boundaries rules engine documentation.

## Limitations

- Requires a Coder Enterprise license.
- The `coder boundary` CLI version should match your Coder deployment.
- `nsjail` requires Docker with the default `runc` runtime.
- This example does not provide filesystem isolation. Use the bubblewrap
  example for that, or combine both approaches.

## Usage

```bash
cd examples/templates/x/docker-boundary-sandbox
coder templates push docker-boundary-sandbox \
  --var docker_socket="$(docker context inspect --format '{{ .Endpoints.docker.Host }}')"
```

## Smoke test

1. Create a workspace from this template.
2. Confirm only the `dev` agent appears in the UI.
3. From the hidden chat agent, verify an allowlisted domain is reachable, for
   example `curl https://api.anthropic.com`.
4. From the hidden chat agent, verify a non-allowlisted domain is blocked, for
   example `curl https://example.com` should fail.
5. Inspect `/tmp/boundary_logs/` for allow and deny entries.
