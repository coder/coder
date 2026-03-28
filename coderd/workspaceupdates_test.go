package coderd_test

import (
	"context"
	"encoding/json"
	"slices"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/database/pubsub"
	"github.com/coder/coder/v2/coderd/rbac"
	"github.com/coder/coder/v2/coderd/rbac/policy"
	"github.com/coder/coder/v2/coderd/wspubsub"
	"github.com/coder/coder/v2/tailnet"
	"github.com/coder/coder/v2/tailnet/proto"
	"github.com/coder/coder/v2/testutil"
)

func TestWorkspaceUpdates(t *testing.T) {
	t.Parallel()

	ws1ID := uuid.UUID{0x01}
	ws1IDSlice := tailnet.UUIDToByteSlice(ws1ID)
	agent1ID := uuid.UUID{0x02}
	agent1IDSlice := tailnet.UUIDToByteSlice(agent1ID)
	ws2ID := uuid.UUID{0x03}
	ws2IDSlice := tailnet.UUIDToByteSlice(ws2ID)
	ws3ID := uuid.UUID{0x04}
	ws3IDSlice := tailnet.UUIDToByteSlice(ws3ID)
	agent2ID := uuid.UUID{0x05}
	agent2IDSlice := tailnet.UUIDToByteSlice(agent2ID)
	ws4ID := uuid.UUID{0x06}
	ws4IDSlice := tailnet.UUIDToByteSlice(ws4ID)
	agent3ID := uuid.UUID{0x07}
	agent3IDSlice := tailnet.UUIDToByteSlice(agent3ID)

	ownerID := uuid.UUID{0x08}
	memberRole, err := rbac.RoleByName(rbac.RoleMember())
	require.NoError(t, err)
	ownerSubject := rbac.Subject{
		FriendlyName: "member",
		ID:           ownerID.String(),
		Roles:        rbac.Roles{memberRole},
		Scope:        rbac.ScopeAll,
	}

	t.Run("Basic", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitShort)

		db := &mockWorkspaceStore{
			orderedRows: []database.GetWorkspacesAndAgentsByOwnerIDRow{
				// Gains agent2
				{
					ID:         ws1ID,
					Name:       "ws1",
					JobStatus:  database.ProvisionerJobStatusRunning,
					Transition: database.WorkspaceTransitionStart,
					Agents: []database.AgentIDNamePair{
						{
							ID:   agent1ID,
							Name: "agent1",
						},
					},
				},
				// Changes status
				{
					ID:         ws2ID,
					Name:       "ws2",
					JobStatus:  database.ProvisionerJobStatusRunning,
					Transition: database.WorkspaceTransitionStart,
				},
				// Is deleted
				{
					ID:         ws3ID,
					Name:       "ws3",
					JobStatus:  database.ProvisionerJobStatusSucceeded,
					Transition: database.WorkspaceTransitionStop,
					Agents: []database.AgentIDNamePair{
						{
							ID:   agent3ID,
							Name: "agent3",
						},
					},
				},
			},
		}

		ps := &mockPubsub{
			cbs: map[string]pubsub.ListenerWithErr{},
		}

		updateProvider := coderd.NewUpdatesProvider(testutil.Logger(t), ps, db, &mockAuthorizer{})
		t.Cleanup(func() {
			_ = updateProvider.Close()
		})

		sub, err := updateProvider.Subscribe(dbauthz.As(ctx, ownerSubject), ownerID)
		require.NoError(t, err)
		t.Cleanup(func() {
			_ = sub.Close()
		})

		update := testutil.TryReceive(ctx, t, sub.Updates())
		slices.SortFunc(update.UpsertedWorkspaces, func(a, b *proto.Workspace) int {
			return strings.Compare(a.Name, b.Name)
		})
		slices.SortFunc(update.UpsertedAgents, func(a, b *proto.Agent) int {
			return strings.Compare(a.Name, b.Name)
		})
		require.Equal(t, &proto.WorkspaceUpdate{
			UpsertedWorkspaces: []*proto.Workspace{
				{
					Id:     ws1IDSlice,
					Name:   "ws1",
					Status: proto.Workspace_STARTING,
				},
				{
					Id:     ws2IDSlice,
					Name:   "ws2",
					Status: proto.Workspace_STARTING,
				},
				{
					Id:     ws3IDSlice,
					Name:   "ws3",
					Status: proto.Workspace_STOPPED,
				},
			},
			UpsertedAgents: []*proto.Agent{
				{
					Id:          agent1IDSlice,
					Name:        "agent1",
					WorkspaceId: ws1IDSlice,
				},
				{
					Id:          agent3IDSlice,
					Name:        "agent3",
					WorkspaceId: ws3IDSlice,
				},
			},
			DeletedWorkspaces: []*proto.Workspace{},
			DeletedAgents:     []*proto.Agent{},
		}, update)

		// Update the database
		db.orderedRows = []database.GetWorkspacesAndAgentsByOwnerIDRow{
			{
				ID:         ws1ID,
				Name:       "ws1",
				JobStatus:  database.ProvisionerJobStatusRunning,
				Transition: database.WorkspaceTransitionStart,
				Agents: []database.AgentIDNamePair{
					{
						ID:   agent1ID,
						Name: "agent1",
					},
					{
						ID:   agent2ID,
						Name: "agent2",
					},
				},
			},
			{
				ID:         ws2ID,
				Name:       "ws2",
				JobStatus:  database.ProvisionerJobStatusRunning,
				Transition: database.WorkspaceTransitionStop,
			},
			{
				ID:         ws4ID,
				Name:       "ws4",
				JobStatus:  database.ProvisionerJobStatusRunning,
				Transition: database.WorkspaceTransitionStart,
			},
		}
		publishWorkspaceEvent(t, ps, ownerID, &wspubsub.WorkspaceEvent{
			Kind:        wspubsub.WorkspaceEventKindStateChange,
			WorkspaceID: ws1ID,
		})

		update = testutil.TryReceive(ctx, t, sub.Updates())
		slices.SortFunc(update.UpsertedWorkspaces, func(a, b *proto.Workspace) int {
			return strings.Compare(a.Name, b.Name)
		})
		require.Equal(t, &proto.WorkspaceUpdate{
			UpsertedWorkspaces: []*proto.Workspace{
				{
					// Changed status
					Id:     ws2IDSlice,
					Name:   "ws2",
					Status: proto.Workspace_STOPPING,
				},
				{
					// New workspace
					Id:     ws4IDSlice,
					Name:   "ws4",
					Status: proto.Workspace_STARTING,
				},
			},
			UpsertedAgents: []*proto.Agent{
				{
					Id:          agent2IDSlice,
					Name:        "agent2",
					WorkspaceId: ws1IDSlice,
				},
			},
			DeletedWorkspaces: []*proto.Workspace{
				{
					Id:     ws3IDSlice,
					Name:   "ws3",
					Status: proto.Workspace_STOPPED,
				},
			},
			DeletedAgents: []*proto.Agent{
				{
					Id:          agent3IDSlice,
					Name:        "agent3",
					WorkspaceId: ws3IDSlice,
				},
			},
		}, update)
	})

	t.Run("FiltersChatAgents", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitShort)

		chatWorkspaceID := uuid.UUID{0x09}
		chatWorkspaceIDSlice := tailnet.UUIDToByteSlice(chatWorkspaceID)
		visibleAgentID := uuid.UUID{0x0A}
		visibleAgentIDSlice := tailnet.UUIDToByteSlice(visibleAgentID)
		hiddenAgentID := uuid.UUID{0x0B}
		hiddenAgentUpdatedID := uuid.UUID{0x0C}
		visibleAgentUpdatedID := uuid.UUID{0x0D}
		visibleAgentUpdatedIDSlice := tailnet.UUIDToByteSlice(visibleAgentUpdatedID)
		hiddenDescendantID := uuid.UUID{0x0E}
		hiddenDescendantUpdatedID := uuid.UUID{0x0F}

		db := &mockWorkspaceStore{
			orderedRows: []database.GetWorkspacesAndAgentsByOwnerIDRow{
				{
					ID:         chatWorkspaceID,
					Name:       "chat-workspace",
					JobStatus:  database.ProvisionerJobStatusRunning,
					Transition: database.WorkspaceTransitionStart,
					Agents: []database.AgentIDNamePair{
						{
							ID:   visibleAgentID,
							Name: "agent1",
						},
						{
							ID:   hiddenAgentID,
							Name: "agent1-CODERD-CHAT",
						},
						{
							ID:   hiddenDescendantID,
							Name: "agent1-child",
						},
					},
				},
			},
			workspaceAgents: map[uuid.UUID][]database.WorkspaceAgent{
				chatWorkspaceID: {
					{ID: visibleAgentID, Name: "agent1"},
					{ID: hiddenAgentID, Name: "agent1-CODERD-CHAT"},
					{
						ID:       hiddenDescendantID,
						Name:     "agent1-child",
						ParentID: uuid.NullUUID{UUID: hiddenAgentID, Valid: true},
					},
				},
			},
		}

		ps := &mockPubsub{
			cbs: map[string]pubsub.ListenerWithErr{},
		}

		updateProvider := coderd.NewUpdatesProvider(testutil.Logger(t), ps, db, &mockAuthorizer{})
		t.Cleanup(func() {
			_ = updateProvider.Close()
		})

		sub, err := updateProvider.Subscribe(dbauthz.As(ctx, ownerSubject), ownerID)
		require.NoError(t, err)
		t.Cleanup(func() {
			_ = sub.Close()
		})

		update := testutil.TryReceive(ctx, t, sub.Updates())
		require.Equal(t, &proto.WorkspaceUpdate{
			UpsertedWorkspaces: []*proto.Workspace{
				{
					Id:     chatWorkspaceIDSlice,
					Name:   "chat-workspace",
					Status: proto.Workspace_STARTING,
				},
			},
			UpsertedAgents: []*proto.Agent{
				{
					Id:          visibleAgentIDSlice,
					Name:        "agent1",
					WorkspaceId: chatWorkspaceIDSlice,
				},
			},
			DeletedWorkspaces: []*proto.Workspace{},
			DeletedAgents:     []*proto.Agent{},
		}, update)

		db.orderedRows = []database.GetWorkspacesAndAgentsByOwnerIDRow{
			{
				ID:         chatWorkspaceID,
				Name:       "chat-workspace",
				JobStatus:  database.ProvisionerJobStatusRunning,
				Transition: database.WorkspaceTransitionStart,
				Agents: []database.AgentIDNamePair{
					{
						ID:   visibleAgentID,
						Name: "agent1",
					},
					{
						ID:   hiddenAgentUpdatedID,
						Name: "agent1-coderd-chat",
					},
					{
						ID:   hiddenDescendantUpdatedID,
						Name: "agent1-child-updated",
					},
				},
			},
		}
		db.workspaceAgents[chatWorkspaceID] = []database.WorkspaceAgent{
			{ID: visibleAgentID, Name: "agent1"},
			{ID: hiddenAgentUpdatedID, Name: "agent1-coderd-chat"},
			{
				ID:       hiddenDescendantUpdatedID,
				Name:     "agent1-child-updated",
				ParentID: uuid.NullUUID{UUID: hiddenAgentUpdatedID, Valid: true},
			},
		}
		publishWorkspaceEvent(t, ps, ownerID, &wspubsub.WorkspaceEvent{
			Kind:        wspubsub.WorkspaceEventKindAgentConnectionUpdate,
			WorkspaceID: chatWorkspaceID,
		})
		select {
		case update := <-sub.Updates():
			require.Failf(t, "unexpected update", "%v", update)
		default:
		}

		db.orderedRows = []database.GetWorkspacesAndAgentsByOwnerIDRow{
			{
				ID:         chatWorkspaceID,
				Name:       "chat-workspace",
				JobStatus:  database.ProvisionerJobStatusRunning,
				Transition: database.WorkspaceTransitionStart,
				Agents: []database.AgentIDNamePair{
					{
						ID:   visibleAgentID,
						Name: "agent1",
					},
					{
						ID:   visibleAgentUpdatedID,
						Name: "agent2",
					},
					{
						ID:   hiddenAgentUpdatedID,
						Name: "agent1-coderd-chat",
					},
					{
						ID:   hiddenDescendantUpdatedID,
						Name: "agent1-child-updated",
					},
				},
			},
		}
		db.workspaceAgents[chatWorkspaceID] = []database.WorkspaceAgent{
			{ID: visibleAgentID, Name: "agent1"},
			{ID: visibleAgentUpdatedID, Name: "agent2"},
			{ID: hiddenAgentUpdatedID, Name: "agent1-coderd-chat"},
			{
				ID:       hiddenDescendantUpdatedID,
				Name:     "agent1-child-updated",
				ParentID: uuid.NullUUID{UUID: hiddenAgentUpdatedID, Valid: true},
			},
		}
		publishWorkspaceEvent(t, ps, ownerID, &wspubsub.WorkspaceEvent{
			Kind:        wspubsub.WorkspaceEventKindAgentConnectionUpdate,
			WorkspaceID: chatWorkspaceID,
		})
		update = testutil.TryReceive(ctx, t, sub.Updates())
		require.Equal(t, &proto.WorkspaceUpdate{
			UpsertedWorkspaces: []*proto.Workspace{},
			UpsertedAgents: []*proto.Agent{
				{
					Id:          visibleAgentUpdatedIDSlice,
					Name:        "agent2",
					WorkspaceId: chatWorkspaceIDSlice,
				},
			},
			DeletedWorkspaces: []*proto.Workspace{},
			DeletedAgents:     []*proto.Agent{},
		}, update)
	})

	t.Run("Resubscribe", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitShort)

		db := &mockWorkspaceStore{
			orderedRows: []database.GetWorkspacesAndAgentsByOwnerIDRow{
				{
					ID:         ws1ID,
					Name:       "ws1",
					JobStatus:  database.ProvisionerJobStatusRunning,
					Transition: database.WorkspaceTransitionStart,
					Agents: []database.AgentIDNamePair{
						{
							ID:   agent1ID,
							Name: "agent1",
						},
					},
				},
			},
		}

		ps := &mockPubsub{
			cbs: map[string]pubsub.ListenerWithErr{},
		}

		updateProvider := coderd.NewUpdatesProvider(testutil.Logger(t), ps, db, &mockAuthorizer{})
		t.Cleanup(func() {
			_ = updateProvider.Close()
		})

		sub, err := updateProvider.Subscribe(dbauthz.As(ctx, ownerSubject), ownerID)
		require.NoError(t, err)
		t.Cleanup(func() {
			_ = sub.Close()
		})

		expected := &proto.WorkspaceUpdate{
			UpsertedWorkspaces: []*proto.Workspace{
				{
					Id:     ws1IDSlice,
					Name:   "ws1",
					Status: proto.Workspace_STARTING,
				},
			},
			UpsertedAgents: []*proto.Agent{
				{
					Id:          agent1IDSlice,
					Name:        "agent1",
					WorkspaceId: ws1IDSlice,
				},
			},
			DeletedWorkspaces: []*proto.Workspace{},
			DeletedAgents:     []*proto.Agent{},
		}

		update := testutil.TryReceive(ctx, t, sub.Updates())
		slices.SortFunc(update.UpsertedWorkspaces, func(a, b *proto.Workspace) int {
			return strings.Compare(a.Name, b.Name)
		})
		require.Equal(t, expected, update)

		resub, err := updateProvider.Subscribe(dbauthz.As(ctx, ownerSubject), ownerID)
		require.NoError(t, err)
		t.Cleanup(func() {
			_ = resub.Close()
		})

		update = testutil.TryReceive(ctx, t, resub.Updates())
		slices.SortFunc(update.UpsertedWorkspaces, func(a, b *proto.Workspace) int {
			return strings.Compare(a.Name, b.Name)
		})
		require.Equal(t, expected, update)
	})
}

func publishWorkspaceEvent(t *testing.T, ps pubsub.Pubsub, ownerID uuid.UUID, event *wspubsub.WorkspaceEvent) {
	msg, err := json.Marshal(event)
	require.NoError(t, err)
	ps.Publish(wspubsub.WorkspaceEventChannel(ownerID), msg)
}

type mockWorkspaceStore struct {
	orderedRows     []database.GetWorkspacesAndAgentsByOwnerIDRow
	workspaceAgents map[uuid.UUID][]database.WorkspaceAgent
}

// GetAuthorizedWorkspacesAndAgentsByOwnerID implements coderd.UpdatesQuerier.
func (m *mockWorkspaceStore) GetWorkspacesAndAgentsByOwnerID(context.Context, uuid.UUID) ([]database.GetWorkspacesAndAgentsByOwnerIDRow, error) {
	return m.orderedRows, nil
}

// GetWorkspaceAgentsInLatestBuildByWorkspaceID implements coderd.UpdatesQuerier.
func (m *mockWorkspaceStore) GetWorkspaceAgentsInLatestBuildByWorkspaceID(_ context.Context, workspaceID uuid.UUID) ([]database.WorkspaceAgent, error) {
	if m.workspaceAgents != nil {
		if agents, ok := m.workspaceAgents[workspaceID]; ok {
			return agents, nil
		}
	}
	for _, row := range m.orderedRows {
		if row.ID != workspaceID {
			continue
		}
		agents := make([]database.WorkspaceAgent, 0, len(row.Agents))
		for _, agent := range row.Agents {
			agents = append(agents, database.WorkspaceAgent{
				ID:   agent.ID,
				Name: agent.Name,
			})
		}
		return agents, nil
	}
	return nil, nil
}

// GetWorkspaceByAgentID implements coderd.UpdatesQuerier.
func (*mockWorkspaceStore) GetWorkspaceByAgentID(context.Context, uuid.UUID) (database.Workspace, error) {
	return database.Workspace{}, nil
}

var _ coderd.UpdatesQuerier = (*mockWorkspaceStore)(nil)

type mockPubsub struct {
	cbs map[string]pubsub.ListenerWithErr
}

// Close implements pubsub.Pubsub.
func (*mockPubsub) Close() error {
	panic("unimplemented")
}

// Publish implements pubsub.Pubsub.
func (m *mockPubsub) Publish(event string, message []byte) error {
	cb, ok := m.cbs[event]
	if !ok {
		return nil
	}
	cb(context.Background(), message, nil)
	return nil
}

func (*mockPubsub) Subscribe(string, pubsub.Listener) (cancel func(), err error) {
	panic("unimplemented")
}

func (m *mockPubsub) SubscribeWithErr(event string, listener pubsub.ListenerWithErr) (func(), error) {
	m.cbs[event] = listener
	return func() {}, nil
}

var _ pubsub.Pubsub = (*mockPubsub)(nil)

type mockAuthorizer struct{}

func (*mockAuthorizer) Authorize(context.Context, rbac.Subject, policy.Action, rbac.Object) error {
	return nil
}

// Prepare implements rbac.Authorizer.
func (*mockAuthorizer) Prepare(context.Context, rbac.Subject, policy.Action, string) (rbac.PreparedAuthorized, error) {
	//nolint:nilnil
	return nil, nil
}

var _ rbac.Authorizer = (*mockAuthorizer)(nil)
