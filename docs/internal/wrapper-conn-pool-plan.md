# Dual TCP-loopback connections for coderd/x/nats.Pubsub

This document is the current recommended design for fixing wide fan-out
failures in the `coderd/x/nats` pubsub wrapper. It supersedes the
per-subscription-connection design recorded in commit `f056ddcbf0`, which
itself superseded the original striped TCP loopback connection pool design
recorded in commit `bdc5406932`. Both prior designs are obsolete. This file
is the single source of truth for the target design; there is no separate
"superseded" doc.

There is no menu of alternatives here: the recommendation is exactly two
`*nats.Conn`s per wrapper, both dialed over TCP loopback to the embedded
server's client listener. All publishes go through one conn; all
subscriptions are multiplexed over the other.

The public `pubsub.Pubsub` interface (`Publish`, `Subscribe`,
`SubscribeWithErr`) does not change. Internal `coderd/x/nats.Pubsub`,
`Options`, `New`, and `NewFromConn` do.

## 1. Problem restatement

Today the wrapper concentrates all publish and subscribe traffic for a given
`coderd/x/nats.Pubsub` instance through a single `*nats.Conn` dialed via
`nats.InProcessServer(ns)` over an in-memory `net.Pipe`:

- `Pubsub` stores one embedded server pointer and one NATS client pointer
  (`coderd/x/nats/pubsub.go:20-26`).
- `New` starts the embedded server, dials it once via `connectInProcess`,
  and stores the result in `p.nc` (`coderd/x/nats/pubsub.go:115-166`).
- `Publish` writes through `p.nc` (`coderd/x/nats/pubsub.go:290-317`).
- `SubscribeWithErr` creates every NATS subscription on `p.nc` and starts
  one drain goroutine per subscription
  (`coderd/x/nats/pubsub.go:335-390`, `coderd/x/nats/pubsub.go:408-429`).

With many local subscriptions all owned by one client connection, the
embedded server must enqueue one outbound copy per local subscription into
that single client's outbound queue. Once that per-client queue passes the
server's `MaxPending` budget the server disconnects the client as a slow
consumer. The in-memory `net.Pipe` makes this strictly worse: it is
unbuffered, so any stall in the client's reader stalls the server's writer
immediately, with no kernel socket buffer to absorb the spike.

Previously captured benches show this concentration failure mode clearly:

- `BenchmarkPubsub/coder/cluster10/subj1/512KiB` fails on wide fan-out.
- `BenchmarkPubsub/coder/cluster10/subj10/512KiB` fails the same way.
- `BenchmarkPubsubHotSubjectConcentrated/coder/standalone/subs1000/512KiB`
  and `subs5000/512KiB` fail with the same per-client backpressure pattern.

Thin fan-out cases pass because the per-client outbound budget is never
under pressure.

Server route pooling is not the fix. `Options.RoutePoolSize` configures NATS
server-to-server route pool size and is forwarded into the embedded server's
cluster options (`coderd/x/nats/server.go:98-115`). It controls route
traffic between servers, not the server's outbound queue toward a local
wrapper client connection. The bottleneck described above lives entirely on
the server-to-local-client edge.

The recommended fix is structural on two axes:

1. Move off `net.Pipe` (zero kernel buffer) onto TCP loopback (real kernel
   socket buffer), so the server has slack to absorb transient bursts on
   the server-to-client edge.
2. Split publish from subscribe traffic onto two connections, so PUB-side
   flow control cannot interact with MSG-side flow control on the same
   socket (no head-of-line blocking between publishes and deliveries).

All subscriptions still multiplex over a single subscriber connection.
Per-subscription slow-consumer isolation is provided by NATS client-side
`PendingLimits` on each `*nats.Subscription`, not by separate conns.

## 2. Design recommendation: dual TCP-loopback conns

Architecture:

- `New` starts the embedded server (unchanged), waits for
  `ReadyForConnections`, then opens exactly two `*nats.Conn`s:
  - `pubConn`: used by `Publish` for every publish.
  - `subConn`: used by `Subscribe` and `SubscribeWithErr` for every
    subscription.
- Both conns dial `ns.ClientURL()` over TCP loopback
  (`127.0.0.1:<ephemeral>`). The embedded server already binds its client
  listener at `127.0.0.1:RANDOM_PORT` (`coderd/x/nats/server.go:74-79`); no
  new config surface is needed.
- Every `Subscribe` and `SubscribeWithErr` call creates one NATS
  subscription on `subConn` via `subConn.Subscribe(subject, cb)` and
  applies `SetPendingLimits` on the returned `*nats.Subscription`.
- Canceling the returned `pubsub.CancelFunc` calls `Drain` (or
  `Unsubscribe`) on that one subscription. No connection is closed.
- `Close` drains `subConn`, closes both conns, then shuts down the embedded
  server.

Consequences:

- Exactly two client connections to the embedded server, regardless of
  subscription count.
- Per-subscription overhead drops from roughly six goroutines plus
  ~550 KiB (per-sub-conn design) to roughly two goroutines plus a few KiB
  of subscription state. At 1000 subs/replica this is ~2000 vs. ~6000
  goroutines and a few MiB vs. ~550 MiB.
- TCP loopback gives the server-to-client edge a real kernel socket buffer
  to absorb bursts. The unbuffered `net.Pipe` failure mode is gone.
- Per-subscription slow-consumer isolation comes from client-side
  `PendingLimits` per `*nats.Subscription`, not from connection separation.
  See section 5 for the honest treatment of residual risk.
- No pool size knob. No subject striping. No FNV hash. No `connIndex`
  bookkeeping. No per-sub goroutine fleet.
- The cluster route port (`ClusterPort`, default ~6222) is unchanged.
  Routes use a different wire protocol (RS+/RS-/RMSG/route-CONNECT and
  ClusterToken auth) on a separate listener. We do not collapse client and
  route listeners (see section 9).

Compact shape, not a full implementation:

```go
type Pubsub struct {
    ns      *natsserver.Server
    pubConn *natsgo.Conn
    subConn *natsgo.Conn
    subs    map[uuid.UUID]*subscription
    // existing wrapper metrics, peer refresh state, locks, and close state.
}

type subscription struct {
    id       uuid.UUID
    sub      *natsgo.Subscription // owned by Pubsub.subConn
    listener pubsub.ListenerWithErr
    cancel   func() // drains/unsubscribes sub and removes from p.subs.
    // existing context, drop counters, and listener-error plumbing.
}
```

No per-sub `nc *natsgo.Conn`. No per-sub `connWG`. Delivery to the listener
runs on the async delivery goroutine that nats.go spawns per
`*Subscription` internally; the wrapper does not need its own drain
goroutine per subscription.

`NewFromConn` is the explicit exception:

- It accepts one external `*nats.Conn` and uses it for both publish and
  subscribe.
- It does not get the publish/subscribe split.
- It stays intentionally simple. Callers who choose `NewFromConn` own the
  topology and the per-client budget themselves.

Metrics stay wrapper-scoped. Existing metric cardinality is already low and
pending gauges sum over `p.subs` (`coderd/x/nats/metrics.go:38-108`,
`coderd/x/nats/metrics.go:143-170`). Do not add per-connection labels.

## 3. `NoEcho` removal

`Options.NoEcho` is removed rather than emulated. With the publisher and
subscriber on separate connections, the subscriber connection will not see
the publisher's own messages echoed back, which preserves the existing
no-callers-need-this property. No repository caller depends on `NoEcho`.

- `Options.NoEcho` is defined in `coderd/x/nats/options.go:75-77`.
- It is applied via `natsgo.NoEcho()` in `coderd/x/nats/server.go:179-181`
  (inside the misnamed `connectInProcess`).
- It is covered by `TestStandalone_NoEcho` in
  `coderd/x/nats/pubsub_test.go:98-116`.
- It is documented in `coderd/x/nats/doc.go:60-65`.
- A repository grep finds no production callers; remaining references are
  the option, its application, the doc comment, and the test.

Implementation work for the follow-up code change:

- Remove `Options.NoEcho` from `coderd/x/nats/options.go`.
- Remove the `natsgo.NoEcho()` branch from the connection option builder in
  `coderd/x/nats/server.go`.
- Delete `TestStandalone_NoEcho` from `coderd/x/nats/pubsub_test.go`.
- Update the echo section of `coderd/x/nats/doc.go` so it no longer
  advertises `Options.NoEcho`.

The previous "wrapper-instance-ID header" suppression workaround is
explicitly rejected. With `NoEcho` removed, there is nothing to suppress
and no compatibility shim is needed.

## 4. Lifecycle

### Construction

- Start the embedded server as today.
- Wait for `ns.ReadyForConnections` with the existing timeout.
- Read `ns.ClientURL()` once.
- Dial it twice, building `pubConn` and `subConn` via
  `natsgo.Connect(url, opts...)` with:
  - `MaxReconnects(-1)`: the server lives in this same process; reconnect
    is effectively "the server crashed and came back", which should not
    happen but we want continuity if it does.
  - Existing handlers (`ErrorHandler`, `DisconnectErrHandler`,
    `ReconnectHandler`, `ClosedHandler`) installed as closures over `p` so
    wrapper-level counters and slow-consumer handling continue to work.
- Do not pass `nats.InProcessServer(ns)`. The new design uses TCP loopback.
- Peer refresh and route clustering behavior are unrelated to local client
  connections and remain unchanged.

### Subscribe / SubscribeWithErr

- Validate closed state and map the legacy event subject through
  `LegacyEventSubject`, as today.
- Call `p.subConn.Subscribe(subject, cb)` (or `ChanSubscribe`/`QueueSubscribe`
  if the existing path uses them; match current semantics) to create
  exactly one NATS subscription on the shared subscriber connection.
- Apply per-subscription pending limits via
  `sub.SetPendingLimits(opts.PendingLimits.Msgs, opts.PendingLimits.Bytes)`.
  This is the per-subscription slow-consumer budget that gives isolation
  between subs multiplexed on `subConn`.
- Register a `subscription{id, sub, listener, cancel, ...}` in `p.subs`.
- The wrapper's per-subscription drain goroutine goes away. nats.go's own
  async delivery goroutine (one per `*Subscription`) invokes the callback.

### Unsubscribe

- Cancel the subscription context to unblock any in-flight listener work
  that observes context.
- Call `sub.Drain()` with a bounded timeout so in-flight delivery
  completes, then fall back to `sub.Unsubscribe()` if drain exceeds the
  timeout. (Match current semantics; current path uses `Unsubscribe`.)
- Unregister from `p.subs`.
- Do not close any connection. `subConn` keeps serving other subscriptions.

### Close

- Mark the wrapper closed (idempotent).
- Cancel every active subscription and drain `subConn` so queued messages
  flush to listeners.
- Close `subConn`, then close `pubConn`.
- Shut down the owned embedded server.

The current single `closedCh` field in `Pubsub`
(`coderd/x/nats/pubsub.go:483-528`) is tied to today's single connection.
It can stay single, but now it represents the joined closed state of both
`pubConn` and `subConn` (signal once both `ClosedHandler`s have fired). No
per-subscription `connWG` is needed because no per-subscription connection
exists.

## 5. Slow-listener and shared-conn backpressure

This section is the most important honesty check. TCP loopback plus
client-side `PendingLimits` reduces blast radius compared to the prior
`net.Pipe` design and compared to the per-sub-conn design's goroutine
overhead, but it does not eliminate the slow-listener risk.

### What TCP loopback buys us

- The previous failure mode was N subscriptions multiplexed through one
  unbuffered `net.Pipe`. One slow listener stalled the pipe and therefore
  stalled every other subscription sharing it.
- TCP loopback has a real kernel socket buffer in both directions. A
  transient slow listener can have its client-side pending queue fill
  without immediately stalling the server's writer: the server can keep
  writing into the kernel buffer until that buffer also fills.
- nats.go uses one async delivery goroutine per `*Subscription` by default.
  Per-sub callbacks do not directly contend with each other for CPU
  scheduling at the callback layer.

### What client-side `PendingLimits` buys us

- Each `*nats.Subscription` has its own pending-message and pending-bytes
  buffer on the client side, sized by `SetPendingLimits`.
- If subscription A's callback blocks, only A's pending queue fills. Once
  it exceeds the configured limit, nats.go drops messages for A and fires
  the async error handler with `ErrSlowConsumer` for A only. Subscriptions
  B and C, multiplexed on the same `subConn`, keep receiving.
- This is the idiomatic NATS model: one conn per process, many subs
  multiplexed, slow-consumer detection per sub.

### Residual risk, step by step

All subscriptions share one TCP read loop on `subConn`. nats.go's reader
goroutine reads framed messages off the socket and dispatches each one
into the matching subscription's pending queue. The dispatch step is
non-blocking under `PendingLimits`: an over-limit sub gets its message
dropped and an async error, not a stall. So the read loop should not stall
purely because one sub is slow.

The remaining failure path:

1. If the read loop itself stalls for any reason (a bug, a hostile build of
   nats.go, or the Go scheduler starving that goroutine under load), the
   kernel receive buffer on the client side fills.
2. TCP backpressure propagates to the server's send buffer for that one
   conn.
3. The server's per-client outbound queue fills.
4. Once that queue passes `MaxPending`, the server disconnects `subConn`
   as a slow consumer.
5. Reconnect (we keep `MaxReconnects(-1)`) brings `subConn` back, but
   every subscription on it must be re-established. During the gap,
   messages are lost.

Blast radius: reduced versus `net.Pipe` (kernel buffer gives real
breathing room) but not eliminated. If the single read loop stalls, every
subscription on `subConn` is affected.

### Listener latency contract (still required)

Listeners must return quickly. Concretely:

- Target under 10 ms per callback under normal operation.
- Anything longer must enqueue or hand off to another goroutine and
  return.
- Synchronous database work in a listener is a known risk.

Known offender, listed as follow-up and not in scope here:

- `coderd/workspaceupdates.go:109` subscribes
  `wspubsub.HandleWorkspaceEvent(s.handleEvent)`.
- `handleEvent` holds a mutex and calls `GetWorkspacesAndAgentsByOwnerID`
  inside the callback (`coderd/workspaceupdates.go:60-83`).
- That listener must be refactored to hand off DB work to a worker
  before production rollout of this design.

### Wrapper-level mitigation (exactly one)

- Default `New` to generous per-subscription pending limits:
  `PendingLimits{Msgs: -1, Bytes: 512 * 1024 * 1024}` (defined in
  `coderd/x/nats/options.go`), unless the caller overrides via
  `Options.PendingLimits`.
- Apply those limits to every subscription via `SetPendingLimits` in the
  subscribe path. These limits now apply to the single shared `subConn`
  rather than per-sub conns, which is the correct place: they bound
  per-sub client-side pending queues directly.

Do not add internal wrapper buffering, worker pools, asynchronous listener
dispatch, or fall back to `net.Pipe`. Those would either hide the
slow-listener bug or reintroduce a worse failure mode.

## 6. Worked examples: recomputing the previously failing benches

Common assumptions:

- Payload is 512 KiB.
- NATS default per-client `MaxPending` is 64 MiB.
- 64 MiB / 512 KiB = 128 messages of headroom per client connection.
- All local subscriptions share `subConn`, so the 64 MiB budget is the
  per-publish-fan-out budget for the whole wrapper's subscriber side, not
  per-subscription. However, the server's local fan-out loop drains
  `subConn`'s outbound as fast as the kernel buffer accepts, and the
  kernel buffer for TCP loopback (typically several MiB by default,
  tunable via `wmem`/`rmem`) absorbs bursts before the per-client queue
  saturates.
- The numbers below are pre-rerun estimates. The implementation task will
  capture actuals.

### `BenchmarkPubsub/coder/cluster10/subj1/512KiB`

- 100 subscriptions per replica on one subject; all multiplexed on
  `subConn`.
- Each publish causes the server to enqueue 100 outbound copies onto
  `subConn`'s send queue. The server's fan-out loop here is serial per
  publish; that bottleneck is unchanged from the prior design.
- The kernel TCP buffer plus the per-client `MaxPending` of 64 MiB absorb
  the burst. 100 * 512 KiB = 50 MiB per publish, just under the 64 MiB
  budget for a single in-flight publish.
- Steady-state delivery completeness should reach 100 percent as long as
  listener callbacks keep up.

### `BenchmarkPubsubHotSubjectConcentrated/coder/standalone/subs1000/512KiB`

- Standalone has 1000 subscriptions on `subConn`.
- One publish causes 1000 * 512 KiB = 500 MiB of outbound on `subConn`.
  That exceeds the default 64 MiB `MaxPending` for a single in-flight
  publish.
- The server's local fan-out loop blocks on the per-client send queue
  before completing the fan-out, which throttles the publisher naturally
  through the publish path on `pubConn`. Because `pubConn` is separate,
  this throttling does not block the subscriber read loop on `subConn`.
- Delivery completeness should reach 100 percent at lower throughput.
  The bottleneck is fan-out serialization plus the per-client outbound
  budget, both unchanged from the prior design.

### `BenchmarkPubsubHotSubjectConcentrated/coder/standalone/subs5000/512KiB`

- Standalone has 5000 subscriptions on `subConn`.
- One publish causes 5000 * 512 KiB = 2.5 GiB of outbound, far over the
  64 MiB `MaxPending` budget.
- Throughput will be limited by per-client outbound drain rate.
  Throughput (pubs/s) may be lower than a striped pool would deliver;
  delivery completeness should still reach 100 percent given listeners
  keep up.
- Per-sub overhead is roughly two goroutines plus a few KiB, versus the
  per-sub-conn design's six goroutines plus ~550 KiB, so process-wide
  memory and scheduler load are dramatically lower at this subscription
  count.

The all-100%-delivery cases should perform similarly to the per-sub-conn
design: the server's serial fan-out loop is the dominant cost there, and
that has not changed. The win is overhead and operational simplicity.

## 7. Test strategy

Existing tests in `coderd/x/nats/pubsub_test.go` (round-trip,
`SubscribeWithErr`, default echo, ordering, `NewFromConn`, idempotent
`Close`) should largely survive the structural rework.

Changes to existing tests:

- Delete `TestStandalone_NoEcho` because `NoEcho` is removed.
- Delete `coderd/x/nats/persubconn_test.go` in full. Its five tests
  (distinct-conns-per-sub, cancel-closes-conn, close-drains-all,
  slow-consumer-isolation-via-separate-conns, subscribe-latency-per-conn)
  encode the prior per-sub-conn design and are no longer meaningful.
- The ordering test should still pass: each listener still owns one NATS
  subscription with one async delivery goroutine.

New tests to add:

1. Connection count is independent of subscription count. After creating
   N subscriptions (e.g., N = 10, 100), the embedded server reports
   exactly two client connections from the wrapper (`pubConn` and
   `subConn`). Use `ns.NumClients()` or equivalent.
2. Slow-listener isolation under client-side `PendingLimits`. Three
   subscribers on the same subject:
   - A's callback blocks indefinitely.
   - B and C's callbacks return immediately.
   - Publish enough messages to exceed A's `PendingLimits`.
   - Assert that A receives `pubsub.ErrDroppedMessages` via its error
     callback.
   - Assert that B and C continue receiving messages without drops and
     that `subConn` stays connected (no disconnect, no reconnect event).
3. `Close` drains `subConn`, closes both conns, and remains idempotent
   across repeated calls.
4. Subscription creation stays fast. Assert `Subscribe` latency under a
   small bound (for example 5 ms in local test conditions). With no new
   conn per sub, this should be trivially satisfied.

Validation commands during implementation:

- `make test RUN=TestStandalone` for the narrowest first pass.
- `make test RUN=...` for the broader `coderd/x/nats` package.
- `make lint` after code changes.

## 8. Benchmark sweeps to validate after implementation

The old `-bench.connpool` flag proposal is gone. There is no pool to size,
so there is no new bench flag. `-bench.type=coder` already exercises the
wrapper and will automatically pick up the new dual-conn architecture.

Re-run the previously failing wide fan-out leaves and confirm delivery
completeness:

- `BenchmarkPubsub/coder/cluster10/subj1/512KiB`.
- `BenchmarkPubsub/coder/cluster10/subj10/512KiB`.
- `BenchmarkPubsubHotSubjectConcentrated/coder/standalone/subs1000/512KiB`.
- `BenchmarkPubsubHotSubjectConcentrated/coder/standalone/subs5000/512KiB`.

Specific caveat:

- `BenchmarkPubsubHotSubjectConcentrated/coder/standalone/subs5000/8KiB`
  previously hit a `Flush` timeout at `-benchtime>=500x`. Re-test under
  the new design. TCP loopback with kernel buffer headroom should behave
  more gracefully than `net.Pipe` here, but the result is genuinely
  uncertain until measured.

The numbers in section 6 are pre-rerun estimates. The implementation task
captures the actuals and updates this document if any expectation is
materially wrong.

Success criteria:

- Delivery completeness approaches or reaches 100 percent for the
  previously failing wide-fan-out cases.
- Throughput at very high subscription counts is bounded by server-side
  fan-out serialization and the per-client outbound budget. Accept that
  tradeoff in exchange for correctness and per-sub overhead reduction.

## 9. Out of scope and rejected alternatives

Out of scope for this design:

- Unix domain sockets as the local transport. TCP loopback is sufficient
  and avoids platform-specific socket-path management.
- JetStream.
- Public `pubsub.Pubsub` interface changes.
- Server `MaxPending` tuning as part of this design. Defaults stay; if
  measurements after rollout show the per-client budget is the binding
  constraint, tuning is a separate change.
- `NoEcho` compatibility shims, origin headers, or wrapper-instance-ID
  suppression. `NoEcho` is simply removed (section 3).
- Refactoring slow listeners such as `coderd/workspaceupdates.go:109`.
  The listener latency contract is documented here (section 5); the
  refactor is a separate change that must land before production
  rollout.
- Internal wrapper buffering, worker pools, or asynchronous listener
  dispatch inside `coderd/x/nats.Pubsub`.

Rejected alternatives, recorded so we do not relitigate:

- Per-subscription `*nats.Conn` (one fresh in-process conn per
  `Subscribe`). Rejected: roughly six goroutines plus ~550 KiB per
  subscription means ~6000 goroutines and ~550 MiB at 1000 subs/replica.
  The marginal isolation benefit over TCP loopback plus client-side
  `PendingLimits` does not justify that overhead.
- `nats.InProcessServer` over `net.Pipe` as the default transport.
  Rejected: `net.Pipe` is unbuffered. Any slow consumer stalls the
  server's writer immediately, which is the blast-radius failure mode the
  prior design was working around. TCP loopback's kernel socket buffer is
  what makes a shared `subConn` viable.
- A striped fixed connection pool, pool size option, FNV hashing of
  subjects, or live rebalancing of subscriptions across connections.
  Rejected: complexity does not pay for itself once `subConn` plus
  client-side `PendingLimits` already gives per-sub isolation. This was
  the original superseded design.
- Protocol-sniffing single-port multiplexing of the client and route
  listeners. Rejected: nats-server dispatches by listener, not by
  sniffing the CONNECT payload, and the route protocol is materially
  different from the client protocol (RS+/RS-/RMSG, route-CONNECT,
  ClusterToken auth). Collapsing them would require forking or proxying
  nats-server. Both listeners already bind to 127.0.0.1; using two
  listeners on two ports is fine.
