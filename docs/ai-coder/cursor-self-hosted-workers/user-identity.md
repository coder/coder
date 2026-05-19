# User identity: per-developer attribution

User identity lets Coder workspaces host Cursor private workers on
behalf of the **individual developer** who started the session,
not just a fleet-wide bot. The worker workspace becomes the
developer's, their git push credential is used, their commits are
authored by them, and Coder's audit log attributes worker activity to
them.

<img src="../../images/guides/cursor-self-hosted-workers/user-identity-flow.svg" alt="When a Cursor session is queued for a user, a routing component maps the Cursor user to their Coder user and claims a warm prebuild on their behalf. The workspace owner flips to the human, Coder external auth wires their git push token, and the worker registers in the Cursor pool under the team service account. Per-user attribution lives entirely on the Coder side. User identity is planned and not yet generally available." />

> [!IMPORTANT]
> The shape of User identity is **not** what the EAP-era docs
> described, and we have now validated the working shape end to end
> against a live Coder server and a real Cursor service-account key.
> Two findings drive the redesign:
>
> 1. **Cursor's CLI explicitly rejects `--pool` with a delegated
>    sub-token**, with
>    `Error: Delegated service-account tokens cannot start pool workers (--pool).`
>    Sub-tokens are scoped to the My Machines (personal-worker)
>    surface only. Pool workers must authenticate with the team
>    service-account key directly.
> 2. **`GET /v0/private-workers/pending-requests` is the only Cursor
>    scaling signal that fires before a worker is chosen.** It only
>    returns rows for pools that have queued requests, so a pool that
>    holds zero workers is what creates a routable queue.
>
> Together those two constraints define the design. Per-user
> attribution lives **entirely on the Coder side** (workspace
> owner, external auth, user secrets, audit log). The Cursor side
> stays on the team service account, because that is the only
> identity that can register as a pool worker. The `coder-owner`
> label on each worker keeps the human visible in Cursor's fleet view
> for audit, and Cursor's existing pool-scheduler dispatch is the
> trigger.
>
> What's confirmed working today (with a real service-account API key
> that has `agent:*` scope):
>
> - `GET /v0/private-workers/pending-requests` returns 200 and lists
>   queued sessions with `userId`, `repoUrl`, and `labels`.
> - `cursor-agent worker start --pool --pool-name pool-user-claim`
>   with the team SA key registers a real pool worker visible in
>   `/v0/private-workers`, and Cursor's pool scheduler dispatches
>   queued requests to it.
> - `CreateUserWorkspace` from a Coder admin token claims a warm
>   prebuild for the target user and atomically flips the owner from
>   `prebuilds` to that user. The agent re-runs startup under the new
>   owner's environment.
> - End-to-end claim flow: a queued `pool=pool-user-claim` request
>   triggers the router, the router maps `cursorUserId -> Coder
>   user`, claims a warm prebuild on that user's behalf, the new
>   workspace registers a pool worker, and Cursor dispatches the
>   queued request to it.
>
> What's still needed from Cursor before this is GA-grade:
>
> - **A user-lookup endpoint** (`GET /v1/users/{id}` or a `userEmail`
>   field on the `pending-requests` payload). Today the router maps
>   `cursorUserId -> Coder user` from an operator-maintained
>   `users.json`, populated from OIDC subject claims at first login.
> - **A queue-event webhook.** Polling
>   `GET /v0/private-workers/pending-requests` is rate-limited at
>   600 requests per hour, so the router's poll interval (~10s) sets
>   the cold-start floor. A webhook would drop that to
>   round-trip latency.
> - **GitHub integration enabled on the Cursor team** to use the
>   wider agent API surface (`POST /v1/agents`). Pool registration
>   works without it, so the validated recipe does not need it.

## What user identity gives you

Compared to [System identity](./system-identity.md), user identity
restores the per-developer audit trail on the Coder side and lights
up per-user external auth:

| Concern                                 | System identity                                                                              | User identity                                                                              |
|-----------------------------------------|----------------------------------------------------------------------------------------------|--------------------------------------------------------------------------------------------|
| Coder workspace owner                   | A bot service account                                                                        | The Coder user who matches the Cursor session creator                                      |
| Git push                                | Blocked (`remote.origin.pushurl = no_push`)                                                  | Enabled via the user's own credential through Coder external auth                          |
| Git author on commits                   | The bot                                                                                      | The user                                                                                   |
| Coder audit log                         | Attributes to the bot service account                                                        | Attributes to the user, with the router service account shown as the on-behalf-of creator  |
| Coder external auth / user secrets      | Disabled (synthetic prebuilds owner has no OAuth flow)                                       | Enabled after the claim flips the workspace to the human owner                             |
| Cursor worker identity                  | Team service account                                                                         | Team service account (Cursor-side constraint; pool registration requires the SA key)       |
| Cursor session log                      | Attributes to the human, joinable on `activeBcId`                                            | Attributes to the human, joinable on `activeBcId` and on the worker's `coder-owner` label  |
| Pool size / concurrency                 | Fixed per repo: at most `instances` concurrent sessions per repo, every workspace is the bot | Dynamic: one workspace per session, spawned on demand; warm prebuilds hide cold-start time |
| Failure if the user is missing in Coder | Not possible to detect: the workspace runs as the bot regardless                             | Pre-flight rejects with a friendly error so onboarding can complete first                  |

The single biggest practical win is that **per-developer git push,
external-auth refresh, Coder user secrets, and Coder audit log all
just work the way the rest of Coder works**. You stop having to
special-case Cursor sessions in your audit and policy story.

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
> - **Per-worker audit context.** The router service account shows
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
- The same prebuilt-workspace pool primitive.
- The same metadata blocks.
- The same Cursor pool configuration.
- The same team service-account key as the only Cursor credential.

So the System identity rollout you ship today is the foundation. User
identity turns on by adding two more presets to the same template:
`Pool: user identity` and `Router`.

## The flow

1. A Cursor user creates a Background Agent job and selects the
   `pool-user-claim` self-hosted pool. The session queues on
   Cursor's side as a pending request with `userId`, `repoUrl`, and
   a request id. Because the `pool-user-claim` pool holds zero
   workers (the prebuilds are warm but unregistered), the request
   stays queued until a worker registers.
2. The router learns about the queued request by polling
   `GET /v0/private-workers/pending-requests` every ten seconds.
   There is no queue webhook from Cursor yet; the `statusChange`
   webhook fires after a run is already running, which is too late
   to influence routing.
3. The router maps the queued request's `userId` to a Coder user.
   The mapping lives in an operator-maintained `users.json` on the
   router workspace, populated from OIDC subject claims at first
   login. A future `GET /v1/users/{id}` (or a `userEmail` field on
   the `pending-requests` payload) would let the router skip the
   mapping file.
4. The router claims a warm `Pool: user identity` prebuild on that
   user's behalf with Coder's `POST /api/v2/users/{user}/workspaces`
   API, passing the preset id. Coder claims a warm prebuild and
   atomically flips ownership from the `prebuilds` service account
   to the user. Coder external auth and user secrets resolve to the
   user's credentials in the new build.
5. The agent inside the workspace re-runs its startup script under
   the new owner's environment. Now that the workspace is owned by
   the human, the startup script runs `cursor-agent worker start
   --pool --pool-name pool-user-claim --label coder-owner=<email>`
   with the team service-account key. The worker registers in the
   `pool-user-claim` pool, and Cursor's pool scheduler dispatches
   the queued request to it on the next tick.
6. The session runs inside a workspace owned by the human, with the
   human's external-auth tokens resolvable for `git push`, the
   human's user secrets visible, and Coder's audit log attributing
   the build to the human (with the router service account as
   on-behalf-of creator).

The Cursor-side identity stays on the team service account, but the
`coder-owner=<email>` label on each pool worker keeps the human
visible in Cursor's fleet view and joinable to Cursor's session log
on `activeBcId`. Per-user attribution lives end to end on the Coder
side, where it carries the most weight (git push, secrets, audit).

### Three presets, one template

A single Terraform template ships three presets. The same image, the
same metadata blocks; only the startup behavior diverges per preset.

| Preset                  | What it builds                                                                                                                                                                                                                                                       | Prebuilds   |
|-------------------------|----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|-------------|
| `Pool: system identity` | Warm pool registered against the team service-account key with `cursor-agent worker start --pool --pool-name pool-system`. Workers are visible to Cursor as live capacity. This is the [Worker Pool](./system-identity.md) recipe.                                   | N per repo  |
| `Pool: user identity`   | Warm prebuild that **does not register a worker**. The `pool-user-claim` pool stays empty in Cursor's view, so user-targeted sessions queue. On claim, the workspace owner flips to the matching human and the agent registers a `pool-user-claim` worker.           | N per repo  |
| `Router`                | Singleton workspace that polls `pending-requests`, maps `cursorUserId` to a Coder user, and claims `Pool: user identity` prebuilds on their behalf via `POST /api/v2/users/{user}/workspaces`. No sub-tokens, no per-claim secrets; the router only flips ownership. | 1 singleton |

The router is itself a Coder workspace with `prebuilds { instances =
1 }`, so the reconciler keeps it alive: delete or stop it and Coder
builds a fresh one. That gives you a free supervisor for the routing
service without a separate deployment or a sidecar process to
operate. The router only ever talks to Coder's admin API and
Cursor's team API, so the prebuilds service account that owns it is
fine; it does not need to impersonate users.

## Where this depends on Cursor

The router operates today against the live API, but two refinements
would let it ship without operator-maintained glue:

- **A stable user-lookup endpoint.** The router needs `userId ->
  email` (or `userId -> ssoSubject`) for users **other than the
  caller**. `GET /v1/me` returns the caller's profile only; a
  `GET /v1/users/{id}` (or a `userEmail` field on the
  pending-request payload) is what the router needs. Until that
  ships, the router reads `users.json` on disk.
- **A scaling-signal webhook.** Polling
  `GET /v0/private-workers/pending-requests` works but is rate-
  limited at 600 requests per hour, so the router polls every ten
  seconds. A webhook on queue events would drop the user-visible
  cold start to the round-trip latency of one claim build.

Send your requirements on these to your Cursor account team. Each
ships independently; the recipe degrades gracefully without them.

## Where this depends on Coder

On the Coder side, the **routing component itself** is what's new.
Everything else ships today: preset-based prebuilds, on-claim agent
reinitialization, the workspace-owner flip on claim, and the
`CreateUserWorkspace` API that accepts a preset id. The remaining
work falls into two buckets:

- **Where the router runs.** Two architectures are viable:
  - **As a Coder workspace** (the shape this guide validates).
    A `Router` preset with `prebuilds { instances = 1 }` makes the
    reconciler responsible for keeping the routing service alive,
    using the same primitives that already manage worker pools. No
    separate deployment to operate, no sidecar to monitor; the
    routing process is just another workspace that happens to be a
    singleton.
  - **Built into `coderd`.** Coder's server consumes the Cursor
    scaling signal directly. No router workspace at all; the
    integration becomes a config block in the template. Better
    long-term shape, but a larger product change.
- **How the router carries its secrets.** Today the prebuilds
  service account that owns the router workspace cannot complete an
  OAuth flow, so Coder external auth resolves to nothing for it.
  The router pulls its Cursor team key and Coder admin token from
  Terraform variables on the template instead. That works, but it
  means the operator stores both secrets as template variables on a
  prebuilds-owned workspace. Allowing prebuilds to optionally run
  as a configurable service account would let the router pull both
  from Coder's normal secret-management instead of as Terraform
  variables. Tracked in
  [coder/coder#25419](https://github.com/coder/coder/issues/25419).

We expect early adopters to start with the singleton-workspace
router and, once the integration shape settles, fold it into
`coderd`.

## What to do today

The recipe is shippable today on Enterprise Coder with a Cursor
Enterprise team:

1. Stand up [System identity](./system-identity.md) for a baseline.
   That validates the template push, the prebuilds reconciler, and
   the Cursor SA key.
2. Add the `Pool: user identity` and `Router` presets from the
   validation template. The `pool-user-claim` pool will stay empty
   in Cursor's view until the router claims for a queued request.
3. Populate `users.json` on the router workspace with the
   `cursorUserId -> coderUsername` mapping for the developers you
   want to onboard. Most teams pull this from their SSO IDP
   directory at sync time.
4. Have a developer queue a Cursor session against the
   `pool-user-claim` pool. The router picks it up on the next poll
   tick, claims a warm prebuild, and the worker registers under
   the human's Coder workspace.

## Where to next

- [System identity](./system-identity.md): the baseline recipe.
- [Implementation notes](./plan.md): the staged plan and the open
  questions for both Cursor and Coder.
