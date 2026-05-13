# Client-side connection-pool extension for `coderd/x/nats.Pubsub`

## 1. Problem restatement

`coderd/x/nats.Pubsub` currently concentrates all wrapper traffic for one
replica through one NATS client connection. The `Pubsub` struct stores a single
`*natsgo.Conn` in `p.nc` (`coderd/x/nats/pubsub.go:20-26`), `New` creates one
connection and assigns it to `p.nc` (`coderd/x/nats/pubsub.go:159-166`), and
both `Publish` and `SubscribeWithErr` use that same connection
(`coderd/x/nats/pubsub.go:290-317`, `coderd/x/nats/pubsub.go:335-360`). With
wide local fan-out, NATS server must enqueue one outbound copy per local
subscription behind that one client. The captured benchmarks show that this is
healthy for thin fan-out, `BenchmarkPubsubThinFanout/coder/cluster10/subj1/512KiB`
reached 100 percent delivery at 1653 pubs/s, but fails when many subscriptions
are concentrated on each wrapper client: `BenchmarkPubsub/coder/cluster10/subj1/512KiB`
reached only 2.99 percent delivery, `BenchmarkPubsub/coder/cluster10/subj10/512KiB`
reached 40.92 percent, and the hot-subject concentrated benchmark reached only
0.23 to 14.69 percent. This is distinct from server route pooling: server
options map `Options.RoutePoolSize` into `natsserver.ClusterOpts.PoolSize`
(`coderd/x/nats/server.go:98-115`), which parallelizes route traffic, not the
server-to-local-wrapper client outbound queue.

## 2. Existing wrapper facts that constrain the design

- `Pubsub` stores one embedded server pointer and one NATS client pointer today
  (`coderd/x/nats/pubsub.go:20-26`). `New` connects once through
  `connectInProcess` and stores the result in `p.nc`
  (`coderd/x/nats/pubsub.go:115-166`), while `NewFromConn` wraps one external
  connection without owning it (`coderd/x/nats/pubsub.go:169-180`).
- `connectInProcess` connects to `ns.ClientURL()` over TCP loopback and applies
  client options and handlers (`coderd/x/nats/server.go:168-207`). `Options`
  has no client pool setting today (`coderd/x/nats/options.go:19-97`).
- `Publish` maps the event with `LegacyEventSubject`, publishes on `p.nc`, and
  records wrapper metrics (`coderd/x/nats/pubsub.go:290-317`).
  `SubscribeWithErr` maps the event, subscribes synchronously on `p.nc`, applies
  pending limits, registers bookkeeping, and starts one goroutine per
  subscription (`coderd/x/nats/pubsub.go:335-390`).
- Each subscription goroutine calls `NextMsgWithContext` and invokes its
  listener independently (`coderd/x/nats/pubsub.go:408-429`), so the wrapper
  does not provide global callback ordering across listeners on one subject.
- Slow-consumer handling is subscription-specific via `subsByNATS` and shared
  wrapper metrics (`coderd/x/nats/pubsub.go:432-481`). `Close` drains one owned
  connection and waits on one `closedCh` (`coderd/x/nats/pubsub.go:483-528`).
- Metrics are wrapper-scoped and unlabeled by connection; `Collect` sums pending
  messages and bytes across all subscriptions (`coderd/x/nats/metrics.go:38-108`,
  `coderd/x/nats/metrics.go:131-170`).
- `RefreshPeers` reloads embedded server route URLs and does not touch the
  client connection (`coderd/x/nats/pubsub.go:183-255`).

## 3. Design recommendation

### 3.1 High-level shape

Add a client-side subscriber connection pool to `Pubsub`. The pool must reduce
server outbound queue concentration by making the embedded server see multiple
local wrapper client connections, each with only a fraction of the wrapper's
subscriptions.

Recommended struct shape, expressed as type signatures only:

```go
type Options struct {
    ClientConnPoolSize int
}

type Pubsub struct {
    subConns []pooledConn
    pubConn  *pooledConn
}

type pooledConn struct {
    index    int
    nc       *natsgo.Conn
    closedCh chan struct{}
}
```

Implementation notes:

- Replace `p.nc` with a set of owned client connections. A field name like
  `subConns` is more precise than `ncs` because the pool's main job is spreading
  subscriptions across server outbound queues.
- Keep `NewFromConn` as a single-connection wrapper. It should populate
  `subConns` with one external connection, set `pubConn` to that same entry, and
  leave ownership false. A future `NewFromConns` is out of scope.
- Normalize `Options.ClientConnPoolSize <= 0` to `1` at construction time. This
  preserves current default behavior for existing tests and callers.
- For `ClientConnPoolSize == 1`, reuse the one subscriber connection as the
  publisher connection. This keeps the current connection count and the current
  one-connection behavior by default.
- For `ClientConnPoolSize > 1`, create `N` subscriber connections plus one
  dedicated publisher connection. The dedicated publisher connection has no
  wrapper subscriptions, which keeps publisher writes isolated from subscriber
  receive pressure and avoids choosing a subscriber connection for publishes.

### 3.2 Subscription routing

Do not use pure subject-hash routing for subscriptions. It is the natural first
idea, but it cannot fix the hot-subject benchmark because every subscription for
one subject would still sit behind one server-to-client outbound queue.

Recommended assignment:

1. Convert the legacy event to the actual NATS subject with `LegacyEventSubject`,
   matching current publish and subscribe validation
   (`coderd/x/nats/pubsub.go:300-304`, `coderd/x/nats/pubsub.go:344-348`,
   `coderd/x/nats/subject.go:28-43`).
2. Compute a stable per-subject starting offset:
   `offset = fnv64a(subject) % len(subConns)`.
3. Maintain a wrapper map like `nextSubSeqBySubject map[string]uint64`, guarded
   by `p.mu`.
4. On each `SubscribeWithErr(subject)`, reserve the next sequence number for
   that subject and assign:
   `connIndex = (offset + seq) % len(subConns)`.
5. Create the NATS subscription on `subConns[connIndex].nc`.
6. Store `connIndex` on the wrapper `subscription` for debugging, tests, and
   future metrics.

This design uses subject hashing as a deterministic starting offset, then
stripes multiple subscriptions for the same subject across the pool. It
preserves per-listener ordering because each listener is still backed by exactly
one NATS subscription on exactly one connection. It intentionally does not
promise global callback ordering across separate listeners on the same subject,
which the current wrapper already does not guarantee because every subscription
runs its own goroutine (`coderd/x/nats/pubsub.go:377-379`,
`coderd/x/nats/pubsub.go:408-429`).

Subscription assignment should be stable for a subscription's lifetime. Existing
subscriptions should not be moved when the pool size changes because pool size
is a construction-time option, and rebalancing live NATS subscriptions would add
complexity and transient duplicate or missed delivery risks.

### 3.3 Publish routing

Recommend a dedicated publisher connection when `ClientConnPoolSize > 1`.

Rationale:

- A dedicated publisher connection has no wrapper subscriptions, so server
  outbound fan-out to local subscribers is spread only across `subConns`.
- One publisher connection preserves wrapper-originated publish order better
  than round-robin publishing because all publish calls enter the server through
  one client stream.
- Round-robin publishing is the weakest option. It can reorder consecutive
  publishes to the same subject across multiple client connections and gives no
  benefit for the measured server outbound queue bottleneck.
- Subject-hash publishing over the subscriber pool is acceptable for a minimal
  first implementation, but it couples writes to one of the receive-heavy
  connections and interacts poorly with connection-level `NoEcho` once
  same-subject subscriptions are striped.

`NoEcho` needs explicit handling. Existing `Options.NoEcho` is currently passed
as a NATS connection option (`coderd/x/nats/server.go:179-181`), and the default
unit test expects a local publish not to deliver back to the same wrapper when
`NoEcho` is true (`coderd/x/nats/pubsub_test.go:98-116`). NATS connection-level
`NoEcho` suppresses delivery only to subscriptions on the publishing connection;
that is insufficient once one wrapper owns multiple subscriber connections.

Recommended compatibility approach:

- Add an unexported wrapper instance ID generated in `New` and `NewFromConn`.
- When `Options.NoEcho` is true and the wrapper publishes, use `PublishMsg` with
  a small internal NATS header, for example `Coder-Pubsub-Origin: <instanceID>`.
- In `runSubscription`, inspect `msg.Header` before metrics and listener
  delivery. If `Options.NoEcho` is true and the origin header matches the
  wrapper instance ID, skip the message.
- Keep the connection-level `natsgo.NoEcho()` option for the single-connection
  default path if desired, but do not rely on it for correctness when the pool
  size is greater than one.

This preserves wrapper-level `NoEcho` semantics without changing the public
publish or subscribe signatures.

### 3.4 Lifecycle, handlers, and metrics

Connection lifecycle should remain wrapper-level, not per-subscription.
Construction should build the embedded server as today, normalize
`ClientConnPoolSize`, create one handler set per connection with shared wrapper
state, and clean up already-created connections if a later connection fails,
matching the current single-connection failure path
(`coderd/x/nats/pubsub.go:159-164`).

`Close` should preserve today's order of operations, mark closed, cancel and
unregister subscriptions, drain owned connections, then shut down the owned
server (`coderd/x/nats/pubsub.go:483-528`). Drain all unique owned connections
concurrently, drain an aliased `pubConn` only once, and join errors with
connection indexes. Replace the single `closedCh` with one closed channel per
connection, or a wait group decremented by each `ClosedHandler`; the current
single channel is tied to one connection and would report success after only the
first pooled connection closed (`coderd/x/nats/pubsub.go:37-40`,
`coderd/x/nats/pubsub.go:144-146`, `coderd/x/nats/pubsub.go:513-518`).

Handlers should share wrapper-level state. Disconnect, reconnect, and closed
handlers increment the existing wrapper counters through closures over `p`
(`coderd/x/nats/pubsub.go:133-157`). Async slow-consumer handling should remain
subscription-based through `subsByNATS` (`coderd/x/nats/pubsub.go:432-481`).

Keep metrics wrapper-scoped and do not add a `conn` label in the first version.
Existing metrics have low cardinality and pending gauges already sum across all
subscriptions (`coderd/x/nats/metrics.go:38-108`,
`coderd/x/nats/metrics.go:143-170`). A connection label would multiply series by
pool size. If needed later, add new debug-only sampled gauges rather than
changing existing counters.

## 4. Subject-to-connection hash function

Use Go's standard `hash/fnv` FNV-1a 64-bit hash over the final NATS subject
string, then modulo by pool size.

Recommendation details:

- Hash the subject after `LegacyEventSubject`, not the raw legacy event. This
  makes routing match the actual NATS subject namespace used by `Publish` and
  `SubscribeWithErr` today (`coderd/x/nats/pubsub.go:300-305`,
  `coderd/x/nats/pubsub.go:344-350`).
- Use `fnv.New64a` or an equivalent small helper based on FNV-1a.
- Do not use `hash/maphash`; it is intentionally seeded per process, which
  makes tests, logs, and operational reproduction harder.
- Do not try to match an internal NATS subject hash. The pool assignment is a
  local wrapper implementation detail, not part of NATS routing semantics.
- Use the hash only as the subject's starting offset. Actual subscription
  placement should be striped with the per-subject sequence described above.

## 5. Backward compatibility

- `Options.ClientConnPoolSize == 0` and negative values should normalize to `1`.
- With the default normalized size of `1`, existing tests should continue to
  pass without changes. Current unit tests construct `Options{}` in round-trip,
  echo, ordering, and close tests (`coderd/x/nats/pubsub_test.go:34-144`,
  `coderd/x/nats/pubsub_test.go:206-225`).
- The public `pubsub.Pubsub` interface remains unchanged. `Subscribe`,
  `SubscribeWithErr`, `Publish`, `RefreshPeers`, and `Close` signatures do not
  change.
- `NewFromConn` remains single-connection. `ClientConnPoolSize` is relevant to
  `New`, not to callers that explicitly provide a connection.
- Pool size is construction-time only. There is no runtime resize API.

## 6. `RefreshPeers` interaction

`RefreshPeers` should not interact with `ClientConnPoolSize`.

Reason: `RefreshPeers` updates embedded server route URLs by cloning server
options and calling `p.ns.ReloadOptions(newOpts)`
(`coderd/x/nats/pubsub.go:247-255`). Client connection pool size is local
client topology against the already-running server's client listener, while
`RefreshPeers` is server route topology. The existing route pool setting is
`Options.RoutePoolSize`, applied to `natsserver.ClusterOpts.PoolSize`
(`coderd/x/nats/server.go:98-115`), and should remain a separate server-side
routing knob.

## 7. Worked example, `ClientConnPoolSize = 8`

Assumptions:

- Payload is 512 KiB, which is 524,288 bytes.
- NATS default server `MaxPending` is 64 MiB, which is 67,108,864 bytes.
- One client queue can therefore hold at most `67,108,864 / 524,288 = 128`
  queued 512 KiB message copies before exhausting the 64 MiB budget.

Workload: one replica has 100 subscribers split evenly across 10 subjects, so
there are 10 subscribers per subject. If all 10 subjects receive one 512 KiB
message during the same burst, total local fan-out is 100 message copies, or
50 MiB.

Current single-connection wrapper:

- All 100 local subscriptions sit behind one server-to-client queue.
- One full 10-subject burst queues about `100 * 512 KiB = 50 MiB` for that
  client if the client cannot drain immediately.
- Headroom is `64 MiB / 50 MiB = 1.28` such bursts. A second burst can push the
  client past `MaxPending`, which matches the measured slow-consumer behavior.

Recommended striped subscriber pool with 8 subscriber connections:

- Each subject gets a deterministic offset, and its 10 subscribers are striped
  across 8 connections.
- For one subject, each connection gets either 1 or 2 subscribers.
- Across 10 subjects and 100 subscribers total, each connection should carry
  about 12 or 13 subscriptions in aggregate, subject to small hash and sequence
  skew.
- One full 10-subject burst therefore queues about `12 * 512 KiB = 6 MiB` to
  `13 * 512 KiB = 6.5 MiB` per client connection.
- Headroom becomes about `64 MiB / 6.5 MiB = 9.8` full bursts on the busiest
  connection.

This should lift the measured bottleneck for the 100-subscriber, 10-subject
case because it reduces the server's per-client outbound pressure by roughly
the pool size. It should also substantially improve the 100-subscriber,
1-subject case under the recommended striped design: the 100 subscribers spread
about 12 or 13 per connection, instead of all 100 staying on one connection.

Important limit for the hot-subject benchmark:

- Pooling raises the threshold linearly with the number of subscriber
  connections, but it does not make unbounded fan-out free.
- For 1000 subscribers on one 512 KiB subject and pool size 8, each connection
  carries about 125 subscriptions, or `125 * 512 KiB = 62.5 MiB` per publish.
  That is barely under 64 MiB, so pool size 8 has almost no burst headroom.
  Pool size 16 is a more realistic starting point for reliable delivery.
- For 5000 subscribers on one 512 KiB subject, pool size 8 would put about 625
  subscriptions on each connection, or 312.5 MiB per publish, which exceeds the
  default 64 MiB queue budget before considering any burst. A pool of at least
  `ceil(5000 / 128) = 40` subscriber connections is required for even one queued
  publish to fit under 64 MiB per connection, and a larger value such as 64 is a
  more realistic benchmark sweep point.

Therefore, the benchmark expectation should be phrased as: the previously
failing leaves should reach 100 percent delivery when `ClientConnPoolSize` is
bumped high enough for the fan-out and payload size. Pool size 8 is enough for
the 100-subscriber worked example, but not for every hot-subject case.

## 8. Test strategy

### 8.1 Existing tests

Run the existing package tests with default options. Because
`ClientConnPoolSize` defaults to 1, these should pass without changing their
call sites:

- Round trip and normal `SubscribeWithErr` delivery
  (`coderd/x/nats/pubsub_test.go:34-75`).
- Default echo and `NoEcho` behavior (`coderd/x/nats/pubsub_test.go:77-116`).
- Per-listener ordering (`coderd/x/nats/pubsub_test.go:119-144`).
- `NewFromConn` ownership behavior (`coderd/x/nats/pubsub_test.go:146-204`).
- Idempotent `Close` (`coderd/x/nats/pubsub_test.go:206-225`).

### 8.2 New unit tests

Add white-box tests in package `nats` when they need unexported connection
indexes or subscription bookkeeping. Cover:

1. Default normalization: `Options{}` and `ClientConnPoolSize: 0` create one
   subscriber connection and reuse it for publishing.
2. Pool construction: `ClientConnPoolSize: 4` creates four subscriber
   connections plus an owned publisher connection.
3. Deterministic subject offsets: subjects with different FNV offsets for pool
   size 4 assign their first subscription to the expected indexes.
4. Same-subject striping: repeated subscriptions follow `(offset + seq) % 4`.
   This intentionally replaces the weaker same-subject-to-same-connection test,
   which would not solve hot-subject fan-out.
5. Subscription lifetime stability: adding or canceling other subscriptions does
   not move an existing subscription's connection index.
6. `Close` drains all unique owned connections and remains idempotent.
7. Slow-consumer async lookup still maps pooled NATS subscriptions through
   `subsByNATS` and increments shared drop metrics.
8. `NoEcho` with pool size greater than one suppresses self-published messages
   for listeners spread across multiple subscriber connections.

### 8.3 Benchmark changes

The existing benchmark has a package-level `-bench.type` flag
(`coderd/x/nats/bench_test.go:55-60`) and creates Coder-mode pubsubs with
`xnats.Options{}` in standalone mode (`coderd/x/nats/bench_test.go:306-314`) or
cluster options in cluster mode (`coderd/x/nats/bench_test.go:337-347`). Add a
new `-bench.connpool=N` flag that defaults to `1`.

Benchmark integration:

- Default the flag to `1` to preserve today's benchmark behavior.
- Apply it only in Coder mode by setting `Options.ClientConnPoolSize` in both
  standalone and cluster `setupCoder` paths.
- Native mode can ignore the flag, or fail fast if the flag is not 1. Ignoring
  is less surprising for scripts that sweep common flags across both modes.
- Include the pool size in the benchmark leaf name only when it is not 1, or log
  it at leaf start. Keeping default names unchanged preserves comparability.

Manual sweeps after implementation:

- `BenchmarkPubsub/coder/cluster10/subj1/512KiB` with `-bench.connpool=8` should
  improve materially because 100 same-subject subscribers per replica stripe
  across 8 local client queues.
- `BenchmarkPubsub/coder/cluster10/subj10/512KiB` with `-bench.connpool=8`
  should have about 10 bursts of 512 KiB headroom per busiest connection in the
  worked example.
- `BenchmarkPubsubHotSubjectConcentrated/coder/standalone/subj1/subs1000/512KiB`
  should be swept at 8, 16, and 32. Pool size 8 is mathematically fragile,
  while 16 gives useful headroom.
- `BenchmarkPubsubHotSubjectConcentrated/coder/standalone/subj1/subs5000/512KiB`
  should be swept at 40, 64, and possibly higher. Pool size 8 cannot fit one
  full 512 KiB publish under the default 64 MiB per-client budget.

## 9. Out of scope

- Do not change server-side `Cluster.PoolSize` or `Options.RoutePoolSize`; that
  is route pooling, not local wrapper client pooling
  (`coderd/x/nats/server.go:98-115`).
- Do not modify `RefreshPeers` beyond ensuring it continues to work with pooled
  client connections (`coderd/x/nats/pubsub.go:183-255`).
- Do not add JetStream support. The embedded server is currently configured with
  `JetStream: false` (`coderd/x/nats/server.go:54-60`).
- Do not change the public subscription handler signatures. `Subscribe` and
  `SubscribeWithErr` should keep their current API shape
  (`coderd/x/nats/pubsub.go:320-335`).
- Do not bump server `MaxPending` as part of this change. It is a separate
  tuning knob and should be evaluated independently after the client-side queue
  concentration is removed.
- Do not implement runtime pool resizing or live subscription rebalancing.
- Do not add per-connection Prometheus labels in the first implementation.
