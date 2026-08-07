package chatd //nolint:testpackage // Exercises unexported re-derivation helpers.

import (
	"encoding/json"
	"strings"
	"testing"

	"charm.land/fantasy"
	fantasyopenai "charm.land/fantasy/providers/openai"
	"github.com/google/uuid"
	"github.com/sqlc-dev/pqtype"
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

func TestEnabledMCPServerConfigsForChatOrg(t *testing.T) {
	t.Parallel()

	newOrgWithConfig := func(t *testing.T, db database.Store, enabled bool) (database.Organization, database.MCPServerConfig) {
		t.Helper()
		org := dbgen.Organization(t, db, database.Organization{})
		cfg := dbgen.MCPServerConfig(t, db, database.MCPServerConfig{
			OrganizationID: org.ID,
			Enabled:        enabled,
		})
		return org, cfg
	}

	t.Run("DefaultOrgConfigExcluded", func(t *testing.T) {
		t.Parallel()
		db, _ := dbtestutil.NewDB(t)
		ctx := testutil.Context(t, testutil.WaitShort)

		defaultOrg, err := db.GetDefaultOrganization(ctx)
		require.NoError(t, err)

		// Configs resolve strictly against the chat's organization; the
		// default organization gets no special treatment.
		chatOrg, chatOrgCfg := newOrgWithConfig(t, db, true)
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

		chatOrg, chatOrgCfg := newOrgWithConfig(t, db, true)
		_, foreignCfg := newOrgWithConfig(t, db, true)

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

		// The array has no uniqueness constraint. Preserve the legacy SQL shape:
		// one row per unique ID, ordered by display_name.
		chatOrg, cfgA := newOrgWithConfig(t, db, true)
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
		require.ElementsMatch(t, []uuid.UUID{cfgA.ID, cfgB.ID}, gotIDs)
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

		// A chat whose organization has no configs resolves nothing,
		// even when the requested ID exists in the default organization.
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
