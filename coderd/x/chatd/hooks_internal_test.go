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
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"cdr.dev/slog/v3/sloggers/slogtest"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbgen"
	"github.com/coder/coder/v2/coderd/database/dbtestutil"
	"github.com/coder/coder/v2/coderd/x/chatd/chatprompt"
	"github.com/coder/coder/v2/coderd/x/chatd/chatstate"
	"github.com/coder/coder/v2/coderd/x/chathooks"
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
		data    agenthooks.SessionStartData
	}
	receivedCh := make(chan received, 2)
	consumer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request agenthooks.Request
		require.NoError(t, json.NewDecoder(r.Body).Decode(&request))
		claims, err := agenthooks.Verify(r.Header.Get("Authorization"), []byte(secret))
		require.NoError(t, err)
		var data agenthooks.SessionStartData
		require.NoError(t, json.Unmarshal(request.Data, &data))
		receivedCh <- received{request: request, claims: claims, data: data}
		_, err = w.Write([]byte(`{}`))
		require.NoError(t, err)
	}))
	t.Cleanup(consumer.Close)

	db, _ := dbtestutil.NewDB(t)
	dispatcher := chathooks.New(
		slogtest.Make(t, &slogtest.Options{IgnoreErrors: true}),
		consumer.Client(),
		consumer.URL,
		secret,
		time.Second,
		"test-deployment",
		"test-version",
		prometheus.NewRegistry(),
	)
	server := &Server{hooks: newHookTrigger(dispatcher)}
	user := dbgen.User(t, db, database.User{})
	org := dbgen.Organization(t, db, database.Organization{})
	model := dbgen.ChatModelConfig(t, db, database.ChatModelConfig{})
	chat := dbgen.Chat(t, db, database.Chat{OwnerID: user.ID, OrganizationID: org.ID, LastModelConfigID: model.ID})
	turnID := uuid.New()
	ctx := testutil.Context(t, testutil.WaitLong)

	_, err := server.hooks.trigger(ctx, hookChatFor(chat, &turnID), hookMessage{Source: sessionStartSource(nil)}, agenthooks.EventSessionStart)
	require.NoError(t, err)
	_, err = server.hooks.trigger(ctx, hookChatFor(chat, &turnID), hookMessage{Source: sessionStartSource([]database.ChatMessage{{Role: database.ChatMessageRoleAssistant}})}, agenthooks.EventSessionStart)
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
		consumer.Client(),
		consumer.URL,
		"test-hook-secret-32-bytes-minimum!!",
		time.Second,
		"test-deployment",
		"test-version",
		prometheus.NewRegistry(),
	)
	starter := newTestTaskStarter(t, f, newTaskSideEffectRecorder())
	starter.server.hooks = newHookTrigger(dispatcher)
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
	input := chatWorkerTaskStartInput{
		ChatID:         chat.ID,
		WorkerID:       workerID,
		RunnerID:       runnerID,
		HistoryVersion: chat.HistoryVersion,
		Status:         database.ChatStatusRunning,
	}
	_, err := applySessionStartResponse(
		ctx,
		chatstate.NewChatMachine(f.db, f.pubsub, chat.ID),
		input,
		chat,
		&hookResult{
			ModelContext: "model context",
			UserMessage:  "user notice",
		},
	)
	require.NoError(t, err)

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
	)
	require.NoError(t, err)
	require.Equal(t, chat.SnapshotVersion, result.Chat.SnapshotVersion)
	require.Empty(t, f.pubsub.events())
}

func hookMessageTextInternal(t *testing.T, message database.ChatMessage) string {
	t.Helper()
	parts, err := chatprompt.ParseContent(message)
	require.NoError(t, err)
	require.Len(t, parts, 1)
	return parts[0].Text
}

func TestRejectDuplicateToolUseIDs(t *testing.T) {
	t.Parallel()

	require.NoError(t, rejectDuplicateToolUseIDs([]fantasy.ToolCallContent{
		{ToolCallID: "first", ToolName: "read_file", Input: `{}`},
		{ToolCallID: "second", ToolName: "execute", Input: `{}`},
	}))
	require.ErrorContains(t, rejectDuplicateToolUseIDs([]fantasy.ToolCallContent{
		{ToolCallID: "duplicate", ToolName: "read_file", Input: `{}`},
		{ToolCallID: "duplicate", ToolName: "execute", Input: `{}`},
	}), "duplicate tool use ID")
}

func newTestHookTrigger(t *testing.T, handler http.Handler) *hookTrigger {
	t.Helper()
	consumer := httptest.NewServer(handler)
	t.Cleanup(consumer.Close)
	return newHookTrigger(chathooks.New(
		slogtest.Make(t, &slogtest.Options{IgnoreErrors: true}),
		consumer.Client(),
		consumer.URL,
		"test-hook-secret-32-bytes-minimum!!",
		time.Second,
		"test-deployment",
		"test-version",
		prometheus.NewRegistry(),
	))
}

func TestHookTriggerDisabled(t *testing.T) {
	t.Parallel()

	for name, trigger := range map[string]*hookTrigger{
		"NilTrigger":    nil,
		"NilDispatcher": newHookTrigger(nil),
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			require.False(t, trigger.enabled())
			result, err := trigger.trigger(t.Context(), hookChat{ID: uuid.New()}, hookMessage{}, agenthooks.EventStop)
			require.NoError(t, err)
			require.Empty(t, result.modelContext())
			require.Empty(t, result.userMessage())
			require.Empty(t, result.InputOverride)
		})
	}
}

func TestHookTriggerDeny(t *testing.T) {
	t.Parallel()

	trigger := newTestHookTrigger(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, err := w.Write([]byte(`{
			"permission": {"decision": "deny", "reason": "policy"},
			"model_context": "try another tool",
			"user_message": "blocked by policy"
		}`))
		assert.NoError(t, err)
	}))
	ctx := testutil.Context(t, testutil.WaitShort)
	result, err := trigger.trigger(ctx, hookChat{ID: uuid.New(), OwnerID: uuid.New()}, hookMessage{
		ToolUseID: "call_1",
		ToolName:  "execute",
		ToolInput: json.RawMessage(`{}`),
	}, agenthooks.EventPreToolUse)
	require.Nil(t, result)
	var denied *hookDeniedError
	require.ErrorAs(t, err, &denied)
	require.Equal(t, agenthooks.EventPreToolUse, denied.Event)
	require.Equal(t, "policy", denied.Reason)
	require.Equal(t, "try another tool", denied.ModelContext)
	require.Equal(t, "blocked by policy", denied.UserMessage)
}

func TestHookTriggerEventPayloads(t *testing.T) {
	t.Parallel()

	requests := make(chan agenthooks.Request, 1)
	trigger := newTestHookTrigger(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request agenthooks.Request
		assert.NoError(t, json.NewDecoder(r.Body).Decode(&request))
		requests <- request
		_, err := w.Write([]byte(`{}`))
		assert.NoError(t, err)
	}))
	chat := hookChat{
		ID:          uuid.New(),
		OwnerID:     uuid.New(),
		WorkspaceID: uuid.NullUUID{UUID: uuid.New(), Valid: true},
	}
	ctx := testutil.Context(t, testutil.WaitShort)
	dispatch := func(t *testing.T, msg hookMessage, event agenthooks.EventType) agenthooks.Request {
		t.Helper()
		_, err := trigger.trigger(ctx, chat, msg, event)
		require.NoError(t, err)
		request := <-requests
		require.Equal(t, event, request.Type)
		require.Equal(t, chat.ID, request.Meta.ChatID)
		require.Equal(t, chat.OwnerID, request.Meta.OwnerID)
		require.NotNil(t, request.Meta.WorkspaceID)
		require.Equal(t, chat.WorkspaceID.UUID, *request.Meta.WorkspaceID)
		return request
	}

	sessionStart := dispatch(t, hookMessage{Source: sessionStartSourceClear}, agenthooks.EventSessionStart)
	var sessionStartData agenthooks.SessionStartData
	require.NoError(t, json.Unmarshal(sessionStart.Data, &sessionStartData))
	require.Equal(t, sessionStartSourceClear, sessionStartData.Source)

	prompt := dispatch(t, hookMessage{Prompt: "hello", Parts: json.RawMessage(`[{"type":"text","text":"hello"}]`)}, agenthooks.EventUserPromptSubmit)
	var promptData agenthooks.UserPromptSubmitData
	require.NoError(t, json.Unmarshal(prompt.Data, &promptData))
	require.Equal(t, "hello", promptData.Prompt)
	require.JSONEq(t, `[{"type":"text","text":"hello"}]`, string(promptData.Parts))

	preToolUse := dispatch(t, hookMessage{ToolUseID: "call_1", ToolName: "execute", ToolInput: json.RawMessage(`{"cmd":"ls"}`)}, agenthooks.EventPreToolUse)
	var preToolUseData agenthooks.PreToolUseData
	require.NoError(t, json.Unmarshal(preToolUse.Data, &preToolUseData))
	require.Equal(t, "call_1", preToolUseData.ToolUseID)
	require.Equal(t, "execute", preToolUseData.ToolName)
	require.JSONEq(t, `{"cmd":"ls"}`, string(preToolUseData.ToolInput))

	postToolUse := dispatch(t, hookMessage{ToolUseID: "call_1", ToolName: "execute", ToolResponse: json.RawMessage(`{"ok":true}`), ToolError: "boom"}, agenthooks.EventPostToolUse)
	var postToolUseData agenthooks.PostToolUseData
	require.NoError(t, json.Unmarshal(postToolUse.Data, &postToolUseData))
	require.Equal(t, "call_1", postToolUseData.ToolUseID)
	require.Equal(t, "execute", postToolUseData.ToolName)
	require.JSONEq(t, `{"ok":true}`, string(postToolUseData.ToolResponse))
	require.Equal(t, "boom", postToolUseData.ToolError)

	for _, event := range []agenthooks.EventType{agenthooks.EventPreCompact, agenthooks.EventPostCompact, agenthooks.EventStop} {
		dispatch(t, hookMessage{}, event)
	}

	_, err := trigger.trigger(ctx, chat, hookMessage{}, agenthooks.EventType("bogus"))
	require.ErrorContains(t, err, "unsupported hook event")
}

func TestRestoreToolCallOrder(t *testing.T) {
	t.Parallel()

	calls := []fantasy.ToolCallContent{
		{ToolCallID: "call_a", ToolName: "write_file"},
		{ToolCallID: "call_b", ToolName: "read_file"},
		{ToolCallID: "call_c", ToolName: "execute"},
	}
	content := []fantasy.Content{
		fantasy.ToolResultContent{ToolCallID: "call_c", ToolName: "execute"},
		fantasy.ToolResultContent{ToolCallID: "call_b", ToolName: "read_file"},
		fantasy.ToolResultContent{ToolCallID: "call_a", ToolName: "write_file"},
	}
	restoreToolCallOrder(content, calls)
	gotIDs := make([]string, 0, len(content))
	for _, entry := range content {
		result, ok := entry.(fantasy.ToolResultContent)
		require.True(t, ok)
		gotIDs = append(gotIDs, result.ToolCallID)
	}
	require.Equal(t, []string{"call_a", "call_b", "call_c"}, gotIDs)

	mixed := []fantasy.Content{
		fantasy.ToolResultContent{ToolCallID: "call_b", ToolName: "read_file"},
		fantasy.TextContent{Text: "note"},
		fantasy.ToolResultContent{ToolCallID: "unknown", ToolName: "other"},
		fantasy.ToolResultContent{ToolCallID: "call_a", ToolName: "write_file"},
	}
	restoreToolCallOrder(mixed, calls)
	first, ok := mixed[0].(fantasy.ToolResultContent)
	require.True(t, ok)
	require.Equal(t, "call_a", first.ToolCallID)
	_, ok = mixed[1].(fantasy.TextContent)
	require.True(t, ok)
	unknown, ok := mixed[2].(fantasy.ToolResultContent)
	require.True(t, ok)
	require.Equal(t, "unknown", unknown.ToolCallID)
	last, ok := mixed[3].(fantasy.ToolResultContent)
	require.True(t, ok)
	require.Equal(t, "call_b", last.ToolCallID)
}
