# Autoscaling the Worker Pool

> [!NOTE]
> Concept. Validated against live Cursor and Coder APIs but not yet
> shipped as a recipe.

[Worker Pool](../system-identity.md) sizes itself with Coder prebuilds:
the template declares `instances = N` on the preset's `prebuilds`
block, and Coder keeps that many warm workspaces ready at all times.
That gets you a fixed baseline.

If your team's session load is bursty, the baseline can either be
oversized (idle workspaces costing money) or undersized (sessions
queue in Cursor while Coder is idle). The router pattern closes that
gap: an external process watches Cursor's fleet API for utilization
and queue depth, and tells Coder to create or delete workspaces on
top of the prebuild baseline.

## Signals

Cursor exposes two endpoints we care about:

| Endpoint | What it returns | When to use |
|---|---|---|
| `GET /v0/private-workers/summary` | `teamSummary.totalConnected`, `teamSummary.inUse` | Primary signal. Cheap, no documented rate limit. |
| `GET /v0/private-workers/pending-requests` | `requests[]` with `userId`, `repoOwner`, `repoName`, `labels`, `createdAtMs` | Queue depth. Rate-limited to 600/h per team; only poll when utilization is already high. |

Both are authenticated with the pool's service-account API key via
`Authorization: Bearer <key>` or HTTP basic auth.

Cursor's own [docs](https://cursor.com/docs/cloud-agent/self-hosted-pool.md#fleet-management-api)
recommend scaling up at `inUse / totalConnected >= 0.9`. Queue depth
sharpens that: even at 0.9 utilization, if the queue is empty you
don't need more workers; if the queue is non-empty you need at least
`queue_depth` more.

## Decision

A simple, useful loop:

```
active_load = inUse + queue_depth
desired     = clamp(max(MIN_WARM, active_load * (1 + BURST_BUFFER)),
                    MIN_WARM, MAX_WORKERS)
delta       = desired - totalConnected
```

- `MIN_WARM` is the floor that prebuilds already provide. Set the
  autoscaler's floor to the same number; the autoscaler is purely
  burst capacity on top.
- `BURST_BUFFER` (e.g. 0.5) keeps headroom so a new session lands on a
  free worker without queueing.
- `MAX_WORKERS` caps the bill. Cursor's hard limit is 50 per team.

When `delta > 0`, create that many workspaces. When `delta < 0` and
some workers are idle, delete the oldest idle autoscaler-owned
workspaces. Never delete prebuilds; let Coder's reconciler manage
those.

## Where workspaces come from

Two options, depending on how much logic you want in Coder versus the
autoscaler:

**Option 1: prebuilds for the floor, autoscaler for burst.**
Recommended. Coder prebuilds (`coder_workspace_preset.prebuilds.instances`)
give you `MIN_WARM` warm workspaces with the reconciler's lifecycle
guarantees. The autoscaler creates additional ad-hoc workspaces owned
by a system bot user (e.g. `cursor-pool-autoscaler`) when `delta > 0`.
Bot-owned workspaces stay outside the prebuild lifecycle; the
autoscaler deletes them directly when `delta < 0`.

```
POST /api/v2/users/cursor-pool-autoscaler/workspaces
{ "template_id": "<cursor-workers>", "name": "autoscale-<ts>-<i>" }

POST /api/v2/workspaces/<id>/builds { "transition": "delete" }
```

**Option 2: autoscaler-only, no prebuilds.**
Simpler, slower cold start. Set `instances = 0` in the preset and let
the autoscaler manage every workspace. Each new session waits for a
workspace to provision and the worker to register; cold start can be
30s to several minutes depending on your image size and provisioner.

The decision logic is the same. Option 1 is what we recommend; it
gives you Coder's prebuild correctness for the steady state and the
autoscaler only handles the part Coder can't (per-session burst).

## Reference autoscaler

A minimal Python reference autoscaler lives at
[`examples/autoscaler.py`](../examples/autoscaler.py). It reads both
Cursor endpoints, computes the decision above, and prints (or in
non-dry-run mode, issues) the Coder API calls needed to converge.

Run it once in dry-run mode against your team's live data to see what
it would do:

```bash
export CURSOR_SA_KEY=<your-team-sa-key>
export CODER_URL=https://coder.example.com
export CODER_SESSION_TOKEN=<admin-or-template-admin-pat>
export TEMPLATE_NAME=cursor-workers
export BOT_USERNAME=cursor-pool-autoscaler
export MIN_WARM=2
export MAX_WORKERS=20
export BURST_BUFFER=0.5
export DRY_RUN=1
python3 autoscaler.py
```

Expected output for an idle pool with 4 connected workers and
`MIN_WARM=2`:

```json
{"ts": 1779328132, "total": 4, "in_use": 0, "idle": 4,
 "queue": 0, "desired": 2, "delta": -2, "utilization": 0.0}
  scale down by 2: delete oldest idle bot-owned workspaces beyond MIN_WARM
```

Drop `DRY_RUN=1` and the script issues the writes. Plug the same loop
into a long-lived process, a Kubernetes Deployment, a Cloud Run
worker, or a cron job: the shape is the same.

## Operational notes

- **Don't autoscale prebuilds.** The prebuild reconciler keeps the
  floor; the autoscaler manages workspaces on top. If you scale both
  in the same loop you'll race the reconciler.
- **Pick the bot user's role carefully.** `cursor-pool-autoscaler`
  needs permission to create and delete workspaces from the worker
  template, and nothing else. Don't reuse an admin PAT.
- **Audit the autoscaler's actions.** Every workspace it creates and
  deletes lands in Coder's audit log under that bot user. That's how
  you tell autoscaler activity apart from human or prebuild activity.
- **Idle-release timeout matters.** Each `cursor-agent worker` in the
  template starts with `--idle-release-timeout` (8h default). The
  worker exits 0 after that idle window even before the autoscaler
  decides to scale down, so the autoscaler's scale-down policy
  mostly handles the long-tail recycle, not every idle worker.

## What this is not

- **Not per-user routing.** This autoscaler scales the *shared*
  `pool-system` pool. Every worker still runs under the team service
  account; sessions are matched by repo and pool labels, not user
  identity. For per-user identity see
  [Personal Workers](../personal-workers.md).
- **Not a queue scheduler.** Cursor's pool scheduler dispatches
  sessions to workers; the autoscaler only changes how many workers
  exist. If your team's load is hot enough to need session
  prioritization, talk to Cursor.

## Related

- [Worker Pool](../system-identity.md): the baseline pool the
  autoscaler scales.
- [Implementation notes](./implementation-notes.md): open questions
  and design history.
- [User identity on a shared pool](./user-identity.md): why the same
  router pattern can't be used to enforce per-user routing today.
