package chatd

import (
	"context"
	"encoding/json"
	"testing"

	"charm.land/fantasy"
	"github.com/google/uuid"
	"github.com/sqlc-dev/pqtype"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/aibridge"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbgen"
	"github.com/coder/coder/v2/coderd/database/dbtestutil"
	"github.com/coder/coder/v2/coderd/x/chatd/chatprompt"
	"github.com/coder/coder/v2/coderd/x/chatd/chatprovider"
	"github.com/coder/coder/v2/codersdk"
)

func createParentChatWithInheritedContext(
	ctx context.Context,
	t *testing.T,
	db database.Store,
	server *Server,
) database.Chat {
	t.Helper()

	user, org, model := seedInternalChatDeps(t, db)

	parent, err := server.CreateChat(ctx, CreateOptions{
		OrganizationID:     org.ID,
		OwnerID:            user.ID,
		APIKeyID:           testAPIKeyID(t, db, user.ID),
		Title:              "parent-with-context",
		ModelConfigID:      model.ID,
		InitialUserContent: []codersdk.ChatMessagePart{codersdk.ChatMessageText("hello")},
	})
	require.NoError(t, err)

	inheritedParts := []codersdk.ChatMessagePart{
		{
			Type:                 codersdk.ChatMessagePartTypeContextFile,
			ContextFilePath:      "/home/coder/project/AGENTS.md",
			ContextFileContent:   "# Project instructions",
			ContextFileOS:        "linux",
			ContextFileDirectory: "/home/coder/project",
		},
		{
			Type:                     codersdk.ChatMessagePartTypeSkill,
			SkillName:                "my-skill",
			SkillDescription:         "A test skill",
			SkillDir:                 "/home/coder/project/.agents/skills/my-skill",
			ContextFileSkillMetaFile: "SKILL.md",
		},
		{
			Type:            codersdk.ChatMessagePartTypeContextFile,
			ContextFilePath: "/home/coder/project/.agents/skills/my-skill/SKILL.md",
		},
	}
	content, err := json.Marshal(inheritedParts)
	require.NoError(t, err)

	_ = dbgen.ChatMessage(t, db, database.ChatMessage{
		ChatID:         parent.ID,
		CreatedBy:      uuid.NullUUID{UUID: user.ID, Valid: true},
		ModelConfigID:  uuid.NullUUID{UUID: model.ID, Valid: true},
		Role:           database.ChatMessageRoleUser,
		Content:        pqtype.NullRawMessage{RawMessage: content, Valid: true},
		ContentVersion: chatprompt.CurrentContentVersion,
	})

	parentChat, err := db.GetChatByID(ctx, parent.ID)
	require.NoError(t, err)
	return parentChat
}

func TestSpawnComputerUseAgentCreatesComputerUseChild(t *testing.T) {
	t.Parallel()

	db, ps := dbtestutil.NewDB(t)
	require.NoError(t, db.UpsertChatDesktopEnabled(chatdTestContext(t), true))
	server := newInternalTestServer(t, db, ps, chatprovider.ProviderAPIKeys{})

	ctx := chatdTestContext(t)
	parentChat := createParentChatWithInheritedContext(ctx, t, db, server)
	insertEnabledAnthropicProvider(t, db, parentChat.OwnerID)
	// The direct DB insert above bypasses the pubsub event that
	// production uses to invalidate the provider cache. Explicitly
	// invalidate here so the background processing goroutine does
	// not serve a stale provider list (OpenAI only) that was cached
	// before the Anthropic provider was inserted.
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
}
