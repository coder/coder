package chatd_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/sqlc-dev/pqtype"
	"github.com/stretchr/testify/require"

	"cdr.dev/slog/v3/sloggers/slogtest"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbgen"
	"github.com/coder/coder/v2/coderd/database/dbtestutil"
	dbpubsub "github.com/coder/coder/v2/coderd/database/pubsub"
	"github.com/coder/coder/v2/coderd/x/chatd"
	"github.com/coder/coder/v2/coderd/x/chatd/chathooks"
	"github.com/coder/coder/v2/coderd/x/chatd/chatprompt"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/codersdk/agenthooks"
	"github.com/coder/coder/v2/testutil"
)

func TestSendMessageUserPromptSubmitHook(t *testing.T) {
	t.Parallel()

	t.Run("override", func(t *testing.T) {
		t.Parallel()
		db, ps := dbtestutil.NewDB(t)
		ctx := testutil.Context(t, testutil.WaitLong)
		user, org, model := seedChatDependencies(t, db)
		chat := dbgen.Chat(t, db, database.Chat{
			OrganizationID:    org.ID,
			OwnerID:           user.ID,
			LastModelConfigID: model.ID,
		})

		consumer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			var request agenthooks.Request
			require.NoError(t, json.NewDecoder(r.Body).Decode(&request))
			data, err := request.Decode()
			require.NoError(t, err)
			require.Equal(t, &agenthooks.UserPromptSubmitData{Prompt: "before"}, data)
			require.NotNil(t, request.Meta.TurnID)
			_, err = w.Write([]byte(`{"permission":{"decision":"allow","input_override":{"prompt":"after"}},"model_context":"model only","user_message":"user only","allowed_tools":["read","write"]}`))
			require.NoError(t, err)
		}))
		t.Cleanup(consumer.Close)

		server := newHookTestServer(t, db, ps, consumer)
		result, err := server.SendMessage(ctx, chatd.SendMessageOptions{
			ChatID:    chat.ID,
			CreatedBy: user.ID,
			Content:   []codersdk.ChatMessagePart{codersdk.ChatMessageText("before")},
			APIKeyID:  testAPIKeyID(t, db, user.ID),
		})
		require.NoError(t, err)
		require.True(t, result.Message.TurnID.Valid)
		parts, err := chatprompt.ParseContent(result.Message)
		require.NoError(t, err)
		require.Equal(t, []codersdk.ChatMessagePart{codersdk.ChatMessageText("after")}, parts)
	})

	t.Run("deny", func(t *testing.T) {
		t.Parallel()
		db, ps := dbtestutil.NewDB(t)
		ctx := testutil.Context(t, testutil.WaitLong)
		user, org, model := seedChatDependencies(t, db)
		chat := dbgen.Chat(t, db, database.Chat{
			OrganizationID:    org.ID,
			OwnerID:           user.ID,
			LastModelConfigID: model.ID,
		})

		consumer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			_, err := w.Write([]byte(`{"permission":{"decision":"deny"},"user_message":"blocked"}`))
			require.NoError(t, err)
		}))
		t.Cleanup(consumer.Close)

		server := newHookTestServer(t, db, ps, consumer)
		_, err := server.SendMessage(ctx, chatd.SendMessageOptions{
			ChatID:    chat.ID,
			CreatedBy: user.ID,
			Content:   []codersdk.ChatMessagePart{codersdk.ChatMessageText("blocked prompt")},
			APIKeyID:  testAPIKeyID(t, db, user.ID),
		})
		var denied *chatd.UserPromptDeniedError
		require.ErrorAs(t, err, &denied)
		require.Equal(t, "blocked", denied.UserMessage)

		messages, err := db.GetChatMessagesByChatID(ctx, database.GetChatMessagesByChatIDParams{ChatID: chat.ID})
		require.NoError(t, err)
		require.Empty(t, messages)
	})
}

func newHookDispatcher(t *testing.T, db database.Store, consumer *httptest.Server) *chathooks.Dispatcher {
	t.Helper()
	return chathooks.New(
		slogtest.Make(t, &slogtest.Options{IgnoreErrors: true}),
		db,
		consumer.Client(),
		consumer.URL,
		"test-hook-secret-32-bytes-minimum!!",
		time.Second,
		"test-deployment",
		"test-version",
		prometheus.NewRegistry(),
	)
}

func newHookTestServer(t *testing.T, db database.Store, ps dbpubsub.Pubsub, consumer *httptest.Server) *chatd.Server {
	t.Helper()
	return newTestServer(t, db, ps, uuid.New(), func(cfg *chatd.Config) {
		cfg.HookDispatcher = newHookDispatcher(t, db, consumer)
	})
}

func lifecycleDispatch(t *testing.T, db database.Store, chatID uuid.UUID, event agenthooks.EventType) database.ChatHookDispatch {
	t.Helper()
	rows, err := db.ListChatHookDispatchesByChatID(t.Context(), chatID)
	require.NoError(t, err)
	for _, row := range rows {
		if row.Event == string(event) {
			return row
		}
	}
	require.FailNow(t, "lifecycle dispatch not found", "event: %s", event)
	return database.ChatHookDispatch{}
}

func TestSendMessageUserPromptSubmitPassthrough(t *testing.T) {
	t.Parallel()
	db, ps := dbtestutil.NewDB(t)
	ctx := testutil.Context(t, testutil.WaitLong)
	user, org, model := seedChatDependencies(t, db)
	chat := dbgen.Chat(t, db, database.Chat{
		OrganizationID:    org.ID,
		OwnerID:           user.ID,
		LastModelConfigID: model.ID,
	})
	server := newHookTestServer(t, db, ps, hookConsumer(t, `{}`))
	result, err := server.SendMessage(ctx, chatd.SendMessageOptions{
		ChatID:   chat.ID,
		Content:  []codersdk.ChatMessagePart{codersdk.ChatMessageText("passthrough")},
		APIKeyID: testAPIKeyID(t, db, user.ID),
	})
	require.NoError(t, err)
	require.Equal(t, "passthrough", hookMessageText(t, result.Message))
}

func TestSendMessageUserPromptSubmitEndChat(t *testing.T) {
	t.Parallel()
	db, ps := dbtestutil.NewDB(t)
	ctx := testutil.Context(t, testutil.WaitLong)
	user, org, model := seedChatDependencies(t, db)
	chat := dbgen.Chat(t, db, database.Chat{
		OrganizationID:    org.ID,
		OwnerID:           user.ID,
		LastModelConfigID: model.ID,
	})
	consumer := hookConsumer(t, `{"end_chat":true}`)
	server := newHookTestServer(t, db, ps, consumer)

	result, err := server.SendMessage(ctx, chatd.SendMessageOptions{
		ChatID:   chat.ID,
		Content:  []codersdk.ChatMessagePart{codersdk.ChatMessageText("do not persist")},
		APIKeyID: testAPIKeyID(t, db, user.ID),
	})
	require.NoError(t, err)
	require.True(t, result.Ended)
	require.True(t, result.Chat.Archived)
	require.Equal(t, database.ChatStatusWaiting, result.Chat.Status)
	messages, err := db.GetChatMessagesByChatID(ctx, database.GetChatMessagesByChatIDParams{ChatID: chat.ID})
	require.NoError(t, err)
	require.Empty(t, messages)
}

func TestSendMessageUserPromptSubmitQueue(t *testing.T) {
	t.Parallel()
	db, ps := dbtestutil.NewDB(t)
	ctx := testutil.Context(t, testutil.WaitLong)
	user, org, model := seedChatDependencies(t, db)
	chat, err := newTestServer(t, db, ps, uuid.New()).CreateChat(ctx, chatd.CreateOptions{
		OrganizationID:     org.ID,
		OwnerID:            user.ID,
		Title:              "queued hook",
		ModelConfigID:      model.ID,
		InitialUserContent: []codersdk.ChatMessagePart{codersdk.ChatMessageText("running")},
		APIKeyID:           testAPIKeyID(t, db, user.ID),
	})
	require.NoError(t, err)
	consumer := hookConsumer(t, `{"permission":{"decision":"allow","input_override":{"prompt":"queued override"}},"model_context":"queued context"}`)
	server := newHookTestServer(t, db, ps, consumer)

	result, err := server.SendMessage(ctx, chatd.SendMessageOptions{
		ChatID:       chat.ID,
		Content:      []codersdk.ChatMessagePart{codersdk.ChatMessageText("queued original")},
		APIKeyID:     testAPIKeyID(t, db, user.ID),
		BusyBehavior: chatd.SendMessageBusyBehaviorQueue,
	})
	require.NoError(t, err)
	require.True(t, result.Queued)
	require.NotNil(t, result.QueuedMessage)
	require.True(t, result.QueuedMessage.TurnID.Valid)
	queuedParts, err := chatprompt.ParseContent(database.ChatMessage{
		Role:           database.ChatMessageRoleUser,
		Content:        pqtype.NullRawMessage{RawMessage: result.QueuedMessage.Content, Valid: true},
		ContentVersion: chatprompt.CurrentContentVersion,
	})
	require.NoError(t, err)
	require.Equal(t, []codersdk.ChatMessagePart{codersdk.ChatMessageText("queued override")}, queuedParts)
	messages, err := db.GetChatMessagesForPromptByChatID(ctx, chat.ID)
	require.NoError(t, err)
	var prefixMessage *database.ChatMessage
	for i := range messages {
		if messages[i].TurnID == result.QueuedMessage.TurnID {
			prefixMessage = &messages[i]
			break
		}
	}
	require.NotNil(t, prefixMessage)
	require.Equal(t, database.ChatMessageVisibilityModel, prefixMessage.Visibility)
	require.Equal(t, "queued context", hookMessageText(t, *prefixMessage))
}

func TestSendMessageUserPromptSubmitQueuedRejections(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name       string
		statusCode int
		response   string
		assertErr  func(*testing.T, error)
	}{
		{
			name:       "deny",
			statusCode: http.StatusOK,
			response:   `{"permission":{"decision":"deny"},"user_message":"blocked"}`,
			assertErr: func(t *testing.T, err error) {
				var denied *chatd.UserPromptDeniedError
				require.ErrorAs(t, err, &denied)
			},
		},
		{
			name:       "dispatch failure",
			statusCode: http.StatusInternalServerError,
			assertErr: func(t *testing.T, err error) {
				var dispatchErr *chathooks.DispatchError
				require.ErrorAs(t, err, &dispatchErr)
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			db, ps := dbtestutil.NewDB(t)
			ctx := testutil.Context(t, testutil.WaitLong)
			user, org, model := seedChatDependencies(t, db)
			chat, err := newTestServer(t, db, ps, uuid.New()).CreateChat(ctx, chatd.CreateOptions{
				OrganizationID:     org.ID,
				OwnerID:            user.ID,
				Title:              "queued rejection",
				ModelConfigID:      model.ID,
				InitialUserContent: []codersdk.ChatMessagePart{codersdk.ChatMessageText("running")},
				APIKeyID:           testAPIKeyID(t, db, user.ID),
			})
			require.NoError(t, err)
			consumer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(test.statusCode)
				if test.response != "" {
					_, err := w.Write([]byte(test.response))
					require.NoError(t, err)
				}
			}))
			t.Cleanup(consumer.Close)
			server := newHookTestServer(t, db, ps, consumer)
			_, err = server.SendMessage(ctx, chatd.SendMessageOptions{
				ChatID:       chat.ID,
				Content:      []codersdk.ChatMessagePart{codersdk.ChatMessageText("queued")},
				APIKeyID:     testAPIKeyID(t, db, user.ID),
				BusyBehavior: chatd.SendMessageBusyBehaviorQueue,
			})
			test.assertErr(t, err)
			queued, err := db.GetChatQueuedMessages(ctx, chat.ID)
			require.NoError(t, err)
			require.Empty(t, queued)
			updated, err := db.GetChatByID(ctx, chat.ID)
			require.NoError(t, err)
			require.Equal(t, database.ChatStatusRunning, updated.Status)
			require.False(t, updated.LastError.Valid)
		})
	}
}

func TestSendMessageUserPromptSubmitDispatchFailure(t *testing.T) {
	t.Parallel()
	db, ps := dbtestutil.NewDB(t)
	ctx := testutil.Context(t, testutil.WaitLong)
	user, org, model := seedChatDependencies(t, db)
	chat := dbgen.Chat(t, db, database.Chat{
		OrganizationID:    org.ID,
		OwnerID:           user.ID,
		LastModelConfigID: model.ID,
	})
	consumer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	t.Cleanup(consumer.Close)
	server := newHookTestServer(t, db, ps, consumer)

	_, err := server.SendMessage(ctx, chatd.SendMessageOptions{
		ChatID:   chat.ID,
		Content:  []codersdk.ChatMessagePart{codersdk.ChatMessageText("fails")},
		APIKeyID: testAPIKeyID(t, db, user.ID),
	})
	var dispatchErr *chathooks.DispatchError
	require.ErrorAs(t, err, &dispatchErr)
	require.Equal(t, "http_error", dispatchErr.Class)
	updated, err := db.GetChatByID(ctx, chat.ID)
	require.NoError(t, err)
	require.Equal(t, database.ChatStatusError, updated.Status)
	var chatErr codersdk.ChatError
	require.NoError(t, json.Unmarshal(updated.LastError.RawMessage, &chatErr))
	require.Equal(t, "hook dispatch failed: user_prompt_submit: http_error (dispatch "+dispatchErr.DispatchID.String()+")", chatErr.Message)
}

func TestEditMessageUserPromptSubmitHook(t *testing.T) {
	t.Parallel()
	db, ps := dbtestutil.NewDB(t)
	ctx := testutil.Context(t, testutil.WaitLong)
	user, org, model := seedChatDependencies(t, db)
	chat := dbgen.Chat(t, db, database.Chat{
		OrganizationID:    org.ID,
		OwnerID:           user.ID,
		LastModelConfigID: model.ID,
	})
	apiKeyID := testAPIKeyID(t, db, user.ID)
	content, err := chatprompt.MarshalParts([]codersdk.ChatMessagePart{codersdk.ChatMessageText("original")})
	require.NoError(t, err)
	inserted, err := db.InsertChatMessages(ctx, chatd.BuildSingleUserChatMessageInsertParams(
		chat.ID, apiKeyID, content, database.ChatMessageVisibilityBoth, model.ID, chatprompt.CurrentContentVersion, user.ID,
	))
	require.NoError(t, err)
	require.Len(t, inserted, 1)
	type receivedHook struct {
		request agenthooks.Request
		claims  agenthooks.Claims
	}
	received := make([]receivedHook, 0, 2)
	consumer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request agenthooks.Request
		require.NoError(t, json.NewDecoder(r.Body).Decode(&request))
		claims, err := agenthooks.Verify(r.Header.Get("Authorization"), []byte("test-hook-secret-32-bytes-minimum!!"))
		require.NoError(t, err)
		received = append(received, receivedHook{request: request, claims: claims})
		response := `{"model_context":"clear context","user_message":"clear notice","allowed_tools":["read_file"]}`
		if request.Type == agenthooks.EventUserPromptSubmit {
			response = `{"permission":{"decision":"allow","input_override":{"prompt":"edited override"}}}`
		}
		_, err = w.Write([]byte(response))
		require.NoError(t, err)
	}))
	t.Cleanup(consumer.Close)
	server := newHookTestServer(t, db, ps, consumer)

	result, err := server.EditMessage(ctx, chatd.EditMessageOptions{
		ChatID:          chat.ID,
		CreatedBy:       user.ID,
		EditedMessageID: inserted[0].ID,
		Content:         []codersdk.ChatMessagePart{codersdk.ChatMessageText("edited original")},
		APIKeyID:        apiKeyID,
	})
	require.NoError(t, err)
	require.True(t, result.Message.TurnID.Valid)
	require.NotEqual(t, inserted[0].TurnID, result.Message.TurnID)
	require.Equal(t, "edited override", hookMessageText(t, result.Message))
	require.Len(t, received, 2)
	require.Equal(t, agenthooks.EventSessionStart, received[0].request.Type)
	data, err := received[0].request.Decode()
	require.NoError(t, err)
	require.Equal(t, &agenthooks.SessionStartData{Source: "clear"}, data)
	require.Equal(t, received[0].request.Meta.DispatchID, received[0].claims.JTI)
	require.Equal(t, agenthooks.EventUserPromptSubmit, received[1].request.Type)
	require.Equal(t, received[1].request.Meta.DispatchID, received[1].claims.JTI)
	require.NotEqual(t, received[0].claims.JTI, received[1].claims.JTI)
	rows, err := db.GetChatMessagesByChatID(ctx, database.GetChatMessagesByChatIDParams{ChatID: chat.ID})
	require.NoError(t, err)
	var foundNotice bool
	for _, row := range rows {
		if row.Role == database.ChatMessageRoleSystem && row.Visibility == database.ChatMessageVisibilityUser && hookMessageText(t, row) == "clear notice" {
			foundNotice = true
		}
	}
	require.True(t, foundNotice)
	promptRows, err := db.GetChatMessagesForPromptByChatID(ctx, chat.ID)
	require.NoError(t, err)
	var foundContext bool
	for _, row := range promptRows {
		if row.Visibility == database.ChatMessageVisibilityModel && hookMessageText(t, row) == "clear context" {
			foundContext = true
		}
	}
	require.True(t, foundContext)
	updated, err := db.GetChatByID(ctx, chat.ID)
	require.NoError(t, err)
	require.JSONEq(t, `["read_file"]`, string(updated.HookAllowedTools.RawMessage))
}

func TestSendMessageHooksDisabled(t *testing.T) {
	t.Parallel()
	db, ps := dbtestutil.NewDB(t)
	ctx := testutil.Context(t, testutil.WaitLong)
	user, org, model := seedChatDependencies(t, db)
	chat := dbgen.Chat(t, db, database.Chat{
		OrganizationID:    org.ID,
		OwnerID:           user.ID,
		LastModelConfigID: model.ID,
	})
	server := newTestServer(t, db, ps, uuid.New())
	result, err := server.SendMessage(ctx, chatd.SendMessageOptions{
		ChatID:   chat.ID,
		Content:  []codersdk.ChatMessagePart{codersdk.ChatMessageText("unchanged")},
		APIKeyID: testAPIKeyID(t, db, user.ID),
	})
	require.NoError(t, err)
	require.True(t, result.Message.TurnID.Valid)
	require.Equal(t, "unchanged", hookMessageText(t, result.Message))
}

func hookConsumer(t *testing.T, response string) *httptest.Server {
	t.Helper()
	consumer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, err := w.Write([]byte(response))
		require.NoError(t, err)
	}))
	t.Cleanup(consumer.Close)
	return consumer
}

func hookMessageText(t *testing.T, message database.ChatMessage) string {
	t.Helper()
	parts, err := chatprompt.ParseContent(message)
	require.NoError(t, err)
	require.Len(t, parts, 1)
	return parts[0].Text
}
