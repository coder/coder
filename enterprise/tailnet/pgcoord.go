package tailnet

import (
	"context"
	"database/sql"
	"encoding/base64"
	"maps"
	"math"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/cenkalti/backoff/v4"
	"github.com/google/uuid"
	"golang.org/x/xerrors"
	gProto "google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/database/pubsub"
	"github.com/coder/coder/v2/coderd/rbac"
	"github.com/coder/coder/v2/coderd/rbac/policy"
	agpl "github.com/coder/coder/v2/tailnet"
	"github.com/coder/coder/v2/tailnet/proto"
	"github.com/coder/quartz"
)

const (
	EventHeartbeats        = "tailnet_coordinator_heartbeat"
	eventPeerUpdate        = "tailnet_peer_update"
	eventTunnelUpdate      = "tailnet_tunnel_update"
	eventReadyForHandshake = "tailnet_ready_for_handshake"
	HeartbeatPeriod        = time.Second * 2
	MissedHeartbeats       = 3
	numQuerierWorkers      = 10
	numBinderWorkers       = 10
	numTunnelerWorkers     = 10
	numHandshakerWorkers   = 5
	dbMaxBackoff           = 10 * time.Second
	cleanupPeriod          = time.Hour
	mapperRefreshInterval  = 30 * time.Second
	CloseErrUnhealthy      = "coordinator unhealthy"
)

// proto status values for TailnetPeerUpdate.Status. The proto comment
// documents a stable order: 0=ok, 1=lost, 2=disconnected. The first two
// map to database.TailnetStatus values; "disconnected" is a wire-only
// signal used when a peer row has been deleted and has no DB equivalent.
const (
	peerUpdateStatusOK           int32 = 0
	peerUpdateStatusLost         int32 = 1
	peerUpdateStatusDisconnected int32 = 2
)

// tailnetStatusToProto maps a database.TailnetStatus to its proto wire
// value. Unknown values are treated as "lost" since that is the safer
// invalidation choice for subscribers.
func tailnetStatusToProto(s database.TailnetStatus) int32 {
	switch s {
	case database.TailnetStatusOk:
		return peerUpdateStatusOK
	case database.TailnetStatusLost:
		return peerUpdateStatusLost
	default:
		return peerUpdateStatusLost
	}
}

// publishPeerUpdate marshals an enriched TailnetPeerUpdate proto and
// publishes it on eventPeerUpdate. Subscribers decode the payload and
// use it to skip a per-peer bindings query in the steady state. If the
// payload fails to decode (or is empty), subscribers fall back to the
// pre-§6b full-query path so older publishers remain compatible.
func publishPeerUpdate(
	ctx context.Context,
	ps pubsub.Pubsub,
	logger slog.Logger,
	peerID uuid.UUID,
	coordID uuid.UUID,
	status int32,
	node []byte,
	updatedAt time.Time,
) {
	msg := &proto.TailnetPeerUpdate{
		PeerId:        peerID[:],
		CoordinatorId: coordID[:],
		Status:        status,
		Node:          node,
		UpdatedAt:     timestamppb.New(updatedAt),
	}
	raw, err := gProto.Marshal(msg)
	if err != nil {
		// This should be impossible: all fields are well-formed bytes
		// or a timestamp. Fall back to a UUID-only payload so the
		// subscriber's decode-failure path still triggers a full query.
		logger.Critical(ctx, "failed to marshal peer update", slog.F("peer_id", peerID), slog.Error(err))
		if err := ps.Publish(eventPeerUpdate, []byte(peerID.String())); err != nil {
			logger.Warn(ctx, "failed to publish peer update", slog.F("peer_id", peerID), slog.Error(err))
		}
		return
	}
	// PG NOTIFY requires valid UTF-8 text; raw protobuf bytes can
	// contain arbitrary bytes that PostgreSQL rejects with "invalid
	// byte sequence for encoding UTF8". Base64-encode the payload so
	// it round-trips through PG pubsub. This shim goes away once the
	// NATS transport owns this subject (NATS is binary-safe).
	payload := []byte(base64.StdEncoding.EncodeToString(raw))
	if err := ps.Publish(eventPeerUpdate, payload); err != nil {
		logger.Warn(ctx, "failed to publish peer update", slog.F("peer_id", peerID), slog.Error(err))
	}
}

// publishTunnelUpdate marshals an enriched TailnetTunnelUpdate proto and
// publishes it on eventTunnelUpdate. The op (UPSERT/DELETE) lets
// subscribers skip GetTailnetTunnelPeerBindingsBatch on DELETE since
// there is nothing to fetch. UPSERT subscribers still query bindings
// because the publishing replica may not host the dst peer. If the
// payload fails to marshal (or decode on the receiver side), the
// subscriber falls back to today's CSV "src,dst" parser so older
// publishers and corrupted payloads still trigger a full-query refresh.
func publishTunnelUpdate(
	ctx context.Context,
	ps pubsub.Pubsub,
	logger slog.Logger,
	srcID, dstID, coordID uuid.UUID,
	op proto.TailnetTunnelUpdate_Op,
) {
	msg := &proto.TailnetTunnelUpdate{
		SrcId:         srcID[:],
		DstId:         dstID[:],
		CoordinatorId: coordID[:],
		Op:            op,
	}
	raw, err := gProto.Marshal(msg)
	if err != nil {
		// This should be impossible: all fields are well-formed bytes
		// or an enum. Fall back to the CSV payload so the subscriber's
		// decode-failure path still triggers a full query.
		logger.Critical(ctx, "failed to marshal tunnel update",
			slog.F("src_id", srcID), slog.F("dst_id", dstID), slog.Error(err))
		if err := ps.Publish(eventTunnelUpdate, []byte(srcID.String()+","+dstID.String())); err != nil {
			logger.Warn(ctx, "failed to publish tunnel update",
				slog.F("src_id", srcID), slog.F("dst_id", dstID), slog.Error(err))
		}
		return
	}
	// PG NOTIFY requires valid UTF-8 text; raw protobuf bytes can
	// contain arbitrary bytes that PostgreSQL rejects with "invalid
	// byte sequence for encoding UTF8". Base64-encode the payload so
	// it round-trips through PG pubsub. This shim goes away once the
	// NATS transport owns this subject (NATS is binary-safe).
	payload := []byte(base64.StdEncoding.EncodeToString(raw))
	if err := ps.Publish(eventTunnelUpdate, payload); err != nil {
		logger.Warn(ctx, "failed to publish tunnel update",
			slog.F("src_id", srcID), slog.F("dst_id", dstID), slog.Error(err))
	}
}

func publishCoordinatorHeartbeat(ctx context.Context, ps pubsub.Pubsub, logger slog.Logger, id uuid.UUID) {
	if err := ps.Publish(EventHeartbeats, []byte(id.String())); err != nil {
		logger.Warn(ctx, "failed to publish coordinator heartbeat", slog.F("coordinator_id", id), slog.Error(err))
	}
}

// pgCoord is a postgres-backed coordinator
//
//	                 ┌────────────┐
//	    ┌────────────► handshaker ├────────┐
//	    │            └────────────┘        │
//	    │            ┌──────────┐          │
//	    ├────────────► tunneler ├──────────┤
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
	handshakerCh     chan readyForHandshake
	id               uuid.UUID

	cancel    context.CancelFunc
	closeOnce sync.Once
	closed    chan struct{}

	binder     *binder
	tunneler   *tunneler
	handshaker *handshaker
	querier    *querier
}

var pgCoordSubject = rbac.Subject{
	ID: uuid.Nil.String(),
	Roles: rbac.Roles([]rbac.Role{
		{
			Identifier:  rbac.RoleIdentifier{Name: "tailnetcoordinator"},
			DisplayName: "Tailnet Coordinator",
			Site: rbac.Permissions(map[string][]policy.Action{
				rbac.ResourceTailnetCoordinator.Type: {policy.WildcardSymbol},
			}),
			User:    []rbac.Permission{},
			ByOrgID: map[string]rbac.OrgPermissions{},
		},
	}),
	Scope: rbac.ScopeAll,
}.WithCachedASTValue()

// NewPGCoord creates a high-availability coordinator that stores state in the PostgreSQL database and
// receives notifications of updates via the pubsub.
//
// The optional appPS is a secondary pubsub used for tailnet pub/sub channels
// (heartbeat, peer_update, tunnel_update, ready_for_handshake). When appPS is
// nil, ps is used for tailnet pub/sub, preserving prior behavior. This is
// part of the migration of high-volume tailnet channels from PG LISTEN/NOTIFY
// to embedded NATS.
func NewPGCoord(ctx context.Context, logger slog.Logger, ps pubsub.Pubsub, store database.Store, appPS pubsub.Pubsub) (agpl.Coordinator, error) {
	return newPGCoordInternal(ctx, logger, ps, store, quartz.NewReal(), appPS)
}

// NewTestPGCoord is only used in testing to pass a clock.Clock in.
func NewTestPGCoord(ctx context.Context, logger slog.Logger, ps pubsub.Pubsub, store database.Store, clk quartz.Clock, appPS pubsub.Pubsub) (agpl.Coordinator, error) {
	return newPGCoordInternal(ctx, logger, ps, store, clk, appPS)
}

func newPGCoordInternal(
	ctx context.Context, logger slog.Logger, ps pubsub.Pubsub, store database.Store, clk quartz.Clock, appPS pubsub.Pubsub,
) (
	*pgCoord, error,
) {
	// chosen is the pubsub used for all tailnet channels. When appPS is
	// provided, it overrides ps for tailnet traffic; the original ps is
	// then unused by this coordinator.
	chosen := appPS
	if chosen == nil {
		chosen = ps
	}
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
	// for communicating ready for handshakes with the handshaker
	rfhCh := make(chan readyForHandshake)
	// signals when first heartbeat has been sent, so it's safe to start binding.
	fHB := make(chan struct{})

	c := &pgCoord{
		ctx:              ctx,
		cancel:           cancel,
		logger:           logger,
		pubsub:           chosen,
		store:            store,
		binder:           newBinder(ctx, logger, id, store, chosen, bCh, fHB),
		bindings:         bCh,
		newConnections:   cCh,
		closeConnections: ccCh,
		tunneler:         newTunneler(ctx, logger, id, store, chosen, sCh, fHB),
		tunnelerCh:       sCh,
		handshaker:       newHandshaker(ctx, logger, id, chosen, rfhCh, fHB),
		handshakerCh:     rfhCh,
		id:               id,
		querier:          newQuerier(ctx, logger, id, chosen, store, id, cCh, ccCh, numQuerierWorkers, fHB, clk),
		closed:           make(chan struct{}),
	}
	logger.Info(ctx, "starting coordinator")
	return c, nil
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
	if bestN == nil {
		return nil
	}
	node, err := agpl.ProtoToNode(bestN)
	if err != nil {
		c.logger.Critical(c.ctx, "failed to convert node", slog.F("node", bestN), slog.Error(err))
		return nil
	}
	return node
}

func (c *pgCoord) Close() error {
	c.logger.Info(c.ctx, "closing coordinator")
	c.cancel()
	c.closeOnce.Do(func() { close(c.closed) })
	c.querier.wait()
	c.binder.wait()
	c.tunneler.workerWG.Wait()
	c.handshaker.workerWG.Wait()
	return nil
}

func (c *pgCoord) Coordinate(
	ctx context.Context, id uuid.UUID, name string, a agpl.CoordinateeAuth,
) (
	chan<- *proto.CoordinateRequest, <-chan *proto.CoordinateResponse,
) {
	logger := c.logger.With(slog.F("peer_id", id))
	reqs := make(chan *proto.CoordinateRequest, agpl.RequestBufferSize)
	resps := make(chan *proto.CoordinateResponse, agpl.ResponseBufferSize)
	if !c.querier.isHealthy() {
		// If the coordinator is unhealthy, we don't want to hook this Coordinate call up to the
		// binder, as that can cause an unnecessary call to DeleteTailnetPeer when the connIO is
		// closed.  Instead, we just close the response channel and bail out.
		// c.f. https://github.com/coder/coder/issues/12923
		c.logger.Info(ctx, "closed incoming coordinate call while unhealthy",
			slog.F("peer_id", id),
		)
		resps <- &proto.CoordinateResponse{Error: CloseErrUnhealthy}
		close(resps)
		return reqs, resps
	}
	cIO := newConnIO(c.ctx, ctx, logger, c.bindings, c.tunnelerCh, c.handshakerCh, reqs, resps, id, name, a)
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
	pubsub        pubsub.Pubsub
	updates       <-chan tunnel

	mu     sync.Mutex
	latest map[uuid.UUID]map[uuid.UUID]tunnel
	workQ  *workQ[tKey]

	workerWG sync.WaitGroup
}

func newTunneler(ctx context.Context,
	logger slog.Logger,
	id uuid.UUID,
	store database.Store,
	ps pubsub.Pubsub,
	updates <-chan tunnel,
	startWorkers <-chan struct{},
) *tunneler {
	s := &tunneler{
		ctx:           ctx,
		logger:        logger,
		coordinatorID: id,
		store:         store,
		pubsub:        ps,
		updates:       updates,
		latest:        make(map[uuid.UUID]map[uuid.UUID]tunnel),
		workQ:         newWorkQ[tKey](ctx),
	}
	go s.handle()
	// add to the waitgroup immediately to avoid any races waiting for it before
	// the workers start.
	s.workerWG.Add(numTunnelerWorkers)
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
	defer t.workerWG.Done()
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
		var deleted []database.DeleteAllTailnetTunnelsRow
		deleted, err = t.store.DeleteAllTailnetTunnels(t.ctx, database.DeleteAllTailnetTunnelsParams{
			SrcID:         tun.src,
			CoordinatorID: t.coordinatorID,
		})
		t.logger.Debug(t.ctx, "deleted all tunnels",
			slog.F("src_id", tun.src),
			slog.Error(err),
		)
		if err == nil {
			for _, row := range deleted {
				publishTunnelUpdate(t.ctx, t.pubsub, t.logger,
					row.SrcID, row.DstID, t.coordinatorID,
					proto.TailnetTunnelUpdate_OP_DELETE)
			}
		}
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
			return nil // No row deleted, skip publish.
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
	// Publish for upsert/delete single tunnel cases. The DeleteAll case
	// publishes its own updates above since it returns multiple rows.
	if err == nil && tun.dst != uuid.Nil {
		op := proto.TailnetTunnelUpdate_OP_UPSERT
		if !tun.active {
			op = proto.TailnetTunnelUpdate_OP_DELETE
		}
		publishTunnelUpdate(t.ctx, t.pubsub, t.logger,
			tun.src, tun.dst, t.coordinatorID, op)
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
	pubsub        pubsub.Pubsub
	bindings      <-chan binding

	mu     sync.Mutex
	latest map[bKey]binding
	workQ  *workQ[bKey]

	workerWG sync.WaitGroup
	close    chan struct{}
}

func newBinder(ctx context.Context,
	logger slog.Logger,
	id uuid.UUID,
	store database.Store,
	ps pubsub.Pubsub,
	bindings <-chan binding,
	startWorkers <-chan struct{},
) *binder {
	b := &binder{
		ctx:           ctx,
		logger:        logger,
		coordinatorID: id,
		store:         store,
		pubsub:        ps,
		bindings:      bindings,
		latest:        make(map[bKey]binding),
		workQ:         newWorkQ[bKey](ctx),
		close:         make(chan struct{}),
	}
	go b.handleBindings()
	// add to the waitgroup immediately to avoid any races waiting for it before
	// the workers start.
	b.workerWG.Add(numBinderWorkers)
	go func() {
		<-startWorkers
		for i := 0; i < numBinderWorkers; i++ {
			go b.worker()
		}
	}()

	go func() {
		defer close(b.close)
		<-b.ctx.Done()
		b.logger.Debug(b.ctx, "binder exiting, waiting for workers")

		b.workerWG.Wait()

		b.logger.Debug(b.ctx, "updating peers to lost")

		ctx, cancel := context.WithTimeout(dbauthz.As(context.Background(), pgCoordSubject), time.Second*15)
		defer cancel()
		peerRows, err := b.store.UpdateTailnetPeerStatusByCoordinator(ctx, database.UpdateTailnetPeerStatusByCoordinatorParams{
			CoordinatorID: b.coordinatorID,
			Status:        database.TailnetStatusLost,
		})
		if err != nil {
			b.logger.Error(b.ctx, "update peer status to lost", slog.Error(err))
		}
		for _, row := range peerRows {
			publishPeerUpdate(ctx, b.pubsub, b.logger,
				row.ID, row.CoordinatorID,
				tailnetStatusToProto(row.Status),
				row.Node, row.UpdatedAt)
		}
	}()
	return b
}

func (b *binder) handleBindings() {
	for {
		select {
		case <-b.ctx.Done():
			b.logger.Debug(b.ctx, "binder exiting")
			return
		case bnd := <-b.bindings:
			if b.storeBinding(bnd) {
				b.workQ.enqueue(bnd.bKey)
			}
		}
	}
}

func (b *binder) worker() {
	defer b.workerWG.Done()
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
	var pubPeerID, pubCoordID uuid.UUID
	var pubStatus int32
	var pubNode []byte
	var pubUpdatedAt time.Time
	if bnd.kind == proto.CoordinateResponse_PeerUpdate_DISCONNECTED {
		var delRow database.DeleteTailnetPeerRow
		delRow, err = b.store.DeleteTailnetPeer(b.ctx, database.DeleteTailnetPeerParams{
			ID:            uuid.UUID(bnd.bKey),
			CoordinatorID: b.coordinatorID,
		})
		if xerrors.Is(err, sql.ErrNoRows) {
			// No row deleted; peer was already gone. Skip publish.
			return nil
		}
		// DeleteTailnetPeer only returns id and coordinator_id, so the
		// DB-sourced updated_at is not available here. Wall-clock is
		// acceptable because subscribers treat status=disconnected as
		// invalidation regardless of updated_at: the staleness map is
		// dropped for this peer when status indicates the peer is gone.
		pubPeerID = delRow.ID
		pubCoordID = delRow.CoordinatorID
		pubStatus = peerUpdateStatusDisconnected
		pubNode = nil
		pubUpdatedAt = time.Now()
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
		var row database.TailnetPeer
		row, err = b.store.UpsertTailnetPeer(b.ctx, database.UpsertTailnetPeerParams{
			ID:            uuid.UUID(bnd.bKey),
			CoordinatorID: b.coordinatorID,
			Node:          nodeRaw,
			Status:        status,
		})
		if err == nil {
			pubPeerID = row.ID
			pubCoordID = row.CoordinatorID
			pubStatus = tailnetStatusToProto(row.Status)
			pubNode = row.Node
			pubUpdatedAt = row.UpdatedAt
		}
	}

	if err != nil && !database.IsQueryCanceledError(err) {
		b.logger.Error(b.ctx, "failed to write binding to database",
			slog.F("binding_id", bnd.bKey),
			slog.F("node", bnd.node),
			slog.Error(err))
	}
	if err == nil {
		publishPeerUpdate(b.ctx, b.pubsub, b.logger,
			pubPeerID, pubCoordID, pubStatus, pubNode, pubUpdatedAt)
	}
	return err
}

// storeBinding stores the latest binding, where we interpret kind == DISCONNECTED as removing the binding. This keeps the map
// from growing without bound.
func (b *binder) storeBinding(bnd binding) bool {
	b.mu.Lock()
	defer b.mu.Unlock()

	switch bnd.kind {
	case proto.CoordinateResponse_PeerUpdate_NODE:
		old, ok := b.latest[bnd.bKey]
		if ok && old.kind == proto.CoordinateResponse_PeerUpdate_NODE &&
			nodesEqual(old.node, bnd.node) {
			return false
		}
		b.latest[bnd.bKey] = bnd
	case proto.CoordinateResponse_PeerUpdate_DISCONNECTED:
		delete(b.latest, bnd.bKey)
	case proto.CoordinateResponse_PeerUpdate_LOST:
		// We need to coalesce with the previously stored node, since it
		// must be non-nil in the database.
		old, ok := b.latest[bnd.bKey]
		if !ok {
			// Lost before we ever got a node update. No action.
			return false
		}
		bnd.node = old.node
		b.latest[bnd.bKey] = bnd
	}
	return true
}

// nodesEqual compares two proto.Node messages, ignoring the AsOf
// timestamp which changes on every node build even when nothing else
// has changed.
func nodesEqual(a, b *proto.Node) bool {
	if a == nil || b == nil {
		return a == b
	}
	aClone, _ := gProto.Clone(a).(*proto.Node)
	bClone, _ := gProto.Clone(b).(*proto.Node)
	aClone.AsOf = nil
	bClone.AsOf = nil
	return gProto.Equal(aClone, bClone)
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

func (b *binder) wait() {
	<-b.close
}

// mapper tracks data sent to a peer, and sends updates based on changes read from the database.
type mapper struct {
	ctx    context.Context
	logger slog.Logger

	// reads from this channel trigger recomputing the set of mappings to send, and sending any updates. It is used when
	// coordinators are added or removed
	update chan struct{}

	// resetSent is signaled when a coordinator recovers and mappers need to
	// clear their sent cache so all peers are re-evaluated from scratch.
	resetSent chan struct{}

	mappings chan []mapping

	// enriched delivers single-peer mappings synthesized from a
	// TailnetPeerUpdate proto payload. The mapper merges the entry into
	// its current snapshot (c.latest) and runs the same diff/send path
	// as a full snapshot from m.mappings. This lets enriched pubsub
	// updates avoid GetTailnetTunnelPeerBindingsBatch in the steady
	// state while preserving snapshot-replacement semantics for the
	// other peers the mapper is tracking.
	enriched chan mapping

	c *connIO

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
		ctx:       c.peerCtx, // mapper has same lifetime as the underlying connection it serves
		logger:    logger,
		c:         c,
		update:    make(chan struct{}),
		resetSent: make(chan struct{}, 1),
		mappings:  make(chan []mapping),
		// Buffer enriched mapping deliveries so a brief mapper stall
		// does not force the querier to fall back to a full bindings
		// query for unrelated peers in the same batch.
		enriched:   make(chan mapping, maxBatchSize),
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
			m.c.setLatestMapping(mappings)
			best = m.bestMappings(mappings)
		case enriched := <-m.enriched:
			m.logger.Debug(m.ctx, "got enriched mapping", slog.F("peer_id", enriched.peer))
			// Merge the single-peer enriched mapping into the latest
			// snapshot so bestToUpdate retains snapshot-replacement
			// semantics for all other peers tracked by this mapper.
			// A DISCONNECTED enriched mapping is treated as removal
			// from the snapshot so bestToUpdate emits a disconnect
			// against m.sent rather than a synthetic node entry that
			// would persist forever.
			latest := m.c.getLatestMapping()
			merged := make([]mapping, 0, len(latest)+1)
			isDisconnect := enriched.kind == proto.CoordinateResponse_PeerUpdate_DISCONNECTED
			replaced := false
			for _, mpng := range latest {
				if mpng.peer == enriched.peer {
					replaced = true
					if isDisconnect {
						// Skip; do not include in new snapshot.
						continue
					}
					merged = append(merged, enriched)
					continue
				}
				merged = append(merged, mpng)
			}
			if !replaced && !isDisconnect {
				merged = append(merged, enriched)
			}
			m.c.setLatestMapping(merged)
			best = m.bestMappings(merged)
		case <-m.update:
			m.logger.Debug(m.ctx, "triggered update")
			// Check if a reset was requested. The resetSent channel is
			// buffered so the signal arrives before or concurrently with
			// the update signal.
			select {
			case <-m.resetSent:
				m.logger.Debug(m.ctx, "clearing sent cache due to coordinator recovery")
				m.sent = make(map[uuid.UUID]mapping)
			default:
			}
			best = m.bestMappings(m.c.getLatestMapping())
		}
		su := m.bestToUpdate(best)
		if su == nil {
			m.logger.Debug(m.ctx, "skipping nil node update")
			continue
		}
		failed := false
		for _, chunk := range su.resp.Chunked() {
			if err := m.c.Enqueue(chunk); err != nil {
				// lots of reasons this could happen, most usually, the peer has disconnected.
				m.logger.Debug(m.ctx, "failed to enqueue chunk", slog.Error(err))
				failed = true
				break
			}
		}
		// Only commit the sent cache mutations once all chunks have been
		// enqueued successfully. If any Enqueue failed (e.g. the peer's
		// response channel was full and dropped the update), leave m.sent
		// unchanged so the mapper retries on the next cycle.
		if !failed {
			m.commitSent(su)
		}
	}
}

// sentUpdate captures a coordinate response together with the pending
// mutations to the mapper's sent cache. The mutations are only applied to
// m.sent after the response has been successfully enqueued for delivery.
type sentUpdate struct {
	resp    *proto.CoordinateResponse
	upserts map[uuid.UUID]mapping
	deletes []uuid.UUID
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

// bestToUpdate computes the CoordinateResponse needed to bring the peer's
// view in line with the current "best" mappings. It does NOT mutate m.sent;
// instead, it returns a *sentUpdate describing the pending mutations to apply
// once the response has been successfully enqueued. This avoids losing track
// of pending updates if Enqueue fails (for example because the peer's
// response channel is full and the update gets dropped).
func (m *mapper) bestToUpdate(best map[uuid.UUID]mapping) *sentUpdate {
	resp := new(proto.CoordinateResponse)
	upserts := make(map[uuid.UUID]mapping)
	var deletes []uuid.UUID

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
			Id:     agpl.UUIDToByteSlice(k),
			Node:   mpng.node,
			Kind:   mpng.kind,
			Reason: reason,
		})
		upserts[k] = mpng
	}

	for k := range m.sent {
		if _, ok := best[k]; !ok {
			resp.PeerUpdates = append(resp.PeerUpdates, &proto.CoordinateResponse_PeerUpdate{
				Id:     agpl.UUIDToByteSlice(k),
				Kind:   proto.CoordinateResponse_PeerUpdate_DISCONNECTED,
				Reason: "disconnected",
			})
			deletes = append(deletes, k)
		}
	}

	if len(resp.PeerUpdates) == 0 {
		return nil
	}
	return &sentUpdate{
		resp:    resp,
		upserts: upserts,
		deletes: deletes,
	}
}

// commitSent applies the pending sent-cache mutations from a sentUpdate.
// Callers must only invoke this after the response has been successfully
// enqueued, so that a failure to enqueue does not desynchronize m.sent from
// what the peer has actually received.
func (m *mapper) commitSent(su *sentUpdate) {
	for k, mpng := range su.upserts {
		m.sent[k] = mpng
	}
	for _, k := range su.deletes {
		delete(m.sent, k)
	}
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

	peerUpdateQ *workQ[uuid.UUID]
	mappingQ    *workQ[mKey]

	wg sync.WaitGroup

	heartbeats *heartbeats
	updates    <-chan hbUpdate

	mu      sync.Mutex
	mappers map[mKey]*mapper
	healthy bool

	// enrichedUpdates carries proto-decoded peer updates pulled off the
	// pubsub. The map is consulted by peerUpdateWorker after acquiring a
	// peer ID from peerUpdateQ; entries are removed once consumed. When
	// no entry exists for a peer ID, the worker falls back to today's
	// full DB-query path.
	//
	// peerUpdateLastSeen tracks the last observed updated_at per peer so
	// stale enriched messages are dropped before they reach the queue.
	// Entries are removed when a peer transitions to lost or
	// disconnected, bounding map size to currently-active peers.
	peerUpdatesMu      sync.Mutex
	enrichedUpdates    map[uuid.UUID]*proto.TailnetPeerUpdate
	peerUpdateLastSeen map[uuid.UUID]time.Time

	clock quartz.Clock
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
	clk quartz.Clock,
) *querier {
	updates := make(chan hbUpdate)
	q := &querier{
		ctx:                ctx,
		logger:             logger.Named("querier"),
		coordinatorID:      coordinatorID,
		pubsub:             ps,
		store:              store,
		newConnections:     newConnections,
		closeConnections:   closeConnections,
		peerUpdateQ:        newWorkQ[uuid.UUID](ctx),
		mappingQ:           newWorkQ[mKey](ctx),
		heartbeats:         newHeartbeats(ctx, logger, ps, store, self, updates, firstHeartbeat, clk),
		clock:              clk,
		mappers:            make(map[mKey]*mapper),
		updates:            updates,
		healthy:            true, // assume we start healthy
		enrichedUpdates:    make(map[uuid.UUID]*proto.TailnetPeerUpdate),
		peerUpdateLastSeen: make(map[uuid.UUID]time.Time),
	}
	q.subscribe()

	// For an odd number of workers we allocate more to the mapping workers since they're busier.
	mappingWorkers := int(math.Ceil(float64(numWorkers) / 2))
	peerWorkers := numWorkers - mappingWorkers

	q.wg.Add(3 + mappingWorkers + peerWorkers)
	go func() {
		<-firstHeartbeat
		go q.handleIncoming()
		go q.handleUpdates()
		go q.periodicRefresh()
		for range mappingWorkers {
			go q.mappingWorker()
		}
		for range peerWorkers {
			go q.peerUpdateWorker()
		}
	}()
	return q
}

func (q *querier) wait() {
	q.wg.Wait()
	q.heartbeats.wg.Wait()
}

func (q *querier) periodicRefresh() {
	defer q.wg.Done()
	tkr := q.clock.TickerFunc(q.ctx, mapperRefreshInterval, func() error {
		q.resyncPeerMappings()
		return nil
	}, "querier", "periodicRefresh")
	err := tkr.Wait()
	if err != nil && q.ctx.Err() == nil {
		q.logger.Error(q.ctx, "periodic refresh ended unexpectedly", slog.Error(err))
	}
}

func (q *querier) handleIncoming() {
	defer q.wg.Done()
	for {
		select {
		case <-q.ctx.Done():
			return

		case c := <-q.newConnections:
			q.logger.Debug(q.ctx, "new connection received", slog.F("peer_id", c.UniqueID()))
			q.newConn(c)

		case c := <-q.closeConnections:
			q.logger.Debug(q.ctx, "connection close request", slog.F("peer_id", c.UniqueID()))
			q.cleanupConn(c)
		}
	}
}

func (q *querier) newConn(c *connIO) {
	q.mu.Lock()
	defer q.mu.Unlock()
	if !q.healthy {
		_ = c.Enqueue(&proto.CoordinateResponse{Error: CloseErrUnhealthy})
		err := c.Close()
		// This can only happen during a narrow window where we were healthy
		// when pgCoord checked before accepting the connection, but now are
		// unhealthy now that we get around to processing it. Seeing a small
		// number of these logs is not worrying, but a large number probably
		// indicates something is amiss.
		q.logger.Warn(q.ctx, "closed incoming connection while unhealthy",
			slog.Error(err),
			slog.F("peer_id", c.UniqueID()),
		)
		return
	}
	mpr := newMapper(c, q.logger, q.heartbeats)
	mk := mKey(c.UniqueID())
	dup, ok := q.mappers[mk]
	if ok {
		q.logger.Debug(q.ctx, "duplicate mapper found; closing old connection", slog.F("peer_id", dup.c.UniqueID()))
		// overwrite and close the old one
		atomic.StoreInt64(&c.overwrites, dup.c.Overwrites()+1)
		err := dup.c.CoordinatorClose()
		if err != nil {
			q.logger.Error(q.ctx, "failed to close duplicate mapper", slog.F("peer_id", dup.c.UniqueID()), slog.Error(err))
		}
	}
	q.mappers[mk] = mpr
	q.mappingQ.enqueue(mk)
	q.logger.Debug(q.ctx, "added new mapper", slog.F("peer_id", c.UniqueID()))
}

func (q *querier) isHealthy() bool {
	q.mu.Lock()
	defer q.mu.Unlock()
	return q.healthy
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
	q.logger.Debug(q.ctx, "removed mapper", slog.F("peer_id", c.UniqueID()))
}

// maxBatchSize is the maximum number of keys to process in a single batch
// query.
const maxBatchSize = 50

func (q *querier) peerUpdateWorker() {
	defer q.wg.Done()
	defer q.logger.Debug(q.ctx, "peerUpdate worker exited")
	eb := backoff.NewExponentialBackOff()
	eb.MaxElapsedTime = 0 // retry indefinitely
	eb.MaxInterval = dbMaxBackoff
	bkoff := backoff.WithContext(eb, q.ctx)
	for {
		allKeys, err := q.peerUpdateQ.acquireBatch(maxBatchSize)
		if err != nil {
			return
		}
		peers := make([]uuid.UUID, 0, len(allKeys))
		peers = append(peers, allKeys...)
		err = backoff.Retry(func() error {
			return q.peerUpdate(peers)
		}, bkoff)
		if err != nil {
			bkoff.Reset()
		}
		q.peerUpdateQ.done(allKeys...)
	}
}

func (q *querier) mappingWorker() {
	defer q.wg.Done()
	defer q.logger.Debug(q.ctx, "mapping worker exited")
	eb := backoff.NewExponentialBackOff()
	eb.MaxElapsedTime = 0 // retry indefinitely
	eb.MaxInterval = dbMaxBackoff
	bkoff := backoff.WithContext(eb, q.ctx)
	for {
		allKeys, err := q.mappingQ.acquireBatch(maxBatchSize)
		if err != nil {
			return
		}
		mkeys := make([]mKey, 0, len(allKeys))
		mkeys = append(mkeys, allKeys...)
		err = backoff.Retry(func() error {
			return q.mappingQuery(mkeys)
		}, bkoff)
		if err != nil {
			bkoff.Reset()
		}
		q.mappingQ.done(allKeys...)
	}
}

// peerUpdate is work scheduled in response to a new peer->binding.  We need to find out all the
// other peers that share a tunnel with the indicated peer, and then schedule a mapping update on
// each, so that they can find out about the new binding.
//
// When listenPeer received an enriched TailnetPeerUpdate proto for one of
// these peer IDs, we look up the synthesized payload from
// q.enrichedUpdates and dispatch the resulting mapping directly to each
// active sharing-peer mapper, skipping GetTailnetTunnelPeerBindingsBatch
// for the message's own peer. Sharing peers without an enriched payload
// for this update fall back to today's full mapping-query path.
func (q *querier) peerUpdate(peers []uuid.UUID) error {
	// Drain any enriched payloads that landed while these peer IDs were
	// queued. A missing entry means this batch contains a UUID-only
	// fallback (or the enriched payload was already consumed) and we
	// must use the full DB-query path.
	enriched := make(map[uuid.UUID]*proto.TailnetPeerUpdate, len(peers))
	q.peerUpdatesMu.Lock()
	for _, p := range peers {
		if e, ok := q.enrichedUpdates[p]; ok {
			enriched[p] = e
			delete(q.enrichedUpdates, p)
		}
	}
	q.peerUpdatesMu.Unlock()

	q.logger.Debug(q.ctx, "batch querying peers that share tunnels",
		slog.F("num_peers", len(peers)),
		slog.F("num_enriched", len(enriched)))
	others, err := q.store.GetTailnetTunnelPeerIDsBatch(q.ctx, peers)
	if err != nil && !xerrors.Is(err, sql.ErrNoRows) {
		return xerrors.Errorf("get tunnel peer IDs batch: %w", err)
	}
	q.logger.Debug(q.ctx, "batch queried tunnel peers",
		slog.F("num_results", len(others)))
	q.mu.Lock()
	defer q.mu.Unlock()
	for _, other := range others {
		mk := mKey(other.PeerID)
		mpr, ok := q.mappers[mk]
		if !ok {
			continue
		}
		if e, hasEnriched := enriched[other.LookupID]; hasEnriched {
			mpng, err := enrichedToMapping(e)
			if err != nil {
				// Decoding the embedded node failed; fall back to a
				// full DB query for this sharing peer rather than
				// dropping the update.
				q.logger.Error(q.ctx, "failed to decode enriched peer node",
					slog.F("peer_id", other.LookupID), slog.Error(err))
				q.mappingQ.enqueue(mk)
				continue
			}
			// Best-effort non-blocking send; mapper.run drives this
			// channel, but if it is busy we fall back to a full
			// mapping query so the update is not dropped.
			select {
			case mpr.enriched <- mpng:
			default:
				q.mappingQ.enqueue(mk)
			}
			continue
		}
		q.mappingQ.enqueue(mk)
	}
	return nil
}

// enrichedToMapping synthesizes a mapping from a TailnetPeerUpdate
// proto. It mirrors bindingsToMappings but operates on a single
// already-decoded payload rather than rows from
// GetTailnetTunnelPeerBindingsBatch.
func enrichedToMapping(e *proto.TailnetPeerUpdate) (mapping, error) {
	peerBytes := e.GetPeerId()
	coordBytes := e.GetCoordinatorId()
	if len(peerBytes) != len(uuid.UUID{}) || len(coordBytes) != len(uuid.UUID{}) {
		return mapping{}, xerrors.Errorf("enriched peer update has invalid id lengths")
	}
	var peer, coord uuid.UUID
	copy(peer[:], peerBytes)
	copy(coord[:], coordBytes)
	node := new(proto.Node)
	if len(e.GetNode()) > 0 {
		if err := gProto.Unmarshal(e.GetNode(), node); err != nil {
			return mapping{}, xerrors.Errorf("unmarshal enriched node: %w", err)
		}
	}
	kind := proto.CoordinateResponse_PeerUpdate_NODE
	switch e.GetStatus() {
	case peerUpdateStatusLost:
		kind = proto.CoordinateResponse_PeerUpdate_LOST
	case peerUpdateStatusDisconnected:
		kind = proto.CoordinateResponse_PeerUpdate_DISCONNECTED
	}
	return mapping{
		peer:        peer,
		coordinator: coord,
		updatedAt:   e.GetUpdatedAt().AsTime(),
		node:        node,
		kind:        kind,
	}, nil
}

// mappingQuery queries the database for all the mappings that the given peers should know about,
// that is, all the peers that it shares a tunnel with and their current node mappings (if they
// exist).  It then sends the mapping snapshot to the corresponding mapper, where it will get
// transmitted to the peer.
func (q *querier) mappingQuery(peers []mKey) error {
	// Filter to peers with active mappers before hitting the DB.
	q.mu.Lock()
	active := make([]uuid.UUID, 0, len(peers))
	activeKeys := make([]mKey, 0, len(peers))
	for _, p := range peers {
		if _, ok := q.mappers[p]; ok {
			active = append(active, uuid.UUID(p))
			activeKeys = append(activeKeys, p)
		}
	}
	q.mu.Unlock()
	if len(active) == 0 {
		q.logger.Debug(q.ctx, "batch mapping query: no active mappers")
		return nil
	}

	q.logger.Debug(q.ctx, "batch querying mappings",
		slog.F("num_peers", len(active)))
	bindings, err := q.store.GetTailnetTunnelPeerBindingsBatch(q.ctx, active)
	if err != nil && !xerrors.Is(err, sql.ErrNoRows) {
		return xerrors.Errorf("get tunnel peer bindings batch: %w", err)
	}
	q.logger.Debug(q.ctx, "batch queried mappings",
		slog.F("num_bindings", len(bindings)))

	// Group bindings by lookup_id (the peer that needs the mapping).
	grouped := make(map[uuid.UUID][]database.GetTailnetTunnelPeerBindingsBatchRow)
	for _, b := range bindings {
		grouped[b.LookupID] = append(grouped[b.LookupID], b)
	}

	// Dispatch each peer's mappings to its mapper.
	for _, mk := range activeKeys {
		peerID := uuid.UUID(mk)
		rows := grouped[peerID]
		mappings, err := q.bindingsToMappings(rows)
		if err != nil {
			q.logger.Error(q.ctx, "failed to convert batch mappings",
				slog.F("peer_id", peerID), slog.Error(err))
			continue
		}
		q.mu.Lock()
		mpr, ok := q.mappers[mk]
		q.mu.Unlock()
		if !ok {
			continue
		}
		if err := agpl.SendCtx(mpr.ctx, mpr.mappings, mappings); err != nil {
			q.logger.Debug(q.ctx, "failed to send mappings to peer",
				slog.F("peer_id", peerID), slog.Error(err))
			continue
		}
	}
	return nil
}

// bindingsToMappings converts binding rows to mappings.
func (q *querier) bindingsToMappings(bindings []database.GetTailnetTunnelPeerBindingsBatchRow) ([]mapping, error) {
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
		defer func() {
			q.logger.Info(q.ctx, "canceling peer updates subscription")
			cancelPeer()
		}()
		bkoff.Reset()
		q.logger.Info(q.ctx, "subscribed to peer updates")

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
		defer func() {
			q.logger.Info(q.ctx, "canceling tunnel updates subscription")
			cancelTunnel()
		}()
		q.logger.Info(q.ctx, "subscribed to tunnel updates")

		var cancelRFH context.CancelFunc
		err = backoff.Retry(func() error {
			cancelFn, err := q.pubsub.SubscribeWithErr(eventReadyForHandshake, q.listenReadyForHandshake)
			if err != nil {
				q.logger.Warn(q.ctx, "failed to subscribe to ready for handshakes", slog.Error(err))
				return err
			}
			cancelRFH = cancelFn
			return nil
		}, bkoff)
		if err != nil {
			if q.ctx.Err() == nil {
				q.logger.Error(q.ctx, "code bug: retry failed before context canceled", slog.Error(err))
			}
			return
		}
		defer func() {
			q.logger.Info(q.ctx, "canceling ready for handshake subscription")
			cancelRFH()
		}()
		q.logger.Info(q.ctx, "subscribed to ready for handshakes")

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
	// Try the enriched proto payload first. PG NOTIFY transport
	// base64-encodes the raw proto, so attempt that decode before
	// trying raw bytes (forward-compat with the binary-safe NATS
	// transport). If everything fails, fall back to today's UUID
	// string parser so older publishers and decode failures still
	// drive a full-query refresh rather than being silently dropped.
	if enriched, ok := tryDecodeEnrichedPeerUpdate(msg); ok {
		peer := uuid.UUID{}
		copy(peer[:], enriched.GetPeerId())
		updatedAt := enriched.GetUpdatedAt().AsTime()
		status := enriched.GetStatus()
		q.peerUpdatesMu.Lock()
		prev, seen := q.peerUpdateLastSeen[peer]
		if seen && !updatedAt.After(prev) {
			q.peerUpdatesMu.Unlock()
			q.logger.Debug(q.ctx, "dropping stale enriched peer update",
				slog.F("peer_id", peer),
				slog.F("updated_at", updatedAt),
				slog.F("last_seen", prev))
			return
		}
		// Drop the entry when the peer transitions away from "ok"
		// (lost or disconnected) so the map is bounded by currently
		// active peers.
		if status == peerUpdateStatusOK {
			q.peerUpdateLastSeen[peer] = updatedAt
		} else {
			delete(q.peerUpdateLastSeen, peer)
		}
		q.enrichedUpdates[peer] = enriched
		q.peerUpdatesMu.Unlock()

		q.logger.Debug(q.ctx, "got enriched peer update", slog.F("peer_id", peer))
		q.peerUpdateQ.enqueue(peer)
		return
	}

	peer, err := parsePeerUpdate(string(msg))
	if err != nil {
		q.logger.Error(q.ctx, "failed to parse peer update",
			slog.F("msg", string(msg)), slog.Error(err))
		return
	}

	logger := q.logger.With(slog.F("peer_id", peer))
	logger.Debug(q.ctx, "got peer update (fallback)")

	// we know that this peer has an updated node mapping, but we don't yet know who to send that
	// update to. We need to query the database to find all the other peers that share a tunnel with
	// this one, and then run mapping queries against all of them.
	q.peerUpdateQ.enqueue(peer)
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
		return
	}
	// Try the enriched proto payload first. PG NOTIFY transport
	// base64-encodes the raw proto, so attempt that decode before
	// trying raw bytes (forward-compat with the binary-safe NATS
	// transport). On decode failure we fall back to today's CSV
	// "src,dst" parser so older publishers and corrupted payloads
	// still trigger a full-query refresh.
	if enriched, ok := tryDecodeEnrichedTunnelUpdate(msg); ok {
		var src, dst, coord uuid.UUID
		copy(src[:], enriched.GetSrcId())
		copy(dst[:], enriched.GetDstId())
		copy(coord[:], enriched.GetCoordinatorId())
		op := enriched.GetOp()
		q.logger.Debug(q.ctx, "got enriched tunnel update",
			slog.F("src_id", src), slog.F("dst_id", dst),
			slog.F("coordinator_id", coord), slog.F("op", op.String()))
		switch op {
		case proto.TailnetTunnelUpdate_OP_DELETE:
			q.handleTunnelDelete(src, dst, coord)
		default:
			// OP_UPSERT and OP_UNSPECIFIED (defensive) keep today's
			// behavior: enqueue mappingQ so the bindings query runs.
			// The publishing replica may not host the dst peer, so we
			// cannot synthesize the mapping from the payload alone.
			q.enqueueTunnelMapping(src, dst)
		}
		return
	}

	peers, err := parseTunnelUpdate(string(msg))
	if err != nil {
		q.logger.Error(q.ctx, "failed to parse tunnel update", slog.F("msg", string(msg)), slog.Error(err))
		return
	}
	q.logger.Debug(q.ctx, "got tunnel update (fallback)", slog.F("peers", peers))
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
		q.mappingQ.enqueue(mk)
	}
}

// enqueueTunnelMapping enqueues a mappingQ refresh for whichever of (src,
// dst) we host a mapper for. This is the UPSERT path: we do not yet know
// if the publishing replica hosts the relevant peer, so we still run
// GetTailnetTunnelPeerBindingsBatch to fetch the latest bindings.
func (q *querier) enqueueTunnelMapping(src, dst uuid.UUID) {
	for _, peer := range [...]uuid.UUID{src, dst} {
		mk := mKey(peer)
		q.mu.Lock()
		_, ok := q.mappers[mk]
		q.mu.Unlock()
		if !ok {
			q.logger.Debug(q.ctx, "ignoring tunnel update because we have no mapper",
				slog.F("peer_id", peer))
			continue
		}
		q.mappingQ.enqueue(mk)
	}
}

// handleTunnelDelete synthesizes a DISCONNECTED enriched mapping for the
// (src, dst) pair and dispatches it directly to the affected mapper(s),
// skipping GetTailnetTunnelPeerBindingsBatch. The mapper's existing
// disconnect handling on m.enriched removes the peer from its snapshot.
// If the mapper.enriched send would block we fall back to mappingQ so
// the update is not dropped.
func (q *querier) handleTunnelDelete(src, dst, coord uuid.UUID) {
	type pair struct {
		mapperPeer uuid.UUID // mapper we want to notify.
		removePeer uuid.UUID // peer to drop from that mapper's snapshot.
	}
	pairs := [...]pair{
		{mapperPeer: src, removePeer: dst},
		{mapperPeer: dst, removePeer: src},
	}
	q.mu.Lock()
	defer q.mu.Unlock()
	for _, p := range pairs {
		mk := mKey(p.mapperPeer)
		mpr, ok := q.mappers[mk]
		if !ok {
			continue
		}
		mpng := mapping{
			peer:        p.removePeer,
			coordinator: coord,
			updatedAt:   time.Now().UTC(),
			kind:        proto.CoordinateResponse_PeerUpdate_DISCONNECTED,
		}
		// Best-effort non-blocking send; mapper.run drives this
		// channel, but if it is busy we fall back to a full mapping
		// query so the update is not dropped.
		select {
		case mpr.enriched <- mpng:
		default:
			q.mappingQ.enqueue(mk)
		}
	}
}

func (q *querier) listenReadyForHandshake(_ context.Context, msg []byte, err error) {
	if err != nil {
		if xerrors.Is(err, pubsub.ErrDroppedMessages) {
			q.logger.Warn(q.ctx, "pubsub dropped ready-for-handshake messages")
		} else {
			q.logger.Warn(q.ctx, "unhandled pubsub error", slog.Error(err))
		}
		return
	}

	to, from, err := parseReadyForHandshake(string(msg))
	if err != nil {
		q.logger.Error(q.ctx, "failed to parse ready for handshake", slog.F("msg", string(msg)), slog.Error(err))
		return
	}

	mk := mKey(to)
	q.mu.Lock()
	mpr, ok := q.mappers[mk]
	q.mu.Unlock()
	if !ok {
		q.logger.Debug(q.ctx, "ignoring ready for handshake because we have no mapper",
			slog.F("peer_id", to))
		return
	}

	_ = mpr.c.Enqueue(&proto.CoordinateResponse{
		PeerUpdates: []*proto.CoordinateResponse_PeerUpdate{{
			Id:   from[:],
			Kind: proto.CoordinateResponse_PeerUpdate_READY_FOR_HANDSHAKE,
		}},
	})
}

func (q *querier) resyncPeerMappings() {
	q.mu.Lock()
	keys := make([]mKey, 0, len(q.mappers))
	for mk := range q.mappers {
		keys = append(keys, mk)
	}
	q.mu.Unlock()
	q.mappingQ.enqueue(keys...)
}

func (q *querier) handleUpdates() {
	defer q.wg.Done()
	for {
		select {
		case <-q.ctx.Done():
			return
		case u := <-q.updates:
			if u.filter == filterUpdateReset {
				q.resetAllSentAndUpdate()
			} else if u.filter == filterUpdateUpdated {
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

// resetAllSentAndUpdate signals all mappers to clear their sent cache and then
// triggers an update. This is called when a coordinator recovers after being
// expired, so that previously-skipped LOST peers get re-evaluated.
func (q *querier) resetAllSentAndUpdate() {
	q.mu.Lock()
	defer q.mu.Unlock()

	for _, mpr := range q.mappers {
		go func(m *mapper) {
			// Signal reset first (buffered channel, non-blocking).
			select {
			case m.resetSent <- struct{}{}:
			default:
			}
			// Then trigger the update so the mapper picks up the reset
			// signal when it processes the update.
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
			_ = c.Enqueue(&proto.CoordinateResponse{Error: CloseErrUnhealthy})
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

// tryDecodeEnrichedPeerUpdate attempts to decode a TailnetPeerUpdate
// proto from a pubsub message. The PG transport base64-encodes the
// proto bytes (PG NOTIFY only carries valid UTF-8); the NATS transport
// will deliver raw proto bytes. We try base64 first, then raw, and
// validate that the decoded message has the required fields. Returns
// (nil, false) when the payload is missing, malformed, or looks like a
// legacy UUID-string update so the caller can fall back.
func tryDecodeEnrichedPeerUpdate(msg []byte) (*proto.TailnetPeerUpdate, bool) {
	if len(msg) == 0 {
		return nil, false
	}
	tryUnmarshal := func(b []byte) (*proto.TailnetPeerUpdate, bool) {
		if len(b) == 0 {
			return nil, false
		}
		var pu proto.TailnetPeerUpdate
		if err := gProto.Unmarshal(b, &pu); err != nil {
			return nil, false
		}
		// Validate required fields. A legacy UUID-string payload may
		// happen to unmarshal into an empty proto, so we require the
		// peer_id and coordinator_id to be valid UUIDs and updated_at
		// to be present.
		if len(pu.GetPeerId()) != len(uuid.UUID{}) {
			return nil, false
		}
		if len(pu.GetCoordinatorId()) != len(uuid.UUID{}) {
			return nil, false
		}
		if pu.GetUpdatedAt() == nil {
			return nil, false
		}
		return &pu, true
	}
	if raw, err := base64.StdEncoding.DecodeString(string(msg)); err == nil {
		if pu, ok := tryUnmarshal(raw); ok {
			return pu, true
		}
	}
	return tryUnmarshal(msg)
}

// tryDecodeEnrichedTunnelUpdate attempts to decode a TailnetTunnelUpdate
// proto from a pubsub message. The PG transport base64-encodes the
// proto bytes (PG NOTIFY only carries valid UTF-8); the NATS transport
// will deliver raw proto bytes. We try base64 first, then raw, and
// validate that the decoded message has the required fields. Returns
// (nil, false) when the payload is missing, malformed, or looks like a
// legacy CSV "src,dst" update so the caller can fall back.
func tryDecodeEnrichedTunnelUpdate(msg []byte) (*proto.TailnetTunnelUpdate, bool) {
	if len(msg) == 0 {
		return nil, false
	}
	tryUnmarshal := func(b []byte) (*proto.TailnetTunnelUpdate, bool) {
		if len(b) == 0 {
			return nil, false
		}
		var tu proto.TailnetTunnelUpdate
		if err := gProto.Unmarshal(b, &tu); err != nil {
			return nil, false
		}
		// Validate required fields. A legacy CSV payload may happen to
		// unmarshal into an empty proto, so we require src_id, dst_id,
		// and coordinator_id to be valid UUID-length byte slices.
		if len(tu.GetSrcId()) != len(uuid.UUID{}) {
			return nil, false
		}
		if len(tu.GetDstId()) != len(uuid.UUID{}) {
			return nil, false
		}
		if len(tu.GetCoordinatorId()) != len(uuid.UUID{}) {
			return nil, false
		}
		return &tu, true
	}
	if raw, err := base64.StdEncoding.DecodeString(string(msg)); err == nil {
		if tu, ok := tryUnmarshal(raw); ok {
			return tu, true
		}
	}
	return tryUnmarshal(msg)
}

func parsePeerUpdate(msg string) (peer uuid.UUID, err error) {
	peer, err = uuid.Parse(msg)
	if err != nil {
		return uuid.Nil, xerrors.Errorf("failed to parse peer update message UUID: %w", err)
	}
	return peer, nil
}

func parseReadyForHandshake(msg string) (to uuid.UUID, from uuid.UUID, err error) {
	parts := strings.Split(msg, ",")
	if len(parts) != 2 {
		return uuid.Nil, uuid.Nil, xerrors.Errorf("expected 2 parts separated by comma")
	}
	ids := make([]uuid.UUID, 2)
	for i, part := range parts {
		ids[i], err = uuid.Parse(part)
		if err != nil {
			return uuid.Nil, uuid.Nil, xerrors.Errorf("failed to parse UUID: %w", err)
		}
	}
	return ids[0], ids[1], nil
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

type queueKey interface {
	bKey | tKey | uuid.UUID | mKey
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
func (q *workQ[K]) enqueue(keys ...K) {
	q.cond.L.Lock()
	defer q.cond.L.Unlock()
	for _, key := range keys {
		if slices.Contains(q.pending, key) {
			continue
		}
		q.pending = append(q.pending, key)
	}
	q.cond.Signal()
}

// acquireBatch blocks until at least one pending key is available, then
// returns up to limit keys, moving them to inProgress. Caller must call
// done() for each returned key.
// An error is returned if the workQ context is canceled to unblock waiting workers.
func (q *workQ[K]) acquireBatch(limit int) ([]K, error) {
	q.cond.L.Lock()
	defer q.cond.L.Unlock()
	for {
		if q.ctx.Err() != nil {
			return nil, q.ctx.Err()
		}
		var batch []K
		remaining := make([]K, 0, len(q.pending))
		for _, k := range q.pending {
			if len(batch) >= limit {
				remaining = append(remaining, k)
				continue
			}
			if _, inProg := q.inProgress[k]; inProg {
				remaining = append(remaining, k)
				continue
			}
			batch = append(batch, k)
			q.inProgress[k] = true
		}
		q.pending = remaining
		if len(batch) > 0 {
			return batch, nil
		}
		q.cond.Wait()
	}
}

// acquire blocks until a work item is available and returns it. After
// acquiring a key, the worker MUST call done() with the same key to mark
// it complete and allow new pending work to be acquired for the key.
func (q *workQ[K]) acquire() (key K, err error) {
	items, err := q.acquireBatch(1)
	if err != nil {
		return key, err
	}
	return items[0], nil
}

// done marks the key completed; MUST be called after acquire() for each key.
func (q *workQ[K]) done(keys ...K) {
	q.cond.L.Lock()
	defer q.cond.L.Unlock()
	for _, key := range keys {
		delete(q.inProgress, key)
	}
	q.cond.Signal()
}

type filterUpdate int

const (
	filterUpdateNone filterUpdate = iota
	filterUpdateUpdated
	filterUpdateReset
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

	lock                sync.RWMutex
	coordinators        map[uuid.UUID]time.Time
	lastDBHeartbeat     map[uuid.UUID]time.Time
	expiredCoordinators map[uuid.UUID]struct{}
	timer               *quartz.Timer

	wg sync.WaitGroup

	// for testing
	clock quartz.Clock
}

func newHeartbeats(
	ctx context.Context, logger slog.Logger,
	ps pubsub.Pubsub, store database.Store,
	self uuid.UUID, update chan<- hbUpdate,
	firstHeartbeat chan<- struct{},
	clk quartz.Clock,
) *heartbeats {
	h := &heartbeats{
		ctx:                 ctx,
		logger:              logger,
		pubsub:              ps,
		store:               store,
		self:                self,
		update:              update,
		firstHeartbeat:      firstHeartbeat,
		coordinators:        make(map[uuid.UUID]time.Time),
		lastDBHeartbeat:     make(map[uuid.UUID]time.Time),
		expiredCoordinators: make(map[uuid.UUID]struct{}),
		clock:               clk,
	}
	// Start the expiry timer so checkExpiry runs even if no pubsub
	// heartbeat is ever received. This enables DB-based discovery of
	// coordinators whose pubsub notifications are permanently lost.
	h.timer = h.clock.AfterFunc(MissedHeartbeats*HeartbeatPeriod, h.checkExpiry, "heartbeats", "newHeartbeats")
	h.wg.Add(3)
	h.subscribe()
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
			if !ok {
				// If a mapping exists to a coordinator lost to heartbeats,
				// still add the mapping as LOST. If a coordinator misses
				// heartbeats but a client is still connected to it, this may be
				// the only mapping available for it. Newer mappings will take
				// precedence.
				m.kind = proto.CoordinateResponse_PeerUpdate_LOST
			}
		}

		out = append(out, m)
	}
	return out
}

func (h *heartbeats) subscribe() {
	defer h.wg.Done()
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
	go func() {
		// cancel subscription when context finishes
		<-h.ctx.Done()
		cancel()
	}()
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
		// Determine whether this coordinator is recovering after expiry
		// or joining for the first time. Recovery needs a full reset so
		// mappers re-evaluate peers that were LOST during the absence.
		// First-time joins only need an incremental update.
		_, wasExpired := h.expiredCoordinators[id]
		delete(h.expiredCoordinators, id)
		filter := filterUpdateUpdated
		if wasExpired {
			filter = filterUpdateReset
		}
		h.logger.Info(h.ctx, "heartbeats (re)started",
			slog.F("other_coordinator_id", id),
			slog.F("was_expired", wasExpired),
		)
		// send on a separate goroutine to avoid holding lock.
		go func() {
			_ = agpl.SendCtx(h.ctx, h.update, hbUpdate{filter: filter})
		}()
	}
	h.coordinators[id] = h.clock.Now("heartbeats", "recvBeat")

	h.resetExpiryTimerWithLock()
}

func (h *heartbeats) resetExpiryTimerWithLock() {
	var oldestTime time.Time
	for _, t := range h.coordinators {
		if oldestTime.IsZero() || t.Before(oldestTime) {
			oldestTime = t
		}
	}
	d := h.clock.Until(
		oldestTime.Add(MissedHeartbeats*HeartbeatPeriod),
		"heartbeats", "resetExpiryTimerWithLock",
	)
	if len(h.coordinators) == 0 {
		// Even with no known coordinators, schedule a fallback timer so
		// checkExpiry runs periodically and can discover coordinators
		// from the database whose pubsub heartbeats were never received.
		fallback := MissedHeartbeats * HeartbeatPeriod
		h.logger.Debug(h.ctx, "no coordinators known, setting fallback expiry timer", slog.F("fallback_duration", fallback))
		h.timer.Reset(fallback, "heartbeats", "resetExpiryTimerWithLock")
		return
	}
	h.logger.Debug(h.ctx, "computed oldest heartbeat", slog.F("oldest", oldestTime), slog.F("time_to_expiry", d))
	if d < 0 {
		d = 0
	}
	h.timer.Reset(d, "heartbeats", "resetExpiryTimerWithLock")
}

// filterCandidates rescues expiry candidates whose database heartbeat
// has advanced since the last observation, proving they are still alive
// despite missed pubsub heartbeats.
func (h *heartbeats) filterCandidates(candidates map[uuid.UUID]time.Duration, dbHeartbeats map[uuid.UUID]time.Time, oldDBHeartbeats map[uuid.UUID]time.Time) {
	now := h.clock.Now()
	for id := range candidates {
		dbTime, inDB := dbHeartbeats[id]
		// If the coordinator is not in the database, it is not alive.
		if !inDB {
			continue
		}

		// If the database heartbeat has not advanced since the last observation, it is not alive.
		prevTime, hasPrev := oldDBHeartbeats[id]
		if !hasPrev || !dbTime.After(prevTime) {
			continue
		}
		// Require absolute freshness as well: tailnet_coordinators rows
		// persist for 24h after a coordinator dies, so a stale row with
		// one advancing window must not rescue a dead coordinator.
		if now.Sub(dbTime) >= MissedHeartbeats*HeartbeatPeriod {
			continue
		}
		h.logger.Info(h.ctx, "coordinator heartbeat recovered from database",
			slog.F("other_coordinator_id", id),
			slog.F("db_heartbeat_at", dbTime),
		)
		h.coordinators[id] = now
		delete(candidates, id)
	}
}

// discoverCoordinators finds coordinators present in the database that
// have never been seen via pubsub. Uses a two-observation pattern: the
// first sighting stores a baseline in lastDBHeartbeat, and only on the
// second observation with an advanced heartbeat_at is the coordinator
// added.
func (h *heartbeats) discoverCoordinators(dbMap map[uuid.UUID]time.Time, prevDBHeartbeats map[uuid.UUID]time.Time) filterUpdate {
	now := h.clock.Now()
	var discovered []uuid.UUID
	for id, dbTime := range dbMap {
		if id == h.self {
			continue
		}
		if _, known := h.coordinators[id]; known {
			continue
		}
		prevTime, hasPrev := prevDBHeartbeats[id]
		if !hasPrev {
			h.logger.Debug(h.ctx, "recorded baseline for unknown coordinator",
				slog.F("other_coordinator_id", id),
				slog.F("db_heartbeat_at", dbTime),
			)
			continue
		}
		if !dbTime.After(prevTime) {
			continue
		}
		// Require absolute freshness: prevent discovery of dead
		// coordinators whose rows haven't been cleaned up yet (24h
		// retention on tailnet_coordinators).
		if now.Sub(dbTime) >= MissedHeartbeats*HeartbeatPeriod {
			continue
		}
		h.logger.Info(h.ctx, "discovered coordinator from database",
			slog.F("other_coordinator_id", id),
			slog.F("db_heartbeat_at", dbTime),
		)
		h.coordinators[id] = now
		discovered = append(discovered, id)
	}
	if len(discovered) == 0 {
		return filterUpdateNone
	}
	// If any discovered coordinator was previously expired, mappers
	// need a full reset to re-evaluate peers marked LOST.
	needsReset := false
	for _, id := range discovered {
		if _, ok := h.expiredCoordinators[id]; ok {
			delete(h.expiredCoordinators, id)
			needsReset = true
		}
	}
	if needsReset {
		return filterUpdateReset
	}
	return filterUpdateUpdated
}

// cleanupStaleEntries removes tracking state for coordinators that
// are no longer active in memory or present in the database. This
// prevents lastDBHeartbeat and expiredCoordinators from growing
// monotonically as coordinators come and go.
func (h *heartbeats) cleanupStaleEntries(dbMap map[uuid.UUID]time.Time) {
	isStale := func(id uuid.UUID) bool {
		if _, inCoords := h.coordinators[id]; inCoords {
			return false
		}
		_, inDB := dbMap[id]
		return !inDB
	}
	maps.DeleteFunc(h.lastDBHeartbeat, func(id uuid.UUID, _ time.Time) bool {
		return isStale(id)
	})
	maps.DeleteFunc(h.expiredCoordinators, func(id uuid.UUID, _ struct{}) bool {
		return isStale(id)
	})
}

func (h *heartbeats) checkExpiry() {
	if h.ctx.Err() != nil {
		return
	}
	h.logger.Debug(h.ctx, "checking heartbeat expiry")

	// Query the database BEFORE acquiring the lock to avoid blocking
	// heartbeat processing during the DB round-trip.
	dbCoords, err := h.store.GetAllTailnetCoordinators(h.ctx)

	h.lock.Lock()
	defer h.lock.Unlock()

	// Collect candidates whose pubsub heartbeats have expired.
	now := h.clock.Now()
	threshold := MissedHeartbeats * HeartbeatPeriod
	candidates := make(map[uuid.UUID]time.Duration)
	for id, t := range h.coordinators {
		lastHB := now.Sub(t)
		h.logger.Debug(h.ctx, "last heartbeat from coordinator",
			slog.F("other_coordinator_id", id),
			slog.F("last_heartbeat", lastHB),
		)
		if lastHB >= threshold {
			candidates[id] = lastHB
		}
	}

	if err != nil {
		h.logger.Warn(h.ctx, "failed to query coordinators from database for heartbeat fallback", slog.Error(err))
	} else {
		dbHeartbeats := make(map[uuid.UUID]time.Time, len(dbCoords))
		for _, c := range dbCoords {
			dbHeartbeats[c.ID] = c.HeartbeatAt
		}

		// Snapshot previous DB heartbeats before updating, so helpers
		// can compare old vs new to detect advancement.
		oldDBHeartbeats := make(map[uuid.UUID]time.Time, len(h.lastDBHeartbeat))
		maps.Copy(oldDBHeartbeats, h.lastDBHeartbeat)

		// Update all baselines from the fresh DB snapshot.
		maps.Copy(h.lastDBHeartbeat, dbHeartbeats)

		// Remove candidates that are still alive from the database.
		h.filterCandidates(candidates, dbHeartbeats, oldDBHeartbeats)
		// Discover coordinators that are alive in the database but not in the memory.
		if filter := h.discoverCoordinators(dbHeartbeats, oldDBHeartbeats); filter != filterUpdateNone {
			go func() {
				_ = agpl.SendCtx(h.ctx, h.update, hbUpdate{filter: filter})
			}()
		}
		h.cleanupStaleEntries(dbHeartbeats)
	}

	// Expire remaining candidates.
	expired := false
	for id, lastHB := range candidates {
		expired = true
		delete(h.coordinators, id)
		h.expiredCoordinators[id] = struct{}{}
		h.logger.Info(h.ctx, "coordinator failed heartbeat check",
			slog.F("other_coordinator_id", id),
			slog.F("last_heartbeat", lastHB),
		)
	}
	if expired {
		go func() {
			_ = agpl.SendCtx(h.ctx, h.update, hbUpdate{filter: filterUpdateUpdated})
		}()
	}

	h.resetExpiryTimerWithLock()
}

func (h *heartbeats) sendBeats() {
	defer h.wg.Done()
	// send an initial heartbeat so that other coordinators can start using our bindings right away.
	h.sendBeat()
	close(h.firstHeartbeat) // signal binder it can start writing
	tkr := h.clock.TickerFunc(h.ctx, HeartbeatPeriod, func() error {
		h.sendBeat()
		return nil
	}, "heartbeats", "sendBeats")
	err := tkr.Wait()
	h.logger.Debug(h.ctx, "ending heartbeats", slog.Error(err))
}

func (h *heartbeats) sendBeat() {
	_, err := h.store.UpsertTailnetCoordinator(h.ctx, h.self)
	if database.IsQueryCanceledError(err) {
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
	if h.ctx.Err() != nil {
		return
	}
	publishCoordinatorHeartbeat(h.ctx, h.pubsub, h.logger, h.self)
	if h.failedHeartbeats >= 3 {
		h.logger.Info(h.ctx, "coordinator sent heartbeat and is healthy")
		_ = agpl.SendCtx(h.ctx, h.update, hbUpdate{health: healthUpdateHealthy})
	}
	h.failedHeartbeats = 0
}

func (h *heartbeats) cleanupLoop() {
	defer h.wg.Done()
	h.cleanup()
	tkr := h.clock.TickerFunc(h.ctx, cleanupPeriod, func() error {
		h.cleanup()
		return nil
	}, "heartbeats", "cleanupLoop")
	err := tkr.Wait()
	h.logger.Debug(h.ctx, "ending cleanupLoop", slog.Error(err))
}

// cleanup issues a DB command to clean out any old expired coordinators or lost peer state.  The
// cleanup is idempotent, so no need to synchronize with other coordinators.
func (h *heartbeats) cleanup() {
	// the records we are attempting to clean up do no serious harm other than
	// accumulating in the tables, so we don't bother retrying if it fails.
	err := h.store.CleanTailnetCoordinators(h.ctx)
	if err != nil && !database.IsQueryCanceledError(err) {
		h.logger.Error(h.ctx, "failed to cleanup old coordinators", slog.Error(err))
	}
	deletedPeers, err := h.store.CleanTailnetLostPeers(h.ctx)
	if err != nil && !database.IsQueryCanceledError(err) {
		h.logger.Error(h.ctx, "failed to cleanup lost peers", slog.Error(err))
	}
	for _, row := range deletedPeers {
		// row.CoordinatorID is the expired coordinator's ID, NOT h.self.
		publishPeerUpdate(h.ctx, h.pubsub, h.logger,
			row.ID, row.CoordinatorID,
			tailnetStatusToProto(row.Status),
			row.Node, row.UpdatedAt)
	}
	deletedTunnels, err := h.store.CleanTailnetTunnels(h.ctx)
	if err != nil && !database.IsQueryCanceledError(err) {
		h.logger.Error(h.ctx, "failed to cleanup abandoned tunnels", slog.Error(err))
	}
	for _, tun := range deletedTunnels {
		// tun.CoordinatorID is the expired coordinator's ID, NOT h.self.
		publishTunnelUpdate(h.ctx, h.pubsub, h.logger,
			tun.SrcID, tun.DstID, tun.CoordinatorID,
			proto.TailnetTunnelUpdate_OP_DELETE)
	}
	h.logger.Debug(h.ctx, "completed cleanup")
}
