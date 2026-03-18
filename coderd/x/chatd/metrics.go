package chatd

import (
	"errors"

	"github.com/prometheus/client_golang/prometheus"

	coderdpubsub "github.com/coder/coder/v2/coderd/pubsub"
)

const (
	chatStreamNotifyReasonMessagePersisted = "message_persisted"
	chatStreamNotifyReasonFullRefresh      = "full_refresh"
	chatStreamNotifyReasonStatusChange     = "status_change"
	chatStreamNotifyReasonQueueUpdate      = "queue_update"
	chatStreamNotifyReasonError            = "error"
	chatStreamNotifyReasonRetry            = "retry"
)

type chatStreamMetrics struct {
	notifyPublished     *prometheus.CounterVec
	dbCatchupQueries    *prometheus.CounterVec
	dbCatchupMessages   *prometheus.CounterVec
	queueRefreshQueries prometheus.Counter
}

func newChatStreamMetrics(reg prometheus.Registerer) *chatStreamMetrics {
	m := &chatStreamMetrics{
		notifyPublished: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: "coderd",
			Subsystem: "chat_stream",
			Name:      "notify_published_total",
			Help:      "Total number of chat stream notifications published, partitioned by reason.",
		}, []string{"reason"}),
		dbCatchupQueries: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: "coderd",
			Subsystem: "chat_stream",
			Name:      "db_catchup_queries_total",
			Help:      "Total number of chat stream message catch-up queries triggered by pubsub notifications, partitioned by reason.",
		}, []string{"reason"}),
		dbCatchupMessages: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: "coderd",
			Subsystem: "chat_stream",
			Name:      "db_catchup_messages_total",
			Help:      "Total number of messages returned by chat stream catch-up queries, partitioned by reason.",
		}, []string{"reason"}),
		queueRefreshQueries: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: "coderd",
			Subsystem: "chat_stream",
			Name:      "queue_refresh_queries_total",
			Help:      "Total number of queued-message refresh queries triggered by chat stream notifications.",
		}),
	}

	m.notifyPublished = registerChatStreamCounterVec(reg, m.notifyPublished)
	m.dbCatchupQueries = registerChatStreamCounterVec(reg, m.dbCatchupQueries)
	m.dbCatchupMessages = registerChatStreamCounterVec(reg, m.dbCatchupMessages)
	m.queueRefreshQueries = registerChatStreamCounter(reg, m.queueRefreshQueries)
	return m
}

func registerChatStreamCounterVec(reg prometheus.Registerer, collector *prometheus.CounterVec) *prometheus.CounterVec {
	if reg == nil {
		return collector
	}
	if err := reg.Register(collector); err != nil {
		var alreadyRegistered prometheus.AlreadyRegisteredError
		if errors.As(err, &alreadyRegistered) {
			existing, ok := alreadyRegistered.ExistingCollector.(*prometheus.CounterVec)
			if ok {
				return existing
			}
		}
	}
	return collector
}

func registerChatStreamCounter(reg prometheus.Registerer, collector prometheus.Counter) prometheus.Counter {
	if reg == nil {
		return collector
	}
	if err := reg.Register(collector); err != nil {
		var alreadyRegistered prometheus.AlreadyRegisteredError
		if errors.As(err, &alreadyRegistered) {
			existing, ok := alreadyRegistered.ExistingCollector.(prometheus.Counter)
			if ok {
				return existing
			}
		}
	}
	return collector
}

func (m *chatStreamMetrics) observeNotifyPublished(reason string) {
	if m == nil || reason == "" {
		return
	}
	m.notifyPublished.WithLabelValues(reason).Inc()
}

func (m *chatStreamMetrics) observeDBCatchupQuery(reason string) {
	if m == nil || reason == "" {
		return
	}
	m.dbCatchupQueries.WithLabelValues(reason).Inc()
}

func (m *chatStreamMetrics) observeDBCatchupMessages(reason string, count int) {
	if m == nil || reason == "" || count < 0 {
		return
	}
	m.dbCatchupMessages.WithLabelValues(reason).Add(float64(count))
}

func (m *chatStreamMetrics) observeQueueRefreshQuery() {
	if m == nil {
		return
	}
	m.queueRefreshQueries.Inc()
}

func chatStreamCatchupReason(notify coderdpubsub.ChatStreamNotifyMessage) string {
	switch {
	case notify.FullRefresh:
		return chatStreamNotifyReasonFullRefresh
	case notify.AfterMessageID > 0:
		return chatStreamNotifyReasonMessagePersisted
	default:
		return ""
	}
}
