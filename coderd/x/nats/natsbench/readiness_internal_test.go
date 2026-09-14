package main

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestProbeRoundTrip(t *testing.T) {
	t.Parallel()

	for _, node := range []int{0, 1, 7, 1 << 30} {
		got, ok := probeNode(probePayload(node))
		require.True(t, ok)
		require.Equal(t, node, got)
	}
}

func TestProbeNodeRejectsBenchmarkPayloads(t *testing.T) {
	t.Parallel()

	// Benchmark payloads are all zeros, at any size.
	for _, size := range []int{1, 2, 9, Payload8KB} {
		_, ok := probeNode(make([]byte, size))
		require.False(t, ok)
	}
	// The bare prefix has no node index.
	_, ok := probeNode([]byte(probePrefix))
	require.False(t, ok)
	// A trailing non-digit byte fails decoding.
	_, ok = probeNode(append(probePayload(3), 0))
	require.False(t, ok)
}

func TestProbeTracker(t *testing.T) {
	t.Parallel()

	tracker := newProbeTracker()
	required := map[int]struct{}{0: {}, 2: {}}

	require.Equal(t, []int{0, 2}, tracker.missing(required))
	tracker.observe(0)
	tracker.observe(1) // Not required; ignored by missing.
	require.Equal(t, []int{2}, tracker.missing(required))
	tracker.observe(2)
	require.Empty(t, tracker.missing(required))
	require.Empty(t, tracker.missing(nil))
}

func TestSubjectNodes(t *testing.T) {
	t.Parallel()

	pl := buildPlan(Config{
		Messages: 30, Publishers: 5, Subjects: 2, Subscribers: 3, Replicas: 3,
	})

	// subjectNodes must map each subject to exactly the set of nodes
	// hosting its publishers, derived independently from the plan.
	want := make(map[int]map[int]struct{})
	for i, subject := range pl.pubSubject {
		if want[subject] == nil {
			want[subject] = make(map[int]struct{})
		}
		want[subject][pl.pubNode[i]] = struct{}{}
	}
	require.Equal(t, want, subjectNodes(pl))
}

func TestReadinessConverged(t *testing.T) {
	t.Parallel()

	trackers := []*probeTracker{newProbeTracker(), newProbeTracker()}
	required := []map[int]struct{}{{0: {}}, {0: {}, 1: {}}}

	require.False(t, isReady(trackers, required))
	trackers[0].observe(0)
	trackers[1].observe(0)
	require.False(t, isReady(trackers, required))
	require.Contains(t, unreadySubscribers(trackers, required), "subscriber 1")
	trackers[1].observe(1)
	require.True(t, isReady(trackers, required))
}
