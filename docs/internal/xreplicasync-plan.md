# xreplicasync PeerProvider plan

## Goals

- Design a new experimental enterprise package,
  `enterprise/coderd/x/xreplicasync`, that implements
  `coderd/x/nats.PeerProvider` using replica data maintained by the existing
  `enterprise/replicasync` manager.
- Let multi-replica enterprise deployments discover embedded NATS route peers
  from the `replicas` table instead of hand-configuring
  `nats.StaticPeerProvider`.
- Make the address gap explicit: `replicas.relay_address` is an HTTP(S) DERP
  relay URL, while `nats.Peer.RouteURL` must be a NATS route URL such as
  `nats://host:6222` or `tls://host:6222`.
- Recommend a v1 startup-snapshot provider, with a clear future path for
  dynamic route refresh.
- Keep the package under `enterprise/coderd/x/` because both x/nats and this
  adapter are experimental, and because `replicasync` is the enterprise HA
  replica discovery mechanism.

## Non-goals

- Do not wire embedded NATS into production coderd in this change. Current
  production startup still creates the Postgres-backed pubsub in
  `cli/server.go:766-778`, and `coderd/x/nats` documents that it is not wired
  into coderd yet at `coderd/x/nats/doc.go:1-12`.
- Do not change `coderd/x/nats` as part of the initial provider package unless
  the dynamic-refresh option is selected.
- Do not use `Replica.RelayAddress` directly as a NATS route URL. It is an
  HTTP URL consumed by DERP mesh and latency checks, not a route listener URL.
- Do not include workspace proxy replicas as NATS peers. NATS clustering should
  target primary coderd replicas only.

## Background

### Existing NATS PeerProvider contract

- `coderd/x/nats.Peer` has optional `Name` and a `RouteURL` string for the
  route endpoint, with examples `nats://10.0.0.12:6222` and
  `tls://nats-1.internal:6222` at `coderd/x/nats/cluster.go:17-25`.
- `PeerProvider` is `Peers(context.Context) ([]Peer, error)`, and its comment
  says v1 consults it once during `New` at `coderd/x/nats/cluster.go:27-31`.
- `nats.New` calls `opts.PeerProvider.Peers(ctx)` only when a provider is set,
  wraps provider errors as `nats peer discovery`, normalizes the snapshot, and
  passes it to `startEmbeddedServer` at `coderd/x/nats/pubsub.go:72-88`.
- `normalizePeers` trims and parses `RouteURL`, rejects empty values, and only
  accepts `nats` or `tls` schemes at `coderd/x/nats/cluster.go:41-65`.
- If there are no peers, x/nats starts in standalone mode by setting
  `DontListen = true` and returning before cluster route options are populated
  at `coderd/x/nats/server.go:33-58`.
- Cluster mode requires `Options.ClusterToken`, otherwise server construction
  returns `ClusterToken is required when peers are configured` at
  `coderd/x/nats/server.go:60-62`.
- x/nats route listener options already exist as `ClusterHost`, `ClusterPort`,
  and `ClusterAdvertise` at `coderd/x/nats/options.go:54-65`. Route TLS is
  `ClusterTLSConfig` at `coderd/x/nats/options.go:50-52`.

### Existing replicasync behavior

- The requested `enterprise/coderd/replicasync/replicasync.go` path does not
  exist in this worktree. The package is `enterprise/replicasync`, with coderd
  integration under `enterprise/coderd`.
- `replicasync.Options` contains `ID`, `UpdateInterval`, `PeerTimeout`,
  `RelayAddress`, `RegionID`, and `TLSConfig` at
  `enterprise/replicasync/replicasync.go:31-39`.
- `replicasync.New` defaults a nil ID to `uuid.New()`, inserts the local
  replica row, publishes the replica update event, syncs peers, subscribes to
  updates, and starts background sync loops at
  `enterprise/replicasync/replicasync.go:41-109`.
- `Manager` stores the local `self database.Replica`, a mutex-protected
  `peers []database.Replica`, and a single callback at
  `enterprise/replicasync/replicasync.go:112-129`.
- `Manager.ID()` returns the local replica ID at
  `enterprise/replicasync/replicasync.go:131-133`, and `Self()` returns the
  local database row at `enterprise/replicasync/replicasync.go:394-399`.
- `syncReplicas` computes the active threshold as `now - 3*UpdateInterval` at
  `enterprise/replicasync/replicasync.go:145-150`, then reads replicas updated
  after that threshold at `enterprise/replicasync/replicasync.go:246-252`.
- The backing SQL for active replicas is
  `SELECT * FROM replicas WHERE updated_at > $1 AND stopped_at IS NULL` at
  `coderd/database/queries/replicas.sql:1-2`.
- `syncReplicas` excludes self and replicas with empty `RelayAddress`, then
  stores the remaining peers at `enterprise/replicasync/replicasync.go:254-269`.
- `AllPrimary()` returns primary replicas from peers plus self at
  `enterprise/replicasync/replicasync.go:401-415`; `Regional()` delegates to
  `InRegion(m.regionID())` at `enterprise/replicasync/replicasync.go:431-439`.
- `InRegion(regionID)` returns same-region peers but does not filter by
  `Primary` at `enterprise/replicasync/replicasync.go:417-429`, so it can
  include workspace proxy replicas.
- `SetCallback` stores one callback and immediately invokes it asynchronously
  at `enterprise/replicasync/replicasync.go:442-450`; `syncReplicas` invokes
  the callback after updating local state at
  `enterprise/replicasync/replicasync.go:354-365`.

### Replica fields and address conventions

- `database.Replica` fields are `ID`, timestamps, `Hostname`, `RegionID`,
  `RelayAddress`, `DatabaseLatency`, `Version`, `Error`, and `Primary` at
  `coderd/database/models.go:5037-5050`.
- The `replicas` migration describes `relay_address` as an address accessible
  to other replicas and `region_id` as DERP region state at
  `coderd/database/migrations/000061_replicas.up.sql:15-19`.
- Primary coderd replicas are inserted with `Primary: true` at
  `enterprise/replicasync/replicasync.go:62-80`.
- Workspace proxy registration also writes to `replicas`, but uses
  `Primary: false` at `enterprise/coderd/workspaceproxy.go:644-687`.
- Deployment config describes DERP server relay URL as an HTTP URL accessible
  by other replicas and required for HA at `codersdk/deployment.go:1918-1928`.
- `PingPeerReplica` treats `RelayAddress` as an HTTP URL base and probes
  `/derp/latency-check` at `enterprise/replicasync/replicasync.go:368-391`.
- Therefore, the existing replicas table does not store a NATS route port or a
  complete NATS route URL.

### Current enterprise wiring constraints

- Root server construction creates the existing pubsub before enterprise
  `coderd.New` runs, at `cli/server.go:766-778` and `cli/server.go:992-997`.
- Enterprise constructs `replicasync.Manager` later, after `api.AGPL =
  coderd.New(options.Options)`, at `enterprise/coderd/coderd.go:211-224` and
  `enterprise/coderd/coderd.go:654-667`.
- That ordering means a provider requiring an already-constructed manager cannot
  simply be passed into `nats.New` at the current production pubsub construction
  point without additional startup-order work.

## Design

### Provider shape

Create package `enterprise/coderd/x/xreplicasync` with a small adapter that
uses an interface over the manager rather than hard-coding the concrete type in
all tests:

```go
package xreplicasync

type ReplicaSource interface {
	ID() uuid.UUID
	AllPrimary() []database.Replica
	UpdateNow(context.Context) error
}

type RouteURLFunc func(database.Replica) (string, error)

type Options struct {
	Manager ReplicaSource
	RouteURL RouteURLFunc
	WaitForInitialSync bool
	AllowStandalone bool
}
```

The concrete constructor should validate required options up front:

- `Manager` is required.
- `RouteURL` is required for v1 unless the team first adds a NATS route address
  to `database.Replica`.
- Option names can be adjusted during implementation, but the key idea is to
  make the route-address source explicit.

`Provider.Peers(ctx)` should:

1. Optionally call `Manager.UpdateNow(ctx)` to avoid returning a stale initial
   snapshot. `UpdateNow` delegates to synchronous `syncReplicas` at
   `enterprise/replicasync/replicasync.go:135-138`.
2. Read `Manager.AllPrimary()` so only primary coderd replicas are candidates.
   This avoids `Regional()` and `InRegion(...)` because those do not filter out
   workspace proxies at `enterprise/replicasync/replicasync.go:417-429`.
3. Exclude self by comparing each `database.Replica.ID` to `Manager.ID()`.
   Existing `AllPrimary()` includes self at
   `enterprise/replicasync/replicasync.go:401-415`.
4. For each remaining replica, call `RouteURL(replica)` and return
   `nats.Peer{Name: replica.Hostname, RouteURL: routeURL}`. If hostname is
   empty, use `replica.ID.String()` as a stable name.
5. Reject an empty derived route URL as provider error rather than silently
   dropping a primary replica, unless `AllowStandalone` is explicitly true and
   there are no candidates.

### Route URL derivation

Do not derive the NATS route URL from `Replica.RelayAddress` by changing only
its scheme. That would incorrectly reuse the Coder HTTP or DERP relay port,
while x/nats route listeners are configured with `ClusterHost`, `ClusterPort`,
and `ClusterAdvertise` at `coderd/x/nats/options.go:54-65`.

Recommended v1 design:

- Require an explicit `RouteURLFunc` in `xreplicasync.Options`.
- Provide helper constructors for common conventions only if they are honest
  about their assumptions, for example:
  - `RouteURLFromAdvertiseMap(map[uuid.UUID]string)` for tests or controlled
    deployments.
  - `RouteURLFromReplicaHostname(scheme string, port int)` for deployments
    where `Replica.Hostname` is routable and every replica uses the same NATS
    route port.
- Validate generated URLs by relying on x/nats normalization when `nats.New` is
  called, and also add unit tests in xreplicasync for obvious invalid callback
  returns.

Better long-term design:

- Add a dedicated advertised NATS route address to the replica registration
  data, probably as a new column or adjacent HA discovery record. This requires
  database migration, sqlc generation, audit review if the type becomes
  auditable, and deployment/config work for route advertise addresses.
- Once that address exists, `xreplicasync` can map a replica directly to
  `nats.Peer{RouteURL: replica.NATSRouteURL}` without callback conventions.

### Static vs dynamic peers

Recommend v1: startup snapshot only.

Reasons:

- x/nats explicitly documents one-shot provider reads at
  `coderd/x/nats/cluster.go:27-31` and `coderd/x/nats/doc.go:30-40`.
- `nats.New` currently bakes the provider snapshot into server options before
  startup at `coderd/x/nats/pubsub.go:72-88` and
  `coderd/x/nats/server.go:92-108`.
- A startup-snapshot adapter is small, testable, and useful for initial
  experiments if operators understand that scale-up or route-address changes
  require restarting replicas.
- Existing NATS route behavior retries explicit routes, so known but not-yet-up
  peers can still connect after they start. The missing piece is discovery of
  peers that were unknown when this replica started.

Document v1 operational semantics clearly:

- A replica discovers peers known to `replicasync` at `nats.New` time.
- New replicas added later are not added to existing NATS server route lists
  until those existing replicas restart.
- Removing stale replicas may leave obsolete explicit routes until restart, but
  route failures should not break local pubsub operation.

Dynamic option for v2:

- Add an x/nats route refresh API that accepts a new peer list or re-calls the
  provider, normalizes peers, converts them to authenticated route URLs, updates
  `natsserver.Options.Routes`, and calls `Server.ReloadOptions`.
- Upstream nats-server v2.12.8 exposes `Server.ReloadOptions(newOpts *Options)
  error`, and route URLs are reloadable through route diffing in its
  `server/reload.go`.
- Wire `replicasync.Manager.SetCallback` to call the x/nats refresh API, since
  the manager already invokes callbacks on updates at
  `enterprise/replicasync/replicasync.go:442-450` and
  `enterprise/replicasync/replicasync.go:354-365`.
- This is a medium-sized change because x/nats must preserve and clone server
  options safely, test reload behavior, and define concurrency/error handling.

Effort estimate:

- Startup-snapshot provider: small, roughly one package plus unit tests.
- Dynamic refresh: medium to large, requiring x/nats API changes, route reload
  tests, callback wiring, and operational decisions for failed reloads.

### Error semantics

Recommended defaults:

- Constructor returns errors for nil manager or nil route URL function.
- `Peers(ctx)` returns an error if `UpdateNow(ctx)` fails, because a stale or
  empty snapshot can silently degrade enterprise HA into standalone mode.
- If `AllPrimary()` returns only self after a successful sync, return an empty
  peer slice. This is valid for a one-replica deployment and matches x/nats
  standalone semantics at `coderd/x/nats/doc.go:23-34`.
- If there are other primary replicas but any cannot be mapped to a NATS route
  URL, return an error. Silent dropping would create a partial cluster that is
  harder to diagnose.
- Add `AllowStandalone` or similar only for explicit dev/test scenarios where
  empty peers should not fail startup even if the deployment expected HA.

Potential enhancement:

- Add `MinPeers` or `RequirePeers` to distinguish intentional single-replica
  startup from an enterprise multi-replica deployment whose `replicasync`
  snapshot has not populated yet. This likely belongs near deployment wiring,
  where the server knows whether HA NATS is required.

### Self-exclusion

Use `Manager.ID()` as the source of truth. In enterprise coderd, the manager is
constructed with `api.AGPL.ID` at `enterprise/coderd/coderd.go:656-658`, and
`replicasync.New` stores that ID in the manager and local replica row at
`enterprise/replicasync/replicasync.go:88-98`. Because `AllPrimary()` includes
self, the provider must remove it before returning peers.

### Example future wiring

This is illustrative only. It should not be implemented in the provider PR.

```go
provider, err := xreplicasync.New(xreplicasync.Options{
	Manager: api.replicaManager,
	RouteURL: xreplicasync.RouteURLFromReplicaHostname("tls", natsRoutePort),
	WaitForInitialSync: true,
})
if err != nil {
	return err
}

ps, err := nats.New(ctx, logger.Named("nats"), nats.Options{
	PeerProvider: provider,
	ClusterToken: clusterToken,
	ClusterTLSConfig: routeTLSConfig,
	ClusterHost: clusterHost,
	ClusterPort: clusterPort,
	ClusterAdvertise: clusterAdvertise,
})
```

This example does not fit the current production startup order without a pubsub
factory or earlier manager construction, because production pubsub is created in
`cli/server.go:766-778` and `replicasync.Manager` is created later in
`enterprise/coderd/coderd.go:654-667`.

## Package layout

Proposed files under `enterprise/coderd/x/xreplicasync`:

- `provider.go`
  - `type Provider struct`.
  - `type Options struct`.
  - `type ReplicaSource interface`.
  - `type RouteURLFunc func(database.Replica) (string, error)`.
  - `func New(opts Options) (*Provider, error)`.
  - `func (p *Provider) Peers(ctx context.Context) ([]nats.Peer, error)`.
- `routeurl.go`
  - Optional helpers such as `RouteURLFromReplicaHostname(scheme string, port
    int)` and test-oriented map helpers.
  - Keep helpers conservative. They should validate scheme is `nats` or `tls`,
    port is non-zero when required, and hostname is non-empty.
- `provider_test.go`
  - Unit tests with a fake `ReplicaSource`.
  - Tests for self-exclusion, primary-only source use, route mapping errors,
    empty peer behavior, and `UpdateNow` error propagation.
- `routeurl_test.go`
  - Tests for helper URL generation and invalid schemes or ports.
- Optional `doc.go`
  - Package comment explaining experimental status and the startup-snapshot
    limitation.

## Open questions

1. Where should each replica's NATS route address come from?
   - Option A: v1 callback mapping from existing replica fields and deployment
     convention.
   - Option B: add a dedicated advertised NATS route URL to replica
     registration data.
   - Recommendation: use Option A for experimental v1, but do not treat it as a
     permanent HA discovery contract.
2. Should v1 require dynamic peer refresh?
   - Option A: startup snapshot, scale-up requires restart.
   - Option B: extend x/nats with `ReloadOptions`-based route refresh.
   - Recommendation: Option A for v1. Option B is feasible but should be a
     separate x/nats design and test effort.
3. Should NATS peers be all primary replicas or region-local primary replicas?
   - All primary replicas maximizes cluster connectivity across regions.
   - Region-local peers mimic DERP mesh locality but may partition pubsub if no
     inter-region route path exists.
   - Recommendation: all primary replicas for pubsub correctness unless product
     explicitly wants region-scoped pubsub clusters.
4. How should enterprise startup know whether zero discovered peers is valid?
   - A single-replica deployment should be allowed.
   - A multi-replica HA deployment may prefer fail-fast if discovery is empty.
   - Recommendation: expose `RequirePeers` or `MinPeers` in wiring, not in the
     low-level mapper alone.
5. Which TLS config should NATS routes use?
   - Reuse the DERP mesh TLS material only if its trust and server-name
     semantics are correct for NATS route endpoints.
   - Otherwise introduce route-specific TLS config.
   - Recommendation: keep provider scheme generation independent of TLS config;
     require the x/nats caller to pass matching `ClusterTLSConfig`.

## Implementation phases, TDD-friendly

### Phase 1: Docs-only design record

- Create `docs/internal/xreplicasync-plan.md` from this plan.
- Keep it docs-only and do not modify `coderd/x/nats` or enterprise startup.
- Validate with markdown lint if available for internal docs.

### Phase 2: Provider package unit tests

- Add fake `ReplicaSource` tests first under
  `enterprise/coderd/x/xreplicasync`.
- Cover:
  - constructor rejects nil manager and nil route callback;
  - `Peers` calls `UpdateNow` when configured;
  - `UpdateNow` errors are returned;
  - self replica is excluded;
  - non-self primary replicas become `nats.Peer` values;
  - route callback errors fail `Peers`;
  - empty peer result is allowed for single-replica mode.
- Run `go test ./enterprise/coderd/x/xreplicasync`.

### Phase 3: Implement provider

- Implement only the smallest code needed for Phase 2 tests.
- Use `xerrors.Errorf` for contextual errors.
- Avoid extra abstractions beyond `ReplicaSource` and `RouteURLFunc`.
- Ensure comments on exported symbols are proper Go doc comments.

### Phase 4: Route URL helper tests and implementation

- Add tests for `RouteURLFromReplicaHostname` or whichever helper is chosen.
- Validate invalid scheme, empty hostname, and invalid port behavior.
- Confirm helper output is accepted by x/nats normalization through either
  direct x/nats tests or integration with `nats.New` test helpers.

### Phase 5: Optional DB-backed integration test

- If useful, add an integration test using real `replicasync.Manager` with
  `dbtestutil.NewDB(t)`, mirroring patterns in
  `enterprise/replicasync/replicasync_test.go:60-80`.
- Insert or create multiple managers with route URL mapping by ID or hostname.
- Assert the provider returns only primary non-self replicas.
- Keep this test focused on adapter behavior, not full embedded NATS clustering.

### Phase 6: Future dynamic refresh, only if selected

- Add x/nats tests that start multiple servers, call a new route refresh API,
  and verify new routes form without restart.
- Implement x/nats route reload using NATS server `ReloadOptions`.
- Wire `replicasync.SetCallback` to refresh route peers.
- Define logging and retry behavior for failed refreshes.

## Testing strategy

- Unit tests should not need a real database. The fake source is enough for the
  provider's mapping and error semantics.
- DB-backed tests should be limited to verifying compatibility with the real
  `replicasync.Manager` surface and `AllPrimary()` behavior.
- Full NATS cluster tests should stay in `coderd/x/nats` unless dynamic refresh
  is implemented. Existing cluster tests already exercise static peer route
  formation at `coderd/x/nats/cluster_test.go:92-104` and TLS route formation
  at `coderd/x/nats/cluster_tls_test.go:16-44`.
- Run targeted tests first:
  - `go test ./enterprise/coderd/x/xreplicasync`
  - `go test ./enterprise/replicasync`
  - `go test ./coderd/x/nats`
- Before merging implementation, run `make lint` and relevant pre-commit hooks.

## Risks

- Address ambiguity: the existing `replicas` table has no NATS route address or
  port, so any v1 mapping convention can be wrong in real deployments.
- Startup ordering: production pubsub is created before enterprise
  `replicasync.Manager`, so real wiring needs a startup-order or pubsub-factory
  change beyond this provider package.
- Silent standalone fallback: x/nats treats no peers as standalone mode, which
  can mask broken discovery in an HA deployment.
- Partial clusters: returning only the peers whose route URL can be derived may
  create confusing split-brain-like pubsub behavior. Prefer fail-fast for
  mapping errors.
- Workspace proxy contamination: using `Regional()` or `InRegion()` would risk
  including `Primary: false` workspace proxy replicas. Use `AllPrimary()` or an
  explicit primary filter.
- Dynamic update complexity: replicasync has callbacks, but x/nats does not yet
  expose a refresh API. Implementing dynamic routes touches server option reload
  semantics and needs separate tests.
