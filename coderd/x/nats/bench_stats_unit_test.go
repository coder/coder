//nolint:testpackage
package nats

import (
	"testing"
	"time"
)

func TestIsFixedBenchtime(t *testing.T) {
	t.Parallel()
	cases := map[string]bool{
		"100x": true,
		"1x":   true,
		"0x":   true,
		"1s":   false,
		"":     false,
		"x":    false,
		"10":   false,
		"10xx": false,
		"1.5x": false,
	}
	for in, want := range cases {
		if got := isFixedBenchtime(in); got != want {
			t.Errorf("isFixedBenchtime(%q)=%v want %v", in, got, want)
		}
	}
}

func TestSplitCounts(t *testing.T) {
	t.Parallel()
	cases := []struct {
		total, clients int
		want           []int
	}{
		{10, 1, []int{10}},
		{10, 2, []int{5, 5}},
		{10, 3, []int{4, 3, 3}},
		{2, 5, []int{1, 1, 0, 0, 0}},
		{0, 3, []int{0, 0, 0}},
	}
	for _, c := range cases {
		got := splitCounts(c.total, c.clients)
		if len(got) != len(c.want) {
			t.Fatalf("len mismatch for %+v: got %v", c, got)
		}
		var sum int
		for i, v := range got {
			if v != c.want[i] {
				t.Errorf("splitCounts(%d,%d)[%d]=%d want %d", c.total, c.clients, i, v, c.want[i])
			}
			sum += v
		}
		if sum != c.total {
			t.Errorf("sum=%d want %d", sum, c.total)
		}
	}
	if got := splitCounts(10, 0); got != nil {
		t.Errorf("splitCounts(10,0)=%v want nil", got)
	}
}

func TestBenchSampleRateAndThroughput(t *testing.T) {
	t.Parallel()
	start := time.Unix(0, 0)
	end := start.Add(2 * time.Second)
	s := newBenchSample("x", 1000, 100, start, end, nil)
	if got := s.Rate(); got != 500 {
		t.Errorf("Rate=%v want 500", got)
	}
	if got := s.Throughput(); got != 50000 {
		t.Errorf("Throughput=%v want 50000", got)
	}
	zero := newBenchSample("z", 100, 100, start, start, nil)
	if got := zero.Rate(); got != 0 {
		t.Errorf("zero Rate=%v", got)
	}
	if got := zero.Throughput(); got != 0 {
		t.Errorf("zero Throughput=%v", got)
	}
}

func TestBenchSampleGroupAddSampleAggregatesWindowAndCounts(t *testing.T) {
	t.Parallel()
	g := newBenchSampleGroup()
	// Out of chronological order on purpose.
	s2 := newBenchSample("b", 200, 10,
		time.Unix(0, 2_000_000_000),
		time.Unix(0, 5_000_000_000),
		[]uint64{3000, 1000},
	)
	s1 := newBenchSample("a", 100, 10,
		time.Unix(0, 1_000_000_000),
		time.Unix(0, 4_000_000_000),
		[]uint64{2000},
	)
	g.AddSample(s2)
	g.AddSample(s1)
	if g.start != time.Unix(0, 1_000_000_000) {
		t.Errorf("earliest start wrong: %v", g.start)
	}
	if g.end != time.Unix(0, 5_000_000_000) {
		t.Errorf("latest end wrong: %v", g.end)
	}
	if g.jobMsgCnt != 300 {
		t.Errorf("jobMsgCnt=%d want 300", g.jobMsgCnt)
	}
	if g.msgBytes != 3000 {
		t.Errorf("msgBytes=%d want 3000", g.msgBytes)
	}
	if len(g.samples) != 2 {
		t.Errorf("samples=%d", len(g.samples))
	}
	if len(g.latencies) != 3 {
		t.Errorf("latencies=%d", len(g.latencies))
	}
	g.SortLatencies()
	want := []uint64{1000, 2000, 3000}
	for i, v := range want {
		if g.latencies[i] != v {
			t.Errorf("sorted[%d]=%d want %d", i, g.latencies[i], v)
		}
	}
}

func TestLatencyStatisticsMatchUpstreamIndexing(t *testing.T) {
	t.Parallel()
	// 100 sorted nanosecond latencies: 1000, 2000, ..., 100000.
	xs := make([]uint64, 100)
	for i := range xs {
		xs[i] = uint64((i + 1) * 1000)
	}
	// percentile upstream indexing: ceil(p/100*n)-1
	// P50 -> idx=49 -> 50_000 ns -> 50us
	if got := percentileLatencyUS(xs, 50); got != 50 {
		t.Errorf("P50=%v want 50", got)
	}
	// P90 -> idx=89 -> 90us
	if got := percentileLatencyUS(xs, 90); got != 90 {
		t.Errorf("P90=%v want 90", got)
	}
	// P99 -> idx=98 -> 99us
	if got := percentileLatencyUS(xs, 99); got != 99 {
		t.Errorf("P99=%v want 99", got)
	}
	// P99.9 -> ceil(99.9)=100 -> idx=99 -> 100us
	if got := percentileLatencyUS(xs, 99.9); got != 100 {
		t.Errorf("P99.9=%v want 100", got)
	}
	if got := minLatencyUS(xs); got != 1 {
		t.Errorf("min=%v want 1", got)
	}
	if got := maxLatencyUS(xs); got != 100 {
		t.Errorf("max=%v want 100", got)
	}
	if got := avgLatencyUS(xs); got != 50.5 {
		t.Errorf("avg=%v want 50.5", got)
	}
	// Defensive on empty.
	if got := percentileLatencyUS(nil, 50); got != 0 {
		t.Errorf("empty P50=%v", got)
	}
}
