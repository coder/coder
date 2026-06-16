package chatd_test

import (
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	agentproto "github.com/coder/coder/v2/agent/proto"
	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbgen"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/codersdk/agentsdk"
	"github.com/coder/coder/v2/provisioner/echo"
	"github.com/coder/coder/v2/testutil"
)

// TestChatContextDirtyFromAgentPush is an end-to-end check of the chat
// context integration. An echo-provisioned workspace agent pushes a context
// snapshot that hydrates a bound chat; a later push with a different hash
// marks the chat dirty; the experimental API reports the dirty state and the
// snapshot error; the refresh endpoint re-pins the latest snapshot and clears
// it; and a re-push of the now-pinned hash stays clean. A second chat bound to
// no agent stays untouched throughout, guarding the agent-scoped queries.
func TestChatContextDirtyFromAgentPush(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitLong)
	client, db := coderdtest.NewWithDatabase(t, &coderdtest.Options{
		DeploymentValues:         directChatRoutingDeploymentValues(t),
		IncludeProvisionerDaemon: true,
	})
	user := coderdtest.CreateFirstUser(t, client)
	expClient := codersdk.NewExperimentalClient(client)

	// Build a workspace with an agent via the echo provisioner.
	agentToken := uuid.NewString()
	version := coderdtest.CreateTemplateVersion(t, client, user.OrganizationID, &echo.Responses{
		Parse:          echo.ParseComplete,
		ProvisionPlan:  echo.PlanComplete,
		ProvisionApply: echo.ApplyComplete,
		ProvisionGraph: echo.ProvisionGraphWithAgent(agentToken),
	})
	coderdtest.AwaitTemplateVersionJobCompleted(t, client, version.ID)
	template := coderdtest.CreateTemplate(t, client, user.OrganizationID, version.ID)
	workspace := coderdtest.CreateWorkspace(t, client, template.ID)
	coderdtest.AwaitWorkspaceBuildJobCompleted(t, client, workspace.LatestBuild.ID)

	ws, err := client.Workspace(ctx, workspace.ID)
	require.NoError(t, err)
	require.Len(t, ws.LatestBuild.Resources, 1)
	require.Len(t, ws.LatestBuild.Resources[0].Agents, 1)
	agentID := ws.LatestBuild.Resources[0].Agents[0].ID

	// A chat bound to the agent. In production agent_id is set lazily during
	// a workspace turn (chatd.persistBuildAgentBinding); bind it directly here
	// so the test exercises the context flow rather than turn resolution.
	// dbgen.ChatModelConfig provisions an AI provider as needed so the chat's
	// last_model_config_id foreign key is satisfied.
	model := dbgen.ChatModelConfig(t, db, database.ChatModelConfig{})
	chat := dbgen.Chat(t, db, database.Chat{
		OrganizationID:    user.OrganizationID,
		OwnerID:           user.UserID,
		WorkspaceID:       uuid.NullUUID{UUID: workspace.ID, Valid: true},
		AgentID:           uuid.NullUUID{UUID: agentID, Valid: true},
		LastModelConfigID: model.ID,
		Status:            database.ChatStatusWaiting,
	})

	// An unrelated chat bound to no agent. The hydrate and dirty queries
	// scope by agent_id, so this chat must stay untouched by every push
	// below; it guards against the scoping clause silently breaking.
	otherChat := dbgen.Chat(t, db, database.Chat{
		OrganizationID:    user.OrganizationID,
		OwnerID:           user.UserID,
		LastModelConfigID: model.ID,
		Status:            database.ChatStatusWaiting,
	})

	// Before any push there is no pinned context.
	got, err := expClient.GetChat(ctx, chat.ID)
	require.NoError(t, err)
	require.Nil(t, got.Context, "no pinned context before the first push")

	requireChatContextNil := func(id uuid.UUID, msg string) {
		t.Helper()
		unrelated, err := expClient.GetChat(ctx, id)
		require.NoError(t, err)
		require.Nil(t, unrelated.Context, msg)
	}
	requireChatContextNil(otherChat.ID, "agent-less chat has no pinned context")

	// Connect as the agent and push the initial snapshot. The push runs the
	// hydrate/dirty fan-out synchronously inside its transaction, so the chat
	// reflects the change by the time the RPC returns.
	agentClient := agentsdk.New(client.URL, agentsdk.WithFixedToken(agentToken))
	aAPI, _, err := agentClient.ConnectRPC210(ctx)
	require.NoError(t, err)
	defer func() { _ = aAPI.DRPCConn().Close() }()

	hashA := []byte{0x01, 0x02, 0x03}
	resp, err := aAPI.PushContextState(ctx, &agentproto.PushContextStateRequest{
		Version:       1,
		Initial:       true,
		AggregateHash: hashA,
	})
	require.NoError(t, err)
	require.True(t, resp.GetAccepted())

	// The initial push hydrates the chat to a clean (not dirty) context.
	got, err = expClient.GetChat(ctx, chat.ID)
	require.NoError(t, err)
	require.NotNil(t, got.Context, "chat should be hydrated after the initial push")
	require.False(t, got.Context.Dirty, "initial hydration is clean")
	require.Nil(t, got.Context.DirtySince)

	// The agent refreshes its context and pushes a different hash carrying a
	// snapshot-level error, which drifts from the pinned hash and marks the
	// chat dirty.
	hashB := []byte{0x04, 0x05, 0x06}
	const snapshotError = "two sources failed to resolve"
	resp, err = aAPI.PushContextState(ctx, &agentproto.PushContextStateRequest{
		Version:       2,
		AggregateHash: hashB,
		SnapshotError: snapshotError,
	})
	require.NoError(t, err)
	require.True(t, resp.GetAccepted())

	got, err = expClient.GetChat(ctx, chat.ID)
	require.NoError(t, err)
	require.NotNil(t, got.Context)
	require.True(t, got.Context.Dirty, "drift should mark the chat dirty")
	require.NotNil(t, got.Context.DirtySince)
	requireChatContextNil(otherChat.ID, "agent-less chat unaffected by the dirty fan-out")

	// Refreshing re-pins the latest snapshot (hash and error) and clears the
	// dirty marker.
	refreshed, err := expClient.RefreshChatContext(ctx, chat.ID)
	require.NoError(t, err)
	require.NotNil(t, refreshed.Context)
	require.False(t, refreshed.Context.Dirty, "refresh clears the dirty marker")
	require.Equal(t, snapshotError, refreshed.Context.Error, "refresh re-pins the snapshot error")

	got, err = expClient.GetChat(ctx, chat.ID)
	require.NoError(t, err)
	require.NotNil(t, got.Context)
	require.False(t, got.Context.Dirty)

	// Re-pushing the now-pinned hash proves the refresh advanced the pin to
	// hashB: a matching hash must not re-dirty the chat.
	resp, err = aAPI.PushContextState(ctx, &agentproto.PushContextStateRequest{
		Version:       3,
		AggregateHash: hashB,
	})
	require.NoError(t, err)
	require.True(t, resp.GetAccepted())

	got, err = expClient.GetChat(ctx, chat.ID)
	require.NoError(t, err)
	require.NotNil(t, got.Context)
	require.False(t, got.Context.Dirty, "re-push of the pinned hash stays clean")
}
