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

	t.Run("AddSuccessFiltersPartsAndUpdatesCache", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)
		setup := newAgentChatContextTestSetup(t)
		model := coderd.InsertAgentChatTestModelConfig(ctx, t, setup.db, setup.user.UserID)
		chat := createAgentChatContextChat(ctx, t, setup.db, setup.user.UserID, model.ID, setup.workspace.Agents[0].ID, t.Name())

		resp, err := setup.agentClient.AddChatContext(ctx, agentsdk.AddChatContextRequest{
			Parts: []codersdk.ChatMessagePart{
				codersdk.ChatMessageText("ignore this text part"),
				{
					Type:               codersdk.ChatMessagePartTypeContextFile,
					ContextFilePath:    "/workspace/AGENTS.md",
					ContextFileContent: "context from the agent",
				},
			},
		})
		require.NoError(t, err)
		require.Equal(t, chat.ID, resp.ChatID)
		require.Equal(t, 1, resp.Count)

		messages, err := setup.db.GetChatMessagesByChatID(
			dbauthz.AsSystemRestricted(ctx),
			database.GetChatMessagesByChatIDParams{ChatID: chat.ID, AfterID: 0},
		)
		require.NoError(t, err)
		require.Len(t, messages, 1)
		require.Equal(t, database.ChatMessageRoleUser, messages[0].Role)
		require.True(t, messages[0].Content.Valid)

		storedParts := requireAgentChatContextParts(t, messages[0].Content.RawMessage)
		require.Len(t, storedParts, 1)
		require.Equal(t, codersdk.ChatMessagePartTypeContextFile, storedParts[0].Type)
		require.Equal(t, "/workspace/AGENTS.md", storedParts[0].ContextFilePath)
		require.Equal(t, "context from the agent", storedParts[0].ContextFileContent)
		require.True(t, storedParts[0].ContextFileAgentID.Valid)
		require.Equal(t, setup.workspace.Agents[0].ID, storedParts[0].ContextFileAgentID.UUID)
		require.Equal(t, setup.workspace.Agents[0].OperatingSystem, storedParts[0].ContextFileOS)
		require.Equal(t, agentChatContextDirectory(setup.workspace.Agents[0]), storedParts[0].ContextFileDirectory)

		persistedChat, err := setup.db.GetChatByID(dbauthz.AsSystemRestricted(ctx), chat.ID)
		require.NoError(t, err)
		require.True(t, persistedChat.LastInjectedContext.Valid)

		cachedParts := requireAgentChatContextParts(t, persistedChat.LastInjectedContext.RawMessage)
		require.Len(t, cachedParts, 1)
		require.Equal(t, codersdk.ChatMessagePartTypeContextFile, cachedParts[0].Type)
		require.Equal(t, "/workspace/AGENTS.md", cachedParts[0].ContextFilePath)
		require.True(t, cachedParts[0].ContextFileAgentID.Valid)
		require.Equal(t, setup.workspace.Agents[0].ID, cachedParts[0].ContextFileAgentID.UUID)
		require.Empty(t, cachedParts[0].ContextFileContent)
		require.Empty(t, cachedParts[0].ContextFileOS)
		require.Empty(t, cachedParts[0].ContextFileDirectory)
	})

	t.Run("AddSuccessIsAdditive", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)
		setup := newAgentChatContextTestSetup(t)
		model := coderd.InsertAgentChatTestModelConfig(ctx, t, setup.db, setup.user.UserID)
		chat := createAgentChatContextChat(ctx, t, setup.db, setup.user.UserID, model.ID, setup.workspace.Agents[0].ID, t.Name())

		firstPart := codersdk.ChatMessagePart{
			Type:               codersdk.ChatMessagePartTypeContextFile,
			ContextFilePath:    "/workspace/file-a.md",
			ContextFileContent: "file A context",
		}
		resp, err := setup.agentClient.AddChatContext(ctx, agentsdk.AddChatContextRequest{
			Parts: []codersdk.ChatMessagePart{firstPart},
		})
		require.NoError(t, err)
		require.Equal(t, chat.ID, resp.ChatID)
		require.Equal(t, 1, resp.Count)

		persistedChat, err := setup.db.GetChatByID(dbauthz.AsSystemRestricted(ctx), chat.ID)
		require.NoError(t, err)
		require.True(t, persistedChat.LastInjectedContext.Valid)

		cachedParts := requireAgentChatContextParts(t, persistedChat.LastInjectedContext.RawMessage)
		require.Len(t, cachedParts, 1)
		require.Equal(t, firstPart.ContextFilePath, cachedParts[0].ContextFilePath)
		require.True(t, cachedParts[0].ContextFileAgentID.Valid)
		require.Equal(t, setup.workspace.Agents[0].ID, cachedParts[0].ContextFileAgentID.UUID)
		require.Empty(t, cachedParts[0].ContextFileContent)

		secondPart := codersdk.ChatMessagePart{
			Type:               codersdk.ChatMessagePartTypeContextFile,
			ContextFilePath:    "/workspace/file-b.md",
			ContextFileContent: "file B context",
		}
		resp, err = setup.agentClient.AddChatContext(ctx, agentsdk.AddChatContextRequest{
			Parts: []codersdk.ChatMessagePart{secondPart},
		})
		require.NoError(t, err)
		require.Equal(t, chat.ID, resp.ChatID)
		require.Equal(t, 1, resp.Count)

		persistedChat, err = setup.db.GetChatByID(dbauthz.AsSystemRestricted(ctx), chat.ID)
		require.NoError(t, err)
		require.True(t, persistedChat.LastInjectedContext.Valid)

		cachedParts = requireAgentChatContextParts(t, persistedChat.LastInjectedContext.RawMessage)
		require.Len(t, cachedParts, 2)
		require.ElementsMatch(
			t,
			[]string{firstPart.ContextFilePath, secondPart.ContextFilePath},
			[]string{cachedParts[0].ContextFilePath, cachedParts[1].ContextFilePath},
		)
		for _, part := range cachedParts {
			require.Equal(t, codersdk.ChatMessagePartTypeContextFile, part.Type)
			require.True(t, part.ContextFileAgentID.Valid)
			require.Equal(t, setup.workspace.Agents[0].ID, part.ContextFileAgentID.UUID)
			require.Empty(t, part.ContextFileContent)
			require.Empty(t, part.ContextFileOS)
			require.Empty(t, part.ContextFileDirectory)
		}

		messages, err := setup.db.GetChatMessagesByChatID(
			dbauthz.AsSystemRestricted(ctx),
			database.GetChatMessagesByChatIDParams{ChatID: chat.ID, AfterID: 0},
		)
		require.NoError(t, err)
		require.Len(t, messages, 2)

		expectedContents := map[string]string{
			firstPart.ContextFilePath:  firstPart.ContextFileContent,
			secondPart.ContextFilePath: secondPart.ContextFileContent,
		}
		for _, message := range messages {
			storedParts := requireAgentChatContextParts(t, message.Content.RawMessage)
			require.Len(t, storedParts, 1)

			storedPart := storedParts[0]
			require.Equal(t, codersdk.ChatMessagePartTypeContextFile, storedPart.Type)
			expectedContent, ok := expectedContents[storedPart.ContextFilePath]
			require.True(t, ok)
			require.Equal(t, expectedContent, storedPart.ContextFileContent)
			require.True(t, storedPart.ContextFileAgentID.Valid)
			require.Equal(t, setup.workspace.Agents[0].ID, storedPart.ContextFileAgentID.UUID)
			require.Equal(t, setup.workspace.Agents[0].OperatingSystem, storedPart.ContextFileOS)
			require.Equal(t, agentChatContextDirectory(setup.workspace.Agents[0]), storedPart.ContextFileDirectory)
		}
	})

	t.Run("AddSuccessWithSkillOnlyPartsGetsSentinel", func(t *testing.T) {
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
		resp, err := setup.agentClient.AddChatContext(ctx, agentsdk.AddChatContextRequest{
			Parts: []codersdk.ChatMessagePart{skillPart},
		})
		require.NoError(t, err)
		require.Equal(t, chat.ID, resp.ChatID)
		require.Equal(t, 1, resp.Count)

		messages, err := setup.db.GetChatMessagesByChatID(
			dbauthz.AsSystemRestricted(ctx),
			database.GetChatMessagesByChatIDParams{ChatID: chat.ID, AfterID: 0},
		)
		require.NoError(t, err)
		require.Len(t, messages, 1)

		storedParts := requireAgentChatContextParts(t, messages[0].Content.RawMessage)
		require.Len(t, storedParts, 2)

		sentinelPart := storedParts[0]
		require.Equal(t, codersdk.ChatMessagePartTypeContextFile, sentinelPart.Type)
		require.Empty(t, sentinelPart.ContextFilePath)
		require.Empty(t, sentinelPart.ContextFileContent)
		require.True(t, sentinelPart.ContextFileAgentID.Valid)
		require.Equal(t, setup.workspace.Agents[0].ID, sentinelPart.ContextFileAgentID.UUID)
		require.Equal(t, setup.workspace.Agents[0].OperatingSystem, sentinelPart.ContextFileOS)
		require.Equal(t, agentChatContextDirectory(setup.workspace.Agents[0]), sentinelPart.ContextFileDirectory)

		persistedSkillPart := storedParts[1]
		require.Equal(t, codersdk.ChatMessagePartTypeSkill, persistedSkillPart.Type)
		require.Equal(t, skillPart.SkillName, persistedSkillPart.SkillName)
		require.Equal(t, skillPart.SkillDescription, persistedSkillPart.SkillDescription)
		require.Equal(t, skillPart.SkillDir, persistedSkillPart.SkillDir)
		require.Equal(t, skillPart.ContextFileSkillMetaFile, persistedSkillPart.ContextFileSkillMetaFile)
		require.True(t, persistedSkillPart.ContextFileAgentID.Valid)
		require.Equal(t, setup.workspace.Agents[0].ID, persistedSkillPart.ContextFileAgentID.UUID)

		persistedChat, err := setup.db.GetChatByID(dbauthz.AsSystemRestricted(ctx), chat.ID)
		require.NoError(t, err)
		require.True(t, persistedChat.LastInjectedContext.Valid)

		cachedParts := requireAgentChatContextParts(t, persistedChat.LastInjectedContext.RawMessage)
		require.Len(t, cachedParts, 1)
		require.Equal(t, codersdk.ChatMessagePartTypeSkill, cachedParts[0].Type)
		require.Equal(t, skillPart.SkillName, cachedParts[0].SkillName)
		require.Equal(t, skillPart.SkillDescription, cachedParts[0].SkillDescription)
		require.True(t, cachedParts[0].ContextFileAgentID.Valid)
		require.Equal(t, setup.workspace.Agents[0].ID, cachedParts[0].ContextFileAgentID.UUID)
		require.Empty(t, cachedParts[0].SkillDir)
		require.Empty(t, cachedParts[0].ContextFileSkillMetaFile)
	})

	t.Run("AddSuccessWithMixedPartsNoSentinel", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)
		setup := newAgentChatContextTestSetup(t)
		model := coderd.InsertAgentChatTestModelConfig(ctx, t, setup.db, setup.user.UserID)
		chat := createAgentChatContextChat(ctx, t, setup.db, setup.user.UserID, model.ID, setup.workspace.Agents[0].ID, t.Name())

		parts := []codersdk.ChatMessagePart{
			{
				Type:               codersdk.ChatMessagePartTypeContextFile,
				ContextFilePath:    "/workspace/AGENTS.md",
				ContextFileContent: "project instructions",
			},
			{
				Type:                     codersdk.ChatMessagePartTypeSkill,
				SkillName:                "repo-helper",
				SkillDescription:         "Repository instructions",
				SkillDir:                 "/workspace/.agents/skills/repo-helper",
				ContextFileSkillMetaFile: "SKILL.md",
			},
		}
		resp, err := setup.agentClient.AddChatContext(ctx, agentsdk.AddChatContextRequest{Parts: parts})
		require.NoError(t, err)
		require.Equal(t, chat.ID, resp.ChatID)
		require.Equal(t, 2, resp.Count)

		messages, err := setup.db.GetChatMessagesByChatID(
			dbauthz.AsSystemRestricted(ctx),
			database.GetChatMessagesByChatIDParams{ChatID: chat.ID, AfterID: 0},
		)
		require.NoError(t, err)
		require.Len(t, messages, 1)

		storedParts := requireAgentChatContextParts(t, messages[0].Content.RawMessage)
		require.Len(t, storedParts, 2)

		storedContextFilePart := storedParts[0]
		require.Equal(t, codersdk.ChatMessagePartTypeContextFile, storedContextFilePart.Type)
		require.Equal(t, parts[0].ContextFilePath, storedContextFilePart.ContextFilePath)
		require.Equal(t, parts[0].ContextFileContent, storedContextFilePart.ContextFileContent)
		require.True(t, storedContextFilePart.ContextFileAgentID.Valid)
		require.Equal(t, setup.workspace.Agents[0].ID, storedContextFilePart.ContextFileAgentID.UUID)
		require.Equal(t, setup.workspace.Agents[0].OperatingSystem, storedContextFilePart.ContextFileOS)
		require.Equal(t, agentChatContextDirectory(setup.workspace.Agents[0]), storedContextFilePart.ContextFileDirectory)

		storedSkillPart := storedParts[1]
		require.Equal(t, codersdk.ChatMessagePartTypeSkill, storedSkillPart.Type)
		require.Equal(t, parts[1].SkillName, storedSkillPart.SkillName)
		require.Equal(t, parts[1].SkillDescription, storedSkillPart.SkillDescription)
		require.Equal(t, parts[1].SkillDir, storedSkillPart.SkillDir)
		require.Equal(t, parts[1].ContextFileSkillMetaFile, storedSkillPart.ContextFileSkillMetaFile)
		require.True(t, storedSkillPart.ContextFileAgentID.Valid)
		require.Equal(t, setup.workspace.Agents[0].ID, storedSkillPart.ContextFileAgentID.UUID)

		persistedChat, err := setup.db.GetChatByID(dbauthz.AsSystemRestricted(ctx), chat.ID)
		require.NoError(t, err)
		require.True(t, persistedChat.LastInjectedContext.Valid)

		cachedParts := requireAgentChatContextParts(t, persistedChat.LastInjectedContext.RawMessage)
		require.Len(t, cachedParts, 2)
		require.Equal(t, codersdk.ChatMessagePartTypeContextFile, cachedParts[0].Type)
		require.Equal(t, parts[0].ContextFilePath, cachedParts[0].ContextFilePath)
		require.Equal(t, codersdk.ChatMessagePartTypeSkill, cachedParts[1].Type)
		require.Equal(t, parts[1].SkillName, cachedParts[1].SkillName)
		require.True(t, cachedParts[1].ContextFileAgentID.Valid)
		require.Equal(t, setup.workspace.Agents[0].ID, cachedParts[1].ContextFileAgentID.UUID)
	})

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
