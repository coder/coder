package coderd_test

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/google/uuid"
	"github.com/sqlc-dev/pqtype"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd"
	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/database/dbfake"
	"github.com/coder/coder/v2/coderd/x/chatd"
	"github.com/coder/coder/v2/coderd/x/chatd/chatprompt"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/codersdk/agentsdk"
	"github.com/coder/coder/v2/testutil"
)

type agentChatContextTestSetup struct {
	client      *codersdk.Client
	db          database.Store
	user        codersdk.CreateFirstUserResponse
	workspace   dbfake.WorkspaceResponse
	agentClient *agentsdk.Client
}

func TestAgentChatContext(t *testing.T) {
	t.Parallel()

	type addSuccessStep struct {
		req       agentsdk.AddChatContextRequest
		wantCount int
	}

	type addSuccessCase struct {
		name          string
		steps         []addSuccessStep
		wantStored    [][]codersdk.ChatMessagePart
		storedOrdered bool
		wantCached    []codersdk.ChatMessagePart
		cachedOrdered bool
	}

	agentInstructionsPart := codersdk.ChatMessagePart{
		Type:               codersdk.ChatMessagePartTypeContextFile,
		ContextFilePath:    "/workspace/AGENTS.md",
		ContextFileContent: "context from the agent",
	}
	fileAPart := codersdk.ChatMessagePart{
		Type:               codersdk.ChatMessagePartTypeContextFile,
		ContextFilePath:    "/workspace/file-a.md",
		ContextFileContent: "file A context",
	}
	fileBPart := codersdk.ChatMessagePart{
		Type:               codersdk.ChatMessagePartTypeContextFile,
		ContextFilePath:    "/workspace/file-b.md",
		ContextFileContent: "file B context",
	}
	repoHelperSkillPart := codersdk.ChatMessagePart{
		Type:                     codersdk.ChatMessagePartTypeSkill,
		SkillName:                "repo-helper",
		SkillDescription:         "Repository instructions",
		SkillDir:                 "/workspace/.agents/skills/repo-helper",
		ContextFileSkillMetaFile: "SKILL.md",
	}
	projectInstructionsPart := codersdk.ChatMessagePart{
		Type:               codersdk.ChatMessagePartTypeContextFile,
		ContextFilePath:    "/workspace/AGENTS.md",
		ContextFileContent: "project instructions",
	}
	cachedAgentInstructionsPart := codersdk.ChatMessagePart{
		Type:            codersdk.ChatMessagePartTypeContextFile,
		ContextFilePath: agentInstructionsPart.ContextFilePath,
	}
	cachedFileAPart := codersdk.ChatMessagePart{
		Type:            codersdk.ChatMessagePartTypeContextFile,
		ContextFilePath: fileAPart.ContextFilePath,
	}
	cachedFileBPart := codersdk.ChatMessagePart{
		Type:            codersdk.ChatMessagePartTypeContextFile,
		ContextFilePath: fileBPart.ContextFilePath,
	}
	cachedRepoHelperSkillPart := codersdk.ChatMessagePart{
		Type:             codersdk.ChatMessagePartTypeSkill,
		SkillName:        repoHelperSkillPart.SkillName,
		SkillDescription: repoHelperSkillPart.SkillDescription,
	}
	cachedProjectInstructionsPart := codersdk.ChatMessagePart{
		Type:            codersdk.ChatMessagePartTypeContextFile,
		ContextFilePath: projectInstructionsPart.ContextFilePath,
	}

	addSuccessCases := []addSuccessCase{
		{
			name:          "AddSuccessFiltersPartsAndUpdatesCache",
			steps:         []addSuccessStep{{req: agentsdk.AddChatContextRequest{Parts: []codersdk.ChatMessagePart{codersdk.ChatMessageText("ignore this text part"), agentInstructionsPart}}, wantCount: 1}},
			wantStored:    [][]codersdk.ChatMessagePart{{agentInstructionsPart}},
			storedOrdered: true,
			wantCached:    []codersdk.ChatMessagePart{cachedAgentInstructionsPart},
			cachedOrdered: true,
		},
		{
			name:          "AddSuccessIsAdditive",
			steps:         []addSuccessStep{{req: agentsdk.AddChatContextRequest{Parts: []codersdk.ChatMessagePart{fileAPart}}, wantCount: 1}, {req: agentsdk.AddChatContextRequest{Parts: []codersdk.ChatMessagePart{fileBPart}}, wantCount: 1}},
			wantStored:    [][]codersdk.ChatMessagePart{{fileAPart}, {fileBPart}},
			storedOrdered: false,
			wantCached:    []codersdk.ChatMessagePart{cachedFileAPart, cachedFileBPart},
			cachedOrdered: false,
		},
		{
			name:          "AddSuccessWithSkillOnlyPartsGetsSentinel",
			steps:         []addSuccessStep{{req: agentsdk.AddChatContextRequest{Parts: []codersdk.ChatMessagePart{repoHelperSkillPart}}, wantCount: 1}},
			wantStored:    [][]codersdk.ChatMessagePart{{{Type: codersdk.ChatMessagePartTypeContextFile}, repoHelperSkillPart}},
			storedOrdered: true,
			wantCached:    []codersdk.ChatMessagePart{cachedRepoHelperSkillPart},
			cachedOrdered: true,
		},
		{
			name:          "AddSuccessWithMixedPartsNoSentinel",
			steps:         []addSuccessStep{{req: agentsdk.AddChatContextRequest{Parts: []codersdk.ChatMessagePart{projectInstructionsPart, repoHelperSkillPart}}, wantCount: 2}},
			wantStored:    [][]codersdk.ChatMessagePart{{projectInstructionsPart, repoHelperSkillPart}},
			storedOrdered: true,
			wantCached:    []codersdk.ChatMessagePart{cachedProjectInstructionsPart, cachedRepoHelperSkillPart},
			cachedOrdered: true,
		},
	}

	for _, tc := range addSuccessCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			ctx := testutil.Context(t, testutil.WaitLong)
			setup := newAgentChatContextTestSetup(t)
			model := coderd.InsertAgentChatTestModelConfig(ctx, t, setup.db, setup.user.UserID)
			chat := createAgentChatContextChat(ctx, t, setup.db, setup.user.UserID, model.ID, setup.workspace.Agents[0].ID, t.Name())

			for _, step := range tc.steps {
				resp, err := setup.agentClient.AddChatContext(ctx, step.req)
				require.NoError(t, err)
				require.Equal(t, chat.ID, resp.ChatID)
				require.Equal(t, step.wantCount, resp.Count)
			}

			actualStored := requireAgentChatContextStoredMessages(t, requireAgentChatContextMessages(ctx, t, setup.db, chat.ID))
			agent := setup.workspace.Agents[0]
			wantStored := agentChatContextExpectedMessages(agent, tc.wantStored)
			if tc.storedOrdered {
				require.Equal(t, wantStored, actualStored)
			} else {
				require.ElementsMatch(t, wantStored, actualStored)
			}

			wantCached := agentChatContextExpectedCachedParts(agent, tc.wantCached)
			actualCached := requireAgentChatContextCachedParts(ctx, t, setup.db, chat.ID)
			if tc.cachedOrdered {
				require.Equal(t, wantCached, actualCached)
			} else {
				require.ElementsMatch(t, wantCached, actualCached)
			}
		})
	}

	t.Run("ClearDeletesSkillMessages", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)
		setup := newAgentChatContextTestSetup(t)
		model := coderd.InsertAgentChatTestModelConfig(ctx, t, setup.db, setup.user.UserID)
		chat := createAgentChatContextChat(ctx, t, setup.db, setup.user.UserID, model.ID, setup.workspace.Agents[0].ID, t.Name())

		skillPart := codersdk.ChatMessagePart{
			Type:                     codersdk.ChatMessagePartTypeSkill,
			SkillName:                "repo-helper",
			SkillDescription:         "Repository instructions",
			SkillDir:                 "/workspace/.agents/skills/repo-helper",
			ContextFileSkillMetaFile: "SKILL.md",
		}
		_, err := setup.agentClient.AddChatContext(ctx, agentsdk.AddChatContextRequest{
			Parts: []codersdk.ChatMessagePart{skillPart},
		})
		require.NoError(t, err)

		messages, err := setup.db.GetChatMessagesByChatID(
			dbauthz.AsSystemRestricted(ctx),
			database.GetChatMessagesByChatIDParams{ChatID: chat.ID, AfterID: 0},
		)
		require.NoError(t, err)
		require.Len(t, messages, 1)

		storedParts := requireAgentChatContextParts(t, messages[0].Content.RawMessage)
		require.Len(t, storedParts, 2)

		// Strip the sentinel so clear must delete the skill message via
		// the skill-part scan instead of the context-file bulk delete.
		rawSkillOnly, err := json.Marshal([]codersdk.ChatMessagePart{storedParts[1]})
		require.NoError(t, err)
		_, err = setup.db.UpdateChatMessageByID(
			dbauthz.AsSystemRestricted(ctx),
			database.UpdateChatMessageByIDParams{
				ID: messages[0].ID,
				Content: pqtype.NullRawMessage{
					RawMessage: rawSkillOnly,
					Valid:      true,
				},
			},
		)
		require.NoError(t, err)

		resp, err := setup.agentClient.ClearChatContext(ctx, agentsdk.ClearChatContextRequest{})
		require.NoError(t, err)
		require.Equal(t, chat.ID, resp.ChatID)

		messages, err = setup.db.GetChatMessagesByChatID(
			dbauthz.AsSystemRestricted(ctx),
			database.GetChatMessagesByChatIDParams{ChatID: chat.ID, AfterID: 0},
		)
		require.NoError(t, err)
		require.Empty(t, messages)

		persistedChat, err := setup.db.GetChatByID(dbauthz.AsSystemRestricted(ctx), chat.ID)
		require.NoError(t, err)
		require.False(t, persistedChat.LastInjectedContext.Valid)
	})

	t.Run("ClearSuccessDeletesInjectedContext", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)
		setup := newAgentChatContextTestSetup(t)
		model := coderd.InsertAgentChatTestModelConfig(ctx, t, setup.db, setup.user.UserID)
		chat := createAgentChatContextChat(ctx, t, setup.db, setup.user.UserID, model.ID, setup.workspace.Agents[0].ID, t.Name())

		_, err := setup.agentClient.AddChatContext(ctx, agentsdk.AddChatContextRequest{
			Parts: []codersdk.ChatMessagePart{{
				Type:               codersdk.ChatMessagePartTypeContextFile,
				ContextFilePath:    "/workspace/instructions.md",
				ContextFileContent: "remember this file",
			}},
		})
		require.NoError(t, err)

		regularContent, err := chatprompt.MarshalParts([]codersdk.ChatMessagePart{
			codersdk.ChatMessageText("keep this user message"),
		})
		require.NoError(t, err)
		_, err = setup.db.InsertChatMessages(
			dbauthz.AsSystemRestricted(ctx),
			chatd.BuildSingleChatMessageInsertParams(
				chat.ID,
				database.ChatMessageRoleUser,
				regularContent,
				database.ChatMessageVisibilityBoth,
				chat.LastModelConfigID,
				chatprompt.CurrentContentVersion,
				setup.user.UserID,
			),
		)
		require.NoError(t, err)

		resp, err := setup.agentClient.ClearChatContext(ctx, agentsdk.ClearChatContextRequest{})
		require.NoError(t, err)
		require.Equal(t, chat.ID, resp.ChatID)

		messages, err := setup.db.GetChatMessagesByChatID(
			dbauthz.AsSystemRestricted(ctx),
			database.GetChatMessagesByChatIDParams{ChatID: chat.ID, AfterID: 0},
		)
		require.NoError(t, err)
		require.Len(t, messages, 1)
		require.Equal(t, database.ChatMessageRoleUser, messages[0].Role)

		remainingParts := requireAgentChatContextParts(t, messages[0].Content.RawMessage)
		require.Len(t, remainingParts, 1)
		require.Equal(t, codersdk.ChatMessagePartTypeText, remainingParts[0].Type)
		require.Equal(t, "keep this user message", remainingParts[0].Text)

		persistedChat, err := setup.db.GetChatByID(dbauthz.AsSystemRestricted(ctx), chat.ID)
		require.NoError(t, err)
		require.False(t, persistedChat.LastInjectedContext.Valid)
	})

	t.Run("AddFailsWhenAgentHasNoActiveChat", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)
		setup := newAgentChatContextTestSetup(t)

		_, err := setup.agentClient.AddChatContext(ctx, agentsdk.AddChatContextRequest{
			Parts: []codersdk.ChatMessagePart{{
				Type:               codersdk.ChatMessagePartTypeContextFile,
				ContextFilePath:    "/workspace/AGENTS.md",
				ContextFileContent: "missing chat",
			}},
		})
		sdkErr := requireSDKError(t, err, http.StatusNotFound)
		require.Equal(t, "No active chats found for this agent.", sdkErr.Message)
	})

	t.Run("AddRejectsChatOwnedByAnotherAgent", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)
		client, db := coderdtest.NewWithDatabase(t, nil)
		user := coderdtest.CreateFirstUser(t, client)
		model := coderd.InsertAgentChatTestModelConfig(ctx, t, db, user.UserID)

		firstWorkspace := dbfake.WorkspaceBuild(t, db, database.WorkspaceTable{
			OrganizationID: user.OrganizationID,
			OwnerID:        user.UserID,
		}).WithAgent().Do()
		secondWorkspace := dbfake.WorkspaceBuild(t, db, database.WorkspaceTable{
			OrganizationID: user.OrganizationID,
			OwnerID:        user.UserID,
		}).WithAgent().Do()

		chat := createAgentChatContextChat(ctx, t, db, user.UserID, model.ID, firstWorkspace.Agents[0].ID, t.Name())
		secondAgentClient := agentsdk.New(client.URL, agentsdk.WithFixedToken(secondWorkspace.AgentToken))

		_, err := secondAgentClient.AddChatContext(ctx, agentsdk.AddChatContextRequest{
			ChatID: chat.ID,
			Parts: []codersdk.ChatMessagePart{{
				Type:               codersdk.ChatMessagePartTypeContextFile,
				ContextFilePath:    "/workspace/foreign.md",
				ContextFileContent: "not your chat",
			}},
		})
		sdkErr := requireSDKError(t, err, http.StatusForbidden)
		require.Equal(t, "Chat does not belong to this agent.", sdkErr.Message)
	})

	t.Run("AddRejectsTooManyParts", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)
		setup := newAgentChatContextTestSetup(t)
		parts := make([]codersdk.ChatMessagePart, 101)
		for i := range parts {
			parts[i] = codersdk.ChatMessagePart{
				Type:               codersdk.ChatMessagePartTypeContextFile,
				ContextFilePath:    "/workspace/file.md",
				ContextFileContent: "too many",
			}
		}

		_, err := setup.agentClient.AddChatContext(ctx, agentsdk.AddChatContextRequest{Parts: parts})
		sdkErr := requireSDKError(t, err, http.StatusBadRequest)
		require.Contains(t, sdkErr.Message, "Too many context parts")
	})

	t.Run("ClearIsIdempotentWhenNoActiveChatExists", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)
		setup := newAgentChatContextTestSetup(t)

		resp, err := setup.agentClient.ClearChatContext(ctx, agentsdk.ClearChatContextRequest{})
		require.NoError(t, err)
		require.Equal(t, uuid.Nil, resp.ChatID)
	})

	t.Run("AddFailsWithMultipleActiveChats", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)
		setup := newAgentChatContextTestSetup(t)
		model := coderd.InsertAgentChatTestModelConfig(ctx, t, setup.db, setup.user.UserID)
		createAgentChatContextChat(ctx, t, setup.db, setup.user.UserID, model.ID, setup.workspace.Agents[0].ID, t.Name()+"-chat1")
		createAgentChatContextChat(ctx, t, setup.db, setup.user.UserID, model.ID, setup.workspace.Agents[0].ID, t.Name()+"-chat2")

		_, err := setup.agentClient.AddChatContext(ctx, agentsdk.AddChatContextRequest{
			Parts: []codersdk.ChatMessagePart{{
				Type:               codersdk.ChatMessagePartTypeContextFile,
				ContextFilePath:    "/workspace/file.go",
				ContextFileContent: "content",
			}},
		})
		sdkErr := requireSDKError(t, err, http.StatusConflict)
		require.Contains(t, sdkErr.Message, "multiple active chats")
	})

	t.Run("AddFailsWhenChatIsNotActive", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)
		setup := newAgentChatContextTestSetup(t)
		model := coderd.InsertAgentChatTestModelConfig(ctx, t, setup.db, setup.user.UserID)
		chat := createAgentChatContextChat(ctx, t, setup.db, setup.user.UserID, model.ID, setup.workspace.Agents[0].ID, t.Name())

		_, err := setup.db.UpdateChatStatus(dbauthz.AsSystemRestricted(ctx), database.UpdateChatStatusParams{
			ID:     chat.ID,
			Status: database.ChatStatusCompleted,
		})
		require.NoError(t, err)

		_, err = setup.agentClient.AddChatContext(ctx, agentsdk.AddChatContextRequest{
			ChatID: chat.ID,
			Parts: []codersdk.ChatMessagePart{{
				Type:               codersdk.ChatMessagePartTypeContextFile,
				ContextFilePath:    "/workspace/file.go",
				ContextFileContent: "content",
			}},
		})
		sdkErr := requireSDKError(t, err, http.StatusConflict)
		require.Equal(t, "Cannot modify context: this chat is no longer active.", sdkErr.Message)
	})
}

func requireAgentChatContextMessages(ctx context.Context, t testing.TB, db database.Store, chatID uuid.UUID) []database.ChatMessage {
	t.Helper()

	messages, err := db.GetChatMessagesByChatID(
		dbauthz.AsSystemRestricted(ctx),
		database.GetChatMessagesByChatIDParams{ChatID: chatID, AfterID: 0},
	)
	require.NoError(t, err)
	return messages
}

func requireAgentChatContextCachedParts(ctx context.Context, t testing.TB, db database.Store, chatID uuid.UUID) []codersdk.ChatMessagePart {
	t.Helper()

	chat, err := db.GetChatByID(dbauthz.AsSystemRestricted(ctx), chatID)
	require.NoError(t, err)
	require.True(t, chat.LastInjectedContext.Valid)
	return requireAgentChatContextParts(t, chat.LastInjectedContext.RawMessage)
}

func requireAgentChatContextStoredMessages(t testing.TB, messages []database.ChatMessage) [][]codersdk.ChatMessagePart {
	t.Helper()

	stored := make([][]codersdk.ChatMessagePart, len(messages))
	for i, message := range messages {
		require.Equal(t, database.ChatMessageRoleUser, message.Role)
		require.True(t, message.Content.Valid)
		stored[i] = requireAgentChatContextParts(t, message.Content.RawMessage)
	}
	return stored
}

func agentChatContextExpectedMessages(agent database.WorkspaceAgent, messages [][]codersdk.ChatMessagePart) [][]codersdk.ChatMessagePart {
	expected := make([][]codersdk.ChatMessagePart, len(messages))
	for i, parts := range messages {
		expected[i] = agentChatContextExpectedStoredParts(agent, parts)
	}
	return expected
}

func agentChatContextExpectedStoredParts(agent database.WorkspaceAgent, parts []codersdk.ChatMessagePart) []codersdk.ChatMessagePart {
	expected := make([]codersdk.ChatMessagePart, len(parts))
	for i, part := range parts {
		part.ContextFileAgentID = uuid.NullUUID{UUID: agent.ID, Valid: true}
		if part.Type == codersdk.ChatMessagePartTypeContextFile {
			part.ContextFileOS = agent.OperatingSystem
			part.ContextFileDirectory = agentChatContextDirectory(agent)
		}
		expected[i] = part
	}
	return expected
}

func agentChatContextExpectedCachedParts(agent database.WorkspaceAgent, parts []codersdk.ChatMessagePart) []codersdk.ChatMessagePart {
	expected := make([]codersdk.ChatMessagePart, len(parts))
	for i, part := range parts {
		part.ContextFileAgentID = uuid.NullUUID{UUID: agent.ID, Valid: true}
		expected[i] = part
	}
	return expected
}

func newAgentChatContextTestSetup(t *testing.T) agentChatContextTestSetup {
	t.Helper()

	client, db := coderdtest.NewWithDatabase(t, nil)
	user := coderdtest.CreateFirstUser(t, client)
	workspace := dbfake.WorkspaceBuild(t, db, database.WorkspaceTable{
		OrganizationID: user.OrganizationID,
		OwnerID:        user.UserID,
	}).WithAgent().Do()

	return agentChatContextTestSetup{
		client:      client,
		db:          db,
		user:        user,
		workspace:   workspace,
		agentClient: agentsdk.New(client.URL, agentsdk.WithFixedToken(workspace.AgentToken)),
	}
}

func createAgentChatContextChat(
	ctx context.Context,
	t testing.TB,
	db database.Store,
	ownerID uuid.UUID,
	modelConfigID uuid.UUID,
	agentID uuid.UUID,
	title string,
) database.Chat {
	t.Helper()

	chat, err := db.InsertChat(dbauthz.AsSystemRestricted(ctx), database.InsertChatParams{
		Status:            database.ChatStatusWaiting,
		OwnerID:           ownerID,
		LastModelConfigID: modelConfigID,
		Title:             title,
		AgentID:           uuid.NullUUID{UUID: agentID, Valid: true},
	})
	require.NoError(t, err)

	return chat
}

func requireAgentChatContextParts(t testing.TB, raw json.RawMessage) []codersdk.ChatMessagePart {
	t.Helper()

	var parts []codersdk.ChatMessagePart
	require.NoError(t, json.Unmarshal(raw, &parts))
	return parts
}

func agentChatContextDirectory(agent database.WorkspaceAgent) string {
	if agent.ExpandedDirectory != "" {
		return agent.ExpandedDirectory
	}
	return agent.Directory
}
