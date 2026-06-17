package nats

import (
	"context"
	"sync"
	"sync/atomic"

	"github.com/prometheus/client_golang/prometheus"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/coderd/database/pubsub"
)

// Descriptors for metrics that are not backed by a stored collector:
// current_subscribers and current_events are read from atomic counters,
// and the latency metrics are measured during each scrape.
var (
	currentSubscribersDesc = prometheus.NewDesc(
		"coder_nats_pubsub_current_subscribers",
		"The current number of active pubsub subscribers",
		nil, nil,
	)
	currentEventsDesc = prometheus.NewDesc(
		"coder_nats_pubsub_current_events",
		"The current number of pubsub event channels listened for",
		nil, nil,
	)
	sendLatencyDesc = prometheus.NewDesc(
		"coder_nats_pubsub_send_latency_seconds",
		"The time taken to send a message into a pubsub event channel",
		nil, nil,
	)
	recvLatencyDesc = prometheus.NewDesc(
		"coder_nats_pubsub_receive_latency_seconds",
		"The time taken to receive a message from a pubsub event channel",
		nil, nil,
	)
	latencyMeasureCountDesc = prometheus.NewDesc(
		"coder_nats_pubsub_latency_measures_total",
		"The number of pubsub latency measurements",
		nil, nil,
	)
	latencyMeasureErrDesc = prometheus.NewDesc(
		"coder_nats_pubsub_latency_measure_errs_total",
		"The number of pubsub latency measurement failures",
		nil, nil,
	)
)

// metrics owns all Prometheus state for the NATS Pubsub. Collaborators
// such as groupSub depend on this narrow type rather than the whole
// Pubsub, so the only thing they can do is record metrics.
type metrics struct {
	logger slog.Logger

	publishesTotal      *prometheus.CounterVec
	subscribesTotal     *prometheus.CounterVec
	messagesTotal       *prometheus.CounterVec
	publishedBytesTotal prometheus.Counter
	receivedBytesTotal  prometheus.Counter
	disconnectionsTotal prometheus.Counter
	connected           prometheus.Gauge

	latencyMeasurer       *pubsub.LatencyMeasurer
	latencyMeasureCounter atomic.Int64
	latencyErrCounter     atomic.Int64

	// connMu guards the connection-state accounting below. Connect and
	// disconnect callbacks are rare, so a mutex keeps the gauge update
	// atomic with the count without meaningful contention.
	connMu         sync.Mutex
	totalConns     int
	connectedConns int

	// currentEvents and currentSubscribers shadow the sizes of the
	// Pubsub's subscriptions map and per-event localSubs maps. They are
	// maintained at the subscribe/unsubscribe sites so Collect can read
	// the gauges without locking the Pubsub.
	currentEvents      atomic.Int64
	currentSubscribers atomic.Int64
}

// newMetrics builds all metric instruments up front so collaborators
// never observe nil metric fields.
func newMetrics(logger slog.Logger) *metrics {
	return &metrics{
		logger:          logger,
		latencyMeasurer: pubsub.NewLatencyMeasurer(logger.Named("latency-measurer")),

		publishesTotal: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: "coder",
			Subsystem: "nats_pubsub",
			Name:      "publishes_total",
			Help:      "Total number of calls to Publish",
		}, []string{"success"}),
		subscribesTotal: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: "coder",
			Subsystem: "nats_pubsub",
			Name:      "subscribes_total",
			Help:      "Total number of calls to Subscribe/SubscribeWithErr",
		}, []string{"success"}),
		messagesTotal: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: "coder",
			Subsystem: "nats_pubsub",
			Name:      "messages_total",
			Help:      "Total number of messages received from nats",
		}, []string{"size"}),
		publishedBytesTotal: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: "coder",
			Subsystem: "nats_pubsub",
			Name:      "published_bytes_total",
			Help:      "Total number of bytes successfully published across all publishes",
		}),
		receivedBytesTotal: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: "coder",
			Subsystem: "nats_pubsub",
			Name:      "received_bytes_total",
			Help:      "Total number of bytes received across all messages",
		}),
		disconnectionsTotal: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: "coder",
			Subsystem: "nats_pubsub",
			Name:      "disconnections_total",
			Help:      "Total number of times we disconnected unexpectedly from nats",
		}),
		connected: prometheus.NewGauge(prometheus.GaugeOpts{
			Namespace: "coder",
			Subsystem: "nats_pubsub",
			Name:      "connected",
			Help:      "Whether we are connected (1) or not connected (0) to nats",
		}),
	}
}

// recordPublishSuccess records a successful Publish of n bytes.
func (m *metrics) recordPublishSuccess(n int) {
	m.publishesTotal.WithLabelValues("true").Inc()
	m.publishedBytesTotal.Add(float64(n))
}

// recordPublishFailure records a failed Publish.
func (m *metrics) recordPublishFailure() {
	m.publishesTotal.WithLabelValues("false").Inc()
}

// recordSubscribeSuccess records a successful Subscribe/SubscribeWithErr.
func (m *metrics) recordSubscribeSuccess() {
	m.subscribesTotal.WithLabelValues("true").Inc()
}

// recordSubscribeFailure records a failed Subscribe/SubscribeWithErr.
func (m *metrics) recordSubscribeFailure() {
	m.subscribesTotal.WithLabelValues("false").Inc()
}

// recordReceived records metrics for a single received NATS message.
// Size is classified using the shared pubsub thresholds so the
// messages_total "size" label matches PGPubsub.
func (m *metrics) recordReceived(data []byte) {
	sizeLabel := pubsub.MessageSizeNormal
	if len(data) >= pubsub.ColossalThreshold {
		sizeLabel = pubsub.MessageSizeColossal
	}
	m.messagesTotal.WithLabelValues(sizeLabel).Inc()
	m.receivedBytesTotal.Add(float64(len(data)))
}

// markConnected records that all total owned connections have dialed
// successfully. The connected gauge is 1 only while every owned
// connection is up.
func (m *metrics) markConnected(total int) {
	m.connMu.Lock()
	defer m.connMu.Unlock()
	m.totalConns = total
	m.connectedConns = total
	m.setConnectedLocked()
}

// markClosed records that the Pubsub is shutting down and forces the
// connected gauge to 0. Closing our own connections does not fire the
// disconnect handler (see NoCallbacksAfterClientClose), so without this
// the gauge would still read 1 after Close. connectedConns is zeroed so
// a late reconnect callback cannot flip the gauge back to 1.
func (m *metrics) markClosed() {
	m.connMu.Lock()
	defer m.connMu.Unlock()
	// Zero both counters so setConnectedLocked's totalConns > 0 guard is
	// permanently false: a late reconnect callback during the shutdown
	// window cannot increment connectedConns back up and flip the gauge
	// to 1.
	m.totalConns = 0
	m.connectedConns = 0
	m.connected.Set(0)
}

// onDisconnect records an unexpected disconnect of one owned connection.
func (m *metrics) onDisconnect() {
	m.disconnectionsTotal.Inc()
	m.connMu.Lock()
	defer m.connMu.Unlock()
	if m.connectedConns > 0 {
		m.connectedConns--
	}
	m.setConnectedLocked()
}

// onReconnect records that one owned connection reconnected.
func (m *metrics) onReconnect() {
	m.connMu.Lock()
	defer m.connMu.Unlock()
	if m.connectedConns < m.totalConns {
		m.connectedConns++
	}
	m.setConnectedLocked()
}

// setConnectedLocked sets the connected gauge to 1 only when every owned
// connection is up. Callers must hold connMu.
func (m *metrics) setConnectedLocked() {
	if m.totalConns > 0 && m.connectedConns == m.totalConns {
		m.connected.Set(1)
		return
	}
	m.connected.Set(0)
}

// addEvent and removeEvent track the number of subscribed event
// channels. addSubscriber and removeSubscriber track the number of
// local subscribers across all events.
func (m *metrics) addEvent()         { m.currentEvents.Add(1) }
func (m *metrics) removeEvent()      { m.currentEvents.Add(-1) }
func (m *metrics) addSubscriber()    { m.currentSubscribers.Add(1) }
func (m *metrics) removeSubscriber() { m.currentSubscribers.Add(-1) }

// describe sends every metric descriptor on behalf of the owning
// Pubsub's prometheus.Collector implementation.
func (m *metrics) describe(descs chan<- *prometheus.Desc) {
	// explicit metrics
	m.publishesTotal.Describe(descs)
	m.subscribesTotal.Describe(descs)
	m.messagesTotal.Describe(descs)
	m.publishedBytesTotal.Describe(descs)
	m.receivedBytesTotal.Describe(descs)
	m.disconnectionsTotal.Describe(descs)
	m.connected.Describe(descs)

	// implicit metrics
	descs <- currentSubscribersDesc
	descs <- currentEventsDesc

	// additional metrics
	descs <- sendLatencyDesc
	descs <- recvLatencyDesc
	descs <- latencyMeasureCountDesc
	descs <- latencyMeasureErrDesc
}

// collect emits all metrics. p is the pubsub used for the out-of-band
// latency measurement. The current subscriber and event gauges are read
// from atomic counters maintained at the subscribe/unsubscribe sites, so
// Collect does not lock the Pubsub.
func (m *metrics) collect(ch chan<- prometheus.Metric, p pubsub.Pubsub) {
	// explicit metrics
	m.publishesTotal.Collect(ch)
	m.subscribesTotal.Collect(ch)
	m.messagesTotal.Collect(ch)
	m.publishedBytesTotal.Collect(ch)
	m.receivedBytesTotal.Collect(ch)
	m.disconnectionsTotal.Collect(ch)
	m.connected.Collect(ch)

	// implicit metrics
	ch <- prometheus.MustNewConstMetric(currentSubscribersDesc, prometheus.GaugeValue, float64(m.currentSubscribers.Load()))
	ch <- prometheus.MustNewConstMetric(currentEventsDesc, prometheus.GaugeValue, float64(m.currentEvents.Load()))

	// additional metrics
	ctx, cancel := context.WithTimeout(context.Background(), pubsub.LatencyMeasureTimeout)
	defer cancel()
	send, recv, err := m.latencyMeasurer.Measure(ctx, p)

	ch <- prometheus.MustNewConstMetric(latencyMeasureCountDesc, prometheus.CounterValue, float64(m.latencyMeasureCounter.Add(1)))
	if err != nil {
		m.logger.Warn(context.Background(), "failed to measure latency", slog.Error(err))
		ch <- prometheus.MustNewConstMetric(latencyMeasureErrDesc, prometheus.CounterValue, float64(m.latencyErrCounter.Add(1)))
		return
	}
	ch <- prometheus.MustNewConstMetric(sendLatencyDesc, prometheus.GaugeValue, send.Seconds())
	ch <- prometheus.MustNewConstMetric(recvLatencyDesc, prometheus.GaugeValue, recv.Seconds())
}
