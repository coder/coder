package pubsub

import (
	"bytes"
	"context"
	"sync"
	"testing"
	"time"

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
		BatchSize:     8,
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
	require.Equal(t, float64(2), prom_testutil.ToFloat64(ps.metrics.LogicalPublishesTotal.WithLabelValues(batchChannelClassStreamNotify, batchResultAccepted)))
	require.Zero(t, prom_testutil.ToFloat64(ps.metrics.LogicalPublishesTotal.WithLabelValues(batchChannelClassStreamNotify, batchResultRejected)))
	require.Equal(t, float64(6), prom_testutil.ToFloat64(ps.metrics.LogicalPublishBytesTotal.WithLabelValues(batchChannelClassStreamNotify)))
	require.Equal(t, float64(2), prom_testutil.ToFloat64(ps.metrics.QueueDepthHighWatermark))
	require.Equal(t, float64(2), prom_testutil.ToFloat64(ps.metrics.FlushedNotifications.WithLabelValues(batchFlushScheduled)))
	require.Equal(t, float64(6), prom_testutil.ToFloat64(ps.metrics.FlushedBytes.WithLabelValues(batchFlushScheduled)))
	queueWaitCount, queueWaitSum := histogramCountAndSum(t, ps.metrics.QueueWait.WithLabelValues(batchChannelClassStreamNotify))
	require.Equal(t, uint64(2), queueWaitCount)
	require.InDelta(t, 0.02, queueWaitSum, 0.000001)
	require.Equal(t, float64(1), prom_testutil.ToFloat64(ps.metrics.FlushAttemptsTotal.WithLabelValues(batchFlushStageBegin, batchResultSuccess)))
	require.Equal(t, float64(1), prom_testutil.ToFloat64(ps.metrics.FlushAttemptsTotal.WithLabelValues(batchFlushStageExec, batchResultSuccess)))
	require.Equal(t, float64(1), prom_testutil.ToFloat64(ps.metrics.FlushAttemptsTotal.WithLabelValues(batchFlushStageCommit, batchResultSuccess)))
	require.Equal(t, float64(1), prom_testutil.ToFloat64(ps.metrics.FlushesTotal.WithLabelValues(batchFlushScheduled)))
	require.Zero(t, prom_testutil.ToFloat64(ps.metrics.FlushInflight))
	require.Zero(t, prom_testutil.ToFloat64(ps.metrics.QueueDepth))
}

func TestBatchingPubsubDefaultConfigUsesDedicatedSenderFirstDefaults(t *testing.T) {
	t.Parallel()

	clock := quartz.NewMock(t)
	sender := newFakeBatchSender()
	ps, _ := newTestBatchingPubsub(t, sender, BatchingConfig{Clock: clock})

	require.Equal(t, DefaultBatchingFlushInterval, ps.flushInterval)
	require.Equal(t, DefaultBatchingBatchSize, ps.batchSize)
	require.Equal(t, DefaultBatchingQueueSize, cap(ps.publishCh))
	require.Equal(t, defaultBatchingPressureWait, ps.pressureWait)
	require.Equal(t, defaultBatchingFinalFlushLimit, ps.finalFlushTimeout)
	require.Equal(t, float64(DefaultBatchingQueueSize), prom_testutil.ToFloat64(ps.metrics.QueueCapacity))
	require.Zero(t, prom_testutil.ToFloat64(ps.metrics.QueueDepthHighWatermark))
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

func TestBatchingPubsubCapacityFlush(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitShort)
	clock := quartz.NewMock(t)
	newTimerTrap := clock.Trap().NewTimer("pubsubBatcher", "scheduledFlush")
	defer newTimerTrap.Close()
	resetTrap := clock.Trap().TimerReset("pubsubBatcher", "capacityFlush")
	defer resetTrap.Close()

	sender := newFakeBatchSender()
	ps, _ := newTestBatchingPubsub(t, sender, BatchingConfig{
		Clock:         clock,
		FlushInterval: time.Hour,
		BatchSize:     3,
		QueueSize:     8,
	})

	call, err := newTimerTrap.Wait(ctx)
	require.NoError(t, err)
	call.MustRelease(ctx)

	require.NoError(t, ps.Publish("chat:stream:a", []byte("one")))
	require.NoError(t, ps.Publish("chat:stream:a", []byte("two")))
	require.NoError(t, ps.Publish("chat:stream:a", []byte("three")))

	resetCall, err := resetTrap.Wait(ctx)
	require.NoError(t, err)
	resetCall.MustRelease(ctx)

	batch := testutil.TryReceive(ctx, t, sender.flushes)
	require.Len(t, batch, 3)
	require.Equal(t, []byte("one"), batch[0].message)
	require.Equal(t, []byte("two"), batch[1].message)
	require.Equal(t, []byte("three"), batch[2].message)
	require.Equal(t, float64(3), prom_testutil.ToFloat64(ps.metrics.QueueDepthHighWatermark))
	require.Equal(t, float64(1), prom_testutil.ToFloat64(ps.metrics.FlushesTotal.WithLabelValues(batchFlushCapacity)))
	require.Zero(t, prom_testutil.ToFloat64(ps.metrics.QueueDepth))
}

func TestBatchingPubsubQueueCapFailure(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitShort)
	clock := quartz.NewMock(t)
	newTimerTrap := clock.Trap().NewTimer("pubsubBatcher", "scheduledFlush")
	defer newTimerTrap.Close()
	pressureTrap := clock.Trap().NewTimer("pubsubBatcher", "pressureWait")
	defer pressureTrap.Close()

	sender := newFakeBatchSender()
	sender.blockCh = make(chan struct{})
	ps, _ := newTestBatchingPubsub(t, sender, BatchingConfig{
		Clock:         clock,
		FlushInterval: time.Hour,
		BatchSize:     1,
		QueueSize:     1,
		PressureWait:  10 * time.Millisecond,
	})

	call, err := newTimerTrap.Wait(ctx)
	require.NoError(t, err)
	call.MustRelease(ctx)

	require.NoError(t, ps.Publish("chat:stream:a", []byte("one")))
	<-sender.started
	require.NoError(t, ps.Publish("chat:stream:a", []byte("two")))

	errCh := make(chan error, 1)
	go func() {
		errCh <- ps.Publish("chat:stream:a", []byte("three"))
	}()

	pressureCall, err := pressureTrap.Wait(ctx)
	require.NoError(t, err)
	pressureCall.MustRelease(ctx)
	clock.Advance(10 * time.Millisecond).MustWait(ctx)

	err = testutil.TryReceive(ctx, t, errCh)
	require.ErrorIs(t, err, ErrBatchingPubsubQueueFull)
	require.Equal(t, float64(2), prom_testutil.ToFloat64(ps.metrics.LogicalPublishesTotal.WithLabelValues(batchChannelClassStreamNotify, batchResultAccepted)))
	require.Equal(t, float64(1), prom_testutil.ToFloat64(ps.metrics.LogicalPublishesTotal.WithLabelValues(batchChannelClassStreamNotify, batchResultRejected)))
	require.Equal(t, float64(6), prom_testutil.ToFloat64(ps.metrics.LogicalPublishBytesTotal.WithLabelValues(batchChannelClassStreamNotify)))
	require.Equal(t, float64(2), prom_testutil.ToFloat64(ps.metrics.QueueDepthHighWatermark))
	require.Equal(t, float64(1), prom_testutil.ToFloat64(ps.metrics.PublishRejectionsTotal.WithLabelValues("queue_full")))

	close(sender.blockCh)
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
		BatchSize:     8,
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
	require.Equal(t, float64(3), prom_testutil.ToFloat64(ps.metrics.QueueDepthHighWatermark))
	require.Equal(t, float64(1), prom_testutil.ToFloat64(ps.metrics.FlushesTotal.WithLabelValues(batchFlushShutdown)))
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
		BatchSize:     2,
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
	resetTrap := clock.Trap().TimerReset("pubsubBatcher", "capacityFlush")
	defer resetTrap.Close()

	sender := newFakeBatchSender()
	sender.err = context.DeadlineExceeded
	sender.errStage = batchFlushStageExec
	ps, delegate := newTestBatchingPubsub(t, sender, BatchingConfig{
		Clock:         clock,
		FlushInterval: time.Hour,
		BatchSize:     1,
		QueueSize:     8,
	})

	call, err := newTimerTrap.Wait(ctx)
	require.NoError(t, err)
	call.MustRelease(ctx)

	require.NoError(t, ps.Publish("chat:stream:a", []byte("one")))
	resetCall, err := resetTrap.Wait(ctx)
	require.NoError(t, err)
	resetCall.MustRelease(ctx)

	require.Equal(t, float64(1), prom_testutil.ToFloat64(ps.metrics.FlushFailuresTotal.WithLabelValues(batchFlushCapacity)))
	require.Equal(t, float64(1), prom_testutil.ToFloat64(ps.metrics.FlushAttemptsTotal.WithLabelValues(batchFlushStageBegin, batchResultSuccess)))
	require.Equal(t, float64(1), prom_testutil.ToFloat64(ps.metrics.FlushAttemptsTotal.WithLabelValues(batchFlushStageExec, batchResultError)))
	require.Zero(t, prom_testutil.ToFloat64(ps.metrics.FlushAttemptsTotal.WithLabelValues(batchFlushStageCommit, batchResultSuccess)))
	require.Equal(t, float64(1), prom_testutil.ToFloat64(delegate.publishesTotal.WithLabelValues("false")))
	require.Zero(t, prom_testutil.ToFloat64(delegate.publishesTotal.WithLabelValues("true")))
	require.Zero(t, prom_testutil.ToFloat64(ps.metrics.QueueDepth))
}

func newTestBatchingPubsub(t *testing.T, sender batchSender, cfg BatchingConfig) (*BatchingPubsub, *PGPubsub) {
	t.Helper()
	logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true}).Leveled(slog.LevelDebug)
	delegate := newWithoutListener(logger.Named("delegate"), nil)
	ps, err := newBatchingPubsub(logger.Named("batcher"), delegate, sender, cfg)
	require.NoError(t, err)
	t.Cleanup(func() {
		_ = ps.Close()
	})
	return ps, delegate
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
