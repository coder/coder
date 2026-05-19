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
external reconciler daemon. Cursor's label-based routing chooses which
workspace serves which session; Coder's role is to keep N workspaces
warm per repo and to refill the pool when one drains.

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

## User identity

Two recipes that ship today, both validated end to end against the
live Cursor API:

- **[Personal Workers](./personal-workers.md)** for per-user
  identity. One Coder workspace per user, the user's own Cursor API
  key, no `--pool`. Per-user attribution end to end (workspace
  owner, git push via Coder external auth, Cursor session log,
  Coder audit log). Programmatic session submission via
  `POST /v1/agents { env: { type: machine, name } }` lets a Coder
  Tasks UI launch sessions on a developer's worker without the user
  leaving Coder.
- **[Worker Pool](./system-identity.md)** for fleet-wide bot
  identity. Shared warm pool, every worker authenticates with the
  team service-account key.

The shape we originally tried (per-user identity **with** a shared
warm pool, driven by a router that polls `pending-requests` and
claims prebuilds for the matching user) is **not shippable today**.
Three live findings explain why and what would unblock it. See
[User identity: status](./user-identity.md) for the long form; the
gating Cursor gaps are filed below under
[Open questions for Cursor](#open-questions-for-cursor).

### User identity acceptance criteria

When the shared-pool shape becomes shippable, the recipe must:

- Convert one Cursor pending request into one Coder workspace owned
  by the matching human (no out-of-band trigger required).
- Register a worker on the claimed workspace that Cursor's pool
  scheduler will dispatch the matching queued request to, without
  ambiguity from other queued requests for other users.
- Use the human's external-auth token for `git push` from the
  worker.
- Attribute the workspace to the human in Coder's audit log, with
  the router service account shown as the on-behalf-of creator.

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

This stage is **not shippable today**. The structural reason is
documented separately in
[AI Governance integration (notes)](./ai-governance.md): Cursor's
worker process does not make model-provider API calls. The agent
loop runs in Cursor's cloud and sends tool calls down to the worker
over an outbound HTTPS connection to `api2.cursor.sh`. AI Gateway
does not sit on a path the worker traverses, so there is nothing for
a wrapper script to redirect.

For the page to become shippable, one of the following Cursor-side
changes has to land:

- A session-level wrapper hook on the worker that mediates model calls
  with operator-supplied provider config (analogous to Claude Code's
  `--exec-path`).
- A first-party gateway feature on Cursor's side that an enterprise
  can point at an internal endpoint.

Until then, the linked addendum is the canonical answer when a reader
asks about AI Gateway coverage. The desktop Cursor client is also not
supported by AI Gateway today, for an unrelated upstream reason. Both
are tracked in the addendum.

When this stage becomes shippable, the page will cover:

- The Cursor-side hook contract and which env vars or config it accepts.
- How AI Governance Add-on entitlements interact with the worker.
- A clear note that this is opt-in. The worker's own outbound traffic
  to `api2.cursor.sh` is unaffected.

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

| Stage           | Pages                           | Reviewers                   | Notes                                                                                                                                                                          |
|-----------------|---------------------------------|-----------------------------|--------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| System identity | overview, system-identity, plan | docs, AI team, platform-eng | Ship as a single PR. Don't gate on User identity. Includes the prebuilds primary path and the daemon alternative.                                                              |
| Stage A         | per-creator credentials page    | infra, security             | Pair with a reference wrapper script.                                                                                                                                          |
| Stage B         | lifecycle hooks page            | infra, source-control       | Pair with a Coder template that demonstrates the cache volume layout.                                                                                                          |
| Stage C         | AI Gateway integration page     | AI Gateway maintainers      | Behind AI Governance Add-on entitlement. Blocked on Cursor exposing a worker-side hook that mediates model calls. See [AI Governance integration (notes)](./ai-governance.md). |
| Stage D         | permissions and allowlists page | security, AI team           | Cribs from the Cursor docs + existing settings content.                                                                                                                        |
| Personal Workers | personal-workers, user-identity | docs, AI team, platform-eng | Ships today for per-user identity. Includes the programmatic `env.type: machine` submission flow that lets a Coder Tasks UI drive sessions.                                  |
| User identity (shared pool) | user-identity (status only) | platform-eng, AI team       | Blocked on Cursor shipping empty-pool visibility or per-user pool affinity. See [Open questions for Cursor](#open-questions-for-cursor).                                       |

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

Each question is marked with its current status: **Confirmed shipped**
when the Cursor docs document a stable answer we have verified against
the live API; **Partially shipped** when a workable surface exists but
an important refinement is still missing; **Still open** when there is
no documented answer or the surface we need does not exist yet.

### Empty-pool visibility in the UI (still open, blocks shared-pool user identity)

Cursor's `Self-hosted Pools` dropdown only lists pools that
currently have at least one registered worker. A pool with zero
workers is invisible, so a user cannot pick it from cursor.com to
submit a session. The pool name does not persist after the last
worker disconnects.

The shared-pool user-identity shape needs the user to submit a
session against a pool that has no workers yet, so the router can
claim a workspace for the right human and register a worker for
them. Without UI visibility for the empty pool, the user has no
entry point. We need one of:

- A documented way to create a pool in the team's UI without
  registering a worker first (e.g., a `POST /v1/pools` endpoint or
  a team-admin setting in `cursor.com > Settings`).
- Per-user pool affinity on the scheduler so a "decoy" worker can
  exist for visibility without catching the next session (see
  below).

### Pool scheduler dispatches opportunistically (still open, blocks shared-pool user identity)

When a pool has any registered worker, Cursor's scheduler dispatches
the next incoming request to that worker immediately. There is no
queue moment a router can observe.

That is correct behavior for the fleet-bot pool case
([Worker Pool](./system-identity.md)), where any free worker is fine
because they all share one identity. It breaks the shared-pool
user-identity case, where the worker needs to be registered **for
the requesting user** before the dispatch happens.

We worked around the visibility problem during validation with a
"decoy" worker registered to `pool-user-claim` so the pool would
appear in the UI. The decoy then caught the first session the user
submitted (`activeBcId` bound to the decoy worker owned by the
`prebuilds` service account) before the router could see a queue.

The unblock options Cursor could ship:

- **Per-user pool affinity on the scheduler.** A worker declares
  `coder-owner=<email>` (or any documented identity claim), and the
  scheduler routes only matching users to it. The decoy then never
  matches anyone, so it can sit there for visibility while a router
  claims real workspaces on demand.
- **A pre-dispatch webhook on queue.** A pool can be configured to
  hold sessions until a hook returns the worker name. The router
  becomes the hook: claim a workspace, return its worker name. No
  decoy needed.
- **A documented per-session worker pre-allocation API.** The user
  submits a session, Cursor's UI hits a router-defined endpoint to
  resolve the target worker before posting to `/v1/agents`. Closest
  to today's `env.type: machine` shape but driven from cursor.com.

Any one of these is sufficient. Until one ships, per-user identity
ships as [Personal Workers](./personal-workers.md) with a one-time
workspace setup per developer.

### `agent:*` scope (confirmed shipped, not used in the shipping recipes)

The shipping recipes do not need `agent:*`-scoped service-account
keys. [Worker Pool](./system-identity.md) uses the un-scoped team
service-account key directly. [Personal Workers](./personal-workers.md)
uses each developer's own personal API key.

`agent:*` scope is what lets a key call `POST /v1/sub-tokens` to
mint per-user delegated tokens, which would otherwise live on a
single team key. We validated that flow against a real
`agent:*`-scoped key, but it is not on the path of either shipping
recipe.

### Pool worker auth: SA key only (confirmed shipped, accepted)

Cursor's CLI rejects `--pool` for delegated sub-token or personal-
key authentication. The error messages are unambiguous:
`Delegated service-account tokens cannot start pool workers (--pool).`
and
`You must use a team API key or service account to start a pool worker (--pool).`

This is not a limitation the shipping recipes try to remove. Pool
workers ([Worker Pool](./system-identity.md)) use the team SA key;
personal workers ([Personal Workers](./personal-workers.md)) do not
use `--pool` at all.

If Cursor later supports per-user pool workers (e.g., a `--pool`
and `--auth-token` combination, with the scheduler routing by the
delegated user), the shared-pool user-identity shape becomes
tractable. Until then, per-user identity ships as Personal
Workers.

### Stable user-lookup endpoint (partially shipped)

A `cursorUserId -> email` mapping is needed before the shared-pool
user-identity shape can route a queued session to the right Coder
user. The current surface area:

- `GET /v0/private-workers/pending-requests` returns `userId` on
  each row, but no email.
- `GET /v1/me` returns the caller's `userId`, `userEmail`, and
  display name. We confirmed this with a personal API key during
  validation: any user can self-discover their `userId` by hitting
  `/v1/me` with their own key.
- No `GET /v1/users/{id}` exists, so a router cannot resolve
  `userId -> email` for users other than itself.

A practical workaround for any future router is a self-registration
flow: each developer hits `/v1/me` with their own key once, the
result lands in a Coder-side mapping table, and the router reads
from there. We did not need this on the shipping path because
Personal Workers does not require server-side `userId` resolution.

### Pending-requests stability, rate limit, and a queue webhook (partially shipped, blocks shared-pool user identity)

The scaling signal works today, but only via polling, and the rate
limit is the cold-start floor:

- **Polling works.** `GET /v0/private-workers/pending-requests` is
  reachable with a team service-account key (HTTP Basic auth: SA key
  as username, empty password), paginated, and returns
  `{id, userId, serviceAccountId, repoOwner, repoName, repoUrl,
  labels[], createdAtMs}` per request. It is on the v0 surface; a v1
  equivalent has not been published.
- **Rate limit is 600 requests per hour.** The endpoint returns
  `429` with `retry-after` (in seconds) when the bucket is empty.
  At 600/hour a poller can hit it every 6 seconds with no margin;
  every 10 seconds gives headroom.
- **No queue-event webhook.** The only webhook Cursor documents is
  `statusChange` at
  [Webhooks](https://cursor.com/docs/cloud-agent/api/webhooks.md),
  which fires `FINISHED` or `ERROR` on a run that is **already
  executing**. By that point the worker has been chosen and the
  identity is fixed; this event is too late to influence routing.

What is still needed before this is a usable scaling signal:

- A `/v1/` equivalent of `pending-requests` so consumers do not
  depend on the legacy surface.
- A queue-event webhook for sub-second routing. Polling latency is
  the dominant factor in cold-start time on the shared-pool
  user-identity shape; a webhook would drop the user-visible delay
  to round-trip plus one Coder build.
- A higher rate limit, if multiple integrations on one team end up
  hitting it simultaneously.

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

### Worker-side hook that mediates session model calls

The Cursor self-hosted worker doesn't make any model-provider calls
from inside the workspace; the agent loop runs in Cursor's cloud and
sends tool calls down to the worker over `api2.cursor.sh`. That means
there is no traffic for Coder's AI Gateway (or any other internal
proxy) to apply policy to from the workspace side.

A worker-side hook that lets the operator mediate model calls (route
them through an internal proxy, swap providers, redact prompts before
they leave the workspace, etc.) would close this gap. Claude Code
exposes the equivalent via `--exec-path` on the `claude` CLI, which is
what the [Claude Code AI Gateway client preset](../ai-gateway/clients/claude-code.md)
relies on.

Full treatment of why AI Gateway is structurally unreachable today is
in [AI Governance integration (notes)](./ai-governance.md). This is the
open question that unblocks Stage C above.

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
workspace. User secrets are similarly disabled.

The workaround System identity ships with is "block pushes" plus
"pass the Cursor team API key as a sensitive Terraform variable."
That is fine for the fleet-bot pool, but it means we cannot use
Coder's external-auth refresh story for the bot identity. Personal
Workers sidesteps the issue because the workspace owner is the
human, so external auth resolves normally.

The product change that would close this gap: **let prebuilds be
configured to use a specific operator-supplied service-account user**
instead of the built-in synthetic one. The operator can grant that
user external-auth and user secrets normally, and every prebuild
inherits the link. The prebuild becomes a credentialed bot
workspace, not just an anonymous warm body.

This is a small, contained change. It also makes prebuilds useful
for other non-claim use cases (background data processing, scheduled
maintenance) where "we want N workspaces owned by a bot to do work"
is the actual goal.

Tracked in [coder/coder#25419](https://github.com/coder/coder/issues/25419).

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

The shared-pool user-identity shape (once Cursor unblocks it) needs
a small queue manager around Coder's workspace API: claim a
prebuild on behalf of a specific external identity, register a
worker for them, reclaim on drain. A Coder primitive that did this
natively, driven by an external scaling signal (webhook or polled
queue), would mean the integration is "a config block in the
template" instead of "a service to write."

This becomes interesting the moment the Cursor side surfaces a
pre-dispatch hook or per-user pool affinity. Until then there is no
queued signal for Coder to consume in the first place, so the
product change is gated on the Cursor change.

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
