package chatd

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbgen"
	"github.com/coder/coder/v2/coderd/database/dbtestutil"
	"github.com/coder/coder/v2/coderd/database/dbtime"
	"github.com/coder/coder/v2/coderd/x/chatd/chatprovider"
	"github.com/coder/coder/v2/coderd/x/chatd/chattool"
	"github.com/coder/coder/v2/testutil"
)

// TestPrepareGenerationMCPDiscoveryBudget verifies preparation waits for a
// pending current-run snapshot only until the agent run's discovery limit,
// and that later preparations, including another chat's on the same agent,
// do not wait again once the limit has passed.
func TestPrepareGenerationMCPDiscoveryBudget(t *testing.T) {
	t.Parallel()
	db, ps := dbtestutil.NewDB(t)
	ctx := chatdTestContext(t)
	user := dbgen.User(t, db, database.User{})
	ws, build, agent := seedWorkspaceBinding(t, db, user.ID)
	dbgen.OrganizationMember(t, db, database.OrganizationMember{UserID: user.ID, OrganizationID: ws.OrganizationID})
	dbgen.AIProviderWithOptionalKey(t, db, database.AIProvider{Type: database.AIProviderTypeOpenai}, "test-key")
	model := insertInternalChatModelConfig(t, db, ws.OrganizationID, "gpt-4o-mini", true)
	newChat := func() database.Chat {
		return dbgen.Chat(t, db, database.Chat{
			OwnerID: user.ID, OrganizationID: ws.OrganizationID, LastModelConfigID: model.ID,
			WorkspaceID: uuid.NullUUID{UUID: ws.ID, Valid: true},
			BuildID:     uuid.NullUUID{UUID: build.ID, Valid: true},
			AgentID:     uuid.NullUUID{UUID: agent.ID, Valid: true},
		})
	}
	chat := newChat()
	require.NoError(t, db.UpdateWorkspaceAgentStartupByID(ctx, database.UpdateWorkspaceAgentStartupByIDParams{
		ID: agent.ID, Version: "v1.0.0", APIVersion: "2.0", Subsystems: []database.WorkspaceAgentSubsystem{}, AgentRunID: "run-a",
	}))
	// Leave a few seconds of the run's limit so the first preparation
	// has something to wait for without spending the full limit.
	startedAt := dbtime.Now().Add(-chattool.MCPDiscoveryWaitTimeout + 3*time.Second)
	require.NoError(t, db.UpdateWorkspaceAgentLifecycleStateByID(ctx, database.UpdateWorkspaceAgentLifecycleStateByIDParams{
		ID: agent.ID, LifecycleState: database.WorkspaceAgentLifecycleStateReady,
		StartedAt:             sql.NullTime{Time: startedAt, Valid: true},
		ReadyAt:               sql.NullTime{Time: startedAt, Valid: true},
		McpDiscoveryStartedAt: sql.NullTime{Time: startedAt, Valid: true},
	}))
	_, err := db.UpsertWorkspaceAgentContextSnapshot(ctx, database.UpsertWorkspaceAgentContextSnapshotParams{
		WorkspaceAgentID: agent.ID, Version: 1, AggregateHash: []byte("pending"),
		ReceivedAt: dbtime.Now(), AgentRunID: "run-a",
		McpDiscoveryPhase: database.WorkspaceAgentMcpDiscoveryPhasePending,
	})
	require.NoError(t, err)
	server := newInternalTestServer(t, db, ps, chatprovider.ProviderAPIKeys{}, withInternalTestServerTransportFactory(&aibridgeTestFactory{}))
	prepare := func(ctx context.Context, chat database.Chat) {
		t.Helper()
		result, err := server.buildTurnEnvironment(ctx, generationPrepareInput{Chat: chat})
		require.NoError(t, err)
		result.Close()
	}
	start := time.Now()
	prepare(ctx, chat)
	require.False(t, time.Now().Before(startedAt.Add(chattool.MCPDiscoveryWaitTimeout)),
		"first preparation must wait for the pending snapshot until the run's limit")
	require.Greater(t, time.Since(start), time.Second, "first preparation must wait")
	// Each call reconstructs preparation state, as steps and later user
	// turns do; another chat on the same agent shares the run's limit.
	for _, next := range []database.Chat{chat, chat, newChat()} {
		nextCtx, cancel := context.WithTimeout(ctx, testutil.WaitShort)
		prepare(nextCtx, next)
		require.NoError(t, nextCtx.Err(), "later preparation must not wait once the run's limit passed")
		cancel()
	}
}

func TestAppendWorkspaceMCPNote(t *testing.T) {
	t.Parallel()
	const summary = "Workspace MCP discovery is still initializing."
	require.Equal(t, "<workspace-context>\nOperating System: linux\n</workspace-context>",
		appendWorkspaceMCPNote("<workspace-context>\nOperating System: linux\n</workspace-context>", ""),
		"an empty summary leaves the block untouched")
	require.Equal(t, "<workspace-context>\nOperating System: linux\n\nWorkspace MCP: "+summary+"\n</workspace-context>",
		appendWorkspaceMCPNote("<workspace-context>\nOperating System: linux\n</workspace-context>", summary))
	require.Equal(t, "<workspace-context>\nWorkspace MCP: "+summary+"\n</workspace-context>",
		appendWorkspaceMCPNote("", summary), "a chat without instruction files still gets the note")
}
