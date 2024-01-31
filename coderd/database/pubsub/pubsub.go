package pubsub

import (
	"context"
	"database/sql"
	"errors"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/lib/pq"
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

// Pubsub implementation using PostgreSQL.
type pgPubsub struct {
	ctx              context.Context
	cancel           context.CancelFunc
	logger           slog.Logger
	listenDone       chan struct{}
	pgListener       *pq.Listener
	db               *sql.DB
	mut              sync.Mutex
	queues           map[string]map[uuid.UUID]*msgQueue
	closedListener   bool
	closeListenerErr error
}

// BufferSize is the maximum number of unhandled messages we will buffer
// for a subscriber before dropping messages.
const BufferSize = 2048

// Subscribe calls the listener when an event matching the name is received.
func (p *pgPubsub) Subscribe(event string, listener Listener) (cancel func(), err error) {
	return p.subscribeQueue(event, newMsgQueue(p.ctx, listener, nil))
}

func (p *pgPubsub) SubscribeWithErr(event string, listener ListenerWithErr) (cancel func(), err error) {
	return p.subscribeQueue(event, newMsgQueue(p.ctx, nil, listener))
}

func (p *pgPubsub) subscribeQueue(event string, newQ *msgQueue) (cancel func(), err error) {
	p.mut.Lock()
	defer p.mut.Unlock()
	defer func() {
		if err != nil {
			// if we hit an error, we need to close the queue so we don't
			// leak its goroutine.
			newQ.close()
		}
	}()

	err = p.pgListener.Listen(event)
	if err == nil {
		p.logger.Debug(p.ctx, "started listening to event channel", slog.F("event", event))
	}
	if errors.Is(err, pq.ErrChannelAlreadyOpen) {
		// It's ok if it's already open!
		err = nil
	}
	if err != nil {
		return nil, xerrors.Errorf("listen: %w", err)
	}

	var eventQs map[uuid.UUID]*msgQueue
	var ok bool
	if eventQs, ok = p.queues[event]; !ok {
		eventQs = make(map[uuid.UUID]*msgQueue)
		p.queues[event] = eventQs
	}
	id := uuid.New()
	eventQs[id] = newQ
	return func() {
		p.mut.Lock()
		defer p.mut.Unlock()
		listeners := p.queues[event]
		q := listeners[id]
		q.close()
		delete(listeners, id)

		if len(listeners) == 0 {
			uErr := p.pgListener.Unlisten(event)
			if uErr != nil && !p.closedListener {
				p.logger.Warn(p.ctx, "failed to unlisten", slog.Error(uErr), slog.F("event", event))
			} else {
				p.logger.Debug(p.ctx, "stopped listening to event channel", slog.F("event", event))
			}
		}
	}, nil
}

func (p *pgPubsub) Publish(event string, message []byte) error {
	p.logger.Debug(p.ctx, "publish", slog.F("event", event), slog.F("message_len", len(message)))
	// This is safe because we are calling pq.QuoteLiteral. pg_notify doesn't
	// support the first parameter being a prepared statement.
	//nolint:gosec
	_, err := p.db.ExecContext(p.ctx, `select pg_notify(`+pq.QuoteLiteral(event)+`, $1)`, message)
	if err != nil {
		return xerrors.Errorf("exec pg_notify: %w", err)
	}
	return nil
}

// Close closes the pubsub instance.
func (p *pgPubsub) Close() error {
	p.logger.Info(p.ctx, "pubsub is closing")
	p.cancel()
	err := p.closeListener()
	<-p.listenDone
	p.logger.Debug(p.ctx, "pubsub closed")
	return err
}

// closeListener closes the pgListener, unless it has already been closed.
func (p *pgPubsub) closeListener() error {
	p.mut.Lock()
	defer p.mut.Unlock()
	if p.closedListener {
		return p.closeListenerErr
	}
	p.closeListenerErr = p.pgListener.Close()
	p.closedListener = true
	return p.closeListenerErr
}

// listen begins receiving messages on the pq listener.
func (p *pgPubsub) listen() {
	defer func() {
		p.logger.Info(p.ctx, "pubsub listen stopped receiving notify")
		cErr := p.closeListener()
		if cErr != nil {
			p.logger.Error(p.ctx, "failed to close listener")
		}
		close(p.listenDone)
	}()

	var (
		notif *pq.Notification
		ok    bool
	)
	for {
		select {
		case <-p.ctx.Done():
			return
		case notif, ok = <-p.pgListener.Notify:
			if !ok {
				return
			}
		}
		// A nil notification can be dispatched on reconnect.
		if notif == nil {
			p.logger.Debug(p.ctx, "notifying subscribers of a reconnection")
			p.recordReconnect()
			continue
		}
		p.listenReceive(notif)
	}
}

func (p *pgPubsub) listenReceive(notif *pq.Notification) {
	p.mut.Lock()
	defer p.mut.Unlock()
	queues, ok := p.queues[notif.Channel]
	if !ok {
		return
	}
	extra := []byte(notif.Extra)
	for _, q := range queues {
		q.enqueue(extra)
	}
}

func (p *pgPubsub) recordReconnect() {
	p.mut.Lock()
	defer p.mut.Unlock()
	for _, listeners := range p.queues {
		for _, q := range listeners {
			q.dropped()
		}
	}
}

// New creates a new Pubsub implementation using a PostgreSQL connection.
func New(ctx context.Context, logger slog.Logger, database *sql.DB, connectURL string) (Pubsub, error) {
	// Creates a new listener using pq.
	errCh := make(chan error)
	listener := pq.NewListener(connectURL, time.Second, time.Minute, func(t pq.ListenerEventType, err error) {
		switch t {
		case pq.ListenerEventConnected:
			logger.Info(ctx, "pubsub connected to postgres")
		case pq.ListenerEventDisconnected:
			logger.Error(ctx, "pubsub disconnected from postgres", slog.Error(err))
		case pq.ListenerEventReconnected:
			logger.Info(ctx, "pubsub reconnected to postgres")
		case pq.ListenerEventConnectionAttemptFailed:
			logger.Error(ctx, "pubsub failed to connect to postgres", slog.Error(err))
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
	})
	select {
	case err := <-errCh:
		if err != nil {
			_ = listener.Close()
			return nil, xerrors.Errorf("create pq listener: %w", err)
		}
	case <-ctx.Done():
		_ = listener.Close()
		return nil, ctx.Err()
	}

	// Start a new context that will be canceled when the pubsub is closed.
	ctx, cancel := context.WithCancel(context.Background())
	pgPubsub := &pgPubsub{
		ctx:        ctx,
		cancel:     cancel,
		logger:     logger,
		listenDone: make(chan struct{}),
		db:         database,
		pgListener: listener,
		queues:     make(map[string]map[uuid.UUID]*msgQueue),
	}
	go pgPubsub.listen()
	logger.Info(ctx, "pubsub has started")
	return pgPubsub, nil
}
