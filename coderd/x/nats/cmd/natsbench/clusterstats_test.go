package main

import (
	"strings"
	"testing"
)

func TestRenderClusterStatsAggregatesAndDetailsPerReplica(t *testing.T) {
	t.Parallel()
	snaps := []replicaStatsSnapshot{
		{Replica: 0, Available: true, NumClients: 4, NumRoutes: 9, NumSubscriptions: 30, NumSlowConsumers: 2, NumStaleConnections: 0, MaxPending: 1 << 30},
		{Replica: 1, Available: true, NumClients: 6, NumRoutes: 9, NumSubscriptions: 28, NumSlowConsumers: 0, NumStaleConnections: 1, MaxPending: 1 << 30},
	}
	got := renderClusterStats(snaps)
	for _, want := range []string{
		"slow_consumers=2",
		"stale=1",
		"clients=10",
		"routes=18",
		"subs=58",
		"r0{slow=2",
		"r1{slow=0",
		"max_pending=1.00 GiB",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("renderClusterStats output missing %q\nfull output: %s", want, got)
		}
	}
}

func TestRenderClusterStatsHandlesUnavailable(t *testing.T) {
	t.Parallel()
	snaps := []replicaStatsSnapshot{
		{Replica: 0, Available: true, NumClients: 1},
		{Replica: 1, Available: false},
	}
	got := renderClusterStats(snaps)
	if !strings.Contains(got, "unavailable_replicas=[1]") {
		t.Errorf("missing unavailable_replicas tag: %s", got)
	}
	if !strings.Contains(got, "r1=unavailable") {
		t.Errorf("missing r1=unavailable detail: %s", got)
	}
}

func TestRenderClusterStatsAllUnavailable(t *testing.T) {
	t.Parallel()
	snaps := []replicaStatsSnapshot{
		{Replica: 0, Available: false},
		{Replica: 1, Available: false},
	}
	got := renderClusterStats(snaps)
	if !strings.Contains(got, "unavailable for all 2 replicas") {
		t.Errorf("expected all-unavailable note, got %q", got)
	}
}

func TestRenderClusterStatsEmpty(t *testing.T) {
	t.Parallel()
	got := renderClusterStats(nil)
	if !strings.Contains(got, "no replicas") {
		t.Errorf("expected 'no replicas' note, got %q", got)
	}
}
