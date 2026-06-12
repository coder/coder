package coderd_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/sqlc-dev/pqtype"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd"
	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/database/dbfake"
	"github.com/coder/coder/v2/coderd/database/dbgen"
	"github.com/coder/coder/v2/coderd/database/dbtestutil"
	dbpubsub "github.com/coder/coder/v2/coderd/database/pubsub"
	coderdpubsub "github.com/coder/coder/v2/coderd/pubsub"
	"github.com/coder/coder/v2/coderd/x/chatd"
	"github.com/coder/coder/v2/coderd/x/chatd/chatprompt"
	"github.com/coder/coder/v2/coderd/x/chatd/chatstate"
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

type agentChatContextBeforeInTxStore struct {
	database.Store
	beforeInTx func()
}

func (s *agentChatContextBeforeInTxStore) InTx(fn func(database.Store) error, opts *database.TxOptions) error {
	if s.beforeInTx != nil {
		beforeInTx := s.beforeInTx
		s.beforeInTx = nil
		beforeInTx()
	}
	return s.Store.InTx(fn, opts)
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
			name:  "AddSuccessWithSkillOnlyPartsGetsSentinel",
			steps: []addSuccessStep{{req: agentsdk.AddChatContextRequest{Parts: []codersdk.ChatMessagePart{repoHelperSkillPart}}, wantCount: 1}},
			wantStored: [][]codersdk.ChatMessagePart{{{
				Type:            codersdk.ChatMessagePartTypeContextFile,
				ContextFilePath: chatd.AgentChatContextSentinelPath,
			}, repoHelperSkillPart}},
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
			model := coderd.InsertAgentChatTestModelConfig(t, setup.db, setup.user.UserID)
			chat := createAgentChatContextChat(t, setup.db, setup.user.OrganizationID, setup.user.UserID, model.ID, setup.workspace.Agents[0].ID, t.Name())

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

	t.Run("AddUsesLockedChatModelConfig", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)
		baseDB, pubsub := dbtestutil.NewDB(t)
		interceptDB := &agentChatContextBeforeInTxStore{Store: baseDB}
		client := coderdtest.New(t, &coderdtest.Options{
			Database: interceptDB,
			Pubsub:   pubsub,
		})
		user := coderdtest.CreateFirstUser(t, client)
		workspace := dbfake.WorkspaceBuild(t, baseDB, database.WorkspaceTable{
			OrganizationID: user.OrganizationID,
			OwnerID:        user.UserID,
		}).WithAgent().Do()
		agentClient := agentsdk.New(client.URL, agentsdk.WithFixedToken(workspace.AgentToken))

		originalModel := coderd.InsertAgentChatTestModelConfig(t, baseDB, user.UserID)
		updatedModel := dbgen.ChatModelConfig(t, baseDB, database.ChatModelConfig{
			Provider:             originalModel.Provider,
			Model:                "gpt-4o-mini-updated",
			DisplayName:          "Updated Test Model",
			CreatedBy:            uuid.NullUUID{UUID: user.UserID, Valid: true},
			UpdatedBy:            uuid.NullUUID{UUID: user.UserID, Valid: true},
			ContextLimit:         originalModel.ContextLimit,
			CompressionThreshold: originalModel.CompressionThreshold,
		})
		chat := createAgentChatContextChat(t, baseDB, user.OrganizationID, user.UserID, originalModel.ID, workspace.Agents[0].ID, t.Name())

		interceptDB.beforeInTx = func() {
			_, err := baseDB.UpdateChatLastModelConfigByID(
				dbauthz.AsSystemRestricted(ctx),
				database.UpdateChatLastModelConfigByIDParams{
					ID:                chat.ID,
					LastModelConfigID: updatedModel.ID,
				},
			)
			require.NoError(t, err)
		}

		resp, err := agentClient.AddChatContext(ctx, agentsdk.AddChatContextRequest{
			ChatID: chat.ID,
			Parts: []codersdk.ChatMessagePart{{
				Type:               codersdk.ChatMessagePartTypeContextFile,
				ContextFilePath:    "/workspace/instructions.md",
				ContextFileContent: "remember this file",
			}},
		})
		require.NoError(t, err)
		require.Equal(t, chat.ID, resp.ChatID)
		require.Equal(t, 1, resp.Count)

		messages := requireAgentChatContextMessages(ctx, t, baseDB, chat.ID)
		require.Len(t, messages, 1)
		require.True(t, messages[0].ModelConfigID.Valid)
		require.Equal(t, updatedModel.ID, messages[0].ModelConfigID.UUID)

		persistedChat, err := baseDB.GetChatByID(dbauthz.AsSystemRestricted(ctx), chat.ID)
		require.NoError(t, err)
		require.Equal(t, updatedModel.ID, persistedChat.LastModelConfigID)
	})

	t.Run("AddSuccessUpdatesChatStateVersionsAndPublishes", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)
		baseDB, pubsub := dbtestutil.NewDB(t)
		client := coderdtest.New(t, &coderdtest.Options{
			Database: baseDB,
			Pubsub:   pubsub,
		})
		user := coderdtest.CreateFirstUser(t, client)
		workspace := dbfake.WorkspaceBuild(t, baseDB, database.WorkspaceTable{
			OrganizationID: user.OrganizationID,
			OwnerID:        user.UserID,
		}).WithAgent().Do()
		agentClient := agentsdk.New(client.URL, agentsdk.WithFixedToken(workspace.AgentToken))
		model := coderd.InsertAgentChatTestModelConfig(t, baseDB, user.UserID)
		chat := createAgentChatContextChat(t, baseDB, user.OrganizationID, user.UserID, model.ID, workspace.Agents[0].ID, t.Name())

		updateCh := make(chan []byte, 1)
		cancelSub, err := pubsub.Subscribe(coderdpubsub.ChatStateUpdateChannel(chat.ID), func(_ context.Context, msg []byte) {
			updateCh <- msg
		})
		require.NoError(t, err)
		defer cancelSub()

		resp, err := agentClient.AddChatContext(ctx, agentsdk.AddChatContextRequest{
			ChatID: chat.ID,
			Parts: []codersdk.ChatMessagePart{{
				Type:               codersdk.ChatMessagePartTypeContextFile,
				ContextFilePath:    "/workspace/instructions.md",
				ContextFileContent: "remember this file",
			}},
		})
		require.NoError(t, err)
		require.Equal(t, chat.ID, resp.ChatID)
		require.Equal(t, 1, resp.Count)

		persisted, err := baseDB.GetChatByID(dbauthz.AsSystemRestricted(ctx), chat.ID)
		require.NoError(t, err)
		require.Equal(t, chat.SnapshotVersion+1, persisted.SnapshotVersion)
		require.Equal(t, persisted.SnapshotVersion, persisted.HistoryVersion)

		messages := requireAgentChatContextMessages(ctx, t, baseDB, chat.ID)
		require.Len(t, messages, 1)
		require.Equal(t, persisted.SnapshotVersion, messages[0].Revision)

		cached := requireAgentChatContextCachedParts(ctx, t, baseDB, chat.ID)
		require.Len(t, cached, 1)
		require.Equal(t, "/workspace/instructions.md", cached[0].ContextFilePath)

		select {
		case raw := <-updateCh:
			var update coderdpubsub.ChatStateUpdateMessage
			require.NoError(t, json.Unmarshal(raw, &update))
			require.Equal(t, persisted.SnapshotVersion, update.SnapshotVersion)
			require.Equal(t, persisted.HistoryVersion, update.HistoryVersion)
		case <-ctx.Done():
			t.Fatal("timed out waiting for chat state update")
		}
	})

	t.Run("AddInterruptsAndQueuesWhenChatIsRunning", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)
		setup := newAgentChatContextTestSetup(t)
		model := coderd.InsertAgentChatTestModelConfig(t, setup.db, setup.user.UserID)
		chat := createAgentChatContextChat(t, setup.db, setup.user.OrganizationID, setup.user.UserID, model.ID, setup.workspace.Agents[0].ID, t.Name())
		chat = setAgentChatContextChatStatus(ctx, t, setup.db, chat.ID, database.ChatStatusRunning)
		chat = acquireAgentChatContextChat(ctx, t, setup.db, chat.ID)
		apiKeyID := currentAgentChatContextAPIKeyID(t, setup.client)

		resp, err := setup.agentClient.AddChatContext(ctx, agentsdk.AddChatContextRequest{
			ChatID: chat.ID,
			Parts: []codersdk.ChatMessagePart{{
				Type:               codersdk.ChatMessagePartTypeContextFile,
				ContextFilePath:    "/workspace/queued.md",
				ContextFileContent: "queued context",
			}},
		})
		require.NoError(t, err)
		require.Equal(t, chat.ID, resp.ChatID)
		require.Equal(t, 1, resp.Count)

		require.Empty(t, requireAgentChatContextMessages(ctx, t, setup.db, chat.ID))

		queued, err := setup.db.GetChatQueuedMessages(dbauthz.AsSystemRestricted(ctx), chat.ID)
		require.NoError(t, err)
		require.Len(t, queued, 1)
		require.Equal(t, setup.user.UserID, queued[0].CreatedBy)
		require.True(t, queued[0].ModelConfigID.Valid)
		require.Equal(t, model.ID, queued[0].ModelConfigID.UUID)
		require.True(t, queued[0].APIKeyID.Valid)
		require.Equal(t, apiKeyID, queued[0].APIKeyID.String)

		parts := requireAgentChatContextParts(t, queued[0].Content)
		require.Len(t, parts, 1)
		require.Equal(t, "/workspace/queued.md", parts[0].ContextFilePath)
		require.Equal(t, "queued context", parts[0].ContextFileContent)
		require.Equal(t, uuid.NullUUID{UUID: setup.workspace.Agents[0].ID, Valid: true}, parts[0].ContextFileAgentID)

		persisted, err := setup.db.GetChatByID(dbauthz.AsSystemRestricted(ctx), chat.ID)
		require.NoError(t, err)
		require.False(t, persisted.LastInjectedContext.Valid)
		require.Equal(t, database.ChatStatusInterrupting, persisted.Status)
		require.Equal(t, chat.SnapshotVersion+1, persisted.SnapshotVersion)
		require.Equal(t, chat.HistoryVersion, persisted.HistoryVersion)
		require.Equal(t, persisted.SnapshotVersion, persisted.QueueVersion)
	})

	t.Run("AddFailsWhenQueueIsFull", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)
		setup := newAgentChatContextTestSetup(t)
		model := coderd.InsertAgentChatTestModelConfig(t, setup.db, setup.user.UserID)
		chat := createAgentChatContextChat(t, setup.db, setup.user.OrganizationID, setup.user.UserID, model.ID, setup.workspace.Agents[0].ID, t.Name())
		chat = setAgentChatContextChatStatus(ctx, t, setup.db, chat.ID, database.ChatStatusRunning)
		chat = acquireAgentChatContextChat(ctx, t, setup.db, chat.ID)
		apiKeyID := currentAgentChatContextAPIKeyID(t, setup.client)
		for i := range int(chatstate.MaxQueueSize) {
			content, err := chatprompt.MarshalParts([]codersdk.ChatMessagePart{
				codersdk.ChatMessageText(fmt.Sprintf("queued %d", i)),
			})
			require.NoError(t, err)
			_, err = setup.db.InsertChatQueuedMessageWithCreator(
				dbauthz.AsSystemRestricted(ctx),
				database.InsertChatQueuedMessageWithCreatorParams{
					ChatID:        chat.ID,
					Content:       content.RawMessage,
					ModelConfigID: uuid.NullUUID{UUID: model.ID, Valid: true},
					APIKeyID:      sql.NullString{String: apiKeyID, Valid: true},
					CreatedBy:     setup.user.UserID,
				},
			)
			require.NoError(t, err)
		}

		_, err := setup.agentClient.AddChatContext(ctx, agentsdk.AddChatContextRequest{
			ChatID: chat.ID,
			Parts: []codersdk.ChatMessagePart{{
				Type:               codersdk.ChatMessagePartTypeContextFile,
				ContextFilePath:    "/workspace/overflow.md",
				ContextFileContent: "overflow context",
			}},
		})
		sdkErr := requireSDKError(t, err, http.StatusTooManyRequests)
		require.Equal(t, "Message queue is full.", sdkErr.Message)
		require.Contains(t, sdkErr.Detail, "Maximum")
	})

	t.Run("AddFailsWhenChatStateIsInvalid", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)
		setup := newAgentChatContextTestSetup(t)
		model := coderd.InsertAgentChatTestModelConfig(t, setup.db, setup.user.UserID)
		chat := createAgentChatContextChat(t, setup.db, setup.user.OrganizationID, setup.user.UserID, model.ID, setup.workspace.Agents[0].ID, t.Name())
		_ = setAgentChatContextChatStatus(ctx, t, setup.db, chat.ID, database.ChatStatusPending)

		_, err := setup.agentClient.AddChatContext(ctx, agentsdk.AddChatContextRequest{
			ChatID: chat.ID,
			Parts: []codersdk.ChatMessagePart{{
				Type:               codersdk.ChatMessagePartTypeContextFile,
				ContextFilePath:    "/workspace/invalid.md",
				ContextFileContent: "invalid state context",
			}},
		})
		sdkErr := requireSDKError(t, err, http.StatusConflict)
		require.Equal(t, "Chat is in an invalid state.", sdkErr.Message)
	})

	t.Run("ClearDeletesSkillMessages", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)
		setup := newAgentChatContextTestSetup(t)
		model := coderd.InsertAgentChatTestModelConfig(t, setup.db, setup.user.UserID)
		chat := createAgentChatContextChat(t, setup.db, setup.user.OrganizationID, setup.user.UserID, model.ID, setup.workspace.Agents[0].ID, t.Name())

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

	t.Run("ClearDeletesSkillMessagesBeforeCompressedSummary", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)
		setup := newAgentChatContextTestSetup(t)
		model := coderd.InsertAgentChatTestModelConfig(t, setup.db, setup.user.UserID)
		chat := createAgentChatContextChat(t, setup.db, setup.user.OrganizationID, setup.user.UserID, model.ID, setup.workspace.Agents[0].ID, t.Name())

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

		messages := requireAgentChatContextMessages(ctx, t, setup.db, chat.ID)
		require.Len(t, messages, 1)

		storedParts := requireAgentChatContextParts(t, messages[0].Content.RawMessage)
		require.Len(t, storedParts, 2)

		// Strip the sentinel so the skill message must be found by the
		// full-history scan even after compaction hides it from the
		// prompt-scoped query.
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

		summaryContent, err := chatprompt.MarshalParts([]codersdk.ChatMessagePart{
			codersdk.ChatMessageText("compressed summary"),
		})
		require.NoError(t, err)
		_ = dbgen.ChatMessage(t, setup.db, database.ChatMessage{
			ChatID:         chat.ID,
			Role:           database.ChatMessageRoleUser,
			Content:        summaryContent,
			Visibility:     database.ChatMessageVisibilityModel,
			ModelConfigID:  uuid.NullUUID{UUID: chat.LastModelConfigID, Valid: true},
			ContentVersion: chatprompt.CurrentContentVersion,
			CreatedBy:      uuid.NullUUID{UUID: setup.user.UserID, Valid: true},
			Compressed:     true,
		})

		regularContent, err := chatprompt.MarshalParts([]codersdk.ChatMessagePart{
			codersdk.ChatMessageText("keep this user message"),
		})
		require.NoError(t, err)
		_ = dbgen.ChatMessage(t, setup.db, database.ChatMessage{
			ChatID:         chat.ID,
			Role:           database.ChatMessageRoleUser,
			Content:        regularContent,
			Visibility:     database.ChatMessageVisibilityBoth,
			ModelConfigID:  uuid.NullUUID{UUID: chat.LastModelConfigID, Valid: true},
			ContentVersion: chatprompt.CurrentContentVersion,
			CreatedBy:      uuid.NullUUID{UUID: setup.user.UserID, Valid: true},
		})
		resp, err := setup.agentClient.ClearChatContext(ctx, agentsdk.ClearChatContextRequest{})
		require.NoError(t, err)
		require.Equal(t, chat.ID, resp.ChatID)

		messages = requireAgentChatContextMessages(ctx, t, setup.db, chat.ID)
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

	t.Run("ClearSuccessDeletesInjectedContext", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)
		setup := newAgentChatContextTestSetup(t)
		model := coderd.InsertAgentChatTestModelConfig(t, setup.db, setup.user.UserID)
		chat := createAgentChatContextChat(t, setup.db, setup.user.OrganizationID, setup.user.UserID, model.ID, setup.workspace.Agents[0].ID, t.Name())

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
		_ = dbgen.ChatMessage(t, setup.db, database.ChatMessage{
			ChatID:         chat.ID,
			Role:           database.ChatMessageRoleUser,
			Content:        regularContent,
			Visibility:     database.ChatMessageVisibilityBoth,
			ModelConfigID:  uuid.NullUUID{UUID: chat.LastModelConfigID, Valid: true},
			ContentVersion: chatprompt.CurrentContentVersion,
			CreatedBy:      uuid.NullUUID{UUID: setup.user.UserID, Valid: true},
		})
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

	t.Run("ClearSuccessResetsProviderResponseChain", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)
		setup := newAgentChatContextTestSetup(t)
		model := coderd.InsertAgentChatTestModelConfig(t, setup.db, setup.user.UserID)
		chat := createAgentChatContextChat(t, setup.db, setup.user.OrganizationID, setup.user.UserID, model.ID, setup.workspace.Agents[0].ID, t.Name())

		_, err := setup.agentClient.AddChatContext(ctx, agentsdk.AddChatContextRequest{
			Parts: []codersdk.ChatMessagePart{{
				Type:               codersdk.ChatMessagePartTypeContextFile,
				ContextFilePath:    "/workspace/instructions.md",
				ContextFileContent: "remember this file",
			}},
		})
		require.NoError(t, err)

		assistantContent, err := chatprompt.MarshalParts([]codersdk.ChatMessagePart{
			codersdk.ChatMessageText("assistant reply"),
		})
		require.NoError(t, err)
		_ = dbgen.ChatMessage(t, setup.db, database.ChatMessage{
			ChatID:             chat.ID,
			Role:               database.ChatMessageRoleAssistant,
			Content:            assistantContent,
			Visibility:         database.ChatMessageVisibilityBoth,
			ModelConfigID:      uuid.NullUUID{UUID: chat.LastModelConfigID, Valid: true},
			ContentVersion:     chatprompt.CurrentContentVersion,
			ProviderResponseID: sql.NullString{String: "resp-123", Valid: true},
		})

		messages := requireAgentChatContextMessages(ctx, t, setup.db, chat.ID)
		require.Len(t, messages, 2)
		require.Equal(t, database.ChatMessageRoleAssistant, messages[1].Role)
		require.True(t, messages[1].ProviderResponseID.Valid)
		require.Equal(t, "resp-123", messages[1].ProviderResponseID.String)

		resp, err := setup.agentClient.ClearChatContext(ctx, agentsdk.ClearChatContextRequest{})
		require.NoError(t, err)
		require.Equal(t, chat.ID, resp.ChatID)

		messages = requireAgentChatContextMessages(ctx, t, setup.db, chat.ID)
		require.Len(t, messages, 1)
		require.Equal(t, database.ChatMessageRoleAssistant, messages[0].Role)
		require.False(t, messages[0].ProviderResponseID.Valid)

		remainingParts := requireAgentChatContextParts(t, messages[0].Content.RawMessage)
		require.Len(t, remainingParts, 1)
		require.Equal(t, codersdk.ChatMessagePartTypeText, remainingParts[0].Type)
		require.Equal(t, "assistant reply", remainingParts[0].Text)

		persistedChat, err := setup.db.GetChatByID(dbauthz.AsSystemRestricted(ctx), chat.ID)
		require.NoError(t, err)
		require.False(t, persistedChat.LastInjectedContext.Valid)
	})

	t.Run("ClearWithoutContextPreservesProviderResponseChain", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)
		setup := newAgentChatContextTestSetup(t)
		model := coderd.InsertAgentChatTestModelConfig(t, setup.db, setup.user.UserID)
		chat := createAgentChatContextChat(t, setup.db, setup.user.OrganizationID, setup.user.UserID, model.ID, setup.workspace.Agents[0].ID, t.Name())

		assistantContent, err := chatprompt.MarshalParts([]codersdk.ChatMessagePart{
			codersdk.ChatMessageText("assistant reply"),
		})
		require.NoError(t, err)
		_ = dbgen.ChatMessage(t, setup.db, database.ChatMessage{
			ChatID:             chat.ID,
			Role:               database.ChatMessageRoleAssistant,
			Content:            assistantContent,
			Visibility:         database.ChatMessageVisibilityBoth,
			ModelConfigID:      uuid.NullUUID{UUID: chat.LastModelConfigID, Valid: true},
			ContentVersion:     chatprompt.CurrentContentVersion,
			ProviderResponseID: sql.NullString{String: "resp-123", Valid: true},
		})
		resp, err := setup.agentClient.ClearChatContext(ctx, agentsdk.ClearChatContextRequest{ChatID: chat.ID})
		require.NoError(t, err)
		require.Equal(t, chat.ID, resp.ChatID)

		messages := requireAgentChatContextMessages(ctx, t, setup.db, chat.ID)
		require.Len(t, messages, 1)
		require.True(t, messages[0].ProviderResponseID.Valid)
		require.Equal(t, "resp-123", messages[0].ProviderResponseID.String)
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
		model := coderd.InsertAgentChatTestModelConfig(t, db, user.UserID)

		firstWorkspace := dbfake.WorkspaceBuild(t, db, database.WorkspaceTable{
			OrganizationID: user.OrganizationID,
			OwnerID:        user.UserID,
		}).WithAgent().Do()
		secondWorkspace := dbfake.WorkspaceBuild(t, db, database.WorkspaceTable{
			OrganizationID: user.OrganizationID,
			OwnerID:        user.UserID,
		}).WithAgent().Do()

		chat := createAgentChatContextChat(t, db, user.OrganizationID, user.UserID, model.ID, firstWorkspace.Agents[0].ID, t.Name())
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

	t.Run("AddRejectsChatOwnedByAnotherUserOnSameAgent", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)
		setup := newAgentChatContextTestSetup(t)
		_, otherUser := coderdtest.CreateAnotherUser(t, setup.client, setup.user.OrganizationID)
		model := coderd.InsertAgentChatTestModelConfig(t, setup.db, setup.user.UserID)
		chat := createAgentChatContextChat(t, setup.db, setup.user.OrganizationID, otherUser.ID, model.ID, setup.workspace.Agents[0].ID, t.Name())

		_, err := setup.agentClient.AddChatContext(ctx, agentsdk.AddChatContextRequest{
			ChatID: chat.ID,
			Parts: []codersdk.ChatMessagePart{{
				Type:               codersdk.ChatMessagePartTypeContextFile,
				ContextFilePath:    "/workspace/foreign.md",
				ContextFileContent: "not your chat",
			}},
		})
		sdkErr := requireSDKError(t, err, http.StatusForbidden)
		require.Equal(t, "Chat does not belong to this workspace owner.", sdkErr.Message)
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

	t.Run("AddRejectsEmptyContextFileParts", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)
		setup := newAgentChatContextTestSetup(t)

		_, err := setup.agentClient.AddChatContext(ctx, agentsdk.AddChatContextRequest{
			Parts: []codersdk.ChatMessagePart{{
				Type:            codersdk.ChatMessagePartTypeContextFile,
				ContextFilePath: "/workspace/empty.md",
			}},
		})
		sdkErr := requireSDKError(t, err, http.StatusBadRequest)
		require.Equal(t, "No context-file or skill parts provided.", sdkErr.Message)
	})

	t.Run("AddRejectsWhitespaceOnlyContextFileParts", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)
		setup := newAgentChatContextTestSetup(t)
		model := coderd.InsertAgentChatTestModelConfig(t, setup.db, setup.user.UserID)
		chat := createAgentChatContextChat(t, setup.db, setup.user.OrganizationID, setup.user.UserID, model.ID, setup.workspace.Agents[0].ID, t.Name())

		_, err := setup.agentClient.AddChatContext(ctx, agentsdk.AddChatContextRequest{
			ChatID: chat.ID,
			Parts: []codersdk.ChatMessagePart{{
				Type:               codersdk.ChatMessagePartTypeContextFile,
				ContextFilePath:    "/workspace/whitespace.md",
				ContextFileContent: "   \n\t",
			}},
		})
		sdkErr := requireSDKError(t, err, http.StatusBadRequest)
		require.Equal(t, "No context-file or skill parts provided.", sdkErr.Message)
	})

	t.Run("AddTruncatesOversizedContextFileParts", func(t *testing.T) {
		t.Parallel()

		const maxContextFileBytes = 64 * 1024

		ctx := testutil.Context(t, testutil.WaitLong)
		setup := newAgentChatContextTestSetup(t)
		model := coderd.InsertAgentChatTestModelConfig(t, setup.db, setup.user.UserID)
		chat := createAgentChatContextChat(t, setup.db, setup.user.OrganizationID, setup.user.UserID, model.ID, setup.workspace.Agents[0].ID, t.Name())
		largeContent := strings.Repeat("a", maxContextFileBytes+100)

		resp, err := setup.agentClient.AddChatContext(ctx, agentsdk.AddChatContextRequest{
			ChatID: chat.ID,
			Parts: []codersdk.ChatMessagePart{{
				Type:               codersdk.ChatMessagePartTypeContextFile,
				ContextFilePath:    "/workspace/AGENTS.md",
				ContextFileContent: largeContent,
			}},
		})
		require.NoError(t, err)
		require.Equal(t, chat.ID, resp.ChatID)
		require.Equal(t, 1, resp.Count)

		messages := requireAgentChatContextStoredMessages(t, requireAgentChatContextMessages(ctx, t, setup.db, chat.ID))
		require.Len(t, messages, 1)
		require.Len(t, messages[0], 1)
		require.True(t, messages[0][0].ContextFileTruncated)
		require.Len(t, messages[0][0].ContextFileContent, maxContextFileBytes)
		require.Equal(t, largeContent[:maxContextFileBytes], messages[0][0].ContextFileContent)

		cached := requireAgentChatContextCachedParts(ctx, t, setup.db, chat.ID)
		require.Len(t, cached, 1)
		require.True(t, cached[0].ContextFileTruncated)
	})

	t.Run("AddSanitizesBeforeApplyingContextFileSizeCap", func(t *testing.T) {
		t.Parallel()

		const maxContextFileBytes = 64 * 1024

		ctx := testutil.Context(t, testutil.WaitLong)
		setup := newAgentChatContextTestSetup(t)
		model := coderd.InsertAgentChatTestModelConfig(t, setup.db, setup.user.UserID)
		chat := createAgentChatContextChat(t, setup.db, setup.user.OrganizationID, setup.user.UserID, model.ID, setup.workspace.Agents[0].ID, t.Name())

		visible := strings.Repeat("a", maxContextFileBytes-1)
		content := visible + strings.Repeat("\u200b", 100) + "z"

		resp, err := setup.agentClient.AddChatContext(ctx, agentsdk.AddChatContextRequest{
			ChatID: chat.ID,
			Parts: []codersdk.ChatMessagePart{{
				Type:               codersdk.ChatMessagePartTypeContextFile,
				ContextFilePath:    "/workspace/AGENTS.md",
				ContextFileContent: content,
			}},
		})
		require.NoError(t, err)
		require.Equal(t, chat.ID, resp.ChatID)
		require.Equal(t, 1, resp.Count)

		messages := requireAgentChatContextStoredMessages(t, requireAgentChatContextMessages(ctx, t, setup.db, chat.ID))
		require.Len(t, messages, 1)
		require.Len(t, messages[0], 1)
		require.False(t, messages[0][0].ContextFileTruncated)
		require.Equal(t, visible+"z", messages[0][0].ContextFileContent)
	})

	t.Run("ClearIsIdempotentWhenNoActiveChatExists", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)
		setup := newAgentChatContextTestSetup(t)

		resp, err := setup.agentClient.ClearChatContext(ctx, agentsdk.ClearChatContextRequest{})
		require.NoError(t, err)
		require.Equal(t, uuid.Nil, resp.ChatID)
	})

	t.Run("AddUsesWorkspaceOwnerChatWhenAnotherUsersChatIsActive", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)
		setup := newAgentChatContextTestSetup(t)
		_, otherUser := coderdtest.CreateAnotherUser(t, setup.client, setup.user.OrganizationID)
		model := coderd.InsertAgentChatTestModelConfig(t, setup.db, setup.user.UserID)
		ownerChat := createAgentChatContextChat(t, setup.db, setup.user.OrganizationID, setup.user.UserID, model.ID, setup.workspace.Agents[0].ID, t.Name()+"-owner")
		foreignChat := createAgentChatContextChat(t, setup.db, setup.user.OrganizationID, otherUser.ID, model.ID, setup.workspace.Agents[0].ID, t.Name()+"-foreign")

		resp, err := setup.agentClient.AddChatContext(ctx, agentsdk.AddChatContextRequest{
			Parts: []codersdk.ChatMessagePart{{
				Type:               codersdk.ChatMessagePartTypeContextFile,
				ContextFilePath:    "/workspace/file.go",
				ContextFileContent: "content",
			}},
		})
		require.NoError(t, err)
		require.Equal(t, ownerChat.ID, resp.ChatID)

		ownerMessages := requireAgentChatContextMessages(ctx, t, setup.db, ownerChat.ID)
		require.Len(t, ownerMessages, 1)
		require.Empty(t, requireAgentChatContextMessages(ctx, t, setup.db, foreignChat.ID))
	})

	t.Run("AddUsesRootChatWhenOnlySubagentMakesActiveChatAmbiguous", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)
		setup := newAgentChatContextTestSetup(t)
		model := coderd.InsertAgentChatTestModelConfig(t, setup.db, setup.user.UserID)
		rootChat := createAgentChatContextChat(t, setup.db, setup.user.OrganizationID, setup.user.UserID, model.ID, setup.workspace.Agents[0].ID, t.Name()+"-root")
		childChat := createAgentChatContextChildChat(t, setup.db, setup.user.OrganizationID, setup.user.UserID, model.ID, setup.workspace.Agents[0].ID, rootChat.ID, t.Name()+"-child")

		resp, err := setup.agentClient.AddChatContext(ctx, agentsdk.AddChatContextRequest{
			Parts: []codersdk.ChatMessagePart{{
				Type:               codersdk.ChatMessagePartTypeContextFile,
				ContextFilePath:    "/workspace/file.go",
				ContextFileContent: "content",
			}},
		})
		require.NoError(t, err)
		require.Equal(t, rootChat.ID, resp.ChatID)

		rootMessages := requireAgentChatContextMessages(ctx, t, setup.db, rootChat.ID)
		require.Len(t, rootMessages, 1)
		require.Empty(t, requireAgentChatContextMessages(ctx, t, setup.db, childChat.ID))
	})

	t.Run("AddFailsWithMultipleActiveChats", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)
		setup := newAgentChatContextTestSetup(t)
		model := coderd.InsertAgentChatTestModelConfig(t, setup.db, setup.user.UserID)
		createAgentChatContextChat(t, setup.db, setup.user.OrganizationID, setup.user.UserID, model.ID, setup.workspace.Agents[0].ID, t.Name()+"-chat1")
		createAgentChatContextChat(t, setup.db, setup.user.OrganizationID, setup.user.UserID, model.ID, setup.workspace.Agents[0].ID, t.Name()+"-chat2")

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

	t.Run("ClearUsesRootChatWhenOnlySubagentMakesActiveChatAmbiguous", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)
		setup := newAgentChatContextTestSetup(t)
		model := coderd.InsertAgentChatTestModelConfig(t, setup.db, setup.user.UserID)
		rootChat := createAgentChatContextChat(t, setup.db, setup.user.OrganizationID, setup.user.UserID, model.ID, setup.workspace.Agents[0].ID, t.Name()+"-root")
		childChat := createAgentChatContextChildChat(t, setup.db, setup.user.OrganizationID, setup.user.UserID, model.ID, setup.workspace.Agents[0].ID, rootChat.ID, t.Name()+"-child")

		_, err := setup.agentClient.AddChatContext(ctx, agentsdk.AddChatContextRequest{
			ChatID: rootChat.ID,
			Parts: []codersdk.ChatMessagePart{{
				Type:               codersdk.ChatMessagePartTypeContextFile,
				ContextFilePath:    "/workspace/file.go",
				ContextFileContent: "content",
			}},
		})
		require.NoError(t, err)

		resp, err := setup.agentClient.ClearChatContext(ctx, agentsdk.ClearChatContextRequest{})
		require.NoError(t, err)
		require.Equal(t, rootChat.ID, resp.ChatID)

		require.Empty(t, requireAgentChatContextMessages(ctx, t, setup.db, rootChat.ID))
		require.Empty(t, requireAgentChatContextMessages(ctx, t, setup.db, childChat.ID))
	})

	t.Run("ClearUsesWorkspaceOwnerChatWhenAnotherUsersChatIsActive", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)
		setup := newAgentChatContextTestSetup(t)
		_, otherUser := coderdtest.CreateAnotherUser(t, setup.client, setup.user.OrganizationID)
		model := coderd.InsertAgentChatTestModelConfig(t, setup.db, setup.user.UserID)
		ownerChat := createAgentChatContextChat(t, setup.db, setup.user.OrganizationID, setup.user.UserID, model.ID, setup.workspace.Agents[0].ID, t.Name()+"-owner")
		_ = createAgentChatContextChat(t, setup.db, setup.user.OrganizationID, otherUser.ID, model.ID, setup.workspace.Agents[0].ID, t.Name()+"-foreign")

		_, err := setup.agentClient.AddChatContext(ctx, agentsdk.AddChatContextRequest{
			ChatID: ownerChat.ID,
			Parts: []codersdk.ChatMessagePart{{
				Type:               codersdk.ChatMessagePartTypeContextFile,
				ContextFilePath:    "/workspace/file.go",
				ContextFileContent: "content",
			}},
		})
		require.NoError(t, err)

		resp, err := setup.agentClient.ClearChatContext(ctx, agentsdk.ClearChatContextRequest{})
		require.NoError(t, err)
		require.Equal(t, ownerChat.ID, resp.ChatID)
		require.Empty(t, requireAgentChatContextMessages(ctx, t, setup.db, ownerChat.ID))
	})

	t.Run("ClearRejectsChatOwnedByAnotherUserOnSameAgent", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)
		setup := newAgentChatContextTestSetup(t)
		_, otherUser := coderdtest.CreateAnotherUser(t, setup.client, setup.user.OrganizationID)
		model := coderd.InsertAgentChatTestModelConfig(t, setup.db, setup.user.UserID)
		chat := createAgentChatContextChat(t, setup.db, setup.user.OrganizationID, otherUser.ID, model.ID, setup.workspace.Agents[0].ID, t.Name())

		_, err := setup.agentClient.ClearChatContext(ctx, agentsdk.ClearChatContextRequest{ChatID: chat.ID})
		sdkErr := requireSDKError(t, err, http.StatusForbidden)
		require.Equal(t, "Chat does not belong to this workspace owner.", sdkErr.Message)
	})

	t.Run("AddFailsWhenChatIsNotActive", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)
		setup := newAgentChatContextTestSetup(t)
		model := coderd.InsertAgentChatTestModelConfig(t, setup.db, setup.user.UserID)
		chat := createAgentChatContextChat(t, setup.db, setup.user.OrganizationID, setup.user.UserID, model.ID, setup.workspace.Agents[0].ID, t.Name())

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

func currentAgentChatContextAPIKeyID(t testing.TB, client *codersdk.Client) string {
	t.Helper()

	apiKeyID, _, ok := strings.Cut(client.SessionToken(), "-")
	require.True(t, ok)
	require.NotEmpty(t, apiKeyID)
	return apiKeyID
}

func setAgentChatContextChatStatus(
	ctx context.Context,
	t testing.TB,
	db database.Store,
	chatID uuid.UUID,
	status database.ChatStatus,
) database.Chat {
	t.Helper()

	chat, err := db.UpdateChatStatus(dbauthz.AsSystemRestricted(ctx), database.UpdateChatStatusParams{
		ID:     chatID,
		Status: status,
	})
	require.NoError(t, err)
	return chat
}

func acquireAgentChatContextChat(ctx context.Context, t testing.TB, db database.Store, chatID uuid.UUID) database.Chat {
	t.Helper()

	machine := chatstate.NewChatMachine(db, dbpubsub.NewInMemory(), chatID)
	require.NoError(t, machine.Update(dbauthz.AsSystemRestricted(ctx), func(tx *chatstate.Tx, store database.Store) error {
		_, err := tx.Acquire(chatstate.AcquireInput{WorkerID: uuid.New(), RunnerID: uuid.New()})
		return err
	}))
	chat, err := db.GetChatByID(dbauthz.AsSystemRestricted(ctx), chatID)
	require.NoError(t, err)
	return chat
}

func createAgentChatContextChat(
	t testing.TB,
	db database.Store,
	orgID uuid.UUID,
	ownerID uuid.UUID,
	modelConfigID uuid.UUID,
	agentID uuid.UUID,
	title string,
) database.Chat {
	t.Helper()

	return dbgen.Chat(t, db, database.Chat{
		OrganizationID:    orgID,
		OwnerID:           ownerID,
		LastModelConfigID: modelConfigID,
		Title:             title,
		AgentID:           uuid.NullUUID{UUID: agentID, Valid: true},
	})
}

func createAgentChatContextChildChat(
	t testing.TB,
	db database.Store,
	orgID uuid.UUID,
	ownerID uuid.UUID,
	modelConfigID uuid.UUID,
	agentID uuid.UUID,
	parentChatID uuid.UUID,
	title string,
) database.Chat {
	t.Helper()

	return dbgen.Chat(t, db, database.Chat{
		OrganizationID:    orgID,
		OwnerID:           ownerID,
		LastModelConfigID: modelConfigID,
		Title:             title,
		AgentID:           uuid.NullUUID{UUID: agentID, Valid: true},
		ParentChatID:      uuid.NullUUID{UUID: parentChatID, Valid: true},
		RootChatID:        uuid.NullUUID{UUID: parentChatID, Valid: true},
	})
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
