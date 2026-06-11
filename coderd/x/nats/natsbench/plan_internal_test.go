package natsbench

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestBuildPlan(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name              string
		cfg               Config
		wantPerPubMsgs    []int
		wantPubSubject    []int
		wantPubNode       []int
		wantSubSubject    []int
		wantSubNode       []int
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
			wantPubNode:       []int{0, 1, 0, 1},
			wantSubSubject:    []int{0, 1, 0, 1},
			wantSubNode:       []int{0, 1, 0, 1},
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
			wantPubNode:       []int{0, 0, 0},
			wantSubSubject:    []int{0, 1, 2},
			wantSubNode:       []int{0, 0, 0},
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
			wantPubNode:    []int{0, 1, 2, 0, 1},
			wantSubSubject: []int{0, 1},
			wantSubNode:    []int{0, 1},
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
			wantPubNode:    []int{0},
			wantSubSubject: []int{0, 1, 0, 1},
			wantSubNode:    []int{0, 0, 0, 0},
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
			wantPubNode:       []int{0, 1},
			wantSubSubject:    []int{0, 0, 0, 0, 0, 0, 0, 0, 0, 0},
			wantSubNode:       []int{0, 1, 2, 3, 4, 0, 1, 2, 3, 4},
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
			require.Equal(t, tc.wantPubNode, pl.pubNode)
			require.Equal(t, tc.wantSubSubject, pl.subSubject)
			require.Equal(t, tc.wantSubNode, pl.subNode)
			require.Equal(t, tc.wantExpectPerSub, pl.expectPerSub)
			require.Equal(t, tc.wantTotalExpected, pl.totalExpected)
		})
	}
}

func TestSubjectName(t *testing.T) {
	t.Parallel()
	require.Equal(t, "bench.0", subjectName(0))
	require.Equal(t, "bench.17", subjectName(17))
}
