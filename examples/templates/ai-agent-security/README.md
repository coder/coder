---
display_name: AI Agent Security (demo)
description: AI-designated workspaces with credential starvation, default-deny egress, and an optional AI sandbox
icon: ../../../site/static/icon/docker.png
maintainer_github: coder
tags: [docker, ai, security, demo]
---

# AI Agent Security demo template

Provisions a Docker workspace that exercises the AI agent security work:
AI agent identity, credential starvation, network confinement, the
template egress policy, the retained egress audit stream, and optionally a
sandboxed child agent.

This is a demonstration template. It is intentionally simple, it grants the
workspace access to the host Docker socket when the sandbox is enabled, and
it is not a hardened production example.

## What it demonstrates

| Parameter | Effect |
|---|---|
| `coder_ai_agent` | Marks the workspace AI-designated. Every agent binds to an AI identity, all four credential channels are denied, and a workspace-scoped AI token replaces the owner session token. |
| `confinement` | `proxy` routes agent traffic through a local policy proxy (advisory). `netns` runs the agent in a network namespace whose only route is that proxy (structural). `off` disables egress control. |
| `enable_sandbox` | The workspace agent creates a Docker sandbox holding a second, AI-bound agent, using the bundled scripts. |
| `sandbox_enforcement` | The attestation the sandbox script declares. The bundled script honors it: `forced` builds an internal Docker network, `advisory` only sets proxy variables. |

Designation is **sticky**. Setting `coder_ai_agent` back to false on a later
build does not restore the owner's credentials, because a parameter edit
must never be able to un-starve an AI workspace. This is worth showing.

## Requirements

- A Coder deployment built from the branch carrying this work.
- Docker on the provisioner host, as in the standard `docker` example.
- For `confinement = netns`: the `NET_ADMIN` capability (the template adds
  it) and the `ip` binary. The entrypoint installs `iproute2` if missing.
  If namespace setup fails the supervisor cleans up, falls back to
  advisory proxy mode, and reports degraded status rather than running the
  agent unconfined.
- For `enable_sandbox`: the Docker CLI inside the workspace image. The
  stock `codercom/example-base:ubuntu` image does **not** include it, so
  either bake it into a custom image or leave the sandbox disabled.

## Why the declarations are container environment, not `coder_agent.env`

`CODER_AGENT_CONFINE` and the `CODER_AI_SANDBOX_*` variables are read by
the agent **process** when it starts. Values in `coder_agent.env` are
delivered in the agent manifest and injected into processes the agent
spawns, which is too late and the wrong scope. This template therefore sets
them in the container environment, and that is the correct pattern for any
template until the Terraform resource lands.

## Layout

```text
.
├── README.md
├── main.tf
└── scripts
    ├── sandbox-create.sh   # builds the isolation boundary, starts the child agent
    └── sandbox-destroy.sh  # removes the container and network
```

The scripts are staged into the container by the entrypoint before the
agent starts, so the sandbox controller cannot race a script that has not
been written yet.

## Platform-provided script environment

The platform passes these to both scripts, appended last so a template
environment cannot override them:

| Variable | Meaning |
|---|---|
| `CODER_AI_AGENT_URL` | coderd URL for the child agent |
| `CODER_AI_AGENT_TOKEN` | the bound child agent token, minted server side |
| `CODER_AI_SESSION_TOKEN` | scoped AI session token for CLI use inside the sandbox (create only) |
| `CODER_EGRESS_PROXY` | parent-side proxy as a bare `host:port` |
| `CODER_SANDBOX_ID` | lifecycle correlation ID |

The script cannot choose the child's identity, its binding, or its
credentials. Those are resolved server side from the parent's own binding.

## Egress policy

The workspace starts default-deny: it can reach the control plane and
nothing else. Widen it without a rebuild:

```bash
coder curl -X PUT /api/v2/templates/<template-id>/ai-egress-policy \
  -d '{"rules":[{"host":"github.com","ports":[443]}]}'
```

The supervisor applies new revisions atomically to the running proxy. The
confined child never fetches or sees policy.

## Seeing the results

- Workspace page: an **AI** badge on the topbar, an **AI-bound** badge on
  each bound agent, and an **AI egress activity** section listing sessions
  with their attestation and every allowed or denied destination.
- CLI: `coder show <workspace>` annotates the workspace and bound agents.
- API: `GET /api/v2/workspaces/{workspace}/ai-sandbox-sessions` and
  `.../{session}/network-events`.
- Audit: entries record the agent as the actor and the sponsoring human as
  `on_behalf_of`.

## Known limits

- `egress_enforcement` is an attestation the platform records but cannot
  verify. A script that claims `forced` and leaks a side channel is not
  detectable at declaration time.
- There is no DNS relay inside the network namespace yet, so direct DNS
  from inside times out. Proxied traffic resolves at the proxy, and the
  agent injects proxy variables into everything it spawns.
- Scoped AI tokens last 24 hours and there is no renewal loop yet. Rebuild
  a long-lived demo workspace before using it.
- Nothing server side yet requires an AI-designated workspace to run
  confined. Confinement is opted into by this template.
