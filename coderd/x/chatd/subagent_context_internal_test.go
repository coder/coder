package chatd

import (
	"context"
	"encoding/json"
	"testing"

	"charm.land/fantasy"
	"github.com/google/uuid"
	"github.com/sqlc-dev/pqtype"
	"github.com/stretchr/testify/require"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbtestutil"
	"github.com/coder/coder/v2/coderd/x/chatd/chatprompt"
	"github.com/coder/coder/v2/coderd/x/chatd/chatprovider"
	"github.com/coder/coder/v2/codersdk"
)

func TestCollectContextPartsFromMessagesSkipsSentinelContextFiles(t *testing.T) {
	t.Parallel()

	content, err := json.Marshal([]codersdk.ChatMessagePart{
		{
			Type:            codersdk.ChatMessagePartTypeContextFile,
			ContextFilePath: "/home/coder/project/.agents/skills/my-skill/SKILL.md",
		},
		{
			Type:             codersdk.ChatMessagePartTypeSkill,
			SkillName:        "my-skill",
			SkillDescription: "A test skill",
		},
		{
			Type:               codersdk.ChatMessagePartTypeContextFile,
			ContextFilePath:    "/home/coder/project/AGENTS.md",
			ContextFileContent: "# Project instructions",
		},
		codersdk.ChatMessageText("ignored"),
	})
	require.NoError(t, err)

	parts, err := CollectContextPartsFromMessages(context.Background(), slog.Make(), []database.ChatMessage{ //nolint:exhaustruct // Only content fields matter for this unit test.
		{
			ID: 1,
			Content: pqtype.NullRawMessage{
				RawMessage: content,
				Valid:      true,
			},
		},
	}, false)
	require.NoError(t, err)
	require.Len(t, parts, 2)
	require.Equal(t, codersdk.ChatMessagePartTypeSkill, parts[0].Type)
	require.Equal(t, "my-skill", parts[0].SkillName)
	require.Equal(t, codersdk.ChatMessagePartTypeContextFile, parts[1].Type)
	require.Equal(t, "/home/coder/project/AGENTS.md", parts[1].ContextFilePath)
	require.Equal(t, "# Project instructions", parts[1].ContextFileContent)
}

func TestCollectContextPartsFromMessagesKeepsEmptyContextFilesWhenRequested(t *testing.T) {
	t.Parallel()

	content, err := json.Marshal([]codersdk.ChatMessagePart{
		{
			Type:            codersdk.ChatMessagePartTypeContextFile,
			ContextFilePath: AgentChatContextSentinelPath,
			ContextFileAgentID: uuid.NullUUID{
				UUID:  uuid.New(),
				Valid: true,
			},
		},
		{
			Type:      codersdk.ChatMessagePartTypeSkill,
			SkillName: "my-skill",
		},
	})
	require.NoError(t, err)

	parts, err := CollectContextPartsFromMessages(context.Background(), slog.Make(), []database.ChatMessage{ //nolint:exhaustruct // Only content fields matter for this unit test.
		{
			ID: 1,
			Content: pqtype.NullRawMessage{
				RawMessage: content,
				Valid:      true,
			},
		},
	}, true)
	require.NoError(t, err)
	require.Len(t, parts, 2)
	require.Equal(t, AgentChatContextSentinelPath, parts[0].ContextFilePath)
	require.Equal(t, "my-skill", parts[1].SkillName)
}

func TestFilterContextPartsToLatestAgent(t *testing.T) {
	t.Parallel()

	oldAgentID := uuid.New()
	newAgentID := uuid.New()
	parts := []codersdk.ChatMessagePart{
		{
			Type:               codersdk.ChatMessagePartTypeContextFile,
			ContextFilePath:    "/legacy/AGENTS.md",
			ContextFileContent: "legacy instructions",
		},
		{
			Type:      codersdk.ChatMessagePartTypeSkill,
			SkillName: "repo-helper-legacy",
		},
		{
			Type:               codersdk.ChatMessagePartTypeContextFile,
			ContextFilePath:    "/old/AGENTS.md",
			ContextFileAgentID: uuid.NullUUID{UUID: oldAgentID, Valid: true},
		},
		{
			Type:               codersdk.ChatMessagePartTypeSkill,
			SkillName:          "repo-helper-old",
			ContextFileAgentID: uuid.NullUUID{UUID: oldAgentID, Valid: true},
		},
		{
			Type:            codersdk.ChatMessagePartTypeContextFile,
			ContextFilePath: AgentChatContextSentinelPath,
			ContextFileAgentID: uuid.NullUUID{
				UUID:  newAgentID,
				Valid: true,
			},
		},
		{
			Type:               codersdk.ChatMessagePartTypeSkill,
			SkillName:          "repo-helper-new",
			ContextFileAgentID: uuid.NullUUID{UUID: newAgentID, Valid: true},
		},
	}

	got := FilterContextPartsToLatestAgent(parts)
	require.Len(t, got, 4)
	require.Equal(t, "/legacy/AGENTS.md", got[0].ContextFilePath)
	require.Equal(t, "repo-helper-legacy", got[1].SkillName)
	require.Equal(t, AgentChatContextSentinelPath, got[2].ContextFilePath)
	require.Equal(t, "repo-helper-new", got[3].SkillName)
}

func createParentChatWithInheritedContext(
	ctx context.Context,
	t *testing.T,
	db database.Store,
	server *Server,
) database.Chat {
	t.Helper()

	user, org, model := seedInternalChatDeps(ctx, t, db)

	parent, err := server.CreateChat(ctx, CreateOptions{
		OrganizationID:     org.ID,
		OwnerID:            user.ID,
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

	_, err = db.InsertChatMessages(ctx, database.InsertChatMessagesParams{
		ChatID:              parent.ID,
		CreatedBy:           []uuid.UUID{user.ID},
		ModelConfigID:       []uuid.UUID{model.ID},
		Role:                []database.ChatMessageRole{database.ChatMessageRoleUser},
		Content:             []string{string(content)},
		ContentVersion:      []int16{chatprompt.CurrentContentVersion},
		Visibility:          []database.ChatMessageVisibility{database.ChatMessageVisibilityBoth},
		InputTokens:         []int64{0},
		OutputTokens:        []int64{0},
		TotalTokens:         []int64{0},
		ReasoningTokens:     []int64{0},
		CacheCreationTokens: []int64{0},
		CacheReadTokens:     []int64{0},
		ContextLimit:        []int64{0},
		Compressed:          []bool{false},
		TotalCostMicros:     []int64{0},
		RuntimeMs:           []int64{0},
	})
	require.NoError(t, err)

	parentChat, err := db.GetChatByID(ctx, parent.ID)
	require.NoError(t, err)
	return parentChat
}

func assertChildInheritedContext(
	ctx context.Context,
	t *testing.T,
	db database.Store,
	childID uuid.UUID,
	prompt string,
) {
	t.Helper()

	childChat, err := db.GetChatByID(ctx, childID)
	require.NoError(t, err)
	require.True(t, childChat.LastInjectedContext.Valid)

	var cached []codersdk.ChatMessagePart
	require.NoError(t, json.Unmarshal(childChat.LastInjectedContext.RawMessage, &cached))
	require.Len(t, cached, 2)

	var sawContextFile bool
	var sawSkill bool
	for _, part := range cached {
		switch part.Type {
		case codersdk.ChatMessagePartTypeContextFile:
			sawContextFile = true
			require.Equal(t, "/home/coder/project/AGENTS.md", part.ContextFilePath)
			require.Empty(t, part.ContextFileContent)
			require.Empty(t, part.ContextFileOS)
			require.Empty(t, part.ContextFileDirectory)
		case codersdk.ChatMessagePartTypeSkill:
			sawSkill = true
			require.Equal(t, "my-skill", part.SkillName)
			require.Equal(t, "A test skill", part.SkillDescription)
			require.Empty(t, part.SkillDir)
			require.Empty(t, part.ContextFileSkillMetaFile)
		default:
			t.Fatalf("unexpected cached part type %q", part.Type)
		}
	}
	require.True(t, sawContextFile)
	require.True(t, sawSkill)

	childMessages, err := db.GetChatMessagesByChatID(ctx, database.GetChatMessagesByChatIDParams{
		ChatID:  childID,
		AfterID: 0,
	})
	require.NoError(t, err)

	var (
		contextMessageIndexes      []int
		userPromptIndex            = -1
		sawDBAgentsContextFile     bool
		sawDBSkillCompanionContext bool
		sawDBSkill                 bool
	)
	for i, msg := range childMessages {
		if !msg.Content.Valid {
			continue
		}

		var parts []codersdk.ChatMessagePart
		require.NoError(t, json.Unmarshal(msg.Content.RawMessage, &parts))

		if len(parts) == 1 && parts[0].Type == codersdk.ChatMessagePartTypeText && parts[0].Text == prompt {
			require.Equal(t, database.ChatMessageRoleUser, msg.Role)
			userPromptIndex = i
			continue
		}

		hasInheritedContext := false
		for _, part := range parts {
			switch part.Type {
			case codersdk.ChatMessagePartTypeContextFile:
				hasInheritedContext = true
				switch part.ContextFilePath {
				case "/home/coder/project/AGENTS.md":
					sawDBAgentsContextFile = true
					require.Equal(t, "# Project instructions", part.ContextFileContent)
					require.Equal(t, "linux", part.ContextFileOS)
					require.Equal(t, "/home/coder/project", part.ContextFileDirectory)
				case "/home/coder/project/.agents/skills/my-skill/SKILL.md":
					sawDBSkillCompanionContext = true
					require.Empty(t, part.ContextFileContent)
					require.Empty(t, part.ContextFileOS)
					require.Empty(t, part.ContextFileDirectory)
				default:
					t.Fatalf("unexpected child inherited context file path %q", part.ContextFilePath)
				}
			case codersdk.ChatMessagePartTypeSkill:
				hasInheritedContext = true
				sawDBSkill = true
				require.Equal(t, "my-skill", part.SkillName)
				require.Equal(t, "A test skill", part.SkillDescription)
				require.Equal(t, "/home/coder/project/.agents/skills/my-skill", part.SkillDir)
				require.Equal(t, "SKILL.md", part.ContextFileSkillMetaFile)
			default:
				t.Fatalf("unexpected child inherited part type %q", part.Type)
			}
		}
		if hasInheritedContext {
			require.Equal(t, database.ChatMessageRoleUser, msg.Role)
			contextMessageIndexes = append(contextMessageIndexes, i)
		}
	}

	require.NotEmpty(t, contextMessageIndexes)
	require.NotEqual(t, -1, userPromptIndex)
	for _, idx := range contextMessageIndexes {
		require.Less(t, idx, userPromptIndex)
	}
	require.True(t, sawDBAgentsContextFile)
	require.True(t, sawDBSkillCompanionContext)
	require.True(t, sawDBSkill)
}

func createParentChatWithRotatedInheritedContext(
	ctx context.Context,
	t *testing.T,
	db database.Store,
	server *Server,
) database.Chat {
	t.Helper()

	user, org, model := seedInternalChatDeps(ctx, t, db)

	parent, err := server.CreateChat(ctx, CreateOptions{
		OrganizationID:     org.ID,
		OwnerID:            user.ID,
		Title:              "parent-with-rotated-context",
		ModelConfigID:      model.ID,
		InitialUserContent: []codersdk.ChatMessagePart{codersdk.ChatMessageText("hello")},
	})
	require.NoError(t, err)

	oldAgentID := uuid.New()
	newAgentID := uuid.New()
	oldContent, err := json.Marshal([]codersdk.ChatMessagePart{
		{
			Type:                 codersdk.ChatMessagePartTypeContextFile,
			ContextFilePath:      "/home/coder/project-old/AGENTS.md",
			ContextFileContent:   "# Old instructions",
			ContextFileOS:        "darwin",
			ContextFileDirectory: "/home/coder/project-old",
			ContextFileAgentID:   uuid.NullUUID{UUID: oldAgentID, Valid: true},
		},
		{
			Type:               codersdk.ChatMessagePartTypeSkill,
			SkillName:          "old-skill",
			SkillDescription:   "Old skill",
			SkillDir:           "/home/coder/project-old/.agents/skills/old-skill",
			ContextFileAgentID: uuid.NullUUID{UUID: oldAgentID, Valid: true},
		},
	})
	require.NoError(t, err)
	newContent, err := json.Marshal([]codersdk.ChatMessagePart{
		{
			Type:                 codersdk.ChatMessagePartTypeContextFile,
			ContextFilePath:      "/home/coder/project-new/AGENTS.md",
			ContextFileContent:   "# New instructions",
			ContextFileOS:        "linux",
			ContextFileDirectory: "/home/coder/project-new",
			ContextFileAgentID:   uuid.NullUUID{UUID: newAgentID, Valid: true},
		},
		{
			Type:               codersdk.ChatMessagePartTypeSkill,
			SkillName:          "new-skill",
			SkillDescription:   "New skill",
			SkillDir:           "/home/coder/project-new/.agents/skills/new-skill",
			ContextFileAgentID: uuid.NullUUID{UUID: newAgentID, Valid: true},
		},
	})
	require.NoError(t, err)

	_, err = db.InsertChatMessages(ctx, database.InsertChatMessagesParams{
		ChatID:              parent.ID,
		CreatedBy:           []uuid.UUID{user.ID, user.ID},
		ModelConfigID:       []uuid.UUID{model.ID, model.ID},
		Role:                []database.ChatMessageRole{database.ChatMessageRoleUser, database.ChatMessageRoleUser},
		Content:             []string{string(oldContent), string(newContent)},
		ContentVersion:      []int16{chatprompt.CurrentContentVersion, chatprompt.CurrentContentVersion},
		Visibility:          []database.ChatMessageVisibility{database.ChatMessageVisibilityBoth, database.ChatMessageVisibilityBoth},
		InputTokens:         []int64{0, 0},
		OutputTokens:        []int64{0, 0},
		TotalTokens:         []int64{0, 0},
		ReasoningTokens:     []int64{0, 0},
		CacheCreationTokens: []int64{0, 0},
		CacheReadTokens:     []int64{0, 0},
		ContextLimit:        []int64{0, 0},
		Compressed:          []bool{false, false},
		TotalCostMicros:     []int64{0, 0},
		RuntimeMs:           []int64{0, 0},
	})
	require.NoError(t, err)

	parentChat, err := db.GetChatByID(ctx, parent.ID)
	require.NoError(t, err)
	return parentChat
}

func TestCreateChildSubagentChatCopiesOnlyLatestAgentContext(t *testing.T) {
	t.Parallel()

	db, ps := dbtestutil.NewDB(t)
	server := newInternalTestServer(t, db, ps, chatprovider.ProviderAPIKeys{})

	ctx := chatdTestContext(t)
	parentChat := createParentChatWithRotatedInheritedContext(ctx, t, db, server)

	child, err := server.createChildSubagentChat(ctx, parentChat, "inspect bindings", "")
	require.NoError(t, err)

	childChat, err := db.GetChatByID(ctx, child.ID)
	require.NoError(t, err)
	require.True(t, childChat.LastInjectedContext.Valid)

	var cached []codersdk.ChatMessagePart
	require.NoError(t, json.Unmarshal(childChat.LastInjectedContext.RawMessage, &cached))
	require.Len(t, cached, 2)
	require.Equal(t, "/home/coder/project-new/AGENTS.md", cached[0].ContextFilePath)
	require.Equal(t, "new-skill", cached[1].SkillName)

	childMessages, err := db.GetChatMessagesByChatID(ctx, database.GetChatMessagesByChatIDParams{
		ChatID:  child.ID,
		AfterID: 0,
	})
	require.NoError(t, err)

	var inherited [][]codersdk.ChatMessagePart
	for _, msg := range childMessages {
		if !msg.Content.Valid {
			continue
		}
		var parts []codersdk.ChatMessagePart
		require.NoError(t, json.Unmarshal(msg.Content.RawMessage, &parts))
		if len(parts) == 0 || parts[0].Type == codersdk.ChatMessagePartTypeText {
			continue
		}
		inherited = append(inherited, parts)
	}
	require.Len(t, inherited, 1)
	require.Len(t, inherited[0], 2)
	require.Equal(t, "/home/coder/project-new/AGENTS.md", inherited[0][0].ContextFilePath)
	require.Equal(t, "# New instructions", inherited[0][0].ContextFileContent)
	require.Equal(t, "new-skill", inherited[0][1].SkillName)
}

func TestCreateChildSubagentChatUpdatesInheritedLastInjectedContext(t *testing.T) {
	t.Parallel()

	db, ps := dbtestutil.NewDB(t)
	server := newInternalTestServer(t, db, ps, chatprovider.ProviderAPIKeys{})

	ctx := chatdTestContext(t)
	parentChat := createParentChatWithInheritedContext(ctx, t, db, server)

	child, err := server.createChildSubagentChat(ctx, parentChat, "inspect bindings", "")
	require.NoError(t, err)

	assertChildInheritedContext(ctx, t, db, child.ID, "inspect bindings")
}

func TestSpawnComputerUseAgentInheritsContext(t *testing.T) {
	t.Parallel()

	db, ps := dbtestutil.NewDB(t)
	require.NoError(t, db.UpsertChatDesktopEnabled(chatdTestContext(t), true))
	server := newInternalTestServer(t, db, ps, chatprovider.ProviderAPIKeys{})

	ctx := chatdTestContext(t)
	parentChat := createParentChatWithInheritedContext(ctx, t, db, server)
	insertEnabledAnthropicProvider(ctx, t, db, parentChat.OwnerID)

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

	assertChildInheritedContext(ctx, t, db, childID, "inspect bindings")
}
