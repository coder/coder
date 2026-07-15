package chatd

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"charm.land/fantasy"
	"github.com/google/uuid"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/sqlc-dev/pqtype"
	"github.com/stretchr/testify/require"

	"cdr.dev/slog/v3/sloggers/slogtest"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbgen"
	"github.com/coder/coder/v2/coderd/database/dbtestutil"
	"github.com/coder/coder/v2/coderd/x/chatd/chathooks"
	"github.com/coder/coder/v2/coderd/x/chatd/chatloop"
	"github.com/coder/coder/v2/coderd/x/chatd/chatprompt"
	"github.com/coder/coder/v2/coderd/x/chatd/chatstate"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/codersdk/agenthooks"
	"github.com/coder/coder/v2/testutil"
)

func TestSessionStartDispatchSources(t *testing.T) {
	t.Parallel()

	const secret = "test-hook-secret-32-bytes-minimum!!"
	type received struct {
		request agenthooks.Request
		claims  agenthooks.Claims
		data    *agenthooks.SessionStartData
	}
	receivedCh := make(chan received, 2)
	consumer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request agenthooks.Request
		require.NoError(t, json.NewDecoder(r.Body).Decode(&request))
		claims, err := agenthooks.Verify(r.Header.Get("Authorization"), []byte(secret))
		require.NoError(t, err)
		decoded, err := request.Decode()
		require.NoError(t, err)
		data, ok := decoded.(*agenthooks.SessionStartData)
		require.True(t, ok)
		receivedCh <- received{request: request, claims: claims, data: data}
		_, err = w.Write([]byte(`{}`))
		require.NoError(t, err)
	}))
	t.Cleanup(consumer.Close)

	db, _ := dbtestutil.NewDB(t)
	dispatcher := chathooks.New(
		slogtest.Make(t, &slogtest.Options{IgnoreErrors: true}),
		db,
		consumer.Client(),
		consumer.URL,
		secret,
		time.Second,
		"test-deployment",
		"test-version",
		prometheus.NewRegistry(),
	)
	server := &Server{hookDispatcher: dispatcher}
	user := dbgen.User(t, db, database.User{})
	org := dbgen.Organization(t, db, database.Organization{})
	model := dbgen.ChatModelConfig(t, db, database.ChatModelConfig{})
	chat := dbgen.Chat(t, db, database.Chat{OwnerID: user.ID, OrganizationID: org.ID, LastModelConfigID: model.ID})
	turnID := uuid.New()
	ctx := testutil.Context(t, testutil.WaitLong)

	_, err := server.dispatchSessionStart(ctx, chat, &turnID, sessionStartSource(nil))
	require.NoError(t, err)
	_, err = server.dispatchSessionStart(ctx, chat, &turnID, sessionStartSource([]database.ChatMessage{{Role: database.ChatMessageRoleAssistant}}))
	require.NoError(t, err)

	startup := <-receivedCh
	resume := <-receivedCh
	require.Equal(t, agenthooks.EventSessionStart, startup.request.Type)
	require.Equal(t, sessionStartSourceStartup, startup.data.Source)
	require.Equal(t, startup.request.Meta.DispatchID, startup.claims.JTI)
	require.Equal(t, agenthooks.EventSessionStart, resume.request.Type)
	require.Equal(t, sessionStartSourceResume, resume.data.Source)
	require.Equal(t, resume.request.Meta.DispatchID, resume.claims.JTI)
	require.NotEqual(t, startup.claims.JTI, resume.claims.JTI)
}

func TestSessionStartTrackerRetriesIncompleteDispatch(t *testing.T) {
	t.Parallel()
	tracker := &sessionStartTracker{}
	claimed, complete, err := tracker.claim(t.Context())
	require.NoError(t, err)
	require.True(t, claimed)

	canceled, cancel := context.WithCancel(t.Context())
	cancel()
	_, _, err = tracker.claim(canceled)
	require.ErrorIs(t, err, context.Canceled)
	complete(false)

	claimed, complete, err = tracker.claim(t.Context())
	require.NoError(t, err)
	require.True(t, claimed)
	complete(true)
	claimed, _, err = tracker.claim(t.Context())
	require.NoError(t, err)
	require.False(t, claimed)
}

func TestSessionStartDispatchFailureFinishesGeneration(t *testing.T) {
	t.Parallel()
	f := newTaskTestFixture(t)
	chat := f.createRunningChat(t)
	workerID := uuid.New()
	runnerID := uuid.New()
	chat = f.acquireChat(t, chat.ID, workerID, runnerID)
	consumer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	t.Cleanup(consumer.Close)
	dispatcher := chathooks.New(
		slogtest.Make(t, &slogtest.Options{IgnoreErrors: true}),
		f.db,
		consumer.Client(),
		consumer.URL,
		"test-hook-secret-32-bytes-minimum!!",
		time.Second,
		"test-deployment",
		"test-version",
		prometheus.NewRegistry(),
	)
	starter := newTestTaskStarter(t, f, newTaskSideEffectRecorder())
	starter.server.hookDispatcher = dispatcher
	ctx := testutil.Context(t, testutil.WaitLong)
	debugTurn := newRunnerDebugTurn(ctx, starter.opts.Logger)
	defer debugTurn.Finalize(ctx)
	err := starter.StartGeneration(ctx, chatWorkerTaskStartInput{
		ChatID:            chat.ID,
		WorkerID:          workerID,
		RunnerID:          runnerID,
		HistoryVersion:    chat.HistoryVersion,
		GenerationAttempt: chat.GenerationAttempt,
		Status:            database.ChatStatusRunning,
		DebugTurn:         debugTurn,
		SessionStart:      &sessionStartTracker{},
	})
	require.NoError(t, err)
	updated, err := f.db.GetChatByID(ctx, chat.ID)
	require.NoError(t, err)
	require.Equal(t, database.ChatStatusError, updated.Status)
	var chatErr codersdk.ChatError
	require.NoError(t, json.Unmarshal(updated.LastError.RawMessage, &chatErr))
	require.Equal(t, codersdk.ChatErrorKindHookDispatchFailed, chatErr.Kind)
	require.Contains(t, chatErr.Message, "hook dispatch failed: session_start: http_error (dispatch ")
	require.False(t, chatErr.Retryable)
}

func TestApplySessionStartResponse(t *testing.T) {
	t.Parallel()
	f := newTaskTestFixture(t)
	chat := f.createRunningChat(t)
	workerID := uuid.New()
	runnerID := uuid.New()
	chat = f.acquireChat(t, chat.ID, workerID, runnerID)
	ctx := testutil.Context(t, testutil.WaitLong)
	messages, err := f.db.GetChatMessagesByChatID(ctx, database.GetChatMessagesByChatIDParams{ChatID: chat.ID})
	require.NoError(t, err)
	require.NotEmpty(t, messages)
	turnID := messages[len(messages)-1].TurnID.UUID
	input := chatWorkerTaskStartInput{
		ChatID:         chat.ID,
		WorkerID:       workerID,
		RunnerID:       runnerID,
		HistoryVersion: chat.HistoryVersion,
		Status:         database.ChatStatusRunning,
	}
	result, err := applySessionStartResponse(
		ctx,
		chatstate.NewChatMachine(f.db, f.pubsub, chat.ID),
		input,
		chat,
		&turnID,
		agenthooks.Response{
			ModelContext: "model context",
			UserMessage:  "user notice",
			AllowedTools: []string{"read_file"},
		},
	)
	require.NoError(t, err)
	require.False(t, result.Ended)

	promptRows, err := f.db.GetChatMessagesForPromptByChatID(ctx, chat.ID)
	require.NoError(t, err)
	require.Equal(t, "model context", hookMessageTextInternal(t, promptRows[len(promptRows)-1]))
	require.Equal(t, database.ChatMessageVisibilityModel, promptRows[len(promptRows)-1].Visibility)
	allRows, err := f.db.GetChatMessagesByChatID(ctx, database.GetChatMessagesByChatIDParams{ChatID: chat.ID})
	require.NoError(t, err)
	userNotice := allRows[len(allRows)-1]
	require.Equal(t, database.ChatMessageRoleSystem, userNotice.Role)
	require.Equal(t, database.ChatMessageVisibilityUser, userNotice.Visibility)
	require.Equal(t, "user notice", hookMessageTextInternal(t, userNotice))
	require.JSONEq(t, `["read_file"]`, string(result.Chat.HookAllowedTools.RawMessage))
}

func TestApplySessionStartResponseNoOp(t *testing.T) {
	t.Parallel()
	f := newTaskTestFixture(t)
	chat := f.createRunningChat(t)
	f.pubsub.clear()
	result, err := applySessionStartResponse(
		testutil.Context(t, testutil.WaitLong),
		chatstate.NewChatMachine(f.db, f.pubsub, chat.ID),
		chatWorkerTaskStartInput{},
		chat,
		nil,
		agenthooks.Response{},
	)
	require.NoError(t, err)
	require.Equal(t, chat.SnapshotVersion, result.Chat.SnapshotVersion)
	require.Empty(t, f.pubsub.events())
}

func TestApplySessionStartResponseEndChat(t *testing.T) {
	t.Parallel()
	f := newTaskTestFixture(t)
	chat := f.createRunningChat(t)
	workerID := uuid.New()
	runnerID := uuid.New()
	chat = f.acquireChat(t, chat.ID, workerID, runnerID)
	ctx := testutil.Context(t, testutil.WaitLong)
	messages, err := f.db.GetChatMessagesByChatID(ctx, database.GetChatMessagesByChatIDParams{ChatID: chat.ID})
	require.NoError(t, err)
	turnID := messages[len(messages)-1].TurnID.UUID
	result, err := applySessionStartResponse(
		ctx,
		chatstate.NewChatMachine(f.db, f.pubsub, chat.ID),
		chatWorkerTaskStartInput{
			ChatID:         chat.ID,
			WorkerID:       workerID,
			RunnerID:       runnerID,
			HistoryVersion: chat.HistoryVersion,
			Status:         database.ChatStatusRunning,
		},
		chat,
		&turnID,
		agenthooks.Response{UserMessage: "ended by hook", EndChat: true},
	)
	require.NoError(t, err)
	require.True(t, result.Ended)
	require.True(t, result.Chat.Archived)
	require.Equal(t, database.ChatStatusWaiting, result.Chat.Status)
	require.False(t, result.Chat.WorkerID.Valid)
	require.False(t, result.Chat.RunnerID.Valid)
	rows, err := f.db.GetChatMessagesByChatID(ctx, database.GetChatMessagesByChatIDParams{ChatID: chat.ID})
	require.NoError(t, err)
	require.Equal(t, "ended by hook", hookMessageTextInternal(t, rows[len(rows)-1]))
}

func hookMessageTextInternal(t *testing.T, message database.ChatMessage) string {
	t.Helper()
	parts, err := chatprompt.ParseContent(message)
	require.NoError(t, err)
	require.Len(t, parts, 1)
	return parts[0].Text
}

func TestFilterHookAllowedTools(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		allowed       pqtype.NullRawMessage
		wantTools     []string
		wantProviders []string
	}{
		{
			name:          "null",
			wantTools:     []string{"read_file", "dynamic_tool"},
			wantProviders: []string{"web_search"},
		},
		{
			name:          "empty",
			allowed:       pqtype.NullRawMessage{RawMessage: []byte(`[]`), Valid: true},
			wantTools:     []string{},
			wantProviders: []string{},
		},
		{
			name:          "subset",
			allowed:       pqtype.NullRawMessage{RawMessage: []byte(`["dynamic_tool","web_search"]`), Valid: true},
			wantTools:     []string{"dynamic_tool"},
			wantProviders: []string{"web_search"},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			tools := []fantasy.AgentTool{newTestAgentTool("read_file"), newTestAgentTool("dynamic_tool")}
			providerTools := []chatloop.ProviderTool{{
				Definition: fantasy.ProviderDefinedTool{ID: "web_search", Name: "web_search"},
			}}
			tools, providerTools, _, err := filterHookAllowedTools(test.allowed, tools, providerTools)
			require.NoError(t, err)
			toolNames := make([]string, 0, len(tools))
			for _, tool := range tools {
				toolNames = append(toolNames, tool.Info().Name)
			}
			providerNames := make([]string, 0, len(providerTools))
			for _, tool := range providerTools {
				providerNames = append(providerNames, tool.Definition.GetName())
			}
			require.Equal(t, test.wantTools, toolNames)
			require.Equal(t, test.wantProviders, providerNames)
		})
	}
}
