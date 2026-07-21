package chatd //nolint:testpackage // Exercises historian policy boundaries.

import (
	"context"
	"database/sql"
	"encoding/json"
	"testing"
	"time"

	"charm.land/fantasy"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/database/dbgen"
	"github.com/coder/coder/v2/coderd/database/dbmock"
	"github.com/coder/coder/v2/coderd/database/dbtestutil"
	"github.com/coder/coder/v2/coderd/x/chatd/chatprompt"
	"github.com/coder/coder/v2/coderd/x/chatd/chatprovider"
	"github.com/coder/coder/v2/coderd/x/chatd/chatstate"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/testutil"
	"github.com/coder/quartz"
)

func historianMessage(
	t *testing.T,
	id int64,
	role database.ChatMessageRole,
	parts ...codersdk.ChatMessagePart,
) database.ChatMessage {
	t.Helper()
	content, err := chatprompt.MarshalParts(parts)
	require.NoError(t, err)
	return database.ChatMessage{
		ID:             id,
		Role:           role,
		Content:        content,
		ContentVersion: chatprompt.CurrentContentVersion,
	}
}

func TestBuildHistorianTranscript(t *testing.T) {
	t.Parallel()

	dispatchTime := time.Date(2026, time.July, 21, 23, 59, 0, 0, time.FixedZone("offset", -7*60*60))
	messages := []database.ChatMessage{
		historianMessage(t, 1, database.ChatMessageRoleUser,
			codersdk.ChatMessageText("user text"),
			codersdk.ChatMessageReasoning("private reasoning"),
		),
		historianMessage(t, 2, database.ChatMessageRoleAssistant,
			codersdk.ChatMessageText("pure assistant"),
		),
		historianMessage(t, 3, database.ChatMessageRoleAssistant,
			codersdk.ChatMessageText("must be excluded"),
			codersdk.ChatMessageToolCall("call-1", "execute", json.RawMessage(`{"command":"secret"}`)),
		),
		historianMessage(t, 4, database.ChatMessageRoleAssistant,
			codersdk.ChatMessageText("tool-calling assistant text must be excluded"),
			codersdk.ChatMessageToolCall("call-2", slackSendMessageToolName, json.RawMessage(`{"text":"agent reply","actions":[{"text":"Open","url":"https://example.com"}]}`)),
		),
		historianMessage(t, 5, database.ChatMessageRoleUser,
			codersdk.ChatMessageText("quote: \" and newline\nnext"),
		),
	}

	payload, ok, err := buildHistorianTranscript(messages, dispatchTime)
	require.NoError(t, err)
	require.True(t, ok)
	require.JSONEq(t, `{
		"daily_memory_path":"/daily/2026-07-22.md",
		"messages":[
			{"role":"user","text":"user text"},
			{"role":"assistant","text":"pure assistant"},
			{"role":"assistant","text":"{\"text\":\"agent reply\",\"actions\":[{\"text\":\"Open\",\"url\":\"https://example.com\"}]}"},
			{"role":"user","text":"quote: \" and newline\nnext"}
		]
	}`, string(payload))
	require.NotContains(t, string(payload), "private reasoning")
	require.NotContains(t, string(payload), "must be excluded")
	require.NotContains(t, string(payload), "secret")
}

func TestBuildHistorianTranscriptEmpty(t *testing.T) {
	t.Parallel()

	payload, ok, err := buildHistorianTranscript([]database.ChatMessage{
		historianMessage(t, 1, database.ChatMessageRoleAssistant,
			codersdk.ChatMessageToolCall("call-1", "read_memory", json.RawMessage(`{}`)),
		),
	}, time.Now())
	require.NoError(t, err)
	require.False(t, ok)
	require.Nil(t, payload)
}

func TestHistorianLoopCadence(t *testing.T) {
	t.Parallel()

	clock := quartz.NewMock(t)
	tickerTrap := clock.Trap().NewTicker("chatworker", "historian")
	defer tickerTrap.Close()
	store := dbmock.NewMockStore(gomock.NewController(t))
	passCh := make(chan struct{}, 4)
	store.EXPECT().GetChatHistorianClaims(gomock.Any()).DoAndReturn(func(context.Context) ([]database.GetChatHistorianClaimsRow, error) {
		passCh <- struct{}{}
		return nil, nil
	}).AnyTimes()
	store.EXPECT().GetChatHistorianCandidates(gomock.Any(), database.GetChatHistorianCandidatesParams{
		LimitCount:  defaultHistorianBatchSize,
		IdleSeconds: int32(defaultHistorianIdleThreshold / time.Second),
	}).Return(nil, nil).AnyTimes()

	worker := &chatWorker{opts: chatWorkerOptions{
		Store:                  store,
		Logger:                 testutil.Logger(t),
		Clock:                  clock,
		HistorianInterval:      defaultHistorianInterval,
		HistorianIdleThreshold: defaultHistorianIdleThreshold,
		HistorianBatchSize:     defaultHistorianBatchSize,
	}}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		defer close(done)
		worker.historianLoop(ctx)
	}()

	waitCtx := testutil.Context(t, testutil.WaitLong)
	select {
	case <-passCh:
	case <-waitCtx.Done():
		t.Fatal("historian loop did not run immediately")
	}
	tickerTrap.MustWait(waitCtx).MustRelease(waitCtx)
	clock.Advance(9 * time.Second).MustWait(waitCtx)
	select {
	case <-passCh:
		t.Fatal("historian loop ran before the 10 second interval")
	default:
	}
	clock.Advance(time.Second).MustWait(waitCtx)
	select {
	case <-passCh:
	case <-waitCtx.Done():
		t.Fatal("historian loop did not run after 10 seconds")
	}
	cancel()
	select {
	case <-done:
	case <-waitCtx.Done():
		t.Fatal("historian loop did not stop")
	}
}

func TestHistorianIsNotSpawnable(t *testing.T) {
	t.Parallel()

	_, ok := lookupSubagentDefinition("historian")
	require.False(t, ok)
	for _, definition := range allSubagentDefinitions() {
		require.NotEqual(t, "historian", definition.id)
	}
	require.Equal(t, "historian", subagentTypeFromChat(database.Chat{
		Mode: database.NullChatMode{ChatMode: database.ChatModeHistorian, Valid: true},
	}))
	_, err := resolveSubagentDefinition(context.Background(), nil, database.Chat{
		PlanMode: database.NullChatPlanMode{ChatPlanMode: database.ChatPlanModePlan, Valid: true},
	}, "historian")
	require.Error(t, err)
	require.ErrorContains(t, err, "type must be one of: general, explore")
	require.NotContains(t, defaultSystemPromptPlanningGuidance, "historian")
}

func TestPrepareHistorianGenerationToolBoundary(t *testing.T) {
	t.Parallel()

	db, ps := dbtestutil.NewDB(t)
	ctx := chatdTestContext(t)
	user := dbgen.User(t, db, database.User{})
	apiKey, _ := dbgen.APIKey(t, db, database.APIKey{UserID: user.ID})
	org := dbgen.Organization(t, db, database.Organization{})
	dbgen.OrganizationMember(t, db, database.OrganizationMember{UserID: user.ID, OrganizationID: org.ID})
	provider := dbgen.AIProviderWithOptionalKey(t, db, database.AIProvider{Type: database.AIProviderTypeOpenai}, "test-key")
	modelConfig := dbgen.ChatModelConfig(t, db, database.ChatModelConfig{
		Model:        "gpt-4o-mini",
		AIProviderID: uuid.NullUUID{UUID: provider.ID, Valid: true},
	}, func(params *database.InsertChatModelConfigParams) {
		params.Enabled = true
	})
	message := func(text string) chatstate.Message {
		return chatstate.Message{
			Role:           database.ChatMessageRoleUser,
			Content:        mustMarshalText(t, text),
			Visibility:     database.ChatMessageVisibilityBoth,
			ModelConfigID:  uuid.NullUUID{UUID: modelConfig.ID, Valid: true},
			CreatedBy:      uuid.NullUUID{UUID: user.ID, Valid: true},
			ContentVersion: chatprompt.CurrentContentVersion,
			APIKeyID:       sql.NullString{String: apiKey.ID, Valid: true},
		}
	}
	rootResult, err := chatstate.CreateChat(ctx, db, ps, chatstate.CreateChatInput{
		OrganizationID:    org.ID,
		OwnerID:           user.ID,
		LastModelConfigID: modelConfig.ID,
		Title:             "root",
		ClientType:        database.ChatClientTypeApi,
		InitialMessages:   []chatstate.Message{message("source")},
	})
	require.NoError(t, err)
	childResult, err := chatstate.CreateChat(ctx, db, ps, chatstate.CreateChatInput{
		OrganizationID:    org.ID,
		OwnerID:           user.ID,
		ParentChatID:      uuid.NullUUID{UUID: rootResult.Chat.ID, Valid: true},
		RootChatID:        uuid.NullUUID{UUID: rootResult.Chat.ID, Valid: true},
		LastModelConfigID: modelConfig.ID,
		Title:             "Historian",
		Mode:              database.NullChatMode{ChatMode: database.ChatModeHistorian, Valid: true},
		ClientType:        database.ChatClientTypeApi,
		InitialMessages:   []chatstate.Message{message(`{"daily_memory_path":"/daily/2026-07-21.md","messages":[]}`)},
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
		Chat:     childResult.Chat,
		Messages: childResult.InitialMessages,
	})
	require.NoError(t, err)
	t.Cleanup(prepared.Cleanup)

	require.Equal(t, []string{
		"read_memory",
		"search_memories",
		"list_memories",
		"write_memory",
		"edit_memory",
	}, prepared.ActiveTools)
	require.Empty(t, prepared.ProviderTools)
	require.Empty(t, prepared.DynamicToolNames)
	require.Len(t, prepared.Tools, 5)
	require.NotEmpty(t, prepared.Prompt)
	require.Equal(t, fantasy.MessageRoleSystem, prepared.Prompt[0].Role)
	systemPart, ok := prepared.Prompt[0].Content[0].(fantasy.TextPart)
	require.True(t, ok)
	require.Equal(t, historianSystemPrompt, systemPart.Text)
	require.NotContains(t, systemPart.Text, DefaultSystemPrompt)

	require.NoError(t, db.UpdateChatACLByID(ctx, database.UpdateChatACLByIDParams{
		ID:       rootResult.Chat.ID,
		UserACL:  database.ChatACL{uuid.NewString(): {}},
		GroupACL: database.ChatACL{},
	}))
	_, err = server.prepareGeneration(ctx, generationPrepareInput{
		Chat:     childResult.Chat,
		Messages: childResult.InitialMessages,
	})
	require.Error(t, err)
	require.True(t, isTerminalGeneration(err))
	require.ErrorContains(t, err, "not eligible for memory access")
}

func TestHistorianLifecycle(t *testing.T) {
	t.Parallel()

	db, ps, sqlDB := dbtestutil.NewDBWithSQLDB(t)
	ctx := chatdTestContext(t)
	user := dbgen.User(t, db, database.User{})
	apiKey, _ := dbgen.APIKey(t, db, database.APIKey{UserID: user.ID})
	org := dbgen.Organization(t, db, database.Organization{})
	dbgen.OrganizationMember(t, db, database.OrganizationMember{UserID: user.ID, OrganizationID: org.ID})
	provider := dbgen.AIProviderWithOptionalKey(t, db, database.AIProvider{Type: database.AIProviderTypeOpenai}, "test-key")
	modelConfig := dbgen.ChatModelConfig(t, db, database.ChatModelConfig{
		Model:        "gpt-4o-mini",
		AIProviderID: uuid.NullUUID{UUID: provider.ID, Valid: true},
	}, func(params *database.InsertChatModelConfigParams) {
		params.Enabled = true
	})
	historianModelConfig := dbgen.ChatModelConfig(t, db, database.ChatModelConfig{
		Model:        "gpt-4.1-mini",
		AIProviderID: uuid.NullUUID{UUID: provider.ID, Valid: true},
	}, func(params *database.InsertChatModelConfigParams) {
		params.Enabled = true
	})
	require.NoError(t, db.UpsertChatHistorianModelOverride(
		dbauthz.AsSystemRestricted(ctx),
		historianModelConfig.ID.String(),
	))
	rootResult, err := chatstate.CreateChat(ctx, db, ps, chatstate.CreateChatInput{
		OrganizationID:    org.ID,
		OwnerID:           user.ID,
		LastModelConfigID: modelConfig.ID,
		Title:             "root",
		ClientType:        database.ChatClientTypeApi,
		InitialMessages: []chatstate.Message{{
			Role:           database.ChatMessageRoleUser,
			Content:        mustMarshalText(t, "remember that I prefer concise answers"),
			Visibility:     database.ChatMessageVisibilityBoth,
			ModelConfigID:  uuid.NullUUID{UUID: modelConfig.ID, Valid: true},
			CreatedBy:      uuid.NullUUID{UUID: user.ID, Valid: true},
			ContentVersion: chatprompt.CurrentContentVersion,
			APIKeyID:       sql.NullString{String: apiKey.ID, Valid: true},
		}},
	})
	require.NoError(t, err)
	rootID := rootResult.Chat.ID
	candidateIDs := func() []uuid.UUID {
		rows, queryErr := db.GetChatHistorianCandidates(ctx, database.GetChatHistorianCandidatesParams{
			LimitCount:  100,
			IdleSeconds: 60,
		})
		require.NoError(t, queryErr)
		ids := make([]uuid.UUID, 0, len(rows))
		for _, row := range rows {
			ids = append(ids, row.ID)
		}
		return ids
	}
	_, err = sqlDB.ExecContext(ctx, `
		UPDATE chats
		SET status = 'waiting', updated_at = NOW() - INTERVAL '2 minutes'
		WHERE id = $1
	`, rootID)
	require.NoError(t, err)
	require.Contains(t, candidateIDs(), rootID)

	_, err = sqlDB.ExecContext(ctx, `UPDATE chats SET status = 'running' WHERE id = $1`, rootID)
	require.NoError(t, err)
	require.NotContains(t, candidateIDs(), rootID)
	_, err = sqlDB.ExecContext(ctx, `UPDATE chats SET status = 'error', updated_at = NOW() - INTERVAL '2 minutes' WHERE id = $1`, rootID)
	require.NoError(t, err)
	require.Contains(t, candidateIDs(), rootID)
	_, err = sqlDB.ExecContext(ctx, `UPDATE chats SET updated_at = NOW() - INTERVAL '59 seconds' WHERE id = $1`, rootID)
	require.NoError(t, err)
	require.NotContains(t, candidateIDs(), rootID)
	_, err = sqlDB.ExecContext(ctx, `UPDATE chats SET archived = true, updated_at = NOW() - INTERVAL '2 minutes' WHERE id = $1`, rootID)
	require.NoError(t, err)
	require.NotContains(t, candidateIDs(), rootID)
	_, err = sqlDB.ExecContext(ctx, `UPDATE chats SET archived = false, labels = '{"slack_shared":"true"}'::jsonb WHERE id = $1`, rootID)
	require.NoError(t, err)
	require.NotContains(t, candidateIDs(), rootID)
	_, err = sqlDB.ExecContext(ctx, `UPDATE chats SET labels = '{}'::jsonb, group_acl = '{"group":{}}'::jsonb WHERE id = $1`, rootID)
	require.NoError(t, err)
	require.NotContains(t, candidateIDs(), rootID)
	_, err = sqlDB.ExecContext(ctx, `UPDATE chats SET group_acl = '{}'::jsonb, user_acl = jsonb_build_object($2::text, '{}'::jsonb) WHERE id = $1`, rootID, uuid.New())
	require.NoError(t, err)
	require.NotContains(t, candidateIDs(), rootID)
	_, err = sqlDB.ExecContext(ctx, `UPDATE chats SET user_acl = '{}'::jsonb, status = 'waiting', updated_at = NOW() - INTERVAL '2 minutes' WHERE id = $1`, rootID)
	require.NoError(t, err)
	require.Contains(t, candidateIDs(), rootID)
	_, err = sqlDB.ExecContext(ctx, `UPDATE chat_messages SET created_at = NOW() - INTERVAL '24 hours 1 minute' WHERE chat_id = $1`, rootID)
	require.NoError(t, err)
	require.NotContains(t, candidateIDs(), rootID)
	_, err = sqlDB.ExecContext(ctx, `
		UPDATE chat_messages
		SET created_at = NOW() - INTERVAL '23 hours 59 minutes',
			role = 'system', visibility = 'model', deleted = true
		WHERE chat_id = $1
	`, rootID)
	require.NoError(t, err)
	require.Contains(t, candidateIDs(), rootID)
	_, err = sqlDB.ExecContext(ctx, `
		UPDATE chat_messages
		SET created_at = NOW(), role = 'user', visibility = 'both', deleted = false
		WHERE chat_id = $1
	`, rootID)
	require.NoError(t, err)

	server := newInternalTestServer(
		t,
		db,
		ps,
		chatprovider.ProviderAPIKeys{},
		withInternalTestServerTransportFactory(&aibridgeTestFactory{}),
	)
	worker, err := newChatWorker(server, chatWorkerOptions{
		WorkerID:          uuid.New(),
		Store:             db,
		Pubsub:            ps,
		Clock:             quartz.NewMock(t),
		TaskStarter:       newRecordingTaskStarter(),
		HistorianInterval: time.Hour,
	})
	require.NoError(t, err)

	staleClaimID := uuid.New()
	_, err = db.ClaimChatHistorianHistory(ctx, database.ClaimChatHistorianHistoryParams{
		RootChatID:               rootID,
		ProcessingHistoryVersion: rootResult.Chat.HistoryVersion,
		ProcessingStartedAt:      time.Now(),
		DispatchID:               staleClaimID,
	})
	require.NoError(t, err)
	_, err = sqlDB.ExecContext(ctx, `UPDATE chat_messages SET created_at = NOW() - INTERVAL '24 hours 1 minute' WHERE chat_id = $1`, rootID)
	require.NoError(t, err)
	worker.historianOnce(ctx)
	claims, err := db.GetChatHistorianClaims(ctx)
	require.NoError(t, err)
	require.Empty(t, claims)
	_, err = sqlDB.ExecContext(ctx, `UPDATE chat_messages SET created_at = NOW() WHERE chat_id = $1`, rootID)
	require.NoError(t, err)

	worker.historianOnce(ctx)
	claims, err = db.GetChatHistorianClaims(ctx)
	require.NoError(t, err)
	require.Len(t, claims, 1)
	require.True(t, claims[0].DispatchedAt.Valid)
	require.True(t, claims[0].HistorianChatID.Valid)
	firstChildID := claims[0].HistorianChatID.UUID
	firstDispatchID := claims[0].DispatchID.UUID
	child, err := db.GetChatByID(ctx, firstChildID)
	require.NoError(t, err)
	require.Equal(t, database.ChatModeHistorian, child.Mode.ChatMode)
	require.False(t, child.WorkspaceID.Valid)
	require.Empty(t, child.MCPServerIDs)
	require.False(t, child.PlanMode.Valid)
	require.Equal(t, rootID, child.ParentChatID.UUID)
	require.Equal(t, rootID, child.RootChatID.UUID)
	require.Equal(t, historianModelConfig.ID, child.LastModelConfigID)
	childMessages, err := db.GetChatMessagesByChatID(ctx, database.GetChatMessagesByChatIDParams{ChatID: firstChildID})
	require.NoError(t, err)
	require.Len(t, childMessages, 1)
	_, err = sqlDB.ExecContext(ctx, `UPDATE chat_historian_states SET dispatched_at = NULL WHERE root_chat_id = $1`, rootID)
	require.NoError(t, err)
	worker.historianOnce(ctx)
	childMessages, err = db.GetChatMessagesByChatID(ctx, database.GetChatMessagesByChatIDParams{ChatID: firstChildID})
	require.NoError(t, err)
	require.Len(t, childMessages, 1, "interrupted dispatch recovery must not duplicate the message")
	claims, err = db.GetChatHistorianClaims(ctx)
	require.NoError(t, err)
	require.True(t, claims[0].DispatchedAt.Valid)

	createEligibleRoot := func(ownerID uuid.UUID, ownerAPIKeyID string, title string) uuid.UUID {
		result, createErr := chatstate.CreateChat(ctx, db, ps, chatstate.CreateChatInput{
			OrganizationID:    org.ID,
			OwnerID:           ownerID,
			LastModelConfigID: modelConfig.ID,
			Title:             title,
			ClientType:        database.ChatClientTypeApi,
			InitialMessages: []chatstate.Message{{
				Role:           database.ChatMessageRoleUser,
				Content:        mustMarshalText(t, "durable preference"),
				Visibility:     database.ChatMessageVisibilityBoth,
				ModelConfigID:  uuid.NullUUID{UUID: modelConfig.ID, Valid: true},
				CreatedBy:      uuid.NullUUID{UUID: ownerID, Valid: true},
				ContentVersion: chatprompt.CurrentContentVersion,
				APIKeyID:       sql.NullString{String: ownerAPIKeyID, Valid: true},
			}},
		})
		require.NoError(t, createErr)
		_, createErr = sqlDB.ExecContext(ctx, `
			UPDATE chats
			SET status = 'waiting', updated_at = NOW() - INTERVAL '2 minutes'
			WHERE id = $1
		`, result.Chat.ID)
		require.NoError(t, createErr)
		return result.Chat.ID
	}
	sameOwnerRootID := createEligibleRoot(user.ID, apiKey.ID, "same owner root")
	otherUser := dbgen.User(t, db, database.User{})
	otherAPIKey, _ := dbgen.APIKey(t, db, database.APIKey{UserID: otherUser.ID})
	dbgen.OrganizationMember(t, db, database.OrganizationMember{UserID: otherUser.ID, OrganizationID: org.ID})
	otherOwnerRootID := createEligibleRoot(otherUser.ID, otherAPIKey.ID, "parallel owner root")

	worker.historianOnce(ctx)
	claims, err = db.GetChatHistorianClaims(ctx)
	require.NoError(t, err)
	require.Len(t, claims, 2)
	var otherHistorianID uuid.UUID
	for _, activeClaim := range claims {
		require.NotEqual(t, sameOwnerRootID, activeClaim.RootChatID)
		if activeClaim.RootChatID == otherOwnerRootID {
			otherHistorianID = activeClaim.HistorianChatID.UUID
		}
	}
	require.NotEqual(t, uuid.Nil, otherHistorianID)
	_, err = sqlDB.ExecContext(ctx, `UPDATE chats SET archived = true WHERE id = $1`, sameOwnerRootID)
	require.NoError(t, err)
	_, err = sqlDB.ExecContext(ctx, `UPDATE chats SET status = 'waiting' WHERE id = $1`, otherHistorianID)
	require.NoError(t, err)
	worker.historianOnce(ctx)
	claims, err = db.GetChatHistorianClaims(ctx)
	require.NoError(t, err)
	require.Len(t, claims, 1)
	require.Equal(t, rootID, claims[0].RootChatID)

	_, err = sqlDB.ExecContext(ctx, `UPDATE chats SET status = 'waiting' WHERE id = $1`, firstChildID)
	require.NoError(t, err)
	worker.historianOnce(ctx)
	claims, err = db.GetChatHistorianClaims(ctx)
	require.NoError(t, err)
	require.Empty(t, claims)
	var processedVersion int64
	require.NoError(t, sqlDB.QueryRowContext(ctx, `
		SELECT last_processed_history_version
		FROM chat_historian_states
		WHERE root_chat_id = $1
	`, rootID).Scan(&processedVersion))
	require.Equal(t, rootResult.Chat.HistoryVersion, processedVersion)

	_, err = server.SendMessage(ctx, SendMessageOptions{
		ChatID:        rootID,
		CreatedBy:     user.ID,
		Content:       []codersdk.ChatMessagePart{codersdk.ChatMessageText("Please keep answers concise")},
		ModelConfigID: modelConfig.ID,
		APIKeyID:      apiKey.ID,
	})
	require.NoError(t, err)
	_, err = sqlDB.ExecContext(ctx, `
		UPDATE chats
		SET status = 'waiting', updated_at = NOW() - INTERVAL '2 minutes'
		WHERE id = $1
	`, rootID)
	require.NoError(t, err)

	worker.historianOnce(ctx)
	claims, err = db.GetChatHistorianClaims(ctx)
	require.NoError(t, err)
	require.Len(t, claims, 1)
	require.Equal(t, firstChildID, claims[0].HistorianChatID.UUID)
	require.NotEqual(t, firstDispatchID, claims[0].DispatchID.UUID)

	_, err = sqlDB.ExecContext(ctx, `UPDATE chats SET status = 'error' WHERE id = $1`, firstChildID)
	require.NoError(t, err)
	worker.historianOnce(ctx)
	claims, err = db.GetChatHistorianClaims(ctx)
	require.NoError(t, err)
	require.Empty(t, claims)
	worker.historianOnce(ctx)
	claims, err = db.GetChatHistorianClaims(ctx)
	require.NoError(t, err)
	require.Len(t, claims, 1)
	require.Equal(t, firstChildID, claims[0].HistorianChatID.UUID)

	_, err = sqlDB.ExecContext(ctx, `DELETE FROM chats WHERE id = $1`, firstChildID)
	require.NoError(t, err)
	worker.historianOnce(ctx)
	worker.historianOnce(ctx)
	claims, err = db.GetChatHistorianClaims(ctx)
	require.NoError(t, err)
	require.Len(t, claims, 1)
	require.True(t, claims[0].HistorianChatID.Valid)
	require.NotEqual(t, firstChildID, claims[0].HistorianChatID.UUID)
}
