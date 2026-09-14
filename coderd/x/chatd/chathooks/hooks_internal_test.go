package chathooks

import (
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
	"github.com/coder/coder/v2/coderd/x/agenthooks/dispatch"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/codersdk/x/agenthooks"
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
	dispatcher := dispatch.New(
		slogtest.Make(t, &slogtest.Options{IgnoreErrors: true}),
		consumer.Client(),
		consumer.URL,
		false,
		secret,
		time.Second,
		"test-deployment",
		"test-version",
		prometheus.NewRegistry(),
	)
	trigger := NewTrigger(dispatcher)
	user := dbgen.User(t, db, database.User{})
	org := dbgen.Organization(t, db, database.Organization{})
	model := dbgen.ChatModelConfig(t, db, database.ChatModelConfig{})
	chat := dbgen.Chat(t, db, database.Chat{OwnerID: user.ID, OrganizationID: org.ID, LastModelConfigID: model.ID})
	turnID := uuid.New()
	ctx := testutil.Context(t, testutil.WaitLong)

	_, err := trigger.Trigger(ctx, ChatFor(chat, &turnID), Message{Source: SessionStartSource(nil)}, agenthooks.EventSessionStart, dispatch.CapacityClassGeneration)
	require.NoError(t, err)
	_, err = trigger.Trigger(ctx, ChatFor(chat, &turnID), Message{Source: SessionStartSource([]database.ChatMessage{{Role: database.ChatMessageRoleAssistant}})}, agenthooks.EventSessionStart, dispatch.CapacityClassGeneration)
	require.NoError(t, err)

	startup := <-receivedCh
	resume := <-receivedCh
	require.Equal(t, agenthooks.EventSessionStart, startup.request.Type)
	require.Equal(t, SessionStartSourceStartup, startup.data.Source)
	require.Equal(t, startup.request.Meta.DispatchID, startup.claims.JTI)
	require.Equal(t, agenthooks.EventSessionStart, resume.request.Type)
	require.Equal(t, SessionStartSourceResume, resume.data.Source)
	require.Equal(t, resume.request.Meta.DispatchID, resume.claims.JTI)
	require.NotEqual(t, startup.claims.JTI, resume.claims.JTI)
}

func TestRejectDuplicateToolUseIDs(t *testing.T) {
	t.Parallel()

	require.NoError(t, RejectDuplicateToolUseIDs([]fantasy.ToolCallContent{
		{ToolCallID: "first", ToolName: "read_file", Input: `{}`},
		{ToolCallID: "second", ToolName: "execute", Input: `{}`},
	}))
	require.ErrorContains(t, RejectDuplicateToolUseIDs([]fantasy.ToolCallContent{
		{ToolCallID: "duplicate", ToolName: "read_file", Input: `{}`},
		{ToolCallID: "duplicate", ToolName: "execute", Input: `{}`},
	}), "duplicate tool use ID")
}

func newTestTrigger(t *testing.T, handler http.Handler) *Trigger {
	t.Helper()
	consumer := httptest.NewServer(handler)
	t.Cleanup(consumer.Close)
	return NewTrigger(dispatch.New(
		slogtest.Make(t, &slogtest.Options{IgnoreErrors: true}),
		consumer.Client(),
		consumer.URL,
		false,
		"test-hook-secret-32-bytes-minimum!!",
		time.Second,
		"test-deployment",
		"test-version",
		prometheus.NewRegistry(),
	))
}

func TestHookTriggerDisabled(t *testing.T) {
	t.Parallel()

	for name, trigger := range map[string]*Trigger{
		"NilTrigger":    nil,
		"NilDispatcher": NewTrigger(nil),
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			require.False(t, trigger.Enabled())
			result, err := trigger.Trigger(t.Context(), Chat{ID: uuid.New()}, Message{}, agenthooks.EventStop, dispatch.CapacityClassGeneration)
			require.NoError(t, err)
			require.Empty(t, result.GetModelContext())
			require.Empty(t, result.GetUserMessage())
			require.Empty(t, result.InputOverride)
		})
	}
}

func TestHookTriggerDeny(t *testing.T) {
	t.Parallel()

	trigger := newTestTrigger(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, err := w.Write([]byte(`{
			"permission": {"decision": "deny", "reason": "policy"},
			"model_context": "try another tool",
			"user_message": "blocked by policy"
		}`))
		assert.NoError(t, err)
	}))
	ctx := testutil.Context(t, testutil.WaitShort)
	result, err := trigger.Trigger(ctx, Chat{ID: uuid.New(), OwnerID: uuid.New()}, Message{
		ToolUseID: "call_1",
		ToolName:  "execute",
		ToolInput: json.RawMessage(`{}`),
	}, agenthooks.EventPreToolUse, dispatch.CapacityClassGeneration)
	require.Nil(t, result)
	var denied *deniedError
	require.ErrorAs(t, err, &denied)
	require.Equal(t, agenthooks.EventPreToolUse, denied.Event)
	require.Equal(t, "policy", denied.Reason)
	require.Equal(t, "try another tool", denied.ModelContext)
	require.Equal(t, "blocked by policy", denied.UserMessage)
}

func TestHookTriggerEventPayloads(t *testing.T) {
	t.Parallel()

	requests := make(chan agenthooks.Request, 1)
	trigger := newTestTrigger(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request agenthooks.Request
		assert.NoError(t, json.NewDecoder(r.Body).Decode(&request))
		requests <- request
		_, err := w.Write([]byte(`{}`))
		assert.NoError(t, err)
	}))
	chat := Chat{
		ID:          uuid.New(),
		OwnerID:     uuid.New(),
		WorkspaceID: uuid.NullUUID{UUID: uuid.New(), Valid: true},
	}
	ctx := testutil.Context(t, testutil.WaitShort)
	dispatchEvent := func(t *testing.T, msg Message, event agenthooks.EventType) agenthooks.Request {
		t.Helper()
		_, err := trigger.Trigger(ctx, chat, msg, event, dispatch.CapacityClassGeneration)
		require.NoError(t, err)
		request := <-requests
		require.Equal(t, event, request.Type)
		require.Equal(t, chat.ID, request.Meta.ChatID)
		require.Equal(t, chat.OwnerID, request.Meta.OwnerID)
		require.NotNil(t, request.Meta.WorkspaceID)
		require.Equal(t, chat.WorkspaceID.UUID, *request.Meta.WorkspaceID)
		return request
	}

	sessionStart := dispatchEvent(t, Message{Source: SessionStartSourceClear}, agenthooks.EventSessionStart)
	var sessionStartData agenthooks.SessionStartData
	require.NoError(t, json.Unmarshal(sessionStart.Data, &sessionStartData))
	require.Equal(t, SessionStartSourceClear, sessionStartData.Source)

	prompt := dispatchEvent(t, Message{Prompt: "hello", Parts: json.RawMessage(`[{"type":"text","text":"hello"}]`)}, agenthooks.EventUserPromptSubmit)
	var promptData agenthooks.UserPromptSubmitData
	require.NoError(t, json.Unmarshal(prompt.Data, &promptData))
	require.Equal(t, "hello", promptData.Prompt)
	require.JSONEq(t, `[{"type":"text","text":"hello"}]`, string(promptData.Parts))

	preToolUse := dispatchEvent(t, Message{ToolUseID: "call_1", ToolName: "execute", ToolInput: json.RawMessage(`{"cmd":"ls"}`)}, agenthooks.EventPreToolUse)
	var preToolUseData agenthooks.PreToolUseData
	require.NoError(t, json.Unmarshal(preToolUse.Data, &preToolUseData))
	require.Equal(t, "call_1", preToolUseData.ToolUseID)
	require.Equal(t, "execute", preToolUseData.ToolName)
	require.JSONEq(t, `{"cmd":"ls"}`, string(preToolUseData.ToolInput))

	postToolUse := dispatchEvent(t, Message{ToolUseID: "call_1", ToolName: "execute", ToolResponse: json.RawMessage(`{"ok":true}`), ToolError: "boom"}, agenthooks.EventPostToolUse)
	var postToolUseData agenthooks.PostToolUseData
	require.NoError(t, json.Unmarshal(postToolUse.Data, &postToolUseData))
	require.Equal(t, "call_1", postToolUseData.ToolUseID)
	require.Equal(t, "execute", postToolUseData.ToolName)
	require.JSONEq(t, `{"ok":true}`, string(postToolUseData.ToolResponse))
	require.Equal(t, "boom", postToolUseData.ToolError)

	for _, event := range []agenthooks.EventType{agenthooks.EventPreCompact, agenthooks.EventPostCompact, agenthooks.EventStop} {
		dispatchEvent(t, Message{}, event)
	}

	_, err := trigger.Trigger(ctx, chat, Message{}, agenthooks.EventType("bogus"), dispatch.CapacityClassGeneration)
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
	RestoreToolCallOrder(content, calls)
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
	RestoreToolCallOrder(mixed, calls)
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

func TestEventMessagesSkipsBlankModelContext(t *testing.T) {
	t.Parallel()

	modelConfigID := uuid.New()
	messages, err := EventMessages(&Result{ModelContext: " \n\t "}, modelConfigID)
	require.NoError(t, err)
	require.Empty(t, messages)

	messages, err = EventMessages(&Result{ModelContext: "real context"}, modelConfigID)
	require.NoError(t, err)
	require.Len(t, messages, 1)
	require.Equal(t, database.ChatMessageVisibilityModel, messages[0].Visibility)
}

func TestComposeUserPromptContentOverride(t *testing.T) {
	t.Parallel()

	text := codersdk.ChatMessageText("original")
	reference := codersdk.ChatMessageFileReference("main.go", 1, 3, "package main")
	upload := codersdk.ChatMessageFile(uuid.New(), "image/png", "shot.png")
	override := &Result{InputOverride: json.RawMessage(`{"prompt":"replacement"}`)}

	t.Run("ReplacesTextInPlaceAndKeepsAttachments", func(t *testing.T) {
		t.Parallel()

		parts, overridden, err := ComposeUserPromptContent([]codersdk.ChatMessagePart{reference, text, upload}, override)
		require.NoError(t, err)
		require.True(t, overridden)
		require.Equal(t, []codersdk.ChatMessagePart{
			reference,
			codersdk.ChatMessageText("replacement"),
			upload,
		}, parts)
	})

	t.Run("CollapsesEveryTextPart", func(t *testing.T) {
		t.Parallel()

		parts, overridden, err := ComposeUserPromptContent([]codersdk.ChatMessagePart{
			text,
			upload,
			codersdk.ChatMessageText("trailing"),
		}, override)
		require.NoError(t, err)
		require.True(t, overridden)
		require.Equal(t, []codersdk.ChatMessagePart{
			codersdk.ChatMessageText("replacement"),
			upload,
		}, parts)
	})

	t.Run("AppendsWhenSubmissionHasNoText", func(t *testing.T) {
		t.Parallel()

		parts, overridden, err := ComposeUserPromptContent([]codersdk.ChatMessagePart{upload}, override)
		require.NoError(t, err)
		require.True(t, overridden)
		require.Equal(t, []codersdk.ChatMessagePart{upload, codersdk.ChatMessageText("replacement")}, parts)
	})

	t.Run("KeepsSubmittedPartsWithoutOverride", func(t *testing.T) {
		t.Parallel()

		submitted := []codersdk.ChatMessagePart{text, upload}
		parts, overridden, err := ComposeUserPromptContent(submitted, &Result{UserMessage: "notice"})
		require.NoError(t, err)
		require.False(t, overridden)
		require.Equal(t, []codersdk.ChatMessagePart{
			text,
			upload,
			{Type: codersdk.ChatMessagePartTypeHookNotice, Text: "notice"},
		}, parts)
	})
}
