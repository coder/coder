# Per-subscription in-memory NATS connections for coderd/x/nats.Pubsub

This document is the recommended design for fixing wide fan-out failures in
the `coderd/x/nats` pubsub wrapper. It supersedes an earlier proposal for a
striped TCP loopback connection pool. There is no menu of alternatives here:
the recommendation is one `*nats.Conn` per local subscription, using
`nats.InProcessServer`, plus one wrapper-owned in-process publisher
connection.

The public `pubsub.Pubsub` interface (`Publish`, `Subscribe`,
`SubscribeWithErr`) does not change. Internal `coderd/x/nats.Pubsub`,
`Options`, `New`, and `NewFromConn` do.

## 1. Problem restatement

Today the wrapper concentrates all publish and subscribe traffic for a given
`coderd/x/nats.Pubsub` instance through a single `*nats.Conn`:

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
consumer.

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

The recommended fix is structural: stop multiplexing many local
subscriptions through one client connection. Give each subscription its own
in-memory client connection so the server's per-client outbound budget
applies per subscription instead of per wrapper.

## 2. Design recommendation: the per-sub-conn model

Architecture:

- `New` starts the embedded server (unchanged) and opens exactly one
  in-process publisher connection, `pubConn`, via
  `nats.Connect("", nats.InProcessServer(ns), ...)`.
- Every `Subscribe` and `SubscribeWithErr` call opens a fresh
  `*nats.Conn` with `nats.InProcessServer(p.ns)` and creates exactly one
  NATS subscription on that connection.
- Canceling the returned `pubsub.CancelFunc` closes that subscription's
  dedicated connection.
- `Close` tears down all subscription connections, then `pubConn`, then the
  embedded server.

Consequences:

- No TCP loopback in the default `New` path. No ephemeral port consumption.
- No pool size knob. No subject striping. No FNV hash. No `connIndex`
  bookkeeping.
- The server's per-client `MaxPending` becomes a per-subscription budget,
  not a shared budget across all subscriptions owned by one wrapper.
- nats.go's per-client reader goroutine count grows with subscription
  count. That is an explicit, accepted tradeoff (see section 6).

Compact shape, not a full implementation:

```go
type Pubsub struct {
    ns      *natsserver.Server
    pubConn *natsgo.Conn
    subs    map[uuid.UUID]*subscription
    // existing wrapper metrics, peer refresh state, locks, and close state.
}

type subscription struct {
    id  uuid.UUID
    nc  *natsgo.Conn
    sub *natsgo.Subscription
    // existing context, cancel, wait group, listener, and drop counters.
}
```

`NewFromConn` is the explicit exception:

- It accepts one external `*nats.Conn` and uses that connection for both
  publish and subscribe.
- It does not get the per-subscription connection isolation benefit.
- It stays intentionally simple. Callers who choose `NewFromConn` own the
  topology and the per-client budget themselves.

Metrics stay wrapper-scoped. Existing metric cardinality is already low and
pending gauges sum over `p.subs` (`coderd/x/nats/metrics.go:38-108`,
`coderd/x/nats/metrics.go:143-170`). Do not add per-connection labels: that
would explode cardinality with the subscription count.

## 3. `NoEcho` removal

`Options.NoEcho` is removed rather than emulated. With per-subscription
connections, no caller in this repository depends on it.

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

The previous plan's "wrapper-instance-ID header" suppression workaround is
explicitly rejected. With `NoEcho` removed, there is nothing to suppress
and no compatibility shim is needed.

## 4. Lifecycle

### Construction

- Start the embedded server as today.
- Build `pubConn` via `natsgo.Connect("", natsgo.InProcessServer(ns), ...)`.
- Do not create any subscriber connections at construction time.
- Peer refresh and route clustering behavior are unrelated to local client
  connections and remain unchanged.

### Subscribe / SubscribeWithErr

- Validate closed state and map the legacy event subject through
  `LegacyEventSubject`, as today.
- Open a fresh in-process `*nats.Conn` for this subscription.
- Install per-connection handlers (`ErrorHandler`, `DisconnectErrHandler`,
  `ReconnectHandler`, `ClosedHandler`) as closures over `p` so wrapper-level
  counters and slow-consumer handling continue to work.
- Create exactly one NATS subscription on that connection.
- Apply per-subscription pending limits via `Subscription.SetPendingLimits`
  (this call already exists in the current path,
  `coderd/x/nats/pubsub.go:354-360`).
- Register a `subscription{id, nc, sub, ...}` in `p.subs` and
  `p.subsByNATS`.
- Start the existing per-subscription drain goroutine (no change to
  ordering: still one NATS subscription and one drainer per listener).

### Unsubscribe

- Cancel the subscription context.
- `Unsubscribe` (or `Drain`, depending on whether in-flight delivery should
  be allowed to complete; match current semantics) the NATS subscription.
- Wait for the subscription goroutine via the existing wait-group pattern.
- Unregister from `p.subs` and `p.subsByNATS`.
- Close that subscription's dedicated connection.

### Close

- Mark the wrapper closed (idempotent).
- Cancel and close every active subscription, including each subscription's
  connection.
- Close or drain `pubConn`.
- Shut down the owned embedded server.

The current single `closedCh` field in `Pubsub`
(`coderd/x/nats/pubsub.go:483-528`) is tied to today's single connection
and cannot represent many subscription connections. Replace it with either
per-subscription closed signaling stored on `subscription`, or a wrapper
`sync.WaitGroup` that tracks all subscription goroutines plus the publisher
connection's closed handler.

## 5. Slow-listener and `net.Pipe` backpressure

This section is the most important honesty check. The previous code avoided
`nats.InProcessServer` because `net.Pipe` is unbuffered and a slow consumer
under heavy fan-out could stall the server's writer. That reasoning is
documented in `coderd/x/nats/server.go:71-82`. The new design changes the
shape of that risk but does not eliminate it.

### Why the old `InProcessServer` problem changes shape

- The previous failure mode was N subscriptions multiplexed through one
  unbuffered pipe. One slow listener stalled the pipe and therefore stalled
  every other subscription sharing it.
- With one connection per subscription, only one subscription's messages
  flow through any given `net.Pipe`. A slow listener stalls its own pipe
  and its own server-side outbound queue. Other subscriptions are
  unaffected because they sit on different pipes.
- The wide fan-out concentration case that previously failed is no longer
  pipe-bound on a single shared pipe.

### Residual risk, step by step

1. nats.go's per-connection reader goroutine reads framed messages from the
   pipe and enqueues into the subscription's pending channel.
2. The wrapper drain goroutine calls `NextMsgWithContext` and invokes the
   listener (`coderd/x/nats/pubsub.go:408-429`).
3. If the listener is slow, the pending channel fills.
4. Once the reader cannot enqueue, it stops draining the pipe.
5. With `net.Pipe`, there is no kernel buffer slack, so the server's write
   into that pipe blocks immediately.
6. The server applies its `write_deadline` and eventually disconnects the
   client as a slow consumer. The wrapper observes this via
   `handleSlowConsumer` (`coderd/x/nats/pubsub.go:432-481`), which already
   reports `pubsub.ErrDroppedMessages` for that subscription.

The blast radius is the one slow subscription, not the whole wrapper. That
is the design goal.

### Wrapper-level mitigation (exactly one)

- Default `New` to generous per-subscription pending limits, for example
  `PendingLimits{Msgs: -1, Bytes: 512 * 1024 * 1024}`, unless the caller
  overrides via `Options.PendingLimits`.
- Apply those limits to every subscription via `SetPendingLimits` in the
  subscribe path.

Do not add internal wrapper buffering, worker pools, asynchronous listener
dispatch, or a TCP fallback. Those would either hide the slow-listener bug
or reintroduce the per-client concentration problem this design is
removing.

### Listener latency contract

Listeners must return quickly. Concretely:

- Target under 10 ms per callback under normal operation.
- Anything longer must enqueue or hand off to another goroutine and return.
- Synchronous database work in a listener is a known risk.

Known offender, listed as follow-up and not in scope here:

- `coderd/workspaceupdates.go:109` subscribes
  `wspubsub.HandleWorkspaceEvent(s.handleEvent)`.
- `handleEvent` holds a mutex and calls `GetWorkspacesAndAgentsByOwnerID`
  inside the callback (`coderd/workspaceupdates.go:60-83`).
- That listener should be refactored to hand off DB work to a worker, but
  that is a separate change.

## 6. Worked examples: recomputing the previously failing benches

Common assumptions:

- Payload is 512 KiB.
- NATS default per-client `MaxPending` is 64 MiB.
- 64 MiB / 512 KiB = 128 messages of headroom per client connection.
- In the new design, each local subscription is its own client connection,
  so each subscription gets that 128-message budget independently.

### `BenchmarkPubsub/coder/cluster10/subj1/512KiB`

- 100 subscriptions per replica on one subject means 100 in-process client
  connections per replica.
- Each connection carries messages for exactly one subscription.
- If a subscriber is briefly slow, its server-side per-client outbound
  queue sees one 512 KiB copy per publish for that one subscription.
- The per-client `MaxPending` budget tolerates roughly 128 pending publishes
  before disconnect.
- The previous wide-fan-out concentration case is trivial under this
  per-client budget. Delivery completeness should reach 100 percent.

### `BenchmarkPubsubHotSubjectConcentrated/coder/standalone/subs1000/512KiB`

- Standalone has 1000 in-process subscription connections.
- Each connection gets at most one 512 KiB message copy per publish.
- Per-client outbound is 512 KiB per publish, far below 64 MiB.
- Expected delivery completeness is 100 percent unless a listener is slow
  or `net.Pipe` synchronous backpressure dominates.

### `BenchmarkPubsubHotSubjectConcentrated/coder/standalone/subs5000/512KiB`

- Standalone has 5000 in-process subscription connections.
- Per-client budget is still favorable, one subscription per connection.
- The bottleneck shifts to: server CPU performing serial fan-out writes to
  5000 pipes, Go scheduler overhead for 5000 client reader goroutines, and
  callback drain speed.
- Be candid: throughput (pubs/s) may be lower than a pooled TCP design
  would deliver. Delivery completeness should improve to 100 percent as
  long as listeners keep up and pending limits absorb transient spikes.

## 7. Test strategy

Existing tests in `coderd/x/nats/pubsub_test.go` (round-trip,
`SubscribeWithErr`, default echo, ordering, `NewFromConn`, idempotent
`Close`) should largely survive the structural rework.

Changes to existing tests:

- Delete `TestStandalone_NoEcho` because `NoEcho` is removed.
- The ordering test should still pass: each listener still owns one NATS
  subscription and one drain goroutine.

New tests to add:

1. `Subscribe` creates a fresh connection per subscription. Verify via
   white-box state (e.g., distinct `subscription.nc` pointers across two
   subscriptions on the same wrapper), or via server connection count if
   that is reliable in tests.
2. Canceling a subscription closes that subscription's connection (assert
   on `nc.IsClosed()` after `cancel()` returns).
3. `Close` closes all subscription connections plus `pubConn` and remains
   idempotent across repeated calls.
4. A slow consumer on one subscription does not impact another
   subscription. Two subscribers on the same subject: one blocks; the
   other continues receiving. The slow one may receive
   `pubsub.ErrDroppedMessages` via its error callback; the healthy one
   must keep receiving without drops.
5. Subscription creation stays fast. Assert `Subscribe` latency under a
   small bound (for example 5 ms in local test conditions). This guards
   against accidentally reintroducing TCP setup or another slow path.

Validation commands during implementation:

- `make test RUN=TestStandalone` for the narrowest first pass.
- `make test RUN=...` for the broader `coderd/x/nats` package.
- `make lint` after code changes.

## 8. Benchmark sweeps to validate after implementation

The old `-bench.connpool` flag proposal is gone. There is no pool to size,
so there is no new bench flag. `-bench.type=coder` already exercises the
wrapper and will automatically pick up the new per-subscription
architecture.

Re-run the previously failing wide fan-out leaves and confirm delivery
completeness:

- `BenchmarkPubsub/coder/cluster10/subj1/512KiB`.
- `BenchmarkPubsub/coder/cluster10/subj10/512KiB`.
- `BenchmarkPubsubHotSubjectConcentrated/coder/standalone/subs1000/512KiB`.
- `BenchmarkPubsubHotSubjectConcentrated/coder/standalone/subs5000/512KiB`.

Specific caveat:

- `BenchmarkPubsubHotSubjectConcentrated/coder/standalone/subs5000/8KiB`
  previously hit a `Flush` timeout at `-benchtime>=500x`. Re-test it under
  the new design. High publisher rate combined with many simultaneous
  `net.Pipe` drains may interact differently from the TCP loopback path,
  and the result is genuinely uncertain.

Success criteria:

- Delivery completeness approaches or reaches 100 percent for the
  previously failing wide-fan-out cases.
- Throughput may drop at very high subscription counts because the server
  still serializes fan-out work and the runtime schedules thousands of
  reader goroutines. Accept that tradeoff in exchange for correctness.

## 9. Out of scope

- TCP loopback or Unix domain sockets as alternative default transports.
  The recommendation is `nats.InProcessServer` over in-memory `net.Pipe`.
- A striped fixed connection pool, pool size option, FNV hashing of
  subjects, or live rebalancing of subscriptions across connections.
- Refactoring slow listeners such as `coderd/workspaceupdates.go:109`.
  Document the listener latency contract here; refactor separately.
- Internal wrapper buffering, worker pools, or asynchronous listener
  dispatch inside `coderd/x/nats.Pubsub`.
- JetStream.
- Public `pubsub.Pubsub` interface changes.
- Server `MaxPending` tuning as part of this design.
- `NoEcho` compatibility shims, origin headers, or wrapper-instance-ID
  suppression.
