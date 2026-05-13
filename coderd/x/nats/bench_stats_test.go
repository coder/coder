//nolint:testpackage
package nats

import (
	"fmt"
	"math"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/dustin/go-humanize"
)

// benchSample represents one client's contribution to a benchmark
// run, modeled after upstream natscli/internal/bench/stats.go. Each
// publisher or subscriber connection produces exactly one sample.
type benchSample struct {
	name      string
	jobMsgCnt int64
	start     time.Time
	end       time.Time
	msgBytes  uint64
	// latencies is nanoseconds per operation. May be empty for
	// subscriber samples that only record first-recv / last-recv.
	latencies []uint64

	errored      bool
	disconnected bool
}

func newBenchSample(name string, jobMsgCnt int64, msgSize int, start, end time.Time, latencies []uint64) *benchSample {
	return &benchSample{
		name:      name,
		jobMsgCnt: jobMsgCnt,
		start:     start,
		end:       end,
		msgBytes:  uint64(jobMsgCnt) * uint64(msgSize),
		latencies: latencies,
	}
}

func (s *benchSample) Duration() time.Duration {
	return s.end.Sub(s.start)
}

// Rate returns messages per second. Returns 0 for zero-duration
// samples.
func (s *benchSample) Rate() float64 {
	d := s.Duration().Seconds()
	if d <= 0 {
		return 0
	}
	return float64(s.jobMsgCnt) / d
}

// Throughput returns bytes per second. Returns 0 for zero-duration
// samples.
func (s *benchSample) Throughput() float64 {
	d := s.Duration().Seconds()
	if d <= 0 {
		return 0
	}
	return float64(s.msgBytes) / d
}

func (s *benchSample) HasLatency() bool {
	return len(s.latencies) > 0
}

// benchSampleGroup aggregates per-client samples using upstream
// semantics: earliest start, latest end, summed counts and bytes,
// concatenated latencies (caller must call SortLatencies before
// querying percentiles).
type benchSampleGroup struct {
	benchSample
	samples []*benchSample
}

func newBenchSampleGroup() *benchSampleGroup {
	return &benchSampleGroup{}
}

func (g *benchSampleGroup) AddSample(s *benchSample) {
	if s == nil {
		return
	}
	if len(g.samples) == 0 {
		g.start = s.start
		g.end = s.end
	} else {
		if s.start.Before(g.start) {
			g.start = s.start
		}
		if s.end.After(g.end) {
			g.end = s.end
		}
	}
	g.jobMsgCnt += s.jobMsgCnt
	g.msgBytes += s.msgBytes
	g.latencies = append(g.latencies, s.latencies...)
	if s.errored {
		g.errored = true
	}
	if s.disconnected {
		g.disconnected = true
	}
	g.samples = append(g.samples, s)
}

func (g *benchSampleGroup) SortLatencies() {
	sort.Slice(g.latencies, func(i, j int) bool { return g.latencies[i] < g.latencies[j] })
}

func (g *benchSampleGroup) HasSamples() bool {
	return len(g.samples) > 0
}

// Latency helpers operate on a sorted nanosecond slice and return
// microseconds. They are defensive against empty input.

func minLatencyUS(sortedNS []uint64) float64 {
	if len(sortedNS) == 0 {
		return 0
	}
	return float64(sortedNS[0]) / 1000.0
}

func maxLatencyUS(sortedNS []uint64) float64 {
	if len(sortedNS) == 0 {
		return 0
	}
	return float64(sortedNS[len(sortedNS)-1]) / 1000.0
}

func avgLatencyUS(sortedNS []uint64) float64 {
	if len(sortedNS) == 0 {
		return 0
	}
	var sum float64
	for _, v := range sortedNS {
		sum += float64(v)
	}
	return sum / float64(len(sortedNS)) / 1000.0
}

func stdLatencyUS(sortedNS []uint64) float64 {
	if len(sortedNS) < 2 {
		return 0
	}
	mean := avgLatencyUS(sortedNS) * 1000.0
	var variance float64
	for _, v := range sortedNS {
		d := float64(v) - mean
		variance += d * d
	}
	variance /= float64(len(sortedNS))
	return math.Sqrt(variance) / 1000.0
}

// percentileLatencyUS uses the upstream natscli indexing:
// ceil(percentile/100 * len) - 1, clamped to [0, len-1].
func percentileLatencyUS(sortedNS []uint64, percentile float64) float64 {
	n := len(sortedNS)
	if n == 0 {
		return 0
	}
	idx := int(math.Ceil(percentile/100.0*float64(n))) - 1
	if idx < 0 {
		idx = 0
	}
	if idx >= n {
		idx = n - 1
	}
	return float64(sortedNS[idx]) / 1000.0
}

// rateStatistics returns (min, avg, max, stddev) message rates across
// the per-client samples in msgs/sec.
func (g *benchSampleGroup) rateStatistics() (minR, avgR, maxR, stdR float64) {
	if len(g.samples) == 0 {
		return 0, 0, 0, 0
	}
	rates := make([]float64, len(g.samples))
	var sum float64
	minR = math.Inf(1)
	maxR = math.Inf(-1)
	for i, s := range g.samples {
		r := s.Rate()
		rates[i] = r
		sum += r
		if r < minR {
			minR = r
		}
		if r > maxR {
			maxR = r
		}
	}
	avgR = sum / float64(len(rates))
	var v float64
	for _, r := range rates {
		d := r - avgR
		v += d * d
	}
	stdR = math.Sqrt(v / float64(len(rates)))
	return minR, avgR, maxR, stdR
}

func formatRate(r float64) string {
	return humanize.Commaf(math.Round(r))
}

func formatIECBytesPerSec(bps float64) string {
	return humanize.IBytes(uint64(bps)) + "/sec"
}

// Report renders a multi-line upstream-style summary for the group.
// `name` is a short label like "NATS Pub" or "NATS Sub". `unit` is the
// per-line message unit (e.g. "msgs").
func (g *benchSampleGroup) Report(name, unit string) string {
	var b strings.Builder
	g.SortLatencies()
	fmt.Fprintf(&b, "%s stats: %s msgs/sec ~ %s",
		name,
		formatRate(g.Rate()),
		formatIECBytesPerSec(g.Throughput()),
	)
	if g.HasLatency() {
		fmt.Fprintf(&b, " ~ min: %.2fus avg: %.2fus max: %.2fus",
			minLatencyUS(g.latencies),
			avgLatencyUS(g.latencies),
			maxLatencyUS(g.latencies),
		)
	}
	b.WriteString("\n")
	for i, s := range g.samples {
		sorted := append([]uint64(nil), s.latencies...)
		sort.Slice(sorted, func(a, b int) bool { return sorted[a] < sorted[b] })
		fmt.Fprintf(&b, " [%d] %s msgs/sec ~ %s",
			i+1,
			formatRate(s.Rate()),
			formatIECBytesPerSec(s.Throughput()),
		)
		if len(sorted) > 0 {
			fmt.Fprintf(&b, " ~ min: %.2fus avg: %.2fus max: %.2fus",
				minLatencyUS(sorted),
				avgLatencyUS(sorted),
				maxLatencyUS(sorted),
			)
		}
		fmt.Fprintf(&b, " (%s %s)\n", humanize.Comma(s.jobMsgCnt), unit)
	}
	minR, avgR, maxR, stdR := g.rateStatistics()
	fmt.Fprintf(&b, " message rates min %s | avg %s | max %s | stddev %s msgs\n",
		formatRate(minR), formatRate(avgR), formatRate(maxR), formatRate(stdR),
	)
	if g.HasLatency() {
		fmt.Fprintf(&b, " latencies per operation min %.2fus | avg %.2fus | max %.2fus | stddev %.2fus | P50 %.2fus | P90 %.2fus | P99 %.2fus | P99.9 %.2fus\n",
			minLatencyUS(g.latencies),
			avgLatencyUS(g.latencies),
			maxLatencyUS(g.latencies),
			stdLatencyUS(g.latencies),
			percentileLatencyUS(g.latencies, 50),
			percentileLatencyUS(g.latencies, 90),
			percentileLatencyUS(g.latencies, 99),
			percentileLatencyUS(g.latencies, 99.9),
		)
	}
	return b.String()
}

// benchReportInput bundles the data needed to log a full report and
// emit headline scalar metrics for one benchmark leaf.
type benchReportInput struct {
	name         string
	pubs         *benchSampleGroup
	subs         *benchSampleGroup
	expected     int64
	delivered    int64
	dropEvents   int64
	errored      int64
	disconnected int64
	diagnostics  string
}

func logBenchReport(b *testing.B, in benchReportInput) {
	b.Helper()
	var out strings.Builder
	out.WriteString("\n")
	if in.pubs != nil && in.pubs.HasSamples() {
		out.WriteString(in.pubs.Report("NATS Pub", "msgs"))
	}
	if in.subs != nil && in.subs.HasSamples() {
		out.WriteString("\n")
		out.WriteString(in.subs.Report("NATS Sub", "msgs"))
	}
	var pct float64
	if in.expected > 0 {
		pct = 100.0 * float64(in.delivered) / float64(in.expected)
	}
	fmt.Fprintf(&out, "Delivery: delivered=%d expected=%d delivery_pct=%.2f drop_events=%d errored=%d disconnected=%d\n",
		in.delivered, in.expected, pct, in.dropEvents, in.errored, in.disconnected,
	)
	if in.diagnostics != "" {
		out.WriteString(in.diagnostics)
		if !strings.HasSuffix(in.diagnostics, "\n") {
			out.WriteString("\n")
		}
	}
	b.Logf("%s", out.String())
}

// reportBenchMetrics emits the small set of headline scalars used for
// benchstat regression tracking. The full human report goes through
// b.Logf via logBenchReport.
func reportBenchMetrics(b *testing.B, pubs, subs *benchSampleGroup, expected, delivered int64) {
	b.Helper()
	if pubs != nil && pubs.HasSamples() {
		b.ReportMetric(pubs.Rate(), "pubs/s")
		b.ReportMetric(pubs.Throughput()/(1<<20), "pubMiB/s")
		pubs.SortLatencies()
		if pubs.HasLatency() {
			b.ReportMetric(percentileLatencyUS(pubs.latencies, 50), "p50_us")
			b.ReportMetric(percentileLatencyUS(pubs.latencies, 99), "p99_us")
			b.ReportMetric(percentileLatencyUS(pubs.latencies, 99.9), "p99.9_us")
		}
	}
	if subs != nil && subs.HasSamples() {
		b.ReportMetric(subs.Rate(), "sub_pubs/s")
		b.ReportMetric(subs.Throughput()/(1<<20), "sub_MiB/s")
	}
	var pct float64
	if expected > 0 {
		pct = 100.0 * float64(delivered) / float64(expected)
	}
	b.ReportMetric(pct, "delivery_pct")
}
