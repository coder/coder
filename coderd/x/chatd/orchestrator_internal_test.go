package chatd

import (
	"encoding/json"
	"testing"

	"charm.land/fantasy"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbgen"
	"github.com/coder/coder/v2/coderd/database/dbtestutil"
	"github.com/coder/coder/v2/coderd/x/chatd/chatprovider"
	"github.com/coder/coder/v2/testutil"
)

// TestOrchestratorToolsOwnershipBoundary verifies read_chat and list_chats
// only expose chats owned by the orchestrator's owner, even when another
// user's chat ID is supplied directly.
func TestOrchestratorToolsOwnershipBoundary(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitShort)
	db, ps := dbtestutil.NewDB(t)
	owner, org, model := seedInternalChatDeps(t, db)
	other := dbgen.User(t, db, database.User{})
	dbgen.OrganizationMember(t, db, database.OrganizationMember{UserID: other.ID, OrganizationID: org.ID})

	orchestrator := dbgen.Chat(t, db, database.Chat{
		OrganizationID:    org.ID,
		OwnerID:           owner.ID,
		LastModelConfigID: model.ID,
		Mode:              database.NullChatMode{ChatMode: database.ChatModeOrchestrator, Valid: true},
		Title:             "Orchestrator",
	})
	owned := dbgen.Chat(t, db, database.Chat{
		OrganizationID:    org.ID,
		OwnerID:           owner.ID,
		LastModelConfigID: model.ID,
		Title:             "owned chat",
	})
	foreign := dbgen.Chat(t, db, database.Chat{
		OrganizationID:    org.ID,
		OwnerID:           other.ID,
		LastModelConfigID: model.ID,
		Title:             "foreign chat",
	})

	server := newInternalTestServer(t, db, ps, chatprovider.ProviderAPIKeys{})
	tools := server.orchestratorTools(func() database.Chat { return orchestrator })

	run := func(name string, args any) map[string]any {
		t.Helper()
		tool := findToolByName(tools, name)
		require.NotNil(t, tool)
		input, err := json.Marshal(args)
		require.NoError(t, err)
		resp, err := tool.Run(ctx, fantasy.ToolCall{
			ID:    uuid.NewString(),
			Name:  name,
			Input: string(input),
		})
		require.NoError(t, err)
		if resp.IsError {
			return map[string]any{"error": resp.Content}
		}
		var decoded map[string]any
		require.NoError(t, json.Unmarshal([]byte(resp.Content), &decoded))
		return decoded
	}

	ownedResult := run(readChatToolName, readChatArgs{ChatID: owned.ID.String()})
	require.Equal(t, owned.ID.String(), ownedResult["chat_id"])
	require.Equal(t, "owned chat", ownedResult["title"])

	foreignResult := run(readChatToolName, readChatArgs{ChatID: foreign.ID.String()})
	require.Equal(t, "chat not found", foreignResult["error"],
		"another user's chat must be indistinguishable from a missing one")

	listResult := run(listChatsToolName, listChatsArgs{})
	chats, ok := listResult["chats"].([]any)
	require.True(t, ok)
	ids := make([]string, 0, len(chats))
	for _, entry := range chats {
		chat, ok := entry.(map[string]any)
		require.True(t, ok)
		ids = append(ids, chat["chat_id"].(string))
	}
	require.ElementsMatch(t, []string{owned.ID.String()}, ids,
		"list_chats must return only owned chats and exclude the orchestrator itself")
}
