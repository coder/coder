# Implementation notes: Cursor self-hosted workers on Coder

This page captures the staged plan and the open questions behind the
two customer-facing identity models. It is the place to look if you
are evaluating the [System identity](./system-identity.md) recipe and
want to understand the trade-offs we accepted, or if you are tracking
what blocks [User identity](./user-identity.md) from shipping.

The constraint for the **shippable** parts of this plan is that they
use only Coder primitives that exist today. Anything that would
require a Coder product change is called out explicitly in the
[Open questions for Coder](#open-questions-for-coder) section.
Anything that would require a Cursor change is in the
[Open questions for Cursor](#open-questions-for-cursor) section.

## Goals

- Give platform teams a clear path from "I have a Cursor team and a
  service-account key" to "developers can route Cursor Background
  Agent sessions to Coder workspaces."
- Be explicit about which pieces ship today on Coder primitives and
  which depend on contracts Cursor or Coder has not finalized.
- Make it obvious which Cursor features translate to which Coder
  primitives so we do not accidentally pull product work into a docs
  project.
- Be honest about the rough edges so future product work has a clear
  scope.

## Non-goals

- Building any new Coder UI, API, or module *as part of the system
  identity recipe*. User identity and the
  [Coder open questions](#open-questions-for-coder) describe product
  work that is in scope for the follow-on, but not for the initial
  docs delivery.
- Wrapping the `cursor-agent` binary in a Coder-distributed package.
- Cursor-side changes (pool management, fleet API shape, scheduling).
  These are tracked as
  [open questions for Cursor](#open-questions-for-cursor).

If we want any of those, that is separate product work that should
be proposed in a Linear issue, not in this docs branch.

## System identity (shippable today)

A pool of bot-owned Coder workspaces per repository, each running one
Cursor private worker, behind either a Coder prebuilds preset or an
external reconciler daemon. Cursor's scheduler chooses which workspace
serves which session; Coder's role is to keep N workspaces warm per
repo and to refill the pool when one drains.

**Identity model:** every worker authenticates with the same team
service-account API key. Git pushes are blocked at startup
(`remote.origin.pushurl = no_push`) because there is no per-user
identity to attribute commits to. The per-human signal is the
`activeBcId` field in Cursor's fleet API, which Cursor's session log
correlates back to the user.

### Two reconciler paths

The recipe documents both, with prebuilds as the primary and the
daemon as the alternative:

1. **Coder prebuilds (primary).** `coder_workspace_preset` per repo
   with `prebuilds { instances = N }`. Requires Coder Premium.
   Reconciliation is handled in `coderd`. The upgrade path to user
   identity is cleaner because prebuilds is the natural inventory
   primitive for the per-user claim.
2. **`cursor-worker-pool-daemon` (alternative).** A small external
   binary that polls the Cursor fleet API and Coder's workspace API,
   deletes stale or outdated workers, and creates replacements. Runs
   on any Coder deployment, OSS included. Source is in
   `cursor-runners/daemon/` (about 500 lines of Go).

Both produce the same workspace shape, the same `cursor-agent worker
start` invocation, the same metadata blocks. The difference is only
who watches the pool.

### Pieces

1. **Workspace template** that bakes the `cursor-agent` binary and
   starts it under `coder_agent.startup_script`. The template
   defines a `git_repo_url` parameter pinned per preset (or per
   template, for the daemon path).
2. **Sensitive template variable** for the Cursor team service-account
   API key. One key fleet-wide.
3. **`coder_agent` metadata** that surfaces the worker process state,
   `/readyz` value, and any other Cursor-side signals we can scrape
   from the local management server on `:8080`.
4. **Push block** in `startup_script`:
   `git remote set-url --push origin no_push`. Workers can read and
   search the repo; they cannot push.
5. **Prebuild preset (or daemon `--template` flag) per repo**, since
   one Cursor worker is bound to one repo. The recipe shows a single
   preset and a comment block for adding more.

### What this gives you

- A self-healing pool of N warm workers per repo that recover from
  drain and from EAP-grade rough edges.
- Surface-agnostic: any Cursor surface (web, IDE, mobile) lands on the
  pool the same way.
- The whole pool reproduces from a template push and one sensitive
  variable.

### What this does not give you

- **Per-user git identity.** All commits would be the bot, so we
  block commits entirely.
- **Per-user external auth.** Coder's `coder_external_auth` reads the
  workspace owner's linked token. The owner is the prebuilds service
  account, which has no OAuth flow. So external-auth does not resolve
  anything useful here.
- **Per-user audit attribution in Coder.** Coder's audit log says
  "prebuild service account spawned the workspace." The richer
  "alice ran a session against repo X" trail lives in Cursor's
  session log, joinable on `activeBcId`.
- **Cross-repo concurrency from one pool.** A worker is bound to one
  repo; you need one pool (preset or template) per repo.

### System identity acceptance criteria

- A reader who has never seen Cursor private workers can stand up a
  working Coder template using only the
  [System identity recipe](./system-identity.md) and a Cursor team
  service-account API key.
- The reader uses only shipped Coder primitives. Where system identity
  hits a limitation in those primitives (the prebuilds service
  account cannot complete an OAuth flow, so external auth does not
  resolve inside a prebuilt workspace), the recipe documents the
  consequence (no pushes from workers) and the gap is filed in the
  [open questions for Coder](#open-questions-for-coder).
- The pool maintains N warm workers per repo through drain, error
  exit, and TTL expiry.

## User identity (on the roadmap)

A small router (middleware or in-tree integration) consumes Cursor's
pending-request signal, mints a per-user sub-token, maps the Cursor
user to a Coder user, and claims a warm prebuild on their behalf.

This is what unlocks per-user identity: when the workspace is claimed
on behalf of the human, Coder's external-auth wires git push with the
human's token, audit log entries attribute to the human, and the
worker registers in the Cursor fleet under the human's identity.

### Pieces

1. **Router service, or in-tree integration.** Two implementation
   paths, both valid:
   - **Middleware service.** A few hundred lines of Go or Node.
     Polls (or receives a webhook for) Cursor's
     `pending-requests`, mints sub-tokens, pre-flights that the Coder
     user exists and has external-auth grants, calls Coder's
     workspace-claim API on the user's behalf. Faster to ship; can
     land the day the Cursor `agent:*` key issues.
   - **First-class integration in `coderd`.** Coder's server consumes
     Cursor's scaling signal directly, with user-mapping rules and
     the on-behalf-of claim built in. The router collapses from "a
     service to write" to "a config block in the template." Better
     long-term shape; absorbs whatever middleware deployments teach
     us about the right defaults.
2. **Coder service-account token.** A scoped API token owned by, for
   example, `svc-cursor-router`. Scopes: claim workspaces on behalf
   of users, read users, read and delete workspaces. Vaulted,
   rotated. Never a human admin's PAT.
3. **The system identity prebuilds preset.** User identity reuses the
   system identity pool as inventory. The router claims a warm
   prebuild on behalf of the user, which atomically transfers
   ownership from the prebuilds service account to the human. The
   claim build runs with the human's owner context, so external-auth
   resolves to their token, and the in-workspace `cursor-agent
   worker start` invocation uses the per-user sub-token instead of
   the shared team key.

### User identity acceptance criteria

- The router can convert one Cursor pending request into one Coder
  workspace owned by the matching human.
- The worker registers in Cursor's fleet under the human's identity
  (no shared service-account presentation).
- `git push` from the worker uses the human's external-auth token.
- Coder's audit log attributes the workspace to the human, with the
  service account shown as the on-behalf-of creator.

## Sub-stages within system identity (docs follow-ons)

These layer on top of System identity and are pure documentation and
template work. They are stage-numbered for sequencing, not because
they require User identity to ship.

### Stage A: Per-creator credentials via wrapper script

Cursor's worker can be configured to invoke a wrapper script before
session start. The wrapper receives session context (id, user, repo)
on stdin or via env vars and is the right place to exchange a session
context for short-lived credentials from your IdP (AWS STS, Vault,
GCP) before invoking the real session payload.

This stage will add a docs page that:

- Documents the wrapper invocation contract.
- Provides a wrapper-script template that decodes the session
  context, exchanges it for short-lived credentials, and execs the
  session payload.
- Calls out the warning that the wrapper contract is subject to
  change during the EAP, and recommends pinning a `cursor-agent`
  version before relying on specific fields.

This is a no-op from Coder's perspective; the wrapper just runs
inside the workspace.

### Stage B: Custom checkout via lifecycle hooks

The worker should support hooks to override the default
`git clone / git fetch / git reset --hard` behavior. The most useful
for Coder deployments is a `checkout` hook that lets us:

- Clone from an internal Git replica
  (`ghes-replica.internal`) instead of the verbatim source URL.
- Use a local bare mirror with `--reference-if-able` to skip
  re-fetching shared objects, a big win when many sessions land on
  the same workspace.
- Materialize non-git sources (Perforce, S3 tarball) by replacing the
  default clone entirely.

Docs page will include:

- A reference `checkout` hook with a `/var/cache/git-mirrors` location
  appropriate for a Coder workspace (likely on a persistent volume so
  it survives workspace restart).
- Guidance on which workspace volumes should persist vs. which should
  be ephemeral.

### Stage C: Route the child model traffic through AI Gateway

The worker process itself must talk to `api.cursor.com` for pool
registration and polling. The model calls the session makes can be
routed through Coder's [AI Gateway](../ai-gateway/index.md) by
configuring the appropriate base URL and auth headers in the wrapper
script.

This is interesting because it gives platform admins audit and policy
coverage over the *model traffic* the worker generates, without
changing how Cursor dispatches the session.

Docs page will cover:

- Which env vars to set in the wrapper and where they map in the
  existing AI Gateway docs.
- How AI Governance Add-on entitlements interact with the worker.
- A clear note that this is opt-in. The worker's own outbound
  traffic to `api.cursor.com` is unaffected.

### Stage D: Pin permissions and tool allowlists in the image

Cursor's worker honors session-level configuration: tool allowlists,
permission scopes, model selection, and project-scoped overrides.
Operators can ship a baseline via the image.

Docs page will cover:

- The settings file shape and location.
- How to ship project-scoped overrides via `<repo>/.cursor/`.
- How allowlists, denylists, and permission modes interact, with
  copy-pasteable wrapper examples.

This is purely an image and settings exercise; no product work.

## Sequencing and review

| Stage             | Pages                                  | Reviewers                   | Notes                                                                |
|-------------------|----------------------------------------|-----------------------------|----------------------------------------------------------------------|
| System identity   | overview, system-identity, plan        | docs, AI team, platform-eng | Ship as a single PR. Don't gate on User identity. Includes the prebuilds primary path and the daemon alternative. |
| Stage A           | per-creator credentials page           | infra, security             | Pair with a reference wrapper script.                                |
| Stage B           | lifecycle hooks page                   | infra, source-control       | Pair with a Coder template that demonstrates the cache volume layout. |
| Stage C           | AI Gateway integration page            | AI Gateway maintainers      | Behind AI Governance Add-on entitlement.                             |
| Stage D           | permissions and allowlists page        | security, AI team           | Cribs from the Cursor docs + existing settings content.              |
| User identity     | middleware reference, plan diff        | platform-eng, AI team       | Depends on Cursor publishing `agent:*` and stable pending-requests; either middleware or in-tree `coderd` integration on Coder's side. |

## Risks and open issues

- **EAP churn.** The `cursor-agent worker` flag set, fleet API shape,
  and sub-token contract are all flagged as subject to change. Ship
  System identity with a clear EAP banner and pin the `cursor-agent`
  version baked into the image.
- **Two-source ownership.** Cursor owns the worker binary, the pool,
  and the session control plane. Coder owns the workspace and the
  observability around it. A reader hitting a problem needs to know
  which logs to read first; the troubleshooting section in
  `system-identity.md` is the first attempt at that and will need
  iteration.
- **One worker, one repo.** Cursor's worker is single-repo today.
  Multi-repo sessions would change the pool-per-repo recommendation.
  Track as an open question for Cursor.
- **Persistence.** Cursor's worker expects a fresh checkout per
  session restart. Coder workspaces typically persist `$HOME`. The
  recipe lets `$HOME/workspace` persist across `coder stop / start`
  because the startup script `git reset --hard origin/HEAD`s it
  before each session; a stricter deployment may want an `emptyDir`
  checkout path.

## Open questions for Cursor

### `agent:*` scope graduation

User identity blocks on a team service-account key with `agent:*`
scope being available. Today, calling `POST /v1/sub-tokens` from a
non-scoped team key returns 403 with the message "Sub-tokens require
a team service-account API key with the `agent:*` scope." We need:

- Issuance of the scope, with a documented surface in
  `cursor.com > Settings > Team > API Keys`.
- A stable scope name (`agent:*` vs. something narrower) and a
  documented permission table.
- Clarity on whether scoped keys can be issued to existing teams
  without re-onboarding.

### Sub-token semantics

`POST /v1/sub-tokens { forUserId }` returns an access token. We
need:

- **TTL.** What is the token's lifetime? Does the worker refresh, or
  does a long session outlast its token? If refresh is required, what
  endpoint does the worker call and with what credentials?
- **Revocation.** Does revoking the parent team key revoke
  outstanding sub-tokens? Is there a per-sub-token revoke?
- **Audit.** Does the Cursor session log show "user X via sub-token
  minted by team Y" or just "user X"? Operators will want the chain.

### Stable user-lookup endpoint

The router needs to resolve `userId` -> `email` (or directly to a
Coder username via SSO claims) for **users other than itself**. Today
`GET /v1/me` exposes the caller's profile; the router can't use that
for users it doesn't impersonate. We need a `GET /v1/users/{id}` or
the email on the `pending-requests` payload.

### Pending-requests stability and a webhook scaling signal

The router today polls `GET /v0/private-workers/pending-requests`,
which is on the legacy v0 surface. We need:

- A `/v1/` equivalent with a stable payload shape (`userId`,
  `userEmail` or a stable lookup id, `repoUrl`, `requestId`,
  `queuedAt`).
- A webhook that fires on queue events, so the router can spawn in
  milliseconds instead of waiting for a poll interval. The wire
  format and auth shape need to be documented before we can wire
  receivers.

### One-worker-one-repo: permanent or transitional?

`cursor-agent worker start --worker-dir <path>` takes a single repo.
The fleet API exposes `repoUrl` (singular). Is multi-repo per worker
on the roadmap, or is "one worker per repo" the permanent answer?
This decides whether per-repo pools are an unfortunate workaround or
the canonical shape.

### Lock and drain hooks

The worker has no event hooks. There is no way for the worker to
tell the surrounding orchestrator that it just picked up a session,
finished one, or is about to drain. Today the only signals are:

- Tail `cursor-agent.log` for "claim" / "release" lines.
- Poll `:8080/readyz` and the fleet API.

A `lock` hook (or a per-worker webhook to a configurable URL) would
unlock:

- Push lock and drain events into Coder's audit log so admins have a
  single pane of glass.
- Pre-mint per-user credentials at lock time, not at first-session
  time.
- Update the workspace's Coder display name to "serving alice"
  automatically. Today we can only poll `/readyz` to learn busy/idle
  and update metadata every few seconds.

This is a smaller and more concrete ask than the user-identity work
and would be easy to land in EAP.

## Open questions for Coder

These are things that *would* require a Coder product change. They
are not in scope for this docs effort, but they are the natural
product follow-ons and we want them captured.

### Service-account-owned prebuilds

Coder's prebuilds primitive maintains N warm workspaces owned by a
synthetic prebuilds service account. We use that as the inventory
for System identity, but we hit a real limitation: **the prebuilds
service account cannot complete an OAuth flow**, so
`coder_external_auth` resolves to nothing inside a prebuilt
workspace.

The workaround System identity ships with is "block pushes." That is
fine for system identity, but it means we cannot use Coder's
external-auth refresh story for the bot.

The product change that would close this gap: **let prebuilds be
configured to use a specific operator-supplied service-account user**
instead of the built-in synthetic one. The operator can grant that
user external-auth normally, and every prebuild inherits the link.
The prebuild becomes a credentialed bot workspace, not just an
anonymous warm body.

This is a small, contained change. It also makes prebuilds useful
for other non-claim use cases (background data processing, scheduled
maintenance) where "we want N workspaces owned by a bot to do work"
is the actual goal.

### Configurable workspace naming format for prebuilds

Prebuilt workspaces are named `prebuild-{8-char-random}` and owned
by the synthetic `prebuilds` user. Cursor's worker picker shows that
name verbatim (we pass `--name coder-${owner}-${workspace_name}` to
`cursor-agent worker start`), so the user-facing label becomes
`coder-prebuilds-prebuild-zb5n4qrnrheurty`. Functional, unreadable.

The Cursor template can work around this by overriding `--name` to
something shorter inside the agent startup script (the daemon path's
`cursor-worker-{unix-timestamp}` is in the same shape), but the
underlying issue is that operators have no control over how Coder
names prebuilt workspaces.

The product change: a template-level
`prebuilds { name_format = "..." }` or a deployment-level setting
that lets operators substitute a different template. Useful here,
useful for any prebuilds use case where the workspace name surfaces
in a third-party UI.

Low priority. The template-side workaround is one line.

### First-class headless workspace pool primitive

User identity's router reimplements a small queue manager around
Coder's workspace API. A Coder primitive that knows how to spawn a
workspace per external signal (webhook, queue event), bind it to a
specific external identity, run it until drain, and reclaim it,
would close this gap. The router collapses from "a service to write"
to "a config block in the template."

This is the most useful product addition for the autoscaled fleet
case and would let us ship a single integration story rather than
"two phases, one of which is middleware you write."

### AI Gateway client preset for worker children

Stage C documents how to point the child model traffic at AI
Gateway. A first-class client preset (similar to existing presets
in AI Gateway) would mean template admins do not have to wire the
env vars themselves, and AI Gateway can apply policy specifically
to worker-originated traffic.

### Audit log integration for worker events

Today, Coder's audit log knows nothing about Cursor worker state.
Combined with the Cursor-side "lock and drain hooks" ask above, a
small Coder integration could surface those events alongside Coder's
existing audit log. The plumbing is small once the hook contract
exists on Cursor's side.

None of these Coder asks block System identity or User identity.
They are the follow-on product work that this docs effort makes
possible to scope.
