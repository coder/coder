package nats

// ServerStats is a compact snapshot of the embedded nats-server's
// connection-level counters. It is exposed for diagnostics in
// benchmarks and tests that need to surface slow-consumer disconnects
// or route convergence at the moment of a failure, without taking a
// dependency on the full nats-server Server type.
//
// Fields mirror the corresponding *natsserver.Server accessors:
//   - NumClients:       active client connections.
//   - NumRoutes:        active cluster route connections (peers).
//   - NumSubscriptions: total subscriptions across all clients.
//   - NumSlowConsumers: cumulative slow-consumer disconnects (clients
//     and routes combined). A non-zero value at the end of a benchmark
//     usually means a connection was dropped for exceeding MaxPending.
//   - NumSlowConsumersClients: slow-consumer disconnects observed on
//     plain client connections.
//   - NumSlowConsumersRoutes: slow-consumer disconnects observed on
//     cluster route connections.
//   - NumStaleConnections: cumulative stale-connection disconnects.
//   - MaxPending: the per-client outbound pending byte budget
//     configured on this server (mirrors Options.MaxPending after
//     defaulting).
type ServerStats struct {
	NumClients              int
	NumRoutes               int
	NumSubscriptions        uint32
	NumSlowConsumers        int64
	NumSlowConsumersClients uint64
	NumSlowConsumersRoutes  uint64
	NumStaleConnections     int64
	MaxPending              int64
}

// ServerStats returns a snapshot of the embedded nats-server counters
// alongside ok=true. If p is nil or has no embedded server attached it
// returns the zero value and ok=false so callers can skip diagnostics
// rather than special-casing nil.
//
// The snapshot is intended for benchmark and test diagnostics; it is
// not on the hot path and is safe to call concurrently with publish
// and subscribe traffic because every read is delegated to a
// *natsserver.Server accessor that uses its own internal locking.
func (p *Pubsub) ServerStats() (ServerStats, bool) {
	if p == nil || p.ns == nil {
		return ServerStats{}, false
	}
	maxPending := p.opts.MaxPending
	switch {
	case maxPending == 0:
		maxPending = DefaultMaxPending
	case maxPending < 0:
		// Negative means "use nats-server default"; we cannot read
		// the effective value back from natsserver without a Varz
		// roundtrip, so report zero to indicate "server default".
		maxPending = 0
	}
	return ServerStats{
		NumClients:              p.ns.NumClients(),
		NumRoutes:               p.ns.NumRoutes(),
		NumSubscriptions:        p.ns.NumSubscriptions(),
		NumSlowConsumers:        p.ns.NumSlowConsumers(),
		NumSlowConsumersClients: p.ns.NumSlowConsumersClients(),
		NumSlowConsumersRoutes:  p.ns.NumSlowConsumersRoutes(),
		NumStaleConnections:     p.ns.NumStaleConnections(),
		MaxPending:              maxPending,
	}, true
}
