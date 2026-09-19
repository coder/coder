package exitnode_test

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	promtestutil "github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/stretchr/testify/require"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/enterprise/exitnode"
	"github.com/coder/coder/v2/testutil"
	"github.com/coder/quartz"
)

type fakeFlowClient struct {
	mu       sync.Mutex
	failures int
	batches  chan codersdk.ReportExitNodeFlowsRequest
}

func (c *fakeFlowClient) ReportFlows(_ context.Context, req codersdk.ReportExitNodeFlowsRequest) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.failures > 0 {
		c.failures--
		return xerrors.New("coderd unavailable")
	}
	c.batches <- req
	return nil
}
func newReport() codersdk.ExitNodeFlowReport {
	return codersdk.ExitNodeFlowReport{
		FlowID:          uuid.New(),
		AgentID:         uuid.New(),
		DestinationIP:   "1.2.3.4",
		DestinationPort: 443,
		Decision:        codersdk.ExitNodeFlowAllow,
		ConnectTime:     time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC),
	}
}
func startReporter(ctx context.Context, t *testing.T, failures int, opts exitnode.FlowReporterOptions, trap func(quartz.Trapper) *quartz.Trap) (*exitnode.FlowReporter, *quartz.Mock, *fakeFlowClient, *quartz.Trap) {
	t.Helper()
	mClock := quartz.NewMock(t)
	var tr *quartz.Trap
	if trap != nil {
		tr = trap(mClock.Trap())
		t.Cleanup(tr.Close)
	}
	client := &fakeFlowClient{failures: failures, batches: make(chan codersdk.ReportExitNodeFlowsRequest, 16)}
	opts.Logger, opts.Client, opts.Clock = testutil.Logger(t), client, mClock
	reporter := exitnode.NewFlowReporter(ctx, opts)
	t.Cleanup(func() { _ = reporter.Close(ctx) })
	return reporter, mClock, client, tr
}
func TestFlowReporter_FlushOnBatchSize(t *testing.T) {
	t.Parallel()
	ctx := testutil.Context(t, testutil.WaitShort)
	reporter, _, client, _ := startReporter(ctx, t, 0, exitnode.FlowReporterOptions{BatchSize: 3}, nil)
	want := []codersdk.ExitNodeFlowReport{newReport(), newReport(), newReport()}
	for _, r := range want {
		reporter.Record(r)
	}
	require.Equal(t, want, testutil.RequireReceive(ctx, t, client.batches).Flows)
}
func TestFlowReporter_FlushOnInterval(t *testing.T) {
	t.Parallel()
	ctx := testutil.Context(t, testutil.WaitShort)
	reporter, mClock, client, tickerTrap := startReporter(ctx, t, 0, exitnode.FlowReporterOptions{
		FlushInterval: 2 * time.Second,
		BatchSize:     100,
	}, func(tr quartz.Trapper) *quartz.Trap { return tr.NewTicker("flowreporter", "flush") })
	tickerTrap.MustWait(ctx).MustRelease(ctx)
	want := newReport()
	reporter.Record(want)
	mClock.Advance(2 * time.Second).MustWait(ctx)
	require.Equal(t, []codersdk.ExitNodeFlowReport{want}, testutil.RequireReceive(ctx, t, client.batches).Flows)
}
func TestFlowReporter_RetriesWithBackoff(t *testing.T) {
	t.Parallel()
	ctx := testutil.Context(t, testutil.WaitShort)
	reporter, mClock, client, backoffTrap := startReporter(ctx, t, 1, exitnode.FlowReporterOptions{BatchSize: 2},
		func(tr quartz.Trapper) *quartz.Trap { return tr.NewTimer("flowreporter", "backoff") })
	want := []codersdk.ExitNodeFlowReport{newReport(), newReport()}
	for _, r := range want {
		reporter.Record(r)
	}
	call := backoffTrap.MustWait(ctx)
	require.Equal(t, time.Second, call.Duration)
	call.MustRelease(ctx)
	mClock.Advance(time.Second).MustWait(ctx)
	require.Equal(t, want, testutil.RequireReceive(ctx, t, client.batches).Flows)
}
func TestFlowReporter_DropsOldestWhenFull(t *testing.T) {
	t.Parallel()
	ctx := testutil.Context(t, testutil.WaitShort)
	metrics := exitnode.NewMetrics(nil)
	reporter, mClock, client, tickerTrap := startReporter(ctx, t, 0, exitnode.FlowReporterOptions{
		FlushInterval: 2 * time.Second,
		BatchSize:     100,
		MaxQueue:      2,
		Metrics:       metrics,
	}, func(tr quartz.Trapper) *quartz.Trap { return tr.NewTicker("flowreporter", "flush") })
	tickerTrap.MustWait(ctx).MustRelease(ctx)
	first, second, third := newReport(), newReport(), newReport()
	reporter.Record(first)
	reporter.Record(second)
	reporter.Record(third)
	require.Equal(t, float64(1), promtestutil.ToFloat64(metrics.FlowReportsDropped))
	mClock.Advance(2 * time.Second).MustWait(ctx)
	request := testutil.RequireReceive(ctx, t, client.batches)
	require.Equal(t, []codersdk.ExitNodeFlowReport{second, third}, request.Flows)
	require.Equal(t, 1, request.DroppedReports)
	require.Equal(t, float64(2), promtestutil.ToFloat64(metrics.FlowReportsSent))
	fourth := newReport()
	reporter.Record(fourth)
	mClock.Advance(2 * time.Second).MustWait(ctx)
	request = testutil.RequireReceive(ctx, t, client.batches)
	require.Equal(t, []codersdk.ExitNodeFlowReport{fourth}, request.Flows)
	require.Zero(t, request.DroppedReports)
}
func TestFlowReporter_CloseFlushes(t *testing.T) {
	t.Parallel()
	ctx := testutil.Context(t, testutil.WaitShort)
	reporter, _, client, _ := startReporter(ctx, t, 0, exitnode.FlowReporterOptions{BatchSize: 100}, nil)
	want := newReport()
	reporter.Record(want)
	require.NoError(t, reporter.Close(ctx))
	require.Equal(t, []codersdk.ExitNodeFlowReport{want}, testutil.RequireReceive(ctx, t, client.batches).Flows)
}
