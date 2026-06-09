package nats

import (
	"context"
	"errors"
	"fmt"
	"hash/fnv"
	"net/url"
	"sync"
	"time"

	natsserver "github.com/nats-io/nats-server/v2/server"
	natsgo "github.com/nats-io/nats.go"
	"golang.org/x/xerrors"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/coderd/database/pubsub"
)

// DefaultMaxPending is the per-client outbound pending byte budget.
const DefaultMaxPending int64 = 128 << 20

const (
	defaultClusterName   = "coder"
	defaultClusterPort   = 6222
	defaultRoutePoolSize = 3
)

var errClosed = xerrors.New("nats pubsub closed")

// PendingLimits configures per-subscription NATS pending limits set
// via SetPendingLimits on each *natsgo.Subscription.
type PendingLimits struct {
	// Msgs is the per-subscription pending message limit. Positive
	// values also set each local listener queue capacity.
	// Zero uses the package default. Negative disables this limit.
	Msgs int

	// Bytes is the per-subscription pending byte limit.
	// Zero uses the package default. Negative disables this limit.
	Bytes int
}

// Options configures the embedded NATS Pubsub.
type Options struct {
	// MaxPayload is the NATS max payload. Zero means server default.
	MaxPayload int32

	// MaxPending is the per-client outbound pending byte budget on the
	// embedded server. Zero or negative means the package default,
	// 128 MiB.
	MaxPending int64

	// PendingLimits configures per-subscription NATS pending limits.
	// Positive Msgs also sets local listener queue capacity.
	// Zero fields use package defaults: Msgs -1 and Bytes 512 MiB.
	PendingLimits PendingLimits

	// ReconnectWait controls client reconnect delay. Zero keeps the
	// NATS default.
	ReconnectWait time.Duration

	// InProcess, when true, uses nats.InProcessServer instead of TCP
	// loopback. Intended for benchmarks and tests.
	InProcess bool

	// PublishConns is the number of publisher connections. Each Publish
	// is routed by a stable hash of the subject. Zero or negative means 1.
	PublishConns int

	// SubscribeConns is the number of subscriber connections. Each
	// shared subscription is pinned to one connection by a stable hash
	// of its subject. Zero or negative means 1.
	SubscribeConns int

	// ClusterHost is the embedded NATS route listener host. Empty means
	// all interfaces when cluster mode is enabled.
	ClusterHost string

	// ClusterPort is the embedded NATS route listener port. Zero means
	// 6222 when cluster mode is enabled.
	ClusterPort int

	// ClusterAuthToken is the shared route authentication token for
	// clustered embedded NATS servers. Empty disables route auth.
	ClusterAuthToken string

	// PeerFetcher provides the current set of peer route addresses.
	// RefreshPeers uses it to update the configured cluster routes.
	PeerFetcher PeerFetcher

	// RoutePoolSize is the NATS route pool size. Zero means the package
	// default when cluster mode is enabled.
	RoutePoolSize int

	// disableCluster is intended only for testing. Since we cannot reload a server
	// with a cluster host/port after initialization, we start all production servers
	// with clustering enabled.
	disableCluster bool
}

// Pubsub is an embedded NATS-backed implementation of pubsub.Pubsub.
//
// Each Pubsub owns one embedded server, a pool of publisher
// *natsgo.Conns (Options.PublishConns) and a pool of subscriber
// *natsgo.Conns (Options.SubscribeConns). Publishes and shared
// subscriptions are pinned to a connection by a stable hash of the
// subject, so same-subject traffic preserves per-subject ordering and
// every local subscriber for a subject coalesces onto one underlying
// *natsgo.Subscription.
type Pubsub struct {
	mu sync.Mutex

	logger slog.Logger
	opts   Options

	Server *natsserver.Server
	// publishPool and subscribePool are immutable after construction so
	// the hot path can index without holding p.mu.
	publishPool   []*natsgo.Conn
	subscribePool []*natsgo.Conn

	// subscriptions coalesces concurrent local subscribers on the
	// same subject onto a single underlying *natsgo.Subscription.
	subscriptions map[string]*natsSub
	closeOnce     sync.Once

	// ctx is canceled by Close while holding p.mu so subscriber state
	// cleanup observes the canceled context.
	ctx    context.Context
	cancel context.CancelFunc

	clusterMu     sync.Mutex
	clustered     bool
	serverOpts    *natsserver.Options
	currentRoutes []*url.URL

	peerFetcher PeerFetcher
	peerRefresh chan struct{}
}

// natsSub maps to one underlying *natsgo.Subscription. The first
// local subscriber creates it; later local subscribers attach to it.
// When the last local subscriber detaches, the NATS subscription is
// unsubscribed.
type natsSub struct {
	// sub is set before this natsSub is published in Pubsub.subscriptions
	// and is immutable after that.
	sub *natsgo.Subscription

	// mu guards localSubs.
	mu sync.Mutex
	// localSubs are the local subscribers attached to this NATS subscription.
	localSubs map[*localSub]struct{}

	// dropMu keeps async error accounting independent from listener fan-out.
	dropMu sync.Mutex
	// lastDropped is the cumulative NATS dropped count last reported locally.
	lastDropped uint64
}

// localSub is the local handle returned by Subscribe /
// SubscribeWithErr. Each local subscriber gets its own bounded inbox
// and dispatcher goroutine so one slow listener cannot block peers on
// the same subject.
type localSub struct {
	cancelOnce sync.Once

	ctx context.Context

	event    string
	listener pubsub.ListenerWithErr

	// queue is the per-listener data fan-out inbox. The shared NATS
	// callback enqueues non-blockingly; on overflow the message is
	// dropped and a drop signal is raised.
	queue chan []byte
	// dropSignal is a size-1 buffered channel that coalesces drop
	// notifications from local overflow and NATS slow-consumer
	// broadcasts onto a single pending wake.
	dropSignal chan struct{}
	cancel     context.CancelFunc
}

// Compile-time assertion that *Pubsub satisfies the pubsub.Pubsub interface.
var _ pubsub.Pubsub = (*Pubsub)(nil)

// newPubsub allocates a *Pubsub with initialized maps and cancel ctx.
func newPubsub(ctx context.Context, logger slog.Logger, opts Options) *Pubsub {
	ctx, cancel := context.WithCancel(ctx)
	return &Pubsub{
		logger:        logger,
		opts:          opts,
		subscriptions: make(map[string]*natsSub),
		ctx:           ctx,
		cancel:        cancel,
		peerFetcher:   opts.PeerFetcher,
		peerRefresh:   make(chan struct{}, 1),
	}
}

// defaultPendingLimits returns the effective per-subscription pending
// limits applied at Subscribe time.
func defaultPendingLimits(in PendingLimits) PendingLimits {
	out := in
	if out.Msgs == 0 {
		out.Msgs = -1
	}
	if out.Bytes == 0 {
		out.Bytes = 512 * 1024 * 1024
	}
	return out
}

// buildConnHandlers returns the connHandlers stack installed on every
// owned connection. Handlers close over p so slow-consumer routing
// keeps working.
func (p *Pubsub) buildConnHandlers() connHandlers {
	return connHandlers{
		disconnectErr: func(conn *natsgo.Conn, err error) {
			if err != nil {
				p.logger.Warn(p.ctx, "nats client disconnected", slog.Error(err))
			}
			p.signalSubscribersDroppedForConn(conn)
		},
		reconnect: func(_ *natsgo.Conn) {
			p.logger.Info(p.ctx, "nats client reconnected")
		},
		closed: func(_ *natsgo.Conn) {
			p.logger.Debug(p.ctx, "nats client closed")
		},
		errH: func(_ *natsgo.Conn, sub *natsgo.Subscription, err error) {
			if err != nil && errors.Is(err, natsgo.ErrSlowConsumer) {
				p.handleAsyncError(sub, err)
				return
			}
			if err != nil {
				p.logger.Warn(p.ctx, "nats async error", slog.Error(err))
			}
		},
	}
}

// New creates an embedded NATS Pubsub. The returned *Pubsub owns the
// embedded server and the publisher and subscriber connection pools.
// Close shuts down all owned resources.
func New(ctx context.Context, logger slog.Logger, opts Options) (*Pubsub, error) {
	sopts, err := buildServerOptions(opts)
	if err != nil {
		return nil, err
	}

	ns, err := startEmbeddedServer(sopts)
	if err != nil {
		return nil, err
	}

	logger.Info(context.Background(), "embedded nats server started",
		slog.F("client_url", ns.ClientURL()),
	)

	if opts.PeerFetcher == nil {
		opts.PeerFetcher = NopPeerFetcher{}
	}

	p := newPubsub(ctx, logger, opts)
	p.Server = ns
	p.clustered = !opts.disableCluster
	p.serverOpts = sopts.Clone()
	p.currentRoutes = cloneRouteURLs(sopts.Routes)
	handlers := p.buildConnHandlers()

	publishPool, err := newConnPool(ns, opts, handlers, opts.PublishConns, "coder-pubsub-pub")
	if err != nil {
		p.cancel()
		ns.Shutdown()
		ns.WaitForShutdown()
		return nil, err
	}

	subscribePool, err := newConnPool(ns, opts, handlers, opts.SubscribeConns, "coder-pubsub-sub")
	if err != nil {
		p.cancel()
		for _, c := range publishPool {
			c.Close()
		}
		ns.Shutdown()
		ns.WaitForShutdown()
		return nil, err
	}

	p.publishPool = publishPool
	p.subscribePool = subscribePool

	if p.clustered {
		go p.runPeerRefresh()
	}
	go func() {
		<-p.ctx.Done()
		_ = p.Close()
	}()

	return p, nil
}

func newConnPool(ns *natsserver.Server, opts Options, handlers connHandlers, count int, clientName string) ([]*natsgo.Conn, error) {
	if count <= 0 {
		count = 1
	}
	pool := make([]*natsgo.Conn, 0, count)
	for i := 0; i < count; i++ {
		// Suffix names when the pool has more than one entry so server
		// logs can distinguish connections.
		name := clientName
		if count > 1 {
			name = fmt.Sprintf("%s-%d", clientName, i)
		}
		nc, err := connectClient(ns, opts, handlers, name)
		if err != nil {
			for _, c := range pool {
				c.Close()
			}
			return nil, xerrors.Errorf("dial conn: %w", err)
		}
		pool = append(pool, nc)
	}
	return pool, nil
}

// Publish publishes a message under the given event name. The
// publisher connection is selected by a stable hash of the subject so
// same-subject publishes preserve per-subject ordering.
func (p *Pubsub) Publish(event string, message []byte) error {
	if p.ctx.Err() != nil {
		return errClosed
	}

	if err := pickConn(p.publishPool, event).Publish(event, message); err != nil {
		return xerrors.Errorf("publish: %w", err)
	}
	return nil
}

// Flush blocks until every publisher connection has flushed buffered
// publishes to the embedded server. Returns the first error
// encountered; remaining connections are still flushed.
func (p *Pubsub) Flush() error {
	if p.ctx.Err() != nil {
		return errClosed
	}

	var firstErr error
	for i, nc := range p.publishPool {
		if err := nc.Flush(); err != nil && firstErr == nil {
			firstErr = xerrors.Errorf("flush pub conn %d: %w", i, err)
		}
	}
	return firstErr
}

// Subscribe subscribes a Listener to the given event name. Errors
// such as ErrDroppedMessages are silently ignored, mirroring the
// legacy pubsub Listener semantics.
func (p *Pubsub) Subscribe(event string, listener pubsub.Listener) (cancel func(), err error) {
	return p.SubscribeWithErr(event, func(ctx context.Context, msg []byte, err error) {
		if err != nil {
			return
		}
		listener(ctx, msg)
	})
}

// SubscribeWithErr subscribes a ListenerWithErr to the given event
// name. The listener also receives error deliveries such as
// pubsub.ErrDroppedMessages. Multiple local subscribers on the same
// event share a single underlying *natsgo.Subscription with
// per-listener bounded inboxes so a slow listener cannot block its
// peers.
func (p *Pubsub) SubscribeWithErr(event string, listener pubsub.ListenerWithErr) (cancel func(), err error) {
	s, err := p.addSubscriber(event, listener)
	if err != nil {
		return nil, err
	}

	cancelFn := func() {
		s.close()
		p.unsubscribeLocal(s)
	}
	return cancelFn, nil
}

// listenerQueueSize returns the per-listener inbox capacity. A
// positive PendingLimits.Msgs sets the cap (giving callers a knob to
// trigger local-overflow drops since coalescing makes NATS-level
// slow-consumer signals rare). Otherwise the default is used.
func listenerQueueSize(in PendingLimits) int {
	if in.Msgs > 0 {
		return in.Msgs
	}
	return defaultListenerQueueSize
}

const defaultListenerQueueSize = 1024

// addSubscriber creates a local subscriber and attaches it to the natsSub
// for event. New natsSub entries are published only after NATS setup succeeds.
func (p *Pubsub) addSubscriber(event string, listener pubsub.ListenerWithErr) (*localSub, error) {
	ctx, cancel := context.WithCancel(p.ctx)
	s := &localSub{
		ctx:        ctx,
		cancel:     cancel,
		event:      event,
		listener:   listener,
		queue:      make(chan []byte, listenerQueueSize(p.opts.PendingLimits)),
		dropSignal: make(chan struct{}, 1),
	}
	s.init()

	cleanupSub, err := func() (*natsgo.Subscription, error) {
		p.mu.Lock()
		defer p.mu.Unlock()

		if p.ctx.Err() != nil {
			return nil, errClosed
		}

		nsub, ok := p.subscriptions[event]
		if ok {
			nsub.mu.Lock()
			nsub.localSubs[s] = struct{}{}
			nsub.mu.Unlock()
			return nsub.sub, nil
		}

		nsub = &natsSub{
			localSubs: map[*localSub]struct{}{
				s: {},
			},
		}

		subConn := pickConn(p.subscribePool, event)
		natsSubscription, err := subConn.Subscribe(event, nsub.handleMessage)
		if err != nil {
			return nil, xerrors.Errorf("subscribe: %w", err)
		}
		nsub.sub = natsSubscription

		// Flush the SUB to the server so a publish issued immediately
		// after Subscribe returns cannot race ahead of registration.
		if err := subConn.Flush(); err != nil {
			return natsSubscription, xerrors.Errorf("flush subscribe: %w", err)
		}
		limits := defaultPendingLimits(p.opts.PendingLimits)
		if err := natsSubscription.SetPendingLimits(limits.Msgs, limits.Bytes); err != nil {
			return natsSubscription, xerrors.Errorf("set pending limits: %w", err)
		}

		p.subscriptions[event] = nsub
		return natsSubscription, nil
	}()
	if err != nil {
		s.close()
		if cleanupSub != nil {
			if unsubscribeErr := cleanupSub.Unsubscribe(); unsubscribeErr != nil {
				err = errors.Join(err, xerrors.Errorf("unsubscribe: %w", unsubscribeErr))
			}
		}
		return nil, err
	}
	return s, nil
}

// unsubscribeLocal removes s from its natsSub. If s was the last
// listener, it also removes and unsubscribes the underlying NATS
// subscription.
func (p *Pubsub) unsubscribeLocal(s *localSub) {
	natsSub := func() *natsgo.Subscription {
		p.mu.Lock()
		defer p.mu.Unlock()

		nsub := p.subscriptions[s.event]
		if nsub == nil {
			return nil
		}

		nsub.mu.Lock()
		defer nsub.mu.Unlock()
		if _, tracked := nsub.localSubs[s]; !tracked {
			return nil
		}
		delete(nsub.localSubs, s)
		if len(nsub.localSubs) > 0 {
			return nil
		}
		// Last listener: remove the nsub entry so a new Subscribe to this
		// subject creates a fresh underlying subscription.
		delete(p.subscriptions, s.event)
		return nsub.sub
	}()
	if natsSub != nil {
		_ = natsSub.Unsubscribe()
	}
}

// handleMessage handles messages for the shared subscription. Each
// enqueue is non-blocking and does not call user code, so one slow
// listener cannot stall the NATS delivery goroutine.
//
// Zero-copy fan-out: the same msg.Data slice is delivered to every
// local listener without cloning. Listeners on a coalesced subject MUST
// treat the delivered bytes as immutable.
func (nsub *natsSub) handleMessage(msg *natsgo.Msg) {
	nsub.mu.Lock()
	defer nsub.mu.Unlock()

	for s := range nsub.localSubs {
		s.enqueue(msg.Data)
	}
}

// init starts the per-listener delivery goroutine.
func (s *localSub) init() {
	go func() {
		for {
			select {
			case <-s.ctx.Done():
				return
			case data := <-s.queue:
				s.listener(s.ctx, data, nil)
			case <-s.dropSignal:
				s.listener(s.ctx, nil, pubsub.ErrDroppedMessages)
			}
		}
	}()
}

// close cancels local delivery without waiting for callbacks.
func (s *localSub) close() {
	s.cancelOnce.Do(func() {
		if s.cancel != nil {
			s.cancel()
		}
	})
}

// enqueue non-blockingly sends data onto s.queue. On overflow it drops the
// message and raises a drop signal so pubsub.ErrDroppedMessages is surfaced.
// If s is canceled the message is silently dropped.
func (s *localSub) enqueue(data []byte) {
	select {
	case s.queue <- data:
	default:
		s.signalDrop()
	}
}

// signalDrop pushes onto dropSignal without blocking. Multiple drops
// between dispatcher dequeues coalesce into a single pending signal, so
// the listener observes one ErrDroppedMessages per drop wave.
func (s *localSub) signalDrop() {
	select {
	case s.dropSignal <- struct{}{}:
	default:
	}
}

// signalSubscribersDroppedForConn signals local subscribers assigned to conn.
func (p *Pubsub) signalSubscribersDroppedForConn(conn *natsgo.Conn) {
	if conn == nil || len(p.subscribePool) == 0 {
		return
	}

	p.mu.Lock()
	subs := make([]*localSub, 0)
	for event, nsub := range p.subscriptions {
		if pickConn(p.subscribePool, event) != conn {
			continue
		}
		nsub.mu.Lock()
		for s := range nsub.localSubs {
			subs = append(subs, s)
		}
		nsub.mu.Unlock()
	}
	p.mu.Unlock()

	for _, s := range subs {
		s.signalDrop()
	}
}

// handleAsyncError routes async error callbacks. Only slow-consumer
// errors trigger drop accounting.
func (p *Pubsub) handleAsyncError(sub *natsgo.Subscription, err error) {
	if sub == nil || !errors.Is(err, natsgo.ErrSlowConsumer) {
		return
	}
	p.mu.Lock()
	var nsub *natsSub
	for _, candidate := range p.subscriptions {
		if candidate.sub == sub {
			nsub = candidate
			break
		}
	}
	p.mu.Unlock()
	if nsub == nil {
		return
	}
	p.handleSlowSubscriber(nsub)
}

// handleSlowSubscriber broadcasts pubsub.ErrDroppedMessages to every
// local listener on nsub when NATS reports a new drop delta. The
// slow-consumer signal is per-subscription and cannot be narrowed to a
// single local listener.
func (p *Pubsub) handleSlowSubscriber(nsub *natsSub) {
	nsub.dropMu.Lock()
	dropped, err := nsub.sub.Dropped()
	if err != nil {
		nsub.dropMu.Unlock()
		p.logger.Warn(p.ctx, "nats: query dropped count", slog.Error(err))
		return
	}
	if dropped < 0 {
		nsub.dropMu.Unlock()
		p.logger.Warn(p.ctx, "nats: negative dropped count")
		return
	}
	// Dropped is cumulative per subscription; signal only new drops.
	droppedCount := uint64(dropped)
	if droppedCount < nsub.lastDropped {
		nsub.lastDropped = droppedCount
		nsub.dropMu.Unlock()
		return
	}
	if droppedCount == nsub.lastDropped {
		nsub.dropMu.Unlock()
		return
	}
	nsub.lastDropped = droppedCount
	nsub.dropMu.Unlock()

	nsub.mu.Lock()
	defer nsub.mu.Unlock()

	for s := range nsub.localSubs {
		s.signalDrop()
	}
}

// Close stops local delivery and shuts down the Pubsub. It is idempotent.
// Close does not drain queued listener messages.
func (p *Pubsub) Close() error {
	p.closeOnce.Do(func() {
		p.mu.Lock()
		// Cancel while holding p.mu so subscriber state cleanup below
		// observes the canceled context.
		p.cancel()
		var subs []*localSub
		shareds := make([]*natsSub, 0, len(p.subscriptions))
		for _, ss := range p.subscriptions {
			shareds = append(shareds, ss)
			ss.mu.Lock()
			for s := range ss.localSubs {
				subs = append(subs, s)
				delete(ss.localSubs, s)
			}
			ss.mu.Unlock()
		}
		clear(p.subscriptions)
		p.mu.Unlock()

		// Unsubscribe shared subscriptions before closing connections.
		for _, ss := range shareds {
			if ss.sub != nil {
				_ = ss.sub.Unsubscribe()
			}
		}

		// Signal per-listener goroutines without waiting for callbacks.
		for _, s := range subs {
			s.close()
		}

		for _, nc := range p.subscribePool {
			if nc != nil {
				nc.Close()
			}
		}
		for _, nc := range p.publishPool {
			if nc != nil {
				nc.Close()
			}
		}

		if p.Server != nil {
			p.Server.Shutdown()
			p.Server.WaitForShutdown()
		}
	})
	return nil
}

// pickConn returns the connection assigned to subject. Selection uses
// a stable FNV-1a hash so same-subject traffic always targets the same
// connection within a process; pools are immutable after construction
// so the lookup is lock-free.
func pickConn(pool []*natsgo.Conn, subject string) *natsgo.Conn {
	if len(pool) == 1 {
		return pool[0]
	}
	h := fnv.New32a()
	_, _ = h.Write([]byte(subject))
	n := uint32(len(pool)) //nolint:gosec // pool size bounded by Options.{Publish,Subscribe}Conns
	return pool[h.Sum32()%n]
}
