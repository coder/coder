// Package nats is an experimental embedded NATS-backed implementation
// of coderd/database/pubsub.Pubsub. It is not wired into coderd in v1
// and lives under coderd/x/ so its API can change without notice.
//
// # Status
//
// Experimental. Nothing in this package is currently imported by
// production code. Do not depend on its exported surface remaining
// backwards compatible. The v1 iteration covers standalone and
// clustered modes, TLS for routes, and slow-consumer accounting.
// Migrating existing call sites is an explicit non-goal of v1.
//
// # What it provides
//
// New starts an embedded NATS server (github.com/nats-io/nats-server)
// and a colocated client (github.com/nats-io/nats.go) connected
// in-process to that server. The returned *Pubsub satisfies
// pubsub.Pubsub. Owned servers and connections are shut down by
// Close. NewFromConn wraps an externally owned connection without
// taking ownership of it.
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
// mapping rules and validation.
//
// # Slow-consumer behavior
//
// When the NATS client signals nats.ErrSlowConsumer for a particular
// subscription, that subscription's listener receives a single
// callback with err set to pubsub.ErrDroppedMessages, matching the
// existing pubsub semantics. Reconnect events alone do not synthesize
// dropped-message callbacks; only NATS-reported drops do.
//
// # Echo
//
// Self-published messages are always delivered to local subscribers.
// Publishes flow on different connection(s) (the publisher pool) than
// subscribes (the subscriber pool), so they cross the server boundary
// and are routed back to local subscribers like any other message.
// Callers that need de-duplication should tag publishes at a higher
// layer.
//
// # Connection model
//
// New starts one embedded NATS server and opens two pools of
// TCP-loopback *nats.Conns to it: one or more publisher conns
// (configurable via Options.PublishConns, default 1) and one or more
// subscriber conns (configurable via Options.SubscribeConns, default
// 1). Publish selects a publisher conn by a stable hash of the
// resolved subject so same-subject publishes always target the same
// connection and preserve per-subject ordering. Each shared
// *nats.Subscription created by Subscribe / SubscribeWithErr is
// likewise pinned to a subscriber conn by a stable hash of its
// subject, so all local subscribers for a subject coalesce onto one
// shared NATS subscription on one conn. Per-subscription slow-consumer
// isolation comes from client-side PendingLimits on each
// *nats.Subscription rather than from separate connections. NewFromConn
// is the explicit exception: it reuses the caller-provided *nats.Conn
// for both publish and subscribe and does not get the publish/subscribe
// split or either pool.
//
// # Publish semantics
//
// Publish is a thin passthrough to nats.go's nc.Publish: the message
// is enqueued into the connection's outbound buffer and the call
// returns. nats.go auto-flushes when the buffer fills (default
// WriteBufferSize 32 KiB) and on a short interval; callers that need
// stronger "server has acknowledged" semantics should drive flushing
// at a higher layer. Options.WriteBufferSize raises that per-conn
// flush threshold for every wrapper-owned client connection (both
// pools); zero keeps the nats.go default. NewFromConn does not apply
// WriteBufferSize: it reuses the caller's connection without
// reconfiguring it.
//
// # Cluster auth and TLS
//
// Options.ClusterToken is required whenever PeerProvider returns any
// peers, and must match across replicas. Options.ClusterTLSConfig is
// optional. When non-nil it is applied to route connections; when
// nil, routes are plaintext and protected only by ClusterToken. v1
// does not provision certificates; supply a *tls.Config built from
// material managed elsewhere.
package nats
