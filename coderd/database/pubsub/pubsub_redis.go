package pubsub

import (
	"context"
	"sync"
	"sync/atomic"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	redis "github.com/redis/go-redis/v9"
	"golang.org/x/xerrors"

	"cdr.dev/slog/v3"
)

// RedisPubsub is a pubsub implementation using Redis Pub/Sub.
type RedisPubsub struct {
	logger     slog.Logger
	listenDone chan struct{}
	client     *redis.Client
	pubSub     *redis.PubSub
	messages   <-chan interface{}

	qMu                  sync.Mutex
	queues               map[string]*queueSet
	pendingSubscribeAcks map[string]int
	pendingUnsubscribe   map[string]int
	reconnectPendingAcks int

	closeMu      sync.Mutex
	closed       bool
	closeErr     error
	clientClosed bool
	clientErr    error

	publishesTotal      *prometheus.CounterVec
	subscribesTotal     *prometheus.CounterVec
	messagesTotal       *prometheus.CounterVec
	publishedBytesTotal prometheus.Counter
	receivedBytesTotal  prometheus.Counter
	disconnectionsTotal prometheus.Counter
	droppedSignalsTotal prometheus.Counter
	connected           prometheus.Gauge

	latencyMeasurer       *LatencyMeasurer
	latencyMeasureCounter atomic.Int64
	latencyErrCounter     atomic.Int64
}

// NewRedis creates a new Pubsub implementation using a Redis connection.
func NewRedis(startCtx context.Context, logger slog.Logger, redisURL string) (*RedisPubsub, error) {
	opts, err := redis.ParseURL(redisURL)
	if err != nil {
		return nil, xerrors.Errorf("parse redis URL: %w", err)
	}
	client := redis.NewClient(opts)
	if err := client.Ping(startCtx).Err(); err != nil {
		_ = client.Close()
		return nil, xerrors.Errorf("ping redis: %w", err)
	}

	p := newRedisWithoutListener(logger, client)
	go p.listen()
	logger.Debug(startCtx, "redis pubsub has started")
	return p, nil
}

func newRedisWithoutListener(logger slog.Logger, client *redis.Client) *RedisPubsub {
	pubSub := client.Subscribe(context.Background())
	return &RedisPubsub{
		logger:               logger,
		listenDone:           make(chan struct{}),
		client:               client,
		pubSub:               pubSub,
		messages:             pubSub.ChannelWithSubscriptions(redis.WithChannelSize(BufferSize)),
		queues:               make(map[string]*queueSet),
		pendingSubscribeAcks: make(map[string]int),
		pendingUnsubscribe:   make(map[string]int),
		latencyMeasurer:      NewLatencyMeasurer(logger.Named("latency-measurer")),
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
			Help:      "Total number of messages received from the pubsub backend",
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
		disconnectionsTotal: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: "coder",
			Subsystem: "pubsub",
			Name:      "disconnections_total",
			Help:      "Total number of times the pubsub backend disconnected unexpectedly",
		}),
		droppedSignalsTotal: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: "coder",
			Subsystem: "pubsub",
			Name:      "dropped_signals_total",
			Help:      "Total number of ErrDroppedMessages signals emitted to subscribers",
		}),
		connected: prometheus.NewGauge(prometheus.GaugeOpts{
			Namespace: "coder",
			Subsystem: "pubsub",
			Name:      "connected",
			Help:      "Whether we are connected (1) or not connected (0) to the pubsub backend",
		}),
	}
}

// Subscribe calls the listener when an event matching the name is received.
func (p *RedisPubsub) Subscribe(event string, listener Listener) (cancel func(), err error) {
	return p.subscribeQueue(event, newMsgQueueWithDroppedCallback(context.Background(), listener, nil, p.recordDroppedSignal))
}

// SubscribeWithErr calls the listener when an event matching the name is
// received and forwards ErrDroppedMessages signals to the listener.
func (p *RedisPubsub) SubscribeWithErr(event string, listener ListenerWithErr) (cancel func(), err error) {
	return p.subscribeQueue(event, newMsgQueueWithDroppedCallback(context.Background(), nil, listener, p.recordDroppedSignal))
}

func (p *RedisPubsub) subscribeQueue(event string, newQ *msgQueue) (cancel func(), err error) {
	defer func() {
		if err != nil {
			newQ.close()
			p.subscribesTotal.WithLabelValues("false").Inc()
		} else {
			p.subscribesTotal.WithLabelValues("true").Inc()
		}
	}()

	var (
		unlistenInProgress <-chan struct{}
		qs                 *queueSet
		needsSubscribe     bool
	)
	func() {
		p.qMu.Lock()
		defer p.qMu.Unlock()

		var ok bool
		if qs, ok = p.queues[event]; !ok {
			qs = newQueueSet()
			p.queues[event] = qs
			needsSubscribe = true
		}
		qs.m[newQ] = struct{}{}
		unlistenInProgress = qs.unlistenInProgress
		if unlistenInProgress != nil {
			needsSubscribe = true
		}
	}()
	if unlistenInProgress != nil {
		p.logger.Debug(context.Background(), "waiting for redis unsubscribe in progress", slog.F("event", event))
		<-unlistenInProgress
		p.logger.Debug(context.Background(), "redis unsubscribe complete", slog.F("event", event))
	}
	defer func() {
		if err != nil {
			p.qMu.Lock()
			defer p.qMu.Unlock()
			delete(qs.m, newQ)
			if len(qs.m) == 0 {
				delete(p.queues, event)
			}
		}
	}()

	if needsSubscribe {
		p.qMu.Lock()
		p.pendingSubscribeAcks[event]++
		err = p.pubSub.Subscribe(context.Background(), event)
		if err != nil {
			consumePendingAckLocked(p.pendingSubscribeAcks, event)
			p.qMu.Unlock()
			return nil, xerrors.Errorf("subscribe: %w", err)
		}
		p.qMu.Unlock()
		p.logger.Debug(context.Background(), "started listening to redis event channel", slog.F("event", event))
	}

	return func() {
		var (
			unsubscribing chan struct{}
			shouldUnsub   bool
		)
		func() {
			p.qMu.Lock()
			defer p.qMu.Unlock()
			newQ.close()
			qSet, ok := p.queues[event]
			if !ok {
				p.logger.Critical(context.Background(), "redis event was removed before cancel", slog.F("event", event))
				return
			}
			delete(qSet.m, newQ)
			if len(qSet.m) == 0 {
				unsubscribing = make(chan struct{})
				qSet.unlistenInProgress = unsubscribing
				shouldUnsub = true
			}
		}()

		if shouldUnsub {
			p.qMu.Lock()
			p.pendingUnsubscribe[event]++
			p.qMu.Unlock()
			uErr := p.pubSub.Unsubscribe(context.Background(), event)
			if uErr != nil {
				p.qMu.Lock()
				consumePendingAckLocked(p.pendingUnsubscribe, event)
				p.qMu.Unlock()
			}
			close(unsubscribing)
			func() {
				p.qMu.Lock()
				defer p.qMu.Unlock()
				qSet, ok := p.queues[event]
				if ok && len(qSet.m) == 0 {
					delete(p.queues, event)
				}
			}()

			p.closeMu.Lock()
			closed := p.closed
			p.closeMu.Unlock()
			if uErr != nil && !closed {
				p.logger.Warn(context.Background(), "failed to unsubscribe from redis event channel", slog.Error(uErr), slog.F("event", event))
			} else {
				p.logger.Debug(context.Background(), "stopped listening to redis event channel", slog.F("event", event))
			}
		}
	}, nil
}

// Publish sends a message to the Redis channel.
func (p *RedisPubsub) Publish(event string, message []byte) error {
	p.logger.Debug(context.Background(), "publish", slog.F("event", event), slog.F("message_len", len(message)))
	err := p.client.Publish(context.Background(), event, message).Err()
	if err != nil {
		p.publishesTotal.WithLabelValues("false").Inc()
		return xerrors.Errorf("publish to redis: %w", err)
	}
	p.publishesTotal.WithLabelValues("true").Inc()
	p.publishedBytesTotal.Add(float64(len(message)))
	return nil
}

// Close closes the pubsub instance.
func (p *RedisPubsub) Close() error {
	p.logger.Info(context.Background(), "redis pubsub is closing")
	err := p.closePubSub()
	<-p.listenDone
	if clientErr := p.closeClient(); clientErr != nil && err == nil {
		err = clientErr
	}
	p.logger.Debug(context.Background(), "redis pubsub closed")
	return err
}

func (p *RedisPubsub) closePubSub() error {
	p.closeMu.Lock()
	defer p.closeMu.Unlock()
	if p.closed {
		return p.closeErr
	}
	p.closed = true
	p.closeErr = p.pubSub.Close()
	return p.closeErr
}

func (p *RedisPubsub) closeClient() error {
	p.closeMu.Lock()
	defer p.closeMu.Unlock()
	if p.clientClosed {
		return p.clientErr
	}
	p.clientClosed = true
	p.clientErr = p.client.Close()
	return p.clientErr
}

func (p *RedisPubsub) listen() {
	defer func() {
		p.logger.Info(context.Background(), "redis pubsub listen stopped receiving messages")
		close(p.listenDone)
	}()

	for msg := range p.messages {
		switch msg := msg.(type) {
		case *redis.Message:
			p.listenReceive(msg)
		case *redis.Subscription:
			p.listenSubscription(msg)
		}
	}
}

func (p *RedisPubsub) listenReceive(msg *redis.Message) {
	sizeLabel := messageSizeNormal
	if len(msg.Payload) >= colossalThreshold {
		sizeLabel = messageSizeColossal
	}
	p.messagesTotal.WithLabelValues(sizeLabel).Inc()
	p.receivedBytesTotal.Add(float64(len(msg.Payload)))

	p.qMu.Lock()
	defer p.qMu.Unlock()
	p.reconnectPendingAcks = 0
	qSet, ok := p.queues[msg.Channel]
	if !ok {
		return
	}
	payload := []byte(msg.Payload)
	for q := range qSet.m {
		q.enqueue(payload)
	}
}

func (p *RedisPubsub) listenSubscription(msg *redis.Subscription) {
	if msg.Kind != "subscribe" && msg.Kind != "unsubscribe" {
		return
	}

	p.qMu.Lock()
	defer p.qMu.Unlock()

	var pending map[string]int
	if msg.Kind == "subscribe" {
		pending = p.pendingSubscribeAcks
	} else {
		pending = p.pendingUnsubscribe
	}
	if consumePendingAckLocked(pending, msg.Channel) {
		return
	}
	if msg.Kind != "subscribe" {
		return
	}
	if _, ok := p.queues[msg.Channel]; !ok {
		return
	}

	if p.reconnectPendingAcks == 0 {
		p.logger.Info(context.Background(), "redis pubsub reconnected", slog.F("channel", msg.Channel), slog.F("count", msg.Count))
		p.disconnectionsTotal.Inc()
		p.recordReconnectLocked()
		p.reconnectPendingAcks = len(p.queues) - 1
		if p.reconnectPendingAcks < 0 {
			p.reconnectPendingAcks = 0
		}
		return
	}
	p.reconnectPendingAcks--
}

func (p *RedisPubsub) recordReconnectLocked() {
	for _, qSet := range p.queues {
		for q := range qSet.m {
			q.dropped()
		}
	}
}

func consumePendingAckLocked(pending map[string]int, event string) bool {
	count := pending[event]
	if count == 0 {
		return false
	}
	if count == 1 {
		delete(pending, event)
		return true
	}
	pending[event] = count - 1
	return true
}

func (p *RedisPubsub) recordDroppedSignal() {
	p.droppedSignalsTotal.Inc()
}

// Describe implements, along with Collect, the prometheus.Collector interface
// for metrics.
func (p *RedisPubsub) Describe(descs chan<- *prometheus.Desc) {
	p.publishesTotal.Describe(descs)
	p.subscribesTotal.Describe(descs)
	p.messagesTotal.Describe(descs)
	p.publishedBytesTotal.Describe(descs)
	p.receivedBytesTotal.Describe(descs)
	p.disconnectionsTotal.Describe(descs)
	p.droppedSignalsTotal.Describe(descs)
	p.connected.Describe(descs)
	descs <- currentSubscribersDesc
	descs <- currentEventsDesc
	descs <- pubsubSendLatencyDesc
	descs <- pubsubRecvLatencyDesc
	descs <- pubsubLatencyMeasureCountDesc
	descs <- pubsubLatencyMeasureErrDesc
}

// Collect implements, along with Describe, the prometheus.Collector interface
// for metrics.
func (p *RedisPubsub) Collect(metrics chan<- prometheus.Metric) {
	p.refreshConnected()
	p.publishesTotal.Collect(metrics)
	p.subscribesTotal.Collect(metrics)
	p.messagesTotal.Collect(metrics)
	p.publishedBytesTotal.Collect(metrics)
	p.receivedBytesTotal.Collect(metrics)
	p.disconnectionsTotal.Collect(metrics)
	p.droppedSignalsTotal.Collect(metrics)
	p.connected.Collect(metrics)

	p.qMu.Lock()
	events := len(p.queues)
	subs := 0
	for _, qSet := range p.queues {
		subs += len(qSet.m)
	}
	p.qMu.Unlock()
	metrics <- prometheus.MustNewConstMetric(currentSubscribersDesc, prometheus.GaugeValue, float64(subs))
	metrics <- prometheus.MustNewConstMetric(currentEventsDesc, prometheus.GaugeValue, float64(events))

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

func (p *RedisPubsub) refreshConnected() {
	p.closeMu.Lock()
	closed := p.closed
	p.closeMu.Unlock()
	if closed {
		p.connected.Set(0)
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := p.client.Ping(ctx).Err(); err != nil {
		p.connected.Set(0)
		return
	}
	p.connected.Set(1)
}
