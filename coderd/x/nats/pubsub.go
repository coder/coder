package nats

import (
	"context"
	"errors"
	"sync"
	"time"

	natsserver "github.com/nats-io/nats-server/v2/server"
	natsgo "github.com/nats-io/nats.go"
	"golang.org/x/xerrors"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/coderd/database/pubsub"
)

// Pubsub is an experimental embedded NATS-backed implementation of
// pubsub.Pubsub. See package doc for status.
type Pubsub struct {
	logger slog.Logger
	opts   Options

	ns *natsserver.Server
	nc *natsgo.Conn

	ownsServer bool
	ownsConn   bool

	mu        sync.Mutex
	closed    bool
	subs      map[*subscription]struct{}
	closeOnce sync.Once

	// closedCh is signaled by the NATS ClosedHandler so Close can wait
	// for Drain to fully complete without polling.
	closedCh chan struct{}
}

type subscription struct {
	sub        *natsgo.Subscription
	ctx        context.Context
	cancel     context.CancelFunc
	wg         sync.WaitGroup
	cancelOnce sync.Once
}

// Compile-time assertion that *Pubsub satisfies the pubsub.Pubsub interface.
var _ pubsub.Pubsub = (*Pubsub)(nil)

// New creates a new embedded NATS Pubsub. The returned *Pubsub owns the
// embedded server and client connection and shuts them down on Close.
func New(ctx context.Context, logger slog.Logger, opts Options) (*Pubsub, error) {
	_ = ctx
	ns, err := startEmbeddedServer(opts)
	if err != nil {
		return nil, err
	}

	closedCh := make(chan struct{})
	var closeOnce sync.Once
	handlers := connHandlers{
		disconnectErr: func(_ *natsgo.Conn, err error) {
			if err != nil {
				logger.Warn(context.Background(), "nats client disconnected", slog.Error(err))
			}
		},
		reconnect: func(_ *natsgo.Conn) {
			logger.Info(context.Background(), "nats client reconnected")
		},
		closed: func(_ *natsgo.Conn) {
			closeOnce.Do(func() { close(closedCh) })
			logger.Debug(context.Background(), "nats client closed")
		},
		errH: func(_ *natsgo.Conn, _ *natsgo.Subscription, err error) {
			if err != nil {
				logger.Warn(context.Background(), "nats async error", slog.Error(err))
			}
		},
	}

	nc, err := connectInProcess(ns, opts, handlers)
	if err != nil {
		ns.Shutdown()
		ns.WaitForShutdown()
		return nil, err
	}

	return &Pubsub{
		logger:     logger,
		opts:       opts,
		ns:         ns,
		nc:         nc,
		ownsServer: true,
		ownsConn:   true,
		subs:       make(map[*subscription]struct{}),
		closedCh:   closedCh,
	}, nil
}

// NewFromConn wraps an externally provided *natsgo.Conn. The returned
// *Pubsub does not own the connection; Close cancels package-owned
// subscriptions but does not drain or close the connection or any server.
func NewFromConn(logger slog.Logger, nc *natsgo.Conn) (*Pubsub, error) {
	if nc == nil {
		return nil, xerrors.New("nats: nil connection")
	}
	return &Pubsub{
		logger: logger,
		nc:     nc,
		subs:   make(map[*subscription]struct{}),
	}, nil
}

// Publish publishes a message under the given legacy event name.
func (p *Pubsub) Publish(event string, message []byte) error {
	p.mu.Lock()
	if p.closed {
		p.mu.Unlock()
		return xerrors.New("nats pubsub: closed")
	}
	p.mu.Unlock()

	subj, err := LegacyEventSubject(event)
	if err != nil {
		return xerrors.Errorf("map event %q: %w", event, err)
	}
	if err := p.nc.Publish(string(subj), message); err != nil {
		return xerrors.Errorf("publish: %w", err)
	}
	if p.opts.PublishMode == PublishModeFlush {
		timeout := p.opts.PublishFlushTimeout
		if timeout == 0 {
			timeout = DefaultPublishFlushLimit
		}
		if err := p.nc.FlushTimeout(timeout); err != nil {
			return xerrors.Errorf("flush: %w", err)
		}
	}
	return nil
}

// Subscribe subscribes a Listener to the given legacy event name. Errors
// such as ErrDroppedMessages are silently ignored, mirroring the legacy
// pubsub Listener semantics.
func (p *Pubsub) Subscribe(event string, listener pubsub.Listener) (cancel func(), err error) {
	return p.SubscribeWithErr(event, func(ctx context.Context, msg []byte, err error) {
		if err != nil {
			return
		}
		listener(ctx, msg)
	})
}

// SubscribeWithErr subscribes a ListenerWithErr to the given legacy event
// name. The listener also receives error deliveries such as
// pubsub.ErrDroppedMessages.
func (p *Pubsub) SubscribeWithErr(event string, listener pubsub.ListenerWithErr) (cancel func(), err error) {
	p.mu.Lock()
	if p.closed {
		p.mu.Unlock()
		return nil, xerrors.New("nats pubsub: closed")
	}
	p.mu.Unlock()

	subj, err := LegacyEventSubject(event)
	if err != nil {
		return nil, xerrors.Errorf("map event %q: %w", event, err)
	}
	natsSub, err := p.nc.SubscribeSync(string(subj))
	if err != nil {
		return nil, xerrors.Errorf("subscribe: %w", err)
	}
	if p.opts.PendingLimits.Msgs != 0 || p.opts.PendingLimits.Bytes != 0 {
		if err := natsSub.SetPendingLimits(p.opts.PendingLimits.Msgs, p.opts.PendingLimits.Bytes); err != nil {
			_ = natsSub.Unsubscribe()
			return nil, xerrors.Errorf("set pending limits: %w", err)
		}
	}

	ctx, cancelCtx := context.WithCancel(context.Background())
	s := &subscription{
		sub:    natsSub,
		ctx:    ctx,
		cancel: cancelCtx,
	}

	p.mu.Lock()
	p.subs[s] = struct{}{}
	p.mu.Unlock()

	s.wg.Add(1)
	go p.runSubscription(s, listener)

	cancelFn := func() {
		s.cancelOnce.Do(func() {
			s.cancel()
			_ = s.sub.Unsubscribe()
			s.wg.Wait()
			p.mu.Lock()
			delete(p.subs, s)
			p.mu.Unlock()
		})
	}
	return cancelFn, nil
}

func (p *Pubsub) runSubscription(s *subscription, listener pubsub.ListenerWithErr) {
	defer s.wg.Done()
	for {
		msg, err := s.sub.NextMsgWithContext(s.ctx)
		if err != nil {
			switch {
			case errors.Is(err, context.Canceled), errors.Is(err, context.DeadlineExceeded):
				return
			case errors.Is(err, natsgo.ErrConnectionClosed),
				errors.Is(err, natsgo.ErrBadSubscription):
				return
			case errors.Is(err, natsgo.ErrSlowConsumer):
				// Best-effort drop signal. Per-event dedup is deferred.
				listener(s.ctx, nil, pubsub.ErrDroppedMessages)
				continue
			default:
				p.logger.Warn(s.ctx, "nats subscription error", slog.Error(err))
				return
			}
		}
		listener(s.ctx, msg.Data, nil)
	}
}

// Close drains and shuts down the Pubsub. It is idempotent.
func (p *Pubsub) Close() error {
	var errs []error
	p.closeOnce.Do(func() {
		p.mu.Lock()
		p.closed = true
		subs := make([]*subscription, 0, len(p.subs))
		for s := range p.subs {
			subs = append(subs, s)
		}
		p.mu.Unlock()

		for _, s := range subs {
			s.cancelOnce.Do(func() {
				s.cancel()
				_ = s.sub.Unsubscribe()
				s.wg.Wait()
				p.mu.Lock()
				delete(p.subs, s)
				p.mu.Unlock()
			})
		}

		if p.ownsConn {
			drainTimeout := p.opts.DrainTimeout
			if drainTimeout <= 0 {
				drainTimeout = 30 * time.Second
			}
			if err := p.nc.Drain(); err != nil {
				p.nc.Close()
				errs = append(errs, xerrors.Errorf("drain: %w", err))
			} else {
				select {
				case <-p.closedCh:
				case <-time.After(drainTimeout):
					p.nc.Close()
					errs = append(errs, xerrors.Errorf("drain timeout after %s", drainTimeout))
				}
			}
		}

		if p.ownsServer {
			p.ns.Shutdown()
			p.ns.WaitForShutdown()
		}
	})
	return errors.Join(errs...)
}
