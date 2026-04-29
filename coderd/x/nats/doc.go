// Package nats is an experimental embedded NATS-backed implementation
// of coderd/database/pubsub.Pubsub. It is not wired into coderd in v1
// and lives under coderd/x/ so its API can change without notice.
//
// # Status
//
// Experimental. Nothing in this package is currently imported by
// production code. Do not depend on its exported surface remaining
// backwards compatible. The v1 iteration covers standalone and
// clustered modes, TLS for routes, slow-consumer accounting, and
// Prometheus metrics. Migrating existing call sites is an explicit
// non-goal of v1.
//
// # What it provides
//
// New starts an embedded NATS server (github.com/nats-io/nats-server)
// and a colocated client (github.com/nats-io/nats.go) connected
// in-process to that server. The returned *Pubsub satisfies
// pubsub.Pubsub and prometheus.Collector. Owned servers and
// connections are shut down by Close. NewFromConn wraps an externally
// owned connection without taking ownership of it.
//
// # Modes
//
// Standalone mode runs when Options.PeerProvider is nil or returns no
// peers. Routes are not advertised and the server runs as a single
// node. This is the default for tests and for non-wired package
// usage.
//
// Clustered mode runs when PeerProvider returns one or more peers.
// All replicas must share the same Options.ClusterToken and the same
// Options.RoutePoolSize, which is pinned by this package to keep
// route fan-in deterministic. The PeerProvider snapshot is read once
// at startup; v1 does not support dynamic peer updates.
//
// # Non-goals
//
// JetStream, NKeys/JWT auth, leafnodes, dynamic peer reconfiguration,
// production certificate provisioning, and migration of existing
// pubsub call sites are all out of scope for v1.
//
// # Subject mapping
//
// Legacy event names of the form "event:foo:bar" are mapped to
// dot-separated NATS subjects under a fixed prefix, for example
// "coder.v1.pubsub.event.foo.bar". See subject.go for the full
// mapping rules and validation. The mapping is internal and never
// surfaces in metrics labels.
//
// # Slow-consumer behavior
//
// When the NATS client signals nats.ErrSlowConsumer for a particular
// subscription, that subscription's listener receives a single
// callback with err set to pubsub.ErrDroppedMessages, matching the
// existing pubsub semantics. Reconnect events alone do not synthesize
// dropped-message callbacks; only NATS-reported drops do. The cluster
// connection's reconnect and disconnect counters are exported as
// metrics.
//
// # Echo
//
// Echo is enabled by default so that a single-process publisher and
// subscriber observe each other's messages, preserving parity with
// the legacy Postgres-backed pubsub. Set Options.NoEcho to true to
// drop self-published messages on the local connection.
//
// # Publish modes
//
// PublishModeFlush (the default) flushes the client buffer up to
// Options.PublishFlushTimeout before returning, giving callers
// strong "the server has it" semantics. PublishModeBuffered returns
// once the message is enqueued in the client's outbound buffer,
// trading durability guarantees for throughput.
//
// # Cluster auth and TLS
//
// Options.ClusterToken is required whenever PeerProvider returns any
// peers, and must match across replicas. Options.ClusterTLSConfig is
// optional. When non-nil it is applied to route connections; when
// nil, routes are plaintext and protected only by ClusterToken. v1
// does not provision certificates; supply a *tls.Config built from
// material managed elsewhere.
//
// # Metrics cardinality
//
// Metrics in this package never label by subject, event name, UUID,
// or peer identity. Counters that need a dimension use bounded label
// sets (for example success "true"/"false", or size "normal"/
// "colossal"). Gauges aggregate across the server and are emitted
// without per-subject labels. This contract keeps the package safe
// to scrape in clusters with thousands of distinct event names.
package nats
