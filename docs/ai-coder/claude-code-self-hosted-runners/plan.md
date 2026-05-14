# Implementation notes: Claude Code self-hosted runners on Coder

This page captures the staged plan and the open questions behind the
two customer-facing identity models. It is the place to look if you
are evaluating the [System identity](./system-identity.md) recipe and
want to understand the trade-offs we accepted, or if you are tracking
what blocks [User identity](./user-identity.md) from shipping.

The constraint for the **shippable** parts of this plan is that they
use only Coder primitives that exist today. Anything that would require
a Coder product change is called out explicitly in the
[Open questions for Coder](#open-questions-for-coder) section. Anything
that would require an Anthropic change is in the
[Open questions for Anthropic](#open-questions-for-anthropic) section.

The customer-facing pages describe two identity models:
[System identity](./system-identity.md) (shippable today on Coder
primitives that exist) and [User identity](./user-identity.md) (on the
roadmap, depends on Anthropic protocol pieces still being finalized).
This page is the design history behind both models, plus the sub-stages
and the open questions tracked alongside the delivery.

## Goals

- Give platform teams a clear path from "I have an Anthropic pool" to
  "developers can route Claude Code sessions to Coder workspaces."
- Be explicit about which pieces ship today on Coder primitives and
  which depend on contracts Anthropic or Coder has not finalized.
- Make it obvious which Anthropic features (wrapper scripts, lifecycle
  hooks, multi-account pools) translate to which Coder primitives so we
  don't accidentally pull product work into a docs project.
- Be honest about the rough edges so future product work has a clear scope.

## Non-goals

- Building any new Coder UI, API, or module *as part of the system
  identity recipe*. User identity
  and the [Coder open questions](#open-questions-for-coder) describe
  product work that is in scope for the follow-on, but not for the
  initial docs delivery.
- Wrapping the runner binary in a Coder-distributed package.
- Anthropic-side changes (pool management, JWT contents, scheduling).
  These are tracked as [open questions for Anthropic](#open-questions-for-anthropic).

If we want any of those, that is separate product work that should be
proposed in a Linear issue, not in this docs branch.

## System identity (shippable today)

A pool of bot-owned Coder workspaces, each running one Claude Code
self-hosted runner, behind a Coder prebuilds preset that maintains a
configurable number of warm runners. Anthropic's pool scheduler chooses
which prebuilt workspace serves which user; Coder's role is to keep N
workspaces warm and to refill the pool when one drains.

**Identity model:** every commit is the bot. Pushes use a bot PAT or
deploy key delivered to the workspace via a sensitive template variable.
The Anthropic session JWT identifies the human in the commit trailer that
Claude Code already appends, but the git `Author` and the credential
pushing the commit are both the bot.

### Pieces

1. **Workspace template** that bakes the runner binary and starts it under
   `coder_script` with `run_on_start = true`. Today's template under
   `examples/` plus the [System identity recipe](./system-identity.md) is the reference.
2. **Sensitive template variables** for the pool secret and the bot's git
   credential (PAT or private key). Both are fleet-wide, both rotate by
   re-pushing the template.
3. **`coder_agent` metadata** that scrapes the runner's `/healthz` and
   `/metrics` endpoints to surface the locked Anthropic user, active
   session count, runner ID, and last-poll age on the workspace page.
4. **Self-eviction** in `coder_script`: after the runner exits 0 on drain,
   call `POST /api/v2/workspaces/{id}/builds` with `{"transition":"delete"}`
   using a **per-workspace, scope-restricted** Coder API token minted at
   template-build time. The token is scoped to
   `workspace:delete + workspace:read + template:read + user:read` and
   allow-listed to this workspace's UUID. A leaked copy can only delete
   this one workspace; no read of peer prebuilds, SSH, external auth,
   or git creds. The token is minted via the `Mastercard/restapi`
   Terraform provider hitting `POST /api/v2/users/{svc}/keys/tokens` at
   template build time, using a long-lived bootstrap admin token kept
   in Terraform state but never injected into the workspace. The
   prebuild reconciler sees the deficit and queues a replacement.
5. **A prebuilds preset** with `prebuilds { instances = N }` and a TTL of
   roughly 8 hours, so unused warm workspaces also get recycled and the
   pool always presents fresh disks to Anthropic.
6. **A per-session wrapper script** baked into the image at
   `/opt/claude/wrapper.sh` and wired via `--exec-path`. The wrapper
   appends `--permission-mode bypassPermissions` after `"$@"` so the
   runner never stalls on a tool-approval prompt (sessions have no
   terminal attached). Per the Anthropic PDF, Claude Code's flag
   parser is last-occurrence-wins, so the wrapper overrides whatever
   permission mode the server sent.
7. **Host Docker socket passthrough** so the runner's child claude can
   `docker build` / `docker run` for sessions that need it. The
   container mounts `/var/run/docker.sock` from the host and the
   startup script chgrps the socket to the in-container `docker`
   group if the gid doesn't match. This is root-equivalent on the
   host; it matches dogfood's everyday workspace behavior and is
   acceptable for the EAP recipe.

### What this gives you

- A self-healing pool of N warm runners that recover from drain and from
  the EAP-grade rough edges in the runner binary.
- Surface-agnostic: web tabs, mobile, routines, agents all land on the
  same pool the same way.
- The whole pool reproduces from a template push and three sensitive
  variables. No external orchestrator, no webhook receiver.

### What this does not give you

- **Per-user git identity.** All commits are the bot. The Anthropic
  session URL trailer (auto-appended by Claude Code) is your only
  per-creator audit signal in the git history.
- **Per-user external auth.** Coder's `coder_external_auth.github` reads
  the workspace owner's linked token. The workspace owner is the
  prebuilds service account, which has no OAuth flow. So external-auth
  does nothing useful here; you fall back to a bot PAT or SSH deploy
  key shipped as a sensitive variable.
- **Per-user audit attribution in Coder.** Coder's audit log says
  "prebuild service account did stuff." The richer "alice asked Claude
  to do stuff" trail lives in Anthropic's session log, not Coder's.
- **Capacity guarantees.** A pool of 5 runners can serve 5 concurrent
  Anthropic users. The 6th waits in Anthropic's queue. Tune `instances`
  for your expected concurrency.

### System identity acceptance criteria

- A reader who has never seen self-hosted runners can stand up a working
  Coder template using only the [System identity recipe](./system-identity.md) and the
  runner build from Anthropic.
- The reader uses only shipped Coder primitives. Where System identity hits a
  limitation in those primitives (the prebuilds service account cannot
  complete an OAuth flow, so external auth does not resolve inside a
  prebuilt workspace), the recipe documents the workaround (bot PAT via
  sensitive variable) and the gap is filed in the
  [open questions for Coder](#open-questions-for-coder).
- The pool maintains N warm runners through drain, error exit, and TTL
  expiry.

## User identity (on the roadmap)

A small webhook receiver listens for Anthropic's `runner-needed` event,
maps the session creator (from the event payload) to a Coder user, and
calls Coder's workspace-create API **on behalf of that user**. The
middleware authenticates to Coder as a dedicated service account with a
scoped admin-like token.

This is what unlocks per-user identity: when the workspace is created on
behalf of the human, Coder's external-auth wires `GIT_ASKPASS` with the
human's GitHub token, audit log entries attribute to the human, and the
runner's `--lock-to-account` pre-locks it before the first session
arrives so there is no first-session-wins race.

### Pieces

1. **Webhook receiver, or in-tree integration.** Two implementation
   paths, both valid:
   - **Middleware service.** A few hundred lines of Go or Node.
     Verifies the Anthropic signature, looks up the user in Coder via
     the API, pre-flights that the user has the required
     external-auth grants, calls `POST /workspaces` on the user's
     behalf with `--lock-to-account` as a parameter. Faster to ship;
     can land the day Anthropic's webhook ships.
   - **First-class integration in `coderd`.** Coder's server consumes
     the Anthropic webhook directly, with user-mapping rules and the
     on-behalf-of spawn built in. The webhook receiver collapses from
     "a service to write" to "a config block in the template." Better
     long-term shape; absorbs whatever middleware deployments teach
     us about the right defaults.
2. **Coder service-account token.** A scoped API token owned by, for
   example, `svc-claude-pool`. Scopes: create workspaces on behalf of
   users, read users, read and delete workspaces. Same pattern as the
   `svc-claude-delete` bootstrap token system identity already uses
   for per-workspace self-eviction. Vaulted, rotated. Never a human
   admin's PAT.
3. **`--lock-to-account` parameter on the template.** A new
   `coder_parameter` that flows through to the runner CLI. Default empty
   (behaves like System identity); set by the middleware on every spawn.
4. **The system identity prebuilds preset and self-eviction.** User
   identity reuses the system identity pool as inventory. The middleware
   claims a warm prebuild on behalf of the user, which atomically
   transfers ownership from the prebuilds service account to the human.
   The claim build runs with the human's owner context, so external-auth
   resolves to their token.

### Open questions for Anthropic

These are documented in the dedicated [open questions](#open-questions-for-anthropic)
section below. The two that block User identity specifically:

- The shape and auth contract of the `runner-needed` webhook.
- Graduation of `--lock-to-account` from its current "(pending)" status.

### User identity acceptance criteria

- The middleware can convert one Anthropic `runner-needed` event into one
  Coder workspace owned by the matching human.
- The runner is born locked to that human (no first-session-wins race).
- `git push` from the runner uses the human's external-auth token.
- Coder's audit log attributes the workspace to the human, with the
  service account shown as the on-behalf-of creator.

## Sub-stages within system identity (docs follow-ons)

These layer on top of System identity and are pure documentation and template
work. They are stage-numbered for sequencing, not because they require
User identity to ship.

### Stage A: Per-creator credentials via wrapper script

Anthropic's runner exposes `CLAUDE_CODE_SESSION_ACCESS_TOKEN`, a JWT
whose `act` claim carries the session creator's email and IdP subject.
Operators are expected to decode that JWT in a wrapper script (passed via
`--exec-path`) and provision creator-scoped credentials before exec'ing
into the real binary.

This stage adds a docs page that:

- Explains the JWT and links to the
  [JWKS endpoint](https://api.anthropic.com/v1/code/.well-known/jwks.json).
- Provides a wrapper-script template that decodes the JWT, exchanges it
  for short-lived credentials from your IdP (the PDF example uses AWS
  STS; we should also document a Vault example), and execs the bundled
  `claude` binary.
- Calls out the warning from the Anthropic PDF that JWT claim shapes are
  subject to change during the EAP, and recommends pinning a runner build
  before relying on specific claims.

This is a no-op from Coder's perspective; the wrapper just runs inside the
workspace.

Open questions:

- How does this interact with the workspace's own identity? If the
  workspace is already authenticated to AWS or Vault via instance metadata
  or workload identity, we should document precedence so the wrapper does
  not silently use the workspace identity instead of the creator's.
- Should we recommend that the workspace template inject the wrapper
  script path via `--exec-path` from a `coder_script` argument, or via the
  `--hooks-dir` `command` hook? Both work; pick one as the recommended
  pattern.

### Stage B: Custom checkout via lifecycle hooks

The runner supports `--hooks-dir <path>` and looks for executable scripts
with well-known names. The `checkout` hook is the most useful for Coder
deployments because it lets us:

- Clone from an internal Git replica (`ghes-replica.internal`) instead of
  the verbatim source URL.
- Use a local bare mirror with `--reference-if-able` to skip re-fetching
  shared objects, which is a big win when many sessions land on the same
  workspace.
- Materialize non-git sources (Perforce, S3 tarball) by setting
  `CLAUDE_RUNNER_SKIP_GIT_VERIFY=1`.

Docs page should include:

- A reference `checkout` hook that mirrors the PDF example but uses a
  `/var/cache/git-mirrors` location appropriate for a Coder workspace
  (likely on a persistent volume so it survives workspace restart).
- Guidance on which workspace volumes should persist vs which should be
  ephemeral. The runner *expects* a fresh filesystem on restart, but a
  read-only bare mirror is fine to persist.

### Stage C: Route the child through AI Gateway

The runner process itself must talk to `api.anthropic.com` for pool
registration and polling, but the child `claude` process makes its own
outbound LLM calls. Those calls can be routed through Coder's
[AI Gateway](../ai-gateway/index.md) by setting `ANTHROPIC_BASE_URL` and
the appropriate auth headers in the wrapper script.

This is interesting because it gives platform admins audit and policy
coverage over the *model traffic* the runner generates, without changing
how Anthropic dispatches the session.

Docs page should cover:

- Which env vars to set in the wrapper (`ANTHROPIC_BASE_URL`,
  `ANTHROPIC_AUTH_TOKEN`, `ANTHROPIC_CUSTOM_HEADERS`) and where they map
  in the existing [AI Gateway Claude Code](../ai-gateway/clients/claude-code.md)
  doc.
- How AI Governance Add-on entitlements interact with the runner. If the
  add-on is not enabled, this stage does not apply.
- A clear note that this is opt-in. The runner's own outbound traffic to
  `api.anthropic.com` is unaffected.

### Stage D: Pin permissions and tool allowlists in the image

The runner gives each session its own `CLAUDE_CONFIG_DIR` seeded from
`~/.claude/` in the image. That means a template admin can ship a
`settings.json` with deny rules (`Bash(rm -rf:*)`, etc.) and skills,
commands, and CLAUDE.md content as a baseline for every session served by
the workspace.

Docs page should cover:

- The `settings.json` shape from the Anthropic PDF.
- How to ship project-scoped overrides via `<repo>/.claude/settings.json`.
- How `--permission-mode auto`, `--allowed-tools`, and `--disallowed-tools`
  interact, with copy-pasteable wrapper examples.
- The opt-in commit-nudge `Stop` hook the PDF mentions.

This is purely an image and settings exercise; no product work.

## Sequencing and review

| Stage             | Pages                            | Reviewers                   | Notes                                                                 |
|-------------------|----------------------------------|-----------------------------|-----------------------------------------------------------------------|
| System identity   | overview, system-identity, plan  | docs, AI team, platform-eng | Ship as a single PR. Don't gate on User identity. Includes the per-session wrapper (`--permission-mode bypassPermissions`), per-workspace scoped self-eviction token, and host Docker socket passthrough; Stage A is no longer a separate page. |
| Stage B           | lifecycle hooks page             | infra, source-control       | Pair with a Coder template that demonstrates the cache volume layout. |
| Stage C           | AI Gateway integration page      | AI Gateway maintainers      | Behind AI Governance Add-on entitlement.                              |
| Stage D           | permissions and skills page      | security, AI team           | Mostly cribs from the PDF + existing `~/.claude` content.             |
| User identity     | middleware reference, plan diff  | platform-eng, AI team       | Depends on Anthropic publishing the webhook contract; either middleware or in-tree `coderd` integration on Coder's side. |

## Risks and open issues

- **EAP churn.** The runner build, JWT claim shape, scaling signals, and
  flag set are all flagged in the PDF as subject to change during the EAP.
  We should ship System identity with a clear EAP banner and pin the documented
  `BYOC_VERSION` to whatever Anthropic gave us at write time.
- **Two-source ownership.** Anthropic owns the runner binary, the pool, and
  the session control plane. Coder owns the workspace and the
  observability around it. A reader who hits a problem will need to know
  which logs to read first; the troubleshooting section in `system-identity.md` is
  the first attempt at that and will need iteration.
- **Multi-repo sessions.** The PDF mentions that multi-repo sessions spawn
  from a parent directory with `--add-dir` per repo. System identity does
  not exercise this. We should add a short note once we have tested it.
- **Persistence.** Anthropic expects each runner restart to give a fresh
  filesystem. Coder workspaces typically persist `$HOME`. We default to
  "keep `$HOME` persistent, treat workspace stop/start as the restart
  boundary," but a stricter deployment may want an `emptyDir` checkout
  path. Document both.

## Open questions for Anthropic

### Stateful vs. ephemeral runners

The runner protocol today is explicitly ephemeral. Lock to the first
session's account, serve until drain, `exit 0`, let the orchestrator
hand the next process a fresh disk. From the PDF:

> "This model isolates each user's checked-out code and build artifacts
> without requiring the runner to scrub disk state between users."

That is the right design for fleet pools where many users share
machines. It is not a natural fit for a Coder workspace, which is
durable, single-user, and stateful by design. A Coder workspace
already keeps `$HOME`, dotfiles, branch state, IDE state, and
dependency caches across `coder stop` / `coder start`. The workspace
itself is the persistence boundary.

The concrete user scenario this creates friction for:

1. Developer opens `claude.ai/code`, sends "work on issue 1234" at 10am.
2. Child claude makes progress, hits a permission prompt at 10:15am, and
   stalls waiting for input.
3. Developer closes the laptop, goes to lunch.
4. Developer comes back at 1pm. Today the session is gone:
   `--release-idle-session-min` released the slot, the runner drained
   to zero, `exit 0`, fresh disk, new runner ID. The pending prompt,
   the half-finished working tree, the in-flight tool output are all
   discarded.
5. The developer's mental model says "I'll pick up where I left off."
   The runner contract says they cannot.

We would like to ask Anthropic: **is the ephemeral, drain-and-respawn
contract a permanent design decision, or is there room for a second
mode for runners that are contractually single-user?**

What we mean by "stateful single-user mode":

- **Sticky lock that outlives the process.** When a runner exits 0 on
  drain today, the lock evaporates with it. For a Coder workspace
  where the operator can guarantee one Anthropic user per workspace,
  the lock could be a property of the workspace identity, not the
  runner process. `--lock-to-account` already exists for webhook spawn;
  expose it as something the workspace itself asserts on startup, so
  there's no first-session-wins race.
- **Doesn't leave when the chat stops or stalls.** A "warm pause"
  state where the child claude and its working tree stick around long
  enough for the user to come back and resume the same `cse_id` in
  place. The runner already supports `--drain-grace-sec` for the
  warm-reuse window between sessions; the question is whether the
  same idea can apply to a single session that's idle but not
  abandoned.
- **`/workspace` survives `exit 0`.** This only makes sense if the
  runner is contractually single-user, because today disposal is how
  Anthropic guarantees no cross-user leakage. If the operator can
  prove single-user (Coder workspaces can), the runner could opt out
  of the fresh-disk requirement and let interrupted edits survive a
  respawn.
- **Per-user runner identity.** Today `ccrunner_01...` rotates on
  every spawn. For audit and dashboards, a stable identity tied to
  the Coder workspace rather than the process lifecycle would be more
  useful.

We are not asking Anthropic to throw out the ephemeral model. It is
the right shape for fleet pools and the right answer for "isolation by
disposal." The question is whether there is room for a **second mode**
that opts into "this runner is single-user; please give us state
continuity in exchange for that guarantee."

If the answer is "no, runners are always ephemeral":

- We document `/workspace` as strictly transient and never persist it.
- "The user's Claude session on their workspace" is a transient
  relationship the workspace cannot make durable. The docs say so
  plainly.
- User identity becomes the only correct deployment shape for
  multi-user scenarios. System identity's pool of warm runners is the
  only correct shape for system-identity deployments.

### Webhook payload and `--lock-to-account` graduation

User identity depends on two interfaces the PDF flags as on Anthropic's
roadmap but not yet shipped:

- The `runner-needed` webhook. The PDF describes it (one event per
  queued session with no available runner, plus a CLI poll fallback)
  and says "tell us which scaling signal fits your infrastructure,
  what payload fields you need to provision a runner (for example:
  pool ID, the account the runner should serve, repository URLs to
  pre-clone), and what authentication shape your webhook receiver
  expects." The wire format is not finalized and there is no
  destination URL to configure today. We have a concrete consumer
  (the middleware or coderd-integration in User identity) and want
  to influence the contract before it is finalized.
- The `--lock-to-account` flag is documented today as "intended for
  webhook-driven spawn (pending)." User identity's no-first-session-
  wins property depends on it. We need to know whether it will
  graduate from pending and whether the locked account must already
  have queued sessions, must belong to the pool's org, etc.

Specific asks for the webhook payload:

- Include the IdP `sub` claim, not just `email`. Email is mutable and
  not reliably unique across IdP federations; `sub` is the right key
  for cross-system user mapping.
- Include the pool ID so the middleware can spawn into the right Coder
  template if you have multiple pools.
- Document the signature scheme up front (HMAC with timestamp and
  shared secret is the cheapest workable shape).

### Lock and drain event hooks

The runner has policy hooks (`checkout`, `command`) but no event hooks.
There is no way for the runner to *tell* the surrounding orchestrator
that it just locked to a user, that a session just finished, or that
drain just started. Today the only signals are:

- Tail stderr for `Registered:`, `Picked up session`, `account workload
  drained` log lines.
- Scrape `claude_code_self_hosted_runner_locked_account{email}` from
  `/metrics`.

A `lock` hook (or a per-runner webhook to a configurable URL) would
unlock integrations we want to build:

- Push lock and drain events into Coder's audit log so admins have a
  single pane of glass.
- Pre-mint per-user credentials at lock time, not at first-session
  time. This shaves seconds off the first session.
- Update the workspace's Coder display name to "serving alice@example
  .com" automatically. Today we can only poll `/metrics` to learn this
  and update the metadata every 10 seconds.

This is a smaller and more concrete ask than the stateful-vs-ephemeral
question and would be easy to land in EAP. The hook directory contract
already exists; this would add two hook names (`on_lock`, `on_drain`)
that fire once each per runner lifecycle.

## Open questions for Coder

These are things that *would* require a Coder product change. They are
not in scope for this docs effort, but they are the natural product
follow-ons and we want them captured.

### Service-account-owned prebuilds

Coder's prebuilds primitive maintains N warm workspaces owned by a
synthetic prebuilds service account. We use that as the inventory for
System identity, but we hit a real limitation: **the prebuilds service account
cannot complete an OAuth flow**, so `coder_external_auth` resolves to
nothing inside a prebuilt workspace.

The workaround System identity ships with is "deliver a bot PAT via a sensitive
template variable." That is fine for system identity, but it means we
cannot use Coder's external-auth refresh story for the bot.

The product change that would close this gap: **let prebuilds be
configured to use a specific operator-supplied service-account user**
instead of the built-in synthetic one. The operator can grant that user
external-auth normally, and every prebuild inherits the link. The
prebuild becomes a credentialed bot workspace, not just an anonymous
warm body.

This is a small, contained change. It also makes prebuilds useful for
other non-claim use cases (background data processing, scheduled
maintenance) where "we want N workspaces owned by a bot to do work" is
the actual goal, not "we want N workspaces ready to be claimed by
humans."

### First-class headless workspace pool primitive

user identity's middleware reimplements a small queue manager around Coder's
workspace API. A Coder primitive that knows how to spawn a workspace
per external signal (webhook, queue event), lock it to a specific
external identity, run it until drain, and reclaim it, would close
this gap. The webhook receiver collapses from "a service to write" to
"a config block in the template."

This is the most useful product addition for the autoscaled fleet case
and would let us ship a single integration story rather than "two
phases, one of which is middleware you write."

### AI Gateway client preset for runner children

Stage C documents how to point the child `claude` process at AI Gateway
via `ANTHROPIC_BASE_URL`. A first-class client preset (similar to the
existing Claude Code preset in AI Gateway) would mean template admins
do not have to wire the env vars themselves, and AI Gateway can apply
policy specifically to runner-originated traffic.

### Audit log integration for runner events

Today, Coder's audit log knows nothing about Anthropic runner state.
Combined with the Anthropic-side "lock and drain hooks" ask above, a
small Coder integration could surface those events alongside Coder's
existing audit log. The plumbing is small once the hook contract exists
on Anthropic's side.

None of these Coder asks block System identity or User identity. They are the
follow-on product work that this docs effort makes possible to scope.
