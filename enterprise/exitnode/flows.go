package exitnode

import (
	"context"
	"slices"
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
	FlowReporterOptions

	mu    sync.Mutex
	queue []codersdk.ExitNodeFlowReport
	// flushCh is signaled when the queue reaches BatchSize.
	flushCh chan struct{}

	ctx    context.Context
	cancel context.CancelFunc
	done   chan struct{}
}

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
	if opts.Metrics == nil {
		opts.Metrics = NewMetrics(nil)
	}
	ctx, cancel := context.WithCancel(ctx)
	r := &FlowReporter{
		FlowReporterOptions: opts,
		flushCh:             make(chan struct{}, 1),
		ctx:                 ctx,
		cancel:              cancel,
		done:                make(chan struct{}),
	}
	go r.run()
	return r
}

// Record queues a report. It never blocks; when the queue is full the oldest
// report is dropped so recent activity is what survives a long outage.
func (r *FlowReporter) Record(report codersdk.ExitNodeFlowReport) {
	r.mu.Lock()
	r.queue = r.trimLocked(append(r.queue, report))
	full := len(r.queue) >= r.BatchSize
	r.mu.Unlock()
	if full {
		select {
		case r.flushCh <- struct{}{}:
		default:
		}
	}
}

// trimLocked drops the oldest reports beyond MaxQueue and counts them.
func (r *FlowReporter) trimLocked(queue []codersdk.ExitNodeFlowReport) []codersdk.ExitNodeFlowReport {
	if over := len(queue) - r.MaxQueue; over > 0 {
		r.Metrics.FlowReportsDropped.Add(float64(over))
		return queue[over:]
	}
	return queue
}

func (r *FlowReporter) run() {
	defer close(r.done)
	ticker := r.Clock.NewTicker(r.FlushInterval, "flowreporter", "flush")
	defer ticker.Stop()

	backoff := defaultFlowMinBackoff
	for {
		select {
		case <-r.ctx.Done():
			return
		case <-ticker.C:
		case <-r.flushCh:
		}
		// Keep sending while full batches go out; there may be more waiting.
		for {
			sent, err := r.flushOnce(r.ctx)
			if err == nil {
				backoff = defaultFlowMinBackoff
				if sent < r.BatchSize {
					break
				}
				continue
			}
			if r.ctx.Err() != nil {
				return
			}
			r.Logger.Warn(r.ctx, "failed to report exit node flows; will retry",
				slog.F("backoff", backoff), slog.Error(err))
			timer := r.Clock.NewTimer(backoff, "flowreporter", "backoff")
			select {
			case <-r.ctx.Done():
				timer.Stop()
				return
			case <-timer.C:
			}
			backoff = min(backoff*2, defaultFlowMaxBackoff)
		}
	}
}

// flushOnce sends up to one batch. It returns how many reports were sent. On
// failure the reports are put back at the head of the queue.
func (r *FlowReporter) flushOnce(ctx context.Context) (int, error) {
	r.mu.Lock()
	n := min(len(r.queue), r.BatchSize)
	batch := slices.Clone(r.queue[:n])
	r.queue = r.queue[n:]
	r.mu.Unlock()
	if n == 0 {
		return 0, nil
	}

	sendCtx, cancel := context.WithTimeout(ctx, defaultFlowSendTimeout)
	err := r.Client.ReportFlows(sendCtx, codersdk.ReportExitNodeFlowsRequest{Flows: batch})
	cancel()
	if err != nil {
		r.mu.Lock()
		r.queue = r.trimLocked(slices.Concat(batch, r.queue))
		r.mu.Unlock()
		return 0, xerrors.Errorf("report %d flows: %w", n, err)
	}
	r.Metrics.FlowReportsSent.Add(float64(n))
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
