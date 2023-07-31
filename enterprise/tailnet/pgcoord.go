package tailnet

import (
	"context"
	"database/sql"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/cenkalti/backoff/v4"
	"github.com/google/uuid"
	"golang.org/x/exp/slices"
	"golang.org/x/xerrors"
	"nhooyr.io/websocket"

	"cdr.dev/slog"
	"github.com/coder/coder/coderd/database"
	"github.com/coder/coder/coderd/database/dbauthz"
	"github.com/coder/coder/coderd/database/pubsub"
	"github.com/coder/coder/coderd/rbac"
	agpl "github.com/coder/coder/tailnet"
)

const (
	EventHeartbeats   = "tailnet_coordinator_heartbeat"
	eventClientUpdate = "tailnet_client_update"
	eventAgentUpdate  = "tailnet_agent_update"
	HeartbeatPeriod   = time.Second * 2
	MissedHeartbeats  = 3
	numQuerierWorkers = 10
	numBinderWorkers  = 10
	dbMaxBackoff      = 10 * time.Second
	cleanupPeriod     = time.Hour
)

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

	bindings       chan binding
	newConnections chan *connIO
	id             uuid.UUID

	cancel    context.CancelFunc
	closeOnce sync.Once
	closed    chan struct{}

	binder  *binder
	querier *querier
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
	cCh := make(chan *connIO)
	// signals when first heartbeat has been sent, so it's safe to start binding.
	fHB := make(chan struct{})

	c := &pgCoord{
		ctx:            ctx,
		cancel:         cancel,
		logger:         logger,
		pubsub:         ps,
		store:          store,
		binder:         newBinder(ctx, logger, id, store, bCh, fHB),
		bindings:       bCh,
		newConnections: cCh,
		id:             id,
		querier:        newQuerier(ctx, logger, ps, store, id, cCh, numQuerierWorkers, fHB),
		closed:         make(chan struct{}),
	}
	logger.Info(ctx, "starting coordinator")
	return c, nil
}

func (c *pgCoord) ServeMultiAgent(id uuid.UUID) agpl.MultiAgentConn {
	_, _ = c, id
	panic("not implemented") // TODO: Implement
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
	cIO := newConnIO(c.ctx, c.logger, c.bindings, conn, id, agent, id.String())
	if err := sendCtx(c.ctx, c.newConnections, cIO); err != nil {
		// can only be a context error, no need to log here.
		return err
	}
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
	cIO := newConnIO(c.ctx, logger, c.bindings, conn, uuid.Nil, id, name)
	if err := sendCtx(c.ctx, c.newConnections, cIO); err != nil {
		// can only be a context error, no need to log here.
		return err
	}
	<-cIO.ctx.Done()
	return nil
}

func (c *pgCoord) Close() error {
	c.logger.Info(c.ctx, "closing coordinator")
	c.cancel()
	c.closeOnce.Do(func() { close(c.closed) })
	return nil
}

// connIO manages the reading and writing to a connected client or agent.  Agent connIOs have their client field set to
// uuid.Nil.  It reads node updates via its decoder, then pushes them onto the bindings channel.  It receives mappings
// via its updates TrackedConn, which then writes them.
type connIO struct {
	pCtx     context.Context
	ctx      context.Context
	cancel   context.CancelFunc
	logger   slog.Logger
	client   uuid.UUID
	agent    uuid.UUID
	decoder  *json.Decoder
	updates  *agpl.TrackedConn
	bindings chan<- binding
}

func newConnIO(pCtx context.Context,
	logger slog.Logger,
	bindings chan<- binding,
	conn net.Conn,
	client, agent uuid.UUID,
	name string,
) *connIO {
	ctx, cancel := context.WithCancel(pCtx)
	id := agent
	logger = logger.With(slog.F("agent_id", agent))
	if client != uuid.Nil {
		logger = logger.With(slog.F("client_id", client))
		id = client
	}
	c := &connIO{
		pCtx:     pCtx,
		ctx:      ctx,
		cancel:   cancel,
		logger:   logger,
		client:   client,
		agent:    agent,
		decoder:  json.NewDecoder(conn),
		updates:  agpl.NewTrackedConn(ctx, cancel, conn, id, logger, name, 0),
		bindings: bindings,
	}
	go c.recvLoop()
	go c.updates.SendUpdates()
	logger.Info(ctx, "serving connection")
	return c
}

func (c *connIO) recvLoop() {
	defer func() {
		// withdraw bindings when we exit.  We need to use the parent context here, since our own context might be
		// canceled, but we still need to withdraw bindings.
		b := binding{
			bKey: bKey{
				client: c.client,
				agent:  c.agent,
			},
		}
		if err := sendCtx(c.pCtx, c.bindings, b); err != nil {
			c.logger.Debug(c.ctx, "parent context expired while withdrawing bindings", slog.Error(err))
		}
	}()
	defer c.cancel()
	for {
		var node agpl.Node
		err := c.decoder.Decode(&node)
		if err != nil {
			if xerrors.Is(err, io.EOF) ||
				xerrors.Is(err, io.ErrClosedPipe) ||
				xerrors.Is(err, context.Canceled) ||
				xerrors.Is(err, context.DeadlineExceeded) ||
				websocket.CloseStatus(err) > 0 {
				c.logger.Debug(c.ctx, "exiting recvLoop", slog.Error(err))
			} else {
				c.logger.Error(c.ctx, "failed to decode Node update", slog.Error(err))
			}
			return
		}
		c.logger.Debug(c.ctx, "got node update", slog.F("node", node))
		b := binding{
			bKey: bKey{
				client: c.client,
				agent:  c.agent,
			},
			node: &node,
		}
		if err := sendCtx(c.ctx, c.bindings, b); err != nil {
			c.logger.Debug(c.ctx, "recvLoop ctx expired", slog.Error(err))
			return
		}
	}
}

func sendCtx[A any](ctx context.Context, c chan<- A, a A) (err error) {
	select {
	case <-ctx.Done():
		return ctx.Err()
	case c <- a:
		return nil
	}
}

// bKey, or "binding key" identifies a client or agent in a binding.  Agents have their client field set to uuid.Nil.
type bKey struct {
	client uuid.UUID
	agent  uuid.UUID
}

// binding represents an association between a client or agent and a Node.
type binding struct {
	bKey
	node *agpl.Node
}

func (b *binding) isAgent() bool  { return b.client == uuid.Nil }
func (b *binding) isClient() bool { return b.client != uuid.Nil }

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

func newBinder(ctx context.Context, logger slog.Logger,
	id uuid.UUID, store database.Store,
	bindings <-chan binding, startWorkers <-chan struct{},
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
			ID:            bnd.agent,
			CoordinatorID: b.coordinatorID,
			Node:          nodeRaw,
		})
	case bnd.isAgent() && len(nodeRaw) == 0:
		_, err = b.store.DeleteTailnetAgent(b.ctx, database.DeleteTailnetAgentParams{
			ID:            bnd.agent,
			CoordinatorID: b.coordinatorID,
		})
		if xerrors.Is(err, sql.ErrNoRows) {
			// treat deletes as idempotent
			err = nil
		}
	case bnd.isClient() && len(nodeRaw) > 0:
		_, err = b.store.UpsertTailnetClient(b.ctx, database.UpsertTailnetClientParams{
			ID:            bnd.client,
			CoordinatorID: b.coordinatorID,
			AgentID:       bnd.agent,
			Node:          nodeRaw,
		})
	case bnd.isClient() && len(nodeRaw) == 0:
		_, err = b.store.DeleteTailnetClient(b.ctx, database.DeleteTailnetClientParams{
			ID:            bnd.client,
			CoordinatorID: b.coordinatorID,
		})
		if xerrors.Is(err, sql.ErrNoRows) {
			// treat deletes as idempotent
			err = nil
		}
	default:
		panic("unhittable")
	}
	if err != nil && !database.IsQueryCanceledError(err) {
		b.logger.Error(b.ctx, "failed to write binding to database",
			slog.F("client_id", bnd.client),
			slog.F("agent_id", bnd.agent),
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

	add chan *connIO
	del chan *connIO

	// reads from this channel trigger sending latest nodes to
	// all connections.  It is used when coordinators are added
	// or removed
	update chan struct{}

	mappings chan []mapping

	conns  map[bKey]*connIO
	latest []mapping

	heartbeats *heartbeats
}

func newMapper(ctx context.Context, logger slog.Logger, mk mKey, h *heartbeats) *mapper {
	logger = logger.With(
		slog.F("agent_id", mk.agent),
		slog.F("clients_of_agent", mk.clientsOfAgent),
	)
	m := &mapper{
		ctx:        ctx,
		logger:     logger,
		add:        make(chan *connIO),
		del:        make(chan *connIO),
		update:     make(chan struct{}),
		conns:      make(map[bKey]*connIO),
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
			m.conns[bKey{c.client, c.agent}] = c
			nodes := m.mappingsToNodes(m.latest)
			if len(nodes) == 0 {
				m.logger.Debug(m.ctx, "skipping 0 length node update")
				continue
			}
			if err := c.updates.Enqueue(nodes); err != nil {
				m.logger.Error(m.ctx, "failed to enqueue node update", slog.Error(err))
			}
		case c := <-m.del:
			delete(m.conns, bKey{c.client, c.agent})
		case mappings := <-m.mappings:
			m.latest = mappings
			nodes := m.mappingsToNodes(mappings)
			if len(nodes) == 0 {
				m.logger.Debug(m.ctx, "skipping 0 length node update")
				continue
			}
			for _, conn := range m.conns {
				if err := conn.updates.Enqueue(nodes); err != nil {
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
				if err := conn.updates.Enqueue(nodes); err != nil {
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
		bk := bKey{client: m.client, agent: m.agent}
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
	ctx            context.Context
	logger         slog.Logger
	pubsub         pubsub.Pubsub
	store          database.Store
	newConnections chan *connIO

	workQ      *workQ[mKey]
	heartbeats *heartbeats
	updates    <-chan struct{}

	mu      sync.Mutex
	mappers map[mKey]*countedMapper
}

type countedMapper struct {
	*mapper
	count  int
	cancel context.CancelFunc
}

func newQuerier(
	ctx context.Context, logger slog.Logger,
	ps pubsub.Pubsub, store database.Store,
	self uuid.UUID, newConnections chan *connIO, numWorkers int,
	firstHeartbeat chan<- struct{},
) *querier {
	updates := make(chan struct{})
	q := &querier{
		ctx:            ctx,
		logger:         logger.Named("querier"),
		pubsub:         ps,
		store:          store,
		newConnections: newConnections,
		workQ:          newWorkQ[mKey](ctx),
		heartbeats:     newHeartbeats(ctx, logger, ps, store, self, updates, firstHeartbeat),
		mappers:        make(map[mKey]*countedMapper),
		updates:        updates,
	}
	go q.subscribe()
	go q.handleConnIO()
	for i := 0; i < numWorkers; i++ {
		go q.worker()
	}
	go q.handleUpdates()
	return q
}

func (q *querier) handleConnIO() {
	for {
		select {
		case <-q.ctx.Done():
			return
		case c := <-q.newConnections:
			q.newConn(c)
		}
	}
}

func (q *querier) newConn(c *connIO) {
	q.mu.Lock()
	defer q.mu.Unlock()
	mk := mKey{
		agent: c.agent,
		// if client is Nil, this is an agent connection, and it wants the mappings for all the clients of itself
		clientsOfAgent: c.client == uuid.Nil,
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
	go q.cleanupConn(c)
}

func (q *querier) cleanupConn(c *connIO) {
	<-c.ctx.Done()
	q.mu.Lock()
	defer q.mu.Unlock()
	mk := mKey{
		agent: c.agent,
		// if client is Nil, this is an agent connection, and it wants the mappings for all the clients of itself
		clientsOfAgent: c.client == uuid.Nil,
	}
	cm := q.mappers[mk]
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
	if mk.clientsOfAgent {
		mappings, err = q.queryClientsOfAgent(mk.agent)
		if err != nil {
			return err
		}
	} else {
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
			slog.F("agent_id", mk.agent), slog.F("clients_of_agent", mk.clientsOfAgent))
		return nil
	}
	mpr.mappings <- mappings
	return nil
}

func (q *querier) queryClientsOfAgent(agent uuid.UUID) ([]mapping, error) {
	clients, err := q.store.GetTailnetClientsForAgent(q.ctx, agent)
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
			agent:       client.AgentID,
			coordinator: client.CoordinatorID,
			updatedAt:   client.UpdatedAt,
			node:        node,
		})
	}
	return mappings, nil
}

func (q *querier) queryAgent(agentID uuid.UUID) ([]mapping, error) {
	agents, err := q.store.GetTailnetAgents(q.ctx, agentID)
	if err != nil {
		return nil, err
	}
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

func (q *querier) subscribe() {
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

	// hold subscriptions open until context is canceled
	<-q.ctx.Done()
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
	}
	client, agent, err := parseClientUpdate(string(msg))
	if err != nil {
		q.logger.Error(q.ctx, "failed to parse client update", slog.F("msg", string(msg)), slog.Error(err))
		return
	}
	logger := q.logger.With(slog.F("client_id", client), slog.F("agent_id", agent))
	logger.Debug(q.ctx, "got client update")
	mk := mKey{
		agent:          agent,
		clientsOfAgent: true,
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
	agent, err := parseAgentUpdate(string(msg))
	if err != nil {
		q.logger.Error(q.ctx, "failed to parse agent update", slog.F("msg", string(msg)), slog.Error(err))
		return
	}
	logger := q.logger.With(slog.F("agent_id", agent))
	logger.Debug(q.ctx, "got agent update")
	mk := mKey{
		agent:          agent,
		clientsOfAgent: false,
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
		if mk.clientsOfAgent {
			q.workQ.enqueue(mk)
		}
	}
}

func (q *querier) resyncAgentMappings() {
	q.mu.Lock()
	defer q.mu.Unlock()
	for mk := range q.mappers {
		if !mk.clientsOfAgent {
			q.workQ.enqueue(mk)
		}
	}
}

func (q *querier) handleUpdates() {
	for {
		select {
		case <-q.ctx.Done():
			return
		case <-q.updates:
			q.updateAll()
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
		clientsMap[client.AgentID] = append(clientsMap[client.AgentID], client)
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

func parseAgentUpdate(msg string) (agent uuid.UUID, err error) {
	agent, err = uuid.Parse(msg)
	if err != nil {
		return uuid.Nil, xerrors.Errorf("failed to parse agent UUID: %w", err)
	}
	return agent, nil
}

// mKey identifies a set of node mappings we want to query.
type mKey struct {
	agent uuid.UUID
	// we always query based on the agent ID, but if we have client connection(s), we query the agent itself.  If we
	// have an agent connection, we need the node mappings for all clients of the agent.
	clientsOfAgent bool
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

// workQ allows scheduling work based on a key.  Multiple enqueue requests for the same key are coalesced, and
// only one in-progress job per key is scheduled.
type workQ[K mKey | bKey] struct {
	ctx context.Context

	cond       *sync.Cond
	pending    []K
	inProgress map[K]bool
}

func newWorkQ[K mKey | bKey](ctx context.Context) *workQ[K] {
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

	update         chan<- struct{}
	firstHeartbeat chan<- struct{}

	lock         sync.RWMutex
	coordinators map[uuid.UUID]time.Time
	timer        *time.Timer

	// overwritten in tests, but otherwise constant
	cleanupPeriod time.Duration
}

func newHeartbeats(
	ctx context.Context, logger slog.Logger,
	ps pubsub.Pubsub, store database.Store,
	self uuid.UUID, update chan<- struct{},
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
			_ = sendCtx(h.ctx, h.update, struct{}{})
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
			_ = sendCtx(h.ctx, h.update, struct{}{})
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
	if err != nil {
		// just log errors, heartbeats are rescheduled on a timer
		h.logger.Error(h.ctx, "failed to send heartbeat", slog.Error(err))
		return
	}
	h.logger.Debug(h.ctx, "sent heartbeat")
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
		slices.SortFunc(htmlAgent.Connections, func(a, b *agpl.HTMLClient) bool {
			return a.Name < b.Name
		})

		data.Agents = append(data.Agents, htmlAgent)
		data.Nodes = append(data.Nodes, &agpl.HTMLNode{
			ID: agent.ID,
			// Name: ??, TODO: get agent names
			Node: agent.Node,
		})
	}
	slices.SortFunc(data.Agents, func(a, b *agpl.HTMLAgent) bool {
		return a.Name < b.Name
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
		slices.SortFunc(agent.Connections, func(a, b *agpl.HTMLClient) bool {
			return a.Name < b.Name
		})

		data.MissingAgents = append(data.MissingAgents, agent)
	}
	slices.SortFunc(data.MissingAgents, func(a, b *agpl.HTMLAgent) bool {
		return a.Name < b.Name
	})

	return data, nil
}
