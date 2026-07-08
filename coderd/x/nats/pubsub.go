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
	"github.com/prometheus/client_golang/prometheus"
	"golang.org/x/xerrors"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/coderd/database/pubsub"
)

// DefaultServerMaxPendingBytes caps how many bytes the embedded NATS server will
// hold in memory for a single client connection while waiting to write
// them out to that connection. Each message the server needs to deliver
// to a connection is queued in that connection's outbound buffer until
// the socket can accept it; if the consumer reads slower than messages
// arrive, the buffer grows. When it exceeds this cap, NATS declares the
// connection a slow consumer and drops messages rather than buffer
// without bound.
//
// The connection that fills this buffer in practice is the subscribe
// connection: the server writes every message bound for a replica's
// local subscribers out over its subscribe connection pool, which
// defaults to a single connection. In a cluster, cross-node fan-out
// therefore concentrates all of a replica's inbound deliveries on that
// one connection's outbound buffer. Benchmarking high-fanout cluster
// workloads (10 subjects, 10 publishers, 50 subscribers) showed the 128
// MiB default overflowing and dropping 10-15% of deliveries, while 256
// MiB and above dropped none; 512 MiB is chosen for headroom.
//
// This is a ceiling, not a reservation: the buffer grows only with
// actual backlog, so a connection that keeps up holds nearly nothing and
// raising the cap costs memory only during the overload bursts it
// absorbs.
const DefaultServerMaxPendingBytes int64 = 512 << 20

// DefaultClientMaxPendingBytes is the pending byte limit applied via
// SetPendingLimits to each coalesced *natsgo.Subscription when
// PendingLimits.Bytes is zero. Unlike DefaultServerMaxPendingBytes,
// which bounds the server-side outbound buffer for a whole connection,
// this bounds the nats.go client's per-subscription pending buffer:
// messages the client has received from the server but the
// subscription's async message handler has not yet dispatched. When
// that handler falls behind and the buffer overflows, NATS marks the
// subscription a slow consumer and drops messages, which the package
// surfaces as pubsub.ErrDroppedMessages.
const DefaultClientMaxPendingBytes = 512 * 1024 * 1024

const (
	defaultClusterName   = "coder"
	defaultClusterPort   = 6222
	defaultRoutePoolSize = 3
)

var errClosed = xerrors.New("nats pubsub closed")

// PendingLimits configures per-subscription NATS pending limits set
// via SetPendingLimits on each *natsgo.Subscription.
type PendingLimits struct {
	// Msgs is the per-subscription pending message limit. Zero or
	// negative disables the message limit, leaving the byte limit
	// (PendingLimits.Bytes) as the only per-subscription bound.
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
	// 6222 when cluster mode is enabled. NATS `server.RANDOM_PORT` can be
	// used to select a random port.
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

// conn is a stripped down version of natsgo.Conn with just the methods we use, to allow us to fake it in tests.
type conn interface {
	Publish(event string, message []byte) error
	Close()
	Flush() error
	Subscribe(event string, handler natsgo.MsgHandler) (*natsgo.Subscription, error)
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
	publishPool   []conn
	subscribePool []conn

	// subscriptions coalesces concurrent local subscribers on the
	// same subject onto a single underlying *natsgo.Subscription.
	subscriptions map[string]*groupSub
	closeOnce     sync.Once

	// ctx is canceled by Close while holding p.mu so subscriber state
	// cleanup observes the canceled context.
	ctx    context.Context
	cancel context.CancelFunc

	// unsubscribeRoutines tracks outstanding unsubscribeGroup calls while closing, to ensure they all complete before
	// we start tearing down connections.
	unsubscribeRoutines sync.WaitGroup

	clusterMu     sync.Mutex
	clustered     bool
	serverOpts    *natsserver.Options
	currentRoutes []*url.URL

	peerFetcher PeerFetcher
	peerRefresh chan struct{}

	metrics *metrics
}

// groupSub maps to one underlying *natsgo.Subscription. The first
// local subscriber creates it; later local subscribers attach to it.
// When the last local subscriber detaches, the NATS subscription is
// unsubscribed.
type groupSub struct {
	// metrics records received-message metrics from handleMessage. It is
	// the only part of the owning Pubsub that a groupSub needs.
	metrics *metrics
	event   string
	// mu guards localSubs.
	mu sync.Mutex
	// localSubs are the local subscribers attached to this NATS subscription.
	localSubs map[*localSub]struct{}

	sub *subGetter

	// dropMu keeps async error accounting independent from listener fan-out.
	dropMu sync.Mutex
	// lastDropped is the cumulative NATS dropped count last reported locally.
	lastDropped uint64
}

// localSub is the local handle returned by Subscribe /
// SubscribeWithErr.
type localSub struct {
	event string
	queue *pubsub.MsgQueue
}

// Compile-time assertion that *Pubsub satisfies the pubsub.Pubsub interface.
var _ pubsub.Pubsub = (*Pubsub)(nil)

// Compile-time assertion that *Pubsub is a prometheus.Collector.
var _ prometheus.Collector = (*Pubsub)(nil)

// Describe implements prometheus.Collector.
func (p *Pubsub) Describe(descs chan<- *prometheus.Desc) {
	p.metrics.describe(descs)
}

// Collect implements prometheus.Collector. The subscriber and event
// gauges are maintained as atomic counters by metrics, so Collect does
// not lock the Pubsub.
func (p *Pubsub) Collect(ch chan<- prometheus.Metric) {
	p.metrics.collect(ch, p)
}

// newPubsub allocates a *Pubsub with initialized maps and cancel ctx.
func newPubsub(ctx context.Context, logger slog.Logger, opts Options) *Pubsub {
	ctx, cancel := context.WithCancel(ctx)
	return &Pubsub{
		logger:        logger,
		opts:          opts,
		subscriptions: make(map[string]*groupSub),
		ctx:           ctx,
		cancel:        cancel,
		peerFetcher:   opts.PeerFetcher,
		peerRefresh:   make(chan struct{}, 1),
		metrics:       newMetrics(logger),
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
		out.Bytes = DefaultClientMaxPendingBytes
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
			p.metrics.onDisconnect()
			p.signalSubscribersDroppedForConn(conn)
		},
		reconnect: func(_ *natsgo.Conn) {
			p.logger.Info(p.ctx, "nats client reconnected")
			p.metrics.onReconnect()
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
func New(ctx context.Context, logger slog.Logger, opts Options) (pubSub *Pubsub, retErr error) {
	sopts, err := buildServerOptions(opts)
	if err != nil {
		return nil, err
	}

	ns, err := startEmbeddedServer(sopts)
	if err != nil {
		return nil, err
	}
	defer func() {
		if retErr != nil {
			ns.Shutdown()
			ns.WaitForShutdown()
		}
	}()

	logger.Info(context.Background(), "embedded nats server started",
		slog.F("client_url", ns.ClientURL()),
	)

	if opts.PeerFetcher == nil {
		opts.PeerFetcher = NopPeerFetcher{}
	}

	p := newPubsub(ctx, logger, opts)
	defer func() {
		if retErr != nil {
			p.cancel()
		}
	}()
	p.Server = ns
	p.clustered = !opts.disableCluster
	p.serverOpts = sopts.Clone()
	p.currentRoutes = cloneRouteURLs(sopts.Routes)
	handlers := p.buildConnHandlers()

	publishPool, err := newConnPool(ns, opts, handlers, opts.PublishConns, "coder-pubsub-pub")
	if err != nil {
		return nil, err
	}
	defer func() {
		if retErr != nil {
			for _, c := range publishPool {
				c.Close()
			}
		}
	}()
	p.publishPool = publishPool

	subscribePool, err := newConnPool(ns, opts, handlers, opts.SubscribeConns, "coder-pubsub-sub")
	if err != nil {
		return nil, err
	}
	defer func() {
		if retErr != nil {
			for _, c := range subscribePool {
				c.Close()
			}
		}
	}()
	p.subscribePool = subscribePool
	// All owned connections dialed successfully above.
	p.metrics.markConnected(len(publishPool) + len(subscribePool))

	if p.clustered {
		ca := ns.ClusterAddr()
		if ca == nil {
			return nil, xerrors.New("no cluster address")
		}
		// sec checks, just to be sure
		if ca.Port < 0 || ca.Port > 65535 {
			return nil, xerrors.Errorf("invalid cluster port: %d", ca.Port)
		}
		//nolint:gosec // range checked above so conversion is safe.
		opts.PeerFetcher.SetSelfNATSPort(int32(ca.Port))
		go p.runPeerRefresh()
	}
	go func() {
		<-p.ctx.Done()
		_ = p.Close()
	}()

	return p, nil
}

func newConnPool(ns *natsserver.Server, opts Options, handlers connHandlers, count int, clientName string) ([]conn, error) {
	if count <= 0 {
		count = 1
	}
	pool := make([]conn, 0, count)
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
		p.metrics.recordPublishFailure()
		return errClosed
	}

	if err := pickConn(p.publishPool, event).Publish(event, message); err != nil {
		p.metrics.recordPublishFailure()
		return xerrors.Errorf("publish: %w", err)
	}
	p.metrics.recordPublishSuccess(len(message))
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
	return p.subscribeQueue(event, pubsub.NewMsgQueue(context.Background(), listener, nil))
}

// SubscribeWithErr subscribes a ListenerWithErr to the given event
// name. The listener also receives error deliveries such as
// pubsub.ErrDroppedMessages. Multiple local subscribers on the same
// event share a single underlying *natsgo.Subscription with
// per-listener bounded inboxes so a slow listener cannot block its
// peers.
func (p *Pubsub) SubscribeWithErr(event string, listener pubsub.ListenerWithErr) (cancel func(), err error) {
	return p.subscribeQueue(event, pubsub.NewMsgQueue(context.Background(), nil, listener))
}

// subscribeQueue subscribes the given MsgQueue for the given event.
func (p *Pubsub) subscribeQueue(event string, newQ *pubsub.MsgQueue) (cancel func(), err error) {
	defer func() {
		if err != nil {
			// If we hit an error, close the queue so we don't leak its goroutine.
			newQ.Close()
		}
	}()

	l, g := func() (*localSub, *groupSub) {
		p.mu.Lock()
		defer p.mu.Unlock()

		if p.ctx.Err() != nil {
			return nil, erroredGroupSub(p.metrics, errClosed)
		}

		var (
			gSub *groupSub
			ok   bool
		)
		gSub, ok = p.subscriptions[event]
		if !ok {
			gSub = &groupSub{
				metrics:   p.metrics,
				event:     event,
				localSubs: make(map[*localSub]struct{}),
				sub: &subGetter{
					subscribeDone: make(chan struct{}),
				},
			}
			go p.subscribeGroup(gSub)
			p.subscriptions[event] = gSub
			p.metrics.addEvent()
		}
		lSub := &localSub{
			event: event,
			queue: newQ,
		}
		gSub.mu.Lock()
		defer gSub.mu.Unlock()
		gSub.localSubs[lSub] = struct{}{}
		return lSub, gSub
	}()

	if _, err := g.sub.get(); err != nil {
		p.metrics.recordSubscribeFailure()
		// A failed subscribe was never counted (we increment only on
		// success below), so there is nothing to undo here.
		return nil, err
	}
	p.metrics.recordSubscribeSuccess()
	// Count the subscriber once the NATS subscription is established. The
	// matching decrement is in closeLocalSubFunc when the localSub is
	// removed. A mid-subscribe Close may decrement without a matching
	// increment, but the gauge is irrelevant once we are shutting down.
	p.metrics.addSubscriber()
	return p.closeLocalSubFunc(l, g), nil
}

// signalSubscribersDroppedForConn signals local subscribers assigned to conn.
func (p *Pubsub) signalSubscribersDroppedForConn(c conn) {
	if c == nil || len(p.subscribePool) == 0 {
		return
	}

	p.mu.Lock()
	subs := make([]*localSub, 0)
	for event, nsub := range p.subscriptions {
		if pickConn(p.subscribePool, event) != c {
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
	var nsub *groupSub
	for _, candidate := range p.subscriptions {
		if s, _ := candidate.sub.get(); s == sub {
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
func (p *Pubsub) handleSlowSubscriber(nsub *groupSub) {
	sub, err := nsub.sub.get()
	if err != nil {
		return
	}
	nsub.dropMu.Lock()
	dropped, err := sub.Dropped()
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
		// Report disconnected immediately. The owned connections are
		// closed below without firing the disconnect handler, so nothing
		// else resets the gauge during shutdown.
		p.metrics.markClosed()
		p.mu.Lock()
		p.logger.Debug(p.ctx, "closing pubsub")
		// Cancel while holding p.mu so subscriber state cleanup below
		// observes the canceled context.
		p.cancel()
		var closeFuncs []func()
		for _, g := range p.subscriptions {
			// here we don't need to hold the ss.mu lock because we are not mutating anything and holding the p.mu
			// blocks any new subscriptions.
			for l := range g.localSubs {
				closeFuncs = append(closeFuncs, p.closeLocalSubFunc(l, g))
			}
		}
		p.mu.Unlock()

		for _, f := range closeFuncs {
			f()
		}
		p.logger.Debug(p.ctx, "closed all local subscriptions")
		// Wait for any outstanding unsubscribe routines, kicked off above or before the Close().
		p.unsubscribeRoutines.Wait()
		p.logger.Debug(p.ctx, "unsubscribe routines done")

		for _, nc := range p.subscribePool {
			if nc != nil {
				nc.Close()
			}
		}
		p.logger.Debug(p.ctx, "subscribe pool connections closed")
		for _, nc := range p.publishPool {
			if nc != nil {
				nc.Close()
			}
		}
		p.logger.Debug(p.ctx, "publish pool connections closed")

		if p.Server != nil {
			p.Server.Shutdown()
			p.Server.WaitForShutdown()
			p.logger.Info(p.ctx, "nats server shut down")
		} else {
			p.logger.Debug(p.ctx, "nats server was never started")
		}
	})
	return nil
}

// closeLocalSubFunc returns a function that cancels local delivery without waiting for callbacks.
//
// It returns a func() rather than just closing directly because the PubSub interface wants a func() to cancel a
// subscription.
func (p *Pubsub) closeLocalSubFunc(l *localSub, g *groupSub) func() {
	return func() {
		p.mu.Lock()
		defer p.mu.Unlock()
		g.mu.Lock()
		defer g.mu.Unlock()

		logger := p.logger.With(slog.F("event", l.event))
		logger.Debug(context.Background(), "closing local sub")

		// This function must be idempotent because it is called either by the listener code, or by the pubsub itself
		// while closing. If we're already removed from the group then we must have already been closed.
		if _, exists := g.localSubs[l]; !exists {
			return
		}
		l.queue.Close()

		delete(g.localSubs, l)
		p.metrics.removeSubscriber()
		logger.Debug(context.Background(), "removed local sub from group", slog.F("group_size", len(g.localSubs)))
		if len(g.localSubs) > 0 {
			return // Not last one out
		}
		// Last localSub does the nats unsubscribe. Do this async so we don't hold the pubsub lock too long. Nothing is
		// left listening, so no rush.
		p.unsubscribeRoutines.Add(1)
		go func() {
			defer p.unsubscribeRoutines.Done()
			p.unsubscribeGroup(g)
		}()
		if pSub, ok := p.subscriptions[l.event]; ok && g == pSub {
			delete(p.subscriptions, l.event)
			p.metrics.removeEvent()
		}
	}
}

func (p *Pubsub) subscribeGroup(g *groupSub) {
	defer func() {
		if g.sub.err != nil {
			// failed to subscribe. Kick this out of the pubsub map of subscriptions, so that we don't permanently
			// fail to subscribe to this event. The subscribe that kicked this off as well as any concurrent ones will
			// see an error.
			p.mu.Lock()
			defer p.mu.Unlock()
			if psub := p.subscriptions[g.event]; psub == g {
				delete(p.subscriptions, g.event)
				p.metrics.removeEvent()
			}
		}
		close(g.sub.subscribeDone)
	}()
	logger := p.logger.With(slog.F("event", g.event))
	logger.Debug(context.Background(), "subscribing on nats")
	subConn := pickConn(p.subscribePool, g.event)
	natsSubscription, err := subConn.Subscribe(g.event, g.handleMessage)
	if err != nil {
		g.sub.err = xerrors.Errorf("subscribe: %w", err)
		return
	}
	g.sub.sub = natsSubscription
	defer func() {
		if g.sub.err != nil {
			unsubErr := natsSubscription.Unsubscribe()
			// best effort, just log if it fails
			if unsubErr != nil {
				// nolint: gocritic // false positive because we log two errors
				logger.Error(p.ctx, "failed to unsubscribe after error subscribing",
					slog.Error(unsubErr), slog.F("previous_error", g.sub.err))
			}
		}
	}()

	// Flush the SUB to the server so a publish issued immediately
	// after Subscribe returns cannot race ahead of registration.
	if err := subConn.Flush(); err != nil {
		g.sub.err = xerrors.Errorf("flush subscribe: %w", err)
		return
	}
	limits := defaultPendingLimits(p.opts.PendingLimits)
	if err := natsSubscription.SetPendingLimits(limits.Msgs, limits.Bytes); err != nil {
		g.sub.err = xerrors.Errorf("set pending limits: %w", err)
		return
	}
}

func (p *Pubsub) unsubscribeGroup(g *groupSub) {
	logger := p.logger.With(slog.F("event", g.event))
	logger.Debug(context.Background(), "unsubscribing group subscription from nats")
	// wait for any pending Subscribe to complete before we attempt to unsubscribe
	sub, err := g.sub.get()
	if err != nil {
		// subscribe failed, nothing else to do.
		return
	}
	if err = sub.Unsubscribe(); err != nil {
		logger.Error(context.Background(), "failed to unsubscribe from pubsub", slog.Error(err))
	}
	// TODO: should we retry?
}

// pickConn returns the connection assigned to subject. Selection uses
// a stable FNV-1a hash so same-subject traffic always targets the same
// connection within a process; pools are immutable after construction
// so the lookup is lock-free.
func pickConn(pool []conn, subject string) conn {
	if len(pool) == 1 {
		return pool[0]
	}
	h := fnv.New32a()
	_, _ = h.Write([]byte(subject))
	n := uint32(len(pool)) //nolint:gosec // pool size bounded by Options.{Publish,Subscribe}Conns
	return pool[h.Sum32()%n]
}

// erroredGroupSub returns a groupSub that shows an error rather than an active subscription.
func erroredGroupSub(m *metrics, err error) *groupSub {
	c := make(chan struct{})
	close(c)
	return &groupSub{
		metrics: m,
		sub: &subGetter{
			subscribeDone: c,
			err:           err,
		},
	}
}

// handleMessage handles messages for the shared subscription. Each
// enqueue is non-blocking and does not call user code, so one slow
// listener cannot stall the NATS delivery goroutine.
//
// Zero-copy fan-out: the same msg.Data slice is delivered to every
// local listener without cloning. Listeners on a coalesced subject MUST
// treat the delivered bytes as immutable.
func (g *groupSub) handleMessage(msg *natsgo.Msg) {
	g.metrics.recordReceived(msg.Data)
	g.mu.Lock()
	defer g.mu.Unlock()
	for l := range g.localSubs {
		l.queue.Enqueue(msg.Data)
	}
}

// subGetter allows callers to asynchronously wait for the subscription to complete or error by calling the get()
// method. Routines other than the one that actually starts the natsgo.Subscription should never access sub directly.
type subGetter struct {
	// closed when the initial subscribe completes
	subscribeDone chan struct{}
	// either sub or err are non-nil after subscribeDone is closed
	sub *natsgo.Subscription
	err error
}

func (s *subGetter) get() (*natsgo.Subscription, error) {
	<-s.subscribeDone
	return s.sub, s.err
}

// signalDrop pushes onto dropSignal without blocking. Multiple drops
// between dispatcher dequeues coalesce into a single pending signal, so
// the listener observes one ErrDroppedMessages per drop wave.
func (l *localSub) signalDrop() {
	l.queue.Dropped()
}
