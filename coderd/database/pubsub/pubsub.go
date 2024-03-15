package pubsub

import (
	"context"
	"database/sql"
	"errors"
	"io"
	"net"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/lib/pq"
	"github.com/prometheus/client_golang/prometheus"
	"golang.org/x/xerrors"

	"cdr.dev/slog"
)

// Listener represents a pubsub handler.
type Listener func(ctx context.Context, message []byte)

// ListenerWithErr represents a pubsub handler that can also receive error
// indications
type ListenerWithErr func(ctx context.Context, message []byte, err error)

// ErrDroppedMessages is sent to ListenerWithErr if messages are dropped or
// might have been dropped.
var ErrDroppedMessages = xerrors.New("dropped messages")

// Pubsub is a generic interface for broadcasting and receiving messages.
// Implementors should assume high-availability with the backing implementation.
type Pubsub interface {
	Subscribe(event string, listener Listener) (cancel func(), err error)
	SubscribeWithErr(event string, listener ListenerWithErr) (cancel func(), err error)
	Publish(event string, message []byte) error
	Close() error
}

// msgOrErr either contains a message or an error
type msgOrErr struct {
	msg []byte
	err error
}

// msgQueue implements a fixed length queue with the ability to replace elements
// after they are queued (but before they are dequeued).
//
// The purpose of this data structure is to build something that works a bit
// like a golang channel, but if the queue is full, then we can replace the
// last element with an error so that the subscriber can get notified that some
// messages were dropped, all without blocking.
type msgQueue struct {
	ctx    context.Context
	cond   *sync.Cond
	q      [BufferSize]msgOrErr
	front  int
	size   int
	closed bool
	l      Listener
	le     ListenerWithErr
}

func newMsgQueue(ctx context.Context, l Listener, le ListenerWithErr) *msgQueue {
	if l == nil && le == nil {
		panic("l or le must be non-nil")
	}
	q := &msgQueue{
		ctx:  ctx,
		cond: sync.NewCond(&sync.Mutex{}),
		l:    l,
		le:   le,
	}
	go q.run()
	return q
}

func (q *msgQueue) run() {
	for {
		// wait until there is something on the queue or we are closed
		q.cond.L.Lock()
		for q.size == 0 && !q.closed {
			q.cond.Wait()
		}
		if q.closed {
			q.cond.L.Unlock()
			return
		}
		item := q.q[q.front]
		q.front = (q.front + 1) % BufferSize
		q.size--
		q.cond.L.Unlock()

		// process item without holding lock
		if item.err == nil {
			// real message
			if q.l != nil {
				q.l(q.ctx, item.msg)
				continue
			}
			if q.le != nil {
				q.le(q.ctx, item.msg, nil)
				continue
			}
			// unhittable
			continue
		}
		// if the listener wants errors, send it.
		if q.le != nil {
			q.le(q.ctx, nil, item.err)
		}
	}
}

func (q *msgQueue) enqueue(msg []byte) {
	q.cond.L.Lock()
	defer q.cond.L.Unlock()

	if q.size == BufferSize {
		// queue is full, so we're going to drop the msg we got called with.
		// We also need to record that messages are being dropped, which we
		// do at the last message in the queue.  This potentially makes us
		// lose 2 messages instead of one, but it's more important at this
		// point to warn the subscriber that they're losing messages so they
		// can do something about it.
		back := (q.front + BufferSize - 1) % BufferSize
		q.q[back].msg = nil
		q.q[back].err = ErrDroppedMessages
		return
	}
	// queue is not full, insert the message
	next := (q.front + q.size) % BufferSize
	q.q[next].msg = msg
	q.q[next].err = nil
	q.size++
	q.cond.Broadcast()
}

func (q *msgQueue) close() {
	q.cond.L.Lock()
	defer q.cond.L.Unlock()
	defer q.cond.Broadcast()
	q.closed = true
}

// dropped records an error in the queue that messages might have been dropped
func (q *msgQueue) dropped() {
	q.cond.L.Lock()
	defer q.cond.L.Unlock()

	if q.size == BufferSize {
		// queue is full, but we need to record that messages are being dropped,
		// which we do at the last message in the queue. This potentially drops
		// another message, but it's more important for the subscriber to know.
		back := (q.front + BufferSize - 1) % BufferSize
		q.q[back].msg = nil
		q.q[back].err = ErrDroppedMessages
		return
	}
	// queue is not full, insert the error
	next := (q.front + q.size) % BufferSize
	q.q[next].msg = nil
	q.q[next].err = ErrDroppedMessages
	q.size++
	q.cond.Broadcast()
}

// pqListener is an interface that represents a *pq.Listener for testing
type pqListener interface {
	io.Closer
	Listen(string) error
	Unlisten(string) error
	NotifyChan() <-chan *pq.Notification
}

type pqListenerShim struct {
	*pq.Listener
}

func (l pqListenerShim) NotifyChan() <-chan *pq.Notification {
	return l.Notify
}

// PGPubsub is a pubsub implementation using PostgreSQL.
type PGPubsub struct {
	logger     slog.Logger
	listenDone chan struct{}
	pgListener pqListener
	db         *sql.DB

	qMu    sync.Mutex
	queues map[string]map[uuid.UUID]*msgQueue

	// making the close state its own mutex domain simplifies closing logic so
	// that we don't have to hold the qMu --- which could block processing
	// notifications while the pqListener is closing.
	closeMu          sync.Mutex
	closedListener   bool
	closeListenerErr error

	publishesTotal      *prometheus.CounterVec
	subscribesTotal     *prometheus.CounterVec
	messagesTotal       *prometheus.CounterVec
	publishedBytesTotal prometheus.Counter
	receivedBytesTotal  prometheus.Counter
	disconnectionsTotal prometheus.Counter
	connected           prometheus.Gauge
}

// BufferSize is the maximum number of unhandled messages we will buffer
// for a subscriber before dropping messages.
const BufferSize = 2048

// Subscribe calls the listener when an event matching the name is received.
func (p *PGPubsub) Subscribe(event string, listener Listener) (cancel func(), err error) {
	return p.subscribeQueue(event, newMsgQueue(context.Background(), listener, nil))
}

func (p *PGPubsub) SubscribeWithErr(event string, listener ListenerWithErr) (cancel func(), err error) {
	return p.subscribeQueue(event, newMsgQueue(context.Background(), nil, listener))
}

func (p *PGPubsub) subscribeQueue(event string, newQ *msgQueue) (cancel func(), err error) {
	defer func() {
		if err != nil {
			// if we hit an error, we need to close the queue so we don't
			// leak its goroutine.
			newQ.close()
			p.subscribesTotal.WithLabelValues("false").Inc()
		} else {
			p.subscribesTotal.WithLabelValues("true").Inc()
		}
	}()

	// The pgListener waits for the response to `LISTEN` on a mainloop that also dispatches
	// notifies.  We need to avoid holding the mutex while this happens, since holding the mutex
	// blocks reading notifications and can deadlock the pgListener.
	// c.f. https://github.com/coder/coder/issues/11950
	err = p.pgListener.Listen(event)
	if err == nil {
		p.logger.Debug(context.Background(), "started listening to event channel", slog.F("event", event))
	}
	if errors.Is(err, pq.ErrChannelAlreadyOpen) {
		// It's ok if it's already open!
		err = nil
	}
	if err != nil {
		return nil, xerrors.Errorf("listen: %w", err)
	}
	p.qMu.Lock()
	defer p.qMu.Unlock()

	var eventQs map[uuid.UUID]*msgQueue
	var ok bool
	if eventQs, ok = p.queues[event]; !ok {
		eventQs = make(map[uuid.UUID]*msgQueue)
		p.queues[event] = eventQs
	}
	id := uuid.New()
	eventQs[id] = newQ
	return func() {
		p.qMu.Lock()
		listeners := p.queues[event]
		q := listeners[id]
		q.close()
		delete(listeners, id)
		if len(listeners) == 0 {
			delete(p.queues, event)
		}
		p.qMu.Unlock()
		// as above, we must not hold the lock while calling into pgListener

		if len(listeners) == 0 {
			uErr := p.pgListener.Unlisten(event)
			p.closeMu.Lock()
			defer p.closeMu.Unlock()
			if uErr != nil && !p.closedListener {
				p.logger.Warn(context.Background(), "failed to unlisten", slog.Error(uErr), slog.F("event", event))
			} else {
				p.logger.Debug(context.Background(), "stopped listening to event channel", slog.F("event", event))
			}
		}
	}, nil
}

func (p *PGPubsub) Publish(event string, message []byte) error {
	p.logger.Debug(context.Background(), "publish", slog.F("event", event), slog.F("message_len", len(message)))
	// This is safe because we are calling pq.QuoteLiteral. pg_notify doesn't
	// support the first parameter being a prepared statement.
	//nolint:gosec
	_, err := p.db.ExecContext(context.Background(), `select pg_notify(`+pq.QuoteLiteral(event)+`, $1)`, message)
	if err != nil {
		p.publishesTotal.WithLabelValues("false").Inc()
		return xerrors.Errorf("exec pg_notify: %w", err)
	}
	p.publishesTotal.WithLabelValues("true").Inc()
	p.publishedBytesTotal.Add(float64(len(message)))
	return nil
}

// Close closes the pubsub instance.
func (p *PGPubsub) Close() error {
	p.logger.Info(context.Background(), "pubsub is closing")
	err := p.closeListener()
	<-p.listenDone
	p.logger.Debug(context.Background(), "pubsub closed")
	return err
}

// closeListener closes the pgListener, unless it has already been closed.
func (p *PGPubsub) closeListener() error {
	p.closeMu.Lock()
	defer p.closeMu.Unlock()
	if p.closedListener {
		return p.closeListenerErr
	}
	p.closedListener = true
	p.closeListenerErr = p.pgListener.Close()

	return p.closeListenerErr
}

// listen begins receiving messages on the pq listener.
func (p *PGPubsub) listen() {
	defer func() {
		p.logger.Info(context.Background(), "pubsub listen stopped receiving notify")
		close(p.listenDone)
	}()

	notify := p.pgListener.NotifyChan()
	for notif := range notify {
		// A nil notification can be dispatched on reconnect.
		if notif == nil {
			p.logger.Debug(context.Background(), "notifying subscribers of a reconnection")
			p.recordReconnect()
			continue
		}
		p.listenReceive(notif)
	}
}

func (p *PGPubsub) listenReceive(notif *pq.Notification) {
	sizeLabel := messageSizeNormal
	if len(notif.Extra) >= colossalThreshold {
		sizeLabel = messageSizeColossal
	}
	p.messagesTotal.WithLabelValues(sizeLabel).Inc()
	p.receivedBytesTotal.Add(float64(len(notif.Extra)))

	p.qMu.Lock()
	defer p.qMu.Unlock()
	queues, ok := p.queues[notif.Channel]
	if !ok {
		return
	}
	extra := []byte(notif.Extra)
	for _, q := range queues {
		q.enqueue(extra)
	}
}

func (p *PGPubsub) recordReconnect() {
	p.qMu.Lock()
	defer p.qMu.Unlock()
	for _, listeners := range p.queues {
		for _, q := range listeners {
			q.dropped()
		}
	}
}

// logDialer is a pq.Dialer and pq.DialerContext that logs when it starts
// connecting and when the TCP connection is established.
type logDialer struct {
	logger slog.Logger
	d      net.Dialer
}

var (
	_ pq.Dialer        = logDialer{}
	_ pq.DialerContext = logDialer{}
)

func (d logDialer) Dial(network, address string) (net.Conn, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	return d.DialContext(ctx, network, address)
}

func (d logDialer) DialTimeout(network, address string, timeout time.Duration) (net.Conn, error) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	return d.DialContext(ctx, network, address)
}

func (d logDialer) DialContext(ctx context.Context, network, address string) (net.Conn, error) {
	deadline, hasDeadline := ctx.Deadline()
	timeoutMS := 0
	if hasDeadline {
		timeoutMS = int(time.Until(deadline) / time.Millisecond)
	}

	logger := d.logger.With(slog.F("network", network), slog.F("address", address), slog.F("timeout_ms", timeoutMS))

	logger.Info(ctx, "pubsub dialing postgres")
	start := time.Now()
	conn, err := d.d.DialContext(ctx, network, address)
	if err != nil {
		logger.Error(ctx, "pubsub failed to dial postgres")
		return nil, err
	}
	elapsed := time.Since(start)
	logger.Info(ctx, "pubsub postgres TCP connection established", slog.F("elapsed_ms", elapsed.Milliseconds()))
	return conn, nil
}

func (p *PGPubsub) startListener(ctx context.Context, connectURL string) error {
	p.connected.Set(0)
	// Creates a new listener using pq.
	var (
		errCh  = make(chan error)
		dialer = logDialer{
			logger: p.logger,
			// pq.defaultDialer uses a zero net.Dialer as well.
			d: net.Dialer{},
		}
	)
	p.pgListener = pqListenerShim{
		Listener: pq.NewDialListener(dialer, connectURL, time.Second, time.Minute, func(t pq.ListenerEventType, err error) {
			switch t {
			case pq.ListenerEventConnected:
				p.logger.Info(ctx, "pubsub connected to postgres")
				p.connected.Set(1.0)
			case pq.ListenerEventDisconnected:
				p.logger.Error(ctx, "pubsub disconnected from postgres", slog.Error(err))
				p.connected.Set(0)
			case pq.ListenerEventReconnected:
				p.logger.Info(ctx, "pubsub reconnected to postgres")
				p.connected.Set(1)
			case pq.ListenerEventConnectionAttemptFailed:
				p.logger.Error(ctx, "pubsub failed to connect to postgres", slog.Error(err))
			}
			// This callback gets events whenever the connection state changes.
			// Don't send if the errChannel has already been closed.
			select {
			case <-errCh:
				return
			default:
				errCh <- err
				close(errCh)
			}
		}),
	}
	select {
	case err := <-errCh:
		if err != nil {
			_ = p.pgListener.Close()
			return xerrors.Errorf("create pq listener: %w", err)
		}
	case <-ctx.Done():
		_ = p.pgListener.Close()
		return ctx.Err()
	}
	return nil
}

// these are the metrics we compute implicitly from our existing data structures
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
)

// We'll track messages as size "normal" and "colossal", where the
// latter are messages larger than 7600 bytes, or 95% of the postgres
// notify limit. If we see a lot of colossal packets that's an indication that
// we might be trying to send too much data over the pubsub and are in danger of
// failing to publish.
const (
	colossalThreshold   = 7600
	messageSizeNormal   = "normal"
	messageSizeColossal = "colossal"
)

// Describe implements, along with Collect, the prometheus.Collector interface
// for metrics.
func (p *PGPubsub) Describe(descs chan<- *prometheus.Desc) {
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
}

// Collect implements, along with Describe, the prometheus.Collector interface
// for metrics
func (p *PGPubsub) Collect(metrics chan<- prometheus.Metric) {
	// explicit metrics
	p.publishesTotal.Collect(metrics)
	p.subscribesTotal.Collect(metrics)
	p.messagesTotal.Collect(metrics)
	p.publishedBytesTotal.Collect(metrics)
	p.receivedBytesTotal.Collect(metrics)
	p.disconnectionsTotal.Collect(metrics)
	p.connected.Collect(metrics)

	// implicit metrics
	p.qMu.Lock()
	events := len(p.queues)
	subs := 0
	for _, subscriberMap := range p.queues {
		subs += len(subscriberMap)
	}
	p.qMu.Unlock()
	metrics <- prometheus.MustNewConstMetric(currentSubscribersDesc, prometheus.GaugeValue, float64(subs))
	metrics <- prometheus.MustNewConstMetric(currentEventsDesc, prometheus.GaugeValue, float64(events))
}

// New creates a new Pubsub implementation using a PostgreSQL connection.
func New(startCtx context.Context, logger slog.Logger, database *sql.DB, connectURL string) (*PGPubsub, error) {
	p := newWithoutListener(logger, database)
	if err := p.startListener(startCtx, connectURL); err != nil {
		return nil, err
	}
	go p.listen()
	logger.Info(startCtx, "pubsub has started")
	return p, nil
}

// newWithoutListener creates a new PGPubsub without creating the pqListener.
func newWithoutListener(logger slog.Logger, database *sql.DB) *PGPubsub {
	return &PGPubsub{
		logger:     logger,
		listenDone: make(chan struct{}),
		db:         database,
		queues:     make(map[string]map[uuid.UUID]*msgQueue),

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
			Help:      "Total number of messages received from postgres",
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
			Help:      "Total number of times we disconnected unexpectedly from postgres",
		}),
		connected: prometheus.NewGauge(prometheus.GaugeOpts{
			Namespace: "coder",
			Subsystem: "pubsub",
			Name:      "connected",
			Help:      "Whether we are connected (1) or not connected (0) to postgres",
		}),
	}
}
