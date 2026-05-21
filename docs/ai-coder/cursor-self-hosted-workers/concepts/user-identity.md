# User identity: status

User identity means a Cursor session that lands on a self-hosted
worker runs as **the developer who triggered it**, with that
developer's git push credential, their Coder workspace owner, and
their identity in Cursor's session log. The end goal is per-user
attribution end to end with the warm, shared inventory shape of
[Worker Pool](../system-identity.md).

> [!IMPORTANT]
> **The shared-pool variant of user identity is not shippable today
> and the gap is on Cursor's side.** We validated this end to end
> against the live Cursor API with a real `agent:*`-scoped service-
> account key and a real personal API key. The recipe in this guide
> for **per-user identity that ships today** is
> [Personal Workers](../personal-workers.md).

## What we tried, and what blocks the pool shape

The natural shape, copying the pool inventory model from
[Worker Pool](../system-identity.md), would be:

1. A pool (`pool-user-claim`) holds warm Coder prebuilds that have
   **not** registered a Cursor worker.
2. A user submits a session against `pool-user-claim` from
   cursor.com.
3. The session queues, because the pool has zero registered workers.
4. A router service sees the queued request via
   `GET /v0/private-workers/pending-requests`, maps the requesting
   Cursor user to a Coder user, and claims a warm prebuild on their
   behalf via `POST /api/v2/users/{user}/workspaces`. Coder flips
   the workspace owner from the synthetic `prebuilds` user to the
   human, atomically.
5. The agent inside the claimed workspace registers a Cursor worker
   against `pool-user-claim`. Cursor's pool scheduler dispatches the
   queued request to it.

Three findings against live Cursor that prevent this from working:

- **`cursor-agent worker --pool` rejects delegated sub-tokens.** The
  CLI errors with
  `Delegated service-account tokens cannot start pool workers (--pool).`
  Sub-tokens are scoped to the
  [My Machines](https://cursor.com/docs/cloud-agent/my-machines.md)
  surface only. We worked around this by leaving the Cursor-side
  identity on the team service-account key and moving all per-user
  attribution to the Coder side. But:
- **Cursor's UI only lists pools that currently have a registered
  worker.** A pool with zero workers is invisible in the
  `Self-hosted Pools` dropdown, so a user cannot pick it to submit
  a session. The pool name does not persist after the last worker
  disconnects, and there is no `POST /v1/pools` to declare a pool
  without a worker.
- **Cursor's pool scheduler dispatches opportunistically.** The
  moment any worker is registered to a pool, the next incoming
  request is sent to that worker. A "decoy" worker that exists only
  to make the pool visible therefore catches the very session the
  router was supposed to claim a workspace for. The router never
  sees a queued request because there is no queue.

Confirmed live during validation: a decoy worker registered to
`pool-user-claim`, then a user-submitted session via cursor.com
landed on the decoy (`activeBcId` bound to the decoy worker, owned
by the `prebuilds` service account) instead of queueing.

The result: visibility in the UI and per-user routing are coupled,
and Cursor's scheduler has no per-user pool affinity to break the
coupling. The pool shape needs one of:

- A documented `POST /v1/pools` (or equivalent) that creates a pool
  in the team's UI without a registered worker.
- Per-user pool affinity on the scheduler (the worker declares
  `coder-owner=<email>`, the scheduler only dispatches matching
  users to it).
- A pre-dispatch webhook that fires when a session enters the
  pool's queue, before scheduling, so a router can claim a
  workspace and register a worker on the user's behalf.

Any one of these unblocks the design. None ship today. See
[Open questions for Cursor](./implementation-notes.md#open-questions-for-cursor)
for the gap detail.

## What ships today

| You want                                                  | Use                                                              |
|-----------------------------------------------------------|------------------------------------------------------------------|
| Per-user identity, simple setup                           | [Personal Workers](../personal-workers.md)                        |
| Fleet-wide bot identity, warm pool, autoscaled by Cursor  | [Worker Pool](../system-identity.md) (Cursor Enterprise required) |
| Per-user identity **and** warm shared-pool inventory      | Not shippable yet, see above.                                    |

[Personal Workers](../personal-workers.md) achieves the same per-user
attribution the pool design was aiming for:

- Workspace owner: the human.
- Git push, Coder external auth, user secrets: the human.
- Cursor worker identity: the human.
- Cursor session log: the human.

It runs **one Coder workspace per user instead of N shared warm
workers per pool**. That changes the capacity story (no shared warm
pool across users), but every other attribution property is the
same as the pool design would have been.

### Programmatic session submission

Personal Workers can be driven from a custom UI in addition to
cursor.com. The user creates their workspace in Coder once (one-time
setup, paste API key, hit go). After that, any UI can launch a
session against their worker with one HTTP call:

```http
POST https://api.cursor.com/v1/agents
Authorization: Bearer <user's personal Cursor API key>
Content-Type: application/json

{
  "env": { "type": "machine", "name": "coder-<user>-<workspace>" },
  "repos": [{ "url": "github.com/<org>/<repo>", "startingRef": "main" }],
  "prompt": { "text": "fix the flaky test" }
}
```

Cursor dispatches directly to the named personal worker. No pool,
no scheduler ambiguity, no router. This is what lets a Coder Tasks
page, a Slack bot, or an IDE button launch sessions on a
developer's personal worker without the user ever leaving Coder.

## Where to next

- [Personal Workers](../personal-workers.md): the recipe that ships
  today for per-user identity.
- [Worker Pool](../system-identity.md): the recipe that ships today
  for fleet-wide bot pools.
- [Implementation notes](./implementation-notes.md): the staged plan and the open
  questions for Cursor that gate the shared-pool user-identity
  shape.
