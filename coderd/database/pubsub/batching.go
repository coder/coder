package pubsub

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/lib/pq"
	"github.com/prometheus/client_golang/prometheus"
	"golang.org/x/xerrors"

	"cdr.dev/slog/v3"
	"github.com/coder/quartz"
)

const (
	// DefaultBatchingFlushInterval is the default upper bound on how long chatd
	// publishes wait before a scheduled flush when capacity does not trigger an
	// immediate send.
	DefaultBatchingFlushInterval = 10 * time.Millisecond
	// DefaultBatchingBatchSize keeps the dedicated sender path effectively
	// unbatched by default so chatd avoids shared-pool starvation without adding
	// intentional queue dwell in the common path.
	DefaultBatchingBatchSize = 1
	// DefaultBatchingQueueSize is the default number of buffered chatd publish
	// requests waiting to be flushed.
	DefaultBatchingQueueSize = 1024

	defaultBatchingPressureWait    = 10 * time.Millisecond
	defaultBatchingFinalFlushLimit = 15 * time.Second
	batchingWarnInterval           = 10 * time.Second

	batchFlushCapacity  = "capacity"
	batchFlushScheduled = "scheduled"
	batchFlushPressure  = "pressure"
	batchFlushShutdown  = "shutdown"

	batchFlushStageBegin   = "begin"
	batchFlushStageExec    = "exec"
	batchFlushStageCommit  = "commit"
	batchFlushStageUnknown = "unknown"

	batchResultAccepted = "accepted"
	batchResultRejected = "rejected"
	batchResultSuccess  = "success"
	batchResultError    = "error"

	batchChannelClassStreamNotify = "stream_notify"
	batchChannelClassOwnerEvent   = "owner_event"
	batchChannelClassConfigChange = "config_change"
	batchChannelClassOther        = "other"
)

var (
	// ErrBatchingPubsubClosed is returned when a batched pubsub publish is
	// attempted after shutdown has started.
	ErrBatchingPubsubClosed = xerrors.New("batched pubsub is closed")
	// ErrBatchingPubsubQueueFull is returned when the bounded batching queue
	// stays full even after triggering an immediate flush and waiting briefly for
	// capacity to free up.
	ErrBatchingPubsubQueueFull = xerrors.New("batched pubsub queue is full")
)

// BatchingConfig controls the chatd-specific PostgreSQL pubsub batching path.
type BatchingConfig struct {
	FlushInterval     time.Duration
	BatchSize         int
	QueueSize         int
	PressureWait      time.Duration
	FinalFlushTimeout time.Duration
	Clock             quartz.Clock
}

type queuedPublish struct {
	event        string
	channelClass string
	message      []byte
	enqueuedAt   time.Time
}

type batchSender interface {
	Flush(ctx context.Context, batch []queuedPublish) error
	Close() error
}

type batchFlushError struct {
	stage string
	err   error
}

func (e *batchFlushError) Error() string {
	return e.err.Error()
}

func (e *batchFlushError) Unwrap() error {
	return e.err
}

// BatchingPubsub batches chatd publish traffic onto a dedicated PostgreSQL
// sender connection while delegating subscribe behavior to the shared listener
// pubsub instance.
type BatchingPubsub struct {
	logger   slog.Logger
	delegate *PGPubsub
	sender   batchSender
	clock    quartz.Clock

	publishCh chan queuedPublish
	flushCh   chan struct{}
	closeCh   chan struct{}
	doneCh    chan struct{}

	spaceMu     sync.Mutex
	spaceSignal chan struct{}

	warnTicker *quartz.Ticker

	flushInterval     time.Duration
	batchSize         int
	pressureWait      time.Duration
	finalFlushTimeout time.Duration

	queuedCount             atomic.Int64
	queueDepthHighWatermark atomic.Int64
	closed                  atomic.Bool
	closeOnce               sync.Once
	closeErr                error
	runErr                  error

	runCtx  context.Context
	cancel  context.CancelFunc
	metrics batchingMetrics
}

type batchingMetrics struct {
	QueueDepth               prometheus.Gauge
	QueueDepthHighWatermark  prometheus.Gauge
	QueueCapacity            prometheus.Gauge
	BatchSize                prometheus.Histogram
	LogicalPublishesTotal    *prometheus.CounterVec
	LogicalPublishBytesTotal *prometheus.CounterVec
	FlushedNotifications     *prometheus.CounterVec
	FlushedBytes             *prometheus.CounterVec
	QueueWait                *prometheus.HistogramVec
	FlushesTotal             *prometheus.CounterVec
	FlushDuration            *prometheus.HistogramVec
	FlushAttemptsTotal       *prometheus.CounterVec
	FlushFailuresTotal       *prometheus.CounterVec
	PublishRejectionsTotal   *prometheus.CounterVec
	FlushInflight            prometheus.Gauge
}

func newBatchingMetrics() batchingMetrics {
	return batchingMetrics{
		QueueDepth: prometheus.NewGauge(prometheus.GaugeOpts{
			Namespace: "coder",
			Subsystem: "pubsub",
			Name:      "batch_queue_depth",
			Help:      "The number of chatd notifications waiting in the batching queue.",
		}),
		QueueDepthHighWatermark: prometheus.NewGauge(prometheus.GaugeOpts{
			Namespace: "coder",
			Subsystem: "pubsub",
			Name:      "batch_queue_depth_high_watermark",
			Help:      "The highest chatd batching queue depth observed since process start.",
		}),
		QueueCapacity: prometheus.NewGauge(prometheus.GaugeOpts{
			Namespace: "coder",
			Subsystem: "pubsub",
			Name:      "batch_queue_capacity",
			Help:      "The configured capacity of the chatd batching queue.",
		}),
		BatchSize: prometheus.NewHistogram(prometheus.HistogramOpts{
			Namespace: "coder",
			Subsystem: "pubsub",
			Name:      "batch_size",
			Help:      "The number of logical notifications sent in each chatd batch flush.",
			Buckets:   []float64{1, 2, 4, 8, 16, 32, 64, 128, 256},
		}),
		LogicalPublishesTotal: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: "coder",
			Subsystem: "pubsub",
			Name:      "batch_logical_publishes_total",
			Help:      "The number of logical chatd publishes seen by the batching wrapper by channel class and result.",
		}, []string{"channel_class", "result"}),
		LogicalPublishBytesTotal: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: "coder",
			Subsystem: "pubsub",
			Name:      "batch_logical_publish_bytes_total",
			Help:      "The number of accepted chatd payload bytes enqueued into the batching wrapper by channel class.",
		}, []string{"channel_class"}),
		FlushedNotifications: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: "coder",
			Subsystem: "pubsub",
			Name:      "batch_flushed_notifications_total",
			Help:      "The number of logical chatd notifications removed from the batching queue for flush attempts by reason.",
		}, []string{"reason"}),
		FlushedBytes: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: "coder",
			Subsystem: "pubsub",
			Name:      "batch_flushed_bytes_total",
			Help:      "The number of chatd payload bytes removed from the batching queue for flush attempts by reason.",
		}, []string{"reason"}),
		QueueWait: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Namespace: "coder",
			Subsystem: "pubsub",
			Name:      "batch_queue_wait_seconds",
			Help:      "The time accepted chatd publishes spent waiting in the batching queue before a flush attempt started.",
			Buckets:   []float64{0.0001, 0.00025, 0.0005, 0.001, 0.0025, 0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1},
		}, []string{"channel_class"}),
		FlushesTotal: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: "coder",
			Subsystem: "pubsub",
			Name:      "batch_flushes_total",
			Help:      "The number of chatd batch flush attempts by reason.",
		}, []string{"reason"}),
		FlushDuration: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Namespace: "coder",
			Subsystem: "pubsub",
			Name:      "batch_flush_duration_seconds",
			Help:      "The time spent flushing one chatd batch to PostgreSQL.",
			Buckets:   prometheus.DefBuckets,
		}, []string{"reason"}),
		FlushAttemptsTotal: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: "coder",
			Subsystem: "pubsub",
			Name:      "batch_flush_attempts_total",
			Help:      "The number of chatd sender flush stages by stage and result.",
		}, []string{"stage", "result"}),
		FlushFailuresTotal: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: "coder",
			Subsystem: "pubsub",
			Name:      "batch_flush_failures_total",
			Help:      "The number of failed chatd batch flushes by reason.",
		}, []string{"reason"}),
		PublishRejectionsTotal: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: "coder",
			Subsystem: "pubsub",
			Name:      "batch_publish_rejections_total",
			Help:      "The number of chatd publishes rejected by the batching queue.",
		}, []string{"reason"}),
		FlushInflight: prometheus.NewGauge(prometheus.GaugeOpts{
			Namespace: "coder",
			Subsystem: "pubsub",
			Name:      "batch_flush_inflight",
			Help:      "Whether a chatd batch flush is currently executing against the dedicated sender connection.",
		}),
	}
}

func (m batchingMetrics) Describe(descs chan<- *prometheus.Desc) {
	m.QueueDepth.Describe(descs)
	m.QueueDepthHighWatermark.Describe(descs)
	m.QueueCapacity.Describe(descs)
	m.BatchSize.Describe(descs)
	m.LogicalPublishesTotal.Describe(descs)
	m.LogicalPublishBytesTotal.Describe(descs)
	m.FlushedNotifications.Describe(descs)
	m.FlushedBytes.Describe(descs)
	m.QueueWait.Describe(descs)
	m.FlushesTotal.Describe(descs)
	m.FlushDuration.Describe(descs)
	m.FlushAttemptsTotal.Describe(descs)
	m.FlushFailuresTotal.Describe(descs)
	m.PublishRejectionsTotal.Describe(descs)
	m.FlushInflight.Describe(descs)
}

func (m batchingMetrics) Collect(metrics chan<- prometheus.Metric) {
	m.QueueDepth.Collect(metrics)
	m.QueueDepthHighWatermark.Collect(metrics)
	m.QueueCapacity.Collect(metrics)
	m.BatchSize.Collect(metrics)
	m.LogicalPublishesTotal.Collect(metrics)
	m.LogicalPublishBytesTotal.Collect(metrics)
	m.FlushedNotifications.Collect(metrics)
	m.FlushedBytes.Collect(metrics)
	m.QueueWait.Collect(metrics)
	m.FlushesTotal.Collect(metrics)
	m.FlushDuration.Collect(metrics)
	m.FlushAttemptsTotal.Collect(metrics)
	m.FlushFailuresTotal.Collect(metrics)
	m.PublishRejectionsTotal.Collect(metrics)
	m.FlushInflight.Collect(metrics)
}

// NewBatching creates a chatd-specific batched pubsub wrapper around the
// shared PostgreSQL listener implementation.
func NewBatching(
	ctx context.Context,
	logger slog.Logger,
	delegate *PGPubsub,
	prototype *sql.DB,
	connectURL string,
	cfg BatchingConfig,
) (*BatchingPubsub, error) {
	if delegate == nil {
		return nil, xerrors.New("delegate pubsub is nil")
	}
	if prototype == nil {
		return nil, xerrors.New("prototype database is nil")
	}
	if connectURL == "" {
		return nil, xerrors.New("connect URL is empty")
	}

	sender, err := newPGBatchSender(ctx, logger.Named("sender"), prototype, connectURL)
	if err != nil {
		return nil, err
	}

	ps, err := newBatchingPubsub(logger, delegate, sender, cfg)
	if err != nil {
		_ = sender.Close()
		return nil, err
	}
	return ps, nil
}

func newBatchingPubsub(
	logger slog.Logger,
	delegate *PGPubsub,
	sender batchSender,
	cfg BatchingConfig,
) (*BatchingPubsub, error) {
	if delegate == nil {
		return nil, xerrors.New("delegate pubsub is nil")
	}
	if sender == nil {
		return nil, xerrors.New("batch sender is nil")
	}

	flushInterval := cfg.FlushInterval
	if flushInterval == 0 {
		flushInterval = DefaultBatchingFlushInterval
	}
	if flushInterval < 0 {
		return nil, xerrors.New("flush interval must be positive")
	}

	batchSize := cfg.BatchSize
	if batchSize == 0 {
		batchSize = DefaultBatchingBatchSize
	}
	if batchSize < 0 {
		return nil, xerrors.New("batch size must be positive")
	}

	queueSize := cfg.QueueSize
	if queueSize == 0 {
		queueSize = DefaultBatchingQueueSize
	}
	if queueSize < 0 {
		return nil, xerrors.New("queue size must be positive")
	}

	pressureWait := cfg.PressureWait
	if pressureWait == 0 {
		pressureWait = defaultBatchingPressureWait
	}
	if pressureWait < 0 {
		return nil, xerrors.New("pressure wait must be positive")
	}

	finalFlushTimeout := cfg.FinalFlushTimeout
	if finalFlushTimeout == 0 {
		finalFlushTimeout = defaultBatchingFinalFlushLimit
	}
	if finalFlushTimeout < 0 {
		return nil, xerrors.New("final flush timeout must be positive")
	}

	clock := cfg.Clock
	if clock == nil {
		clock = quartz.NewReal()
	}

	runCtx, cancel := context.WithCancel(context.Background())
	ps := &BatchingPubsub{
		logger:            logger,
		delegate:          delegate,
		sender:            sender,
		clock:             clock,
		publishCh:         make(chan queuedPublish, queueSize),
		flushCh:           make(chan struct{}, 1),
		closeCh:           make(chan struct{}),
		doneCh:            make(chan struct{}),
		spaceSignal:       make(chan struct{}),
		warnTicker:        clock.NewTicker(batchingWarnInterval, "pubsubBatcher", "warn"),
		flushInterval:     flushInterval,
		batchSize:         batchSize,
		pressureWait:      pressureWait,
		finalFlushTimeout: finalFlushTimeout,
		runCtx:            runCtx,
		cancel:            cancel,
		metrics:           newBatchingMetrics(),
	}
	ps.metrics.QueueDepth.Set(0)
	ps.metrics.QueueDepthHighWatermark.Set(0)
	ps.metrics.QueueCapacity.Set(float64(queueSize))
	ps.metrics.FlushInflight.Set(0)

	go ps.run()
	return ps, nil
}

// Describe implements prometheus.Collector.
func (p *BatchingPubsub) Describe(descs chan<- *prometheus.Desc) {
	p.metrics.Describe(descs)
}

// Collect implements prometheus.Collector.
func (p *BatchingPubsub) Collect(metrics chan<- prometheus.Metric) {
	p.metrics.Collect(metrics)
}

// Subscribe delegates to the shared PostgreSQL listener pubsub.
func (p *BatchingPubsub) Subscribe(event string, listener Listener) (func(), error) {
	return p.delegate.Subscribe(event, listener)
}

// SubscribeWithErr delegates to the shared PostgreSQL listener pubsub.
func (p *BatchingPubsub) SubscribeWithErr(event string, listener ListenerWithErr) (func(), error) {
	return p.delegate.SubscribeWithErr(event, listener)
}

// Publish enqueues a logical notification for asynchronous batched delivery.
func (p *BatchingPubsub) Publish(event string, message []byte) error {
	channelClass := batchChannelClass(event)
	if p.closed.Load() {
		p.observeRejectedPublish(channelClass, "closed")
		return ErrBatchingPubsubClosed
	}

	req := queuedPublish{
		event:        event,
		channelClass: channelClass,
		message:      bytes.Clone(message),
	}
	req.enqueuedAt = p.clock.Now()
	if p.tryEnqueue(req) {
		p.observeAcceptedPublish(req)
		return nil
	}

	timer := p.clock.NewTimer(p.pressureWait, "pubsubBatcher", "pressureWait")
	defer timer.Stop("pubsubBatcher", "pressureWait")

	for {
		if p.closed.Load() {
			p.observeRejectedPublish(channelClass, "closed")
			return ErrBatchingPubsubClosed
		}
		p.signalPressureFlush()
		spaceSignal := p.currentSpaceSignal()
		req.enqueuedAt = p.clock.Now()
		if p.tryEnqueue(req) {
			p.observeAcceptedPublish(req)
			return nil
		}

		select {
		case <-spaceSignal:
			continue
		case <-timer.C:
			req.enqueuedAt = p.clock.Now()
			if p.tryEnqueue(req) {
				p.observeAcceptedPublish(req)
				return nil
			}
			p.observeRejectedPublish(channelClass, "queue_full")
			p.logPublishRejection(event)
			return ErrBatchingPubsubQueueFull
		case <-p.doneCh:
			p.observeRejectedPublish(channelClass, "closed")
			return ErrBatchingPubsubClosed
		}
	}
}

// Close stops accepting new publishes, performs a bounded best-effort drain,
// and then closes the dedicated sender connection.
func (p *BatchingPubsub) Close() error {
	p.closeOnce.Do(func() {
		p.closed.Store(true)
		p.cancel()
		p.notifySpaceAvailable()
		close(p.closeCh)
		<-p.doneCh
		p.closeErr = p.runErr
	})
	return p.closeErr
}

func (p *BatchingPubsub) tryEnqueue(req queuedPublish) bool {
	if p.closed.Load() {
		return false
	}
	select {
	case p.publishCh <- req:
		queuedDepth := p.queuedCount.Add(1)
		p.observeQueueDepth(queuedDepth)
		return true
	default:
		return false
	}
}

func (p *BatchingPubsub) observeQueueDepth(depth int64) {
	p.metrics.QueueDepth.Set(float64(depth))
	for {
		currentMax := p.queueDepthHighWatermark.Load()
		if depth <= currentMax {
			return
		}
		if p.queueDepthHighWatermark.CompareAndSwap(currentMax, depth) {
			p.metrics.QueueDepthHighWatermark.Set(float64(depth))
			return
		}
	}
}

func (p *BatchingPubsub) signalPressureFlush() {
	select {
	case p.flushCh <- struct{}{}:
	default:
	}
}

func (p *BatchingPubsub) currentSpaceSignal() <-chan struct{} {
	p.spaceMu.Lock()
	defer p.spaceMu.Unlock()
	return p.spaceSignal
}

func (p *BatchingPubsub) notifySpaceAvailable() {
	p.spaceMu.Lock()
	defer p.spaceMu.Unlock()
	close(p.spaceSignal)
	p.spaceSignal = make(chan struct{})
}

func batchChannelClass(event string) string {
	switch {
	case strings.HasPrefix(event, "chat:stream:"):
		return batchChannelClassStreamNotify
	case strings.HasPrefix(event, "chat:owner:"):
		return batchChannelClassOwnerEvent
	case event == "chat:config_change":
		return batchChannelClassConfigChange
	default:
		return batchChannelClassOther
	}
}

func (p *BatchingPubsub) observeAcceptedPublish(req queuedPublish) {
	p.metrics.LogicalPublishesTotal.WithLabelValues(req.channelClass, batchResultAccepted).Inc()
	p.metrics.LogicalPublishBytesTotal.WithLabelValues(req.channelClass).Add(float64(len(req.message)))
}

func (p *BatchingPubsub) observeRejectedPublish(channelClass string, reason string) {
	p.metrics.LogicalPublishesTotal.WithLabelValues(channelClass, batchResultRejected).Inc()
	p.metrics.PublishRejectionsTotal.WithLabelValues(reason).Inc()
}

func (p *BatchingPubsub) observeFlushStageResults(err error) {
	stage := batchFlushStageCommit
	if err != nil {
		var flushErr *batchFlushError
		if errors.As(err, &flushErr) {
			stage = flushErr.stage
		} else {
			stage = batchFlushStageUnknown
		}
	}

	switch stage {
	case batchFlushStageBegin:
		p.metrics.FlushAttemptsTotal.WithLabelValues(batchFlushStageBegin, batchResultError).Inc()
		return
	case batchFlushStageExec:
		p.metrics.FlushAttemptsTotal.WithLabelValues(batchFlushStageBegin, batchResultSuccess).Inc()
		p.metrics.FlushAttemptsTotal.WithLabelValues(batchFlushStageExec, batchResultError).Inc()
		return
	case batchFlushStageCommit:
		p.metrics.FlushAttemptsTotal.WithLabelValues(batchFlushStageBegin, batchResultSuccess).Inc()
		p.metrics.FlushAttemptsTotal.WithLabelValues(batchFlushStageExec, batchResultSuccess).Inc()
		result := batchResultSuccess
		if err != nil {
			result = batchResultError
		}
		p.metrics.FlushAttemptsTotal.WithLabelValues(batchFlushStageCommit, result).Inc()
		return
	default:
		p.metrics.FlushAttemptsTotal.WithLabelValues(batchFlushStageUnknown, batchResultError).Inc()
		return
	}
}

func (p *BatchingPubsub) run() {
	defer close(p.doneCh)
	defer p.warnTicker.Stop("pubsubBatcher", "warn")

	batch := make([]queuedPublish, 0, p.batchSize)
	timer := p.clock.NewTimer(p.flushInterval, "pubsubBatcher", "scheduledFlush")
	defer timer.Stop("pubsubBatcher", "scheduledFlush")

	for {
		select {
		case item := <-p.publishCh:
			batch = append(batch, item)
			p.notifySpaceAvailable()
			if len(batch) >= p.batchSize {
				count := len(batch)
				var err error
				batch, err = p.flushBatch(p.runCtx, batch, batchFlushCapacity)
				if err != nil {
					p.logger.Error(context.Background(), "failed to flush batched pubsub messages",
						slog.F("reason", batchFlushCapacity),
						slog.F("count", count),
						slog.Error(err),
					)
				}
				timer.Reset(p.flushInterval, "pubsubBatcher", "capacityFlush")
			}
		case <-timer.C:
			batch = p.drainIntoBatch(batch)
			count := len(batch)
			var err error
			batch, err = p.flushBatch(p.runCtx, batch, batchFlushScheduled)
			if err != nil {
				p.logger.Error(context.Background(), "failed to flush batched pubsub messages",
					slog.F("reason", batchFlushScheduled),
					slog.F("count", count),
					slog.Error(err),
				)
			}
			timer.Reset(p.flushInterval, "pubsubBatcher", "scheduledFlush")
		case <-p.flushCh:
			batch = p.drainIntoBatch(batch)
			count := len(batch)
			var err error
			batch, err = p.flushBatch(p.runCtx, batch, batchFlushPressure)
			if err != nil {
				p.logger.Error(context.Background(), "failed to flush batched pubsub messages",
					slog.F("reason", batchFlushPressure),
					slog.F("count", count),
					slog.Error(err),
				)
			}
			timer.Reset(p.flushInterval, "pubsubBatcher", "pressureFlush")
		case <-p.closeCh:
			p.runErr = errors.Join(p.drain(batch), p.sender.Close())
			return
		}
	}
}

func (p *BatchingPubsub) drainIntoBatch(batch []queuedPublish) []queuedPublish {
	drained := false
	for len(batch) < p.batchSize {
		select {
		case item := <-p.publishCh:
			batch = append(batch, item)
			drained = true
		default:
			if drained {
				p.notifySpaceAvailable()
			}
			return batch
		}
	}
	if drained {
		p.notifySpaceAvailable()
	}
	return batch
}

func (p *BatchingPubsub) flushBatch(
	ctx context.Context,
	batch []queuedPublish,
	reason string,
) ([]queuedPublish, error) {
	if len(batch) == 0 {
		return batch[:0], nil
	}

	count := len(batch)
	totalBytes := 0
	start := p.clock.Now()
	for _, item := range batch {
		totalBytes += len(item.message)
		queueWait := start.Sub(item.enqueuedAt)
		if queueWait < 0 {
			queueWait = 0
		}
		p.metrics.QueueWait.WithLabelValues(item.channelClass).Observe(queueWait.Seconds())
	}

	p.metrics.FlushesTotal.WithLabelValues(reason).Inc()
	p.metrics.BatchSize.Observe(float64(count))
	p.metrics.FlushedNotifications.WithLabelValues(reason).Add(float64(count))
	p.metrics.FlushedBytes.WithLabelValues(reason).Add(float64(totalBytes))
	p.metrics.FlushInflight.Set(1)
	err := p.sender.Flush(ctx, batch)
	p.metrics.FlushInflight.Set(0)
	p.observeFlushStageResults(err)
	elapsed := p.clock.Since(start)
	p.metrics.FlushDuration.WithLabelValues(reason).Observe(elapsed.Seconds())
	if err != nil {
		p.metrics.FlushFailuresTotal.WithLabelValues(reason).Inc()
		if p.delegate != nil {
			p.delegate.publishesTotal.WithLabelValues("false").Add(float64(count))
		}
	} else if p.delegate != nil {
		p.delegate.publishesTotal.WithLabelValues("true").Add(float64(count))
		p.delegate.publishedBytesTotal.Add(float64(totalBytes))
	}

	queuedDepth := p.queuedCount.Add(-int64(count))
	p.observeQueueDepth(queuedDepth)
	clear(batch)
	return batch[:0], err
}

func (p *BatchingPubsub) drain(batch []queuedPublish) error {
	ctx, cancel := context.WithTimeout(context.Background(), p.finalFlushTimeout)
	defer cancel()

	var errs []error
	for {
		batch = p.drainIntoBatch(batch)
		if len(batch) == 0 {
			break
		}
		var err error
		batch, err = p.flushBatch(ctx, batch, batchFlushShutdown)
		if err != nil {
			errs = append(errs, err)
		}
		if ctx.Err() != nil {
			break
		}
	}

	dropped := p.dropPendingPublishes()
	if dropped > 0 {
		errs = append(errs, xerrors.Errorf("dropped %d queued notifications during shutdown", dropped))
	}
	if ctx.Err() != nil {
		errs = append(errs, xerrors.Errorf("shutdown flush timed out: %w", ctx.Err()))
	}
	return errors.Join(errs...)
}

func (p *BatchingPubsub) dropPendingPublishes() int {
	count := 0
	for {
		select {
		case <-p.publishCh:
			count++
		default:
			if count > 0 {
				queuedDepth := p.queuedCount.Add(-int64(count))
				p.observeQueueDepth(queuedDepth)
			}
			return count
		}
	}
}

func (p *BatchingPubsub) logPublishRejection(event string) {
	fields := []slog.Field{
		slog.F("event", event),
		slog.F("queue_size", cap(p.publishCh)),
		slog.F("queued", p.queuedCount.Load()),
	}
	select {
	case <-p.warnTicker.C:
		p.logger.Warn(context.Background(), "batched pubsub queue is full", fields...)
	default:
		p.logger.Debug(context.Background(), "batched pubsub queue is full", fields...)
	}
}

type pgBatchSender struct {
	logger slog.Logger
	db     *sql.DB
}

func newPGBatchSender(
	ctx context.Context,
	logger slog.Logger,
	prototype *sql.DB,
	connectURL string,
) (*pgBatchSender, error) {
	connector, err := newConnector(ctx, logger, prototype, connectURL)
	if err != nil {
		return nil, err
	}

	db := sql.OpenDB(connector)
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	db.SetConnMaxIdleTime(0)
	db.SetConnMaxLifetime(0)

	pingCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	if err := db.PingContext(pingCtx); err != nil {
		_ = db.Close()
		return nil, xerrors.Errorf("ping batched pubsub sender database: %w", err)
	}

	return &pgBatchSender{logger: logger, db: db}, nil
}

func (s *pgBatchSender) Flush(ctx context.Context, batch []queuedPublish) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return &batchFlushError{stage: batchFlushStageBegin, err: xerrors.Errorf("begin batched pubsub transaction: %w", err)}
	}
	committed := false
	defer func() {
		if !committed {
			_ = tx.Rollback()
		}
	}()

	for _, item := range batch {
		// This is safe because we are calling pq.QuoteLiteral. pg_notify does
		// not support the first parameter being a prepared statement.
		//nolint:gosec
		_, err = tx.ExecContext(ctx, `select pg_notify(`+pq.QuoteLiteral(item.event)+`, $1)`, item.message)
		if err != nil {
			return &batchFlushError{stage: batchFlushStageExec, err: xerrors.Errorf("exec pg_notify: %w", err)}
		}
	}

	if err := tx.Commit(); err != nil {
		return &batchFlushError{stage: batchFlushStageCommit, err: xerrors.Errorf("commit batched pubsub transaction: %w", err)}
	}
	committed = true
	return nil
}

func (s *pgBatchSender) Close() error {
	return s.db.Close()
}
