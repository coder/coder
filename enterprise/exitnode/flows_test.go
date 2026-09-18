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

// fakeFlowClient captures batches and can be told to fail a number of calls.
type fakeFlowClient struct {
	mu       sync.Mutex
	failures int
	batches  chan []codersdk.ExitNodeFlowReport
}

func newFakeFlowClient() *fakeFlowClient {
	return &fakeFlowClient{batches: make(chan []codersdk.ExitNodeFlowReport, 16)}
}

func (c *fakeFlowClient) failNext(n int) {
	c.mu.Lock()
	c.failures = n
	c.mu.Unlock()
}

func (c *fakeFlowClient) ReportFlows(_ context.Context, req codersdk.ReportExitNodeFlowsRequest) error {
	c.mu.Lock()
	if c.failures > 0 {
		c.failures--
		c.mu.Unlock()
		return xerrors.New("coderd unavailable")
	}
	c.mu.Unlock()
	c.batches <- req.Flows
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

func TestFlowReporter_FlushOnBatchSize(t *testing.T) {
	t.Parallel()
	ctx := testutil.Context(t, testutil.WaitShort)
	mClock := quartz.NewMock(t)
	client := newFakeFlowClient()

	reporter := exitnode.NewFlowReporter(ctx, exitnode.FlowReporterOptions{
		Logger:    testutil.Logger(t),
		Client:    client,
		BatchSize: 3,
		Clock:     mClock,
	})
	defer func() { _ = reporter.Close(ctx) }()

	want := []codersdk.ExitNodeFlowReport{newReport(), newReport(), newReport()}
	for _, r := range want {
		reporter.Record(r)
	}

	// No clock advance: the batch size alone triggers delivery.
	got := testutil.RequireReceive(ctx, t, client.batches)
	require.Equal(t, want, got)
}

func TestFlowReporter_FlushOnInterval(t *testing.T) {
	t.Parallel()
	ctx := testutil.Context(t, testutil.WaitShort)
	mClock := quartz.NewMock(t)
	client := newFakeFlowClient()

	tickerTrap := mClock.Trap().NewTicker("flowreporter", "flush")
	defer tickerTrap.Close()

	reporter := exitnode.NewFlowReporter(ctx, exitnode.FlowReporterOptions{
		Logger:        testutil.Logger(t),
		Client:        client,
		FlushInterval: 2 * time.Second,
		BatchSize:     100,
		Clock:         mClock,
	})
	defer func() { _ = reporter.Close(ctx) }()
	tickerTrap.MustWait(ctx).MustRelease(ctx)

	want := newReport()
	reporter.Record(want)

	mClock.Advance(2 * time.Second).MustWait(ctx)
	got := testutil.RequireReceive(ctx, t, client.batches)
	require.Equal(t, []codersdk.ExitNodeFlowReport{want}, got)
}

func TestFlowReporter_RetriesWithBackoff(t *testing.T) {
	t.Parallel()
	ctx := testutil.Context(t, testutil.WaitShort)
	mClock := quartz.NewMock(t)
	client := newFakeFlowClient()
	client.failNext(1)

	backoffTrap := mClock.Trap().NewTimer("flowreporter", "backoff")
	defer backoffTrap.Close()

	reporter := exitnode.NewFlowReporter(ctx, exitnode.FlowReporterOptions{
		Logger:    testutil.Logger(t),
		Client:    client,
		BatchSize: 2,
		Clock:     mClock,
	})
	defer func() { _ = reporter.Close(ctx) }()

	want := []codersdk.ExitNodeFlowReport{newReport(), newReport()}
	for _, r := range want {
		reporter.Record(r)
	}

	// The first attempt fails and the reporter arms a backoff timer.
	call := backoffTrap.MustWait(ctx)
	require.Equal(t, time.Second, call.Duration)
	call.MustRelease(ctx)

	// Firing the timer retries with the same records, in order.
	mClock.Advance(time.Second).MustWait(ctx)
	got := testutil.RequireReceive(ctx, t, client.batches)
	require.Equal(t, want, got)
}

func TestFlowReporter_DropsOldestWhenFull(t *testing.T) {
	t.Parallel()
	ctx := testutil.Context(t, testutil.WaitShort)
	mClock := quartz.NewMock(t)
	client := newFakeFlowClient()
	metrics := exitnode.NewMetrics(nil)

	tickerTrap := mClock.Trap().NewTicker("flowreporter", "flush")
	defer tickerTrap.Close()

	reporter := exitnode.NewFlowReporter(ctx, exitnode.FlowReporterOptions{
		Logger:        testutil.Logger(t),
		Client:        client,
		FlushInterval: 2 * time.Second,
		BatchSize:     100,
		MaxQueue:      2,
		Metrics:       metrics,
		Clock:         mClock,
	})
	defer func() { _ = reporter.Close(ctx) }()
	tickerTrap.MustWait(ctx).MustRelease(ctx)

	first, second, third := newReport(), newReport(), newReport()
	reporter.Record(first)
	reporter.Record(second)
	reporter.Record(third)
	require.Equal(t, float64(1), promtestutil.ToFloat64(metrics.FlowReportsDropped))

	mClock.Advance(2 * time.Second).MustWait(ctx)
	got := testutil.RequireReceive(ctx, t, client.batches)
	require.Equal(t, []codersdk.ExitNodeFlowReport{second, third}, got)
	require.Equal(t, float64(2), promtestutil.ToFloat64(metrics.FlowReportsSent))
}

func TestFlowReporter_CloseFlushes(t *testing.T) {
	t.Parallel()
	ctx := testutil.Context(t, testutil.WaitShort)
	mClock := quartz.NewMock(t)
	client := newFakeFlowClient()

	reporter := exitnode.NewFlowReporter(ctx, exitnode.FlowReporterOptions{
		Logger:    testutil.Logger(t),
		Client:    client,
		BatchSize: 100,
		Clock:     mClock,
	})

	want := newReport()
	reporter.Record(want)
	require.NoError(t, reporter.Close(ctx))

	got := testutil.RequireReceive(ctx, t, client.batches)
	require.Equal(t, []codersdk.ExitNodeFlowReport{want}, got)
}
