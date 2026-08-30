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
	"github.com/coder/coder/v2/coderd/database/dbauthz"
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

	chatFile, err := db.InsertChatFile(ctx, database.InsertChatFileParams{
		OwnerID:        user.ID,
		OrganizationID: org.ID,
		Name:           "edited.png",
		Mimetype:       "image/png",
		Data:           []byte("png-bytes"),
	})
	require.NoError(t, err)
	upload := codersdk.ChatMessageFile(chatFile.ID, chatFile.Mimetype, chatFile.Name)
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
	linkedFiles, err := db.GetChatFileMetadataByChatID(ctx, chat.ID)
	require.NoError(t, err)
	require.Len(t, linkedFiles, 1)
	require.Equal(t, chatFile.ID, linkedFiles[0].ID)

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

// The goal objective bypasses message content, so user_prompt_submit
// must observe it and its replacement must be persisted (create path).
func TestCreateChatGoalObjectiveHookPayloadAndOverride(t *testing.T) {
	t.Parallel()
	db, ps := dbtestutil.NewDB(t)
	ctx := testutil.Context(t, testutil.WaitLong)
	user, org, model := seedChatDependencies(t, db)
	var received agenthooks.Request
	consumer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request agenthooks.Request
		require.NoError(t, json.NewDecoder(r.Body).Decode(&request))
		response := `{}`
		if request.Type == agenthooks.EventUserPromptSubmit {
			received = request
			response = `{"permission":{"decision":"allow","input_override":{"goal_objective":"  scrubbed objective  "}}}`
		}
		_, err := w.Write([]byte(response))
		require.NoError(t, err)
	}))
	t.Cleanup(consumer.Close)
	server := newHookTestServer(t, db, ps, consumer)

	chat, err := server.CreateChat(ctx, chatd.CreateOptions{
		OrganizationID:     org.ID,
		OwnerID:            user.ID,
		Title:              "goal create",
		ModelConfigID:      model.ID,
		InitialUserContent: []codersdk.ChatMessagePart{codersdk.ChatMessageText("kick off")},
		GoalMutation: &codersdk.ChatGoalMutation{
			Action:    codersdk.ChatGoalMutationActionSet,
			Objective: "original objective",
		},
	})
	require.NoError(t, err)
	promptData := decodeHookData[agenthooks.UserPromptSubmitData](t, received)
	require.Equal(t, "original objective", promptData.GoalObjective)
	require.Equal(t, "kick off", promptData.Prompt)
	goals, err := db.GetCurrentChatGoalsByRootChatIDs(ctx, []uuid.UUID{chat.ID})
	require.NoError(t, err)
	require.Len(t, goals, 1)
	require.Equal(t, "scrubbed objective", goals[0].Objective)
}

// Same contract on the send path: the hook payload carries the
// objective and the override replaces it without touching the message.
func TestSendMessageGoalObjectiveHookPayloadAndOverride(t *testing.T) {
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
		_, err := w.Write([]byte(`{"permission":{"decision":"allow","input_override":{"goal_objective":"scrubbed objective"}}}`))
		require.NoError(t, err)
	}))
	t.Cleanup(consumer.Close)
	server := newHookTestServer(t, db, ps, consumer)

	result, err := server.SendMessage(ctx, chatd.SendMessageOptions{
		ChatID:  chat.ID,
		Content: []codersdk.ChatMessagePart{codersdk.ChatMessageText("pursue this")},
		GoalMutation: &codersdk.ChatGoalMutation{
			Action:    codersdk.ChatGoalMutationActionSet,
			Objective: "original objective",
		},
	})
	require.NoError(t, err)
	promptData := decodeHookData[agenthooks.UserPromptSubmitData](t, received)
	require.Equal(t, "original objective", promptData.GoalObjective)
	require.Equal(t, "pursue this", promptData.Prompt)
	require.NotNil(t, result.Goal)
	require.Equal(t, "scrubbed objective", result.Goal.Objective)
	require.Equal(t, "pursue this", hookMessageText(t, result.Message))
}

// An objective override on a submission that sets no goal is a
// consumer bug: the send is rejected and nothing is persisted.
func TestSendMessageGoalObjectiveOverrideWithoutGoalRejected(t *testing.T) {
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
		_, err := w.Write([]byte(`{"permission":{"decision":"allow","input_override":{"goal_objective":"sneaky objective"}}}`))
		require.NoError(t, err)
	}))
	t.Cleanup(consumer.Close)
	server := newHookTestServer(t, db, ps, consumer)

	_, err := server.SendMessage(ctx, chatd.SendMessageOptions{
		ChatID:  chat.ID,
		Content: []codersdk.ChatMessagePart{codersdk.ChatMessageText("no goal here")},
	})
	require.ErrorIs(t, err, chatd.ErrChatGoalInvalidMutation)
	messages, err := db.GetChatMessagesByChatID(ctx, database.GetChatMessagesByChatIDParams{ChatID: chat.ID})
	require.NoError(t, err)
	require.Empty(t, messages)
}

// A known-busy goal send is inadmissible; hook consumers must never
// observe its prompt.
func TestSendMessageGoalBusyRejectedBeforeHookDispatch(t *testing.T) {
	t.Parallel()
	db, ps := dbtestutil.NewDB(t)
	ctx := testutil.Context(t, testutil.WaitLong)
	user, org, model := seedChatDependencies(t, db)
	var promptDispatches atomic.Int32
	consumer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request agenthooks.Request
		require.NoError(t, json.NewDecoder(r.Body).Decode(&request))
		if request.Type == agenthooks.EventUserPromptSubmit {
			promptDispatches.Add(1)
		}
		_, err := w.Write([]byte(`{}`))
		require.NoError(t, err)
	}))
	t.Cleanup(consumer.Close)
	server := newHookTestServer(t, db, ps, consumer)

	chat, err := server.CreateChat(ctx, chatd.CreateOptions{
		OrganizationID:     org.ID,
		OwnerID:            user.ID,
		Title:              "busy goal",
		ModelConfigID:      model.ID,
		InitialUserContent: []codersdk.ChatMessagePart{codersdk.ChatMessageText("running")},
	})
	require.NoError(t, err)
	require.Equal(t, database.ChatStatusRunning, chat.Status)
	require.Equal(t, int32(1), promptDispatches.Load())

	_, err = server.SendMessage(ctx, chatd.SendMessageOptions{
		ChatID:  chat.ID,
		Content: []codersdk.ChatMessagePart{codersdk.ChatMessageText("pursue this")},
		GoalMutation: &codersdk.ChatGoalMutation{
			Action:    codersdk.ChatGoalMutationActionSet,
			Objective: "goal on a busy chat",
		},
	})
	require.ErrorIs(t, err, chatd.ErrChatGoalBusy)
	require.Equal(t, int32(1), promptDispatches.Load(),
		"user_prompt_submit must not be dispatched for an inadmissible goal send")
}

// A child-chat goal send can never commit because goals are root-only;
// hook consumers must never observe its prompt.
func TestSendMessageGoalChildChatRejectedBeforeHookDispatch(t *testing.T) {
	t.Parallel()
	db, ps := dbtestutil.NewDB(t)
	ctx := testutil.Context(t, testutil.WaitLong)
	user, org, model := seedChatDependencies(t, db)
	var promptDispatches atomic.Int32
	consumer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request agenthooks.Request
		require.NoError(t, json.NewDecoder(r.Body).Decode(&request))
		if request.Type == agenthooks.EventUserPromptSubmit {
			promptDispatches.Add(1)
		}
		_, err := w.Write([]byte(`{}`))
		require.NoError(t, err)
	}))
	t.Cleanup(consumer.Close)
	server := newHookTestServer(t, db, ps, consumer)

	root, err := server.CreateChat(ctx, chatd.CreateOptions{
		OrganizationID:     org.ID,
		OwnerID:            user.ID,
		Title:              "root chat",
		ModelConfigID:      model.ID,
		InitialUserContent: []codersdk.ChatMessagePart{codersdk.ChatMessageText("root start")},
	})
	require.NoError(t, err)
	require.Equal(t, int32(1), promptDispatches.Load())

	childContent, err := chatprompt.MarshalParts([]codersdk.ChatMessagePart{
		codersdk.ChatMessageText("child question"),
	})
	require.NoError(t, err)
	createdChild, err := chatstate.CreateChat(ctx, db, ps, chatstate.CreateChatInput{
		OrganizationID:    org.ID,
		OwnerID:           user.ID,
		ParentChatID:      uuid.NullUUID{UUID: root.ID, Valid: true},
		RootChatID:        uuid.NullUUID{UUID: root.ID, Valid: true},
		LastModelConfigID: model.ID,
		Title:             "child chat",
		ClientType:        database.ChatClientTypeApi,
		InitialMessages: []chatstate.Message{
			{
				Role:           database.ChatMessageRoleUser,
				Content:        childContent,
				Visibility:     database.ChatMessageVisibilityBoth,
				ContentVersion: chatprompt.CurrentContentVersion,
				CreatedBy:      uuid.NullUUID{UUID: user.ID, Valid: true},
				ModelConfigID:  uuid.NullUUID{UUID: model.ID, Valid: true},
			},
		},
	})
	require.NoError(t, err)
	// Waiting status passes the busy pre-check, so only the root-only
	// guard can reject before dispatch.
	_, err = db.UpdateChatStatus(dbauthz.AsSystemRestricted(ctx), database.UpdateChatStatusParams{
		ID:     createdChild.Chat.ID,
		Status: database.ChatStatusWaiting,
	})
	require.NoError(t, err)

	_, err = server.SendMessage(ctx, chatd.SendMessageOptions{
		ChatID:    createdChild.Chat.ID,
		CreatedBy: user.ID,
		Content:   []codersdk.ChatMessagePart{codersdk.ChatMessageText("pursue this")},
		GoalMutation: &codersdk.ChatGoalMutation{
			Action:    codersdk.ChatGoalMutationActionSet,
			Objective: "goal on a child chat",
		},
	})
	require.ErrorIs(t, err, chatd.ErrChatGoalNotRoot)
	require.Equal(t, int32(1), promptDispatches.Load(),
		"user_prompt_submit must not be dispatched for a child-chat goal send")
}

func TestSendMessageGoalPlanModeRejectedBeforeHookDispatch(t *testing.T) {
	t.Parallel()

	planOn := database.NullChatPlanMode{ChatPlanMode: database.ChatPlanModePlan, Valid: true}
	goalMutation := &codersdk.ChatGoalMutation{
		Action:    codersdk.ChatGoalMutationActionSet,
		Objective: "goal under plan mode",
	}
	newPromptCounter := func(t *testing.T) (*httptest.Server, *atomic.Int32) {
		t.Helper()
		var promptDispatches atomic.Int32
		consumer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			var request agenthooks.Request
			require.NoError(t, json.NewDecoder(r.Body).Decode(&request))
			if request.Type == agenthooks.EventUserPromptSubmit {
				promptDispatches.Add(1)
			}
			_, err := w.Write([]byte(`{}`))
			require.NoError(t, err)
		}))
		t.Cleanup(consumer.Close)
		return consumer, &promptDispatches
	}

	t.Run("InheritedPlanMode", func(t *testing.T) {
		t.Parallel()
		db, ps := dbtestutil.NewDB(t)
		ctx := testutil.Context(t, testutil.WaitLong)
		user, org, model := seedChatDependencies(t, db)
		consumer, promptDispatches := newPromptCounter(t)
		server := newHookTestServer(t, db, ps, consumer)
		chat := dbgen.Chat(t, db, database.Chat{
			OrganizationID:    org.ID,
			OwnerID:           user.ID,
			LastModelConfigID: model.ID,
			PlanMode:          planOn,
		})

		_, err := server.SendMessage(ctx, chatd.SendMessageOptions{
			ChatID:       chat.ID,
			Content:      []codersdk.ChatMessagePart{codersdk.ChatMessageText("pursue this")},
			GoalMutation: goalMutation,
		})
		require.ErrorIs(t, err, chatd.ErrChatGoalPlanMode)
		require.Equal(t, int32(0), promptDispatches.Load(),
			"user_prompt_submit must not be dispatched for an inadmissible goal send")
	})

	t.Run("RequestedPlanMode", func(t *testing.T) {
		t.Parallel()
		db, ps := dbtestutil.NewDB(t)
		ctx := testutil.Context(t, testutil.WaitLong)
		user, org, model := seedChatDependencies(t, db)
		consumer, promptDispatches := newPromptCounter(t)
		server := newHookTestServer(t, db, ps, consumer)
		chat := dbgen.Chat(t, db, database.Chat{
			OrganizationID:    org.ID,
			OwnerID:           user.ID,
			LastModelConfigID: model.ID,
		})

		_, err := server.SendMessage(ctx, chatd.SendMessageOptions{
			ChatID:       chat.ID,
			Content:      []codersdk.ChatMessagePart{codersdk.ChatMessageText("pursue this")},
			PlanMode:     &planOn,
			GoalMutation: goalMutation,
		})
		require.ErrorIs(t, err, chatd.ErrChatGoalPlanMode)
		require.Equal(t, int32(0), promptDispatches.Load(),
			"user_prompt_submit must not be dispatched for an inadmissible goal send")
	})
}

// Plan-mode turns exclude goal completion behavior, so goal sets and
// plan mode are mutually exclusive at the API layer, matching the
// composer's mutually exclusive controls.
func TestGoalSetPlanModeRejected(t *testing.T) {
	t.Parallel()

	planOn := database.NullChatPlanMode{ChatPlanMode: database.ChatPlanModePlan, Valid: true}
	goalMutation := &codersdk.ChatGoalMutation{
		Action:    codersdk.ChatGoalMutationActionSet,
		Objective: "objective under plan mode",
	}

	t.Run("CreateRejected", func(t *testing.T) {
		t.Parallel()
		db, ps := dbtestutil.NewDB(t)
		ctx := testutil.Context(t, testutil.WaitLong)
		user, org, model := seedChatDependencies(t, db)
		server := newTestServer(t, db, ps, uuid.New())

		_, err := server.CreateChat(ctx, chatd.CreateOptions{
			OrganizationID:     org.ID,
			OwnerID:            user.ID,
			Title:              "plan mode goal",
			ModelConfigID:      model.ID,
			PlanMode:           planOn,
			GoalMutation:       goalMutation,
			InitialUserContent: []codersdk.ChatMessagePart{codersdk.ChatMessageText("start")},
		})
		require.ErrorIs(t, err, chatd.ErrChatGoalPlanMode)
	})

	t.Run("SendOnPlanModeChatRejected", func(t *testing.T) {
		t.Parallel()
		db, ps := dbtestutil.NewDB(t)
		ctx := testutil.Context(t, testutil.WaitLong)
		user, org, model := seedChatDependencies(t, db)
		server := newTestServer(t, db, ps, uuid.New())
		chat := dbgen.Chat(t, db, database.Chat{
			OrganizationID:    org.ID,
			OwnerID:           user.ID,
			LastModelConfigID: model.ID,
			PlanMode:          planOn,
		})

		_, err := server.SendMessage(ctx, chatd.SendMessageOptions{
			ChatID:       chat.ID,
			Content:      []codersdk.ChatMessagePart{codersdk.ChatMessageText("pursue")},
			GoalMutation: goalMutation,
		})
		require.ErrorIs(t, err, chatd.ErrChatGoalPlanMode)
	})

	t.Run("SendEnablingPlanModeRejected", func(t *testing.T) {
		t.Parallel()
		db, ps := dbtestutil.NewDB(t)
		ctx := testutil.Context(t, testutil.WaitLong)
		user, org, model := seedChatDependencies(t, db)
		server := newTestServer(t, db, ps, uuid.New())
		chat := dbgen.Chat(t, db, database.Chat{
			OrganizationID:    org.ID,
			OwnerID:           user.ID,
			LastModelConfigID: model.ID,
		})

		requested := planOn
		_, err := server.SendMessage(ctx, chatd.SendMessageOptions{
			ChatID:       chat.ID,
			Content:      []codersdk.ChatMessagePart{codersdk.ChatMessageText("pursue")},
			PlanMode:     &requested,
			GoalMutation: goalMutation,
		})
		require.ErrorIs(t, err, chatd.ErrChatGoalPlanMode)
	})

	t.Run("SendDisablingPlanModeAllowed", func(t *testing.T) {
		t.Parallel()
		db, ps := dbtestutil.NewDB(t)
		ctx := testutil.Context(t, testutil.WaitLong)
		user, org, model := seedChatDependencies(t, db)
		server := newTestServer(t, db, ps, uuid.New())
		chat := dbgen.Chat(t, db, database.Chat{
			OrganizationID:    org.ID,
			OwnerID:           user.ID,
			LastModelConfigID: model.ID,
			PlanMode:          planOn,
		})

		cleared := database.NullChatPlanMode{}
		result, err := server.SendMessage(ctx, chatd.SendMessageOptions{
			ChatID:       chat.ID,
			Content:      []codersdk.ChatMessagePart{codersdk.ChatMessageText("pursue")},
			PlanMode:     &cleared,
			GoalMutation: goalMutation,
		})
		require.NoError(t, err)
		require.NotNil(t, result.Goal)
	})
}

// chatstate admits direct sends on waiting and empty-queue error
// states, so a goal-bound recovery send on an idle errored chat must
// reach hook dispatch instead of being rejected as busy.
func TestSendMessageGoalOnErroredChatDispatchesHook(t *testing.T) {
	t.Parallel()
	db, ps := dbtestutil.NewDB(t)
	ctx := testutil.Context(t, testutil.WaitLong)
	user, org, model := seedChatDependencies(t, db)
	chat := dbgen.Chat(t, db, database.Chat{
		OrganizationID:    org.ID,
		OwnerID:           user.ID,
		LastModelConfigID: model.ID,
		Status:            database.ChatStatusError,
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
		Content: []codersdk.ChatMessagePart{codersdk.ChatMessageText("recover with a goal")},
		GoalMutation: &codersdk.ChatGoalMutation{
			Action:    codersdk.ChatGoalMutationActionSet,
			Objective: "recovery objective",
		},
	})
	require.NoError(t, err)
	require.False(t, result.Queued)
	require.NotNil(t, result.Goal)
	require.Equal(t, "recovery objective", result.Goal.Objective)
	require.Equal(t, agenthooks.EventUserPromptSubmit, received.Type)
	require.Equal(t, "recovery objective", decodeHookData[agenthooks.UserPromptSubmitData](t, received).GoalObjective)
}

// The source message anchors the current goal's objective; editing it
// would start a turn from new text while the goal pursues the old
// objective, so the edit is refused until the goal can no longer run.
func TestEditMessageGoalSourceRejected(t *testing.T) {
	t.Parallel()
	db, ps := dbtestutil.NewDB(t)
	ctx := testutil.Context(t, testutil.WaitLong)
	user, org, model := seedChatDependencies(t, db)
	server := newTestServer(t, db, ps, uuid.New())

	chat, err := server.CreateChat(ctx, chatd.CreateOptions{
		OrganizationID:     org.ID,
		OwnerID:            user.ID,
		Title:              "goal source edit",
		ModelConfigID:      model.ID,
		InitialUserContent: []codersdk.ChatMessagePart{codersdk.ChatMessageText("objective text")},
		GoalMutation: &codersdk.ChatGoalMutation{
			Action:    codersdk.ChatGoalMutationActionSet,
			Objective: "objective text",
		},
	})
	require.NoError(t, err)
	_, err = db.UpdateChatStatus(dbauthz.AsSystemRestricted(ctx), database.UpdateChatStatusParams{
		ID:     chat.ID,
		Status: database.ChatStatusWaiting,
	})
	require.NoError(t, err)

	goals, err := db.GetCurrentChatGoalsByRootChatIDs(dbauthz.AsSystemRestricted(ctx), []uuid.UUID{chat.ID})
	require.NoError(t, err)
	require.Len(t, goals, 1)
	goal := goals[0]
	require.True(t, goal.CreatedFromMessageID.Valid)
	sourceID := goal.CreatedFromMessageID.Int64

	edit := func(messageID int64) (chatd.EditMessageResult, error) {
		return server.EditMessage(ctx, chatd.EditMessageOptions{
			ChatID:          chat.ID,
			CreatedBy:       user.ID,
			EditedMessageID: messageID,
			Content:         []codersdk.ChatMessagePart{codersdk.ChatMessageText("rewritten")},
		})
	}
	_, err = edit(sourceID)
	require.ErrorIs(t, err, chatd.ErrChatGoalSourceMessageEdit)

	_, err = db.PauseChatGoalByID(dbauthz.AsSystemRestricted(ctx), database.PauseChatGoalByIDParams{
		RootChatID: chat.ID,
		ID:         goal.ID,
	})
	require.NoError(t, err)
	_, err = edit(sourceID)
	require.ErrorIs(t, err, chatd.ErrChatGoalSourceMessageEdit,
		"a paused goal can resume, so its source stays locked")

	_, err = db.ResumeChatGoalByID(dbauthz.AsSystemRestricted(ctx), database.ResumeChatGoalByIDParams{
		RootChatID: chat.ID,
		ID:         goal.ID,
	})
	require.NoError(t, err)
	_, err = db.BlockChatGoalByID(dbauthz.AsSystemRestricted(ctx), database.BlockChatGoalByIDParams{
		RootChatID:    chat.ID,
		ID:            goal.ID,
		BlockedReason: "waiting on user",
	})
	require.NoError(t, err)
	_, err = edit(sourceID)
	require.ErrorIs(t, err, chatd.ErrChatGoalSourceMessageEdit,
		"a blocked goal can resume, so its source stays locked")

	_, err = db.ResumeChatGoalByID(dbauthz.AsSystemRestricted(ctx), database.ResumeChatGoalByIDParams{
		RootChatID: chat.ID,
		ID:         goal.ID,
	})
	require.NoError(t, err)
	_, err = db.CompleteChatGoalByID(dbauthz.AsSystemRestricted(ctx), database.CompleteChatGoalByIDParams{
		RootChatID:       chat.ID,
		ID:               goal.ID,
		CompletedByAgent: true,
	})
	require.NoError(t, err)
	replaced, err := edit(sourceID)
	require.NoError(t, err, "a completed goal no longer locks its source message")

	// A goal anchored to a later message locks every earlier message
	// too: the edit's suffix deletion would truncate the source away.
	_, err = db.UpdateChatStatus(dbauthz.AsSystemRestricted(ctx), database.UpdateChatStatusParams{
		ID:     chat.ID,
		Status: database.ChatStatusWaiting,
	})
	require.NoError(t, err)
	sendResult, err := server.SendMessage(ctx, chatd.SendMessageOptions{
		ChatID:    chat.ID,
		CreatedBy: user.ID,
		Content:   []codersdk.ChatMessagePart{codersdk.ChatMessageText("second objective")},
		GoalMutation: &codersdk.ChatGoalMutation{
			Action:    codersdk.ChatGoalMutationActionSet,
			Objective: "second objective",
		},
	})
	require.NoError(t, err)
	require.NotNil(t, sendResult.Goal)
	require.Less(t, replaced.Message.ID, sendResult.Message.ID)
	_, err = edit(replaced.Message.ID)
	require.ErrorIs(t, err, chatd.ErrChatGoalSourceMessageEdit,
		"editing a message older than the goal source would delete the source")
}

// Message IDs are allocated globally, so a child chat's rows can be
// older than the root goal's source message. Editing child history
// cannot delete the source row and must not trip the guard.
func TestEditMessageChildChatIgnoresGoalSourceGuard(t *testing.T) {
	t.Parallel()
	db, ps := dbtestutil.NewDB(t)
	ctx := testutil.Context(t, testutil.WaitLong)
	user, org, model := seedChatDependencies(t, db)
	server := newTestServer(t, db, ps, uuid.New())

	chat, err := server.CreateChat(ctx, chatd.CreateOptions{
		OrganizationID:     org.ID,
		OwnerID:            user.ID,
		Title:              "root without goal",
		ModelConfigID:      model.ID,
		InitialUserContent: []codersdk.ChatMessagePart{codersdk.ChatMessageText("root start")},
	})
	require.NoError(t, err)

	childContent, err := chatprompt.MarshalParts([]codersdk.ChatMessagePart{
		codersdk.ChatMessageText("child question"),
	})
	require.NoError(t, err)
	createdChild, err := chatstate.CreateChat(ctx, db, ps, chatstate.CreateChatInput{
		OrganizationID:    org.ID,
		OwnerID:           user.ID,
		ParentChatID:      uuid.NullUUID{UUID: chat.ID, Valid: true},
		RootChatID:        uuid.NullUUID{UUID: chat.ID, Valid: true},
		LastModelConfigID: model.ID,
		Title:             "child before goal",
		ClientType:        database.ChatClientTypeApi,
		InitialMessages: []chatstate.Message{
			{
				Role:           database.ChatMessageRoleUser,
				Content:        childContent,
				Visibility:     database.ChatMessageVisibilityBoth,
				ContentVersion: chatprompt.CurrentContentVersion,
				CreatedBy:      uuid.NullUUID{UUID: user.ID, Valid: true},
				ModelConfigID:  uuid.NullUUID{UUID: model.ID, Valid: true},
			},
		},
	})
	require.NoError(t, err)
	childMessageID := createdChild.InitialMessages[len(createdChild.InitialMessages)-1].ID

	// Set the root goal after the child message exists so the goal
	// source ID exceeds the child message ID.
	_, err = db.UpdateChatStatus(dbauthz.AsSystemRestricted(ctx), database.UpdateChatStatusParams{
		ID:     chat.ID,
		Status: database.ChatStatusWaiting,
	})
	require.NoError(t, err)
	sendResult, err := server.SendMessage(ctx, chatd.SendMessageOptions{
		ChatID:    chat.ID,
		CreatedBy: user.ID,
		Content:   []codersdk.ChatMessagePart{codersdk.ChatMessageText("objective text")},
		GoalMutation: &codersdk.ChatGoalMutation{
			Action:    codersdk.ChatGoalMutationActionSet,
			Objective: "objective text",
		},
	})
	require.NoError(t, err)
	require.NotNil(t, sendResult.Goal)
	require.Greater(t, sendResult.Message.ID, childMessageID)

	_, err = db.UpdateChatStatus(dbauthz.AsSystemRestricted(ctx), database.UpdateChatStatusParams{
		ID:     createdChild.Chat.ID,
		Status: database.ChatStatusWaiting,
	})
	require.NoError(t, err)
	_, err = server.EditMessage(ctx, chatd.EditMessageOptions{
		ChatID:          createdChild.Chat.ID,
		CreatedBy:       user.ID,
		EditedMessageID: childMessageID,
		Content:         []codersdk.ChatMessagePart{codersdk.ChatMessageText("rewritten child question")},
	})
	require.NoError(t, err, "editing child history cannot delete the root goal source")
}
