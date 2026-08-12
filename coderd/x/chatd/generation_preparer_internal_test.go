package chatd //nolint:testpackage // Exercises unexported re-derivation helpers.

import (
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
	"github.com/coder/coder/v2/coderd/util/ptr"
	"github.com/coder/coder/v2/coderd/x/chatd/chatprompt"
	"github.com/coder/coder/v2/coderd/x/chatd/chatprovider"
	"github.com/coder/coder/v2/coderd/x/chatd/chatstate"
	"github.com/coder/coder/v2/codersdk"
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
		ReasoningEffort: &codersdk.ChatModelReasoningEffortConfig{
			Default: ptr.Ref(codersdk.ChatModelReasoningEffortLow),
			Max:     ptr.Ref(codersdk.ChatModelReasoningEffortMedium),
		},
	})
	require.NoError(t, err)
	modelConfig := dbgen.ChatModelConfig(t, db, database.ChatModelConfig{
		Model:        "gpt-4o-mini",
		Options:      modelConfigRaw,
		AIProviderID: uuid.NullUUID{UUID: provider.ID, Valid: true},
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

	providerOptions, ok := prepared.ProviderOptions[fantasyopenai.Name].(*fantasyopenai.ResponsesProviderOptions)
	require.True(t, ok, "%T", prepared.ProviderOptions[fantasyopenai.Name])
	require.NotNil(t, providerOptions.ReasoningEffort)
	require.Equal(t, fantasyopenai.ReasoningEffortMedium, *providerOptions.ReasoningEffort)
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
				User: ptr.Ref("computer-use"),
			},
		},
	})
	require.NoError(t, err)
	modelConfig := dbgen.ChatModelConfig(t, db, database.ChatModelConfig{
		Model:        "gpt-4o-mini",
		Options:      modelConfigRaw,
		AIProviderID: uuid.NullUUID{UUID: provider.ID, Valid: true},
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

	// The computer-use model (gpt-5.5) is Responses-selected by the SDK and
	// its client ignores the config's forced Chat Completions, so the
	// options must be the Responses type or the SDK discards them.
	_, ok := prepared.ProviderOptions[fantasyopenai.Name].(*fantasyopenai.ResponsesProviderOptions)
	require.True(t, ok, "%T", prepared.ProviderOptions[fantasyopenai.Name])

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
		Model:        "gpt-4o-mini",
		AIProviderID: uuid.NullUUID{UUID: provider.ID, Valid: true},
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
			Model:       "gpt-4o-mini",
			DisplayName: "gpt-4o-mini",
			Options:     json.RawMessage(`{"openai_config":{"use_responses_api":false}}`),
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
		require.True(t, result.StatusLabelModel.Valid())
		require.Equal(t, "openai", result.FallbackProvider)
		require.Equal(t, "gpt-4o-mini", result.FallbackModel)
		require.JSONEq(t, `{"openai_config":{"use_responses_api":false}}`, string(result.StatusLabelOptions))
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
		// A disabled AI provider makes resolveChatModel fail, exercising the
		// degraded path that still returns the re-derived text and IDs.
		provider := insertInternalAIProvider(t, db, database.AIProviderTypeOpenai, "provider-api-key", false)
		modelCfg := dbgen.ChatModelConfig(t, db, database.ChatModelConfig{
			Model:        "gpt-4o-mini",
			DisplayName:  "gpt-4o-mini",
			AIProviderID: uuid.NullUUID{UUID: provider.ID, Valid: true},
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
		require.False(t, result.StatusLabelModel.Valid())
		require.Empty(t, result.FallbackProvider)
		require.Empty(t, result.FallbackModel)
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
}
