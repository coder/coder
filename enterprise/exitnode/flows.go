package exitnode

import (
	"context"
	"sync"
	"time"

	"golang.org/x/xerrors"

	"cdr.dev/slog/v3"
	"github.com/coder/quartz"

	"github.com/coder/coder/v2/codersdk"
)

const (
	defaultFlowFlushInterval = 2 * time.Second
	defaultFlowBatchSize     = 500
	defaultFlowMaxQueue      = 10000
	defaultFlowMinBackoff    = time.Second
	defaultFlowMaxBackoff    = 30 * time.Second
	defaultFlowSendTimeout   = 10 * time.Second
)

// FlowClient is the subset of exitnodesdk.Client the reporter needs.
type FlowClient interface {
	ReportFlows(ctx context.Context, req codersdk.ReportExitNodeFlowsRequest) error
}

// FlowRecorder accepts flow reports for asynchronous delivery.
type FlowRecorder interface {
	Record(codersdk.ExitNodeFlowReport)
}

// FlowReporterOptions configures a FlowReporter. Zero values take defaults.
type FlowReporterOptions struct {
	Logger slog.Logger
	Client FlowClient
	// FlushInterval is the longest a report waits before being sent.
	FlushInterval time.Duration
	// BatchSize triggers an immediate flush when this many reports are
	// queued, and caps the number of reports per request.
	BatchSize int
	// MaxQueue bounds the number of unsent reports. When full, the oldest
	// report is dropped and counted in Metrics.
	MaxQueue int
	// Metrics is optional.
	Metrics *Metrics
	// Clock is for testing only.
	Clock quartz.Clock
}

// FlowReporter batches flow reports and delivers them to coderd. Delivery is
// retried with exponential backoff; unsent reports stay queued (bounded by
// MaxQueue) across failures.
type FlowReporter struct {
	logger  slog.Logger
	client  FlowClient
	metrics *Metrics
	clock   quartz.Clock

	flushInterval time.Duration
	batchSize     int
	maxQueue      int

	mu    sync.Mutex
	queue []codersdk.ExitNodeFlowReport
	// flushCh is signaled when the queue reaches batchSize.
	flushCh chan struct{}

	ctx    context.Context
	cancel context.CancelFunc
	done   chan struct{}
}

var _ FlowRecorder = (*FlowReporter)(nil)

// NewFlowReporter starts a reporter. Call Close to flush and stop it.
func NewFlowReporter(ctx context.Context, opts FlowReporterOptions) *FlowReporter {
	if opts.FlushInterval <= 0 {
		opts.FlushInterval = defaultFlowFlushInterval
	}
	if opts.BatchSize <= 0 {
		opts.BatchSize = defaultFlowBatchSize
	}
	if opts.MaxQueue <= 0 {
		opts.MaxQueue = defaultFlowMaxQueue
	}
	if opts.Clock == nil {
		opts.Clock = quartz.NewReal()
	}
	ctx, cancel := context.WithCancel(ctx)
	r := &FlowReporter{
		logger:        opts.Logger,
		client:        opts.Client,
		metrics:       opts.Metrics,
		clock:         opts.Clock,
		flushInterval: opts.FlushInterval,
		batchSize:     opts.BatchSize,
		maxQueue:      opts.MaxQueue,
		flushCh:       make(chan struct{}, 1),
		ctx:           ctx,
		cancel:        cancel,
		done:          make(chan struct{}),
	}
	go r.run()
	return r
}

// Record queues a report. It never blocks; when the queue is full the oldest
// report is dropped.
func (r *FlowReporter) Record(report codersdk.ExitNodeFlowReport) {
	r.mu.Lock()
	if len(r.queue) >= r.maxQueue {
		// Drop the oldest so recent activity is what survives a long outage.
		r.queue = r.queue[1:]
		if r.metrics != nil {
			r.metrics.FlowReportsDropped.Inc()
		}
	}
	r.queue = append(r.queue, report)
	full := len(r.queue) >= r.batchSize
	r.mu.Unlock()

	if full {
		select {
		case r.flushCh <- struct{}{}:
		default:
		}
	}
}

func (r *FlowReporter) run() {
	defer close(r.done)

	ticker := r.clock.NewTicker(r.flushInterval, "flowreporter", "flush")
	defer ticker.Stop()

	backoff := defaultFlowMinBackoff
	for {
		select {
		case <-r.ctx.Done():
			return
		case <-ticker.C:
		case <-r.flushCh:
		}

		for {
			sent, err := r.flushOnce(r.ctx)
			if err != nil {
				if r.ctx.Err() != nil {
					return
				}
				r.logger.Warn(r.ctx, "failed to report exit node flows; will retry",
					slog.F("backoff", backoff), slog.Error(err))
				timer := r.clock.NewTimer(backoff, "flowreporter", "backoff")
				select {
				case <-r.ctx.Done():
					timer.Stop()
					return
				case <-timer.C:
				}
				backoff = min(backoff*2, defaultFlowMaxBackoff)
				continue
			}
			backoff = defaultFlowMinBackoff
			if sent < r.batchSize {
				break
			}
			// A full batch went out; there may be more waiting.
		}
	}
}

// flushOnce sends up to one batch. It returns how many reports were sent. On
// failure the reports are put back at the head of the queue.
func (r *FlowReporter) flushOnce(ctx context.Context) (int, error) {
	r.mu.Lock()
	if len(r.queue) == 0 {
		r.mu.Unlock()
		return 0, nil
	}
	n := min(len(r.queue), r.batchSize)
	batch := make([]codersdk.ExitNodeFlowReport, n)
	copy(batch, r.queue[:n])
	r.queue = r.queue[n:]
	r.mu.Unlock()

	sendCtx, cancel := context.WithTimeout(ctx, defaultFlowSendTimeout)
	err := r.client.ReportFlows(sendCtx, codersdk.ReportExitNodeFlowsRequest{Flows: batch})
	cancel()
	if err != nil {
		r.mu.Lock()
		// Requeue at the head, then trim from the front if that overflowed.
		requeued := make([]codersdk.ExitNodeFlowReport, 0, len(batch)+len(r.queue))
		requeued = append(requeued, batch...)
		requeued = append(requeued, r.queue...)
		r.queue = requeued
		if over := len(r.queue) - r.maxQueue; over > 0 {
			r.queue = r.queue[over:]
			if r.metrics != nil {
				r.metrics.FlowReportsDropped.Add(float64(over))
			}
		}
		r.mu.Unlock()
		return 0, xerrors.Errorf("report %d flows: %w", n, err)
	}
	if r.metrics != nil {
		r.metrics.FlowReportsSent.Add(float64(n))
	}
	return n, nil
}

// Close stops the background loop and makes a final best-effort attempt to
// deliver whatever is queued, bounded by ctx.
func (r *FlowReporter) Close(ctx context.Context) error {
	r.cancel()
	<-r.done

	for {
		sent, err := r.flushOnce(ctx)
		if err != nil {
			return xerrors.Errorf("final flush: %w", err)
		}
		if sent == 0 {
			return nil
		}
	}
}
