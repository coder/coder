package chatd //nolint:testpackage // Exercises unexported re-derivation helpers.

import (
	"database/sql"
	"encoding/json"
	"strings"
	"testing"

	"charm.land/fantasy"
	fantasyopenai "charm.land/fantasy/providers/openai"
	"github.com/google/uuid"
	"github.com/sqlc-dev/pqtype"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"cdr.dev/slog/v3/sloggers/slogtest"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbgen"
	"github.com/coder/coder/v2/coderd/database/dbtestutil"
	"github.com/coder/coder/v2/coderd/x/chatd/chatprompt"
	"github.com/coder/coder/v2/coderd/x/chatd/chatprovider"
	"github.com/coder/coder/v2/coderd/x/chatd/chatstate"
	"github.com/coder/coder/v2/coderd/x/chatd/chattool"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/testutil"
)

func mustMarshalText(t *testing.T, parts ...string) pqtype.NullRawMessage {
	t.Helper()
	messageParts := make([]codersdk.ChatMessagePart, 0, len(parts))
	for _, p := range parts {
		messageParts = append(messageParts, codersdk.ChatMessageText(p))
	}
	content, err := chatprompt.MarshalParts(messageParts)
	require.NoError(t, err)
	return content
}

func textMessage(t *testing.T, id int64, role database.ChatMessageRole, parts ...string) database.ChatMessage {
	t.Helper()
	return database.ChatMessage{
		ID:             id,
		Role:           role,
		Content:        mustMarshalText(t, parts...),
		ContentVersion: chatprompt.CurrentContentVersion,
	}
}

func TestLatestAssistantText(t *testing.T) {
	t.Parallel()

	t.Run("ReturnsMostRecentAssistantMessage", func(t *testing.T) {
		t.Parallel()
		messages := []database.ChatMessage{
			textMessage(t, 1, database.ChatMessageRoleUser, "hi"),
			textMessage(t, 2, database.ChatMessageRoleAssistant, "first answer"),
			textMessage(t, 3, database.ChatMessageRoleTool, "tool result"),
			textMessage(t, 4, database.ChatMessageRoleAssistant, "  final answer  "),
		}
		require.Equal(t, "final answer", latestAssistantText(messages))
	})

	t.Run("ConcatenatesTextParts", func(t *testing.T) {
		t.Parallel()
		messages := []database.ChatMessage{
			textMessage(t, 1, database.ChatMessageRoleAssistant, "foo", "bar"),
		}
		require.Equal(t, "foobar", latestAssistantText(messages))
	})

	t.Run("NoAssistantMessage", func(t *testing.T) {
		t.Parallel()
		messages := []database.ChatMessage{
			textMessage(t, 1, database.ChatMessageRoleUser, "hi"),
			textMessage(t, 2, database.ChatMessageRoleTool, "tool result"),
		}
		require.Empty(t, latestAssistantText(messages))
	})

	t.Run("EmptyAssistantText", func(t *testing.T) {
		t.Parallel()
		messages := []database.ChatMessage{
			textMessage(t, 1, database.ChatMessageRoleAssistant, "   "),
		}
		require.Empty(t, latestAssistantText(messages))
	})

	t.Run("EmptyHistory", func(t *testing.T) {
		t.Parallel()
		require.Empty(t, latestAssistantText(nil))
	})
}

func TestPrepareGenerationClampsRequestedReasoningEffortToMax(t *testing.T) {
	t.Parallel()

	db, ps := dbtestutil.NewDB(t)
	ctx := chatdTestContext(t)
	user := dbgen.User(t, db, database.User{})
	org := dbgen.Organization(t, db, database.Organization{})
	dbgen.OrganizationMember(t, db, database.OrganizationMember{
		UserID:         user.ID,
		OrganizationID: org.ID,
	})
	provider := dbgen.AIProviderWithOptionalKey(t, db, database.AIProvider{
		Type: database.AIProviderTypeOpenai,
	}, "test-key")
	modelConfigRaw, err := json.Marshal(codersdk.ChatModelCallConfig{
		ProviderOptions: &codersdk.ChatModelProviderOptions{
			OpenAI: &codersdk.ChatModelOpenAIProviderOptions{
				User: new("turn-options-sentinel"),
			},
		},
		ReasoningEffort: &codersdk.ChatModelReasoningEffortConfig{
			Default: new(codersdk.ChatModelReasoningEffortLow),
			Max:     new(codersdk.ChatModelReasoningEffortMedium),
		},
	})
	require.NoError(t, err)
	modelConfig := dbgen.ChatModelConfig(t, db, database.ChatModelConfig{
		Model:          "gpt-4o-mini",
		Options:        modelConfigRaw,
		AIProviderID:   uuid.NullUUID{UUID: provider.ID, Valid: true},
		OrganizationID: org.ID,
	}, func(p *database.InsertChatModelConfigParams) {
		p.Enabled = true
	})

	created, err := chatstate.CreateChat(ctx, db, ps, chatstate.CreateChatInput{
		OrganizationID:    org.ID,
		OwnerID:           user.ID,
		LastModelConfigID: modelConfig.ID,
		Title:             "clamp reasoning effort",
		ClientType:        database.ChatClientTypeApi,
		InitialMessages: []chatstate.Message{
			{
				Role:          database.ChatMessageRoleUser,
				Content:       mustMarshalText(t, "hello"),
				Visibility:    database.ChatMessageVisibilityBoth,
				ModelConfigID: uuid.NullUUID{UUID: modelConfig.ID, Valid: true},
				ReasoningEffort: database.NullChatReasoningEffort{
					ChatReasoningEffort: database.ChatReasoningEffortHigh,
					Valid:               true,
				},
				CreatedBy:      uuid.NullUUID{UUID: user.ID, Valid: true},
				ContentVersion: chatprompt.CurrentContentVersion,
			},
		},
	})
	require.NoError(t, err)

	server := newInternalTestServer(
		t,
		db,
		ps,
		chatprovider.ProviderAPIKeys{},
		withInternalTestServerTransportFactory(&aibridgeTestFactory{}),
	)
	prepared, err := server.prepareGeneration(ctx, generationPrepareInput{
		Chat:     created.Chat,
		Messages: created.InitialMessages,
	})
	require.NoError(t, err)
	t.Cleanup(prepared.Cleanup)

	providerOptions, ok := prepared.CallTemplate.ProviderOptions[fantasyopenai.Name].(*fantasyopenai.ResponsesProviderOptions)
	require.True(t, ok, "%T", prepared.CallTemplate.ProviderOptions[fantasyopenai.Name])
	require.NotNil(t, providerOptions.ReasoningEffort)
	require.Equal(t, fantasyopenai.ReasoningEffortMedium, *providerOptions.ReasoningEffort)

	require.NotNil(t, providerOptions.User)
	require.Equal(t, "turn-options-sentinel", *providerOptions.User)
	require.NotNil(t, prepared.CallTemplate.MaxOutputTokens)
	require.Equal(t, defaultChatMaxOutputTokens, *prepared.CallTemplate.MaxOutputTokens)

	require.NotNil(t, prepared.Compaction)
	summaryCall := prepared.Compaction.Options.SummaryCall
	require.Equal(t, prepared.CallTemplate.ProviderOptions, summaryCall.ProviderOptions)
	require.NotNil(t, summaryCall.ToolChoice)
	require.Equal(t, fantasy.ToolChoiceNone, *summaryCall.ToolChoice)
	// Non-streaming summaries must not inherit the default output cap the
	// Anthropic SDK rejects.
	require.Nil(t, summaryCall.MaxOutputTokens)
}

// Without an active goal the built-in goal tools are not offered, so
// their names must not become stop-after entries where they would stop
// the turn after a colliding user-defined dynamic tool.
func TestPrepareGenerationStopAfterGoalToolsRequireActiveGoal(t *testing.T) {
	t.Parallel()

	db, ps := dbtestutil.NewDB(t)
	ctx := chatdTestContext(t)
	user := dbgen.User(t, db, database.User{})
	org := dbgen.Organization(t, db, database.Organization{})
	dbgen.OrganizationMember(t, db, database.OrganizationMember{
		UserID:         user.ID,
		OrganizationID: org.ID,
	})
	provider := dbgen.AIProviderWithOptionalKey(t, db, database.AIProvider{
		Type: database.AIProviderTypeOpenai,
	}, "test-key")
	modelConfig := dbgen.ChatModelConfig(t, db, database.ChatModelConfig{
		Model:          "gpt-4o-mini",
		AIProviderID:   uuid.NullUUID{UUID: provider.ID, Valid: true},
		OrganizationID: org.ID,
	}, func(p *database.InsertChatModelConfigParams) {
		p.Enabled = true
	})
	created, err := chatstate.CreateChat(ctx, db, ps, chatstate.CreateChatInput{
		OrganizationID:    org.ID,
		OwnerID:           user.ID,
		LastModelConfigID: modelConfig.ID,
		Title:             "goal stop-after gating",
		ClientType:        database.ChatClientTypeApi,
		InitialMessages: []chatstate.Message{
			{
				Role:           database.ChatMessageRoleUser,
				Content:        mustMarshalText(t, "hello"),
				Visibility:     database.ChatMessageVisibilityBoth,
				ModelConfigID:  uuid.NullUUID{UUID: modelConfig.ID, Valid: true},
				CreatedBy:      uuid.NullUUID{UUID: user.ID, Valid: true},
				ContentVersion: chatprompt.CurrentContentVersion,
			},
		},
	})
	require.NoError(t, err)

	server := newInternalTestServer(
		t,
		db,
		ps,
		chatprovider.ProviderAPIKeys{},
		withInternalTestServerTransportFactory(&aibridgeTestFactory{}),
	)

	prepared, err := server.prepareGeneration(ctx, generationPrepareInput{
		Chat:     created.Chat,
		Messages: created.InitialMessages,
	})
	require.NoError(t, err)
	t.Cleanup(prepared.Cleanup)
	require.NotContains(t, prepared.StopAfterTools, chattool.CompleteGoalToolName)

	goal, err := db.InsertActiveChatGoal(ctx, database.InsertActiveChatGoalParams{
		RootChatID:      created.Chat.ID,
		Objective:       "stay on task",
		CreatedByUserID: user.ID,
	})
	require.NoError(t, err)

	withGoal, err := server.prepareGeneration(ctx, generationPrepareInput{
		Chat:     created.Chat,
		Messages: created.InitialMessages,
	})
	require.NoError(t, err)
	t.Cleanup(withGoal.Cleanup)
	require.Contains(t, withGoal.StopAfterTools, chattool.CompleteGoalToolName)

	// A dynamic tool may not impersonate the recognized marker name; the
	// registered collision is dropped whenever recognition is armed.
	dynamicTools, err := json.Marshal([]codersdk.DynamicTool{{Name: chattool.CompleteGoalToolName}})
	require.NoError(t, err)
	created.Chat.DynamicTools = pqtype.NullRawMessage{RawMessage: dynamicTools, Valid: true}

	// After complete_goal commits its transition mid-turn, the next pass
	// must still recognize the persisted result as a stop marker.
	_, err = db.CompleteChatGoalByID(ctx, database.CompleteChatGoalByIDParams{
		RootChatID:        created.Chat.ID,
		ID:                goal.ID,
		CompletionSummary: sql.NullString{String: "done", Valid: true},
		CompletedByAgent:  true,
	})
	require.NoError(t, err)
	afterComplete, err := server.prepareGeneration(ctx, generationPrepareInput{
		Chat:     created.Chat,
		Messages: created.InitialMessages,
	})
	require.NoError(t, err)
	t.Cleanup(afterComplete.Cleanup)
	require.Contains(t, afterComplete.StopAfterTools, chattool.CompleteGoalToolName)
	require.NotContains(t, afterComplete.DynamicToolNames, chattool.CompleteGoalToolName)

	// A cleared goal arms nothing, and the dynamic name is usable again.
	_, err = db.ClearChatGoalByID(ctx, database.ClearChatGoalByIDParams{
		RootChatID: created.Chat.ID,
		ID:         goal.ID,
	})
	require.NoError(t, err)
	afterClear, err := server.prepareGeneration(ctx, generationPrepareInput{
		Chat:     created.Chat,
		Messages: created.InitialMessages,
	})
	require.NoError(t, err)
	t.Cleanup(afterClear.Cleanup)
	require.NotContains(t, afterClear.StopAfterTools, chattool.CompleteGoalToolName)
	require.Contains(t, afterClear.DynamicToolNames, chattool.CompleteGoalToolName)
}

func TestPrepareGenerationComputerUseIgnoresChatTransportOverride(t *testing.T) {
	t.Parallel()

	db, ps := dbtestutil.NewDB(t)
	ctx := chatdTestContext(t)
	user := dbgen.User(t, db, database.User{})
	org := dbgen.Organization(t, db, database.Organization{})
	dbgen.OrganizationMember(t, db, database.OrganizationMember{
		UserID:         user.ID,
		OrganizationID: org.ID,
	})
	require.NoError(t, db.UpsertChatComputerUseProvider(ctx, string(codersdk.ChatComputerUseProviderOpenAI)))
	provider := dbgen.AIProviderWithOptionalKey(t, db, database.AIProvider{
		Type: database.AIProviderTypeOpenai,
	}, "test-key")
	forceCompletions := false
	modelConfigRaw, err := json.Marshal(codersdk.ChatModelCallConfig{
		OpenAIConfig: &codersdk.ChatModelOpenAIConfig{
			UseResponsesAPI: &forceCompletions,
		},
		ProviderOptions: &codersdk.ChatModelProviderOptions{
			OpenAI: &codersdk.ChatModelOpenAIProviderOptions{
				User: new("computer-use"),
			},
		},
	})
	require.NoError(t, err)
	modelConfig := dbgen.ChatModelConfig(t, db, database.ChatModelConfig{
		Model:          "gpt-4o-mini",
		Options:        modelConfigRaw,
		AIProviderID:   uuid.NullUUID{UUID: provider.ID, Valid: true},
		OrganizationID: org.ID,
	}, func(p *database.InsertChatModelConfigParams) {
		p.Enabled = true
	})

	const attachmentText = "text attachment body"
	file, err := db.InsertChatFile(ctx, database.InsertChatFileParams{
		OwnerID:        user.ID,
		OrganizationID: org.ID,
		Name:           "notes.txt",
		Mimetype:       "text/plain",
		Data:           []byte(attachmentText),
	})
	require.NoError(t, err)
	content, err := chatprompt.MarshalParts([]codersdk.ChatMessagePart{
		codersdk.ChatMessageText("hello"),
		codersdk.ChatMessageFile(file.ID, "text/plain", "notes.txt"),
	})
	require.NoError(t, err)

	created, err := chatstate.CreateChat(ctx, db, ps, chatstate.CreateChatInput{
		OrganizationID:    org.ID,
		OwnerID:           user.ID,
		LastModelConfigID: modelConfig.ID,
		Title:             "computer use transport",
		ClientType:        database.ChatClientTypeApi,
		Mode:              database.NullChatMode{ChatMode: database.ChatModeComputerUse, Valid: true},
		InitialMessages: []chatstate.Message{
			{
				Role:           database.ChatMessageRoleUser,
				Content:        content,
				Visibility:     database.ChatMessageVisibilityBoth,
				ModelConfigID:  uuid.NullUUID{UUID: modelConfig.ID, Valid: true},
				CreatedBy:      uuid.NullUUID{UUID: user.ID, Valid: true},
				ContentVersion: chatprompt.CurrentContentVersion,
			},
		},
	})
	require.NoError(t, err)

	server := newInternalTestServer(
		t,
		db,
		ps,
		chatprovider.ProviderAPIKeys{},
		withInternalTestServerTransportFactory(&aibridgeTestFactory{}),
	)
	prepared, err := server.prepareGeneration(ctx, generationPrepareInput{
		Chat:     created.Chat,
		Messages: created.InitialMessages,
	})
	require.NoError(t, err)
	t.Cleanup(prepared.Cleanup)

	// The computer-use model is Responses-selected by the SDK and its client
	// ignores the config's forced Chat Completions, so the options must be the
	// Responses type or the SDK discards them.
	_, ok := prepared.CallTemplate.ProviderOptions[fantasyopenai.Name].(*fantasyopenai.ResponsesProviderOptions)
	require.True(t, ok, "%T", prepared.CallTemplate.ProviderOptions[fantasyopenai.Name])

	// File classification must also key on the substituted model: the
	// Responses transport drops native text file parts, so the attachment
	// must be inlined as text rather than kept as a FilePart.
	var sawInlinedText bool
	for _, message := range prepared.Prompt {
		for _, part := range message.Content {
			if filePart, isFile := part.(fantasy.FilePart); isFile {
				t.Fatalf("text attachment survived as FilePart %q", filePart.Filename)
			}
			if textPart, isText := part.(fantasy.TextPart); isText &&
				strings.Contains(textPart.Text, attachmentText) {
				sawInlinedText = true
			}
		}
	}
	require.True(t, sawInlinedText, "attachment was not inlined as text")
}

func TestPrepareGenerationSubagentUsesOwnerSyntheticAPIKey(t *testing.T) {
	t.Parallel()

	db, ps := dbtestutil.NewDB(t)
	ctx := chatdTestContext(t)
	user := dbgen.User(t, db, database.User{})
	org := dbgen.Organization(t, db, database.Organization{})
	dbgen.OrganizationMember(t, db, database.OrganizationMember{
		UserID:         user.ID,
		OrganizationID: org.ID,
	})
	provider := dbgen.AIProviderWithOptionalKey(t, db, database.AIProvider{
		Type: database.AIProviderTypeOpenai,
	}, "test-key")
	modelConfig := dbgen.ChatModelConfig(t, db, database.ChatModelConfig{
		Model:          "gpt-4o-mini",
		AIProviderID:   uuid.NullUUID{UUID: provider.ID, Valid: true},
		OrganizationID: org.ID,
	}, func(p *database.InsertChatModelConfigParams) {
		p.Enabled = true
	})
	parent := dbgen.Chat(t, db, database.Chat{
		OrganizationID:    org.ID,
		OwnerID:           user.ID,
		LastModelConfigID: modelConfig.ID,
	})
	created, err := chatstate.CreateChat(ctx, db, ps, chatstate.CreateChatInput{
		OrganizationID:    org.ID,
		OwnerID:           user.ID,
		ParentChatID:      uuid.NullUUID{UUID: parent.ID, Valid: true},
		RootChatID:        uuid.NullUUID{UUID: parent.ID, Valid: true},
		LastModelConfigID: modelConfig.ID,
		Title:             "subagent attribution",
		ClientType:        database.ChatClientTypeApi,
		InitialMessages: []chatstate.Message{
			{
				Role:           database.ChatMessageRoleUser,
				Content:        mustMarshalText(t, "inspect the workspace"),
				Visibility:     database.ChatMessageVisibilityBoth,
				ModelConfigID:  uuid.NullUUID{UUID: modelConfig.ID, Valid: true},
				CreatedBy:      uuid.NullUUID{UUID: user.ID, Valid: true},
				ContentVersion: chatprompt.CurrentContentVersion,
			},
		},
	})
	require.NoError(t, err)

	server := newInternalTestServer(
		t,
		db,
		ps,
		chatprovider.ProviderAPIKeys{},
		withInternalTestServerTransportFactory(&aibridgeTestFactory{}),
	)
	prepared, err := server.prepareGeneration(ctx, generationPrepareInput{
		Chat:     created.Chat,
		Messages: created.InitialMessages,
	})
	require.NoError(t, err)
	t.Cleanup(prepared.Cleanup)

	gatewayKey, err := db.GetChatGatewayAPIKey(ctx, database.GetChatGatewayAPIKeyParams{
		UserID:    user.ID,
		TokenName: GatewayTokenName(user.ID),
	})
	require.NoError(t, err)
	require.Equal(t, gatewayKey.ID, prepared.ModelBuildOptions.ActiveAPIKeyID)
}

// TestDeriveFinalTurnRunResult exercises the re-derivation path that replaces
// the old in-memory generationSideEffects stash. The server here never ran
// prepareGeneration, so a passing test proves the finish-turn inputs are
// rebuilt purely from persisted state.
func TestDeriveFinalTurnRunResult(t *testing.T) {
	t.Parallel()

	logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true})

	setup := func(t *testing.T) (*Server, database.Chat) {
		t.Helper()
		db, ps := dbtestutil.NewDB(t)
		ctx := chatdTestContext(t)

		user := dbgen.User(t, db, database.User{})
		org := dbgen.Organization(t, db, database.Organization{})
		dbgen.OrganizationMember(t, db, database.OrganizationMember{
			UserID:         user.ID,
			OrganizationID: org.ID,
		})
		dbgen.ChatProvider(t, db, database.ChatProvider{
			Provider:    "openai",
			DisplayName: "OpenAI",
			APIKey:      "test-key",
			Enabled:     true,
			CreatedBy:   uuid.NullUUID{UUID: user.ID, Valid: true},
		})
		modelCfg := dbgen.ChatModelConfig(t, db, database.ChatModelConfig{
			Model:          "gpt-4o-mini",
			DisplayName:    "gpt-4o-mini",
			Options:        json.RawMessage(`{"openai_config":{"use_responses_api":false}}`),
			OrganizationID: org.ID,
		}, func(p *database.InsertChatModelConfigParams) {
			p.Enabled = true
			p.IsDefault = true
		})

		created, err := chatstate.CreateChat(ctx, db, ps, chatstate.CreateChatInput{
			OrganizationID:    org.ID,
			OwnerID:           user.ID,
			LastModelConfigID: modelCfg.ID,
			Title:             "derive-chat",
			ClientType:        database.ChatClientTypeUi,
			InitialMessages: []chatstate.Message{
				{
					Role:           database.ChatMessageRoleUser,
					Content:        mustMarshalText(t, "what is the answer?"),
					Visibility:     database.ChatMessageVisibilityBoth,
					ContentVersion: chatprompt.CurrentContentVersion,
					CreatedBy:      uuid.NullUUID{UUID: user.ID, Valid: true},
					ModelConfigID:  uuid.NullUUID{UUID: modelCfg.ID, Valid: true},
				},
			},
		})
		require.NoError(t, err)

		server := newInternalTestServer(
			t, db, ps, chatprovider.ProviderAPIKeys{},
			withInternalTestServerTransportFactory(&aibridgeTestFactory{}),
		)
		return server, created.Chat
	}

	commitAssistant := func(t *testing.T, server *Server, chat database.Chat, text string) {
		t.Helper()
		ctx := chatdTestContext(t)
		machine := chatstate.NewChatMachine(server.db, server.pubsub, chat.ID)
		require.NoError(t, machine.Update(ctx, func(tx *chatstate.Tx, store database.Store) error {
			_, err := tx.CommitStep(chatstate.CommitStepInput{
				Messages: []chatstate.Message{
					{
						Role:           database.ChatMessageRoleAssistant,
						Content:        mustMarshalText(t, text),
						Visibility:     database.ChatMessageVisibilityBoth,
						ContentVersion: chatprompt.CurrentContentVersion,
						ModelConfigID:  uuid.NullUUID{UUID: chat.LastModelConfigID, Valid: true},
					},
				},
			})
			return err
		}))
	}

	t.Run("WaitingDerivesFromHistory", func(t *testing.T) {
		t.Parallel()
		server, chat := setup(t)
		ctx := chatdTestContext(t)
		commitAssistant(t, server, chat, "the answer is 42")

		rows, err := server.db.GetChatMessagesForPromptByChatID(ctx, chat.ID)
		require.NoError(t, err)
		require.NotEmpty(t, rows)
		var lastUserID int64
		for _, row := range rows {
			if row.Role == database.ChatMessageRoleUser {
				lastUserID = row.ID
			}
		}
		tipID := rows[len(rows)-1].ID

		chat.Status = database.ChatStatusWaiting
		result := server.deriveFinalTurnRunResult(ctx, chat, logger)

		require.Equal(t, "the answer is 42", result.FinalAssistantText)
		require.Equal(t, lastUserID, result.TriggerMessageID)
		require.Equal(t, tipID, result.HistoryTipMessageID)
		require.NotNil(t, result.StatusLabelCall)
		require.True(t, result.StatusLabelCall.model.Valid())
		require.Equal(t, "openai", result.StatusLabelCall.resolvedProvider)
		require.Equal(t, "gpt-4o-mini", result.StatusLabelCall.resolvedModel)
		require.JSONEq(t, `{"openai_config":{"use_responses_api":false}}`, string(result.StatusLabelCall.dbConfig.Options))
	})

	t.Run("NonWaitingReturnsEmpty", func(t *testing.T) {
		t.Parallel()
		server, chat := setup(t)
		ctx := chatdTestContext(t)
		commitAssistant(t, server, chat, "the answer is 42")

		chat.Status = database.ChatStatusError
		result := server.deriveFinalTurnRunResult(ctx, chat, logger)
		require.Equal(t, runChatResult{}, result)
	})

	t.Run("WaitingWithoutAssistantReturnsEmpty", func(t *testing.T) {
		t.Parallel()
		server, chat := setup(t)
		ctx := chatdTestContext(t)

		// No assistant message was committed, so there is nothing to label.
		chat.Status = database.ChatStatusWaiting
		result := server.deriveFinalTurnRunResult(ctx, chat, logger)
		require.Equal(t, runChatResult{}, result)
	})

	t.Run("ModelResolveErrorKeepsTextAndIDs", func(t *testing.T) {
		t.Parallel()
		db, ps := dbtestutil.NewDB(t)
		ctx := chatdTestContext(t)

		user := dbgen.User(t, db, database.User{})
		org := dbgen.Organization(t, db, database.Organization{})
		dbgen.OrganizationMember(t, db, database.OrganizationMember{
			UserID:         user.ID,
			OrganizationID: org.ID,
		})
		provider := insertInternalAIProvider(t, db, database.AIProviderTypeOpenai, "provider-api-key", false)
		modelCfg := dbgen.ChatModelConfig(t, db, database.ChatModelConfig{
			Model:          "gpt-4o-mini",
			DisplayName:    "gpt-4o-mini",
			AIProviderID:   uuid.NullUUID{UUID: provider.ID, Valid: true},
			OrganizationID: org.ID,
		})

		created, err := chatstate.CreateChat(ctx, db, ps, chatstate.CreateChatInput{
			OrganizationID:    org.ID,
			OwnerID:           user.ID,
			LastModelConfigID: modelCfg.ID,
			Title:             "derive-chat-error",
			ClientType:        database.ChatClientTypeUi,
			InitialMessages: []chatstate.Message{
				{
					Role:           database.ChatMessageRoleUser,
					Content:        mustMarshalText(t, "what is the answer?"),
					Visibility:     database.ChatMessageVisibilityBoth,
					ContentVersion: chatprompt.CurrentContentVersion,
					CreatedBy:      uuid.NullUUID{UUID: user.ID, Valid: true},
					ModelConfigID:  uuid.NullUUID{UUID: modelCfg.ID, Valid: true},
				},
			},
		})
		require.NoError(t, err)
		chat := created.Chat

		server := newInternalTestServer(t, db, ps, chatprovider.ProviderAPIKeys{})
		commitAssistant(t, server, chat, "the answer is 42")

		chat.Status = database.ChatStatusWaiting
		result := server.deriveFinalTurnRunResult(ctx, chat, logger)

		require.Equal(t, "the answer is 42", result.FinalAssistantText)
		require.NotZero(t, result.TriggerMessageID)
		require.NotZero(t, result.HistoryTipMessageID)
		require.Nil(t, result.StatusLabelCall)
	})
}

// TestLatestPromptUsage verifies the compaction path reads the last
// persisted assistant usage, not a cumulative sum across steps.
// This is the chatd side of AIGOV-585: the issue blamed chatd for
// summing usage across steps, but latestPromptUsage returns the
// single most-recent non-zero usage. The real bug was in the
// aibridge streaming interceptor (fixed in ad100452d4).
func TestLatestPromptUsage(t *testing.T) {
	t.Parallel()

	t.Run("returns last assistant step not a sum", func(t *testing.T) {
		t.Parallel()
		// Three-step agentic turn: each step re-sends the whole
		// conversation, so InputTokens are 5000, 5200, 5400.
		// A sum would be 15600; the correct answer is 5400.
		messages := []database.ChatMessage{
			dbMessage(t, 1, database.ChatMessageRoleUser, false, codersdk.ChatMessageText("hi")),
			withUsage(dbMessage(t, 2, database.ChatMessageRoleAssistant, false, codersdk.ChatMessageText("step1")), 5000, 0),
			dbMessage(t, 3, database.ChatMessageRoleTool, false, codersdk.ChatMessageText("result")),
			withUsage(dbMessage(t, 4, database.ChatMessageRoleAssistant, false, codersdk.ChatMessageText("step2")), 5200, 0),
			dbMessage(t, 5, database.ChatMessageRoleTool, false, codersdk.ChatMessageText("result")),
			withUsage(dbMessage(t, 6, database.ChatMessageRoleAssistant, false, codersdk.ChatMessageText("step3")), 5400, 0),
		}
		usage := latestPromptUsage(messages)
		assert.Equal(t, int64(5400), usage.InputTokens,
			"must return the last step's usage, not the sum across steps")
	})

	t.Run("returns zero when no messages have usage", func(t *testing.T) {
		t.Parallel()
		messages := []database.ChatMessage{
			dbMessage(t, 1, database.ChatMessageRoleUser, false, codersdk.ChatMessageText("hi")),
			dbMessage(t, 2, database.ChatMessageRoleAssistant, false, codersdk.ChatMessageText("hi")),
		}
		assert.Equal(t, fantasy.Usage{}, latestPromptUsage(messages))
	})

	t.Run("returns zero for empty message list", func(t *testing.T) {
		t.Parallel()
		assert.Equal(t, fantasy.Usage{}, latestPromptUsage(nil))
	})
}

// TestShouldCompactPromptUsage verifies the compaction threshold decision
// is correct for both the inflated values the aibridge bug produced and
// accurate per-step values.
func TestShouldCompactPromptUsage(t *testing.T) {
	t.Parallel()

	const contextLimit = int64(262144) // 256K, as in the poolside report

	t.Run("inflated cumulative usage triggers compaction", func(t *testing.T) {
		t.Parallel()
		// 417,012 tokens: what the aibridge cross-chunk sum bug
		// produced for a ~6,000-token conversation.
		assert.True(t, shouldCompactPromptUsage(
			fantasy.Usage{InputTokens: 417012, TotalTokens: 418846},
			contextLimit, 80))
	})

	t.Run("correct per-step usage does not trigger", func(t *testing.T) {
		t.Parallel()
		assert.False(t, shouldCompactPromptUsage(
			fantasy.Usage{InputTokens: 6000, TotalTokens: 6030},
			contextLimit, 80))
	})

	t.Run("threshold 100 disables compaction", func(t *testing.T) {
		t.Parallel()
		assert.False(t, shouldCompactPromptUsage(
			fantasy.Usage{InputTokens: 500000}, contextLimit, 100))
	})

	t.Run("zero context limit disables compaction", func(t *testing.T) {
		t.Parallel()
		assert.False(t, shouldCompactPromptUsage(
			fantasy.Usage{InputTokens: 6000}, 0, 80))
	})

	t.Run("counts cache read and creation tokens", func(t *testing.T) {
		t.Parallel()
		usage := fantasy.Usage{
			InputTokens:         6000,
			CacheReadTokens:     200000,
			CacheCreationTokens: 5000,
		}
		// 211,000 / 262,144 = ~80.5%
		assert.True(t, shouldCompactPromptUsage(usage, contextLimit, 80))
	})

	t.Run("falls back to TotalTokens when granular fields are missing", func(t *testing.T) {
		t.Parallel()
		assert.True(t, shouldCompactPromptUsage(
			fantasy.Usage{TotalTokens: 211000},
			contextLimit, 80))
	})
}

func TestEnabledMCPServerConfigsForChatOrg(t *testing.T) {
	t.Parallel()

	newOrgWithConfig := func(t *testing.T, db database.Store) (database.Organization, database.MCPServerConfig) {
		t.Helper()
		org := dbgen.Organization(t, db, database.Organization{})
		cfg := dbgen.MCPServerConfig(t, db, database.MCPServerConfig{
			OrganizationID: org.ID,
			Enabled:        true,
		})
		return org, cfg
	}

	t.Run("DefaultOrgConfigExcluded", func(t *testing.T) {
		t.Parallel()
		db, _ := dbtestutil.NewDB(t)
		ctx := testutil.Context(t, testutil.WaitShort)

		defaultOrg, err := db.GetDefaultOrganization(ctx)
		require.NoError(t, err)

		// Configs resolve only within the chat's organization.
		chatOrg, chatOrgCfg := newOrgWithConfig(t, db)
		defaultOrgCfg := dbgen.MCPServerConfig(t, db, database.MCPServerConfig{
			OrganizationID: defaultOrg.ID,
			Enabled:        true,
		})

		configs, err := enabledMCPServerConfigsForChatOrg(ctx, db, chatOrg.ID, []uuid.UUID{chatOrgCfg.ID, defaultOrgCfg.ID})
		require.NoError(t, err)
		require.Len(t, configs, 1)
		require.Equal(t, chatOrgCfg.ID, configs[0].ID)
	})

	t.Run("ThirdOrgConfigExcluded", func(t *testing.T) {
		t.Parallel()
		db, _ := dbtestutil.NewDB(t)
		ctx := testutil.Context(t, testutil.WaitShort)

		chatOrg, chatOrgCfg := newOrgWithConfig(t, db)
		_, foreignCfg := newOrgWithConfig(t, db)

		configs, err := enabledMCPServerConfigsForChatOrg(ctx, db, chatOrg.ID, []uuid.UUID{chatOrgCfg.ID, foreignCfg.ID})
		require.NoError(t, err)
		require.Len(t, configs, 1)
		require.Equal(t, chatOrgCfg.ID, configs[0].ID)
	})

	t.Run("DisabledConfigExcluded", func(t *testing.T) {
		t.Parallel()
		db, _ := dbtestutil.NewDB(t)
		ctx := testutil.Context(t, testutil.WaitShort)

		// dbgen.MCPServerConfig defaults Enabled to true, so insert the
		// disabled config directly.
		chatOrg := dbgen.Organization(t, db, database.Organization{})
		user := dbgen.User(t, db, database.User{})
		disabledCfg, err := db.InsertMCPServerConfig(ctx, database.InsertMCPServerConfigParams{
			ID:             uuid.New(),
			OrganizationID: chatOrg.ID,
			DisplayName:    "Disabled MCP Server",
			Slug:           testutil.GetRandomName(t),
			Url:            "https://mcp.example.com",
			Transport:      "streamable_http",
			AuthType:       "none",
			ToolAllowList:  []string{},
			ToolDenyList:   []string{},
			Availability:   "default_off",
			Enabled:        false,
			CreatedBy:      user.ID,
			UpdatedBy:      user.ID,
		})
		require.NoError(t, err)

		configs, err := enabledMCPServerConfigsForChatOrg(ctx, db, chatOrg.ID, []uuid.UUID{disabledCfg.ID})
		require.NoError(t, err)
		require.Empty(t, configs)
	})

	t.Run("DuplicateIDsYieldOneConfigPerUniqueID", func(t *testing.T) {
		t.Parallel()
		db, _ := dbtestutil.NewDB(t)
		ctx := testutil.Context(t, testutil.WaitShort)

		// The ID array may contain duplicates, but the query returns one row
		// per unique ID ordered by display_name.
		chatOrg, cfgA := newOrgWithConfig(t, db)
		cfgB := dbgen.MCPServerConfig(t, db, database.MCPServerConfig{
			OrganizationID: chatOrg.ID,
			Enabled:        true,
		})

		// The requested order is the reverse of display_name order to
		// prove the output ordering comes from the SQL, not the request.
		requested := []uuid.UUID{cfgB.ID, cfgA.ID, cfgB.ID, cfgA.ID}

		configs, err := enabledMCPServerConfigsForChatOrg(ctx, db, chatOrg.ID, requested)
		require.NoError(t, err)
		require.Len(t, configs, 2)
		gotIDs := []uuid.UUID{configs[0].ID, configs[1].ID}
		wantOrder := []uuid.UUID{cfgA.ID, cfgB.ID}
		if cfgA.DisplayName > cfgB.DisplayName {
			wantOrder = []uuid.UUID{cfgB.ID, cfgA.ID}
		}
		require.Equal(t, wantOrder, gotIDs, "output must follow display_name order, not request order")
	})

	t.Run("ChatOrgWithNoConfigs", func(t *testing.T) {
		t.Parallel()
		db, _ := dbtestutil.NewDB(t)
		ctx := testutil.Context(t, testutil.WaitShort)

		defaultOrg, err := db.GetDefaultOrganization(ctx)
		require.NoError(t, err)

		chatOrg := dbgen.Organization(t, db, database.Organization{})
		defaultOrgCfg := dbgen.MCPServerConfig(t, db, database.MCPServerConfig{
			OrganizationID: defaultOrg.ID,
			Enabled:        true,
		})

		configs, err := enabledMCPServerConfigsForChatOrg(ctx, db, chatOrg.ID, []uuid.UUID{defaultOrgCfg.ID})
		require.NoError(t, err)
		require.Empty(t, configs)
	})
}
