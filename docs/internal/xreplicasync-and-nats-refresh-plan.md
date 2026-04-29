# xreplicasync and coderd/x/nats dynamic refresh plan

> Note: This document supersedes
> [`docs/internal/xreplicasync-plan.md`](./xreplicasync-plan.md). The earlier
> plan is historical and preserved for context only. New work should follow
> this unified plan, which adds a dynamic peer refresh API to `coderd/x/nats`
> and an `enterprise/coderd/x/xreplicasync` provider that adapts
> `enterprise/replicasync.Manager` to it.

## Architectural framing (Option A vs Option B)

This work is the foundation of "Option A":

- **Option A (chosen).** `enterprise/replicasync.Manager` continues to use
  `pgPubsub` for its low-volume replica-registry traffic
  (`replicaUpdateChannel`). NATS is reserved for application-level,
  high-volume events (e.g. workspace agent fanout). The
  `xreplicasync.Provider` reads the same replica set that `replicasync`
  already maintains and feeds it as peers into `coderd/x/nats`. NATS is
  downstream of `replicasync`; replica-registry traffic never flows
  through NATS.
- **Option B (rejected).** Migrate `replicasync`'s own update channel to
  NATS. This would create a circular dependency: NATS needs the replica
  set to discover peers, and `replicasync` would need NATS to publish
  replica updates. Option A avoids the cycle entirely by keeping the
  replica-registry feedback loop on Postgres.

Production wiring of `xreplicasync.Provider` into `cli/server.go` /
`enterprise/coderd/coderd.go` is deliberately out of scope for the
package work described here; it will land in a follow-up PR. This
document describes the package shape and refresh semantics only.

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
- Do not provision TLS certs or NATS route tokens beyond the
  ephemeral cluster token auto-generated when `Options.ClusterToken` is
  empty.
- Do not add JetStream or any persistence layer to embedded NATS.

## Design A: `coderd/x/nats` `RefreshPeers`

### Always-clustered startup ("cluster of 1")

Standalone mode is gone. Every `coderd/x/nats.New` starts the embedded
NATS server in cluster mode, even when `Options.PeerProvider` is nil or
returns zero peers. With zero peers the cluster listener still binds on
a random loopback port; no routes are configured. Late-joining peers
are added via `RefreshPeers` and applied through `Server.ReloadOptions`
without restarting the server.

This decision is forced by upstream: `nats-server`'s `validateClusterOpts`
rejects host/port changes on `ReloadOptions`, so a Pubsub that started
without a cluster listener cannot be promoted to a clustered one at
runtime. We bind the cluster listener up front to make refresh work.

A throwaway smoke test against `nats-server` v2.12.8 confirmed that
`NewServer` with cluster enabled, empty `Cluster.Routes`, random
`Cluster.Port`, valid `ClusterToken`, and a custom router authenticator
starts cleanly and `ReadyForConnections` returns true within a second.

### `ClusterToken`: required, auto-generated when empty

`Options.ClusterToken` is the shared secret applied to NATS route
authentication. Callers SHOULD supply a stable token when this process
is intended to interoperate with other replicas. When left empty, `New`
generates a 32-byte random hex token internally (via `crypto/rand`) and
records it on `*Pubsub` so subsequent `RefreshPeers` calls reuse the
same token when constructing route URLs. The auto-generated path keeps
ergonomics for tests and single-replica deployments where there is no
real cluster to authenticate against.

### Sentinel error

```go
var ErrNoEmbeddedServer = errors.New("nats pubsub has no embedded server")
```

`RefreshPeers` returns `ErrNoEmbeddedServer` only when the Pubsub was
constructed via `NewFromConn` and therefore does not own an embedded
server whose route configuration can be reloaded. The previous
`ErrStandalone` sentinel has been removed.

### New fields on `*Pubsub`

- `provider PeerProvider`, retained from `Options` so refresh can re-query.
- `serverOpts *natsserver.Options`, the running server options snapshot used
  as the base for `ReloadOptions`.
- `refreshMu sync.Mutex`, serializes refresh calls.
- `currentRoutes []*url.URL`, last applied route set, sorted.
- `effectiveClusterToken string`, the token actually applied to the
  embedded server, used to rebuild route URLs in `RefreshPeers` (mirrors
  `Options.ClusterToken` when supplied, otherwise the ephemeral token).

### Semantics of `RefreshPeers`

1. Closed-state check under the existing lifecycle mutex, returning a closed
   error if the pubsub has already been shut down.
2. If the Pubsub has no embedded server (`NewFromConn`), return
   `ErrNoEmbeddedServer`.
3. If `provider == nil`, return a configuration error
   (`"nats pubsub: no PeerProvider configured"`). The server is up but
   there is no source to refresh from. This is a misconfiguration, not
   a runtime topology condition, so it is not the same sentinel as
   `ErrNoEmbeddedServer`.
4. Acquire `refreshMu`. Call `provider.Peers(ctx)`, normalize through
   `normalizePeers`, and rebuild route URLs using `effectiveClusterToken`,
   preserving scheme and the `coder:<token>` userinfo.
5. Sort the resulting routes by `URL.String()`. If the sorted list equals
   `currentRoutes`, return `nil` without calling into the server. This
   includes the empty-set case for a "cluster of 1" whose provider
   returns zero peers.
6. Shallow-clone `serverOpts`, replace only `Routes`, and call
   `p.ns.ReloadOptions(newOpts)`. Do not reuse the original options pointer
   afterward.
7. On success, store the new options pointer and `currentRoutes`. On failure,
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
5. Treats `ErrNoEmbeddedServer` as terminal until the next material
   change, since a Pubsub that wraps an externally provided connection
   has no embedded server to reload no matter how many times we retry.

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
- `ZeroPeersNoOp`: `New` with a provider that returns zero peers
  refreshes successfully as a no-op (cluster of 1).
- `NilProviderConfigError`: `New` with no `PeerProvider` returns a
  config error from refresh (not `ErrNoEmbeddedServer`).
- `NewFromConn`: `RefreshPeers` returns `ErrNoEmbeddedServer`.
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
