# Embedded NATS Pubsub: Research & Migration Plan

Status: Draft proposal
Branch: `feat/nats-pubsub` (based on `feat/app-level-pubsub`)
Audience: Coder engineering

## 1. Why

Coder uses Postgres `LISTEN/NOTIFY` (via `coderd/database/pubsub.Pubsub`) as its
in-deployment messaging fabric. At 100k workspaces this hits hard limits:

- **Single-connection bottleneck.** `pq.Listener` multiplexes every channel for
  a coderd replica onto one socket. Slow drains stall every channel.
- **8000-byte payload cap** at the Postgres protocol level.
  `coderd/database/pubsub/pubsub.go:617-621` already counts oversize messages
  via `colossalThreshold = 7600`; nothing rejects them today.
- **`NOTIFY` takes a global `AccessExclusiveLock` at COMMIT.** Mass agent
  reconnect storms serialize, which is exactly what motivated `feat/app-level-pubsub`
  (commit `25278a505`) to remove the tailnet pg_notify triggers.
- **Channel-cardinality scaling.** Per-owner / per-agent / per-job /
  per-workspace channel naming (`workspace_owner:{uuid}`, `agent-logs:{uuid}`,
  `provisioner-log-logs:{uuid}`, `prebuild_claimed_{uuid}`) means each replica
  may LISTEN on tens of thousands of channels. PG `LISTEN` cost is
  O(channels × backends).
- **No fanout.** Every replica's listener receives every message; we cannot
  scope subjects.
- **No replay, no drop signal nuance.** Reconnects mark *every* queue dropped
  (`pubsub.go:443-451`), forcing all subscribers to resync.

The branch `feat/app-level-pubsub` already removed the tailnet pg_notify
triggers and moved pubsub publishes into application code, decoupling
publication from COMMIT and unblocking a backend swap. That makes this the
right moment to introduce a non-Postgres backend.

## 2. Goal

Replace the Postgres `Pubsub` implementation with an **embedded NATS server
mesh** running inside `coderd`, with no third-party service to deploy.

- Single binary, single deployment story preserved.
- `coderd/database/pubsub.Pubsub` interface unchanged; backend is swappable.
- Scales to 100k workspaces and beyond by removing the PG `LISTEN/NOTIFY`
  bottleneck.
- Core NATS for like-for-like replacement of fire-and-forget topics; opt-in
  JetStream only for the few cases that need durability.

## 3. Current pubsub surface (summary)

Detailed per-channel inventory: see appendix A.

| Class                                                                 | Channels                                                                                                       | Volume @ 100k                                                  | Loss tolerance                                 |
|-----------------------------------------------------------------------|----------------------------------------------------------------------------------------------------------------|----------------------------------------------------------------|------------------------------------------------|
| Workspace lifecycle / stats                                           | `workspace_owner:{ownerID}`                                                                                    | ~3.3k pub/s steady state (StatsUpdate every 30s × 100k agents) | Tolerates loss; subscribers re-query DB        |
| Agent logs                                                            | `agent-logs:{agentID}`                                                                                         | Bursty per build                                               | Tolerates loss; 1 min fallback ticker          |
| Provisioner job logs                                                  | `provisioner-log-logs:{jobID}`                                                                                 | Bursty per build                                               | Tolerates loss; subscriber polls DB            |
| Provisioner job dispatch                                              | `provisioner_job_posted` (global)                                                                              | Per build                                                      | **Has explicit drop handler** (re-poll)        |
| Workspace builds (experimental)                                       | `workspace_updates:all` (global)                                                                               | Per build                                                      | Tolerates loss                                 |
| Agent metadata batch                                                  | `workspace_agent_metadata_batch` (global)                                                                      | 0.2/s, fanout to all replicas                                  | Tolerates loss                                 |
| Inbox notifications                                                   | `inbox_notification:owner:{ownerID}`                                                                           | Per delivered notification                                     | **Assumes reliable delivery**; large payloads  |
| Prebuild claim                                                        | `prebuild_claimed_{workspaceID}`                                                                               | At claim time                                                  | **Assumes reliable delivery**                  |
| Template watch                                                        | `template:{templateID}`                                                                                        | Per template push                                              | Tolerates loss                                 |
| Tailnet (4 channels)                                                  | `tailnet_peer_update`, `tailnet_tunnel_update`, `tailnet_ready_for_handshake`, `tailnet_coordinator_heartbeat` | Bursty during reconnect storms                                 | Tolerates loss; resync on `ErrDroppedMessages` |
| Replicasync, licenses, watchdog, latency-measure, chat (experimental) | various                                                                                                        | Low                                                            | Tolerates loss                                 |

Two call sites need extra care during the swap:

- **`inbox_notification:owner:{ownerID}`**, payload includes full notification
  body and can approach 8 KB; subscriber assumes reliable delivery.
- **`prebuild_claimed_{workspaceID}`**, agent reinit waits on this; drop =
  workspace stuck until request times out.

## 4. NATS embedding model

Confirmed: `nats-server` is a Go library
(`github.com/nats-io/nats-server/v2/server`). Embedding pattern:

```go
opts := &server.Options{
    ServerName: replicaID,
    DontListen: false,                 // true on single-replica deployments
    Cluster:    server.ClusterOpts{ /* Listen, Routes, TLS, Authorization */ },
}
ns, err := server.NewServer(opts)
go ns.Start()
ns.ReadyForConnections(10 * time.Second)

nc, _ := nats.Connect(ns.ClientURL(), nats.InProcessServer(ns))
```

`nats.InProcessServer(ns)` uses `net.Pipe` rather than TCP for the local
client connection. Apache 2.0 licensed.

### Topology decision: embed-in-every replica (full mesh)

|                          | (a) embed-in-every                | (b) leader-elected                     |
|--------------------------|-----------------------------------|----------------------------------------|
| SPOF                     | None                              | Leader is hot path                     |
| Failover behavior        | Local pubsub keeps working        | All clients reconnect on leader change |
| New subsystems needed    | None                              | Leader election + fencing              |
| Config                   | Seed routes from `replicas` table | Leader-election library                |
| Op model match for Coder | Strict superset of today          | New control plane                      |

**Decision: (a)**. Coder already runs N coderd replicas with a shared
`replicas` table that tracks peer addresses. Adding peer-to-peer cluster
routes is a strict superset of today's deployment topology. (b) reintroduces
exactly the leader-election complexity we don't have today.

Discovery: read peer addresses from the existing `replicas` table on startup
and pass them as `Cluster.Routes`. NATS gossip handles steady-state mesh
formation; the table is consulted only for cold start and long-partition
rejoin.

Practical caveats:

- Up to ~20 replicas the full mesh is comfortable. Beyond that, NATS supports
  leaf nodes / superclusters; out of scope for v1.
- v2.10+ defaults to `pool_size: 3` (3 route connections per peer pair); pin
  this in code so rolling upgrades don't fail with mismatched pool sizes.
- Cluster routes should connect replica-to-replica, not through the
  user-facing LB. Use `cluster_advertise` for NAT scenarios.

### Core NATS vs JetStream

**Default: core NATS.** All today's `LISTEN/NOTIFY` channels are best-effort
already; subscribers re-query Postgres on reconnect. Core NATS is a
like-for-like replacement.

JetStream R=3 is reserved for cases where Postgres was being abused as a
durable queue:

- **`provisioner_job_posted` (candidate, deferred).** Already has a
  drop-recovery path via DB poll, so v1 stays on core NATS. Future: migrate
  the provisionerd acquirer to a JetStream WorkQueue stream for true
  exactly-once dispatch. **Out of scope for v1.**
- **Audit fan-out (future).** Out of scope here; flagged for follow-up.

JetStream durability has had Jepsen findings as recent as 2.12.1; treating it
as opt-in per stream rather than blanket-on is the safer bet.

### Auth model

- **Client port**: `DontListen: true` on single-replica deployments, otherwise
  bound to localhost only. All in-deployment publishers/subscribers connect
  via `nats.InProcessServer(ns)`, no TCP, no auth needed.
- **Cluster routes**: TLS with cert verification + a shared cluster secret.
  Reuse Coder's existing deployment signing key material; no NATS operator /
  JWT framework needed for a single-tenant deployment.
- **No external clients**: agents and end users continue to talk to coderd
  over the existing API surface. NATS stays internal.

### Footprint

- Idle embedded server: ~5-15 MiB RSS, ~20-30 goroutines. JetStream-enabled
  empty: ~30-80 MiB.
- Per idle connection: ~20-30 KB.
- Core publish throughput: 14M+ msgs/sec on commodity hardware.
- For Coder's current scale, NATS-attributable memory is dwarfed by the PG
  pool and HTTP handlers.

### Risks / unknowns

1. **Slow consumers.** Core NATS *drops* messages on slow consumers (default
   10 MB pending). We must call `SetPendingLimits` per subscription and
   surface a metric/log so subscribers learn to resync. The existing
   `ErrDroppedMessages` contract maps to this.
2. **Message size.** Default 1 MB max payload (configurable up to 64 MB, NATS
   recommends ≤8 MB). All current callsites are well under this; the only
   risk is the inbox notification payload approaching 8 KB.
3. **Container memory.** NATS sees host memory by default; ensure
   `GOMEMLIMIT` is inherited from coderd's deployment.
4. **JetStream durability.** If/when we adopt JetStream, set `sync_interval:
   always` for the provisioner queue and follow Jepsen-driven version
   guidance.
5. **Pool size mismatch.** Pin `pool_size` in code.
6. **No public case studies of >50-node embedded NATS meshes.** Coder's
   target scale (single-digit to low-double-digit replicas) is well within
   precedent (k3s/KINE, Liftbridge, AnyCable-Go).

## 5. Migration plan

The plan is incremental, reversible, and feature-flagged. Each phase is
independently shippable.

### Phase 0, Branch & baseline (this commit)

- Branch `feat/nats-pubsub` cut from `feat/app-level-pubsub`.
- Land this design doc.
- Add a `BenchmarkPGPubsub` to `coderd/database/pubsub/` simulating the
  hot-path mix (workspace_owner StatsUpdate at 3.3k pub/s, provisioner-log
  bursts, agent-logs cardinality at 100k). This becomes the parity baseline
  for NATS.

**Deliverable**: this doc + benchmark.

### Phase 1, NATS-backed `pubsub.Pubsub` implementation

Add a third backend alongside `PGPubsub` and `MemoryPubsub`.

- New package `coderd/database/pubsub/natspubsub/`.
- Implements `pubsub.Pubsub` with one embedded `*server.Server` and one
  in-process `*nats.Conn` per coderd replica.
- Subject naming: `coder.v1.<channel>` where `<channel>` is the existing
  channel string (UUIDs and all). Wildcard semantics not used in v1.
- Backpressure: per-subscription `SetPendingLimits(2048, 8MB)` matching the
  existing `BufferSize = 2048`. Translate slow-consumer/disconnect events
  into `pubsub.ErrDroppedMessages` so existing resync paths fire unchanged.
- Implement watchdog and latency measurement against the new backend.
- Single-replica mode: `DontListen: true`, no cluster.
- Feature flag: `CODER_PUBSUB_BACKEND` deployment value
  (`postgres` | `nats`), default `postgres`.

**Deliverable**: PR adding the backend + unit tests + parity tests against
the existing PGPubsub test suite.

### Phase 2, Cluster mode + replica discovery

- On startup, read peer addresses from the `replicas` table and pass them as
  NATS `Cluster.Routes`.
- Generate per-replica TLS material from the existing deployment CA; require
  TLS + auth on the cluster listener.
- Bind the client port to localhost only; in-process clients use
  `nats.InProcessServer`.
- On replica heartbeat, refresh route list (NATS gossip handles steady state;
  this only matters for cold start and long partitions).
- Operational endpoints: expose NATS `/healthz`, `/varz`, `/connz`,
  `/routez`, `/jsz` through coderd's existing `/healthz` and Prometheus
  pipelines.

**Deliverable**: PR enabling multi-replica clustering behind the feature
flag.

### Phase 3, Per-callsite cutover (no code changes, config only)

Because the `pubsub.Pubsub` interface is unchanged, every callsite in
appendix A migrates implicitly when the deployment flag is flipped. We do,
however, audit each "assumes reliable delivery" callsite first:

- **`inbox_notification:owner:*`**, verify payload stays under NATS
  default 1 MB cap (it does: ≤8 KB historically). Add explicit size cap +
  log when approaching 1 MB.
- **`prebuild_claimed_{workspaceID}`**, add a server-side timeout +
  retry-on-publish path so a one-shot drop doesn't strand the agent reinit.
  This is a pre-existing weakness regardless of backend, but worth tightening
  during cutover.
- **`provisioner_job_posted`**, already handles drops; verify the NATS
  drop-signal mapping fires the same recovery.

**Deliverable**: small PRs per callsite where extra hardening is needed; no
backend changes.

### Phase 4, Default on for new deployments

After one release on the flag with no regressions:

- Flip default to `nats` for new deployments.
- Existing deployments continue on `postgres` until they opt in.
- Document migration steps + rollback.

### Phase 5 (deferred), JetStream-backed provisioner queue

Out of scope for v1. Tracking issue notes:

- Stream `coder.v1.provisioner.jobs` with WorkQueue retention, R=3, file
  storage, `sync_interval: always`.
- Replace the Postgres-driven `acquirer` with a JetStream pull consumer.
- Removes the "every replica acquirer races every job" model and gives true
  exactly-once dispatch.

### Phase 6 (deferred), Tailnet on NATS

`enterprise/tailnet/pgcoord.go` already uses `pubsub.Pubsub` exclusively
(post-`feat/app-level-pubsub`), so it migrates implicitly with Phase 1+2. No
code change needed beyond verifying:

- `ErrDroppedMessages` mapping fires `resyncPeerMappings` on slow-consumer
  events.
- Reconnect-induced "all queues dropped" semantics translate cleanly. NATS
  reconnect should *not* trigger a global drop unless we explicitly map it
  that way; revisit whether the existing PGPubsub behavior was intentional or
  a side effect.
- The 6 s heartbeat liveness floor (3 × 2 s) survives NATS reconnect-backoff
  defaults. If not, override `nats.ReconnectWait`.

## 6. Open questions

1. **Reconnect drop semantics.** PGPubsub marks every queue dropped on
   reconnect; NATS does not. Which behavior do we want as the contract?
   Probably "only signal drop when slow-consumer threshold hit", which is a
   net improvement.
2. **Discovery source.** `replicas` table vs DNS SRV (k8s-friendly). Start
   with `replicas` since it already exists; revisit if k8s operators want SRV.
3. **JetStream timeline.** Do we ship Phase 5 in the same release as Phase 4,
   or stagger?
4. **Observability parity.** What NATS-specific metrics do we surface in the
   existing health/Prometheus dashboards?
5. **Single-binary embedding for `coder server` + `coder provisionerd`**. The
   provisionerd binary today connects to coderd over HTTP; it does not need
   NATS access. Confirm.

## Appendix A, Call site inventory

(See child reports for full file:line citations. Summarized here.)

### `coderd/database/pubsub/pubsub.go` interface

- `Listener`, `ListenerWithErr`, `Subscriber`, `Publisher`, `Pubsub`.
- `BufferSize = 2048` per subscriber; overflow replaces last entry with
  `ErrDroppedMessages`.
- `colossalThreshold = 7600` is metric-only; oversize publishes fail at PG
  layer.
- Reconnect (nil notification) marks every queue dropped.

### Per-channel inventory

(Full table in §3 above; details and file:line in the explore reports
attached to this branch's task history.)

| Channel                                                                                   | Helper                                                     | Publishers                                                                                                                                                                                              | Subscribers                                                                                    |
|-------------------------------------------------------------------------------------------|------------------------------------------------------------|---------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|------------------------------------------------------------------------------------------------|
| `workspace_owner:{ownerID}`                                                               | `coderd/wspubsub/wspubsub.go:53`                           | `coderd/workspaces.go:2945-2964` (funnel), called from workspaces.go, workspacebuilds.go, workspaceagents.go, workspaceagentsrpc.go, workspacestats/reporter.go, provisionerdserver.go, agentapi/api.go | `coderd/workspaces.go:2124`, `coderd/workspaceagents.go:526`, `coderd/workspaceupdates.go:109` |
| `workspace_updates:all`                                                                   | `coderd/wspubsub/wspubsub.go:18`                           | `coderd/workspacebuilds.go:771`                                                                                                                                                                         | `coderd/workspaces.go:2196`                                                                    |
| `agent-logs:{agentID}`                                                                    | `codersdk/agentsdk/agentsdk.go:741`                        | `coderd/workspaces.go:2966-2975` (via agentapi)                                                                                                                                                         | `coderd/workspaceagents.go:548`                                                                |
| `provisioner-log-logs:{jobID}`                                                            | `provisionersdk/logs.go:11-20`                             | `coderd/provisionerdserver/provisionerdserver.go:1107,1397,1689`, `coderd/jobreaper/detector.go:396`                                                                                                    | `coderd/provisionerjobs.go:538`                                                                |
| `provisioner_job_posted`                                                                  | `coderd/database/provisionerjobs/provisionerjobs.go:13-32` | `provisionerjobs.PostJob` callers                                                                                                                                                                       | `coderd/provisionerdserver/acquirer.go:301`                                                    |
| `workspace_agent_metadata_batch`                                                          | `coderd/agentapi/metadatabatcher/metadata_batcher.go:46`   | `metadata_batcher.go:376` (5s flush)                                                                                                                                                                    | `coderd/workspaceagents.go:1711`                                                               |
| `inbox_notification:owner:{ownerID}`                                                      | `coderd/pubsub/inboxnotification.go:14`                    | `coderd/notifications/dispatch/inbox.go:97`                                                                                                                                                             | `coderd/inboxnotifications.go:152`                                                             |
| `prebuild_claimed_{workspaceID}`                                                          | `codersdk/agentsdk/agentsdk.go:761`                        | `coderd/prebuilds/claim.go:30`                                                                                                                                                                          | `coderd/prebuilds/claim.go:61`                                                                 |
| `template:{templateID}`                                                                   | `coderd/templateversions.go:2011`                          | `coderd/templateversions.go:2016`                                                                                                                                                                       | `coderd/workspaces.go:2148`                                                                    |
| `tailnet_peer_update`                                                                     | `enterprise/tailnet/pgcoord.go:31-34`                      | `binder` after `UpsertTailnetPeer` (pgcoord.go:631) + startup + cleanup                                                                                                                                 | `querier.listenPeer` (pgcoord.go:1207, 1281)                                                   |
| `tailnet_tunnel_update`                                                                   | same                                                       | `tunneler` after each tunnel write (pgcoord.go:417-475)                                                                                                                                                 | `querier.listenTunnel` (pgcoord.go:1230, 1308)                                                 |
| `tailnet_ready_for_handshake`                                                             | same                                                       | `handshaker.worker` (handshaker.go:65)                                                                                                                                                                  | `querier.listenReadyForHandshake` (pgcoord.go:1252, 1339)                                      |
| `tailnet_coordinator_heartbeat`                                                           | same                                                       | `heartbeats.sendBeat` every 2 s                                                                                                                                                                         | `heartbeats.listen` (pgcoord.go:1728)                                                          |
| `replica`                                                                                 | `enterprise/replicasync/replicasync.go:29`                 | `replicasync.go:84,142,207,488`, `enterprise/coderd/workspaceproxy.go:847`                                                                                                                              | replicasync subscribers                                                                        |
| `PubsubEventLicenses`                                                                     | `enterprise/coderd/licenses.go`                            | `licenses.go:139,220,334`, `coderd.go:1313`                                                                                                                                                             | `enterprise/coderd/coderd.go:1323`                                                             |
| `pubsub_watchdog`                                                                         | `coderd/database/pubsub/watchdog.go:14`                    | every 15s per replica                                                                                                                                                                                   | watchdog subscribers                                                                           |
| `latency-measure:{uuid}`                                                                  | `coderd/database/pubsub/latency.go`                        | per Prometheus scrape                                                                                                                                                                                   | per scrape                                                                                     |
| Chat (experimental) `chat:stream:*`, `chat:owner:*`, `chat:config_change`, `chat_debug:*` | `coderd/pubsub/*`                                          | `coderd/x/chatd/*`, `coderd/exp_chats.go`                                                                                                                                                               | same                                                                                           |
