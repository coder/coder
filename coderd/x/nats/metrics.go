package nats

import (
	"context"
	"time"

	"github.com/prometheus/client_golang/prometheus"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/coderd/database/pubsub"
)

// LatencyMeasureTimeout bounds the collect-time latency probe so a
// shutdown race cannot block a Prometheus scrape indefinitely.
const LatencyMeasureTimeout = time.Second * 10

// We track messages as size "normal" and "colossal", mirroring the
// PGPubsub metric labels for dashboard parity. The threshold matches
// PGPubsub (95% of the postgres notify limit). NATS MaxPayload is far
// larger, so for NATS "colossal" is informational, not a failure
// predictor.
const (
	colossalThreshold   = 7600
	messageSizeNormal   = "normal"
	messageSizeColossal = "colossal"
)

// these are the metrics we compute implicitly from our existing data structures
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
)

// additional metrics collected out-of-band
var (
	pubsubSendLatencyDesc = prometheus.NewDesc(
		"coder_nats_pubsub_send_latency_seconds",
		"The time taken to send a message into a pubsub event channel",
		nil, nil,
	)
	pubsubRecvLatencyDesc = prometheus.NewDesc(
		"coder_nats_pubsub_receive_latency_seconds",
		"The time taken to receive a message from a pubsub event channel",
		nil, nil,
	)
	pubsubLatencyMeasureCountDesc = prometheus.NewDesc(
		"coder_nats_pubsub_latency_measures_total",
		"The number of pubsub latency measurements",
		nil, nil,
	)
	pubsubLatencyMeasureErrDesc = prometheus.NewDesc(
		"coder_nats_pubsub_latency_measure_errs_total",
		"The number of pubsub latency measurement failures",
		nil, nil,
	)
)

// initMetrics constructs the explicit Prometheus metrics and latency
// measurer. It is called from newPubsub so that even internal tests
// building via newPubsub never observe nil metric fields.
func (p *Pubsub) initMetrics() {
	p.latencyMeasurer = pubsub.NewLatencyMeasurer(p.logger.Named("latency-measurer"))

	p.publishesTotal = prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace: "coder",
		Subsystem: "nats_pubsub",
		Name:      "publishes_total",
		Help:      "Total number of calls to Publish",
	}, []string{"success"})
	p.subscribesTotal = prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace: "coder",
		Subsystem: "nats_pubsub",
		Name:      "subscribes_total",
		Help:      "Total number of calls to Subscribe/SubscribeWithErr",
	}, []string{"success"})
	p.messagesTotal = prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace: "coder",
		Subsystem: "nats_pubsub",
		Name:      "messages_total",
		Help:      "Total number of messages received from nats",
	}, []string{"size"})
	p.publishedBytesTotal = prometheus.NewCounter(prometheus.CounterOpts{
		Namespace: "coder",
		Subsystem: "nats_pubsub",
		Name:      "published_bytes_total",
		Help:      "Total number of bytes successfully published across all publishes",
	})
	p.receivedBytesTotal = prometheus.NewCounter(prometheus.CounterOpts{
		Namespace: "coder",
		Subsystem: "nats_pubsub",
		Name:      "received_bytes_total",
		Help:      "Total number of bytes received across all messages",
	})
	p.disconnectionsTotal = prometheus.NewCounter(prometheus.CounterOpts{
		Namespace: "coder",
		Subsystem: "nats_pubsub",
		Name:      "disconnections_total",
		Help:      "Total number of times we disconnected unexpectedly from nats",
	})
	p.connected = prometheus.NewGauge(prometheus.GaugeOpts{
		Namespace: "coder",
		Subsystem: "nats_pubsub",
		Name:      "connected",
		Help:      "Whether we are connected (1) or not connected (0) to nats",
	})
}

// countReceived records metrics for a single received NATS message.
func (p *Pubsub) countReceived(data []byte) {
	sizeLabel := messageSizeNormal
	if len(data) >= colossalThreshold {
		sizeLabel = messageSizeColossal
	}
	p.messagesTotal.WithLabelValues(sizeLabel).Inc()
	p.receivedBytesTotal.Add(float64(len(data)))
}

// onDisconnect records an unexpected disconnect of one owned connection.
func (p *Pubsub) onDisconnect() {
	p.disconnectionsTotal.Inc()
	p.disconnectedConns.Add(1)
	p.connected.Set(0)
}

// onReconnect records that one owned connection reconnected. The
// connected gauge returns to 1 only once every owned connection is
// reconnected.
func (p *Pubsub) onReconnect() {
	for {
		cur := p.disconnectedConns.Load()
		if cur <= 0 {
			p.connected.Set(1)
			return
		}
		if p.disconnectedConns.CompareAndSwap(cur, cur-1) {
			if cur-1 <= 0 {
				p.connected.Set(1)
			}
			return
		}
	}
}

// Describe implements, along with Collect, the prometheus.Collector
// interface for metrics.
func (p *Pubsub) Describe(descs chan<- *prometheus.Desc) {
	// explicit metrics
	p.publishesTotal.Describe(descs)
	p.subscribesTotal.Describe(descs)
	p.messagesTotal.Describe(descs)
	p.publishedBytesTotal.Describe(descs)
	p.receivedBytesTotal.Describe(descs)
	p.disconnectionsTotal.Describe(descs)
	p.connected.Describe(descs)

	// implicit metrics
	descs <- currentSubscribersDesc
	descs <- currentEventsDesc

	// additional metrics
	descs <- pubsubSendLatencyDesc
	descs <- pubsubRecvLatencyDesc
	descs <- pubsubLatencyMeasureCountDesc
	descs <- pubsubLatencyMeasureErrDesc
}

// Collect implements, along with Describe, the prometheus.Collector
// interface for metrics.
func (p *Pubsub) Collect(metrics chan<- prometheus.Metric) {
	// explicit metrics
	p.publishesTotal.Collect(metrics)
	p.subscribesTotal.Collect(metrics)
	p.messagesTotal.Collect(metrics)
	p.publishedBytesTotal.Collect(metrics)
	p.receivedBytesTotal.Collect(metrics)
	p.disconnectionsTotal.Collect(metrics)
	p.connected.Collect(metrics)

	// implicit metrics
	p.mu.Lock()
	events := len(p.subscriptions)
	subs := 0
	for _, g := range p.subscriptions {
		g.mu.Lock()
		subs += len(g.localSubs)
		g.mu.Unlock()
	}
	p.mu.Unlock()
	metrics <- prometheus.MustNewConstMetric(currentSubscribersDesc, prometheus.GaugeValue, float64(subs))
	metrics <- prometheus.MustNewConstMetric(currentEventsDesc, prometheus.GaugeValue, float64(events))

	// additional metrics
	ctx, cancel := context.WithTimeout(context.Background(), LatencyMeasureTimeout)
	defer cancel()
	send, recv, err := p.latencyMeasurer.Measure(ctx, p)

	metrics <- prometheus.MustNewConstMetric(pubsubLatencyMeasureCountDesc, prometheus.CounterValue, float64(p.latencyMeasureCounter.Add(1)))
	if err != nil {
		p.logger.Warn(context.Background(), "failed to measure latency", slog.Error(err))
		metrics <- prometheus.MustNewConstMetric(pubsubLatencyMeasureErrDesc, prometheus.CounterValue, float64(p.latencyErrCounter.Add(1)))
		return
	}
	metrics <- prometheus.MustNewConstMetric(pubsubSendLatencyDesc, prometheus.GaugeValue, send.Seconds())
	metrics <- prometheus.MustNewConstMetric(pubsubRecvLatencyDesc, prometheus.GaugeValue, recv.Seconds())
}
