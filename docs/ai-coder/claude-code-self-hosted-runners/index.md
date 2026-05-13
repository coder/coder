# Claude Code Self-Hosted Runners on Coder

> [!NOTE]
> Claude Code self-hosted runners are in early access (EAP) from Anthropic.
> Contact your Anthropic account team to request access and obtain the runner
> build. This guide describes how to run those runners on Coder workspaces
> using only existing Coder capabilities. **No Coder product changes are
> required.**

[Claude Code self-hosted runners](https://docs.claude.com/en/docs/claude-code/self-hosted-runners)
let you execute Claude Code remote sessions on infrastructure you control. A
runner is a long-lived process that registers with a pool in your Anthropic
organization, polls for assigned sessions, spawns a Claude Code child process
per session, and streams results back to Anthropic. Sessions originate from
Claude Code on the web, the mobile apps, scheduled routines, agents, or other
Anthropic surfaces. The runner is surface-agnostic.

Because the runner is just a Linux/macOS process with outbound HTTPS to
`api.anthropic.com`, **a Coder workspace is a natural host for it**. Coder
already gives you the things runners need: a reproducible base image
(template), credential delivery (parameters, env vars, secrets), per-user
isolation (one workspace per user), networking policy (workspace egress to
your internal Git and registries), and a place to attach an IDE if you want a
developer to debug what the runner is doing.

This page describes the **basic flow** for running Claude Code self-hosted
runners on Coder with no product changes. The full deployment lives in two
phases:

- [Phase 1](./plan.md#phase-1-system-identity-shippable-today): a
  self-healing pool of bot-owned runners. Ships today on Coder Premium plus
  the Anthropic EAP, with no Coder product changes. Identity is the bot;
  the human-to-runner binding is set by Anthropic's pool scheduler. This is
  what the [setup guide](./setup.md) walks you through.
- [Phase 2](./plan.md#phase-2-user-identity-requires-middleware): a small
  webhook receiver maps each `runner-needed` event from Anthropic to the
  matching Coder user and claims a warm prebuild on their behalf. Identity
  becomes the human, audit log attributes to them, and external auth
  resolves to their tokens. Depends on Anthropic contracts that are not yet
  finalized.

## What you get

- Claude Code remote sessions execute inside your Coder workspaces, on the
  same network and image your developers already use.
- A pool of warm runners that the Anthropic scheduler can lock to any user
  at session-arrival time, mirroring Anthropic's "one runner is locked to
  one user at a time" model without you having to provision per-user
  workspaces.
- The runner can reach internal Git, package registries, databases, and
  build tooling that the workspace can reach, with no extra network
  plumbing.
- Existing Coder primitives (templates, parameters, prebuilds, RBAC, audit
  log, TTL) govern the runner the same way they govern any other
  workload.

## What this is *not*

- It is not a managed Anthropic integration. Coder does not provision pools,
  rotate pool secrets, or route sessions; Anthropic does.
- It does not require any new Coder feature. The runner is a regular process
  inside a workspace.
- It is not the same as [Coder Agents](../agents/index.md) or
  [AI Gateway](../ai-gateway/index.md). Coder Agents is Coder's own
  control-plane agent. AI Gateway is Coder's egress proxy for LLM traffic.
  Self-hosted runners are Anthropic's product running on your compute. The
  three are complementary and can be used together. See
  [How it relates to Coder Agents and AI Gateway](#how-it-relates-to-coder-agents-and-ai-gateway).

## How it fits together

<img src="../../images/guides/claude-code-self-hosted-runners/architecture.svg" alt="Anthropic surfaces and control plane on the left send session assignments over outbound HTTPS to a Claude Code self-hosted runner that lives inside a Coder workspace. The runner spawns one child Claude Code process per session. Child sessions reach the Claude model API and your internal Git, package registries, and services." />

- A developer starts a Claude Code session at `claude.ai/code` (or from
  mobile, a routine, etc.) and picks the pool that points at your Coder
  workspaces.
- The session is queued on Anthropic's side. A free Coder-hosted runner
  picks it up, clones the requested repo, spawns a child `claude` process,
  and streams events back to Anthropic.
- The developer sees the session in the Anthropic UI exactly as they would
  for an Anthropic-managed session. The fact that the compute is in Coder
  is transparent to them.

## High-level Phase 1 flow

This is the day-one flow the [setup guide](./setup.md) walks you through.
It maps cleanly to roles your team already has.

### Roles

| Role                 | Responsibility                                                                                                                                                                                                                                                                              |
|----------------------|---------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| Anthropic org admin  | Creates and rotates self-hosted runner pools in `claude.ai`. Distributes the pool secret to the platform team.                                                                                                                                                                              |
| Coder template admin | Publishes a Coder template that bakes the runner binary and git identity into a workspace image, adds a `coder_workspace_preset` with `prebuilds { instances = N }`, and supplies three sensitive variables: the pool secret, a git bot credential, and a Coder service-account API token. |
| Anthropic developer  | Starts sessions from `claude.ai/code`, mobile, or routines and picks the pool that targets Coder. They never need to log in to Coder.                                                                                                                                                       |

In small teams the first two roles are the same person.

### End-to-end flow

1. **Anthropic org admin** creates a pool at `claude.ai > Settings > Claude
   Code > Self-hosted runner pools` and copies the pool secret. The secret
   is shown once.
2. **Coder template admin** publishes a Coder template that bakes the
   runner binary into a workspace image, starts the runner via
   `coder_script`, surfaces runner state via `coder_agent.metadata`, and
   `coder delete`s the workspace from inside when the runner exits. The
   template's preset declares `prebuilds { instances = N }`, so Coder
   maintains N warm workspaces. Push credentials come from a bot PAT or
   SSH deploy key shipped as a sensitive template variable.
3. **Coder** continuously maintains N warm workspaces. Each one runs the
   runner, which registers with Anthropic and starts polling. Workspace
   owner is the prebuilds service account; the workspace is anonymous
   from a human's perspective.
4. **Anthropic developer** starts a session at `claude.ai/code` (or from
   mobile, a routine, etc.) and picks the pool that targets Coder.
5. Anthropic's pool scheduler routes the session to one of the warm
   runners. That runner locks to the developer's Anthropic account. The
   Coder workspace's metadata now shows the locked Anthropic user and the
   in-flight session count.
6. The runner serves up to `--capacity` parallel sessions for that one
   user. When the session queue drains to zero, the runner exits 0, the
   workspace deletes itself, and Coder's prebuild reconciler queues a
   replacement.

## Why Phase 1 works with prebuilds

Anthropic's runner model has two properties that map cleanly onto Coder
prebuilds:

1. **One runner serves one user at a time, and it does not know who that
   user is until the first session arrives.** The lock is set at
   session-arrival time, not at runner-spawn time. That means you do not
   have to provision per-user workspaces in advance; you just need a pool
   of warm runners ready for the Anthropic scheduler to claim. Coder's
   prebuilds primitive is exactly that pool.

2. **The runner expects a fresh filesystem on restart.** Anthropic
   recommends Kubernetes, ECS, or systemd as the orchestrator that
   restarts the process on exit. In Coder, the equivalent is: the runner
   exits, the workspace `coder delete`s itself from the inside, and the
   prebuild reconciler queues a replacement workspace with a fresh
   container filesystem.

The identity trade-off Phase 1 ships with: **every commit is the bot**,
not the human. The Anthropic session URL appended as a commit trailer is
your per-human audit signal in the git history. [Phase 2](./plan.md#phase-2-user-identity-requires-middleware)
restores per-user identity via middleware that claims prebuilds on behalf
of the matching Coder user.

See the [setup guide](./setup.md) for the copyable Terraform that builds
this.

## How it relates to Coder Agents and AI Gateway

| Coder feature                                | What it does                                                              | Relationship to self-hosted runners                                                                                                                                                                                 |
|----------------------------------------------|---------------------------------------------------------------------------|---------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| [Coder Agents](../agents/index.md)           | Coder's own agent that runs in the control plane and talks to workspaces. | Independent. You can use both, or pick whichever your team prefers per use case.                                                                                                                                    |
| [AI Gateway](../ai-gateway/index.md)         | Egress proxy for LLM traffic with audit and policy.                       | Optional. You can point the child `claude` process at AI Gateway via `ANTHROPIC_BASE_URL`; the runner itself still calls `api.anthropic.com` for pool registration. Detailed in [Plan: advanced topics](./plan.md). |
| [Agent Firewall](../agent-firewall/index.md) | Process-level egress and command policy inside a workspace.               | Optional. Apply it to the workspace if you want extra guardrails on what the child `claude` process can reach or run.                                                                                               |

## Where to next

- [Setup](./setup.md): full Terraform template, image checklist, Git
  identity config, parameter and env wiring.
- [Plan](./plan.md): the staged roadmap from the basic flow above to more
  advanced variants (per-creator credentials, AI Gateway, fleet pools,
  autoscaled runner workspaces). These are documentation and template
  exercises, not Coder product work.
