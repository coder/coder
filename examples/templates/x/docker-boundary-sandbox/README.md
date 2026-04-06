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

The `docker-chat-sandbox` example uses bubblewrap for **filesystem isolation**.
It keeps the chat agent on a read-only root filesystem with a writable home
mount. This template uses Agent Boundaries with `nsjail` for **namespace and
network isolation**. The hidden chat agent gets its own network stack, its own
PID namespace, and an HTTP and HTTPS allowlist enforced by the Boundary proxy.
These controls are complementary. Bubblewrap limits what the agent can read or
write, and Boundary limits what the agent can see on the network and which web
services it can reach.

## Network policy

| Traffic                               | Policy                                      |
|---------------------------------------|---------------------------------------------|
| Coder server (`host.docker.internal`) | Allowed                                     |
| Anthropic API                         | Allowed                                     |
| All other HTTP/HTTPS                  | Denied by the Boundary allowlist and logged |
| DNS, UDP, and raw TCP                 | Blocked or intercepted inside the namespace |
| Visible dev agent                     | Unrestricted                                |

## How the sandbox works

`boundary-agent.sh` copies `boundary-config.yaml` into
`~/.config/coder_boundary/config.yaml` and launches
`coder boundary -- sh /tmp/coder-init.sh`.

At startup, `nsjail` creates a dedicated network namespace for the hidden chat
agent. The sandboxed process gets its own network stack, so outbound traffic is
contained inside the namespace instead of sharing the visible agent's network
view. That boundary applies to more than proxied web requests. DNS, raw TCP,
and UDP traffic are also constrained inside the namespace, which prevents the
agent from bypassing the policy with direct connections.

`nsjail` also creates a dedicated PID namespace. Inside the sandbox, the chat
agent only sees its own process tree. It cannot enumerate, signal, or attach to
processes that belong to the visible `dev` agent or the Docker host.

Inside that isolated namespace, the Boundary proxy enforces the HTTP and HTTPS
allowlist. Requests to listed domains are forwarded, and requests to unlisted
web destinations are denied and logged. Boundary writes local logs to
`/tmp/boundary_logs/` and streams every allow and deny decision to the Coder
control plane for auditing.

## Namespace isolation

| Isolation layer      | What it provides                                                        | Provided by       |
|----------------------|-------------------------------------------------------------------------|-------------------|
| Network namespace    | Own network stack; all traffic routed through boundary proxy            | nsjail            |
| PID namespace        | Own process tree; cannot see or signal host processes                   | nsjail            |
| HTTP/HTTPS allowlist | Domain-level deny-by-default policy for outbound web traffic            | Boundary proxy    |
| DNS interception     | Prevents DNS-based data exfiltration; queries resolved inside namespace | nsjail + Boundary |
| UDP/raw TCP control  | Non-HTTP traffic blocked by namespace network rules                     | nsjail (iptables) |
| Audit logging        | Every allow/deny decision logged and streamed to control plane          | Boundary          |

## nsjail vs landjail

This template uses `nsjail`, which is the default jail type in
`boundary-config.yaml`.

| Feature                    | nsjail (this template)                 | landjail                          |
|----------------------------|----------------------------------------|-----------------------------------|
| Bypass resistance          | Strong: transparent interception       | Medium: bypassable via proxy port |
| PID isolation              | Yes: own process tree                  | No: can see host processes        |
| Network isolation          | Full namespace with iptables           | HTTP proxy only                   |
| UDP/DNS control            | Yes                                    | No (UDP can leak)                 |
| Linux kernel requirement   | 3.8+ (widely available)                | 6.7+ (very new)                   |
| Docker capabilities needed | `CAP_NET_ADMIN` + `seccomp=unconfined` | None                              |

To switch to `landjail`, change `jail_type: landjail` in
`boundary-config.yaml` and remove the extra Docker capabilities from `main.tf`.

## Docker runtime requirements

| Requirement          | Why                                                                         |
|----------------------|-----------------------------------------------------------------------------|
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
5. From the hidden chat agent, run `ps aux` and confirm it only shows the
   agent's own processes, not processes from the visible `dev` agent or host.
6. From the hidden chat agent, try `ping 8.8.8.8` or `nc` to a non-HTTP port and
   confirm it fails inside the namespace.
7. Inspect `/tmp/boundary_logs/` for allow and deny entries.
