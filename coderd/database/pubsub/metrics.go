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

// latencyBuckets covers sub-millisecond round-trips up to ~16s, which
// comfortably brackets a healthy pubsub as well as an overloaded one.
var latencyBuckets = prometheus.ExponentialBuckets(0.0005, 2, 16)

// Metrics owns the Prometheus instruments shared by all pubsub backends. A
// single instance is registered once; each backend obtains a BackendMetrics
// handle via ForBackend that records into these instruments with its own
// `backend` label value. The send/receive latency histograms are populated
// out-of-band by a background loop (see BackendMetrics.StartLatencyLoop)
// rather than during a scrape, so no backend implements prometheus.Collector.
type Metrics struct {
	publishesTotal      *prometheus.CounterVec // labels: backend, success
	subscribesTotal     *prometheus.CounterVec // labels: backend, success
	messagesTotal       *prometheus.CounterVec // labels: backend, size
	publishedBytesTotal *prometheus.CounterVec // labels: backend
	receivedBytesTotal  *prometheus.CounterVec // labels: backend
	disconnectionsTotal *prometheus.CounterVec // labels: backend
	connected           *prometheus.GaugeVec   // labels: backend
	currentSubscribers  *prometheus.GaugeVec   // labels: backend
	currentEvents       *prometheus.GaugeVec   // labels: backend
	sendLatency         *prometheus.HistogramVec
	recvLatency         *prometheus.HistogramVec
	latencyMeasures     *prometheus.CounterVec // labels: backend
	latencyErrs         *prometheus.CounterVec // labels: backend
}

// NewMetrics builds the shared pubsub instruments. If reg is non-nil every
// instrument is registered with it; otherwise they are created unregistered
// so callers and tests that do not scrape still get working (no-op)
// instruments. Call ForBackend to obtain a per-backend recording handle.
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
}

// ForBackend returns a recording handle bound to the given backend
// ("postgres" or "nats"). Each handle has its own LatencyMeasurer so probe
// channels never clash across backends.
func (m *Metrics) ForBackend(logger slog.Logger, backend string) *BackendMetrics {
	// Instantiate the per-backend series that have no secondary label so they
	// export a 0 value from the start, before any event increments them.
	// Series with a secondary label (success/size) are created on first use,
	// since their label values are not known up front.
	m.publishedBytesTotal.WithLabelValues(backend)
	m.receivedBytesTotal.WithLabelValues(backend)
	m.disconnectionsTotal.WithLabelValues(backend)
	m.connected.WithLabelValues(backend)
	m.currentSubscribers.WithLabelValues(backend)
	m.currentEvents.WithLabelValues(backend)
	m.latencyMeasures.WithLabelValues(backend)
	m.latencyErrs.WithLabelValues(backend)

	return &BackendMetrics{
		logger:          logger,
		backend:         backend,
		m:               m,
		latencyMeasurer: NewLatencyMeasurer(logger.Named("latency-measurer")),
	}
}

// RecordPublishSuccess records a successful Publish of n bytes.
func (b *BackendMetrics) RecordPublishSuccess(n int) {
	b.m.publishesTotal.WithLabelValues(b.backend, "true").Inc()
	b.m.publishedBytesTotal.WithLabelValues(b.backend).Add(float64(n))
}

// RecordPublishFailure records a failed Publish.
func (b *BackendMetrics) RecordPublishFailure() {
	b.m.publishesTotal.WithLabelValues(b.backend, "false").Inc()
}

// RecordSubscribeSuccess records a successful Subscribe/SubscribeWithErr.
func (b *BackendMetrics) RecordSubscribeSuccess() {
	b.m.subscribesTotal.WithLabelValues(b.backend, "true").Inc()
}

// RecordSubscribeFailure records a failed Subscribe/SubscribeWithErr.
func (b *BackendMetrics) RecordSubscribeFailure() {
	b.m.subscribesTotal.WithLabelValues(b.backend, "false").Inc()
}

// RecordReceived records metrics for a single received message. Size is
// classified using the shared thresholds so the messages_total "size" label
// is consistent across backends.
func (b *BackendMetrics) RecordReceived(data []byte) {
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
// current_events and current_subscribers gauges. The internal latency probe
// channel is excluded so its transient subscription does not skew the gauges.
func (b *BackendMetrics) AddEvent(event string) {
	if !b.isMeasurementChannel(event) {
		b.m.currentEvents.WithLabelValues(b.backend).Inc()
	}
}

func (b *BackendMetrics) RemoveEvent(event string) {
	if !b.isMeasurementChannel(event) {
		b.m.currentEvents.WithLabelValues(b.backend).Dec()
	}
}

func (b *BackendMetrics) AddSubscriber(event string) {
	if !b.isMeasurementChannel(event) {
		b.m.currentSubscribers.WithLabelValues(b.backend).Inc()
	}
}

func (b *BackendMetrics) RemoveSubscriber(event string) {
	if !b.isMeasurementChannel(event) {
		b.m.currentSubscribers.WithLabelValues(b.backend).Dec()
	}
}

func (b *BackendMetrics) isMeasurementChannel(event string) bool {
	return b.latencyMeasurer.isMeasurementChannel(event)
}

// StartLatencyLoop measures send/receive latency against p on a ticker until
// ctx is canceled, recording the durations into the latency histograms and
// bumping the measure/error counters. It runs the first measurement
// immediately so a fresh observation exists before the first interval
// elapses. Run it in its own goroutine.
func (b *BackendMetrics) StartLatencyLoop(ctx context.Context, interval time.Duration, p Pubsub) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		b.measureOnce(ctx, p)
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

// measureOnce performs a single latency measurement and records it.
func (b *BackendMetrics) measureOnce(ctx context.Context, p Pubsub) {
	measureCtx, cancel := context.WithTimeout(ctx, LatencyMeasureTimeout)
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
