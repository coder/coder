package main

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestBuildPlan(t *testing.T) {
	t.Parallel()

	// Node placement is random, so these cases assert only the
	// deterministic subject and message assignment.
	cases := []struct {
		name              string
		cfg               Config
		wantPerPubMsgs    []int
		wantPubSubject    []int
		wantSubSubject    []int
		wantExpectPerSub  []int
		wantTotalExpected int
	}{
		{
			name: "EvenSplit",
			cfg: Config{
				Messages: 100, Publishers: 4, Subjects: 2, Subscribers: 4, Replicas: 2,
			},
			wantPerPubMsgs:    []int{25, 25, 25, 25},
			wantPubSubject:    []int{0, 1, 0, 1},
			wantSubSubject:    []int{0, 1, 0, 1},
			wantExpectPerSub:  []int{50, 50, 50, 50},
			wantTotalExpected: 200,
		},
		{
			name: "RemainderToPublisherZero",
			cfg: Config{
				Messages: 10, Publishers: 3, Subjects: 3, Subscribers: 3, Replicas: 1,
			},
			wantPerPubMsgs:    []int{4, 3, 3},
			wantPubSubject:    []int{0, 1, 2},
			wantSubSubject:    []int{0, 1, 2},
			wantExpectPerSub:  []int{4, 3, 3},
			wantTotalExpected: 10,
		},
		{
			name: "MorePublishersThanSubjects",
			cfg: Config{
				Messages: 30, Publishers: 5, Subjects: 2, Subscribers: 2, Replicas: 3,
			},
			// Publisher 0 gets 6 + remainder 0; 30/5 = 6 each.
			wantPerPubMsgs: []int{6, 6, 6, 6, 6},
			wantPubSubject: []int{0, 1, 0, 1, 0},
			wantSubSubject: []int{0, 1},
			// Subject 0 receives from publishers 0, 2, 4 (18 msgs);
			// subject 1 from publishers 1, 3 (12 msgs).
			wantExpectPerSub:  []int{18, 12},
			wantTotalExpected: 30,
		},
		{
			name: "SubscriberOnSubjectWithoutPublishers",
			cfg: Config{
				Messages: 9, Publishers: 1, Subjects: 2, Subscribers: 4, Replicas: 1,
			},
			wantPerPubMsgs: []int{9},
			wantPubSubject: []int{0},
			wantSubSubject: []int{0, 1, 0, 1},
			// Subscribers on subject 1 expect nothing.
			wantExpectPerSub:  []int{9, 0, 9, 0},
			wantTotalExpected: 18,
		},
		{
			name: "FanOutExceedsPublishes",
			cfg: Config{
				Messages: 100, Publishers: 2, Subjects: 1, Subscribers: 10, Replicas: 5,
			},
			wantPerPubMsgs:    []int{50, 50},
			wantPubSubject:    []int{0, 0},
			wantSubSubject:    []int{0, 0, 0, 0, 0, 0, 0, 0, 0, 0},
			wantExpectPerSub:  []int{100, 100, 100, 100, 100, 100, 100, 100, 100, 100},
			wantTotalExpected: 1000,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			pl := buildPlan(tc.cfg)
			require.Equal(t, tc.wantPerPubMsgs, pl.perPubMsgs)
			require.Equal(t, tc.wantPubSubject, pl.pubSubject)
			require.Equal(t, tc.wantSubSubject, pl.subSubject)
			require.Equal(t, tc.wantExpectPerSub, pl.expectPerSub)
			require.Equal(t, tc.wantTotalExpected, pl.totalExpected)

			// Every node index is a valid replica, and pubNodes/subNodes
			// are the sorted distinct sets of those indexes.
			for _, n := range append(append([]int{}, pl.pubNode...), pl.subNode...) {
				require.GreaterOrEqual(t, n, 0)
				require.Less(t, n, tc.cfg.Replicas)
			}
			require.Equal(t, uniqueInts(pl.pubNode), pl.pubNodes)
			require.Equal(t, uniqueInts(pl.subNode), pl.subNodes)
		})
	}
}

func TestBuildPlanNodePlacement(t *testing.T) {
	t.Parallel()

	cfg := Config{Messages: 1000, Publishers: 50, Subjects: 10, Subscribers: 200, Replicas: 7}

	// The same seed reproduces the same placement.
	require.Equal(t, buildPlan(cfg).pubNode, buildPlan(cfg).pubNode)
	require.Equal(t, buildPlan(cfg).subNode, buildPlan(cfg).subNode)

	// A different seed (very likely) produces a different placement.
	other := cfg
	other.Seed = cfg.Seed + 1
	require.NotEqual(t, buildPlan(cfg).subNode, buildPlan(other).subNode)

	// With many clients and few replicas, placement spreads across
	// every node rather than collapsing onto one.
	pl := buildPlan(cfg)
	require.Len(t, pl.pubNodes, cfg.Replicas)
	require.Len(t, pl.subNodes, cfg.Replicas)
}

// TestDefaultSeedSpreadsEvenly guards the DefaultSeed comment's claim:
// with the matrix shape (10 publishers) it places publishers perfectly
// evenly across nodes at every matrix replica count (1, 5, and 10), so
// no node is left publisher-less and cross-node routing is not skewed.
func TestDefaultSeedSpreadsEvenly(t *testing.T) {
	t.Parallel()

	const publishers = 10
	for _, replicas := range []int{1, 5, 10} {
		cfg := Config{
			Messages:   1000,
			Subjects:   10,
			Publishers: publishers,
			// Subscribers do not affect publisher placement, but a plan
			// needs at least one.
			Subscribers: 1,
			Replicas:    replicas,
			Seed:        DefaultSeed,
		}
		pl := buildPlan(cfg)

		perNode := make([]int, replicas)
		for _, n := range pl.pubNode {
			perNode[n]++
		}
		want := publishers / replicas
		for node, got := range perNode {
			require.Equalf(t, want, got,
				"replicas=%d node=%d expected %d publishers, got %d (placement %v)",
				replicas, node, want, got, perNode)
		}
	}
}

func TestSubjectName(t *testing.T) {
	t.Parallel()
	require.Equal(t, "bench.0", subjectName(0))
	require.Equal(t, "bench.17", subjectName(17))
}
