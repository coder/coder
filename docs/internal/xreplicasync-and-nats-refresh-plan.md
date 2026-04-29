# xreplicasync and coderd/x/nats dynamic refresh plan

> Note: This document supersedes
> [`docs/internal/xreplicasync-plan.md`](./xreplicasync-plan.md). The earlier
> plan is preserved for history. New work should follow this unified plan,
> which adds a dynamic peer refresh API to `coderd/x/nats` and an
> `enterprise/coderd/x/xreplicasync` provider that adapts
> `enterprise/replicasync.Manager` to it.

## Goals

- Add `func (p *Pubsub) RefreshPeers(ctx context.Context) error` to
  `coderd/x/nats` so the embedded NATS server can pick up route set changes
  after startup without a full restart.
- Persist `PeerProvider` on `*Pubsub` so refresh can re-query peers after
  startup. Update the `PeerProvider` godoc to make clear that implementations
  must be safe for repeated calls.
- Back refresh with `nats-server`'s `Server.ReloadOptions`, restricted to
  changes in the route set. Cluster host/port/token/TLS/auth must remain
  identical to startup; upstream rejects host/port changes in
  `validateClusterOpts`.
- Add a new package `enterprise/coderd/x/xreplicasync` that implements
  `coderd/x/nats.PeerProvider` over `enterprise/replicasync.Manager`, and
  drives refresh via `Manager.SetCallback`.
- Use same-region primary replicas as NATS route peers; exclude self by
  `Manager.ID()`. Derive route URLs via explicit helpers, without modifying
  the `replicas` schema.

## Non-goals

- Do not modify `enterprise/replicasync` or the `replicas` schema.
- Do not change production startup ordering as part of this change.
- Do not change empty-peer/standalone startup behavior. `coderd/x/nats` will
  continue to set `DontListen = true` when there are no peers.
- Do not provision TLS certs or NATS route tokens; this plan assumes existing
  cluster credentials are configured at startup and remain stable.
- Do not add JetStream or any persistence layer to embedded NATS.

## Design A: `coderd/x/nats` `RefreshPeers`

### New error

```go
var ErrStandalone = errors.New("nats pubsub is in standalone mode")
```

### New fields on `*Pubsub`

- `provider PeerProvider`, retained from `Options` so refresh can re-query.
- `serverOpts *natsserver.Options`, the running server options snapshot used
  as the base for `ReloadOptions`.
- `refreshMu sync.Mutex`, serializes refresh calls.
- `currentRoutes []*url.URL`, last applied route set, sorted.
- `standalone bool`, true when `New` started without peers.

### Semantics of `RefreshPeers`

1. Closed-state check under the existing lifecycle mutex, returning a closed
   error if the pubsub has already been shut down.
2. If `provider == nil` or `standalone`, return `ErrStandalone`. A pubsub
   that started standalone cannot be promoted to clustered via reload because
   reload cannot change cluster host/port.
3. Acquire `refreshMu`. Call `provider.Peers(ctx)`, normalize through
   `normalizePeers`, and rebuild route URLs through the same helper used at
   startup (the `routeURLs`-equivalent path), preserving scheme and the
   `coder:<token>` userinfo when configured.
4. Sort the resulting routes by `URL.String()`. If the sorted list equals
   `currentRoutes`, return `nil` without calling into the server.
5. Shallow-clone `serverOpts`, replace only `Routes`, and call
   `p.ns.ReloadOptions(newOpts)`. Do not reuse the original options pointer
   afterward.
6. On success, store the new options pointer and `currentRoutes`. On failure,
   wrap the error as `reload nats routes: %w` and leave `serverOpts` and
   `currentRoutes` unchanged.

### Self-route handling

Defensively compare each refreshed route host against `p.ns.ClusterAddr()`
and the configured `ClusterAdvertise` and drop matches. Primary self-exclusion
is the responsibility of `xreplicasync`, but the embedded layer should not
trust callers to never include self.

## Design B: `enterprise/coderd/x/xreplicasync`

### Types

- `ReplicaSource` interface:
  - `ID() uuid.UUID`
  - `Regional() []database.Replica`
  - `SetCallback(func())`
- `RouteURLFunc func(database.Replica) (string, error)`
- `Options{ Logger, Source, RouteURL, RefreshFailures prometheus.Counter,
  RetryMinBackoff, RetryMaxBackoff }`
- `Provider` exposing:
  - `Peers(ctx context.Context) ([]nats.Peer, error)`
  - `Start(ctx context.Context, p *nats.Pubsub) error`
  - `Close() error`

### `Peers`

Read `Source.Regional()`, filter to `Primary == true`, exclude
`replica.ID == source.ID()`, derive `RouteURL` via `Options.RouteURL`, set
`Name` to `replica.Hostname` (falling back to `replica.ID.String()` when
empty).

### Route URL helpers

- `RouteURLFromReplicaHostname(scheme string, port int) RouteURLFunc`
- `RouteURLFromRelayAddress(scheme string, port int) RouteURLFunc`

Both accept only `nats` or `tls` schemes and require `port > 0`. The
`RelayAddress` helper parses the stored HTTP URL and extracts the host
without retaining the HTTP port, only the hostname is reused, with the
caller-specified NATS route port appended.

### Material-change fingerprint

Compute an FNV-64a hash over the sorted route URL strings only (not names).
A name-only change must not trigger a reload. The applied fingerprint is
stored on the `Provider` under a mutex. The initial applied fingerprint is
the fingerprint of the startup snapshot used by `nats.New`.

### Callback and retry loop

`Source.SetCallback` registers a coalesced enqueue: a buffered channel of
size 1, with non-blocking send so that bursts collapse into one wakeup.

A worker goroutine:

1. Reads the wakeup signal.
2. Calls `Peers(ctx)`, computes the candidate fingerprint, and compares to
   the applied fingerprint.
3. If unchanged, sleeps until the next wakeup.
4. If changed, calls `Pubsub.RefreshPeers`. On success, stores the new
   fingerprint and resets backoff to `RetryMinBackoff` (default 1s). On
   failure, logs, increments `coder_pubsub_nats_refresh_failures_total`,
   and retries with capped exponential backoff up to `RetryMaxBackoff`
   (default 60s).
5. Treats `ErrStandalone` as terminal until the next material change, since
   no amount of retrying will turn a standalone server into a clustered one.

`Close` cancels the worker context and waits for the goroutine to exit.

### Caveat: `SetCallback` is single-slot

`Manager.SetCallback` stores only one callback. Production wiring must
compose with existing enterprise callbacks (DERP mesh refresh, entitlements
recheck, etc.) rather than overwriting them. See follow-ups.

## Testing strategy

### `coderd/x/nats`

Integration tests against an embedded NATS server:

- `Add`: start with peers `{A}`, refresh to `{A, B}`, observe new route in
  reloaded options.
- `Remove`: start with `{A, B}`, refresh to `{A}`.
- `NoOp`: refresh with identical peer list does not call `ReloadOptions`.
- `Standalone`: `New` with no peers returns `ErrStandalone` from refresh.
- `Token+TLS`: route URL rebuild preserves `coder:<token>` userinfo and
  `tls://` scheme.

### `xreplicasync`

Unit tests with a fake `ReplicaSource`:

- Region/primary/self filtering.
- Route URL error propagation (helper returns error, `Peers` surfaces it).
- Fingerprint sort and name-independence.
- No reload on unchanged fingerprint.
- Reload on material change.
- Retry backoff growth and reset on success.

Optional DB-backed integration test using `replicasync.Manager` with
`dbtestutil`.

## Risks

- `ReloadOptions` in embedded mode is not heavily exercised upstream;
  integration tests are required to validate the route-only reload path.
- Route removal semantics in NATS clustering are topology-dependent and
  partly driven by gossip. Tests should assert on the reloaded options
  rather than on inter-server connection state alone.
- `ReloadOptions` cannot change cluster host/port; the running options
  must be cloned and only `Routes` mutated.
- `SetCallback` is single-slot. Wiring this provider into enterprise coderd
  must compose with other consumers, not overwrite them.
- Fingerprint collisions: FNV-64a is sufficient at expected HA replica
  scale; cryptographic strength is not required.
- `RelayAddress` is an HTTP DERP URL. Deriving a NATS route host from it
  assumes the peer is reachable on the configured NATS route port at the
  same hostname.

## Implementation phases

1. Docs (this file).
2. `coderd/x/nats` `RefreshPeers` tests (red).
3. `RefreshPeers` implementation (green).
4. `xreplicasync` tests (red).
5. `xreplicasync` implementation (green).
6. Route URL helpers and their tests.
7. Refresh loop, retry, and metrics, with tests.
8. Optional DB-backed integration test.

## Follow-ups

- Decide production startup ordering for embedded NATS pubsub vs.
  `replicasync.Manager`, including how the manager's first heartbeat
  relates to `nats.New`.
- Consider adding a dedicated NATS route address column to `replicas` to
  remove ambiguity between DERP relay address and NATS route URL.
- Compose `SetCallback` consumers so xreplicasync, DERP mesh, and
  entitlements all observe replica changes.
- Operator-facing docs for hostname stability: StatefulSet hostnames vs.
  `Deployment` + headless `Service` topologies, and what each means for
  `RouteURLFromReplicaHostname`.
