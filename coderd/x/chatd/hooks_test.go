package chatd_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"slices"
	"sync"
	"sync/atomic"
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
	"github.com/coder/coder/v2/coderd/x/agenthooks/dispatch"
	"github.com/coder/coder/v2/coderd/x/chatd"
	"github.com/coder/coder/v2/coderd/x/chatd/chathooks"
	"github.com/coder/coder/v2/coderd/x/chatd/chatprompt"
	"github.com/coder/coder/v2/coderd/x/chatd/chatstate"
	"github.com/coder/coder/v2/coderd/x/chatd/chattest"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/codersdk/x/agenthooks"
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

		submitted := []codersdk.ChatMessagePart{
			codersdk.ChatMessageText("before"),
			codersdk.ChatMessageFileReference("main.go", 1, 3, "package main"),
		}
		consumer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			var request agenthooks.Request
			require.NoError(t, json.NewDecoder(r.Body).Decode(&request))
			data := decodeHookData[agenthooks.UserPromptSubmitData](t, request)
			require.Equal(t, "before", data.Prompt)
			var hookParts []codersdk.ChatMessagePart
			require.NoError(t, json.Unmarshal(data.Parts, &hookParts))
			require.Equal(t, submitted, hookParts, "hook payload must carry non-text parts")
			require.NotNil(t, request.Meta.TurnID)
			_, err := w.Write([]byte(`{"permission":{"decision":"allow","input_override":{"prompt":"after"}},"model_context":"model only","user_message":"user only"}`))
			require.NoError(t, err)
		}))
		t.Cleanup(consumer.Close)

		server := newHookTestServer(t, db, ps, consumer)
		result, err := server.SendMessage(ctx, chatd.SendMessageOptions{
			ChatID:    chat.ID,
			CreatedBy: user.ID,
			Content:   submitted,
		})
		require.NoError(t, err)
		parts, err := chatprompt.ParseContent(result.Message)
		require.NoError(t, err)
		require.Equal(t, []codersdk.ChatMessagePart{
			codersdk.ChatMessageText("after"),
			codersdk.ChatMessageFileReference("main.go", 1, 3, "package main"),
			{Type: codersdk.ChatMessagePartTypeHookContext, Text: "model only"},
			{Type: codersdk.ChatMessagePartTypeHookNotice, Text: "user only"},
		}, parts)
		require.Len(t, result.InsertedMessages, 1)
		require.Equal(t, result.Message.ID, result.InsertedMessages[0].ID)
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
		})
		var denied *chathooks.UserPromptDeniedError
		require.ErrorAs(t, err, &denied)
		require.Equal(t, "blocked", denied.UserMessage)

		messages, err := db.GetChatMessagesByChatID(ctx, database.GetChatMessagesByChatIDParams{ChatID: chat.ID})
		require.NoError(t, err)
		require.Empty(t, messages)
	})
}

func newHookDispatcher(t *testing.T, _ database.Store, consumer *httptest.Server) *dispatch.Dispatcher {
	t.Helper()
	return dispatch.New(
		slogtest.Make(t, &slogtest.Options{IgnoreErrors: true}),
		consumer.Client(),
		consumer.URL,
		false,
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

func TestHookDispatcherRequiresExperiment(t *testing.T) {
	t.Parallel()
	db, ps := dbtestutil.NewDB(t)
	ctx := testutil.Context(t, testutil.WaitLong)
	user, org, model := seedChatDependencies(t, db)
	chat := dbgen.Chat(t, db, database.Chat{
		OrganizationID:    org.ID,
		OwnerID:           user.ID,
		LastModelConfigID: model.ID,
	})

	var hookRequests atomic.Int32
	consumer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hookRequests.Add(1)
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(consumer.Close)

	server := newTestServer(t, db, ps, uuid.New(), func(cfg *chatd.Config) {
		cfg.HookDispatcher = newHookDispatcher(t, db, consumer)
		cfg.Experiments = slices.DeleteFunc(
			slices.Clone(codersdk.ExperimentsKnown),
			func(e codersdk.Experiment) bool { return e == codersdk.ExperimentAgentLifecycleHooks },
		)
	})
	result, err := server.SendMessage(ctx, chatd.SendMessageOptions{
		ChatID:    chat.ID,
		CreatedBy: user.ID,
		Content:   []codersdk.ChatMessagePart{codersdk.ChatMessageText("hello")},
	})
	require.NoError(t, err)
	parts, err := chatprompt.ParseContent(result.Message)
	require.NoError(t, err)
	require.Equal(t, []codersdk.ChatMessagePart{codersdk.ChatMessageText("hello")}, parts)

	require.Zero(t, hookRequests.Load())
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
	var received agenthooks.Request
	consumer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.NoError(t, json.NewDecoder(r.Body).Decode(&received))
		_, err := w.Write([]byte(`{}`))
		require.NoError(t, err)
	}))
	t.Cleanup(consumer.Close)
	server := newHookTestServer(t, db, ps, consumer)
	result, err := server.SendMessage(ctx, chatd.SendMessageOptions{
		ChatID:  chat.ID,
		Content: []codersdk.ChatMessagePart{codersdk.ChatMessageText("passthrough")},
	})
	require.NoError(t, err)
	require.Equal(t, "passthrough", hookMessageText(t, result.Message))
	require.Equal(t, agenthooks.EventUserPromptSubmit, received.Type)
	promptData := decodeHookData[agenthooks.UserPromptSubmitData](t, received)
	require.Equal(t, "passthrough", promptData.Prompt)
	// The persisted content is jsonb-normalized, so compare JSON
	// semantics rather than raw bytes.
	require.JSONEq(t, string(result.Message.Content.RawMessage), string(promptData.Parts))
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
	})
	require.NoError(t, err)
	var received agenthooks.Request
	consumer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.NoError(t, json.NewDecoder(r.Body).Decode(&received))
		_, err := w.Write([]byte(`{"permission":{"decision":"allow","input_override":{"prompt":"queued override"}},"model_context":"queued context","user_message":"queued notice"}`))
		require.NoError(t, err)
	}))
	t.Cleanup(consumer.Close)
	server := newHookTestServer(t, db, ps, consumer)

	result, err := server.SendMessage(ctx, chatd.SendMessageOptions{
		ChatID:       chat.ID,
		Content:      []codersdk.ChatMessagePart{codersdk.ChatMessageText("queued original")},
		BusyBehavior: chatd.SendMessageBusyBehaviorQueue,
	})
	require.NoError(t, err)
	require.True(t, result.Queued)
	require.NotNil(t, result.QueuedMessage)
	queuedParts, err := chatprompt.ParseContent(database.ChatMessage{
		Role:           database.ChatMessageRoleUser,
		Content:        pqtype.NullRawMessage{RawMessage: result.QueuedMessage.Content, Valid: true},
		ContentVersion: chatprompt.CurrentContentVersion,
	})
	require.NoError(t, err)
	wantQueuedParts := []codersdk.ChatMessagePart{
		codersdk.ChatMessageText("queued override"),
		{Type: codersdk.ChatMessagePartTypeHookContext, Text: "queued context"},
		{Type: codersdk.ChatMessagePartTypeHookNotice, Text: "queued notice"},
	}
	require.Equal(t, wantQueuedParts, queuedParts)
	queued, err := db.GetChatQueuedMessages(ctx, chat.ID)
	require.NoError(t, err)
	require.Len(t, queued, 1)
	persistedParts, err := chatprompt.ParseContent(database.ChatMessage{
		Role:           database.ChatMessageRoleUser,
		Content:        pqtype.NullRawMessage{RawMessage: queued[0].Content, Valid: true},
		ContentVersion: chatprompt.CurrentContentVersion,
	})
	require.NoError(t, err)
	require.Equal(t, wantQueuedParts, persistedParts)
	require.Equal(t, agenthooks.EventUserPromptSubmit, received.Type)
	require.Equal(t, "queued original", decodeHookData[agenthooks.UserPromptSubmitData](t, received).Prompt)
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
				var denied *chathooks.UserPromptDeniedError
				require.ErrorAs(t, err, &denied)
			},
		},
		{
			name:       "dispatch failure",
			statusCode: http.StatusInternalServerError,
			assertErr: func(t *testing.T, err error) {
				var dispatchErr *dispatch.Error
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

func TestSubagentSpawnHookDispatchFailureFailsTurn(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitLong)
	db, ps := dbtestutil.NewDB(t)
	openAIURL := chattest.NewOpenAI(t, func(req *chattest.OpenAIRequest) chattest.OpenAIResponse {
		if !req.Stream {
			return chattest.OpenAINonStreamingResponse("title")
		}
		chunk := chattest.OpenAIToolCallChunk("spawn_agent", `{"type":"general","prompt":"child admission prompt"}`)
		chunk.Choices[0].ToolCalls[0].ID = "call_spawn"
		return chattest.OpenAIStreamingResponse(chunk)
	})
	user, org, model := seedChatDependenciesWithProvider(t, db, "openai-compat", openAIURL)

	consumer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request agenthooks.Request
		require.NoError(t, json.NewDecoder(r.Body).Decode(&request))
		if request.Type == agenthooks.EventUserPromptSubmit {
			data := decodeHookData[agenthooks.UserPromptSubmitData](t, request)
			if data.Prompt == "child admission prompt" {
				w.WriteHeader(http.StatusInternalServerError)
				return
			}
		}
		_, err := w.Write([]byte(`{}`))
		require.NoError(t, err)
	}))
	t.Cleanup(consumer.Close)

	server := newActiveTestServer(t, db, ps, func(cfg *chatd.Config) {
		cfg.AIBridgeTransportFactory = chatAIGatewayTransportFactoryPointer(chattest.NewMockAIBridgeTransport(t, openAIURL))
		cfg.HookDispatcher = newHookDispatcher(t, db, consumer)
	})
	chat, err := server.CreateChat(ctx, chatd.CreateOptions{
		OrganizationID: org.ID,
		OwnerID:        user.ID,
		Title:          "spawn-hook-failure",
		ModelConfigID:  model.ID,
		InitialUserContent: []codersdk.ChatMessagePart{
			codersdk.ChatMessageText("spawn a child"),
		},
	})
	require.NoError(t, err)
	failed := waitForChatStatus(ctx, t, db, chat.ID, database.ChatStatusError)
	require.Contains(t, chatLastErrorMessage(failed.LastError), "hook dispatch failed: user_prompt_submit: http_error")

	messages := chatMessages(ctx, t, db, chat.ID)
	require.Len(t, messages, 3)
	require.Equal(t, database.ChatMessageRoleUser, messages[0].Role)
	require.Equal(t, database.ChatMessageRoleAssistant, messages[1].Role)
	require.Equal(t, database.ChatMessageRoleTool, messages[2].Role)
	require.Contains(t, string(messages[2].Content.RawMessage), "lifecycle hook returned HTTP status 500")

	chats, err := db.GetChats(ctx, database.GetChatsParams{
		OwnedOnly: true,
		ViewerID:  user.ID,
		AfterID:   uuid.Nil,
		OffsetOpt: 0,
		LimitOpt:  100,
	})
	require.NoError(t, err)
	require.Len(t, chats, 1)
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
	var received agenthooks.Request
	consumer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.NoError(t, json.NewDecoder(r.Body).Decode(&received))
		w.WriteHeader(http.StatusInternalServerError)
	}))
	t.Cleanup(consumer.Close)
	server := newHookTestServer(t, db, ps, consumer)

	_, err := server.SendMessage(ctx, chatd.SendMessageOptions{
		ChatID:  chat.ID,
		Content: []codersdk.ChatMessagePart{codersdk.ChatMessageText("fails")},
	})
	var dispatchErr *dispatch.Error
	require.ErrorAs(t, err, &dispatchErr)
	require.Equal(t, dispatch.ResultHTTPError, dispatchErr.Class)
	updated, err := db.GetChatByID(ctx, chat.ID)
	require.NoError(t, err)
	require.Equal(t, database.ChatStatusError, updated.Status)
	var chatErr codersdk.ChatError
	require.NoError(t, json.Unmarshal(updated.LastError.RawMessage, &chatErr))
	require.Equal(t, "hook dispatch failed: user_prompt_submit: http_error (dispatch "+dispatchErr.DispatchID.String()+")", chatErr.Message)
	require.Equal(t, agenthooks.EventUserPromptSubmit, received.Type)
	prompt := decodeHookData[agenthooks.UserPromptSubmitData](t, received)
	require.Equal(t, "fails", prompt.Prompt)
	messages, err := db.GetChatMessagesByChatID(ctx, database.GetChatMessagesByChatIDParams{ChatID: chat.ID})
	require.NoError(t, err)
	require.Empty(t, messages)
	queued, err := db.GetChatQueuedMessages(ctx, chat.ID)
	require.NoError(t, err)
	require.Empty(t, queued)
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
	content, err := chatprompt.MarshalParts([]codersdk.ChatMessagePart{codersdk.ChatMessageText("original")})
	require.NoError(t, err)
	inserted, err := db.InsertChatMessages(ctx, chatd.BuildSingleChatMessageInsertParams(
		chat.ID, database.ChatMessageRoleUser, content, database.ChatMessageVisibilityBoth, model.ID, chatprompt.CurrentContentVersion, user.ID,
	))
	require.NoError(t, err)
	require.Len(t, inserted, 1)
	type receivedHook struct {
		request agenthooks.Request
		claims  agenthooks.Claims
	}
	var receivedMu sync.Mutex
	received := make([]receivedHook, 0, 2)
	consumer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request agenthooks.Request
		require.NoError(t, json.NewDecoder(r.Body).Decode(&request))
		claims, err := agenthooks.Verify(r.Header.Get("Authorization"), []byte("test-hook-secret-32-bytes-minimum!!"))
		require.NoError(t, err)
		receivedMu.Lock()
		received = append(received, receivedHook{request: request, claims: claims})
		receivedMu.Unlock()
		response := `{"model_context":"clear context","user_message":"clear notice"}`
		if request.Type == agenthooks.EventUserPromptSubmit {
			response = `{"permission":{"decision":"allow","input_override":{"prompt":"edited override"}},"model_context":"edit context","user_message":"edit notice"}`
		}
		_, err = w.Write([]byte(response))
		require.NoError(t, err)
	}))
	t.Cleanup(consumer.Close)
	server := newHookTestServer(t, db, ps, consumer)

	upload := codersdk.ChatMessageFile(uuid.New(), "image/png", "edited.png")
	reference := codersdk.ChatMessageFileReference("main.go", 1, 3, "package main")
	result, err := server.EditMessage(ctx, chatd.EditMessageOptions{
		ChatID:          chat.ID,
		CreatedBy:       user.ID,
		EditedMessageID: inserted[0].ID,
		Content: []codersdk.ChatMessagePart{
			reference,
			codersdk.ChatMessageText("edited original"),
			upload,
		},
	})
	require.NoError(t, err)
	parts, err := chatprompt.ParseContent(result.Message)
	require.NoError(t, err)
	require.Equal(t, []codersdk.ChatMessagePart{
		reference,
		codersdk.ChatMessageText("edited override"),
		upload,
		{Type: codersdk.ChatMessagePartTypeHookContext, Text: "edit context"},
		{Type: codersdk.ChatMessagePartTypeHookNotice, Text: "edit notice"},
	}, parts)
	receivedMu.Lock()
	received = slices.Clone(received)
	receivedMu.Unlock()
	require.Len(t, received, 2)
	require.Equal(t, agenthooks.EventSessionStart, received[0].request.Type)
	require.Equal(t, agenthooks.SessionStartData{Source: "clear"}, decodeHookData[agenthooks.SessionStartData](t, received[0].request))
	require.Equal(t, received[0].request.Meta.DispatchID, received[0].claims.JTI)
	require.Equal(t, agenthooks.EventUserPromptSubmit, received[1].request.Type)
	prompt := decodeHookData[agenthooks.UserPromptSubmitData](t, received[1].request)
	require.Equal(t, "edited original", prompt.Prompt)
	require.NotNil(t, received[0].request.Meta.TurnID)
	require.Equal(t, received[0].request.Meta.TurnID, received[1].request.Meta.TurnID)
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
}

func TestEditMessageInvalidTargetSkipsHooks(t *testing.T) {
	t.Parallel()
	db, ps := dbtestutil.NewDB(t)
	ctx := testutil.Context(t, testutil.WaitLong)
	user, org, model := seedChatDependencies(t, db)
	chat := dbgen.Chat(t, db, database.Chat{
		OrganizationID:    org.ID,
		OwnerID:           user.ID,
		LastModelConfigID: model.ID,
	})
	var dispatched atomic.Int32
	consumer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		dispatched.Add(1)
		_, err := w.Write([]byte(`{}`))
		require.NoError(t, err)
	}))
	t.Cleanup(consumer.Close)
	server := newHookTestServer(t, db, ps, consumer)

	_, err := server.EditMessage(ctx, chatd.EditMessageOptions{
		ChatID:          chat.ID,
		CreatedBy:       user.ID,
		EditedMessageID: 999999,
		Content:         []codersdk.ChatMessagePart{codersdk.ChatMessageText("edit of nothing")},
	})
	require.ErrorIs(t, err, chatd.ErrEditedMessageNotFound)

	// dbgen.Chat ignores seed.Archived; archive explicitly.
	archived := dbgen.Chat(t, db, database.Chat{
		OrganizationID:    org.ID,
		OwnerID:           user.ID,
		LastModelConfigID: model.ID,
	})
	_, err = db.ArchiveChatByID(ctx, archived.ID)
	require.NoError(t, err)
	_, err = server.SendMessage(ctx, chatd.SendMessageOptions{
		ChatID:  archived.ID,
		Content: []codersdk.ChatMessagePart{codersdk.ChatMessageText("send to archived")},
	})
	require.ErrorIs(t, err, chatd.ErrChatArchived)
	_, err = server.EditMessage(ctx, chatd.EditMessageOptions{
		ChatID:          archived.ID,
		CreatedBy:       user.ID,
		EditedMessageID: 1,
		Content:         []codersdk.ChatMessagePart{codersdk.ChatMessageText("edit archived")},
	})
	require.ErrorIs(t, err, chatd.ErrChatArchived)

	require.Zero(t, dispatched.Load(), "invalid targets must not dispatch hooks")
}

func TestPromptHooksAdmissionPreflight(t *testing.T) {
	t.Parallel()
	db, ps := dbtestutil.NewDB(t)
	ctx := testutil.Context(t, testutil.WaitLong)
	user, org, model := seedChatDependencies(t, db)
	received := make(chan agenthooks.Request, 8)
	consumer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request agenthooks.Request
		require.NoError(t, json.NewDecoder(r.Body).Decode(&request))
		received <- request
		_, err := w.Write([]byte(`{}`))
		require.NoError(t, err)
	}))
	t.Cleanup(consumer.Close)
	server := newHookTestServer(t, db, ps, consumer)

	chat := dbgen.Chat(t, db, database.Chat{
		OrganizationID:    org.ID,
		OwnerID:           user.ID,
		LastModelConfigID: model.ID,
	})
	_, err := server.SendMessage(ctx, chatd.SendMessageOptions{
		ChatID:        chat.ID,
		Content:       []codersdk.ChatMessagePart{codersdk.ChatMessageText("bad model")},
		ModelConfigID: uuid.New(),
	})
	require.ErrorIs(t, err, chatd.ErrInvalidModelConfigID)

	content, err := chatprompt.MarshalParts([]codersdk.ChatMessagePart{codersdk.ChatMessageText("original")})
	require.NoError(t, err)
	inserted, err := db.InsertChatMessages(ctx, chatd.BuildSingleChatMessageInsertParams(
		chat.ID, database.ChatMessageRoleUser, content, database.ChatMessageVisibilityBoth, model.ID, chatprompt.CurrentContentVersion, user.ID,
	))
	require.NoError(t, err)
	require.Len(t, inserted, 1)
	_, err = server.EditMessage(ctx, chatd.EditMessageOptions{
		ChatID:          chat.ID,
		CreatedBy:       user.ID,
		EditedMessageID: inserted[0].ID,
		Content:         []codersdk.ChatMessagePart{codersdk.ChatMessageText("bad model edit")},
		ModelConfigID:   uuid.New(),
	})
	require.ErrorIs(t, err, chatd.ErrInvalidModelConfigID)

	busy := dbgen.Chat(t, db, database.Chat{
		OrganizationID:    org.ID,
		OwnerID:           user.ID,
		LastModelConfigID: model.ID,
		Status:            database.ChatStatusRunning,
	})
	queuedContent, err := chatprompt.MarshalParts([]codersdk.ChatMessagePart{codersdk.ChatMessageText("queued")})
	require.NoError(t, err)
	for range chatstate.MaxQueueSize {
		_, err = db.InsertChatQueuedMessageWithCreator(ctx, database.InsertChatQueuedMessageWithCreatorParams{
			ChatID:        busy.ID,
			Content:       queuedContent.RawMessage,
			ModelConfigID: uuid.NullUUID{UUID: model.ID, Valid: true},
			CreatedBy:     user.ID,
		})
		require.NoError(t, err)
	}
	_, err = server.SendMessage(ctx, chatd.SendMessageOptions{
		ChatID:  busy.ID,
		Content: []codersdk.ChatMessagePart{codersdk.ChatMessageText("queue full")},
	})
	require.ErrorIs(t, err, chatstate.ErrMessageQueueFull)

	select {
	case request := <-received:
		t.Fatalf("admission-rejected prompt dispatched %s", request.Type)
	default:
	}
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
		ChatID:  chat.ID,
		Content: []codersdk.ChatMessagePart{codersdk.ChatMessageText("unchanged")},
	})
	require.NoError(t, err)
	require.Equal(t, "unchanged", hookMessageText(t, result.Message))
}

func hookMessageText(t *testing.T, message database.ChatMessage) string {
	t.Helper()
	parts, err := chatprompt.ParseContent(message)
	require.NoError(t, err)
	require.Len(t, parts, 1)
	return parts[0].Text
}

func decodeHookData[T any](t *testing.T, request agenthooks.Request) T {
	t.Helper()
	var data T
	require.NoError(t, json.Unmarshal(request.Data, &data))
	return data
}
