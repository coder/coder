package tailnet

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/netip"
	"strings"
	"sync"
	"time"

	"github.com/cenkalti/backoff/v4"
	"github.com/google/uuid"
	"golang.org/x/exp/slices"
	"golang.org/x/xerrors"

	"cdr.dev/slog"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/database/pubsub"
	"github.com/coder/coder/v2/coderd/rbac"
	"github.com/coder/coder/v2/coderd/util/slice"
	agpl "github.com/coder/coder/v2/tailnet"
)

const (
	EventHeartbeats      = "tailnet_coordinator_heartbeat"
	eventClientUpdate    = "tailnet_client_update"
	eventAgentUpdate     = "tailnet_agent_update"
	HeartbeatPeriod      = time.Second * 2
	MissedHeartbeats     = 3
	numQuerierWorkers    = 10
	numBinderWorkers     = 10
	numSubscriberWorkers = 10
	dbMaxBackoff         = 10 * time.Second
	cleanupPeriod        = time.Hour
)

// TODO: add subscriber to this graphic
// pgCoord is a postgres-backed coordinator
//
//	┌────────┐       ┌────────┐        ┌───────┐
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
// each incoming connection (websocket) from a client or agent is wrapped in a connIO which handles reading & writing
// from it.  Node updates from a connIO are sent to the binder, which writes them to the database.Store.  The querier
// is responsible for querying the store for the nodes the connection needs (e.g. for a client, the corresponding
// agent).  The querier receives pubsub notifications about changes, which trigger queries for the latest state.
//
// The querier also sends the coordinator's heartbeat, and monitors the heartbeats of other coordinators.  When
// heartbeats cease for a coordinator, it stops using any nodes discovered from that coordinator and pushes an update
// to affected connIOs.
//
// This package uses the term "binding" to mean the act of registering an association between some connection (client
// or agent) and an agpl.Node.  It uses the term "mapping" to mean the act of determining the nodes that the connection
// needs to receive (e.g. for a client, the node bound to the corresponding agent, or for an agent, the nodes bound to
// all clients of the agent).
type pgCoord struct {
	ctx    context.Context
	logger slog.Logger
	pubsub pubsub.Pubsub
	store  database.Store

	bindings         chan binding
	newConnections   chan agpl.Queue
	closeConnections chan agpl.Queue
	subscriberCh     chan subscribe
	querierSubCh     chan subscribe
	id               uuid.UUID

	cancel    context.CancelFunc
	closeOnce sync.Once
	closed    chan struct{}

	binder     *binder
	subscriber *subscriber
	querier    *querier
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
	ctx, cancel := context.WithCancel(dbauthz.As(ctx, pgCoordSubject))
	id := uuid.New()
	logger = logger.Named("pgcoord").With(slog.F("coordinator_id", id))
	bCh := make(chan binding)
	// used for opening connections
	cCh := make(chan agpl.Queue)
	// used for closing connections
	ccCh := make(chan agpl.Queue)
	// for communicating subscriptions with the subscriber
	sCh := make(chan subscribe)
	// for communicating subscriptions with the querier
	qsCh := make(chan subscribe)
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
		subscriber:       newSubscriber(ctx, logger, id, store, sCh, fHB),
		subscriberCh:     sCh,
		querierSubCh:     qsCh,
		id:               id,
		querier:          newQuerier(ctx, logger, id, ps, store, id, cCh, ccCh, qsCh, numQuerierWorkers, fHB),
		closed:           make(chan struct{}),
	}
	logger.Info(ctx, "starting coordinator")
	return c, nil
}

// This is copied from codersdk because importing it here would cause an import
// cycle. This is just temporary until wsconncache is phased out.
var legacyAgentIP = netip.MustParseAddr("fd7a:115c:a1e0:49d6:b259:b7ac:b1b2:48f4")

func (c *pgCoord) ServeMultiAgent(id uuid.UUID) agpl.MultiAgentConn {
	ma := (&agpl.MultiAgent{
		ID: id,
		AgentIsLegacyFunc: func(agentID uuid.UUID) bool {
			if n := c.Node(agentID); n == nil {
				// If we don't have the node at all assume it's legacy for
				// safety.
				return true
			} else if len(n.Addresses) > 0 && n.Addresses[0].Addr() == legacyAgentIP {
				// An agent is determined to be "legacy" if it's first IP is the
				// legacy IP. Agents with only the legacy IP aren't compatible
				// with single_tailnet and must be routed through wsconncache.
				return true
			} else {
				return false
			}
		},
		OnSubscribe: func(enq agpl.Queue, agent uuid.UUID) (*agpl.Node, error) {
			err := c.addSubscription(enq, agent)
			return c.Node(agent), err
		},
		OnUnsubscribe: c.removeSubscription,
		OnNodeUpdate: func(id uuid.UUID, node *agpl.Node) error {
			return sendCtx(c.ctx, c.bindings, binding{
				bKey: bKey{id, agpl.QueueKindClient},
				node: node,
			})
		},
		OnRemove: func(enq agpl.Queue) {
			_ = sendCtx(c.ctx, c.bindings, binding{
				bKey: bKey{
					id:   enq.UniqueID(),
					kind: enq.Kind(),
				},
			})
			_ = sendCtx(c.ctx, c.subscriberCh, subscribe{
				sKey:   sKey{clientID: id},
				q:      enq,
				active: false,
			})
			_ = sendCtx(c.ctx, c.closeConnections, enq)
		},
	}).Init()

	if err := sendCtx(c.ctx, c.newConnections, agpl.Queue(ma)); err != nil {
		// If we can't successfully send the multiagent, that means the
		// coordinator is shutting down. In this case, just return a closed
		// multiagent.
		ma.CoordinatorClose()
	}

	return ma
}

func (c *pgCoord) addSubscription(q agpl.Queue, agentID uuid.UUID) error {
	sub := subscribe{
		sKey: sKey{
			clientID: q.UniqueID(),
			agentID:  agentID,
		},
		q:      q,
		active: true,
	}
	if err := sendCtx(c.ctx, c.subscriberCh, sub); err != nil {
		return err
	}
	if err := sendCtx(c.ctx, c.querierSubCh, sub); err != nil {
		// There's no need to clean up the sub sent to the subscriber if this
		// fails, since it means the entire coordinator is being torn down.
		return err
	}

	return nil
}

func (c *pgCoord) removeSubscription(q agpl.Queue, agentID uuid.UUID) error {
	sub := subscribe{
		sKey: sKey{
			clientID: q.UniqueID(),
			agentID:  agentID,
		},
		q:      q,
		active: false,
	}
	if err := sendCtx(c.ctx, c.subscriberCh, sub); err != nil {
		return err
	}
	if err := sendCtx(c.ctx, c.querierSubCh, sub); err != nil {
		// There's no need to clean up the sub sent to the subscriber if this
		// fails, since it means the entire coordinator is being torn down.
		return err
	}

	return nil
}

func (c *pgCoord) Node(id uuid.UUID) *agpl.Node {
	// In production, we only ever get this request for an agent.
	// We're going to directly query the database, since we would only have the agent mapping stored locally if we had
	// a client of that agent connected, which isn't always the case.
	mappings, err := c.querier.queryAgent(id)
	if err != nil {
		c.logger.Error(c.ctx, "failed to query agents", slog.Error(err))
	}
	mappings = c.querier.heartbeats.filter(mappings)
	var bestT time.Time
	var bestN *agpl.Node
	for _, m := range mappings {
		if m.updatedAt.After(bestT) {
			bestN = m.node
			bestT = m.updatedAt
		}
	}
	return bestN
}

func (c *pgCoord) ServeClient(conn net.Conn, id uuid.UUID, agent uuid.UUID) error {
	defer func() {
		err := conn.Close()
		if err != nil {
			c.logger.Debug(c.ctx, "closing client connection",
				slog.F("client_id", id),
				slog.F("agent_id", agent),
				slog.Error(err))
		}
	}()

	cIO := newConnIO(c.ctx, c.logger, c.bindings, conn, id, id.String(), agpl.QueueKindClient)
	if err := sendCtx(c.ctx, c.newConnections, agpl.Queue(cIO)); err != nil {
		// can only be a context error, no need to log here.
		return err
	}
	defer func() { _ = sendCtx(c.ctx, c.closeConnections, agpl.Queue(cIO)) }()

	if err := c.addSubscription(cIO, agent); err != nil {
		return err
	}
	defer func() { _ = c.removeSubscription(cIO, agent) }()

	<-cIO.ctx.Done()
	return nil
}

func (c *pgCoord) ServeAgent(conn net.Conn, id uuid.UUID, name string) error {
	defer func() {
		err := conn.Close()
		if err != nil {
			c.logger.Debug(c.ctx, "closing agent connection",
				slog.F("agent_id", id),
				slog.Error(err))
		}
	}()
	logger := c.logger.With(slog.F("name", name))
	cIO := newConnIO(c.ctx, logger, c.bindings, conn, id, name, agpl.QueueKindAgent)
	if err := sendCtx(c.ctx, c.newConnections, agpl.Queue(cIO)); err != nil {
		// can only be a context error, no need to log here.
		return err
	}
	defer func() { _ = sendCtx(c.ctx, c.closeConnections, agpl.Queue(cIO)) }()

	<-cIO.ctx.Done()
	return nil
}

func (c *pgCoord) Close() error {
	c.logger.Info(c.ctx, "closing coordinator")
	c.cancel()
	c.closeOnce.Do(func() { close(c.closed) })
	return nil
}

func sendCtx[A any](ctx context.Context, c chan<- A, a A) (err error) {
	select {
	case <-ctx.Done():
		return ctx.Err()
	case c <- a:
		return nil
	}
}

type sKey struct {
	clientID uuid.UUID
	agentID  uuid.UUID
}

type subscribe struct {
	sKey

	q agpl.Queue
	// whether the subscription should be active. if true, the subscription is
	// added. if false, the subscription is removed.
	active bool
}

type subscriber struct {
	ctx           context.Context
	logger        slog.Logger
	coordinatorID uuid.UUID
	store         database.Store
	subscriptions <-chan subscribe

	mu sync.Mutex
	// map[clientID]map[agentID]subscribe
	latest map[uuid.UUID]map[uuid.UUID]subscribe
	workQ  *workQ[sKey]
}

func newSubscriber(ctx context.Context,
	logger slog.Logger,
	id uuid.UUID,
	store database.Store,
	subscriptions <-chan subscribe,
	startWorkers <-chan struct{},
) *subscriber {
	s := &subscriber{
		ctx:           ctx,
		logger:        logger,
		coordinatorID: id,
		store:         store,
		subscriptions: subscriptions,
		latest:        make(map[uuid.UUID]map[uuid.UUID]subscribe),
		workQ:         newWorkQ[sKey](ctx),
	}
	go s.handleSubscriptions()
	go func() {
		<-startWorkers
		for i := 0; i < numSubscriberWorkers; i++ {
			go s.worker()
		}
	}()
	return s
}

func (s *subscriber) handleSubscriptions() {
	for {
		select {
		case <-s.ctx.Done():
			s.logger.Debug(s.ctx, "subscriber exiting", slog.Error(s.ctx.Err()))
			return
		case sub := <-s.subscriptions:
			s.storeSubscription(sub)
			s.workQ.enqueue(sub.sKey)
		}
	}
}

func (s *subscriber) worker() {
	eb := backoff.NewExponentialBackOff()
	eb.MaxElapsedTime = 0 // retry indefinitely
	eb.MaxInterval = dbMaxBackoff
	bkoff := backoff.WithContext(eb, s.ctx)
	for {
		bk, err := s.workQ.acquire()
		if err != nil {
			// context expired
			return
		}
		err = backoff.Retry(func() error {
			bnd := s.retrieveSubscription(bk)
			return s.writeOne(bnd)
		}, bkoff)
		if err != nil {
			bkoff.Reset()
		}
		s.workQ.done(bk)
	}
}

func (s *subscriber) storeSubscription(sub subscribe) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if sub.active {
		if _, ok := s.latest[sub.clientID]; !ok {
			s.latest[sub.clientID] = map[uuid.UUID]subscribe{}
		}
		s.latest[sub.clientID][sub.agentID] = sub
	} else {
		// If the agentID is nil, clean up all of the clients subscriptions.
		if sub.agentID == uuid.Nil {
			delete(s.latest, sub.clientID)
		} else {
			delete(s.latest[sub.clientID], sub.agentID)
			// clean up the subscription map if all the subscriptions are gone.
			if len(s.latest[sub.clientID]) == 0 {
				delete(s.latest, sub.clientID)
			}
		}
	}
}

// retrieveBinding gets the latest binding for a key.
func (s *subscriber) retrieveSubscription(sk sKey) subscribe {
	s.mu.Lock()
	defer s.mu.Unlock()
	agents, ok := s.latest[sk.clientID]
	if !ok {
		return subscribe{
			sKey:   sk,
			active: false,
		}
	}

	sub, ok := agents[sk.agentID]
	if !ok {
		return subscribe{
			sKey:   sk,
			active: false,
		}
	}

	return sub
}

func (s *subscriber) writeOne(sub subscribe) error {
	var err error
	switch {
	case sub.agentID == uuid.Nil:
		err = s.store.DeleteAllTailnetClientSubscriptions(s.ctx, database.DeleteAllTailnetClientSubscriptionsParams{
			ClientID:      sub.clientID,
			CoordinatorID: s.coordinatorID,
		})
		s.logger.Debug(s.ctx, "deleted all client subscriptions",
			slog.F("client_id", sub.clientID),
			slog.Error(err),
		)
	case sub.active:
		err = s.store.UpsertTailnetClientSubscription(s.ctx, database.UpsertTailnetClientSubscriptionParams{
			ClientID:      sub.clientID,
			CoordinatorID: s.coordinatorID,
			AgentID:       sub.agentID,
		})
		s.logger.Debug(s.ctx, "upserted client subscription",
			slog.F("client_id", sub.clientID),
			slog.F("agent_id", sub.agentID),
			slog.Error(err),
		)
	case !sub.active:
		err = s.store.DeleteTailnetClientSubscription(s.ctx, database.DeleteTailnetClientSubscriptionParams{
			ClientID:      sub.clientID,
			CoordinatorID: s.coordinatorID,
			AgentID:       sub.agentID,
		})
		s.logger.Debug(s.ctx, "deleted client subscription",
			slog.F("client_id", sub.clientID),
			slog.F("agent_id", sub.agentID),
			slog.Error(err),
		)
	default:
		panic("unreachable")
	}
	if err != nil && !database.IsQueryCanceledError(err) {
		s.logger.Error(s.ctx, "write subscription to database",
			slog.F("client_id", sub.clientID),
			slog.F("agent_id", sub.agentID),
			slog.F("active", sub.active),
			slog.Error(err))
	}
	return err
}

// bKey, or "binding key" identifies a client or agent in a binding. Agents and
// clients are differentiated by the kind field.
type bKey struct {
	id   uuid.UUID
	kind agpl.QueueKind
}

// binding represents an association between a client or agent and a Node.
type binding struct {
	bKey
	node *agpl.Node
}

func (b *binding) isAgent() bool  { return b.kind == agpl.QueueKindAgent }
func (b *binding) isClient() bool { return b.kind == agpl.QueueKindClient }

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
	var nodeRaw json.RawMessage
	var err error
	if bnd.node != nil {
		nodeRaw, err = json.Marshal(*bnd.node)
		if err != nil {
			// this is very bad news, but it should never happen because the node was Unmarshalled by this process
			// earlier.
			b.logger.Error(b.ctx, "failed to marshal node", slog.Error(err))
			return err
		}
	}

	switch {
	case bnd.isAgent() && len(nodeRaw) > 0:
		_, err = b.store.UpsertTailnetAgent(b.ctx, database.UpsertTailnetAgentParams{
			ID:            bnd.id,
			CoordinatorID: b.coordinatorID,
			Node:          nodeRaw,
		})
		b.logger.Debug(b.ctx, "upserted agent binding",
			slog.F("agent_id", bnd.id), slog.F("node", nodeRaw), slog.Error(err))
	case bnd.isAgent() && len(nodeRaw) == 0:
		_, err = b.store.DeleteTailnetAgent(b.ctx, database.DeleteTailnetAgentParams{
			ID:            bnd.id,
			CoordinatorID: b.coordinatorID,
		})
		b.logger.Debug(b.ctx, "deleted agent binding",
			slog.F("agent_id", bnd.id), slog.Error(err))
		if xerrors.Is(err, sql.ErrNoRows) {
			// treat deletes as idempotent
			err = nil
		}
	case bnd.isClient() && len(nodeRaw) > 0:
		_, err = b.store.UpsertTailnetClient(b.ctx, database.UpsertTailnetClientParams{
			ID:            bnd.id,
			CoordinatorID: b.coordinatorID,
			Node:          nodeRaw,
		})
		b.logger.Debug(b.ctx, "upserted client binding",
			slog.F("client_id", bnd.id),
			slog.F("node", nodeRaw), slog.Error(err))
	case bnd.isClient() && len(nodeRaw) == 0:
		_, err = b.store.DeleteTailnetClient(b.ctx, database.DeleteTailnetClientParams{
			ID:            bnd.id,
			CoordinatorID: b.coordinatorID,
		})
		b.logger.Debug(b.ctx, "deleted client binding",
			slog.F("client_id", bnd.id))
		if xerrors.Is(err, sql.ErrNoRows) {
			// treat deletes as idempotent
			err = nil
		}
	default:
		panic("unhittable")
	}
	if err != nil && !database.IsQueryCanceledError(err) {
		b.logger.Error(b.ctx, "failed to write binding to database",
			slog.F("binding_id", bnd.id),
			slog.F("kind", bnd.kind),
			slog.F("node", string(nodeRaw)),
			slog.Error(err))
	}
	return err
}

// storeBinding stores the latest binding, where we interpret node == nil as removing the binding. This keeps the map
// from growing without bound.
func (b *binder) storeBinding(bnd binding) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if bnd.node != nil {
		b.latest[bnd.bKey] = bnd
	} else {
		// nil node is interpreted as removing binding
		delete(b.latest, bnd.bKey)
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
		}
	}
	return bnd
}

// mapper tracks a single client or agent ID, and fans out updates to that ID->node mapping to every local connection
// that needs it.
type mapper struct {
	ctx    context.Context
	logger slog.Logger

	add chan agpl.Queue
	del chan agpl.Queue

	// reads from this channel trigger sending latest nodes to
	// all connections.  It is used when coordinators are added
	// or removed
	update chan struct{}

	mappings chan []mapping

	conns  map[bKey]agpl.Queue
	latest []mapping

	heartbeats *heartbeats
}

func newMapper(ctx context.Context, logger slog.Logger, mk mKey, h *heartbeats) *mapper {
	logger = logger.With(
		slog.F("agent_id", mk.agent),
		slog.F("kind", mk.kind),
	)
	m := &mapper{
		ctx:        ctx,
		logger:     logger,
		add:        make(chan agpl.Queue),
		del:        make(chan agpl.Queue),
		update:     make(chan struct{}),
		conns:      make(map[bKey]agpl.Queue),
		mappings:   make(chan []mapping),
		heartbeats: h,
	}
	go m.run()
	return m
}

func (m *mapper) run() {
	for {
		select {
		case <-m.ctx.Done():
			return
		case c := <-m.add:
			m.conns[bKey{id: c.UniqueID(), kind: c.Kind()}] = c
			nodes := m.mappingsToNodes(m.latest)
			if len(nodes) == 0 {
				m.logger.Debug(m.ctx, "skipping 0 length node update")
				continue
			}
			if err := c.Enqueue(nodes); err != nil {
				m.logger.Error(m.ctx, "failed to enqueue node update", slog.Error(err))
			}
		case c := <-m.del:
			delete(m.conns, bKey{id: c.UniqueID(), kind: c.Kind()})
		case mappings := <-m.mappings:
			m.latest = mappings
			nodes := m.mappingsToNodes(mappings)
			if len(nodes) == 0 {
				m.logger.Debug(m.ctx, "skipping 0 length node update")
				continue
			}
			for _, conn := range m.conns {
				if err := conn.Enqueue(nodes); err != nil {
					m.logger.Error(m.ctx, "failed to enqueue node update", slog.Error(err))
				}
			}
		case <-m.update:
			nodes := m.mappingsToNodes(m.latest)
			if len(nodes) == 0 {
				m.logger.Debug(m.ctx, "skipping 0 length node update")
				continue
			}
			for _, conn := range m.conns {
				if err := conn.Enqueue(nodes); err != nil {
					m.logger.Error(m.ctx, "failed to enqueue triggered node update", slog.Error(err))
				}
			}
		}
	}
}

// mappingsToNodes takes a set of mappings and resolves the best set of nodes.  We may get several mappings for a
// particular connection, from different coordinators in the distributed system.  Furthermore, some coordinators
// might be considered invalid on account of missing heartbeats.  We take the most recent mapping from a valid
// coordinator as the "best" mapping.
func (m *mapper) mappingsToNodes(mappings []mapping) []*agpl.Node {
	mappings = m.heartbeats.filter(mappings)
	best := make(map[bKey]mapping, len(mappings))
	for _, m := range mappings {
		var bk bKey
		if m.client == uuid.Nil {
			bk = bKey{id: m.agent, kind: agpl.QueueKindAgent}
		} else {
			bk = bKey{id: m.client, kind: agpl.QueueKindClient}
		}

		bestM, ok := best[bk]
		if !ok || m.updatedAt.After(bestM.updatedAt) {
			best[bk] = m
		}
	}
	nodes := make([]*agpl.Node, 0, len(best))
	for _, m := range best {
		nodes = append(nodes, m.node)
	}
	return nodes
}

// querier is responsible for monitoring pubsub notifications and querying the database for the mappings that all
// connected clients and agents need.  It also checks heartbeats and withdraws mappings from coordinators that have
// failed heartbeats.
type querier struct {
	ctx           context.Context
	logger        slog.Logger
	coordinatorID uuid.UUID
	pubsub        pubsub.Pubsub
	store         database.Store

	newConnections   chan agpl.Queue
	closeConnections chan agpl.Queue
	subscriptions    chan subscribe

	workQ *workQ[mKey]

	heartbeats *heartbeats
	updates    <-chan hbUpdate

	mu      sync.Mutex
	mappers map[mKey]*countedMapper
	conns   map[uuid.UUID]agpl.Queue
	// clientSubscriptions maps client ids to the agent ids they're subscribed to.
	// map[client_id]map[agent_id]
	clientSubscriptions map[uuid.UUID]map[uuid.UUID]struct{}
	healthy             bool
}

type countedMapper struct {
	*mapper
	count  int
	cancel context.CancelFunc
}

func newQuerier(ctx context.Context,
	logger slog.Logger,
	coordinatorID uuid.UUID,
	ps pubsub.Pubsub,
	store database.Store,
	self uuid.UUID,
	newConnections chan agpl.Queue,
	closeConnections chan agpl.Queue,
	subscriptions chan subscribe,
	numWorkers int,
	firstHeartbeat chan struct{},
) *querier {
	updates := make(chan hbUpdate)
	q := &querier{
		ctx:                 ctx,
		logger:              logger.Named("querier"),
		coordinatorID:       coordinatorID,
		pubsub:              ps,
		store:               store,
		newConnections:      newConnections,
		closeConnections:    closeConnections,
		subscriptions:       subscriptions,
		workQ:               newWorkQ[mKey](ctx),
		heartbeats:          newHeartbeats(ctx, logger, ps, store, self, updates, firstHeartbeat),
		mappers:             make(map[mKey]*countedMapper),
		conns:               make(map[uuid.UUID]agpl.Queue),
		updates:             updates,
		clientSubscriptions: make(map[uuid.UUID]map[uuid.UUID]struct{}),
		healthy:             true, // assume we start healthy
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
			switch c.Kind() {
			case agpl.QueueKindAgent:
				q.newAgentConn(c)
			case agpl.QueueKindClient:
				q.newClientConn(c)
			default:
				panic(fmt.Sprint("unreachable: invalid queue kind ", c.Kind()))
			}

		case c := <-q.closeConnections:
			q.cleanupConn(c)

		case sub := <-q.subscriptions:
			if sub.active {
				q.newClientSubscription(sub.q, sub.agentID)
			} else {
				q.removeClientSubscription(sub.q, sub.agentID)
			}
		}
	}
}

func (q *querier) newAgentConn(c agpl.Queue) {
	q.mu.Lock()
	defer q.mu.Unlock()
	if !q.healthy {
		err := c.Close()
		q.logger.Info(q.ctx, "closed incoming connection while unhealthy",
			slog.Error(err),
			slog.F("agent_id", c.UniqueID()),
		)
		return
	}
	mk := mKey{
		agent: c.UniqueID(),
		kind:  c.Kind(),
	}
	cm, ok := q.mappers[mk]
	if !ok {
		ctx, cancel := context.WithCancel(q.ctx)
		mpr := newMapper(ctx, q.logger, mk, q.heartbeats)
		cm = &countedMapper{
			mapper: mpr,
			count:  0,
			cancel: cancel,
		}
		q.mappers[mk] = cm
		// we don't have any mapping state for this key yet
		q.workQ.enqueue(mk)
	}
	if err := sendCtx(cm.ctx, cm.add, c); err != nil {
		return
	}
	cm.count++
	q.conns[c.UniqueID()] = c
}

func (q *querier) newClientSubscription(c agpl.Queue, agentID uuid.UUID) {
	q.mu.Lock()
	defer q.mu.Unlock()

	if _, ok := q.clientSubscriptions[c.UniqueID()]; !ok {
		q.clientSubscriptions[c.UniqueID()] = map[uuid.UUID]struct{}{}
	}

	mk := mKey{
		agent: agentID,
		kind:  agpl.QueueKindClient,
	}
	cm, ok := q.mappers[mk]
	if !ok {
		ctx, cancel := context.WithCancel(q.ctx)
		mpr := newMapper(ctx, q.logger, mk, q.heartbeats)
		cm = &countedMapper{
			mapper: mpr,
			count:  0,
			cancel: cancel,
		}
		q.mappers[mk] = cm
		// we don't have any mapping state for this key yet
		q.workQ.enqueue(mk)
	}
	if err := sendCtx(cm.ctx, cm.add, c); err != nil {
		return
	}
	q.clientSubscriptions[c.UniqueID()][agentID] = struct{}{}
	cm.count++
}

func (q *querier) removeClientSubscription(c agpl.Queue, agentID uuid.UUID) {
	q.mu.Lock()
	defer q.mu.Unlock()

	// Allow duplicate unsubscribes. It's possible for cleanupConn to race with
	// an external call to removeClientSubscription, so we just ensure the
	// client subscription exists before attempting to remove it.
	if _, ok := q.clientSubscriptions[c.UniqueID()][agentID]; !ok {
		return
	}

	mk := mKey{
		agent: agentID,
		kind:  agpl.QueueKindClient,
	}
	cm := q.mappers[mk]
	if err := sendCtx(cm.ctx, cm.del, c); err != nil {
		return
	}
	delete(q.clientSubscriptions[c.UniqueID()], agentID)
	cm.count--
	if cm.count == 0 {
		cm.cancel()
		delete(q.mappers, mk)
	}
	if len(q.clientSubscriptions[c.UniqueID()]) == 0 {
		delete(q.clientSubscriptions, c.UniqueID())
	}
}

func (q *querier) newClientConn(c agpl.Queue) {
	q.mu.Lock()
	defer q.mu.Unlock()
	if !q.healthy {
		err := c.Close()
		q.logger.Info(q.ctx, "closed incoming connection while unhealthy",
			slog.Error(err),
			slog.F("client_id", c.UniqueID()),
		)
		return
	}

	q.conns[c.UniqueID()] = c
}

func (q *querier) cleanupConn(c agpl.Queue) {
	q.mu.Lock()
	defer q.mu.Unlock()
	delete(q.conns, c.UniqueID())

	// Iterate over all subscriptions and remove them from the mappers.
	for agentID := range q.clientSubscriptions[c.UniqueID()] {
		mk := mKey{
			agent: agentID,
			kind:  c.Kind(),
		}
		cm := q.mappers[mk]
		if err := sendCtx(cm.ctx, cm.del, c); err != nil {
			continue
		}
		cm.count--
		if cm.count == 0 {
			cm.cancel()
			delete(q.mappers, mk)
		}
	}
	delete(q.clientSubscriptions, c.UniqueID())

	mk := mKey{
		agent: c.UniqueID(),
		kind:  c.Kind(),
	}
	cm, ok := q.mappers[mk]
	if !ok {
		return
	}

	if err := sendCtx(cm.ctx, cm.del, c); err != nil {
		return
	}
	cm.count--
	if cm.count == 0 {
		cm.cancel()
		delete(q.mappers, mk)
	}
}

func (q *querier) worker() {
	eb := backoff.NewExponentialBackOff()
	eb.MaxElapsedTime = 0 // retry indefinitely
	eb.MaxInterval = dbMaxBackoff
	bkoff := backoff.WithContext(eb, q.ctx)
	for {
		mk, err := q.workQ.acquire()
		if err != nil {
			// context expired
			return
		}
		err = backoff.Retry(func() error {
			return q.query(mk)
		}, bkoff)
		if err != nil {
			bkoff.Reset()
		}
		q.workQ.done(mk)
	}
}

func (q *querier) query(mk mKey) error {
	var mappings []mapping
	var err error
	// If the mapping is an agent, query all of its clients.
	if mk.kind == agpl.QueueKindAgent {
		mappings, err = q.queryClientsOfAgent(mk.agent)
		if err != nil {
			return err
		}
	} else {
		// The mapping is for clients subscribed to the agent. Query the agent
		// itself.
		mappings, err = q.queryAgent(mk.agent)
		if err != nil {
			return err
		}
	}
	q.mu.Lock()
	mpr, ok := q.mappers[mk]
	q.mu.Unlock()
	if !ok {
		q.logger.Debug(q.ctx, "query for missing mapper",
			slog.F("agent_id", mk.agent), slog.F("kind", mk.kind))
		return nil
	}
	q.logger.Debug(q.ctx, "sending mappings", slog.F("mapping_len", len(mappings)))
	mpr.mappings <- mappings
	return nil
}

func (q *querier) queryClientsOfAgent(agent uuid.UUID) ([]mapping, error) {
	clients, err := q.store.GetTailnetClientsForAgent(q.ctx, agent)
	q.logger.Debug(q.ctx, "queried clients of agent",
		slog.F("agent_id", agent), slog.F("num_clients", len(clients)), slog.Error(err))
	if err != nil {
		return nil, err
	}
	mappings := make([]mapping, 0, len(clients))
	for _, client := range clients {
		node := new(agpl.Node)
		err := json.Unmarshal(client.Node, node)
		if err != nil {
			q.logger.Error(q.ctx, "failed to unmarshal node", slog.Error(err))
			return nil, backoff.Permanent(err)
		}
		mappings = append(mappings, mapping{
			client:      client.ID,
			agent:       agent,
			coordinator: client.CoordinatorID,
			updatedAt:   client.UpdatedAt,
			node:        node,
		})
	}
	return mappings, nil
}

func (q *querier) queryAgent(agentID uuid.UUID) ([]mapping, error) {
	agents, err := q.store.GetTailnetAgents(q.ctx, agentID)
	q.logger.Debug(q.ctx, "queried agents",
		slog.F("agent_id", agentID), slog.F("num_agents", len(agents)), slog.Error(err))
	if err != nil {
		return nil, err
	}
	return q.agentsToMappings(agents)
}

func (q *querier) agentsToMappings(agents []database.TailnetAgent) ([]mapping, error) {
	slog.Helper()
	mappings := make([]mapping, 0, len(agents))
	for _, agent := range agents {
		node := new(agpl.Node)
		err := json.Unmarshal(agent.Node, node)
		if err != nil {
			q.logger.Error(q.ctx, "failed to unmarshal node", slog.Error(err))
			return nil, backoff.Permanent(err)
		}
		mappings = append(mappings, mapping{
			agent:       agent.ID,
			coordinator: agent.CoordinatorID,
			updatedAt:   agent.UpdatedAt,
			node:        node,
		})
	}
	return mappings, nil
}

// subscribe starts our subscriptions to client and agent updates in a new goroutine, and returns once we are subscribed
// or the querier context is canceled.
func (q *querier) subscribe() {
	subscribed := make(chan struct{})
	go func() {
		defer close(subscribed)
		eb := backoff.NewExponentialBackOff()
		eb.MaxElapsedTime = 0 // retry indefinitely
		eb.MaxInterval = dbMaxBackoff
		bkoff := backoff.WithContext(eb, q.ctx)
		var cancelClient context.CancelFunc
		err := backoff.Retry(func() error {
			cancelFn, err := q.pubsub.SubscribeWithErr(eventClientUpdate, q.listenClient)
			if err != nil {
				q.logger.Warn(q.ctx, "failed to subscribe to client updates", slog.Error(err))
				return err
			}
			cancelClient = cancelFn
			return nil
		}, bkoff)
		if err != nil {
			if q.ctx.Err() == nil {
				q.logger.Error(q.ctx, "code bug: retry failed before context canceled", slog.Error(err))
			}
			return
		}
		defer cancelClient()
		bkoff.Reset()
		q.logger.Debug(q.ctx, "subscribed to client updates")

		var cancelAgent context.CancelFunc
		err = backoff.Retry(func() error {
			cancelFn, err := q.pubsub.SubscribeWithErr(eventAgentUpdate, q.listenAgent)
			if err != nil {
				q.logger.Warn(q.ctx, "failed to subscribe to agent updates", slog.Error(err))
				return err
			}
			cancelAgent = cancelFn
			return nil
		}, bkoff)
		if err != nil {
			if q.ctx.Err() == nil {
				q.logger.Error(q.ctx, "code bug: retry failed before context canceled", slog.Error(err))
			}
			return
		}
		defer cancelAgent()
		q.logger.Debug(q.ctx, "subscribed to agent updates")

		// unblock the outer function from returning
		subscribed <- struct{}{}

		// hold subscriptions open until context is canceled
		<-q.ctx.Done()
	}()
	<-subscribed
}

func (q *querier) listenClient(_ context.Context, msg []byte, err error) {
	if xerrors.Is(err, pubsub.ErrDroppedMessages) {
		q.logger.Warn(q.ctx, "pubsub may have dropped client updates")
		// we need to schedule a full resync of client mappings
		q.resyncClientMappings()
		return
	}
	if err != nil {
		q.logger.Warn(q.ctx, "unhandled pubsub error", slog.Error(err))
		return
	}
	client, agent, err := parseClientUpdate(string(msg))
	if err != nil {
		q.logger.Error(q.ctx, "failed to parse client update", slog.F("msg", string(msg)), slog.Error(err))
		return
	}
	logger := q.logger.With(slog.F("client_id", client), slog.F("agent_id", agent))
	logger.Debug(q.ctx, "got client update")

	mk := mKey{
		agent: agent,
		kind:  agpl.QueueKindAgent,
	}
	q.mu.Lock()
	_, ok := q.mappers[mk]
	q.mu.Unlock()
	if !ok {
		logger.Debug(q.ctx, "ignoring update because we have no mapper")
		return
	}
	q.workQ.enqueue(mk)
}

func (q *querier) listenAgent(_ context.Context, msg []byte, err error) {
	if xerrors.Is(err, pubsub.ErrDroppedMessages) {
		q.logger.Warn(q.ctx, "pubsub may have dropped agent updates")
		// we need to schedule a full resync of agent mappings
		q.resyncAgentMappings()
		return
	}
	if err != nil {
		q.logger.Warn(q.ctx, "unhandled pubsub error", slog.Error(err))
	}
	agent, err := parseUpdateMessage(string(msg))
	if err != nil {
		q.logger.Error(q.ctx, "failed to parse agent update", slog.F("msg", string(msg)), slog.Error(err))
		return
	}
	logger := q.logger.With(slog.F("agent_id", agent))
	logger.Debug(q.ctx, "got agent update")
	mk := mKey{
		agent: agent,
		kind:  agpl.QueueKindClient,
	}
	q.mu.Lock()
	_, ok := q.mappers[mk]
	q.mu.Unlock()
	if !ok {
		logger.Debug(q.ctx, "ignoring update because we have no mapper")
		return
	}
	q.workQ.enqueue(mk)
}

func (q *querier) resyncClientMappings() {
	q.mu.Lock()
	defer q.mu.Unlock()
	for mk := range q.mappers {
		if mk.kind == agpl.QueueKindClient {
			q.workQ.enqueue(mk)
		}
	}
}

func (q *querier) resyncAgentMappings() {
	q.mu.Lock()
	defer q.mu.Unlock()
	for mk := range q.mappers {
		if mk.kind == agpl.QueueKindAgent {
			q.workQ.enqueue(mk)
		}
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

	for _, cm := range q.mappers {
		// send on goroutine to avoid holding the q.mu.  Heartbeat failures come asynchronously with respect to
		// other kinds of work, so it's fine to deliver the command to refresh async.
		go func(m *mapper) {
			// make sure we send on the _mapper_ context, not our own in case the mapper is
			// shutting down or shut down.
			_ = sendCtx(m.ctx, m.update, struct{}{})
		}(cm.mapper)
	}
}

// unhealthyCloseAll marks the coordinator unhealthy and closes all connections.  We do this so that clients and agents
// are forced to reconnect to the coordinator, and will hopefully land on a healthy coordinator.
func (q *querier) unhealthyCloseAll() {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.healthy = false
	for _, c := range q.conns {
		// close connections async so that we don't block the querier routine that responds to updates
		go func(c agpl.Queue) {
			err := c.Close()
			if err != nil {
				q.logger.Debug(q.ctx, "error closing conn while unhealthy", slog.Error(err))
			}
		}(c)
		// NOTE: we don't need to remove the connection from the map, as that will happen async in q.cleanupConn()
	}
}

func (q *querier) setHealthy() {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.healthy = true
}

func (q *querier) getAll(ctx context.Context) (map[uuid.UUID]database.TailnetAgent, map[uuid.UUID][]database.TailnetClient, error) {
	agents, err := q.store.GetAllTailnetAgents(ctx)
	if err != nil {
		return nil, nil, xerrors.Errorf("get all tailnet agents: %w", err)
	}
	agentsMap := map[uuid.UUID]database.TailnetAgent{}
	for _, agent := range agents {
		agentsMap[agent.ID] = agent
	}
	clients, err := q.store.GetAllTailnetClients(ctx)
	if err != nil {
		return nil, nil, xerrors.Errorf("get all tailnet clients: %w", err)
	}
	clientsMap := map[uuid.UUID][]database.TailnetClient{}
	for _, client := range clients {
		for _, agentID := range client.AgentIds {
			clientsMap[agentID] = append(clientsMap[agentID], client.TailnetClient)
		}
	}

	return agentsMap, clientsMap, nil
}

func parseClientUpdate(msg string) (client, agent uuid.UUID, err error) {
	parts := strings.Split(msg, ",")
	if len(parts) != 2 {
		return uuid.Nil, uuid.Nil, xerrors.Errorf("expected 2 parts separated by comma")
	}
	client, err = uuid.Parse(parts[0])
	if err != nil {
		return uuid.Nil, uuid.Nil, xerrors.Errorf("failed to parse client UUID: %w", err)
	}

	agent, err = uuid.Parse(parts[1])
	if err != nil {
		return uuid.Nil, uuid.Nil, xerrors.Errorf("failed to parse agent UUID: %w", err)
	}

	return client, agent, nil
}

func parseUpdateMessage(msg string) (agent uuid.UUID, err error) {
	agent, err = uuid.Parse(msg)
	if err != nil {
		return uuid.Nil, xerrors.Errorf("failed to parse update message UUID: %w", err)
	}
	return agent, nil
}

// mKey identifies a set of node mappings we want to query.
type mKey struct {
	agent uuid.UUID
	// we always query based on the agent ID, but if we have client connection(s), we query the agent itself.  If we
	// have an agent connection, we need the node mappings for all clients of the agent.
	kind agpl.QueueKind
}

// mapping associates a particular client or agent, and its respective coordinator with a node.  It is generalized to
// include clients or agents: agent mappings will have client set to uuid.Nil.
type mapping struct {
	client      uuid.UUID
	agent       uuid.UUID
	coordinator uuid.UUID
	updatedAt   time.Time
	node        *agpl.Node
}

type queueKey interface {
	mKey | bKey | sKey
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
			h.logger.Warn(h.ctx, "failed to subscribe to heartbeats", slog.Error(err))
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
			_ = sendCtx(h.ctx, h.update, hbUpdate{filter: filterUpdateUpdated})
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
			_ = sendCtx(h.ctx, h.update, hbUpdate{filter: filterUpdateUpdated})
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
			_ = sendCtx(h.ctx, h.update, hbUpdate{health: healthUpdateUnhealthy})
		}
		return
	}
	h.logger.Debug(h.ctx, "sent heartbeat")
	if h.failedHeartbeats >= 3 {
		h.logger.Info(h.ctx, "coordinator sent heartbeat and is healthy")
		_ = sendCtx(h.ctx, h.update, hbUpdate{health: healthUpdateHealthy})
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

// cleanup issues a DB command to clean out any old expired coordinators state.  The cleanup is idempotent, so no need
// to synchronize with other coordinators.
func (h *heartbeats) cleanup() {
	err := h.store.CleanTailnetCoordinators(h.ctx)
	if err != nil {
		// the records we are attempting to clean up do no serious harm other than
		// accumulating in the tables, so we don't bother retrying if it fails.
		h.logger.Error(h.ctx, "failed to cleanup old coordinators", slog.Error(err))
		return
	}
	h.logger.Debug(h.ctx, "cleaned up old coordinators")
}

func (c *pgCoord) ServeHTTPDebug(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	debug, err := c.htmlDebug(ctx)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(err.Error()))
		return
	}

	agpl.CoordinatorHTTPDebug(debug)(w, r)
}

func (c *pgCoord) htmlDebug(ctx context.Context) (agpl.HTMLDebug, error) {
	now := time.Now()
	data := agpl.HTMLDebug{}
	agents, clients, err := c.querier.getAll(ctx)
	if err != nil {
		return data, xerrors.Errorf("get all agents and clients: %w", err)
	}

	for _, agent := range agents {
		htmlAgent := &agpl.HTMLAgent{
			ID: agent.ID,
			// Name: ??, TODO: get agent names
			LastWriteAge: now.Sub(agent.UpdatedAt).Round(time.Second),
		}
		for _, conn := range clients[agent.ID] {
			htmlAgent.Connections = append(htmlAgent.Connections, &agpl.HTMLClient{
				ID:           conn.ID,
				Name:         conn.ID.String(),
				LastWriteAge: now.Sub(conn.UpdatedAt).Round(time.Second),
			})
			data.Nodes = append(data.Nodes, &agpl.HTMLNode{
				ID:   conn.ID,
				Node: conn.Node,
			})
		}
		slices.SortFunc(htmlAgent.Connections, func(a, b *agpl.HTMLClient) int {
			return slice.Ascending(a.Name, b.Name)
		})

		data.Agents = append(data.Agents, htmlAgent)
		data.Nodes = append(data.Nodes, &agpl.HTMLNode{
			ID: agent.ID,
			// Name: ??, TODO: get agent names
			Node: agent.Node,
		})
	}
	slices.SortFunc(data.Agents, func(a, b *agpl.HTMLAgent) int {
		return slice.Ascending(a.Name, b.Name)
	})

	for agentID, conns := range clients {
		if len(conns) == 0 {
			continue
		}

		if _, ok := agents[agentID]; ok {
			continue
		}
		agent := &agpl.HTMLAgent{
			Name: "unknown",
			ID:   agentID,
		}
		for _, conn := range conns {
			agent.Connections = append(agent.Connections, &agpl.HTMLClient{
				Name:         conn.ID.String(),
				ID:           conn.ID,
				LastWriteAge: now.Sub(conn.UpdatedAt).Round(time.Second),
			})
			data.Nodes = append(data.Nodes, &agpl.HTMLNode{
				ID:   conn.ID,
				Node: conn.Node,
			})
		}
		slices.SortFunc(agent.Connections, func(a, b *agpl.HTMLClient) int {
			return slice.Ascending(a.Name, b.Name)
		})

		data.MissingAgents = append(data.MissingAgents, agent)
	}
	slices.SortFunc(data.MissingAgents, func(a, b *agpl.HTMLAgent) int {
		return slice.Ascending(a.Name, b.Name)
	})

	return data, nil
}
