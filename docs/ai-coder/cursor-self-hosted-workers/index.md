# Cursor Self-Hosted Workers on Coder

> [!NOTE]
> Cursor private workers are an early-access feature from Cursor. Contact
> your Cursor account team to enable them for your team and obtain a
> service-account API key. This guide describes how to run those workers
> on Coder workspaces.

Cursor private workers are a long-lived process that registers with a
pool in your Cursor team, binds to one git repository, and serves
Cursor Background Agent sessions on infrastructure you operate. Each
worker needs an OS, an image, an outbound network path to
`api.cursor.com`, and a lifecycle.

That is what [Coder workspaces](../../user-guides/workspace-management.md)
are. The same
[Terraform templates](../../admin/templates/index.md) your platform team
uses to ship developer environments can ship workers.

## Architecture

<img src="../../images/guides/cursor-self-hosted-workers/concepts.svg" alt="A Coder workspace is the outer container, managed by Coder. Inside the workspace runs the Cursor private worker, a long-lived process managed by Cursor. The worker is bound to one git repository and serves one Cursor session at a time, also managed by Cursor. Each layer is nested inside the layer above it." />

Three nested concepts, three levels of management:

- **A Coder workspace** is the outer container: a VM or container built
  from a Coder template, with the image, credentials, and networking
  the worker needs. **Coder manages workspaces.**
- **A worker** is the long-lived `cursor-agent worker` process that
  lives inside one workspace, registers with your Cursor team, and
  polls for work. One worker per workspace. The worker is bound to one
  repository at startup and serves one session at a time. **Cursor
  manages workers.**
- **A session** is a single Cursor Background Agent conversation,
  routed to a free worker by Cursor's scheduler. **Cursor manages
  sessions.**

All traffic from the worker to Cursor is **outbound HTTPS** to
`api.cursor.com`. There is no inbound connectivity from Cursor into
your network; the worker dials out and streams events over the same
long-poll connection that delivers session assignments. From those
sessions, the worker reaches whatever your workspace can already
reach: internal Git, package registries, databases, build tooling.

### How a session flows

1. A developer starts a Cursor Background Agent session and picks the
   private-worker pool that targets Coder, against a specific
   repository.
2. The session queues on Cursor's side. Cursor's scheduler picks a
   free worker that is bound to that repository.
3. The worker pulls the requested branch into its checkout, runs the
   session, and streams events back to Cursor.
4. The developer sees the session in the Cursor UI exactly as they
   would for a Cursor-managed session. The fact that the compute is in
   Coder is transparent to them.
5. When the session ends and the worker stays idle for
   `--idle-release-timeout` (8h default), the worker stops accepting
   new connections. The reconciler then deletes the workspace and
   queues a replacement.

### What Coder primitives map to

**Cursor worker = Coder workspace.** Each worker is one workspace
bound to one repository. A pool of N workers for a repo is N
workspaces of the same shape; a second repo is a second pool.

| Cursor concept                       | Coder primitive                                                                     |
|--------------------------------------|-------------------------------------------------------------------------------------|
| One worker                           | **One workspace**                                                                   |
| Pool of N workers for one repo       | **N workspaces** of one template (or one preset, on Premium)                        |
| Multiple pools across repos          | **One template per repo** (or one preset per repo, on Premium)                      |
| Worker image                         | Workspace template + image                                                          |
| Worker process                       | `cursor-agent worker start` under `coder_agent.startup_script`                      |
| Service-account API key              | Sensitive Terraform variable                                                        |
| "Reconciler refills the pool"        | Coder prebuilds, or an external `cursor-worker-pool-daemon` for OSS                 |
| Per-session checkout                 | `$HOME/workspace`, populated by `git clone` in `startup_script`                     |
| Internal Git, registries, services   | Whatever the workspace can already reach                                            |
| Wrapper scripts and lifecycle hooks  | Files in the workspace image, invoked before `cursor-agent`                         |
| `/healthz`, `/readyz` (cursor-agent) | `coder_agent.metadata` blocks that curl `:8080`                                     |

## Why run them on Coder

Self-host the worker on a Coder workspace and the workspace primitives
you already use carry over:

- **Reproducible workspaces as code.** Coder defines a workspace in
  Terraform: pick a container, VM, or bare-metal host on AWS, GCP,
  Azure, vSphere, Kubernetes, Nomad, or your own provider; bake the
  `cursor-agent` binary, language toolchains, and internal CLIs into
  the image; ship the whole thing with one `coder templates push`.
  Every worker in the fleet is the same code path.
- **Network you already control.** The workspace runs wherever your
  template places it: your VPC, your private subnet, your peered
  on-prem segment. Outbound to `api.cursor.com` is the only thing
  Cursor needs; reaching internal Git, registries, databases, and
  build systems uses the routes that workspace already has.
- **Compute you already capacity-plan.** Same accounts, same nodes,
  same autoscaler your developer workspaces run on. Coder prebuilds
  keep a warm pool of workers ready, recycle them on a TTL, and let
  you size the pool the way you size any other internal service.
- **One environment, two use cases.** The same base image, the same
  internal registries, and the same git access you give a developer
  for interactive work also serves the worker pool. Ship a developer
  template and a worker template that share an image, then diverge
  only where they should: the worker template can tighten egress to
  `api.cursor.com` plus your internal hosts, drop interactive ports,
  and apply [Agent Firewall](../agent-firewall/index.md) rules, while
  the developer template stays open. Same environment for software
  engineering and agents, different network controls and policies per
  workspace.
- **Compliance.** Source code, build artifacts, and the worker's
  working directories stay on infrastructure you own. Workspaces,
  agent logs, and Coder audit trails live in your tenancy with the
  rest of your SDLC.
- **Day-2 ops.** Push the template once to ship a new worker image
  fleet-wide. Use the workspace page, agent logs, and metadata
  surfaces to see in-use state and active session id. Attach an IDE
  to a workspace when you need to debug what a worker is doing.

## What this is and is not

- This is **not a managed Cursor integration**. Coder does not
  provision pools, rotate service-account keys, or route sessions;
  Cursor does.
- This is **not the same as [Coder Agents](../agents/index.md) or
  [AI Gateway](../ai-gateway/index.md)**. Coder Agents is Coder's own
  control-plane agent. AI Gateway is Coder's egress proxy for LLM
  traffic. Self-hosted Cursor workers are Cursor's product running on
  your compute. The three are complementary and can be used together.
- This is **early access, not GA**. The `cursor-agent worker` flag
  set, the fleet API shape, and the sub-token contract are all
  subject to change during early access. Pin a known-good
  `cursor-agent` version in your image and re-test on bumps.

## How it relates to Coder Agents and AI Gateway

| Coder feature                                | What it does                                                              | Relationship to self-hosted workers                                                                                                                                                |
|----------------------------------------------|---------------------------------------------------------------------------|------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| [Coder Agents](../agents/index.md)           | Coder's own agent that runs in the control plane and talks to workspaces. | Independent. You can use both, or pick whichever fits per use case.                                                                                                                |
| [AI Gateway](../ai-gateway/index.md)         | Egress proxy for LLM traffic with audit and policy.                       | Optional. The worker itself talks to `api.cursor.com` for pool registration; child model traffic from sessions can be routed through AI Gateway. Detailed in the implementation notes. |
| [Agent Firewall](../agent-firewall/index.md) | Process-level egress and command policy inside a workspace.               | Optional. Apply it to the worker workspace for extra guardrails on what sessions can reach or run.                                                                                 |

## Identity models

Coder supports running Cursor workers under two different identity
models. They share the same template, image, and pool; they differ in
who owns the workspace and whose credentials the worker uses.

### System identity (Works Today)

Coder (or an external daemon) maintains N warm bot-owned workspaces
per repo. Cursor's scheduler picks one when a session arrives and
routes it there. The workspace, the git credential, and the worker's
Cursor identity are all the same service account, fleet-wide.

Because there is no per-user identity to attribute commits to, **git
pushes are deliberately blocked** at startup
(`remote.origin.pushurl = no_push`). Workers can read and search the
repo and produce diffs in the session UI; they cannot create branches
or open pull requests. The per-human signal lives in Cursor's session
log, keyed by the worker's `activeBcId`.

See [System identity](./system-identity.md) for the copyable Terraform
recipe and the known limitations.

### User identity (Planned)

A routing component pre-binds each worker workspace to the developer
who started the session. The workspace owner is the human, Coder
external auth wires their git push token automatically, and audit log
entries attribute to them.

User identity depends on Cursor API pieces that are still being
finalized: a team service-account key with `agent:*` scope, the
`POST /v1/sub-tokens` per-user token mint, and a stable shape for
`GET /v0/private-workers/pending-requests`. The
[System identity](./system-identity.md) recipe you ship today is the
foundation; turning on user identity means swapping the single
service-account key for per-request sub-tokens and pointing workspace
creation at the matching Coder user.

See [User identity](./user-identity.md) for the design and what stays
the same.

## Where to next

- [System identity](./system-identity.md): the recipe for a
  self-healing pool of bot workers, with a primary path on Coder
  prebuilds and an alternative path on an external daemon for OSS
  Coder.
- [User identity](./user-identity.md): per-developer attribution. On
  the Coder + Cursor roadmap; not yet available.
- [Implementation notes](./plan.md): the staged plan, the sub-stages
  within system identity (per-creator credentials, AI Gateway routing,
  custom checkout, tool allowlists), and the open questions tracked
  alongside this delivery.
