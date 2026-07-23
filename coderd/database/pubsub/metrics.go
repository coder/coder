package pubsub

import (
	"context"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"

	"cdr.dev/slog/v3"
)

const (
	// BackendPostgres labels metrics produced by the PostgreSQL pubsub.
	BackendPostgres = "postgres"
	// BackendNATS labels metrics produced by the NATS pubsub.
	BackendNATS = "nats"
)

// latencyBuckets covers sub-millisecond round-trips up to ~16s.
var latencyBuckets = prometheus.ExponentialBuckets(0.0005, 2, 16)

// Metrics owns the Prometheus instruments shared by all pubsub backends. Each
// backend obtains a BackendMetrics handle via ForBackend that records into
// these instruments with its own `backend` label value.
type Metrics struct {
	publishesTotal      *prometheus.CounterVec
	subscribesTotal     *prometheus.CounterVec
	messagesTotal       *prometheus.CounterVec
	publishedBytesTotal *prometheus.CounterVec
	receivedBytesTotal  *prometheus.CounterVec
	disconnectionsTotal *prometheus.CounterVec
	connected           *prometheus.GaugeVec
	currentSubscribers  *prometheus.GaugeVec
	currentEvents       *prometheus.GaugeVec
	sendLatency         *prometheus.HistogramVec
	recvLatency         *prometheus.HistogramVec
	latencyMeasures     *prometheus.CounterVec
	latencyErrs         *prometheus.CounterVec
}

// NewMetrics builds the shared pubsub instruments, registering them with reg
// when it is non-nil and otherwise creating them unregistered (no-op).
func NewMetrics(reg prometheus.Registerer) *Metrics {
	factory := promauto.With(reg)
	return &Metrics{
		publishesTotal: factory.NewCounterVec(prometheus.CounterOpts{
			Namespace: "coder",
			Subsystem: "pubsub",
			Name:      "publishes_total",
			Help:      "Total number of calls to Publish",
		}, []string{"backend", "success"}),
		subscribesTotal: factory.NewCounterVec(prometheus.CounterOpts{
			Namespace: "coder",
			Subsystem: "pubsub",
			Name:      "subscribes_total",
			Help:      "Total number of calls to Subscribe/SubscribeWithErr",
		}, []string{"backend", "success"}),
		messagesTotal: factory.NewCounterVec(prometheus.CounterOpts{
			Namespace: "coder",
			Subsystem: "pubsub",
			Name:      "messages_total",
			Help:      "Total number of messages received",
		}, []string{"backend", "size"}),
		publishedBytesTotal: factory.NewCounterVec(prometheus.CounterOpts{
			Namespace: "coder",
			Subsystem: "pubsub",
			Name:      "published_bytes_total",
			Help:      "Total number of bytes successfully published across all publishes",
		}, []string{"backend"}),
		receivedBytesTotal: factory.NewCounterVec(prometheus.CounterOpts{
			Namespace: "coder",
			Subsystem: "pubsub",
			Name:      "received_bytes_total",
			Help:      "Total number of bytes received across all messages",
		}, []string{"backend"}),
		disconnectionsTotal: factory.NewCounterVec(prometheus.CounterOpts{
			Namespace: "coder",
			Subsystem: "pubsub",
			Name:      "disconnections_total",
			Help:      "Total number of times we disconnected unexpectedly from the backend",
		}, []string{"backend"}),
		connected: factory.NewGaugeVec(prometheus.GaugeOpts{
			Namespace: "coder",
			Subsystem: "pubsub",
			Name:      "connected",
			Help:      "Whether we are connected (1) or not connected (0) to the backend",
		}, []string{"backend"}),
		currentSubscribers: factory.NewGaugeVec(prometheus.GaugeOpts{
			Namespace: "coder",
			Subsystem: "pubsub",
			Name:      "current_subscribers",
			Help:      "The current number of active pubsub subscribers",
		}, []string{"backend"}),
		currentEvents: factory.NewGaugeVec(prometheus.GaugeOpts{
			Namespace: "coder",
			Subsystem: "pubsub",
			Name:      "current_events",
			Help:      "The current number of pubsub event channels listened for",
		}, []string{"backend"}),
		sendLatency: factory.NewHistogramVec(prometheus.HistogramOpts{
			Namespace: "coder",
			Subsystem: "pubsub",
			Name:      "send_latency_seconds",
			Help:      "The time taken to send a message into a pubsub event channel",
			Buckets:   latencyBuckets,
		}, []string{"backend"}),
		recvLatency: factory.NewHistogramVec(prometheus.HistogramOpts{
			Namespace: "coder",
			Subsystem: "pubsub",
			Name:      "receive_latency_seconds",
			Help:      "The time taken to receive a message from a pubsub event channel",
			Buckets:   latencyBuckets,
		}, []string{"backend"}),
		latencyMeasures: factory.NewCounterVec(prometheus.CounterOpts{
			Namespace: "coder",
			Subsystem: "pubsub",
			Name:      "latency_measures_total",
			Help:      "The number of pubsub latency measurements",
		}, []string{"backend"}),
		latencyErrs: factory.NewCounterVec(prometheus.CounterOpts{
			Namespace: "coder",
			Subsystem: "pubsub",
			Name:      "latency_measure_errs_total",
			Help:      "The number of pubsub latency measurement failures",
		}, []string{"backend"}),
	}
}

// BackendMetrics records into the shared Metrics with a fixed `backend`
// label value. Backends hold one of these instead of the shared Metrics so
// the only thing they can do is record their own series.
type BackendMetrics struct {
	logger          slog.Logger
	backend         string
	m               *Metrics
	latencyMeasurer *LatencyMeasurer

	// latencyCtx/latencyCancel drive the background latency loop; latencyDone
	// closes when it exits. StartLatencyLoop closes latencyStarted so
	// StopLatencyLoop knows whether a loop exists to wait for.
	latencyCtx     context.Context
	latencyCancel  context.CancelFunc
	latencyStarted chan struct{}
	latencyDone    chan struct{}
}

// ForBackend returns a recording handle bound to the given backend. Each handle
// has its own LatencyMeasurer so probe channels never clash across backends.
func (m *Metrics) ForBackend(logger slog.Logger, backend string) *BackendMetrics {
	// Pre-create the single-label series so they export 0 from the start.
	// Series with a second label (success/size) are created on first use.
	m.publishedBytesTotal.WithLabelValues(backend)
	m.receivedBytesTotal.WithLabelValues(backend)
	m.disconnectionsTotal.WithLabelValues(backend)
	m.connected.WithLabelValues(backend)
	m.currentSubscribers.WithLabelValues(backend)
	m.currentEvents.WithLabelValues(backend)
	m.latencyMeasures.WithLabelValues(backend)
	m.latencyErrs.WithLabelValues(backend)

	latencyCtx, latencyCancel := context.WithCancel(context.Background())
	return &BackendMetrics{
		logger:          logger,
		backend:         backend,
		m:               m,
		latencyMeasurer: NewLatencyMeasurer(logger.Named("latency-measurer")),
		latencyCtx:      latencyCtx,
		latencyCancel:   latencyCancel,
		latencyStarted:  make(chan struct{}),
		latencyDone:     make(chan struct{}),
	}
}

// RecordPublishSuccess records a successful Publish of n bytes.
func (b *BackendMetrics) RecordPublishSuccess(event string, n int) {
	if b.latencyMeasurer.isMeasurementChannel(event) {
		return
	}
	b.m.publishesTotal.WithLabelValues(b.backend, "true").Inc()
	b.m.publishedBytesTotal.WithLabelValues(b.backend).Add(float64(n))
}

// RecordPublishFailure records a failed Publish.
func (b *BackendMetrics) RecordPublishFailure(event string) {
	if b.latencyMeasurer.isMeasurementChannel(event) {
		return
	}
	b.m.publishesTotal.WithLabelValues(b.backend, "false").Inc()
}

// RecordSubscribeSuccess records a successful Subscribe/SubscribeWithErr.
func (b *BackendMetrics) RecordSubscribeSuccess(event string) {
	if b.latencyMeasurer.isMeasurementChannel(event) {
		return
	}
	b.m.subscribesTotal.WithLabelValues(b.backend, "true").Inc()
}

// RecordSubscribeFailure records a failed Subscribe/SubscribeWithErr.
func (b *BackendMetrics) RecordSubscribeFailure(event string) {
	if b.latencyMeasurer.isMeasurementChannel(event) {
		return
	}
	b.m.subscribesTotal.WithLabelValues(b.backend, "false").Inc()
}

// RecordReceived records metrics for a single received message. Size is
// classified using the shared thresholds so the messages_total "size" label
// is consistent across backends.
func (b *BackendMetrics) RecordReceived(event string, data []byte) {
	if b.latencyMeasurer.isMeasurementChannel(event) {
		return
	}
	sizeLabel := MessageSizeNormal
	if len(data) >= ColossalThreshold {
		sizeLabel = MessageSizeColossal
	}
	b.m.messagesTotal.WithLabelValues(b.backend, sizeLabel).Inc()
	b.m.receivedBytesTotal.WithLabelValues(b.backend).Add(float64(len(data)))
}

// RecordDisconnect increments the unexpected-disconnect counter.
func (b *BackendMetrics) RecordDisconnect() {
	b.m.disconnectionsTotal.WithLabelValues(b.backend).Inc()
}

// MarkConnected sets the connected gauge to 1.
func (b *BackendMetrics) MarkConnected() {
	b.m.connected.WithLabelValues(b.backend).Set(1)
}

// MarkDisconnected sets the connected gauge to 0.
func (b *BackendMetrics) MarkDisconnected() {
	b.m.connected.WithLabelValues(b.backend).Set(0)
}

// AddEvent, RemoveEvent, AddSubscriber, and RemoveSubscriber maintain the
// current_events and current_subscribers gauges.
func (b *BackendMetrics) AddEvent(event string) {
	if !b.latencyMeasurer.isMeasurementChannel(event) {
		b.m.currentEvents.WithLabelValues(b.backend).Inc()
	}
}

func (b *BackendMetrics) RemoveEvent(event string) {
	if !b.latencyMeasurer.isMeasurementChannel(event) {
		b.m.currentEvents.WithLabelValues(b.backend).Dec()
	}
}

func (b *BackendMetrics) AddSubscriber(event string) {
	if !b.latencyMeasurer.isMeasurementChannel(event) {
		b.m.currentSubscribers.WithLabelValues(b.backend).Inc()
	}
}

func (b *BackendMetrics) RemoveSubscriber(event string) {
	if !b.latencyMeasurer.isMeasurementChannel(event) {
		b.m.currentSubscribers.WithLabelValues(b.backend).Dec()
	}
}

// StartLatencyLoop spawns the background latency loop against p. The first
// measurement runs immediately so a fresh observation exists before the first
// interval elapses. It must be called at most once. Call StopLatencyLoop to
// cancel the loop and wait for it to exit.
func (b *BackendMetrics) StartLatencyLoop(p Pubsub) {
	close(b.latencyStarted)
	go func() {
		defer close(b.latencyDone)
		ticker := time.NewTicker(latencyMeasureInterval)
		defer ticker.Stop()
		for {
			b.recordLatency(b.latencyCtx, p)
			select {
			case <-b.latencyCtx.Done():
				return
			case <-ticker.C:
			}
		}
	}()
}

// StopLatencyLoop cancels the background latency loop and waits for it to exit.
// When StartLatencyLoop was never called (e.g. a pubsub built via
// newWithoutListener) there is no loop to wait for, so it returns immediately.
func (b *BackendMetrics) StopLatencyLoop() {
	b.latencyCancel()
	select {
	case <-b.latencyStarted:
		<-b.latencyDone
	default:
	}
}

// recordLatency runs one send/receive latency measurement and records it into
// the latency histograms and measure/error counters.
func (b *BackendMetrics) recordLatency(ctx context.Context, p Pubsub) {
	measureCtx, cancel := context.WithTimeout(ctx, latencyMeasureTimeout)
	defer cancel()
	send, recv, err := b.latencyMeasurer.Measure(measureCtx, p)

	b.m.latencyMeasures.WithLabelValues(b.backend).Inc()
	if err != nil {
		b.logger.Warn(ctx, "failed to measure latency", slog.Error(err))
		b.m.latencyErrs.WithLabelValues(b.backend).Inc()
		return
	}
	b.m.sendLatency.WithLabelValues(b.backend).Observe(send.Seconds())
	b.m.recvLatency.WithLabelValues(b.backend).Observe(recv.Seconds())
}
