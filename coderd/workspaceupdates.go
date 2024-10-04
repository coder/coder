package coderd

import (
	"context"
	"fmt"
	"sync"

	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/pubsub"
	"github.com/coder/coder/v2/coderd/wspubsub"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/tailnet"
	"github.com/coder/coder/v2/tailnet/proto"
)

type ownedAgent struct {
	AgentName   string
	WorkspaceID uuid.UUID
}

type ownedWorkspace struct {
	WorkspaceName string
	Status        proto.Workspace_Status
}

func convertStatus(status database.ProvisionerJobStatus, trans database.WorkspaceTransition) proto.Workspace_Status {
	wsStatus := codersdk.ConvertWorkspaceStatus(codersdk.ProvisionerJobStatus(status), codersdk.WorkspaceTransition(trans))
	return tailnet.WorkspaceStatusToProto(wsStatus)
}

type sub struct {
	mu         sync.RWMutex
	userID     uuid.UUID
	tx         chan<- *proto.WorkspaceUpdate
	workspaces map[uuid.UUID]ownedWorkspace
	agents     map[uuid.UUID]ownedAgent

	db UpdateQuerier
	ps pubsub.Pubsub

	cancelFn func()
}

func (s *sub) ownsAgent(agentID uuid.UUID) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()

	_, exists := s.agents[agentID]
	return exists
}

func (s *sub) handleEvent(_ context.Context, event wspubsub.WorkspaceEvent) {
	s.mu.Lock()
	defer s.mu.Unlock()

	out := &proto.WorkspaceUpdate{
		UpsertedWorkspaces: []*proto.Workspace{},
		UpsertedAgents:     []*proto.Agent{},
		DeletedWorkspaces:  []*proto.Workspace{},
		DeletedAgents:      []*proto.Agent{},
	}

	switch event.Kind {
	case wspubsub.WorkspaceEventKindAgentUpdate:
		_, _ = fmt.Printf("event: %s\n agentid: %s\n agentname: %s\n workspaceid: %s\n", event.Kind, *event.AgentID, *event.AgentName, event.WorkspaceID)
		out.UpsertedAgents = append(out.UpsertedAgents, &proto.Agent{
			WorkspaceId: tailnet.UUIDToByteSlice(event.WorkspaceID),
			Id:          tailnet.UUIDToByteSlice(*event.AgentID),
			Name:        *event.AgentName,
		})
		s.agents[*event.AgentID] = ownedAgent{
			AgentName:   *event.AgentName,
			WorkspaceID: event.WorkspaceID,
		}
	case wspubsub.WorkspaceEventKindStateChange:
		_, _ = fmt.Printf("event: %s\n jobstatus: %s\n transition: %s\n workspaceid: %s\n", event.Kind, *event.JobStatus, *event.Transition, event.WorkspaceID)
		status := convertStatus(*event.JobStatus, *event.Transition)
		wsProto := &proto.Workspace{
			Id:     tailnet.UUIDToByteSlice(event.WorkspaceID),
			Name:   *event.WorkspaceName,
			Status: status,
		}
		ws := ownedWorkspace{
			WorkspaceName: *event.WorkspaceName,
			Status:        status,
		}
		// State unchanged
		if s.workspaces[event.WorkspaceID] == ws {
			// TODO: We could log here to identify spurious events
			return
		}
		// Deleted
		if *event.Transition == database.WorkspaceTransitionDelete &&
			*event.JobStatus == database.ProvisionerJobStatusSucceeded {
			out.DeletedWorkspaces = append(out.DeletedWorkspaces, wsProto)
			for agentID, agent := range s.agents {
				if agent.WorkspaceID == event.WorkspaceID {
					delete(s.agents, agentID)
					out.DeletedAgents = append(out.DeletedAgents, &proto.Agent{
						Id: tailnet.UUIDToByteSlice(agentID),
					})
				}
			}
		} else {
			// Upserted
			out.UpsertedWorkspaces = append(out.UpsertedWorkspaces, wsProto)
			s.workspaces[event.WorkspaceID] = ws
		}
	default:
		_, _ = fmt.Printf("other event: %s\n", event.Kind)
		return
	}
	// TODO: can this send on a closed channel?
	s.tx <- out
}

func (s *sub) start() (err error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	rows, err := s.db.GetWorkspacesAndAgentsByOwnerID(context.Background(), s.userID)
	if err != nil {
		return xerrors.Errorf("get workspaces and agents by owner ID: %w", err)
	}

	initUpdate := initialUpdate(rows)
	s.tx <- initUpdate
	s.workspaces, s.agents = initialState(rows)

	cancel, err := s.ps.Subscribe(wspubsub.WorkspaceEventChannel(s.userID), wspubsub.HandleWorkspaceEvent(s.handleEvent))
	if err != nil {
		return xerrors.Errorf("subscribe to workspace event channel: %w", err)
	}

	s.cancelFn = cancel
	return nil
}

func (s *sub) stop() {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.cancelFn != nil {
		s.cancelFn()
	}

	close(s.tx)
}

type UpdateQuerier interface {
	GetWorkspacesAndAgentsByOwnerID(ctx context.Context, ownerID uuid.UUID) ([]database.GetWorkspacesAndAgentsByOwnerIDRow, error)
}

type updatesProvider struct {
	mu sync.RWMutex
	// Peer ID -> subscription
	subs map[uuid.UUID]*sub

	db UpdateQuerier
	ps pubsub.Pubsub
}

func (u *updatesProvider) IsOwner(userID uuid.UUID, agentID uuid.UUID) bool {
	u.mu.RLock()
	defer u.mu.RUnlock()

	for _, sub := range u.subs {
		if sub.userID == userID && sub.ownsAgent(agentID) {
			return true
		}
	}
	return false
}

var _ tailnet.WorkspaceUpdatesProvider = (*updatesProvider)(nil)

func NewUpdatesProvider(_ context.Context, db UpdateQuerier, ps pubsub.Pubsub) (tailnet.WorkspaceUpdatesProvider, error) {
	out := &updatesProvider{
		db:   db,
		ps:   ps,
		subs: map[uuid.UUID]*sub{},
	}
	return out, nil
}

func (u *updatesProvider) Stop() {
	for _, sub := range u.subs {
		sub.stop()
	}
}

func (u *updatesProvider) Subscribe(peerID uuid.UUID, userID uuid.UUID) (<-chan *proto.WorkspaceUpdate, error) {
	u.mu.Lock()
	defer u.mu.Unlock()

	tx := make(chan *proto.WorkspaceUpdate, 1)
	sub := &sub{
		userID:     userID,
		tx:         tx,
		db:         u.db,
		ps:         u.ps,
		agents:     map[uuid.UUID]ownedAgent{},
		workspaces: map[uuid.UUID]ownedWorkspace{},
	}
	err := sub.start()
	if err != nil {
		sub.stop()
		return nil, err
	}

	u.subs[peerID] = sub
	return tx, nil
}

func (u *updatesProvider) Unsubscribe(peerID uuid.UUID) {
	u.mu.Lock()
	defer u.mu.Unlock()

	sub, exists := u.subs[peerID]
	if !exists {
		return
	}
	sub.stop()
	delete(u.subs, peerID)
}

func initialUpdate(rows []database.GetWorkspacesAndAgentsByOwnerIDRow) *proto.WorkspaceUpdate {
	out := &proto.WorkspaceUpdate{
		UpsertedWorkspaces: []*proto.Workspace{},
		UpsertedAgents:     []*proto.Agent{},
		DeletedWorkspaces:  []*proto.Workspace{},
		DeletedAgents:      []*proto.Agent{},
	}

	for _, row := range rows {
		out.UpsertedWorkspaces = append(out.UpsertedWorkspaces, &proto.Workspace{
			Id:     tailnet.UUIDToByteSlice(row.ID),
			Name:   row.Name,
			Status: convertStatus(row.JobStatus, row.Transition),
		})
		for _, agent := range row.Agents {
			out.UpsertedAgents = append(out.UpsertedAgents, &proto.Agent{
				Id:          tailnet.UUIDToByteSlice(agent.ID),
				Name:        agent.Name,
				WorkspaceId: tailnet.UUIDToByteSlice(row.ID),
			})
		}
	}
	return out
}

func initialState(rows []database.GetWorkspacesAndAgentsByOwnerIDRow) (map[uuid.UUID]ownedWorkspace, map[uuid.UUID]ownedAgent) {
	agents := make(map[uuid.UUID]ownedAgent)
	workspaces := make(map[uuid.UUID]ownedWorkspace)
	for _, row := range rows {
		workspaces[row.ID] = ownedWorkspace{
			WorkspaceName: row.Name,
			Status:        convertStatus(row.JobStatus, row.Transition),
		}
		for _, agent := range row.Agents {
			agents[agent.ID] = ownedAgent{
				AgentName:   agent.Name,
				WorkspaceID: row.ID,
			}
		}
	}
	return workspaces, agents
}
