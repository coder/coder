package chatd

import (
	"context"
	"database/sql"
	"encoding/json"
	"testing"

	"charm.land/fantasy"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/aibridge"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbgen"
	"github.com/coder/coder/v2/coderd/database/dbtestutil"
	"github.com/coder/coder/v2/coderd/database/dbtime"
	"github.com/coder/coder/v2/coderd/x/chatd/chatprovider"
	"github.com/coder/coder/v2/codersdk"
)

// createWorkspaceBoundParentChat creates a parent chat bound to a workspace
// agent that has already pushed a context snapshot, so CreateChat pins the chat
// to that snapshot at create time. It returns the pinned parent and the source
// of its single pinned resource, letting a spawned child be asserted to inherit
// the same pin.
func createWorkspaceBoundParentChat(
	ctx context.Context,
	t *testing.T,
	db database.Store,
	server *Server,
) (database.Chat, string) {
	t.Helper()

	user, org, model := seedInternalChatDeps(t, db)

	tv := dbgen.TemplateVersion(t, db, database.TemplateVersion{
		OrganizationID: org.ID,
		CreatedBy:      user.ID,
	})
	tmpl := dbgen.Template(t, db, database.Template{
		OrganizationID:  org.ID,
		ActiveVersionID: tv.ID,
		CreatedBy:       user.ID,
	})
	ws := dbgen.Workspace(t, db, database.WorkspaceTable{
		OwnerID:        user.ID,
		OrganizationID: org.ID,
		TemplateID:     tmpl.ID,
	})
	pj := dbgen.ProvisionerJob(t, db, nil, database.ProvisionerJob{
		OrganizationID: org.ID,
		CompletedAt:    sql.NullTime{Valid: true, Time: dbtime.Now()},
	})
	build := dbgen.WorkspaceBuild(t, db, database.WorkspaceBuild{
		WorkspaceID:       ws.ID,
		TemplateVersionID: tv.ID,
		JobID:             pj.ID,
		Transition:        database.WorkspaceTransitionStart,
	})
	res := dbgen.WorkspaceResource(t, db, database.WorkspaceResource{
		Transition: database.WorkspaceTransitionStart,
		JobID:      pj.ID,
	})
	agent := dbgen.WorkspaceAgent(t, db, database.WorkspaceAgent{ResourceID: res.ID})

	const source = "/home/coder/project/AGENTS.md"
	seedAgentContext(ctx, t, db, agent.ID, source, []byte("instruction:parent"),
		database.WorkspaceAgentContextBodyKindInstructionFile,
		json.RawMessage(`{"instruction_file":{"content":"parent"}}`))

	parent, err := server.CreateChat(ctx, CreateOptions{
		OrganizationID:     org.ID,
		OwnerID:            user.ID,
		APIKeyID:           testAPIKeyID(t, db, user.ID),
		Title:              "parent-with-context",
		ModelConfigID:      model.ID,
		WorkspaceID:        uuid.NullUUID{UUID: ws.ID, Valid: true},
		BuildID:            uuid.NullUUID{UUID: build.ID, Valid: true},
		AgentID:            uuid.NullUUID{UUID: agent.ID, Valid: true},
		InitialUserContent: []codersdk.ChatMessagePart{codersdk.ChatMessageText("hello")},
	})
	require.NoError(t, err)

	parentChat, err := db.GetChatByID(ctx, parent.ID)
	require.NoError(t, err)
	return parentChat, source
}

// TestSpawnComputerUseAgentInheritsPinnedContext verifies that a spawned
// subagent inherits its parent's pinned workspace context. The child shares the
// parent's workspace agent, so create-time hydration pins it to the same
// snapshot instead of copying any context through chat history. It also asserts
// the child is created in computer_use mode.
func TestSpawnComputerUseAgentInheritsPinnedContext(t *testing.T) {
	t.Parallel()

	db, ps := dbtestutil.NewDB(t)
	server := newInternalTestServer(t, db, ps, chatprovider.ProviderAPIKeys{})

	ctx := chatdTestContext(t)
	parentChat, wantSource := createWorkspaceBoundParentChat(ctx, t, db, server)

	parentRes, err := db.ListChatContextResourcesByChatID(ctx, parentChat.ID)
	require.NoError(t, err)
	require.Len(t, parentRes, 1, "parent is pinned to its agent's snapshot at create")
	require.Equal(t, wantSource, parentRes[0].Source)
	require.NotNil(t, parentChat.ContextAggregateHash)

	insertEnabledAnthropicProvider(t, db, parentChat.OwnerID)
	// The direct DB insert above bypasses the pubsub event that production
	// uses to invalidate the provider cache. Explicitly invalidate here so the
	// background processing goroutine does not serve a stale provider list
	// (OpenAI only) that was cached before the Anthropic provider was inserted.
	server.configCache.InvalidateProviders()

	ctx = aibridge.WithDelegatedAPIKeyID(ctx, testAPIKeyID(t, db, parentChat.OwnerID))
	tools := server.subagentTools(ctx, func() database.Chat { return parentChat }, parentChat.LastModelConfigID)
	tool := findToolByName(tools, spawnAgentToolName)
	require.NotNil(t, tool)

	resp, err := tool.Run(ctx, fantasy.ToolCall{
		ID:    "call-context",
		Name:  spawnAgentToolName,
		Input: `{"type":"computer_use","prompt":"inspect bindings"}`,
	})
	require.NoError(t, err)
	require.False(t, resp.IsError, "expected success but got: %s", resp.Content)

	var result map[string]any
	require.NoError(t, json.Unmarshal([]byte(resp.Content), &result))
	childIDStr, ok := result["chat_id"].(string)
	require.True(t, ok)

	childID, err := uuid.Parse(childIDStr)
	require.NoError(t, err)

	childChat, err := db.GetChatByID(ctx, childID)
	require.NoError(t, err)
	require.True(t, childChat.Mode.Valid)
	require.Equal(t, database.ChatModeComputerUse, childChat.Mode.ChatMode)

	// The child shares the parent's workspace agent, so create-time hydration
	// pins it to the same snapshot: it inherits the parent's pinned resources
	// verbatim, with nothing copied through chat history.
	childRes, err := db.ListChatContextResourcesByChatID(ctx, childID)
	require.NoError(t, err)
	require.Len(t, childRes, len(parentRes), "child inherits the parent's pinned resource set")
	require.Equal(t, parentRes[0].Source, childRes[0].Source)
	require.Equal(t, parentRes[0].ContentHash, childRes[0].ContentHash)
	require.JSONEq(t, string(parentRes[0].Body), string(childRes[0].Body))
	require.Equal(t, parentChat.ContextAggregateHash, childChat.ContextAggregateHash,
		"child pins the same aggregate hash as the parent")
}
