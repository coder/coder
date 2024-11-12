package coderd

import (
	"context"
	"fmt"
	"sync"

	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"cdr.dev/slog"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/database/pubsub"
	"github.com/coder/coder/v2/coderd/rbac"
	"github.com/coder/coder/v2/coderd/util/slice"
	"github.com/coder/coder/v2/coderd/wspubsub"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/tailnet"
	"github.com/coder/coder/v2/tailnet/proto"
)

type UpdatesQuerier interface {
	// GetAuthorizedWorkspacesAndAgentsByOwnerID requires a context with an actor set
	GetWorkspacesAndAgentsByOwnerID(ctx context.Context, ownerID uuid.UUID) ([]database.GetWorkspacesAndAgentsByOwnerIDRow, error)
	GetWorkspaceByAgentID(ctx context.Context, agentID uuid.UUID) (database.Workspace, error)
}

type workspacesByID = map[uuid.UUID]ownedWorkspace

type ownedWorkspace struct {
	WorkspaceName string
	Status        proto.Workspace_Status
	Agents        []database.AgentIDNamePair
}

// Equal does not compare agents
func (w ownedWorkspace) Equal(other ownedWorkspace) bool {
	return w.WorkspaceName == other.WorkspaceName &&
		w.Status == other.Status
}

type sub struct {
	// ALways contains an actor
	ctx      context.Context
	cancelFn context.CancelFunc

	mu     sync.RWMutex
	userID uuid.UUID
	ch     chan *proto.WorkspaceUpdate
	prev   workspacesByID

	db     UpdatesQuerier
	ps     pubsub.Pubsub
	logger slog.Logger

	psCancelFn func()
}

func (s *sub) handleEvent(ctx context.Context, event wspubsub.WorkspaceEvent, err error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	switch event.Kind {
	case wspubsub.WorkspaceEventKindStateChange:
	case wspubsub.WorkspaceEventKindAgentConnectionUpdate:
	case wspubsub.WorkspaceEventKindAgentTimeout:
	case wspubsub.WorkspaceEventKindAgentLifecycleUpdate:
	default:
		if err == nil {
			return
		} else {
			// Always attempt an update if the pubsub lost connection
			s.logger.Warn(ctx, "failed to handle workspace event", slog.Error(err))
		}
	}

	// Use context containing actor
	rows, err := s.db.GetWorkspacesAndAgentsByOwnerID(s.ctx, s.userID)
	if err != nil {
		s.logger.Warn(ctx, "failed to get workspaces and agents by owner ID", slog.Error(err))
		return
	}
	latest := convertRows(rows)

	out, updated := produceUpdate(s.prev, latest)
	if !updated {
		return
	}

	s.prev = latest
	select {
	case <-s.ctx.Done():
		return
	case s.ch <- out:
	}
}

func (s *sub) start(ctx context.Context) (err error) {
	rows, err := s.db.GetWorkspacesAndAgentsByOwnerID(ctx, s.userID)
	if err != nil {
		return xerrors.Errorf("get workspaces and agents by owner ID: %w", err)
	}

	latest := convertRows(rows)
	initUpdate, _ := produceUpdate(workspacesByID{}, latest)
	s.ch <- initUpdate
	s.prev = latest

	cancel, err := s.ps.SubscribeWithErr(wspubsub.WorkspaceEventChannel(s.userID), wspubsub.HandleWorkspaceEvent(s.handleEvent))
	if err != nil {
		return xerrors.Errorf("subscribe to workspace event channel: %w", err)
	}

	s.psCancelFn = cancel
	return nil
}

func (s *sub) Close() error {
	s.cancelFn()

	if s.psCancelFn != nil {
		s.psCancelFn()
	}

	close(s.ch)
	return nil
}

func (s *sub) Updates() <-chan *proto.WorkspaceUpdate {
	return s.ch
}

var _ tailnet.Subscription = (*sub)(nil)

type updatesProvider struct {
	ps     pubsub.Pubsub
	logger slog.Logger
	db     UpdatesQuerier
	auth   rbac.Authorizer

	ctx      context.Context
	cancelFn func()
}

var _ tailnet.WorkspaceUpdatesProvider = (*updatesProvider)(nil)

func NewUpdatesProvider(
	logger slog.Logger,
	ps pubsub.Pubsub,
	db UpdatesQuerier,
	auth rbac.Authorizer,
) tailnet.WorkspaceUpdatesProvider {
	ctx, cancel := context.WithCancel(context.Background())
	out := &updatesProvider{
		auth:     auth,
		db:       db,
		ps:       ps,
		logger:   logger,
		ctx:      ctx,
		cancelFn: cancel,
	}
	return out
}

func (u *updatesProvider) Close() error {
	u.cancelFn()
	return nil
}

// Subscribe subscribes to workspace updates for a user, for the workspaces
// that user is authorized to `ActionRead` on. The provided context must have
// a dbauthz actor set.
func (u *updatesProvider) Subscribe(ctx context.Context, userID uuid.UUID) (tailnet.Subscription, error) {
	actor, ok := dbauthz.ActorFromContext(ctx)
	if !ok {
		return nil, xerrors.Errorf("actor not found in context")
	}
	ctx, cancel := context.WithCancel(u.ctx)
	ctx = dbauthz.As(ctx, actor)
	ch := make(chan *proto.WorkspaceUpdate, 1)
	sub := &sub{
		ctx:      ctx,
		cancelFn: cancel,
		userID:   userID,
		ch:       ch,
		db:       u.db,
		ps:       u.ps,
		logger:   u.logger.Named(fmt.Sprintf("workspace_updates_subscriber_%s", userID)),
		prev:     workspacesByID{},
	}
	err := sub.start(ctx)
	if err != nil {
		_ = sub.Close()
		return nil, err
	}

	return sub, nil
}

func produceUpdate(old, new workspacesByID) (out *proto.WorkspaceUpdate, updated bool) {
	out = &proto.WorkspaceUpdate{
		UpsertedWorkspaces: []*proto.Workspace{},
		UpsertedAgents:     []*proto.Agent{},
		DeletedWorkspaces:  []*proto.Workspace{},
		DeletedAgents:      []*proto.Agent{},
	}

	for wsID, newWorkspace := range new {
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
		if _, exists := new[wsID]; !exists {
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

func convertRows(rows []database.GetWorkspacesAndAgentsByOwnerIDRow) workspacesByID {
	out := workspacesByID{}
	for _, row := range rows {
		agents := []database.AgentIDNamePair{}
		for _, agent := range row.Agents {
			agents = append(agents, database.AgentIDNamePair{
				ID:   agent.ID,
				Name: agent.Name,
			})
		}
		out[row.ID] = ownedWorkspace{
			WorkspaceName: row.Name,
			Status:        tailnet.WorkspaceStatusToProto(codersdk.ConvertWorkspaceStatus(codersdk.ProvisionerJobStatus(row.JobStatus), codersdk.WorkspaceTransition(row.Transition))),
			Agents:        agents,
		}
	}
	return out
}

type rbacAuthorizer struct {
	sshPrep rbac.PreparedAuthorized
	db      UpdatesQuerier
}

func (r *rbacAuthorizer) AuthorizeTunnel(ctx context.Context, agentID uuid.UUID) error {
	ws, err := r.db.GetWorkspaceByAgentID(ctx, agentID)
	if err != nil {
		return xerrors.Errorf("get workspace by agent ID: %w", err)
	}
	// Authorizes against `ActionSSH`
	return r.sshPrep.Authorize(ctx, ws.RBACObject())
}

var _ tailnet.TunnelAuthorizer = (*rbacAuthorizer)(nil)
