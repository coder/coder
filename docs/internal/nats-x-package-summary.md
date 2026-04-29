# Embedded NATS Pubsub: v1 Implementation Summary

Status: v1 complete, experimental, not wired into coderd
Package: `coderd/x/nats`
Plan: see `docs/internal/nats-pubsub-research-and-plan.md`

## What was built

The `coderd/x/nats` package implements `coderd/database/pubsub.Pubsub`
on top of an embedded NATS server (github.com/nats-io/nats-server)
plus a colocated client (github.com/nats-io/nats.go) connected
in-process. The package is a drop-in replacement candidate for the
Postgres `LISTEN/NOTIFY` backend, but it is intentionally not wired
into coderd in v1. It lives under `coderd/x/` so the API can move
freely.

A single `*Pubsub` value owns its embedded server and client and
shuts both down on `Close`. `NewFromConn` wraps an externally owned
`*nats.Conn` for tests and embedding scenarios.

## Modes

- **Standalone**: `Options.PeerProvider` is nil or returns no peers.
  The embedded server runs as a single node with no advertised
  routes. This is the default for tests and unwired usage.
- **Clustered**: `PeerProvider` returns one or more peers. Replicas
  share `ClusterToken` and a pinned `RoutePoolSize`. The peer set is
  read once at startup; v1 does not re-read it.

## Public API surface

The package exposes a small surface intentionally:

- `New(ctx, logger, Options) (*Pubsub, error)` and
  `NewFromConn(logger, *nats.Conn) (*Pubsub, error)` constructors.
- `*Pubsub` satisfies `pubsub.Pubsub` (`Publish`, `Subscribe`,
  `SubscribeWithErr`, `Close`) and `prometheus.Collector`.
- `Options` covers server identity, cluster setup, route TLS,
  publish mode, drain timeout, pending limits, echo, and reconnect
  knobs. See `options.go` for field-level documentation.
- `Peer`, `PeerProvider`, and `StaticPeerProvider` for cluster
  bootstrap.
- `LegacyEventSubject` for the legacy event to NATS subject mapping.

## Key behaviors

- **Subject mapping**: legacy `event:foo:bar` becomes
  `coder.v1.pubsub.event.foo.bar`. See `subject.go`.
- **Slow consumers**: per-subscription `nats.ErrSlowConsumer` is
  translated to `pubsub.ErrDroppedMessages` exactly once per delta in
  the dropped counter. Reconnect alone does not synthesize a drop
  callback.
- **Echo**: enabled by default for parity with the Postgres
  implementation; opt out with `Options.NoEcho`.
- **Publish modes**: `PublishModeFlush` (default) flushes before
  returning, bounded by `PublishFlushTimeout`. `PublishModeBuffered`
  returns once the message is enqueued in the client buffer.
- **Cluster auth**: `ClusterToken` is required when peers are
  configured; `*tls.Config` for routes is optional, with v1 leaving
  certificate provisioning to callers.
- **Metrics cardinality**: counters and gauges never use subject,
  event, UUID, or peer labels. Bounded labels only (e.g. `success`,
  `size`).

## Non-goals for v1

- No JetStream.
- No NKeys or JWT auth.
- No leafnodes.
- No dynamic peer reconfiguration.
- No production certificate provisioning.
- No migration of existing pubsub call sites.

## File map

- `pubsub.go`: `*Pubsub` lifecycle, Publish/Subscribe, slow-consumer
  accounting, Close/Drain.
- `server.go`: embedded NATS server bootstrap, in-process client
  dial.
- `cluster.go`: peer types, normalization, cluster route options.
- `options.go`: `Options`, `PendingLimits`, `PublishMode`, defaults.
- `subject.go`: legacy event to NATS subject mapping.
- `metrics.go`: Prometheus collector implementation.
- `doc.go`: package-level documentation.
- Tests: `pubsub_test.go`, `cluster_test.go`, `cluster_tls_test.go`,
  `slow_consumer_test.go`, `metrics_test.go`, `subject_test.go`,
  `stress_test.go`, plus `testutil_test.go` for shared helpers.

## Verification

- `go test -race -count=1 ./coderd/x/nats/...` passes locally.
- `go vet ./coderd/x/nats/...` is clean.
- Stress tests (`stress_test.go`) exercise concurrent
  subscribe/publish/cancel and a 100-subscriber single-event fanout.
  Combined runtime is well under 60 seconds.

## Next steps (not part of v1)

- Wire `*Pubsub` into coderd behind a feature flag and a peer
  discovery integration.
- Decide on a route TLS provisioning story (likely shared with the
  existing coder-to-coder TLS plumbing).
- Revisit dynamic peer membership and JetStream once v1 has soaked.
