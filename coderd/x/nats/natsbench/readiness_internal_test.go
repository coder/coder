package natsbench

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestProbeRoundTrip(t *testing.T) {
	t.Parallel()

	for _, node := range []int{0, 1, 7, 1 << 30} {
		payload := probePayload(node)
		require.Len(t, payload, probeLen)
		got, ok := probeNode(payload)
		require.True(t, ok)
		require.Equal(t, node, got)
	}
}

func TestProbeNodeRejectsBenchmarkPayloads(t *testing.T) {
	t.Parallel()

	// Benchmark payloads are all zeros, in any size including the
	// probe length.
	for _, size := range []int{1, probeLen, Payload8KB} {
		_, ok := probeNode(make([]byte, size))
		require.False(t, ok)
	}
	// Truncated or extended probes are not probes.
	_, ok := probeNode(probePayload(3)[:probeLen-1])
	require.False(t, ok)
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
	// Publishers: 0->s0/n0, 1->s1/n1, 2->s0/n2, 3->s1/n0, 4->s0/n1.
	require.Equal(t, map[int]map[int]struct{}{
		0: {0: {}, 1: {}, 2: {}},
		1: {0: {}, 1: {}},
	}, subjectNodes(pl))
}

func TestReadinessConverged(t *testing.T) {
	t.Parallel()

	trackers := []*probeTracker{newProbeTracker(), newProbeTracker()}
	required := []map[int]struct{}{{0: {}}, {0: {}, 1: {}}}

	require.False(t, readinessConverged(trackers, required))
	trackers[0].observe(0)
	trackers[1].observe(0)
	require.False(t, readinessConverged(trackers, required))
	require.Contains(t, readinessShortfall(trackers, required), "subscriber 1")
	trackers[1].observe(1)
	require.True(t, readinessConverged(trackers, required))
}
