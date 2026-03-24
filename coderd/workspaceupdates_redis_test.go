package coderd_test

import (
	"slices"
	"strings"
	"testing"

	"github.com/alicebob/miniredis/v2"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/database/pubsub"
	"github.com/coder/coder/v2/coderd/rbac"
	"github.com/coder/coder/v2/coderd/wspubsub"
	"github.com/coder/coder/v2/tailnet/proto"
	"github.com/coder/coder/v2/testutil"
)

func TestWorkspaceUpdates_RedisPubsub(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitLong)
	redisServer := miniredis.RunT(t)
	ps, err := pubsub.NewRedis(ctx, testutil.Logger(t), "redis://"+redisServer.Addr())
	require.NoError(t, err)
	defer ps.Close()

	ownerID := uuid.New()
	memberRole, err := rbac.RoleByName(rbac.RoleMember())
	require.NoError(t, err)
	ownerSubject := rbac.Subject{
		FriendlyName: "member",
		ID:           ownerID.String(),
		Roles:        rbac.Roles{memberRole},
		Scope:        rbac.ScopeAll,
	}

	ws1ID := uuid.New()
	ws2ID := uuid.New()
	db := &mockWorkspaceStore{
		orderedRows: []database.GetWorkspacesAndAgentsByOwnerIDRow{
			{
				ID:         ws1ID,
				Name:       "ws1",
				JobStatus:  database.ProvisionerJobStatusRunning,
				Transition: database.WorkspaceTransitionStart,
			},
		},
	}

	updateProvider := coderd.NewUpdatesProvider(testutil.Logger(t), ps, db, &mockAuthorizer{})
	defer updateProvider.Close()

	sub, err := updateProvider.Subscribe(dbauthz.As(ctx, ownerSubject), ownerID)
	require.NoError(t, err)
	defer sub.Close()

	initial := testutil.TryReceive(ctx, t, sub.Updates())
	require.Len(t, initial.UpsertedWorkspaces, 1)
	require.Equal(t, "ws1", initial.UpsertedWorkspaces[0].Name)
	require.Equal(t, proto.Workspace_STARTING, initial.UpsertedWorkspaces[0].Status)

	db.orderedRows = []database.GetWorkspacesAndAgentsByOwnerIDRow{
		{
			ID:         ws1ID,
			Name:       "ws1",
			JobStatus:  database.ProvisionerJobStatusRunning,
			Transition: database.WorkspaceTransitionStop,
		},
		{
			ID:         ws2ID,
			Name:       "ws2",
			JobStatus:  database.ProvisionerJobStatusRunning,
			Transition: database.WorkspaceTransitionStart,
		},
	}
	publishWorkspaceEvent(t, ps, ownerID, &wspubsub.WorkspaceEvent{
		Kind:        wspubsub.WorkspaceEventKindStateChange,
		WorkspaceID: ws1ID,
	})

	update := testutil.TryReceive(ctx, t, sub.Updates())
	slices.SortFunc(update.UpsertedWorkspaces, func(a, b *proto.Workspace) int {
		return strings.Compare(a.Name, b.Name)
	})
	require.Equal(t, []string{"ws1", "ws2"}, []string{
		update.UpsertedWorkspaces[0].Name,
		update.UpsertedWorkspaces[1].Name,
	})
	require.Equal(t, proto.Workspace_STOPPING, update.UpsertedWorkspaces[0].Status)
	require.Equal(t, proto.Workspace_STARTING, update.UpsertedWorkspaces[1].Status)
}
