package pubsub

import (
	"bytes"
	"context"
	"database/sql"
	"sync"
	"testing"
	"time"

	_ "github.com/lib/pq"
	prom_testutil "github.com/prometheus/client_golang/prometheus/testutil"
	dto "github.com/prometheus/client_model/go"
	"github.com/stretchr/testify/require"

	"cdr.dev/slog/v3"
	"cdr.dev/slog/v3/sloggers/slogtest"
	"github.com/coder/coder/v2/testutil"
	"github.com/coder/quartz"
)

func TestBatchingPubsubScheduledFlush(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitShort)
	clock := quartz.NewMock(t)
	newTimerTrap := clock.Trap().NewTimer("pubsubBatcher", "scheduledFlush")
	defer newTimerTrap.Close()
	resetTrap := clock.Trap().TimerReset("pubsubBatcher", "scheduledFlush")
	defer resetTrap.Close()

	sender := newFakeBatchSender()
	ps, _ := newTestBatchingPubsub(t, sender, BatchingConfig{
		Clock:         clock,
		FlushInterval: 10 * time.Millisecond,
		QueueSize:     8,
	})

	call, err := newTimerTrap.Wait(ctx)
	require.NoError(t, err)
	call.MustRelease(ctx)

	require.NoError(t, ps.Publish("chat:stream:a", []byte("one")))
	require.NoError(t, ps.Publish("chat:stream:a", []byte("two")))
	require.Empty(t, sender.Batches())

	clock.Advance(10 * time.Millisecond).MustWait(ctx)
	resetCall, err := resetTrap.Wait(ctx)
	require.NoError(t, err)
	resetCall.MustRelease(ctx)

	batch := testutil.TryReceive(ctx, t, sender.flushes)
	require.Len(t, batch, 2)
	require.Equal(t, []byte("one"), batch[0].message)
	require.Equal(t, []byte("two"), batch[1].message)
	batchSizeCount, batchSizeSum := histogramCountAndSum(t, ps.metrics.BatchSize)
	require.Equal(t, uint64(1), batchSizeCount)
	require.InDelta(t, 2, batchSizeSum, 0.000001)
	flushDurationCount, _ := histogramCountAndSum(t, ps.metrics.FlushDuration.WithLabelValues(batchFlushScheduled))
	require.Equal(t, uint64(1), flushDurationCount)
	require.Zero(t, prom_testutil.ToFloat64(ps.metrics.QueueDepth))
}

func TestBatchingPubsubDefaultConfigUsesDedicatedSenderFirstDefaults(t *testing.T) {
	t.Parallel()

	clock := quartz.NewMock(t)
	sender := newFakeBatchSender()
	ps, _ := newTestBatchingPubsub(t, sender, BatchingConfig{Clock: clock})

	require.Equal(t, DefaultBatchingFlushInterval, ps.flushInterval)
	require.Equal(t, DefaultBatchingQueueSize, cap(ps.publishCh))
	require.Equal(t, defaultBatchingPressureWait, ps.pressureWait)
	require.Equal(t, defaultBatchingFinalFlushLimit, ps.finalFlushTimeout)
}

func TestBatchChannelClass(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		event string
		want  string
	}{
		{name: "stream notify", event: "chat:stream:123", want: batchChannelClassStreamNotify},
		{name: "owner event", event: "chat:owner:123", want: batchChannelClassOwnerEvent},
		{name: "config change", event: "chat:config_change", want: batchChannelClassConfigChange},
		{name: "fallback", event: "workspace:owner:123", want: batchChannelClassOther},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, tt.want, batchChannelClass(tt.event))
		})
	}
}

func TestBatchingPubsubTimerFlushDrainsAll(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitShort)
	clock := quartz.NewMock(t)
	newTimerTrap := clock.Trap().NewTimer("pubsubBatcher", "scheduledFlush")
	defer newTimerTrap.Close()
	resetTrap := clock.Trap().TimerReset("pubsubBatcher", "scheduledFlush")
	defer resetTrap.Close()

	sender := newFakeBatchSender()
	ps, _ := newTestBatchingPubsub(t, sender, BatchingConfig{
		Clock:         clock,
		FlushInterval: 10 * time.Millisecond,
		QueueSize:     64,
	})

	call, err := newTimerTrap.Wait(ctx)
	require.NoError(t, err)
	call.MustRelease(ctx)

	// Enqueue many messages before the timer fires — all should be
	// drained and flushed in a single batch.
	for _, msg := range []string{"one", "two", "three", "four", "five"} {
		require.NoError(t, ps.Publish("chat:stream:a", []byte(msg)))
	}
	require.Empty(t, sender.Batches())

	clock.Advance(10 * time.Millisecond).MustWait(ctx)
	resetCall, err := resetTrap.Wait(ctx)
	require.NoError(t, err)
	resetCall.MustRelease(ctx)

	batch := testutil.TryReceive(ctx, t, sender.flushes)
	require.Len(t, batch, 5)
	require.Equal(t, []byte("one"), batch[0].message)
	require.Equal(t, []byte("five"), batch[4].message)
	require.Zero(t, prom_testutil.ToFloat64(ps.metrics.QueueDepth))
}

func TestBatchingPubsubQueueFullFallsBackToDelegate(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitShort)
	clock := quartz.NewMock(t)
	newTimerTrap := clock.Trap().NewTimer("pubsubBatcher", "scheduledFlush")
	defer newTimerTrap.Close()
	resetTrap := clock.Trap().TimerReset("pubsubBatcher", "scheduledFlush")
	defer resetTrap.Close()
	pressureTrap := clock.Trap().NewTimer("pubsubBatcher", "pressureWait")
	defer pressureTrap.Close()

	sender := newFakeBatchSender()
	sender.blockCh = make(chan struct{})
	ps, _ := newTestBatchingPubsub(t, sender, BatchingConfig{
		Clock:         clock,
		FlushInterval: 10 * time.Millisecond,
		QueueSize:     1,
		PressureWait:  10 * time.Millisecond,
	})

	call, err := newTimerTrap.Wait(ctx)
	require.NoError(t, err)
	call.MustRelease(ctx)

	// Fill the queue (capacity 1).
	require.NoError(t, ps.Publish("chat:stream:a", []byte("one")))

	// Fire the timer so the run loop starts flushing "one" — the
	// sender blocks on blockCh so the flush stays in-flight.
	clock.Advance(10 * time.Millisecond).MustWait(ctx)
	<-sender.started

	// The run loop is blocked in flushBatch. Fill the queue again.
	require.NoError(t, ps.Publish("chat:stream:a", []byte("two")))

	// A third publish should fall back to the delegate (which has a
	// closed db, so the delegate Publish itself will error — but we
	// verify the fallback metric was incremented).
	errCh := make(chan error, 1)
	go func() {
		errCh <- ps.Publish("chat:stream:a", []byte("three"))
	}()

	pressureCall, err := pressureTrap.Wait(ctx)
	require.NoError(t, err)
	pressureCall.MustRelease(ctx)
	clock.Advance(10 * time.Millisecond).MustWait(ctx)

	err = testutil.TryReceive(ctx, t, errCh)
	// The delegate has a closed db so it returns an error from the
	// shared pool, not a batching-specific sentinel.
	require.Error(t, err)
	require.Equal(t, float64(1), prom_testutil.ToFloat64(ps.metrics.DelegateFallbacksTotal.WithLabelValues(batchChannelClassStreamNotify, batchDelegateFallbackReasonQueueFull, batchFlushStageNone)))

	close(sender.blockCh)
	// Let the run loop finish the blocked flush and process "two".
	resetCall, err := resetTrap.Wait(ctx)
	require.NoError(t, err)
	resetCall.MustRelease(ctx)
	require.NoError(t, ps.Close())
}

func TestBatchingPubsubCloseDrainsQueue(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitShort)
	clock := quartz.NewMock(t)
	newTimerTrap := clock.Trap().NewTimer("pubsubBatcher", "scheduledFlush")
	defer newTimerTrap.Close()

	sender := newFakeBatchSender()
	ps, _ := newTestBatchingPubsub(t, sender, BatchingConfig{
		Clock:         clock,
		FlushInterval: time.Hour,
		QueueSize:     8,
	})

	call, err := newTimerTrap.Wait(ctx)
	require.NoError(t, err)
	call.MustRelease(ctx)

	require.NoError(t, ps.Publish("chat:stream:a", []byte("one")))
	require.NoError(t, ps.Publish("chat:stream:a", []byte("two")))
	require.NoError(t, ps.Publish("chat:stream:a", []byte("three")))

	require.NoError(t, ps.Close())
	batches := sender.Batches()
	require.Len(t, batches, 1)
	require.Len(t, batches[0], 3)
	require.Equal(t, []byte("one"), batches[0][0].message)
	require.Equal(t, []byte("two"), batches[0][1].message)
	require.Equal(t, []byte("three"), batches[0][2].message)
	require.Zero(t, prom_testutil.ToFloat64(ps.metrics.QueueDepth))
	require.Equal(t, 1, sender.CloseCalls())
}

func TestBatchingPubsubPreservesOrder(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitShort)
	clock := quartz.NewMock(t)
	newTimerTrap := clock.Trap().NewTimer("pubsubBatcher", "scheduledFlush")
	defer newTimerTrap.Close()

	sender := newFakeBatchSender()
	ps, _ := newTestBatchingPubsub(t, sender, BatchingConfig{
		Clock:         clock,
		FlushInterval: time.Hour,
		QueueSize:     8,
	})

	call, err := newTimerTrap.Wait(ctx)
	require.NoError(t, err)
	call.MustRelease(ctx)

	for _, msg := range []string{"one", "two", "three", "four", "five"} {
		require.NoError(t, ps.Publish("chat:stream:a", []byte(msg)))
	}

	require.NoError(t, ps.Close())
	batches := sender.Batches()
	require.NotEmpty(t, batches)

	messages := make([]string, 0, 5)
	for _, batch := range batches {
		for _, item := range batch {
			messages = append(messages, string(item.message))
		}
	}
	require.Equal(t, []string{"one", "two", "three", "four", "five"}, messages)
}

func TestBatchingPubsubFlushFailureMetrics(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitShort)
	clock := quartz.NewMock(t)
	newTimerTrap := clock.Trap().NewTimer("pubsubBatcher", "scheduledFlush")
	defer newTimerTrap.Close()
	resetTrap := clock.Trap().TimerReset("pubsubBatcher", "scheduledFlush")
	defer resetTrap.Close()

	sender := newFakeBatchSender()
	sender.err = context.DeadlineExceeded
	sender.errStage = batchFlushStageExec
	ps, delegate := newTestBatchingPubsub(t, sender, BatchingConfig{
		Clock:         clock,
		FlushInterval: 10 * time.Millisecond,
		QueueSize:     8,
	})

	call, err := newTimerTrap.Wait(ctx)
	require.NoError(t, err)
	call.MustRelease(ctx)

	require.NoError(t, ps.Publish("chat:stream:a", []byte("one")))

	clock.Advance(10 * time.Millisecond).MustWait(ctx)
	resetCall, err := resetTrap.Wait(ctx)
	require.NoError(t, err)
	resetCall.MustRelease(ctx)

	batchSizeCount, batchSizeSum := histogramCountAndSum(t, ps.metrics.BatchSize)
	require.Equal(t, uint64(1), batchSizeCount)
	require.InDelta(t, 1, batchSizeSum, 0.000001)
	flushDurationCount, _ := histogramCountAndSum(t, ps.metrics.FlushDuration.WithLabelValues(batchFlushScheduled))
	require.Equal(t, uint64(1), flushDurationCount)
	require.Equal(t, float64(1), prom_testutil.ToFloat64(delegate.publishesTotal.WithLabelValues("false")))
	require.Zero(t, prom_testutil.ToFloat64(delegate.publishesTotal.WithLabelValues("true")))
	require.Zero(t, prom_testutil.ToFloat64(ps.metrics.QueueDepth))
	require.Equal(t, float64(1), prom_testutil.ToFloat64(ps.metrics.DelegateFallbacksTotal.WithLabelValues(batchChannelClassStreamNotify, batchDelegateFallbackReasonFlushError, batchFlushStageExec)))
}

func TestBatchingPubsubFlushFailureStageAccounting(t *testing.T) {
	t.Parallel()

	stages := []string{batchFlushStageBegin, batchFlushStageExec, batchFlushStageCommit}
	for _, stage := range stages {
		stage := stage
		t.Run(stage, func(t *testing.T) {
			t.Parallel()

			sender := newFakeBatchSender()
			sender.err = context.DeadlineExceeded
			sender.errStage = stage
			ps, delegate := newTestBatchingPubsub(t, sender, BatchingConfig{Clock: quartz.NewMock(t)})

			batch := []queuedPublish{{
				event:        "chat:stream:test",
				channelClass: batchChannelClass("chat:stream:test"),
				message:      []byte("fallback-" + stage),
			}}
			ps.queuedCount.Store(int64(len(batch)))
			_, err := ps.flushBatch(context.Background(), batch, batchFlushScheduled)
			require.Error(t, err)
			require.Equal(t, float64(1), prom_testutil.ToFloat64(ps.metrics.DelegateFallbacksTotal.WithLabelValues(batchChannelClassStreamNotify, batchDelegateFallbackReasonFlushError, stage)))
			require.Equal(t, float64(1), prom_testutil.ToFloat64(delegate.publishesTotal.WithLabelValues("false")))
		})
	}
}

func TestBatchingPubsubFlushFailureResetSender(t *testing.T) {
	t.Parallel()

	clock := quartz.NewMock(t)
	firstSender := newFakeBatchSender()
	firstSender.err = context.DeadlineExceeded
	firstSender.errStage = batchFlushStageExec
	secondSender := newFakeBatchSender()
	ps, _ := newTestBatchingPubsub(t, firstSender, BatchingConfig{Clock: clock})
	ps.newSender = func(context.Context) (batchSender, error) {
		return secondSender, nil
	}

	firstBatch := []queuedPublish{{
		event:        "chat:stream:first",
		channelClass: batchChannelClass("chat:stream:first"),
		message:      []byte("first"),
	}}
	ps.queuedCount.Store(int64(len(firstBatch)))
	_, err := ps.flushBatch(context.Background(), firstBatch, batchFlushScheduled)
	require.Error(t, err)
	require.Equal(t, float64(1), prom_testutil.ToFloat64(ps.metrics.SenderResetsTotal))
	require.Equal(t, 1, firstSender.CloseCalls())

	secondBatch := []queuedPublish{{
		event:        "chat:stream:second",
		channelClass: batchChannelClass("chat:stream:second"),
		message:      []byte("second"),
	}}
	ps.queuedCount.Store(int64(len(secondBatch)))
	_, err = ps.flushBatch(context.Background(), secondBatch, batchFlushScheduled)
	require.NoError(t, err)
	batches := secondSender.Batches()
	require.Len(t, batches, 1)
	require.Len(t, batches[0], 1)
	require.Equal(t, []byte("second"), batches[0][0].message)
}

func TestBatchingPubsubFlushFailureReturnsJoinedErrorWhenReplayFails(t *testing.T) {
	t.Parallel()

	sender := newFakeBatchSender()
	sender.err = context.DeadlineExceeded
	sender.errStage = batchFlushStageExec
	ps, _ := newTestBatchingPubsub(t, sender, BatchingConfig{Clock: quartz.NewMock(t)})

	batch := []queuedPublish{{
		event:        "chat:stream:error",
		channelClass: batchChannelClass("chat:stream:error"),
		message:      []byte("error"),
	}}
	ps.queuedCount.Store(int64(len(batch)))
	_, err := ps.flushBatch(context.Background(), batch, batchFlushScheduled)
	require.Error(t, err)
	require.ErrorContains(t, err, context.DeadlineExceeded.Error())
	require.ErrorContains(t, err, `delegate publish "chat:stream:error"`)
	require.Equal(t, float64(1), prom_testutil.ToFloat64(ps.metrics.DelegateFallbacksTotal.WithLabelValues(batchChannelClassStreamNotify, batchDelegateFallbackReasonFlushError, batchFlushStageExec)))
}

func newTestBatchingPubsub(t *testing.T, sender batchSender, cfg BatchingConfig) (*BatchingPubsub, *PGPubsub) {
	t.Helper()
	logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true}).Leveled(slog.LevelDebug)
	// Use a closed *sql.DB so that delegate.Publish returns a real
	// error instead of panicking on a nil pointer when the batching
	// queue falls back to the shared pool under pressure.
	closedDB := newClosedDB(t)
	delegate := newWithoutListener(logger.Named("delegate"), closedDB)
	ps, err := newBatchingPubsub(logger.Named("batcher"), delegate, sender, cfg)
	require.NoError(t, err)
	t.Cleanup(func() {
		_ = ps.Close()
	})
	return ps, delegate
}

// newClosedDB returns an *sql.DB whose connections have been closed,
// so any ExecContext call returns an error rather than panicking.
func newClosedDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("postgres", "host=localhost dbname=closed_db_stub sslmode=disable connect_timeout=1")
	require.NoError(t, err)
	require.NoError(t, db.Close())
	return db
}

type fakeBatchSender struct {
	mu        sync.Mutex
	batches   [][]queuedPublish
	flushes   chan []queuedPublish
	started   chan struct{}
	blockCh   chan struct{}
	err       error
	errStage  string
	closeErr  error
	closeCall int
}

func newFakeBatchSender() *fakeBatchSender {
	return &fakeBatchSender{
		flushes: make(chan []queuedPublish, 16),
		started: make(chan struct{}, 16),
	}
}

func (s *fakeBatchSender) Flush(ctx context.Context, batch []queuedPublish) error {
	select {
	case s.started <- struct{}{}:
	default:
	}
	if s.blockCh != nil {
		select {
		case <-s.blockCh:
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	clone := make([]queuedPublish, len(batch))
	for i, item := range batch {
		clone[i] = queuedPublish{
			event:   item.event,
			message: bytes.Clone(item.message),
		}
	}

	s.mu.Lock()
	s.batches = append(s.batches, clone)
	s.mu.Unlock()

	select {
	case s.flushes <- clone:
	default:
	}
	if s.err == nil {
		return nil
	}
	if s.errStage != "" {
		return &batchFlushError{stage: s.errStage, err: s.err}
	}
	return s.err
}

type metricWriter interface {
	Write(*dto.Metric) error
}

func histogramCountAndSum(t *testing.T, observer any) (uint64, float64) {
	t.Helper()
	writer, ok := observer.(metricWriter)
	require.True(t, ok)

	metric := &dto.Metric{}
	require.NoError(t, writer.Write(metric))
	histogram := metric.GetHistogram()
	require.NotNil(t, histogram)
	return histogram.GetSampleCount(), histogram.GetSampleSum()
}

func (s *fakeBatchSender) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.closeCall++
	return s.closeErr
}

func (s *fakeBatchSender) Batches() [][]queuedPublish {
	s.mu.Lock()
	defer s.mu.Unlock()
	clone := make([][]queuedPublish, len(s.batches))
	for i, batch := range s.batches {
		clone[i] = make([]queuedPublish, len(batch))
		copy(clone[i], batch)
	}
	return clone
}

func (s *fakeBatchSender) CloseCalls() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.closeCall
}
