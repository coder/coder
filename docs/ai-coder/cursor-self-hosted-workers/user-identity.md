# User identity: per-developer attribution

User identity will let Coder workspaces host Cursor private workers on
behalf of the **individual developer** who started the session, not
just a fleet-wide bot. The worker workspace becomes the developer's,
their git push credential is used, their commits are authored by them,
and Coder's audit log attributes worker activity to them.

<img src="../../images/guides/cursor-self-hosted-workers/user-identity-flow.svg" alt="When a Cursor session is queued for a user, a routing component maps the Cursor user to their Coder user, mints a per-user sub-token, and claims a warm prebuild on their behalf. The workspace owner flips to the human, Coder external auth wires their git push token, and the worker registers with the user's Cursor identity. User identity is planned and not yet available." />

> [!IMPORTANT]
> User identity is **not shaped like a pool**, and that is a Cursor-side
> constraint we have now confirmed with a real service-account API key.
> The full claim mechanism is validated end to end against a live Coder
> server, but with one important correction to the earlier design:
> **Cursor's CLI explicitly rejects `--pool` when the worker is
> authenticated with a delegated sub-token**, with
> `Error: Delegated service-account tokens cannot start pool workers (--pool).`
> Sub-tokens are scoped to the My Machines (personal-worker) surface,
> not the Self-Hosted Pool surface.
>
> What this means in practice:
>
> - The `Pool: user identity` preset claims a warm prebuild for a
>   target Coder user and starts `cursor-agent worker` **without**
>   `--pool`, using the per-user sub-token as `--auth-token-file`. The
>   resulting worker registers as a multi-use **personal worker** for
>   that Cursor user. Cursor's routing then targets it by name
>   (`machine=` or `worker=`), not by `pool=`.
> - The router still drives this: mint a sub-token for the requesting
>   user, claim the warm prebuild via `CreateUserWorkspace`. **The
>   trigger is the missing piece**: queued pool requests do not fire
>   on user-identity pools, because user-identity workers do not live
>   in a pool from Cursor's point of view.
>
> What's confirmed working today (with a real service-account API key
> that has `agent:*` scope):
>
> - `POST /v1/sub-tokens { forUserEmail | forUserId }` mints a JWT
>   with `delegatedUserAuthId` set to that user. One-hour TTL.
> - `GET /v0/private-workers/pending-requests` returns 200 (empty in
>   our test, but the endpoint is reachable).
> - `GET /v0/private-workers` lists the pool workers running under
>   the team service account.
> - `cursor-agent worker start --pool --pool-name <name>` with the
>   team service-account key registers as a real pool worker visible
>   in `/v0/private-workers`.
> - `CreateUserWorkspace` with an ephemeral `cursor_auth_token` claims
>   a warm prebuild for the target Coder user, the agent restarts
>   with the sub-token in env, and the worker authenticates as the
>   delegated user (without `--pool`).
>
> What's still missing:
>
> - **A Cursor-side scaling signal for user-identity workers.** Pool
>   queues only fire on pool workers, and sub-token workers cannot
>   join a pool. So the router has no native "user X is waiting" hook
>   today. The viable workaround is an out-of-band trigger: a Slack
>   bot, GitHub action, or IDE button that calls the router directly
>   with a target email or Coder username.
> - **A Cursor user-lookup endpoint.** The router needs `cursorUserId
>   -> email` to map a queued request back to a Coder user. `GET
>   /v0/me` returns only the caller. For now the router accepts the
>   target identity directly, fed by the out-of-band trigger.
> - **GitHub integration must be connected on the Cursor team** to
>   POST `/v1/agents`. We hit `integration_not_connected` end to end
>   on the validation team. That is a one-time team-admin setup, not
>   a code path we can automate.
>
> The shipping recipe for today is
> [System identity](./system-identity.md) for Enterprise pools or
> [Personal Workers](./personal-workers.md) for Team-plan teams. The
> rest of this page describes the user-identity model that runs on the
> same template, behind the `Pool: user identity` preset. See
> [Open questions for Cursor](./plan.md#open-questions-for-cursor)
> for the per-gap status.

## What user identity gives you

Compared to [System identity](./system-identity.md), user identity
restores the per-developer audit trail across the whole stack:

| Concern                                 | System identity                                                                              | User identity                                                                                                   |
|-----------------------------------------|----------------------------------------------------------------------------------------------|-----------------------------------------------------------------------------------------------------------------|
| Coder workspace owner                   | A bot service account                                                                        | The Coder user who matches the Cursor session creator                                                           |
| Git push                                | Blocked (`remote.origin.pushurl = no_push`)                                                  | Enabled via the user's own credential through Coder external auth                                               |
| Git author on commits                   | The bot                                                                                      | The user                                                                                                        |
| Coder audit log                         | Attributes to the bot service account                                                        | Attributes to the user, with the routing service account shown as on-behalf-of creator                          |
| Routing                                 | Label-based: Cursor picks any free worker whose `repo=` label matches the request            | The worker is born authenticated as the user before sessions arrive                                             |
| Pool size / concurrency                 | Fixed per repo: at most `instances` concurrent sessions per repo, every workspace is the bot | Dynamic: one workspace per session, spawned on demand; prebuilds become a warm cache that hides cold-start time |
| Failure if the user is missing in Coder | Not possible to detect: the workspace runs as the bot regardless                             | Pre-flight rejects with a friendly error so onboarding can complete first                                       |

The single biggest practical win is that **per-developer git push,
external-auth refresh, and Coder audit log all just work the way the
rest of Coder works**. You stop having to special-case Cursor sessions
in your audit and policy story.

> [!TIP]
> If you stay on a bot identity for commits and pushes, the routing
> layer is still useful on its own. Pointing it at a single bot Coder
> user instead of the matching human gives you:
>
> - **Dynamic concurrency.** A workspace per concurrent session,
>   spawned on demand. Prebuilds become a warm cache for cold-start
>   latency, not the inventory itself.
> - **Pre-flight validation.** Sessions for unknown Cursor users
>   reject up front instead of silently running as the bot.
> - **Per-worker audit context.** The routing service account shows
>   up as the on-behalf-of creator in the Coder audit log, so you can
>   still tie a workspace build back to a specific Cursor session,
>   even if the workspace owner is the bot.
>
> You opt into per-human attribution separately by pointing the
> router at the matching Coder user rather than the bot. The two
> decisions (dynamic spawn vs. fixed pool, and human owner vs. bot
> owner) are independent.

## What stays the same

User identity is built on top of the System identity recipe. You
keep:

- The same Coder template and image.
- The same prebuilt-workspace pool per repo as inventory.
- The same metadata blocks.
- The same Cursor pool configuration.

So the System identity rollout you ship today is the foundation. When
user identity ships, you turn it on by adding a thin routing
component between Cursor and Coder and switching the worker's
authentication from the shared team service-account key to per-user
sub-tokens.

## The flow, once it ships

1. A Cursor user creates a Background Agent job. The session queues on
   Cursor's side as a pending request with `userId`, `repoUrl`, and a
   request id.
2. The router learns about the request by polling
   `GET /v0/private-workers/pending-requests`. There is no queue
   webhook from Cursor yet; the `statusChange` webhook fires after a
   run is already running, which is too late to influence routing.
3. The router mints a per-user worker token:
   `POST /v1/sub-tokens { forUserId }` returns an hour-scoped access
   token for that one Cursor user. **This is the gate.** The team
   service-account key calling this endpoint must have `agent:*`
   scope; today's keys do not.
4. The router resolves `userId` to a Coder user: Cursor user id ->
   email (via a Cursor user-lookup endpoint that does not yet exist)
   -> Coder username (via Coder's users API). If the Coder user is
   missing, the router rejects the request before any workspace is
   created.
5. The router claims a warm prebuild from the matching repo's pool on
   the user's behalf by calling Coder's `CreateUserWorkspace` API
   with `template_version_preset_id` set to the `Pool: user identity`
   preset and the user's sub-token passed in `rich_parameter_values`
   as the `cursor_auth_token` ephemeral parameter. The workspace's
   ownership flips atomically from the prebuilds service account to
   the user. Coder external auth then resolves to the user's git
   credentials inside the workspace.
6. The agent inside the workspace re-runs its startup script under
   the new owner's environment, reads the `cursor_auth_token`
   ephemeral parameter, and starts `cursor-agent worker start
   --auth-token-file <path>` **without** `--pool`. Cursor rejects
   `--pool` for sub-token workers, so the worker registers as a
   multi-use personal worker authenticated as the delegated user. The
   pool name still rides in as a custom `--label "pool=<name>"` so
   the same tooling can identify it.
7. The user targets the worker by name (`machine=` or `worker=`)
   from the Cursor UI, Slack bot, GitHub action, or IDE button that
   spawned it. Cursor's pool scheduler is not involved.

The result: end-to-end attribution. Cursor's fleet shows the worker
running as the user, Coder's audit log attributes the workspace to
the user, and any commits the session produces are authored and
pushed by the user.

### Three presets, one template

A single Terraform template ships three presets. The same image, the
same metadata blocks; only the startup behavior diverges per preset.

| Preset                  | What it builds                                                                                                                                                                                                                                                                                                               | Prebuilds   |
|-------------------------|------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|-------------|
| `Pool: system identity` | Warm pool registered against the team service-account key with `cursor-agent worker start --pool --pool-name <name>`. Workers are visible to Cursor as live capacity for the team's pool. This is the [Worker Pool](./system-identity.md) recipe.                                                                            | N per repo  |
| `Pool: user identity`   | Warm prebuild that **does not register a worker** until claimed. On claim, the agent reads the ephemeral `cursor_auth_token`, then starts `cursor-agent worker start --auth-token-file ...` (no `--pool`). The worker registers as a multi-use personal worker authenticated as the delegated user. Targeted by name.        | N per repo  |
| `Router`                | Singleton workspace that mints sub-tokens and claims `Pool: user identity` prebuilds via `CreateUserWorkspace`. Polls `/v0/private-workers/pending-requests` for observability today; the actual user-identity trigger is expected to be an out-of-band signal (Slack bot, GitHub action, IDE button) until Cursor adds one. | 1 singleton |

The router is itself a Coder workspace with `prebuilds { instances = 1
}`, so the reconciler keeps it alive: delete or stop it and Coder
builds a fresh one. That gives you a free supervisor for the routing
service without a separate deployment or a sidecar process to operate.
The router only ever talks to Coder's admin API and Cursor's team
API, so the prebuilds service account that owns it is fine; it does
not need to impersonate users.

## Where this depends on Cursor

Three pieces of the Cursor API need to settle before this can ship:

- **`agent:*` scope on team service-account keys.** Issuance is
  pending. Without it, `POST /v1/sub-tokens` returns 403.
- **`POST /v1/sub-tokens` semantics.** What is the token's TTL? Does
  the worker need to refresh, or is the token long-lived enough to
  cover a session? Does revoking the parent team key revoke
  outstanding sub-tokens? These shape how the router caches and
  rotates.
- **A stable user-lookup endpoint.** The router needs `userId ->
  email` for users **other than the caller**. `GET /v1/me` returns
  the caller's profile; a `/v1/users/{id}` (or a `userEmail` field on
  the pending-request payload) is what the router needs.
- **A scaling signal.** Polling
  `GET /v0/private-workers/pending-requests` works but is latency-
  bound by the poll interval. A webhook on queue events would let
  the router respond in milliseconds; the wire format is not
  finalized.

Send your requirements on these to your Cursor account team. Each
ships independently; user identity needs all three.

## Where this depends on Coder

On the Coder side, the **routing component itself** is what's new.
Everything else is shipping today: preset-based prebuilds, ephemeral
rich parameters, on-claim agent reinitialization, and the
`CreateUserWorkspace` API that accepts a preset id plus parameter
values. The remaining work falls into two buckets:

- **Where the router runs.** Two architectures are viable:
  - **As a Coder workspace** (the shape this guide validates).
    A `Router` preset with `prebuilds { instances = 1 }` makes the
    reconciler responsible for keeping the routing service alive,
    using the same primitives that already manage worker pools.
    No separate deployment to operate, no sidecar to monitor; the
    routing process is just another workspace that happens to be a
    singleton. Lands the day Cursor turns on the `agent:*` key.
  - **Built into `coderd`.** Coder's server consumes the Cursor
    scaling signal directly. No router workspace at all; the
    integration becomes a config block in the template. Better
    long-term shape, but a larger product change.
- **How prebuilds carry user-scoped secrets.** Today, prebuilds run
  as the synthetic `prebuilds` service account, with Coder external
  auth and user secrets disabled. That works for `Pool: system
  identity` (one shared service-account key), but **`Pool: user
  identity` ships a workaround**: the per-user sub-token rides in on
  an ephemeral `rich_parameter_value` at claim time rather than
  through Coder's normal secret-injection paths. This works, but it
  means the operator stores the team API key and Coder admin token
  as template variables on the prebuilds-owned router workspace.
  Allowing prebuilds to optionally run as a configurable service
  account would let the router pull both from Coder's normal
  secret-management instead of as Terraform variables. Tracked in
  [coder/coder#25419](https://github.com/coder/coder/issues/25419).

We expect early adopters to start with the singleton-workspace router
and, once the integration shape settles, fold it into `coderd`.

## What to do today

If you need per-user attribution **today**, the closest thing System
identity offers is the `activeBcId` field in Cursor's fleet API. It
identifies the Cursor session that claimed the worker, and Cursor's
session log attributes the session to the human user. Your tooling
(audit dashboards, attribution reports) can join on `activeBcId` to
recover the per-human signal.

## Where to next

- [System identity](./system-identity.md): the recipe that ships
  today.
- [Implementation notes](./plan.md): the staged plan and the open
  questions for both Cursor and Coder that gate user identity.
