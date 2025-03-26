package coderd

import (
	"context"
	"database/sql"
	"fmt"
	"sort"
	"sync"
	"time"

	"github.com/google/uuid"
	"golang.org/x/exp/slices"
	"golang.org/x/xerrors"
	"storj.io/drpc"

	agproto "github.com/coder/coder/v2/agent/proto"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/database/pubsub"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/tailnet"
	proto "github.com/coder/coder/v2/tailnet/proto"
	"github.com/coder/retry"
	"github.com/coder/slice"
)

// workspaceUpdatesProvider implements the tailnet.WorkspaceUpdatesProvider interface.
// It handles constructing workspace update events needed for reconnecting terminals.
type workspaceUpdatesProvider struct {
	db database.Store
}

func NewWorkspaceUpdatesProvider(db database.Store) tailnet.WorkspaceUpdatesProvider {
	return &workspaceUpdatesProvider{
		db: db,
	}
}

type workspaceData struct {
	WorkspaceName string
	Status        string
	// Order matters here. Sort by ID to get consistent results.
	Agents []agentData
}

func (w workspaceData) Equal(other workspaceData) bool {
	if w.WorkspaceName != other.WorkspaceName || w.Status != other.Status {
		return false
	}
	return slices.Equal(w.Agents, other.Agents)
}

type agentData struct {
	ID   uuid.UUID
	Name string
}

type workspacesByID map[uuid.UUID]workspaceData

// subscription implements tailnet.Subscription
type subscription struct {
	// Context is used to cancel the stream of updates.
	ctx    context.Context
	cancel context.CancelFunc

	// Signal is closed when the subscription is closed.
	closed chan struct{}

	// Updates is the channel that receives workspace updates.
	updates chan *proto.WorkspaceUpdate

	// Pubsub is owned by the subscription. It is closed when subscription is closed.
	ps pubsub.Pubsub

	provider *workspaceUpdatesProvider
	userID   uuid.UUID
}

func (s *subscription) Close() error {
	s.cancel()
	close(s.closed)
	err := s.ps.Close()
	return err
}

func (s *subscription) Updates() <-chan *proto.WorkspaceUpdate {
	return s.updates
}

func (s *subscription) wait() {
	<-s.closed
}

func (w *workspaceUpdatesProvider) Subscribe(ctx context.Context, userID uuid.UUID) (tailnet.Subscription, error) {
	// Create our own cancellable context. If the subscription context is
	// canceled, updates can be stopped.
	ctx, cancel := context.WithCancel(ctx)

	ps := pubsub.New()
	pubsub.SubscribeWorkspaceAgents(ps)

	// A buffered channel is used here so that slow consumers can catch up,
	// but this effectively makes this a unicast channel.
	// Slow consumers will not block the producer, and the producer will just
	// keep going.
	// Tailnet reconnections that include workspace updates will send a full
	// update first, then start sending updates from pubsub.
	updatesCh := make(chan *proto.WorkspaceUpdate, 16)

	curWorkspaces, err := w.listWorkspaces(ctx, userID)
	if err != nil {
		cancel()
		_ = ps.Close()
		return nil, xerrors.Errorf("list workspaces: %w", err)
	}

	sub := &subscription{
		ctx:      ctx,
		cancel:   cancel,
		closed:   make(chan struct{}),
		updates:  updatesCh,
		ps:       ps,
		provider: w,
		userID:   userID,
	}

	// Send the initial udpate
	initialUpdate, _ := produceUpdate(workspacesByID{}, curWorkspaces)
	updatesCh <- initialUpdate

	// Setup a background goroutine to listen for pubsub messages
	// and process them
	go func() {
		defer close(updatesCh)
		defer cancel()
		// This worker processes all pubsub messages.
		err := ps.Subscribe(ctx, func(ctx context.Context, e pubsub.Event) error {
			// Reset the workspace state.
			// We've gotten a notification, so let's check if anything has changed.
			// This is wasteful: we should be able to use the pubsub event that
			// we get to emit changes, but then we can miss changes, since we rely
			// on other fields in the database.

			// If too many workspaces, we might want to make this more efficient.
			// But given that a single owner will likely not have thousands of
			// workspaces, it's not critical.
			newWorkspaces, err := w.listWorkspaces(ctx, userID)
			if err != nil {
				return xerrors.Errorf("list workspaces: %w", err)
			}

			update, updated := produceUpdate(curWorkspaces, newWorkspaces)
			if updated {
				// Swap workspaces
				curWorkspaces = newWorkspaces
				// Send update
				updatesCh <- update
			}

			return nil
		})
		if err != nil && !xerrors.Is(err, context.Canceled) {
			// If context is canceled, it's because the subscription is closed
			// which is OK.
			panic(fmt.Sprintf("subscribe to pubsub: %s", err))
		}
	}()

	return sub, nil
}

func produceUpdate(old, newVal workspacesByID) (out *proto.WorkspaceUpdate, updated bool) {
	out = &proto.WorkspaceUpdate{
		UpsertedWorkspaces: []*proto.Workspace{},
		UpsertedAgents:     []*proto.Agent{},
		DeletedWorkspaces:  []*proto.Workspace{},
		DeletedAgents:      []*proto.Agent{},
	}

	for wsID, newWorkspace := range newVal {
		oldWorkspace, exists := old[wsID]
		// Upsert both workspace and agents if the workspace is new
		if !exists {
			out.UpsertedWorkspaces = append(out.UpsertedWorkspaces, &proto.Workspace{
				Id:     tailnet.UUIDToByteSlice(wsID),
				Name:   newWorkspace.WorkspaceName,
				Status: newWorkspace.Status,
			})
			for _, agent := range newWorkspace.Agents {
				out.UpsertedAgents = append(out.UpsertedAgents, &proto.Agent{
					Id:          tailnet.UUIDToByteSlice(agent.ID),
					Name:        agent.Name,
					WorkspaceId: tailnet.UUIDToByteSlice(wsID),
				})
			}
			updated = true
			continue
		}
		// Upsert workspace if the workspace is updated
		if !newWorkspace.Equal(oldWorkspace) {
			out.UpsertedWorkspaces = append(out.UpsertedWorkspaces, &proto.Workspace{
				Id:     tailnet.UUIDToByteSlice(wsID),
				Name:   newWorkspace.WorkspaceName,
				Status: newWorkspace.Status,
			})
			updated = true
		}

		add, remove := slice.SymmetricDifference(oldWorkspace.Agents, newWorkspace.Agents)
		for _, agent := range add {
			out.UpsertedAgents = append(out.UpsertedAgents, &proto.Agent{
				Id:          tailnet.UUIDToByteSlice(agent.ID),
				Name:        agent.Name,
				WorkspaceId: tailnet.UUIDToByteSlice(wsID),
			})
			updated = true
		}
		for _, agent := range remove {
			out.DeletedAgents = append(out.DeletedAgents, &proto.Agent{
				Id:          tailnet.UUIDToByteSlice(agent.ID),
				Name:        agent.Name,
				WorkspaceId: tailnet.UUIDToByteSlice(wsID),
			})
			updated = true
		}
	}

	// Delete workspace and agents if the workspace is deleted
	for wsID, oldWorkspace := range old {
		if _, exists := newVal[wsID]; !exists {
			out.DeletedWorkspaces = append(out.DeletedWorkspaces, &proto.Workspace{
				Id:     tailnet.UUIDToByteSlice(wsID),
				Name:   oldWorkspace.WorkspaceName,
				Status: oldWorkspace.Status,
			})
			for _, agent := range oldWorkspace.Agents {
				out.DeletedAgents = append(out.DeletedAgents, &proto.Agent{
					Id:          tailnet.UUIDToByteSlice(agent.ID),
					Name:        agent.Name,
					WorkspaceId: tailnet.UUIDToByteSlice(wsID),
				})
			}
			updated = true
		}
	}

	return out, updated
}

func (w *workspaceUpdatesProvider) listWorkspaces(ctx context.Context, userID uuid.UUID) (workspacesByID, error) {
	workspaces, err := w.db.GetWorkspacesByOwnerID(dbauthz.AsSystemRestricted(ctx), database.GetWorkspacesByOwnerIDParams{
		OwnerID: userID,
	})
	if err != nil {
		return nil, xerrors.Errorf("get workspaces by owner: %w", err)
	}

	workspacesState := workspacesByID{}
	for _, workspace := range workspaces {
		// For now, resources are stored by name; we're assuming the name is
		// unique within the latest build. Eventually we'd want to modify this
		// to handle renames and avoid agents swapping connections.
		latestResources, err := w.db.GetWorkspaceResourcesByJobID(dbauthz.AsSystemRestricted(ctx), workspace.LatestBuildID)
		if err != nil {
			return nil, xerrors.Errorf("get workspace resources: %w", err)
		}

		var maybeAgents []database.WorkspaceAgent
		agents := make([]agentData, 0)
		for _, resource := range latestResources {
			agts, err := w.db.GetWorkspaceAgentsByResourceIDs(dbauthz.AsSystemRestricted(ctx), []uuid.UUID{resource.ID})
			if err != nil {
				return nil, xerrors.Errorf("get workspace agents: %w", err)
			}
			maybeAgents = append(maybeAgents, agts...)
		}

		for _, agent := range maybeAgents {
			// Don't include agents that are shutting down or errored.
			// The pty code inside agent closes connections on shutdown,
			// we should as well.
			if agent.Status == database.WorkspaceAgentStatusDisconnected ||
				agent.Status == database.WorkspaceAgentStatusTimeout {
				continue
			}
			agents = append(agents, agentData{
				ID:   agent.ID,
				Name: agent.Name,
			})
		}

		// Sort agents by ID.
		sort.Slice(agents, func(i, j int) bool {
			return agents[i].ID.String() < agents[j].ID.String()
		})

		state := codersdk.WorkspaceDisplayStatus(workspace.Status())
		workspacesState[workspace.ID] = workspaceData{
			WorkspaceName: workspace.Name,
			Status:        string(state),
			Agents:        agents,
		}
	}

	return workspacesState, nil
}

// WorkspaceUpdatesReqOrganization is a server for the workspace.updates RPC.
type WorkspaceUpdatesReqOrganization struct {
	ctx        context.Context
	provider   tailnet.WorkspaceUpdatesProvider
	userID     uuid.UUID
	once       sync.Once
	initErr    error
	sub        tailnet.Subscription
	cancelSend context.CancelFunc
}

// WorkspaceUpdatesRPCCoordinator returns a WorkspaceUpdatesReqOrganization for using
// with the rpc server handler.
func WorkspaceUpdatesRPCCoordinator(ctx context.Context, provider tailnet.WorkspaceUpdatesProvider, userID uuid.UUID) *WorkspaceUpdatesReqOrganization {
	return &WorkspaceUpdatesReqOrganization{
		ctx:      ctx,
		provider: provider,
		userID:   userID,
	}
}

func (s *WorkspaceUpdatesReqOrganization) Close() error {
	// Cancel the sending context.
	if s.cancelSend != nil {
		s.cancelSend()
	}

	// Close the subscription.
	if s.sub != nil {
		return s.sub.Close()
	}

	return nil
}

// Cancel cancels the subscription.
func (s *WorkspaceUpdatesReqOrganization) Cancel() {
	_ = s.Close()
}

func (s *WorkspaceUpdatesReqOrganization) Init() error {
	s.once.Do(func() {
		// If this UserID is zero, we can simply return.
		// The zero-value can be used to disable workspace updates.
		if s.userID == uuid.Nil {
			return
		}

		var err error
		s.sub, err = s.provider.Subscribe(s.ctx, s.userID)
		if err != nil {
			s.initErr = err
		}
	})

	return s.initErr
}

// Setup is part of the WorkspaceUpdatesCoordinator interface.
func (s *WorkspaceUpdatesReqOrganization) Setup(_ *proto.WorkspaceUpdatesRequest) {}

// Send is part of the WorkspaceUpdatesCoordinator interface.
// This is for the server side of the RPC.
func (s *WorkspaceUpdatesReqOrganization) Send(ctx context.Context, server agproto.DRPCTailnetServer_WorkspaceUpdatesStream) error {
	// If we're set up to ignore updates (from userID), do nothing.
	if s.userID == uuid.Nil {
		// We have to prevent this from ending and closing the stream.
		<-ctx.Done()
		return ctx.Err()
	}

	// Create a cancelable context to use for early-termination.
	// This prevents a memory leak if the subscriber exits before the sender.
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	// Use this context to cancel the sender if Close is called.
	s.cancelSend = cancel

	// Every time we get an update, send it on the stream.
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()

		case update, ok := <-s.sub.Updates():
			if !ok {
				// The stream is closed.
				return drpc.ClosedError{}
			}

			err := server.Send(update)
			if err != nil {
				return xerrors.Errorf("send workspace updates stream: %w", err)
			}
		}
	}
}

// Recv is part of the WorkspaceUpdatesCoordinator interface.
// Since this is a server implementation, this is a no-op.
func (s *WorkspaceUpdatesReqOrganization) Recv(ctx context.Context, client agproto.DRPCTailnetClient_WorkspaceUpdatesStream) error {
	<-ctx.Done()
	return ctx.Err()
}

// Coordinator for the client side of the workspace updates RPC.
// On the client side, we just receive updates from the server.
//
//	WorkspaceUpdatesCoordinator is used to configure a tailnet service for workspace
//	updates. It integrates with tailnet.ClientPool, which handles reconnections and
//	retries, so consider using that instead.
type WorkspaceUpdatesReqClient struct {
	ctx       context.Context
	req       *proto.WorkspaceUpdatesRequest
	sendMutex sync.Mutex
	// Wrapped channel is the channel that will receive updates.
	wrappedCh chan *proto.WorkspaceUpdate
	// waitClient is used to block on reconnections.
	waitClient chan struct{}
	// done is used to signal that the client is done, and we can terminate.
	// This could be because an error happened, or because the client was
	// instructed to close.
	done chan struct{}
	// cancel aborts any active streams inside this client.
	cancel context.CancelFunc
	// closedErr is nil until client is closed.
	closedErr atomic.Value
}

var (
	_ tailnet.WorkspaceUpdatesCoordinator = (*WorkspaceUpdatesReqClient)(nil)
	_ tailnet.WorkspaceUpdatesCoordinator = (*WorkspaceUpdatesReqOrganization)(nil)
)

// NewWorkspaceUpdatesReqClient creates a new client for the workspace updates RPC.
// It can be used to configure a tailnet service for workspace updates.
func NewWorkspaceUpdatesReqClient(ctx context.Context, req *proto.WorkspaceUpdatesRequest) *WorkspaceUpdatesReqClient {
	if req == nil {
		req = &proto.WorkspaceUpdatesRequest{}
	}
	ctx, cancel := context.WithCancel(ctx)
	client := &WorkspaceUpdatesReqClient{
		ctx:        ctx,
		req:        req,
		cancel:     cancel,
		wrappedCh:  make(chan *proto.WorkspaceUpdate, 16),
		waitClient: make(chan struct{}),
		done:       make(chan struct{}),
	}
	return client
}

// Close cancels the coordinating client. This will close any active streams.
// After this is called, no new updates should be received and Updates() should
// be closed.
//
// Close doesn't return any errors; it effectively just cancels the internal
// context, and closes the updates channel.
func (c *WorkspaceUpdatesReqClient) Close() error {
	var closed bool
	select {
	case <-c.done:
		closed = true
	default:
		close(c.done)
	}
	if closed {
		return c.closedErr.Load().(error)
	}

	// Cancel just the internal context. The wrapped send context should
	// be canceled by the caller, if it's still active.
	c.cancel()
	return nil
}

// Cancel satisfies the Cancel interface, which is used by the client pool
// to close connections when they are no longer needed.
func (c *WorkspaceUpdatesReqClient) Cancel() {
	_ = c.Close()
}

// Updates returns the channel that will receive workspace update messages.
// This channel is set up during Init by the tailnet client service, and
// filled during Recv until it's closed.
func (c *WorkspaceUpdatesReqClient) Updates() <-chan *proto.WorkspaceUpdate {
	return c.wrappedCh
}

// Setup is part of the WorkspaceUpdatesCoordinator interface.
func (c *WorkspaceUpdatesReqClient) Setup(_ *proto.WorkspaceUpdatesRequest) {}

// Init is part of the WorkspaceUpdatesCoordinator interface.
func (c *WorkspaceUpdatesReqClient) Init() error {
	return nil
}

// waitOrDone waits for a client to be initialized, or for a Done signal.
// It returns true if we need to reap the client because it's done.
// It's used as a signal for the receiver to stop.
func (c *WorkspaceUpdatesReqClient) waitOrDone() bool {
	select {
	case <-c.waitClient:
		return false
	case <-c.done:
		// We're done, clean up the client.
		c.closedErr.CompareAndSwap(nil, xerrors.New("client closed"))
		return true
	}
}

// NotifyClient notifies the client-specific things that a new client was created on reconnection.
// This allows us to unblock any waiting goroutines, and to start receiving updates again.
func (c *WorkspaceUpdatesReqClient) NotifyClient() {
	select {
	case <-c.waitClient:
		return
	default:
		close(c.waitClient)
	}
}

// Send is part of the WorkspaceUpdatesCoordinator interface.
// This is for the client side of the RPC.
// There is a race condition in the tailnet client where setting up a coordinator
// might happen twice, so we should be careful about locking.
func (c *WorkspaceUpdatesReqClient) Send(ctx context.Context, server agproto.DRPCTailnetClient_WorkspaceUpdatesStream) error {
	// Prevent multiple init calls from sending simultaneously
	c.sendMutex.Lock()
	defer c.sendMutex.Unlock()

	// Notify that a client was created on reconnection.
	c.NotifyClient()

	// Wait for our context to be closed, or for this active client stream to be canceled.
	// This is necessary to allow the stream to be closed on reconnection, and to give us
	// a way to tell the tailnet service to disconnect.
	select {
	case <-c.ctx.Done():
		// Our coordinating client was closed
		// on the client API, probably because the client is exiting or re-connecting.
		return c.ctx.Err()

	case <-ctx.Done():
		// If the context we're given is closed, it means our tailnet client is exiting
		// or replacing this client. This is different from the coordinating client
		// being closed.
		return ctx.Err()
	}
}

// Recv is used for the client implementation to receive updates from the server.
// Notice, if there's a stream restart signal, we need to wait again for client to
// be initialized. This is done inside waitOrDone
func (c *WorkspaceUpdatesReqClient) Recv(ctx context.Context, client agproto.DRPCTailnetClient_WorkspaceUpdatesStream) error {
	// Wait for either client to initialize, or for the context to be closed.
	if c.waitOrDone() {
		return xerrors.New("client closed")
	}

	// Create a new "wait" channel for the next reconnection.
	// We don't know when we'll be reconnected, so we need to wait for it.
	c.waitClient = make(chan struct{})

	// A little unusual that we use retry here.
	// The reason is that drpc can throw a bunch of short-lived errors
	// when streams are being set up, so we need to be resilient to those.
	// In particular, "use of closed network connection" can happen, as well
	// as "i/o timeout"
	err := retry.New(100*time.Millisecond, 10).Do(ctx, func() error {
		// Keep going forever.
		// When the stream is closed, or when we get an error, we'll return.
		for {
			select {
			case <-c.ctx.Done():
				// Client is shutting down, so close the updates channel.
				close(c.wrappedCh)
				return c.ctx.Err()

			case <-ctx.Done():
				return ctx.Err()
			default:
			}

			// Get the next update from the server.
			// This blocks waiting for updates, so we might be
			// closed in the meantime, which is why we check the
			// contexts above.
			update, err := client.Recv()
			if err != nil {
				return xerrors.Errorf("recv workspace updates stream: %w", err)
			}

			// This can block if the client doesn't read from the channel.
			// We're OK with that.
			c.wrappedCh <- update
		}
	})

	// Handle closed streams.
	// This can happen a) because of tailnet client reconnections, or b) because the client
	// was explicitly closed.
	if err != nil && !xerrors.Is(err, context.Canceled) && !drpc.IsCanceled(err) && !drpc.IsClosed(err) {
		c.closedErr.CompareAndSwap(nil, err)
		close(c.done)
		close(c.wrappedCh)
		return err
	}

	return nil
}