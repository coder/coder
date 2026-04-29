package nats

import (
	"github.com/prometheus/client_golang/prometheus"
)

// Parity constants with coderd/database/pubsub.
const (
	colossalThreshold   = 7600
	messageSizeNormal   = "normal"
	messageSizeColossal = "colossal"
)

// Implicit / sampled metric descriptors.
var (
	currentSubscribersDesc = prometheus.NewDesc(
		"coder_pubsub_current_subscribers",
		"The current number of active pubsub subscribers",
		nil, nil,
	)
	currentEventsDesc = prometheus.NewDesc(
		"coder_pubsub_current_events",
		"The current number of pubsub event channels listened for",
		nil, nil,
	)
	natsPendingMsgsDesc = prometheus.NewDesc(
		"coder_pubsub_nats_pending_msgs",
		"Sum of NATS per-subscription pending messages across active subscriptions",
		nil, nil,
	)
	natsPendingBytesDesc = prometheus.NewDesc(
		"coder_pubsub_nats_pending_bytes",
		"Sum of NATS per-subscription pending bytes across active subscriptions",
		nil, nil,
	)
)

// pubsubMetrics holds the explicit Prometheus counter set for *Pubsub.
type pubsubMetrics struct {
	publishesTotal      *prometheus.CounterVec
	subscribesTotal     *prometheus.CounterVec
	messagesTotal       *prometheus.CounterVec
	publishedBytesTotal prometheus.Counter
	receivedBytesTotal  prometheus.Counter
	slowConsumersTotal  prometheus.Counter
	reconnectsTotal     prometheus.Counter
	disconnectsTotal    prometheus.Counter
	droppedMsgsTotal    prometheus.Counter
}

func newPubsubMetrics() pubsubMetrics {
	return pubsubMetrics{
		publishesTotal: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: "coder",
			Subsystem: "pubsub",
			Name:      "publishes_total",
			Help:      "Total number of calls to Publish",
		}, []string{"success"}),
		subscribesTotal: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: "coder",
			Subsystem: "pubsub",
			Name:      "subscribes_total",
			Help:      "Total number of calls to Subscribe/SubscribeWithErr",
		}, []string{"success"}),
		messagesTotal: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: "coder",
			Subsystem: "pubsub",
			Name:      "messages_total",
			Help:      "Total number of messages published, labeled by size class",
		}, []string{"size"}),
		publishedBytesTotal: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: "coder",
			Subsystem: "pubsub",
			Name:      "published_bytes_total",
			Help:      "Total number of bytes successfully published across all publishes",
		}),
		receivedBytesTotal: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: "coder",
			Subsystem: "pubsub",
			Name:      "received_bytes_total",
			Help:      "Total number of bytes received across all messages",
		}),
		slowConsumersTotal: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: "coder",
			Subsystem: "pubsub",
			Name:      "nats_slow_consumers_total",
			Help:      "Total number of NATS slow-consumer signals observed",
		}),
		reconnectsTotal: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: "coder",
			Subsystem: "pubsub",
			Name:      "nats_reconnects_total",
			Help:      "Total number of NATS client reconnect events",
		}),
		disconnectsTotal: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: "coder",
			Subsystem: "pubsub",
			Name:      "nats_disconnects_total",
			Help:      "Total number of NATS client disconnect events",
		}),
		droppedMsgsTotal: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: "coder",
			Subsystem: "pubsub",
			Name:      "nats_dropped_msgs_total",
			Help:      "Total number of messages dropped by NATS slow-consumer protection",
		}),
	}
}

// Compile-time assertion that *Pubsub satisfies prometheus.Collector.
var _ prometheus.Collector = (*Pubsub)(nil)

// Describe implements prometheus.Collector.
func (p *Pubsub) Describe(descs chan<- *prometheus.Desc) {
	p.metrics.publishesTotal.Describe(descs)
	p.metrics.subscribesTotal.Describe(descs)
	p.metrics.messagesTotal.Describe(descs)
	p.metrics.publishedBytesTotal.Describe(descs)
	p.metrics.receivedBytesTotal.Describe(descs)
	p.metrics.slowConsumersTotal.Describe(descs)
	p.metrics.reconnectsTotal.Describe(descs)
	p.metrics.disconnectsTotal.Describe(descs)
	p.metrics.droppedMsgsTotal.Describe(descs)

	descs <- currentSubscribersDesc
	descs <- currentEventsDesc
	descs <- natsPendingMsgsDesc
	descs <- natsPendingBytesDesc
}

// Collect implements prometheus.Collector.
func (p *Pubsub) Collect(metrics chan<- prometheus.Metric) {
	p.metrics.publishesTotal.Collect(metrics)
	p.metrics.subscribesTotal.Collect(metrics)
	p.metrics.messagesTotal.Collect(metrics)
	p.metrics.publishedBytesTotal.Collect(metrics)
	p.metrics.receivedBytesTotal.Collect(metrics)
	p.metrics.slowConsumersTotal.Collect(metrics)
	p.metrics.reconnectsTotal.Collect(metrics)
	p.metrics.disconnectsTotal.Collect(metrics)
	p.metrics.droppedMsgsTotal.Collect(metrics)

	// Snapshot subscriptions under lock, but call NATS APIs without holding
	// p.mu since Subscription.Pending() takes NATS-internal locks.
	p.mu.Lock()
	subCount := len(p.subs)
	eventCount := len(p.eventCounts)
	subs := make([]*subscription, 0, len(p.subs))
	for s := range p.subs {
		subs = append(subs, s)
	}
	p.mu.Unlock()

	metrics <- prometheus.MustNewConstMetric(currentSubscribersDesc, prometheus.GaugeValue, float64(subCount))
	metrics <- prometheus.MustNewConstMetric(currentEventsDesc, prometheus.GaugeValue, float64(eventCount))

	var pendingMsgs, pendingBytes int
	for _, s := range subs {
		if s.sub == nil {
			continue
		}
		m, b, err := s.sub.Pending()
		if err != nil {
			continue
		}
		pendingMsgs += m
		pendingBytes += b
	}
	metrics <- prometheus.MustNewConstMetric(natsPendingMsgsDesc, prometheus.GaugeValue, float64(pendingMsgs))
	metrics <- prometheus.MustNewConstMetric(natsPendingBytesDesc, prometheus.GaugeValue, float64(pendingBytes))
}
