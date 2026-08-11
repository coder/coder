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
| `enable_sandbox` | The workspace agent creates a sandbox holding a second, AI-bound agent, using the bundled scripts. |
| `sandbox_backend` | `microvm` (default) drives [coder/sandbox](https://github.com/coder/sandbox), booting a libkrun microVM with its own deny-by-default recorded egress. `docker` runs a container on an internal Docker network routed through the platform proxy. |
| `sandbox_allow` | Extra egress hosts for the microVM backend (the coderd host is always allowed). Ignored by the Docker backend. |
| `sandbox_enforcement` | The attestation the sandbox script declares. Both bundled backends honor `forced`: the microVM through the coder/sandbox egress lock, Docker through an internal network with no route out. |

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
- For `enable_sandbox` with the `docker` backend: the Docker CLI inside
  the workspace image.
- For `enable_sandbox` with the `microvm` backend (default):
  - `/dev/kvm` on the Docker host (the template maps it in). No KVM means
    no microVM; use the `docker` backend instead.
  - The `coder-sandbox` binary in the workspace image or on `PATH`
    (override the location with `CODER_SANDBOX_BIN`). Build it from
    [coder/sandbox](https://github.com/coder/sandbox) with
    `CGO_ENABLED=1`.
  - The microsandbox runtime (`msb` + `libkrunfw`) preseeded under
    `~/.microsandbox`, or an egress policy that allows its download host
    for the first boot. Under the default deny-all policy an unseeded
    first boot deadlocks: it cannot download its own engine. The template
    persists `~/.microsandbox` in a volume so this cost is paid once.
- The stock `codercom/example-base:ubuntu` image carries **neither**
  backend's tooling. Set `workspace_image` to an image that does, or keep
  the sandbox disabled.

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
    ├── sandbox-create-microvm.sh   # boots a coder/sandbox microVM (default backend)
    ├── sandbox-destroy-microvm.sh  # tears the microVM down
    ├── sandbox-create.sh           # docker backend: internal network + container
    └── sandbox-destroy.sh          # removes the container and network
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

## Egress ownership differs per backend

This is the most important thing to understand about the two backends.

The **docker** backend routes the sandbox through the platform's
parent-side proxy, so its traffic is governed by the template egress
policy and lands in the platform's egress event stream (the AI egress
activity section on the workspace page).

The **microvm** backend hands enforcement to coder/sandbox itself. Its
egress lock confines the guest to the sandbox's own host-side recording
proxy, whose allowlist comes from `sandbox_allow` and whose per-request
log is `requests.log` in the sandbox directory. The guest cannot reach the
platform proxy at all, and the guest's proxy variables are reserved by
coder/sandbox for its recorder. Consequently, for a microVM sandbox:

- the platform still owns identity, binding, credential starvation, the
  session record, and the attestation;
- enforcement is stronger (a VM boundary and a structural egress lock);
- but the platform's egress event stream for the sandbox stays **empty**,
  and the per-request record lives in coder/sandbox's `requests.log`
  instead. Feeding that recorder into the platform audit stream is future
  work in the coder/sandbox hardening list.

## Known limits

- `egress_enforcement` is an attestation the platform records but cannot
  verify. A script that claims `forced` and leaks a side channel is not
  detectable at declaration time.
- coder/sandbox is a v0.1 prototype with a documented hardening list
  (unauthenticated proxy listeners, no orphaned-VM reconciliation after a
  daemon restart, first-use runtime download, unconfined bind mounts). The
  microVM backend is an honest demonstration of the script contract, not a
  hardened reference yet.
- The microVM create script passes the child tokens as process arguments,
  visible in the host process list. A production script should mount them
  read-only into the guest instead.
- There is no DNS relay inside the network namespace yet, so direct DNS
  from inside times out. Proxied traffic resolves at the proxy, and the
  agent injects proxy variables into everything it spawns.
- Scoped AI tokens last 24 hours and there is no renewal loop yet. Rebuild
  a long-lived demo workspace before using it.
- Nothing server side yet requires an AI-designated workspace to run
  confined. Confinement is opted into by this template.
