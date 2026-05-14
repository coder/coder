package main

import (
	"fmt"
	"strings"

	natsserver "github.com/nats-io/nats-server/v2/server"

	codernats "github.com/coder/coder/v2/coderd/x/nats"
)

// replicaStatsSnapshot is a compact view of a single replica's
// server-side counters at the moment a benchmark hits a phase timeout.
// Used by both the native and coder cluster runners; the coder
// variant fills in MaxPending from the wrapper (the underlying
// nats-server does not expose its configured MaxPending without a
// Varz roundtrip).
type replicaStatsSnapshot struct {
	Replica             int
	Name                string
	NumClients          int
	NumRoutes           int
	NumSubscriptions    uint32
	NumSlowConsumers    int64
	NumStaleConnections int64
	MaxPending          int64
	// Available is false when the replica's stats could not be read
	// (e.g. a Pubsub without an embedded server). When false the
	// other fields are zero-valued and the renderer notes the gap so
	// users do not mistake it for "everything is clean".
	Available bool
}

// nativeClusterStats snapshots the server-side counters for every
// replica in a native (bare nats-server) cluster.
func nativeClusterStats(servers []*natsserver.Server) []replicaStatsSnapshot {
	out := make([]replicaStatsSnapshot, len(servers))
	for i, ns := range servers {
		if ns == nil {
			out[i] = replicaStatsSnapshot{Replica: i}
			continue
		}
		out[i] = replicaStatsSnapshot{
			Replica:             i,
			Name:                ns.Name(),
			NumClients:          ns.NumClients(),
			NumRoutes:           ns.NumRoutes(),
			NumSubscriptions:    ns.NumSubscriptions(),
			NumSlowConsumers:    ns.NumSlowConsumers(),
			NumStaleConnections: ns.NumStaleConnections(),
			// nats-server does not expose its configured MaxPending
			// via *Server accessors. Leaving zero is intentional;
			// the cluster header already prints the effective value.
			MaxPending: 0,
			Available:  true,
		}
	}
	return out
}

// coderClusterStats snapshots the server-side counters for every
// replica in a Coder Pubsub cluster via Pubsub.ServerStats.
func coderClusterStats(pubsubs []*codernats.Pubsub) []replicaStatsSnapshot {
	out := make([]replicaStatsSnapshot, len(pubsubs))
	for i, p := range pubsubs {
		s, ok := p.ServerStats()
		if !ok {
			out[i] = replicaStatsSnapshot{Replica: i}
			continue
		}
		out[i] = replicaStatsSnapshot{
			Replica:             i,
			NumClients:          s.NumClients,
			NumRoutes:           s.NumRoutes,
			NumSubscriptions:    s.NumSubscriptions,
			NumSlowConsumers:    s.NumSlowConsumers,
			NumStaleConnections: s.NumStaleConnections,
			MaxPending:          s.MaxPending,
			Available:           true,
		}
	}
	return out
}

// renderClusterStats renders a single-line summary suitable for
// inclusion in a timeout error message. Always emits an aggregate
// "slow_consumers=N stale=N" up front so the operator sees the
// headline slow-consumer count without having to parse the per-replica
// breakdown. The per-replica detail is appended in stable replica
// order so two runs can be diffed.
func renderClusterStats(snaps []replicaStatsSnapshot) string {
	if len(snaps) == 0 {
		return "server-stats: no replicas"
	}
	var (
		anyAvailable          bool
		totalSlowConsumers    int64
		totalStale            int64
		totalClients          int
		totalRoutes           int
		totalSubscriptions    uint32
		unavailableReplicaIDs []int
	)
	for _, s := range snaps {
		if !s.Available {
			unavailableReplicaIDs = append(unavailableReplicaIDs, s.Replica)
			continue
		}
		anyAvailable = true
		totalSlowConsumers += s.NumSlowConsumers
		totalStale += s.NumStaleConnections
		totalClients += s.NumClients
		totalRoutes += s.NumRoutes
		totalSubscriptions += s.NumSubscriptions
	}
	if !anyAvailable {
		return fmt.Sprintf("server-stats: unavailable for all %d replicas (no embedded server accessor)", len(snaps))
	}
	var b strings.Builder
	// strings.Builder.WriteString and fmt.Fprintf into a *strings.Builder
	// never return non-nil errors; the discards quiet revive's
	// unhandled-error lint without obscuring real I/O failures.
	_, _ = fmt.Fprintf(&b, "server-stats: slow_consumers=%d stale=%d clients=%d routes=%d subs=%d",
		totalSlowConsumers, totalStale, totalClients, totalRoutes, totalSubscriptions)
	if len(unavailableReplicaIDs) > 0 {
		_, _ = fmt.Fprintf(&b, " unavailable_replicas=%v", unavailableReplicaIDs)
	}
	_, _ = b.WriteString(" [")
	for i, s := range snaps {
		if i > 0 {
			_, _ = b.WriteString(", ")
		}
		if !s.Available {
			_, _ = fmt.Fprintf(&b, "r%d=unavailable", s.Replica)
			continue
		}
		_, _ = fmt.Fprintf(&b, "r%d{slow=%d,stale=%d,clients=%d,routes=%d,subs=%d",
			s.Replica, s.NumSlowConsumers, s.NumStaleConnections,
			s.NumClients, s.NumRoutes, s.NumSubscriptions)
		if s.MaxPending > 0 {
			_, _ = fmt.Fprintf(&b, ",max_pending=%s", humanBytesAbs(s.MaxPending))
		}
		_, _ = b.WriteString("}")
	}
	_, _ = b.WriteString("]")
	return b.String()
}

// nativeClusterStatsDescription is a convenience wrapper used by
// runNativeClusterSymmetric (and any future native cluster runner)
// to produce the timeout diagnostic tail in one call.
func nativeClusterStatsDescription(servers []*natsserver.Server) string {
	return renderClusterStats(nativeClusterStats(servers))
}

// coderClusterStatsDescription is the equivalent helper for Coder
// cluster runners.
func coderClusterStatsDescription(pubsubs []*codernats.Pubsub) string {
	return renderClusterStats(coderClusterStats(pubsubs))
}
