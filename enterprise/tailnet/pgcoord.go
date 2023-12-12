package tailnet

import (
	"context"
	"database/sql"
	"net"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/coder/coder/v2/tailnet/proto"

	"github.com/cenkalti/backoff/v4"
	"github.com/google/uuid"
	"golang.org/x/xerrors"
	gProto "google.golang.org/protobuf/proto"

	"cdr.dev/slog"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/database/pubsub"
	"github.com/coder/coder/v2/coderd/rbac"
	agpl "github.com/coder/coder/v2/tailnet"
)

const (
	EventHeartbeats    = "tailnet_coordinator_heartbeat"
	eventPeerUpdate    = "tailnet_peer_update"
	eventTunnelUpdate  = "tailnet_tunnel_update"
	HeartbeatPeriod    = time.Second * 2
	MissedHeartbeats   = 3
	numQuerierWorkers  = 10
	numBinderWorkers   = 10
	numTunnelerWorkers = 10
	dbMaxBackoff       = 10 * time.Second
	cleanupPeriod      = time.Hour
)

// pgCoord is a postgres-backed coordinator
//
//	                 ┌──────────┐
//	    ┌────────────► tunneler ├──────────┐
//	    │            └──────────┘          │
//	    │                                  │
//	┌────────┐       ┌────────┐        ┌───▼───┐
//	│ connIO ├───────► binder ├────────► store │
//	└───▲────┘       │        │        │       │
//	    │            └────────┘ ┌──────┤       │
//	    │                       │      └───────┘
//	    │                       │
//	    │            ┌──────────▼┐     ┌────────┐
//	    │            │           │     │        │
//	    └────────────┤ querier   ◄─────┤ pubsub │
//	                 │           │     │        │
//	                 └───────────┘     └────────┘
//
// each incoming connection (websocket) from a peer is wrapped in a connIO which handles reading & writing
// from it.  Node updates from a connIO are sent to the binder, which writes them to the database.Store. Tunnel
// updates from a connIO are sent to the tunneler, which writes them to the database.Store. The querier is responsible
// for querying the store for the nodes the connection needs.  The querier receives pubsub notifications about changes,
// which trigger queries for the latest state.
//
// The querier also sends the coordinator's heartbeat, and monitors the heartbeats of other coordinators.  When
// heartbeats cease for a coordinator, it stops using any nodes discovered from that coordinator and pushes an update
// to affected connIOs.
//
// This package uses the term "binding" to mean the act of registering an association between some connection
// and a *proto.Node.  It uses the term "mapping" to mean the act of determining the nodes that the connection
// needs to receive (i.e. the nodes of all peers it shares a tunnel with).
type pgCoord struct {
	ctx    context.Context
	logger slog.Logger
	pubsub pubsub.Pubsub
	store  database.Store

	bindings         chan binding
	newConnections   chan *connIO
	closeConnections chan *connIO
	tunnelerCh       chan tunnel
	id               uuid.UUID

	cancel    context.CancelFunc
	closeOnce sync.Once
	closed    chan struct{}

	binder   *binder
	tunneler *tunneler
	querier  *querier
}

var pgCoordSubject = rbac.Subject{
	ID: uuid.Nil.String(),
	Roles: rbac.Roles([]rbac.Role{
		{
			Name:        "tailnetcoordinator",
			DisplayName: "Tailnet Coordinator",
			Site: rbac.Permissions(map[string][]rbac.Action{
				rbac.ResourceTailnetCoordinator.Type: {rbac.WildcardSymbol},
			}),
			Org:  map[string][]rbac.Permission{},
			User: []rbac.Permission{},
		},
	}),
	Scope: rbac.ScopeAll,
}.WithCachedASTValue()

// NewPGCoord creates a high-availability coordinator that stores state in the PostgreSQL database and
// receives notifications of updates via the pubsub.
func NewPGCoord(ctx context.Context, logger slog.Logger, ps pubsub.Pubsub, store database.Store) (agpl.Coordinator, error) {
	return newPGCoordInternal(ctx, logger, ps, store)
}

func newPGCoordInternal(
	ctx context.Context, logger slog.Logger, ps pubsub.Pubsub, store database.Store,
) (
	*pgCoord, error,
) {
	ctx, cancel := context.WithCancel(dbauthz.As(ctx, pgCoordSubject))
	id := uuid.New()
	logger = logger.Named("pgcoord").With(slog.F("coordinator_id", id))
	bCh := make(chan binding)
	// used for opening connections
	cCh := make(chan *connIO)
	// used for closing connections
	ccCh := make(chan *connIO)
	// for communicating subscriptions with the tunneler
	sCh := make(chan tunnel)
	// signals when first heartbeat has been sent, so it's safe to start binding.
	fHB := make(chan struct{})

	c := &pgCoord{
		ctx:              ctx,
		cancel:           cancel,
		logger:           logger,
		pubsub:           ps,
		store:            store,
		binder:           newBinder(ctx, logger, id, store, bCh, fHB),
		bindings:         bCh,
		newConnections:   cCh,
		closeConnections: ccCh,
		tunneler:         newTunneler(ctx, logger, id, store, sCh, fHB),
		tunnelerCh:       sCh,
		id:               id,
		querier:          newQuerier(ctx, logger, id, ps, store, id, cCh, ccCh, numQuerierWorkers, fHB),
		closed:           make(chan struct{}),
	}
	logger.Info(ctx, "starting coordinator")
	return c, nil
}

// NewPGCoordV2 creates a high-availability coordinator that stores state in the PostgreSQL database and
// receives notifications of updates via the pubsub.
func NewPGCoordV2(ctx context.Context, logger slog.Logger, ps pubsub.Pubsub, store database.Store) (agpl.CoordinatorV2, error) {
	return newPGCoordInternal(ctx, logger, ps, store)
}

func (c *pgCoord) ServeMultiAgent(id uuid.UUID) agpl.MultiAgentConn {
	return agpl.ServeMultiAgent(c, c.logger, id)
}

func (c *pgCoord) Node(id uuid.UUID) *agpl.Node {
	// We're going to directly query the database, since we would only have the mapping stored locally if we had
	// a tunnel peer connected, which is not always the case.
	peers, err := c.store.GetTailnetPeers(c.ctx, id)
	if err != nil {
		c.logger.Error(c.ctx, "failed to query peers", slog.Error(err))
		return nil
	}
	mappings := make([]mapping, 0, len(peers))
	for _, peer := range peers {
		pNode := new(proto.Node)
		err := gProto.Unmarshal(peer.Node, pNode)
		if err != nil {
			c.logger.Critical(c.ctx, "failed to unmarshal node", slog.F("bytes", peer.Node), slog.Error(err))
			return nil
		}
		mappings = append(mappings, mapping{
			peer:        peer.ID,
			coordinator: peer.CoordinatorID,
			updatedAt:   peer.UpdatedAt,
			node:        pNode,
		})
	}
	mappings = c.querier.heartbeats.filter(mappings)
	var bestT time.Time
	var bestN *proto.Node
	for _, m := range mappings {
		if m.updatedAt.After(bestT) {
			bestN = m.node
			bestT = m.updatedAt
		}
	}
	node, err := agpl.ProtoToNode(bestN)
	if err != nil {
		c.logger.Critical(c.ctx, "failed to convert node", slog.F("node", bestN), slog.Error(err))
		return nil
	}
	return node
}

func (c *pgCoord) ServeClient(conn net.Conn, id uuid.UUID, agent uuid.UUID) error {
	return agpl.ServeClientV1(c.ctx, c.logger, c, conn, id, agent)
}

func (c *pgCoord) ServeAgent(conn net.Conn, id uuid.UUID, name string) error {
	return agpl.ServeAgentV1(c.ctx, c.logger, c, conn, id, name)
}

func (c *pgCoord) Close() error {
	c.logger.Info(c.ctx, "closing coordinator")
	c.cancel()
	c.closeOnce.Do(func() { close(c.closed) })
	return nil
}

func (c *pgCoord) Coordinate(
	ctx context.Context, id uuid.UUID, name string, a agpl.TunnelAuth,
) (
	chan<- *proto.CoordinateRequest, <-chan *proto.CoordinateResponse,
) {
	logger := c.logger.With(slog.F("peer_id", id))
	reqs := make(chan *proto.CoordinateRequest, agpl.RequestBufferSize)
	resps := make(chan *proto.CoordinateResponse, agpl.ResponseBufferSize)
	cIO := newConnIO(c.ctx, ctx, logger, c.bindings, c.tunnelerCh, reqs, resps, id, name, a)
	err := agpl.SendCtx(c.ctx, c.newConnections, cIO)
	if err != nil {
		// this can only happen if the context is canceled, no need to log
		return reqs, resps
	}
	go func() {
		<-cIO.Done()
		_ = agpl.SendCtx(c.ctx, c.closeConnections, cIO)
	}()

	return reqs, resps
}

type tKey struct {
	src uuid.UUID
	dst uuid.UUID
}

type tunnel struct {
	tKey
	// whether the subscription should be active. if true, the subscription is
	// added. if false, the subscription is removed.
	active bool
}

type tunneler struct {
	ctx           context.Context
	logger        slog.Logger
	coordinatorID uuid.UUID
	store         database.Store
	updates       <-chan tunnel

	mu     sync.Mutex
	latest map[uuid.UUID]map[uuid.UUID]tunnel
	workQ  *workQ[tKey]
}

func newTunneler(ctx context.Context,
	logger slog.Logger,
	id uuid.UUID,
	store database.Store,
	updates <-chan tunnel,
	startWorkers <-chan struct{},
) *tunneler {
	s := &tunneler{
		ctx:           ctx,
		logger:        logger,
		coordinatorID: id,
		store:         store,
		updates:       updates,
		latest:        make(map[uuid.UUID]map[uuid.UUID]tunnel),
		workQ:         newWorkQ[tKey](ctx),
	}
	go s.handle()
	go func() {
		<-startWorkers
		for i := 0; i < numTunnelerWorkers; i++ {
			go s.worker()
		}
	}()
	return s
}

func (t *tunneler) handle() {
	for {
		select {
		case <-t.ctx.Done():
			t.logger.Debug(t.ctx, "tunneler exiting", slog.Error(t.ctx.Err()))
			return
		case tun := <-t.updates:
			t.cache(tun)
			t.workQ.enqueue(tun.tKey)
		}
	}
}

func (t *tunneler) worker() {
	eb := backoff.NewExponentialBackOff()
	eb.MaxElapsedTime = 0 // retry indefinitely
	eb.MaxInterval = dbMaxBackoff
	bkoff := backoff.WithContext(eb, t.ctx)
	for {
		tk, err := t.workQ.acquire()
		if err != nil {
			// context expired
			return
		}
		err = backoff.Retry(func() error {
			tun := t.retrieve(tk)
			return t.writeOne(tun)
		}, bkoff)
		if err != nil {
			bkoff.Reset()
		}
		t.workQ.done(tk)
	}
}

func (t *tunneler) cache(update tunnel) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if update.active {
		if _, ok := t.latest[update.src]; !ok {
			t.latest[update.src] = map[uuid.UUID]tunnel{}
		}
		t.latest[update.src][update.dst] = update
	} else {
		// If inactive and dst is nil, it means clean up all tunnels.
		if update.dst == uuid.Nil {
			delete(t.latest, update.src)
		} else {
			delete(t.latest[update.src], update.dst)
			// clean up the tunnel map if all the tunnels are gone.
			if len(t.latest[update.src]) == 0 {
				delete(t.latest, update.src)
			}
		}
	}
}

// retrieveBinding gets the latest tunnel for a key.
func (t *tunneler) retrieve(k tKey) tunnel {
	t.mu.Lock()
	defer t.mu.Unlock()
	dstMap, ok := t.latest[k.src]
	if !ok {
		return tunnel{
			tKey:   k,
			active: false,
		}
	}

	tun, ok := dstMap[k.dst]
	if !ok {
		return tunnel{
			tKey:   k,
			active: false,
		}
	}

	return tun
}

func (t *tunneler) writeOne(tun tunnel) error {
	var err error
	switch {
	case tun.dst == uuid.Nil:
		err = t.store.DeleteAllTailnetTunnels(t.ctx, database.DeleteAllTailnetTunnelsParams{
			SrcID:         tun.src,
			CoordinatorID: t.coordinatorID,
		})
		t.logger.Debug(t.ctx, "deleted all tunnels",
			slog.F("src_id", tun.src),
			slog.Error(err),
		)
	case tun.active:
		_, err = t.store.UpsertTailnetTunnel(t.ctx, database.UpsertTailnetTunnelParams{
			CoordinatorID: t.coordinatorID,
			SrcID:         tun.src,
			DstID:         tun.dst,
		})
		t.logger.Debug(t.ctx, "upserted tunnel",
			slog.F("src_id", tun.src),
			slog.F("dst_id", tun.dst),
			slog.Error(err),
		)
	case !tun.active:
		_, err = t.store.DeleteTailnetTunnel(t.ctx, database.DeleteTailnetTunnelParams{
			CoordinatorID: t.coordinatorID,
			SrcID:         tun.src,
			DstID:         tun.dst,
		})
		t.logger.Debug(t.ctx, "deleted tunnel",
			slog.F("src_id", tun.src),
			slog.F("dst_id", tun.dst),
			slog.Error(err),
		)
		// writeOne should be idempotent
		if xerrors.Is(err, sql.ErrNoRows) {
			err = nil
		}
	default:
		panic("unreachable")
	}
	if err != nil && !database.IsQueryCanceledError(err) {
		t.logger.Error(t.ctx, "write tunnel to database",
			slog.F("src_id", tun.src),
			slog.F("dst_id", tun.dst),
			slog.F("active", tun.active),
			slog.Error(err))
	}
	return err
}

// bKey, or "binding key" identifies a peer in a binding
type bKey uuid.UUID

// binding represents an association between a peer and a Node.
type binding struct {
	bKey
	node *proto.Node
	kind proto.CoordinateResponse_PeerUpdate_Kind
}

// binder reads node bindings from the channel and writes them to the database.  It handles retries with a backoff.
type binder struct {
	ctx           context.Context
	logger        slog.Logger
	coordinatorID uuid.UUID
	store         database.Store
	bindings      <-chan binding

	mu     sync.Mutex
	latest map[bKey]binding
	workQ  *workQ[bKey]
}

func newBinder(ctx context.Context,
	logger slog.Logger,
	id uuid.UUID,
	store database.Store,
	bindings <-chan binding,
	startWorkers <-chan struct{},
) *binder {
	b := &binder{
		ctx:           ctx,
		logger:        logger,
		coordinatorID: id,
		store:         store,
		bindings:      bindings,
		latest:        make(map[bKey]binding),
		workQ:         newWorkQ[bKey](ctx),
	}
	go b.handleBindings()
	go func() {
		<-startWorkers
		for i := 0; i < numBinderWorkers; i++ {
			go b.worker()
		}
	}()
	return b
}

func (b *binder) handleBindings() {
	for {
		select {
		case <-b.ctx.Done():
			b.logger.Debug(b.ctx, "binder exiting", slog.Error(b.ctx.Err()))
			return
		case bnd := <-b.bindings:
			b.storeBinding(bnd)
			b.workQ.enqueue(bnd.bKey)
		}
	}
}

func (b *binder) worker() {
	eb := backoff.NewExponentialBackOff()
	eb.MaxElapsedTime = 0 // retry indefinitely
	eb.MaxInterval = dbMaxBackoff
	bkoff := backoff.WithContext(eb, b.ctx)
	for {
		bk, err := b.workQ.acquire()
		if err != nil {
			// context expired
			return
		}
		err = backoff.Retry(func() error {
			bnd := b.retrieveBinding(bk)
			return b.writeOne(bnd)
		}, bkoff)
		if err != nil {
			bkoff.Reset()
		}
		b.workQ.done(bk)
	}
}

func (b *binder) writeOne(bnd binding) error {
	var err error
	if bnd.kind == proto.CoordinateResponse_PeerUpdate_DISCONNECTED {
		_, err = b.store.DeleteTailnetPeer(b.ctx, database.DeleteTailnetPeerParams{
			ID:            uuid.UUID(bnd.bKey),
			CoordinatorID: b.coordinatorID,
		})
		// writeOne is idempotent
		if xerrors.Is(err, sql.ErrNoRows) {
			err = nil
		}
	} else {
		var nodeRaw []byte
		nodeRaw, err = gProto.Marshal(bnd.node)
		if err != nil {
			// this is very bad news, but it should never happen because the node was Unmarshalled or converted by this
			// process earlier.
			b.logger.Critical(b.ctx, "failed to marshal node", slog.Error(err))
			return err
		}
		status := database.TailnetStatusOk
		if bnd.kind == proto.CoordinateResponse_PeerUpdate_LOST {
			status = database.TailnetStatusLost
		}
		_, err = b.store.UpsertTailnetPeer(b.ctx, database.UpsertTailnetPeerParams{
			ID:            uuid.UUID(bnd.bKey),
			CoordinatorID: b.coordinatorID,
			Node:          nodeRaw,
			Status:        status,
		})
	}

	if err != nil && !database.IsQueryCanceledError(err) {
		b.logger.Error(b.ctx, "failed to write binding to database",
			slog.F("binding_id", bnd.bKey),
			slog.F("node", bnd.node),
			slog.Error(err))
	}
	return err
}

// storeBinding stores the latest binding, where we interpret kind == DISCONNECTED as removing the binding. This keeps the map
// from growing without bound.
func (b *binder) storeBinding(bnd binding) {
	b.mu.Lock()
	defer b.mu.Unlock()

	switch bnd.kind {
	case proto.CoordinateResponse_PeerUpdate_NODE:
		b.latest[bnd.bKey] = bnd
	case proto.CoordinateResponse_PeerUpdate_DISCONNECTED:
		delete(b.latest, bnd.bKey)
	case proto.CoordinateResponse_PeerUpdate_LOST:
		// we need to coalesce with the previously stored node, since it must
		// be non-nil in the database
		old, ok := b.latest[bnd.bKey]
		if !ok {
			// lost before we ever got a node update.  No action
			return
		}
		bnd.node = old.node
		b.latest[bnd.bKey] = bnd
	}
}

// retrieveBinding gets the latest binding for a key.
func (b *binder) retrieveBinding(bk bKey) binding {
	b.mu.Lock()
	defer b.mu.Unlock()
	bnd, ok := b.latest[bk]
	if !ok {
		bnd = binding{
			bKey: bk,
			node: nil,
			kind: proto.CoordinateResponse_PeerUpdate_DISCONNECTED,
		}
	}
	return bnd
}

// mapper tracks data sent to a peer, and sends updates based on changes read from the database.
type mapper struct {
	ctx    context.Context
	logger slog.Logger

	// reads from this channel trigger recomputing the set of mappings to send, and sending any updates. It is used when
	// coordinators are added or removed
	update chan struct{}

	mappings chan []mapping

	c *connIO

	// latest is the most recent, unfiltered snapshot of the mappings we know about
	latest []mapping
	// sent is the state of mappings we have actually enqueued; used to compute diffs for updates.
	sent map[uuid.UUID]mapping

	// called to filter mappings to healthy coordinators
	heartbeats *heartbeats
}

func newMapper(c *connIO, logger slog.Logger, h *heartbeats) *mapper {
	logger = logger.With(
		slog.F("peer_id", c.UniqueID()),
	)
	m := &mapper{
		ctx:        c.peerCtx, // mapper has same lifetime as the underlying connection it serves
		logger:     logger,
		c:          c,
		update:     make(chan struct{}),
		mappings:   make(chan []mapping),
		heartbeats: h,
		sent:       make(map[uuid.UUID]mapping),
	}
	go m.run()
	return m
}

func (m *mapper) run() {
	for {
		var best map[uuid.UUID]mapping
		select {
		case <-m.ctx.Done():
			return
		case mappings := <-m.mappings:
			m.logger.Debug(m.ctx, "got new mappings")
			m.latest = mappings
			best = m.bestMappings(mappings)
		case <-m.update:
			m.logger.Debug(m.ctx, "triggered update")
			best = m.bestMappings(m.latest)
		}
		update := m.bestToUpdate(best)
		if update == nil {
			m.logger.Debug(m.ctx, "skipping nil node update")
			continue
		}
		if err := m.c.Enqueue(update); err != nil {
			m.logger.Error(m.ctx, "failed to enqueue node update", slog.Error(err))
		}
	}
}

// bestMappings takes a set of mappings and resolves the best set of nodes.  We may get several mappings for a
// particular connection, from different coordinators in the distributed system.  Furthermore, some coordinators
// might be considered invalid on account of missing heartbeats.  We take the most recent mapping from a valid
// coordinator as the "best" mapping.
func (m *mapper) bestMappings(mappings []mapping) map[uuid.UUID]mapping {
	mappings = m.heartbeats.filter(mappings)
	best := make(map[uuid.UUID]mapping, len(mappings))
	for _, mpng := range mappings {
		bestM, ok := best[mpng.peer]
		switch {
		case !ok:
			// no current best
			best[mpng.peer] = mpng

		// NODE always beats LOST mapping, since the LOST could be from a coordinator that's
		// slow updating the DB, and the peer has reconnected to a different coordinator and
		// given a NODE mapping.
		case bestM.kind == proto.CoordinateResponse_PeerUpdate_LOST && mpng.kind == proto.CoordinateResponse_PeerUpdate_NODE:
			best[mpng.peer] = mpng
		case mpng.updatedAt.After(bestM.updatedAt) && mpng.kind == proto.CoordinateResponse_PeerUpdate_NODE:
			// newer, and it's a NODE update.
			best[mpng.peer] = mpng
		}
	}
	return best
}

func (m *mapper) bestToUpdate(best map[uuid.UUID]mapping) *proto.CoordinateResponse {
	resp := new(proto.CoordinateResponse)

	for k, mpng := range best {
		var reason string
		sm, ok := m.sent[k]
		switch {
		case !ok && mpng.kind == proto.CoordinateResponse_PeerUpdate_LOST:
			// we don't need to send a "lost" update if we've never sent an update about this peer
			continue
		case !ok && mpng.kind == proto.CoordinateResponse_PeerUpdate_NODE:
			reason = "new"
		case ok && sm.kind == proto.CoordinateResponse_PeerUpdate_LOST && mpng.kind == proto.CoordinateResponse_PeerUpdate_LOST:
			// was lost and remains lost, no update needed
			continue
		case ok && sm.kind == proto.CoordinateResponse_PeerUpdate_LOST && mpng.kind == proto.CoordinateResponse_PeerUpdate_NODE:
			reason = "found"
		case ok && sm.kind == proto.CoordinateResponse_PeerUpdate_NODE && mpng.kind == proto.CoordinateResponse_PeerUpdate_LOST:
			reason = "lost"
		case ok && sm.kind == proto.CoordinateResponse_PeerUpdate_NODE && mpng.kind == proto.CoordinateResponse_PeerUpdate_NODE:
			eq, err := sm.node.Equal(mpng.node)
			if err != nil {
				m.logger.Critical(m.ctx, "failed to compare nodes", slog.F("old", sm.node), slog.F("new", mpng.node))
				continue
			}
			if eq {
				continue
			}
			reason = "update"
		}
		resp.PeerUpdates = append(resp.PeerUpdates, &proto.CoordinateResponse_PeerUpdate{
			Uuid:   agpl.UUIDToByteSlice(k),
			Node:   mpng.node,
			Kind:   mpng.kind,
			Reason: reason,
		})
		m.sent[k] = mpng
	}

	for k := range m.sent {
		if _, ok := best[k]; !ok {
			resp.PeerUpdates = append(resp.PeerUpdates, &proto.CoordinateResponse_PeerUpdate{
				Uuid:   agpl.UUIDToByteSlice(k),
				Kind:   proto.CoordinateResponse_PeerUpdate_DISCONNECTED,
				Reason: "disconnected",
			})
			delete(m.sent, k)
		}
	}

	if len(resp.PeerUpdates) == 0 {
		return nil
	}
	return resp
}

// querier is responsible for monitoring pubsub notifications and querying the database for the
// mappings that all connected peers need.  It also checks heartbeats and withdraws mappings from
// coordinators that have failed heartbeats.
//
// There are two kinds of pubsub notifications it listens for and responds to.
//
//  1. Tunnel updates --- a tunnel was added or removed.  In this case we need
//     to recompute the mappings for peers on both sides of the tunnel.
//  2. Peer updates --- a peer got a new binding.  When a peer gets a new
//     binding, we need to update all the _other_ peers it shares a tunnel with.
//     However, we don't keep tunnels in memory (to avoid the
//     complexity of synchronizing with the database), so we first have to query
//     the database to learn the tunnel peers, then schedule an update on each
//     one.
type querier struct {
	ctx           context.Context
	logger        slog.Logger
	coordinatorID uuid.UUID
	pubsub        pubsub.Pubsub
	store         database.Store

	newConnections   chan *connIO
	closeConnections chan *connIO

	workQ *workQ[querierWorkKey]

	heartbeats *heartbeats
	updates    <-chan hbUpdate

	mu      sync.Mutex
	mappers map[mKey]*mapper
	healthy bool
}

func newQuerier(ctx context.Context,
	logger slog.Logger,
	coordinatorID uuid.UUID,
	ps pubsub.Pubsub,
	store database.Store,
	self uuid.UUID,
	newConnections chan *connIO,
	closeConnections chan *connIO,
	numWorkers int,
	firstHeartbeat chan struct{},
) *querier {
	updates := make(chan hbUpdate)
	q := &querier{
		ctx:              ctx,
		logger:           logger.Named("querier"),
		coordinatorID:    coordinatorID,
		pubsub:           ps,
		store:            store,
		newConnections:   newConnections,
		closeConnections: closeConnections,
		workQ:            newWorkQ[querierWorkKey](ctx),
		heartbeats:       newHeartbeats(ctx, logger, ps, store, self, updates, firstHeartbeat),
		mappers:          make(map[mKey]*mapper),
		updates:          updates,
		healthy:          true, // assume we start healthy
	}
	q.subscribe()

	go func() {
		<-firstHeartbeat
		go q.handleIncoming()
		for i := 0; i < numWorkers; i++ {
			go q.worker()
		}
		go q.handleUpdates()
	}()
	return q
}

func (q *querier) handleIncoming() {
	for {
		select {
		case <-q.ctx.Done():
			return

		case c := <-q.newConnections:
			q.newConn(c)

		case c := <-q.closeConnections:
			q.cleanupConn(c)
		}
	}
}

func (q *querier) newConn(c *connIO) {
	q.mu.Lock()
	defer q.mu.Unlock()
	if !q.healthy {
		err := c.Close()
		q.logger.Info(q.ctx, "closed incoming connection while unhealthy",
			slog.Error(err),
			slog.F("peer_id", c.UniqueID()),
		)
		return
	}
	mpr := newMapper(c, q.logger, q.heartbeats)
	mk := mKey(c.UniqueID())
	dup, ok := q.mappers[mk]
	if ok {
		// duplicate, overwrite and close the old one
		atomic.StoreInt64(&c.overwrites, dup.c.Overwrites()+1)
		err := dup.c.CoordinatorClose()
		if err != nil {
			q.logger.Error(q.ctx, "failed to close duplicate mapper", slog.F("peer_id", dup.c.UniqueID()), slog.Error(err))
		}
	}
	q.mappers[mk] = mpr
	q.workQ.enqueue(querierWorkKey{
		mappingQuery: mk,
	})
}

func (q *querier) cleanupConn(c *connIO) {
	logger := q.logger.With(slog.F("peer_id", c.UniqueID()))
	q.mu.Lock()
	defer q.mu.Unlock()

	mk := mKey(c.UniqueID())
	mpr, ok := q.mappers[mk]
	if !ok {
		return
	}
	if mpr.c != c {
		logger.Debug(q.ctx, "attempt to cleanup for duplicate connection, ignoring")
		return
	}
	err := c.CoordinatorClose()
	if err != nil {
		logger.Error(q.ctx, "failed to close connIO", slog.Error(err))
	}
	delete(q.mappers, mk)
	q.logger.Debug(q.ctx, "removed mapper")
}

func (q *querier) worker() {
	eb := backoff.NewExponentialBackOff()
	eb.MaxElapsedTime = 0 // retry indefinitely
	eb.MaxInterval = dbMaxBackoff
	bkoff := backoff.WithContext(eb, q.ctx)
	for {
		qk, err := q.workQ.acquire()
		if err != nil {
			// context expired
			return
		}
		err = backoff.Retry(func() error {
			return q.query(qk)
		}, bkoff)
		if err != nil {
			bkoff.Reset()
		}
		q.workQ.done(qk)
	}
}

func (q *querier) query(qk querierWorkKey) error {
	if uuid.UUID(qk.mappingQuery) != uuid.Nil {
		return q.mappingQuery(qk.mappingQuery)
	}
	if qk.peerUpdate != uuid.Nil {
		return q.peerUpdate(qk.peerUpdate)
	}
	q.logger.Critical(q.ctx, "bad querierWorkKey", slog.F("work_key", qk))
	return backoff.Permanent(xerrors.Errorf("bad querierWorkKey %v", qk))
}

// peerUpdate is work scheduled in response to a new peer->binding.  We need to find out all the
// other peers that share a tunnel with the indicated peer, and then schedule a mapping update on
// each, so that they can find out about the new binding.
func (q *querier) peerUpdate(peer uuid.UUID) error {
	logger := q.logger.With(slog.F("peer_id", peer))
	logger.Debug(q.ctx, "querying peers that share a tunnel")
	others, err := q.store.GetTailnetTunnelPeerIDs(q.ctx, peer)
	if err != nil && !xerrors.Is(err, sql.ErrNoRows) {
		return err
	}
	logger.Debug(q.ctx, "queried peers that share a tunnel", slog.F("num_peers", len(others)))
	for _, other := range others {
		logger.Debug(q.ctx, "got tunnel peer", slog.F("other_id", other.PeerID))
		q.workQ.enqueue(querierWorkKey{mappingQuery: mKey(other.PeerID)})
	}
	return nil
}

// mappingQuery queries the database for all the mappings that the given peer should know about,
// that is, all the peers that it shares a tunnel with and their current node mappings (if they
// exist).  It then sends the mapping snapshot to the corresponding mapper, where it will get
// transmitted to the peer.
func (q *querier) mappingQuery(peer mKey) error {
	logger := q.logger.With(slog.F("peer_id", uuid.UUID(peer)))
	logger.Debug(q.ctx, "querying mappings")
	bindings, err := q.store.GetTailnetTunnelPeerBindings(q.ctx, uuid.UUID(peer))
	logger.Debug(q.ctx, "queried mappings", slog.F("num_mappings", len(bindings)))
	if err != nil && !xerrors.Is(err, sql.ErrNoRows) {
		return err
	}
	mappings, err := q.bindingsToMappings(bindings)
	if err != nil {
		logger.Debug(q.ctx, "failed to convert mappings", slog.Error(err))
		return err
	}
	q.mu.Lock()
	mpr, ok := q.mappers[peer]
	q.mu.Unlock()
	if !ok {
		logger.Debug(q.ctx, "query for missing mapper")
		return nil
	}
	logger.Debug(q.ctx, "sending mappings", slog.F("mapping_len", len(mappings)))
	return agpl.SendCtx(q.ctx, mpr.mappings, mappings)
}

func (q *querier) bindingsToMappings(bindings []database.GetTailnetTunnelPeerBindingsRow) ([]mapping, error) {
	slog.Helper()
	mappings := make([]mapping, 0, len(bindings))
	for _, binding := range bindings {
		node := new(proto.Node)
		err := gProto.Unmarshal(binding.Node, node)
		if err != nil {
			q.logger.Error(q.ctx, "failed to unmarshal node", slog.Error(err))
			return nil, backoff.Permanent(err)
		}
		kind := proto.CoordinateResponse_PeerUpdate_NODE
		if binding.Status == database.TailnetStatusLost {
			kind = proto.CoordinateResponse_PeerUpdate_LOST
		}
		mappings = append(mappings, mapping{
			peer:        binding.PeerID,
			coordinator: binding.CoordinatorID,
			updatedAt:   binding.UpdatedAt,
			node:        node,
			kind:        kind,
		})
	}
	return mappings, nil
}

// subscribe starts our subscriptions to peer and tunnnel updates in a new goroutine, and returns once we are subscribed
// or the querier context is canceled.
func (q *querier) subscribe() {
	subscribed := make(chan struct{})
	go func() {
		defer close(subscribed)
		eb := backoff.NewExponentialBackOff()
		eb.MaxElapsedTime = 0 // retry indefinitely
		eb.MaxInterval = dbMaxBackoff
		bkoff := backoff.WithContext(eb, q.ctx)
		var cancelPeer context.CancelFunc
		err := backoff.Retry(func() error {
			cancelFn, err := q.pubsub.SubscribeWithErr(eventPeerUpdate, q.listenPeer)
			if err != nil {
				q.logger.Warn(q.ctx, "failed to subscribe to peer updates", slog.Error(err))
				return err
			}
			cancelPeer = cancelFn
			return nil
		}, bkoff)
		if err != nil {
			if q.ctx.Err() == nil {
				q.logger.Error(q.ctx, "code bug: retry failed before context canceled", slog.Error(err))
			}
			return
		}
		defer cancelPeer()
		bkoff.Reset()
		q.logger.Debug(q.ctx, "subscribed to peer updates")

		var cancelTunnel context.CancelFunc
		err = backoff.Retry(func() error {
			cancelFn, err := q.pubsub.SubscribeWithErr(eventTunnelUpdate, q.listenTunnel)
			if err != nil {
				q.logger.Warn(q.ctx, "failed to subscribe to tunnel updates", slog.Error(err))
				return err
			}
			cancelTunnel = cancelFn
			return nil
		}, bkoff)
		if err != nil {
			if q.ctx.Err() == nil {
				q.logger.Error(q.ctx, "code bug: retry failed before context canceled", slog.Error(err))
			}
			return
		}
		defer cancelTunnel()
		q.logger.Debug(q.ctx, "subscribed to tunnel updates")

		// unblock the outer function from returning
		subscribed <- struct{}{}

		// hold subscriptions open until context is canceled
		<-q.ctx.Done()
	}()
	<-subscribed
}

func (q *querier) listenPeer(_ context.Context, msg []byte, err error) {
	if xerrors.Is(err, pubsub.ErrDroppedMessages) {
		q.logger.Warn(q.ctx, "pubsub may have dropped peer updates")
		// we need to schedule a full resync of peer mappings
		q.resyncPeerMappings()
		return
	}
	if err != nil {
		q.logger.Warn(q.ctx, "unhandled pubsub error", slog.Error(err))
		return
	}
	peer, err := parsePeerUpdate(string(msg))
	if err != nil {
		q.logger.Error(q.ctx, "failed to parse peer update",
			slog.F("msg", string(msg)), slog.Error(err))
		return
	}

	logger := q.logger.With(slog.F("peer_id", peer))
	logger.Debug(q.ctx, "got peer update")

	// we know that this peer has an updated node mapping, but we don't yet know who to send that
	// update to. We need to query the database to find all the other peers that share a tunnel with
	// this one, and then run mapping queries against all of them.
	q.workQ.enqueue(querierWorkKey{peerUpdate: peer})
}

func (q *querier) listenTunnel(_ context.Context, msg []byte, err error) {
	if xerrors.Is(err, pubsub.ErrDroppedMessages) {
		q.logger.Warn(q.ctx, "pubsub may have dropped tunnel updates")
		// we need to schedule a full resync of peer mappings
		q.resyncPeerMappings()
		return
	}
	if err != nil {
		q.logger.Warn(q.ctx, "unhandled pubsub error", slog.Error(err))
	}
	peers, err := parseTunnelUpdate(string(msg))
	if err != nil {
		q.logger.Error(q.ctx, "failed to parse tunnel update", slog.F("msg", string(msg)), slog.Error(err))
		return
	}
	q.logger.Debug(q.ctx, "got tunnel update", slog.F("peers", peers))
	for _, peer := range peers {
		mk := mKey(peer)
		q.mu.Lock()
		_, ok := q.mappers[mk]
		q.mu.Unlock()
		if !ok {
			q.logger.Debug(q.ctx, "ignoring tunnel update because we have no mapper",
				slog.F("peer_id", peer))
			continue
		}
		q.workQ.enqueue(querierWorkKey{mappingQuery: mk})
	}
}

func (q *querier) resyncPeerMappings() {
	q.mu.Lock()
	defer q.mu.Unlock()
	for mk := range q.mappers {
		q.workQ.enqueue(querierWorkKey{mappingQuery: mk})
	}
}

func (q *querier) handleUpdates() {
	for {
		select {
		case <-q.ctx.Done():
			return
		case u := <-q.updates:
			if u.filter == filterUpdateUpdated {
				q.updateAll()
			}
			if u.health == healthUpdateUnhealthy {
				q.unhealthyCloseAll()
				continue
			}
			if u.health == healthUpdateHealthy {
				q.setHealthy()
				continue
			}
		}
	}
}

func (q *querier) updateAll() {
	q.mu.Lock()
	defer q.mu.Unlock()

	for _, mpr := range q.mappers {
		// send on goroutine to avoid holding the q.mu.  Heartbeat failures come asynchronously with respect to
		// other kinds of work, so it's fine to deliver the command to refresh async.
		go func(m *mapper) {
			// make sure we send on the _mapper_ context, not our own in case the mapper is
			// shutting down or shut down.
			_ = agpl.SendCtx(m.ctx, m.update, struct{}{})
		}(mpr)
	}
}

// unhealthyCloseAll marks the coordinator unhealthy and closes all connections.  We do this so that peers
// are forced to reconnect to the coordinator, and will hopefully land on a healthy coordinator.
func (q *querier) unhealthyCloseAll() {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.healthy = false
	for _, mpr := range q.mappers {
		// close connections async so that we don't block the querier routine that responds to updates
		go func(c *connIO) {
			err := c.Close()
			if err != nil {
				q.logger.Debug(q.ctx, "error closing conn while unhealthy", slog.Error(err))
			}
		}(mpr.c)
		// NOTE: we don't need to remove the connection from the map, as that will happen async in q.cleanupConn()
	}
}

func (q *querier) setHealthy() {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.healthy = true
}

func parseTunnelUpdate(msg string) ([]uuid.UUID, error) {
	parts := strings.Split(msg, ",")
	if len(parts) != 2 {
		return nil, xerrors.Errorf("expected 2 parts separated by comma")
	}
	peers := make([]uuid.UUID, 2)
	var err error
	for i, part := range parts {
		peers[i], err = uuid.Parse(part)
		if err != nil {
			return nil, xerrors.Errorf("failed to parse UUID: %w", err)
		}
	}
	return peers, nil
}

func parsePeerUpdate(msg string) (peer uuid.UUID, err error) {
	peer, err = uuid.Parse(msg)
	if err != nil {
		return uuid.Nil, xerrors.Errorf("failed to parse peer update message UUID: %w", err)
	}
	return peer, nil
}

// mKey identifies a set of node mappings we want to query.
type mKey uuid.UUID

// mapping associates a particular peer, and its respective coordinator with a node.
type mapping struct {
	peer        uuid.UUID
	coordinator uuid.UUID
	updatedAt   time.Time
	node        *proto.Node
	kind        proto.CoordinateResponse_PeerUpdate_Kind
}

// querierWorkKey describes two kinds of work the querier needs to do.  If peerUpdate
// is not uuid.Nil, then the querier needs to find all tunnel peers of the given peer and
// mark them for a mapping query.  If mappingQuery is not uuid.Nil, then the querier has to
// query the mappings of the tunnel peers of the given peer.
type querierWorkKey struct {
	peerUpdate   uuid.UUID
	mappingQuery mKey
}

type queueKey interface {
	bKey | tKey | querierWorkKey
}

// workQ allows scheduling work based on a key.  Multiple enqueue requests for the same key are coalesced, and
// only one in-progress job per key is scheduled.
type workQ[K queueKey] struct {
	ctx context.Context

	cond       *sync.Cond
	pending    []K
	inProgress map[K]bool
}

func newWorkQ[K queueKey](ctx context.Context) *workQ[K] {
	q := &workQ[K]{
		ctx:        ctx,
		cond:       sync.NewCond(&sync.Mutex{}),
		inProgress: make(map[K]bool),
	}
	// wake up all waiting workers when context is done
	go func() {
		<-ctx.Done()
		q.cond.L.Lock()
		defer q.cond.L.Unlock()
		q.cond.Broadcast()
	}()
	return q
}

// enqueue adds the key to the workQ if it is not already pending.
func (q *workQ[K]) enqueue(key K) {
	q.cond.L.Lock()
	defer q.cond.L.Unlock()
	for _, mk := range q.pending {
		if mk == key {
			// already pending, no-op
			return
		}
	}
	q.pending = append(q.pending, key)
	q.cond.Signal()
}

// acquire gets a new key to begin working on.  This call blocks until work is available.  After acquiring a key, the
// worker MUST call done() with the same key to mark it complete and allow new pending work to be acquired for the key.
// An error is returned if the workQ context is canceled to unblock waiting workers.
func (q *workQ[K]) acquire() (key K, err error) {
	q.cond.L.Lock()
	defer q.cond.L.Unlock()
	for !q.workAvailable() && q.ctx.Err() == nil {
		q.cond.Wait()
	}
	if q.ctx.Err() != nil {
		return key, q.ctx.Err()
	}
	for i, mk := range q.pending {
		_, ok := q.inProgress[mk]
		if !ok {
			q.pending = append(q.pending[:i], q.pending[i+1:]...)
			q.inProgress[mk] = true
			return mk, nil
		}
	}
	// this should not be possible because we are holding the lock when we exit the loop that waits
	panic("woke with no work available")
}

// workAvailable returns true if there is work we can do.  Must be called while holding q.cond.L
func (q workQ[K]) workAvailable() bool {
	for _, mk := range q.pending {
		_, ok := q.inProgress[mk]
		if !ok {
			return true
		}
	}
	return false
}

// done marks the key completed; MUST be called after acquire() for each key.
func (q *workQ[K]) done(key K) {
	q.cond.L.Lock()
	defer q.cond.L.Unlock()
	delete(q.inProgress, key)
	q.cond.Signal()
}

type filterUpdate int

const (
	filterUpdateNone filterUpdate = iota
	filterUpdateUpdated
)

type healthUpdate int

const (
	healthUpdateNone healthUpdate = iota
	healthUpdateHealthy
	healthUpdateUnhealthy
)

// hbUpdate is an update sent from the heartbeats to the querier.  Zero values of the fields mean no update of that
// kind.
type hbUpdate struct {
	filter filterUpdate
	health healthUpdate
}

// heartbeats sends heartbeats for this coordinator on a timer, and monitors heartbeats from other coordinators.  If a
// coordinator misses their heartbeat, we remove it from our map of "valid" coordinators, such that we will filter out
// any mappings for it when filter() is called, and we send a signal on the update channel, which triggers all mappers
// to recompute their mappings and push them out to their connections.
type heartbeats struct {
	ctx    context.Context
	logger slog.Logger
	pubsub pubsub.Pubsub
	store  database.Store
	self   uuid.UUID

	update           chan<- hbUpdate
	firstHeartbeat   chan<- struct{}
	failedHeartbeats int

	lock         sync.RWMutex
	coordinators map[uuid.UUID]time.Time
	timer        *time.Timer

	// overwritten in tests, but otherwise constant
	cleanupPeriod time.Duration
}

func newHeartbeats(
	ctx context.Context, logger slog.Logger,
	ps pubsub.Pubsub, store database.Store,
	self uuid.UUID, update chan<- hbUpdate,
	firstHeartbeat chan<- struct{},
) *heartbeats {
	h := &heartbeats{
		ctx:            ctx,
		logger:         logger,
		pubsub:         ps,
		store:          store,
		self:           self,
		update:         update,
		firstHeartbeat: firstHeartbeat,
		coordinators:   make(map[uuid.UUID]time.Time),
		cleanupPeriod:  cleanupPeriod,
	}
	go h.subscribe()
	go h.sendBeats()
	go h.cleanupLoop()
	return h
}

func (h *heartbeats) filter(mappings []mapping) []mapping {
	out := make([]mapping, 0, len(mappings))
	h.lock.RLock()
	defer h.lock.RUnlock()
	for _, m := range mappings {
		ok := m.coordinator == h.self
		if !ok {
			_, ok = h.coordinators[m.coordinator]
		}
		if ok {
			out = append(out, m)
		}
	}
	return out
}

func (h *heartbeats) subscribe() {
	eb := backoff.NewExponentialBackOff()
	eb.MaxElapsedTime = 0 // retry indefinitely
	eb.MaxInterval = dbMaxBackoff
	bkoff := backoff.WithContext(eb, h.ctx)
	var cancel context.CancelFunc
	bErr := backoff.Retry(func() error {
		cancelFn, err := h.pubsub.SubscribeWithErr(EventHeartbeats, h.listen)
		if err != nil {
			h.logger.Warn(h.ctx, "failed to tunnel to heartbeats", slog.Error(err))
			return err
		}
		cancel = cancelFn
		return nil
	}, bkoff)
	if bErr != nil {
		if h.ctx.Err() == nil {
			h.logger.Error(h.ctx, "code bug: retry failed before context canceled", slog.Error(bErr))
		}
		return
	}
	// cancel subscription when context finishes
	defer cancel()
	<-h.ctx.Done()
}

func (h *heartbeats) listen(_ context.Context, msg []byte, err error) {
	if err != nil {
		// in the context of heartbeats, if we miss some messages it will be OK as long
		// as we aren't disconnected for multiple beats.  Still, even if we are disconnected
		// for longer, there isn't much to do except log.  Once we reconnect we will reinstate
		// any expired coordinators that are still alive and continue on.
		h.logger.Warn(h.ctx, "heartbeat notification error", slog.Error(err))
		return
	}
	id, err := uuid.Parse(string(msg))
	if err != nil {
		h.logger.Error(h.ctx, "unable to parse heartbeat", slog.F("msg", string(msg)), slog.Error(err))
		return
	}
	if id == h.self {
		h.logger.Debug(h.ctx, "ignoring our own heartbeat")
		return
	}
	h.recvBeat(id)
}

func (h *heartbeats) recvBeat(id uuid.UUID) {
	h.logger.Debug(h.ctx, "got heartbeat", slog.F("other_coordinator_id", id))
	h.lock.Lock()
	defer h.lock.Unlock()
	if _, ok := h.coordinators[id]; !ok {
		h.logger.Info(h.ctx, "heartbeats (re)started", slog.F("other_coordinator_id", id))
		// send on a separate goroutine to avoid holding lock.  Triggering update can be async
		go func() {
			_ = agpl.SendCtx(h.ctx, h.update, hbUpdate{filter: filterUpdateUpdated})
		}()
	}
	h.coordinators[id] = time.Now()

	if h.timer == nil {
		// this can only happen for the very first beat
		h.timer = time.AfterFunc(MissedHeartbeats*HeartbeatPeriod, h.checkExpiry)
		h.logger.Debug(h.ctx, "set initial heartbeat timeout")
		return
	}
	h.resetExpiryTimerWithLock()
}

func (h *heartbeats) resetExpiryTimerWithLock() {
	var oldestTime time.Time
	for _, t := range h.coordinators {
		if oldestTime.IsZero() || t.Before(oldestTime) {
			oldestTime = t
		}
	}
	d := time.Until(oldestTime.Add(MissedHeartbeats * HeartbeatPeriod))
	h.logger.Debug(h.ctx, "computed oldest heartbeat", slog.F("oldest", oldestTime), slog.F("time_to_expiry", d))
	// only reschedule if it's in the future.
	if d > 0 {
		h.timer.Reset(d)
	}
}

func (h *heartbeats) checkExpiry() {
	h.logger.Debug(h.ctx, "checking heartbeat expiry")
	h.lock.Lock()
	defer h.lock.Unlock()
	now := time.Now()
	expired := false
	for id, t := range h.coordinators {
		lastHB := now.Sub(t)
		h.logger.Debug(h.ctx, "last heartbeat from coordinator", slog.F("other_coordinator_id", id), slog.F("last_heartbeat", lastHB))
		if lastHB > MissedHeartbeats*HeartbeatPeriod {
			expired = true
			delete(h.coordinators, id)
			h.logger.Info(h.ctx, "coordinator failed heartbeat check", slog.F("other_coordinator_id", id), slog.F("last_heartbeat", lastHB))
		}
	}
	if expired {
		// send on a separate goroutine to avoid holding lock.  Triggering update can be async
		go func() {
			_ = agpl.SendCtx(h.ctx, h.update, hbUpdate{filter: filterUpdateUpdated})
		}()
	}
	// we need to reset the timer for when the next oldest coordinator will expire, if any.
	h.resetExpiryTimerWithLock()
}

func (h *heartbeats) sendBeats() {
	// send an initial heartbeat so that other coordinators can start using our bindings right away.
	h.sendBeat()
	close(h.firstHeartbeat) // signal binder it can start writing
	defer h.sendDelete()
	tkr := time.NewTicker(HeartbeatPeriod)
	defer tkr.Stop()
	for {
		select {
		case <-h.ctx.Done():
			h.logger.Debug(h.ctx, "ending heartbeats", slog.Error(h.ctx.Err()))
			return
		case <-tkr.C:
			h.sendBeat()
		}
	}
}

func (h *heartbeats) sendBeat() {
	_, err := h.store.UpsertTailnetCoordinator(h.ctx, h.self)
	if xerrors.Is(err, context.Canceled) {
		return
	}
	if err != nil {
		h.logger.Error(h.ctx, "failed to send heartbeat", slog.Error(err))
		h.failedHeartbeats++
		if h.failedHeartbeats == 3 {
			h.logger.Error(h.ctx, "coordinator failed 3 heartbeats and is unhealthy")
			_ = agpl.SendCtx(h.ctx, h.update, hbUpdate{health: healthUpdateUnhealthy})
		}
		return
	}
	h.logger.Debug(h.ctx, "sent heartbeat")
	if h.failedHeartbeats >= 3 {
		h.logger.Info(h.ctx, "coordinator sent heartbeat and is healthy")
		_ = agpl.SendCtx(h.ctx, h.update, hbUpdate{health: healthUpdateHealthy})
	}
	h.failedHeartbeats = 0
}

func (h *heartbeats) sendDelete() {
	// here we don't want to use the main context, since it will have been canceled
	ctx := dbauthz.As(context.Background(), pgCoordSubject)
	err := h.store.DeleteCoordinator(ctx, h.self)
	if err != nil {
		h.logger.Error(h.ctx, "failed to send coordinator delete", slog.Error(err))
		return
	}
	h.logger.Debug(h.ctx, "deleted coordinator")
}

func (h *heartbeats) cleanupLoop() {
	h.cleanup()
	tkr := time.NewTicker(h.cleanupPeriod)
	defer tkr.Stop()
	for {
		select {
		case <-h.ctx.Done():
			h.logger.Debug(h.ctx, "ending cleanupLoop", slog.Error(h.ctx.Err()))
			return
		case <-tkr.C:
			h.cleanup()
		}
	}
}

// cleanup issues a DB command to clean out any old expired coordinators or lost peer state.  The
// cleanup is idempotent, so no need to synchronize with other coordinators.
func (h *heartbeats) cleanup() {
	// the records we are attempting to clean up do no serious harm other than
	// accumulating in the tables, so we don't bother retrying if it fails.
	err := h.store.CleanTailnetCoordinators(h.ctx)
	if err != nil {
		h.logger.Error(h.ctx, "failed to cleanup old coordinators", slog.Error(err))
	}
	err = h.store.CleanTailnetLostPeers(h.ctx)
	if err != nil {
		h.logger.Error(h.ctx, "failed to cleanup lost peers", slog.Error(err))
	}
	err = h.store.CleanTailnetTunnels(h.ctx)
	if err != nil {
		h.logger.Error(h.ctx, "failed to cleanup abandoned tunnels", slog.Error(err))
	}
	h.logger.Debug(h.ctx, "completed cleanup")
}
