package chatd_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbtestutil"
	dbpubsub "github.com/coder/coder/v2/coderd/database/pubsub"
	"github.com/coder/coder/v2/coderd/x/chatd"
	"github.com/coder/coder/v2/coderd/x/chatd/chatprompt"
	"github.com/coder/coder/v2/coderd/x/chatd/chatstate"
	"github.com/coder/coder/v2/coderd/x/chatd/chattest"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/codersdk/agenthooks"
	"github.com/coder/coder/v2/codersdk/workspacesdk"
	"github.com/coder/coder/v2/codersdk/workspacesdk/agentconnmock"
	"github.com/coder/coder/v2/testutil"
)

func TestPreToolUseHookAllow(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		response     string
		expectedPath string
		decision     string
	}{
		{
			name:         "passthrough",
			response:     `{}`,
			expectedPath: "/tmp/before.txt",
			decision:     "allow",
		},
		{
			name:         "override",
			response:     `{"permission":{"decision":"allow","input_override":{"path":"/tmp/after.txt"}},"user_message":"tool approved","allowed_tools":["read_file"]}`,
			expectedPath: "/tmp/after.txt",
			decision:     "allow",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			ctx := testutil.Context(t, testutil.WaitLong)
			db, ps := dbtestutil.NewDB(t)
			var modelCalls atomic.Int32
			openAIURL := chattest.NewOpenAI(t, func(req *chattest.OpenAIRequest) chattest.OpenAIResponse {
				if !req.Stream {
					return chattest.OpenAINonStreamingResponse("title")
				}
				if modelCalls.Add(1) == 1 {
					chunk := chattest.OpenAIToolCallChunk("read_file", `{"path":"/tmp/before.txt"}`)
					chunk.Choices[0].ToolCalls[0].ID = "call_non_uuid"
					return chattest.OpenAIStreamingResponse(chunk)
				}
				return chattest.OpenAIStreamingResponse(chattest.OpenAITextChunks("done")...)
			})
			user, org, model := seedChatDependenciesWithProvider(t, db, "openai-compat", openAIURL)
			ws, dbAgent := seedWorkspaceWithAgent(t, db, user.ID)

			var hookCalls atomic.Int32
			consumer := preToolUseConsumer(t, func(data agenthooks.PreToolUseData) string {
				hookCalls.Add(1)
				require.Equal(t, "call_non_uuid", data.ToolUseID)
				require.Equal(t, "read_file", data.ToolName)
				require.JSONEq(t, `{"path":"/tmp/before.txt"}`, string(data.ToolInput))
				return tt.response
			})

			ctrl := gomock.NewController(t)
			mockConn := agentconnmock.NewMockAgentConn(ctrl)
			setupToolExecutionAgentConn(t, mockConn)
			mockConn.EXPECT().ReadFileLines(gomock.Any(), tt.expectedPath, int64(1), int64(0), gomock.Any()).
				Return(workspacesdk.ReadFileLinesResponse{
					Success: true, FileSize: 4, TotalLines: 1, LinesRead: 1, Content: "data",
				}, nil).
				Times(1)

			server := newActiveTestServer(t, db, ps, func(cfg *chatd.Config) {
				cfg.AIBridgeTransportFactory = chatAIGatewayTransportFactoryPointer(chattest.NewMockAIBridgeTransport(t, openAIURL))
				cfg.HookDispatcher = newHookDispatcher(t, db, consumer)
				cfg.AgentConn = func(_ context.Context, agentID uuid.UUID) (workspacesdk.AgentConn, func(), error) {
					require.Equal(t, dbAgent.ID, agentID)
					return mockConn, func() {}, nil
				}
			})
			chat, err := server.CreateChat(ctx, chatd.CreateOptions{
				OrganizationID: org.ID,
				OwnerID:        user.ID,
				WorkspaceID:    uuid.NullUUID{UUID: ws.ID, Valid: true},
				AgentID:        uuid.NullUUID{UUID: dbAgent.ID, Valid: true},
				Title:          "pre-tool-use-allow",
				ModelConfigID:  model.ID,
				InitialUserContent: []codersdk.ChatMessagePart{
					codersdk.ChatMessageText("read the file"),
				},
			})
			require.NoError(t, err)
			waitForChatStatus(ctx, t, db, chat.ID, database.ChatStatusWaiting)

			require.Equal(t, int32(1), hookCalls.Load())
			call := requireToolCallPart(t, chatToolParts(ctx, t, db, chat.ID), "read_file")
			require.JSONEq(t, `{"path":"`+tt.expectedPath+`"}`, string(call.Args))
			dispatch := lifecycleDispatch(t, db, chat.ID, agenthooks.EventPreToolUse)
			require.Equal(t, "call_non_uuid", dispatch.ToolUseID.String)
			require.Equal(t, tt.decision != "", dispatch.Decision.Valid)
			if tt.decision != "" {
				require.Equal(t, tt.decision, dispatch.Decision.String)
			}
			if tt.name == "override" {
				chatResult, err := db.GetChatByID(ctx, chat.ID)
				require.NoError(t, err)
				require.JSONEq(t, `["read_file"]`, string(chatResult.HookAllowedTools.RawMessage))
				var foundUserMessage bool
				for _, message := range chatMessages(ctx, t, db, chat.ID) {
					parts, err := chatprompt.ParseContent(message)
					require.NoError(t, err)
					if len(parts) == 1 && parts[0].Text == "tool approved" {
						foundUserMessage = true
					}
				}
				require.True(t, foundUserMessage)
			}
			require.JSONEq(t, `{"path":"/tmp/before.txt"}`, string(dispatch.OriginalInput.RawMessage))
		})
	}
}

func TestPreToolUseHookDeny(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitLong)
	db, ps := dbtestutil.NewDB(t)
	var modelCalls atomic.Int32
	var secondMessages []chattest.OpenAIMessage
	var messagesMu sync.Mutex
	openAIURL := chattest.NewOpenAI(t, func(req *chattest.OpenAIRequest) chattest.OpenAIResponse {
		if !req.Stream {
			return chattest.OpenAINonStreamingResponse("title")
		}
		if modelCalls.Add(1) == 1 {
			chunk := chattest.OpenAIToolCallChunk("read_file", `{"path":"/tmp/secret.txt"}`)
			chunk.Choices[0].ToolCalls[0].ID = "call_denied"
			return chattest.OpenAIStreamingResponse(chunk)
		}
		messagesMu.Lock()
		secondMessages = append([]chattest.OpenAIMessage(nil), req.Messages...)
		messagesMu.Unlock()
		return chattest.OpenAIStreamingResponse(chattest.OpenAITextChunks("used another approach")...)
	})
	user, org, model := seedChatDependenciesWithProvider(t, db, "openai-compat", openAIURL)
	ws, dbAgent := seedWorkspaceWithAgent(t, db, user.ID)

	consumer := preToolUseConsumer(t, func(data agenthooks.PreToolUseData) string {
		require.Equal(t, "call_denied", data.ToolUseID)
		return `{"permission":{"decision":"deny","reason":"blocked by policy"},"model_context":"Do not read secrets."}`
	})

	ctrl := gomock.NewController(t)
	mockConn := agentconnmock.NewMockAgentConn(ctrl)
	setupToolExecutionAgentConn(t, mockConn)
	server := newActiveTestServer(t, db, ps, func(cfg *chatd.Config) {
		cfg.AIBridgeTransportFactory = chatAIGatewayTransportFactoryPointer(chattest.NewMockAIBridgeTransport(t, openAIURL))
		cfg.HookDispatcher = newHookDispatcher(t, db, consumer)
		cfg.AgentConn = func(_ context.Context, agentID uuid.UUID) (workspacesdk.AgentConn, func(), error) {
			require.Equal(t, dbAgent.ID, agentID)
			return mockConn, func() {}, nil
		}
	})
	chat, err := server.CreateChat(ctx, chatd.CreateOptions{
		OrganizationID: org.ID,
		OwnerID:        user.ID,
		WorkspaceID:    uuid.NullUUID{UUID: ws.ID, Valid: true},
		AgentID:        uuid.NullUUID{UUID: dbAgent.ID, Valid: true},
		Title:          "pre-tool-use-deny",
		ModelConfigID:  model.ID,
		InitialUserContent: []codersdk.ChatMessagePart{
			codersdk.ChatMessageText("read the secret"),
		},
	})
	require.NoError(t, err)
	waitForChatStatus(ctx, t, db, chat.ID, database.ChatStatusWaiting)

	parts := chatToolParts(ctx, t, db, chat.ID)
	result := requireToolResultPart(t, parts, "read_file")
	require.True(t, result.IsError)
	require.Contains(t, string(result.Result), "DENIED: blocked by policy")

	messages, err := db.GetChatMessagesForPromptByChatID(ctx, chat.ID)
	require.NoError(t, err)
	var foundModelContext bool
	for _, message := range messages {
		if message.Visibility != database.ChatMessageVisibilityModel {
			continue
		}
		parsed, err := chatprompt.ParseContent(message)
		require.NoError(t, err)
		if len(parsed) == 1 && parsed[0].Text == "Do not read secrets." {
			foundModelContext = true
		}
	}
	require.True(t, foundModelContext)

	messagesMu.Lock()
	modelMessages := append([]chattest.OpenAIMessage(nil), secondMessages...)
	messagesMu.Unlock()
	require.True(t, openAIMessagesContain(modelMessages, "DENIED: blocked by policy"))
	require.True(t, openAIMessagesContain(modelMessages, "Do not read secrets."))
	dispatchRows, err := db.ListChatHookDispatchesByChatID(ctx, chat.ID)
	require.NoError(t, err)
	for _, row := range dispatchRows {
		require.NotEqual(t, string(agenthooks.EventPostToolUse), row.Event)
	}
	dispatch := lifecycleDispatch(t, db, chat.ID, agenthooks.EventPreToolUse)
	require.Equal(t, "deny", dispatch.Decision.String)
}

func TestPreToolUseSkipsProviderExecutedTools(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitLong)
	db, ps := dbtestutil.NewDB(t)
	anthropicURL := chattest.NewAnthropic(t, func(_ *chattest.AnthropicRequest) chattest.AnthropicResponse {
		return chattest.AnthropicStreamingResponse(
			anthropicWebSearchPairChunks("ws-hook-skip", `{"query":"coder"}`, "search done", "end_turn")...,
		)
	})
	user, org, model := seedAnthropicChatDependencies(t, db, anthropicURL)
	model = enableAnthropicWebSearchForTest(t, db, model)
	var preToolCalls atomic.Int32
	consumer := preToolUseConsumer(t, func(agenthooks.PreToolUseData) string {
		preToolCalls.Add(1)
		return `{}`
	})

	server := newActiveTestServer(t, db, ps, func(cfg *chatd.Config) {
		cfg.AIBridgeTransportFactory = chatAIGatewayTransportFactoryPointer(chattest.NewMockAIBridgeTransport(t, anthropicURL, chattest.WithPreservePath()))
		cfg.HookDispatcher = newHookDispatcher(t, db, consumer)
	})
	chat := createChatThroughServer(ctx, t, db, server, org.ID, user.ID, model.ID, "search for coder")
	waitForChatStatus(ctx, t, db, chat.ID, database.ChatStatusWaiting)

	require.Zero(t, preToolCalls.Load())
	call := requireToolCallPart(t, chatToolParts(ctx, t, db, chat.ID), "web_search")
	require.True(t, call.ProviderExecuted)
}

func TestPreToolUseHookDynamicAllowResponse(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitLong)
	db, ps := dbtestutil.NewDB(t)
	openAIURL := chattest.NewOpenAI(t, func(req *chattest.OpenAIRequest) chattest.OpenAIResponse {
		if !req.Stream {
			return chattest.OpenAINonStreamingResponse("title")
		}
		chunk := chattest.OpenAIToolCallChunk("my_dynamic_tool", `{"query":"original"}`)
		chunk.Choices[0].ToolCalls[0].ID = "call_dynamic_allow"
		return chattest.OpenAIStreamingResponse(chunk)
	})
	user, org, model := seedChatDependenciesWithProvider(t, db, "openai-compat", openAIURL)
	consumer := preToolUseConsumer(t, func(data agenthooks.PreToolUseData) string {
		require.Equal(t, "call_dynamic_allow", data.ToolUseID)
		return `{"permission":{"decision":"allow","input_override":{"query":"redacted"}},"model_context":"dynamic context","user_message":"dynamic notice","allowed_tools":["my_dynamic_tool"]}`
	})

	server := newActiveTestServer(t, db, ps, func(cfg *chatd.Config) {
		cfg.AIBridgeTransportFactory = chatAIGatewayTransportFactoryPointer(chattest.NewMockAIBridgeTransport(t, openAIURL))
		cfg.HookDispatcher = newHookDispatcher(t, db, consumer)
	})
	chat, err := server.CreateChat(ctx, chatd.CreateOptions{
		OrganizationID: org.ID,
		OwnerID:        user.ID,
		Title:          "pre-tool-use-dynamic-allow",
		ModelConfigID:  model.ID,
		InitialUserContent: []codersdk.ChatMessagePart{
			codersdk.ChatMessageText("call the dynamic tool"),
		},
		DynamicTools: dynamicToolJSON(t, "my_dynamic_tool"),
	})
	require.NoError(t, err)

	var action database.Chat
	testutil.Eventually(ctx, t, func(ctx context.Context) bool {
		action, err = db.GetChatByID(ctx, chat.ID)
		return err == nil && action.Status == database.ChatStatusRequiresAction
	}, testutil.IntervalFast)
	require.JSONEq(t, `["my_dynamic_tool"]`, string(action.HookAllowedTools.RawMessage))
	call := requireToolCallPart(t, chatToolParts(ctx, t, db, chat.ID), "my_dynamic_tool")
	require.JSONEq(t, `{"query":"redacted"}`, string(call.Args))
	promptMessages, err := db.GetChatMessagesForPromptByChatID(ctx, chat.ID)
	require.NoError(t, err)
	var foundContext bool
	for _, message := range promptMessages {
		parts, err := chatprompt.ParseContent(message)
		require.NoError(t, err)
		if len(parts) == 1 && parts[0].Text == "dynamic context" {
			foundContext = true
		}
	}
	require.True(t, foundContext)
	var foundNotice bool
	for _, message := range chatMessages(ctx, t, db, chat.ID) {
		parts, err := chatprompt.ParseContent(message)
		require.NoError(t, err)
		if len(parts) == 1 && parts[0].Text == "dynamic notice" {
			foundNotice = true
		}
	}
	require.True(t, foundNotice)
}

func TestPreToolUseHookEndChat(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitLong)
	db, ps := dbtestutil.NewDB(t)
	var modelCalls atomic.Int32
	openAIURL := chattest.NewOpenAI(t, func(req *chattest.OpenAIRequest) chattest.OpenAIResponse {
		if !req.Stream {
			return chattest.OpenAINonStreamingResponse("title")
		}
		modelCalls.Add(1)
		chunk := chattest.OpenAIToolCallChunk("my_dynamic_tool", `{}`)
		chunk.Choices[0].ToolCalls[0].ID = "call_end_chat"
		return chattest.OpenAIStreamingResponse(chunk)
	})
	user, org, model := seedChatDependenciesWithProvider(t, db, "openai-compat", openAIURL)
	consumer := preToolUseConsumer(t, func(data agenthooks.PreToolUseData) string {
		require.Equal(t, "call_end_chat", data.ToolUseID)
		return `{"end_chat":true}`
	})

	server := newActiveTestServer(t, db, ps, func(cfg *chatd.Config) {
		cfg.AIBridgeTransportFactory = chatAIGatewayTransportFactoryPointer(chattest.NewMockAIBridgeTransport(t, openAIURL))
		cfg.HookDispatcher = newHookDispatcher(t, db, consumer)
	})
	chat, err := server.CreateChat(ctx, chatd.CreateOptions{
		OrganizationID: org.ID,
		OwnerID:        user.ID,
		Title:          "pre-tool-use-end-chat",
		ModelConfigID:  model.ID,
		InitialUserContent: []codersdk.ChatMessagePart{
			codersdk.ChatMessageText("stop before the tool"),
		},
		DynamicTools: dynamicToolJSON(t, "my_dynamic_tool"),
	})
	require.NoError(t, err)

	var archived database.Chat
	testutil.Eventually(ctx, t, func(ctx context.Context) bool {
		archived, err = db.GetChatByID(ctx, chat.ID)
		return err == nil && archived.Archived && archived.Status == database.ChatStatusWaiting
	}, testutil.IntervalFast)
	require.Equal(t, int32(1), modelCalls.Load())
	require.False(t, archived.RequiresActionDeadlineAt.Valid)
	parts := chatToolParts(ctx, t, db, chat.ID)
	call := requireToolCallPart(t, parts, "my_dynamic_tool")
	require.Equal(t, "call_end_chat", call.ToolCallID)
	result := requireToolResultPart(t, parts, "my_dynamic_tool")
	require.Equal(t, "call_end_chat", result.ToolCallID)
	require.True(t, result.IsError)
	require.Contains(t, string(result.Result), "chat was ended")
}

func TestPreToolUseHookEndChatSkipsLocalToolExecution(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitLong)
	db, ps := dbtestutil.NewDB(t)
	openAIURL := chattest.NewOpenAI(t, func(req *chattest.OpenAIRequest) chattest.OpenAIResponse {
		if !req.Stream {
			return chattest.OpenAINonStreamingResponse("title")
		}
		return chattest.OpenAIStreamingResponse(chattest.OpenAITextChunks("done")...)
	})
	user, org, model := seedChatDependenciesWithProvider(t, db, "openai-compat", openAIURL)
	ws, dbAgent := seedWorkspaceWithAgent(t, db, user.ID)
	chatID := seedPendingToolCall(ctx, t, db, ps, pendingToolCallSeed{
		OrganizationID: org.ID,
		OwnerID:        user.ID,
		WorkspaceID:    ws.ID,
		AgentID:        dbAgent.ID,
		ModelConfigID:  model.ID,
		ToolCallID:     "call_local_end_chat",
		ToolName:       "read_file",
		ToolInput:      `{"path":"/tmp/secret.txt"}`,
	})

	var hookCalls atomic.Int32
	consumer := preToolUseConsumer(t, func(data agenthooks.PreToolUseData) string {
		hookCalls.Add(1)
		require.Equal(t, "call_local_end_chat", data.ToolUseID)
		return `{"end_chat":true}`
	})

	ctrl := gomock.NewController(t)
	mockConn := agentconnmock.NewMockAgentConn(ctrl)
	setupToolExecutionAgentConn(t, mockConn)

	server := newTestServer(t, db, ps, uuid.New(), func(cfg *chatd.Config) {
		cfg.AIBridgeTransportFactory = chatAIGatewayTransportFactoryPointer(chattest.NewMockAIBridgeTransport(t, openAIURL))
		cfg.HookDispatcher = newHookDispatcher(t, db, consumer)
		cfg.AgentConn = func(_ context.Context, agentID uuid.UUID) (workspacesdk.AgentConn, func(), error) {
			require.Equal(t, dbAgent.ID, agentID)
			return mockConn, func() {}, nil
		}
	})
	server.Start()

	var archived database.Chat
	testutil.Eventually(ctx, t, func(ctx context.Context) bool {
		var err error
		archived, err = db.GetChatByID(ctx, chatID)
		return err == nil && archived.Archived && archived.Status == database.ChatStatusWaiting
	}, testutil.IntervalFast)
	require.Equal(t, int32(1), hookCalls.Load())
	require.False(t, archived.LastError.Valid)
	parts := chatToolParts(ctx, t, db, chatID)
	call := requireToolCallPart(t, parts, "read_file")
	require.Equal(t, "call_local_end_chat", call.ToolCallID)
	result := requireToolResultPart(t, parts, "read_file")
	require.Equal(t, "call_local_end_chat", result.ToolCallID)
	require.True(t, result.IsError, "never-executed call must persist as a synthetic cancellation")
}

func TestPreToolUseHookEndChatShortCircuitsLaterDispatches(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitLong)
	db, ps := dbtestutil.NewDB(t)
	openAIURL := chattest.NewOpenAI(t, func(req *chattest.OpenAIRequest) chattest.OpenAIResponse {
		if !req.Stream {
			return chattest.OpenAINonStreamingResponse("title")
		}
		return chattest.OpenAIStreamingResponse(chattest.OpenAITextChunks("done")...)
	})
	user, org, model := seedChatDependenciesWithProvider(t, db, "openai-compat", openAIURL)
	ws, dbAgent := seedWorkspaceWithAgent(t, db, user.ID)

	// Seed one assistant step with two unresolved tool calls so the
	// preflight loop dispatches sequentially.
	userContent, err := chatprompt.MarshalParts([]codersdk.ChatMessagePart{codersdk.ChatMessageText("resume")})
	require.NoError(t, err)
	created, err := chatstate.CreateChat(ctx, db, ps, chatstate.CreateChatInput{
		OrganizationID:    org.ID,
		OwnerID:           user.ID,
		WorkspaceID:       uuid.NullUUID{UUID: ws.ID, Valid: true},
		AgentID:           uuid.NullUUID{UUID: dbAgent.ID, Valid: true},
		LastModelConfigID: model.ID,
		Title:             "pending-tool-calls",
		ClientType:        database.ChatClientTypeApi,
		InitialMessages: []chatstate.Message{
			{
				Role:           database.ChatMessageRoleUser,
				Content:        userContent,
				Visibility:     database.ChatMessageVisibilityBoth,
				TurnID:         uuid.NullUUID{UUID: uuid.New(), Valid: true},
				ContentVersion: chatprompt.CurrentContentVersion,
				CreatedBy:      uuid.NullUUID{UUID: user.ID, Valid: true},
				ModelConfigID:  uuid.NullUUID{UUID: model.ID, Valid: true},
			},
		},
	})
	require.NoError(t, err)
	chatID := created.Chat.ID
	assistantContent, err := chatprompt.MarshalParts([]codersdk.ChatMessagePart{
		{
			Type:       codersdk.ChatMessagePartTypeToolCall,
			ToolCallID: "call_end_chat_first",
			ToolName:   "read_file",
			Args:       json.RawMessage(`{"path":"/tmp/first.txt"}`),
		},
		{
			Type:       codersdk.ChatMessagePartTypeToolCall,
			ToolCallID: "call_never_dispatched",
			ToolName:   "read_file",
			Args:       json.RawMessage(`{"path":"/tmp/second.txt"}`),
		},
	})
	require.NoError(t, err)
	machine := chatstate.NewChatMachine(db, ps, chatID)
	require.NoError(t, machine.Update(ctx, func(tx *chatstate.Tx, _ database.Store) error {
		_, err := tx.CommitStep(chatstate.CommitStepInput{Messages: []chatstate.Message{
			{
				Role:           database.ChatMessageRoleAssistant,
				Content:        assistantContent,
				Visibility:     database.ChatMessageVisibilityBoth,
				TurnID:         created.InitialMessages[0].TurnID,
				ContentVersion: chatprompt.CurrentContentVersion,
				ModelConfigID:  uuid.NullUUID{UUID: model.ID, Valid: true},
			},
		}})
		return err
	}))

	// end_chat on the first call, hard failure on the second: the
	// second dispatch must never happen, so its failure cannot push
	// the chat into an error state.
	var hookCalls atomic.Int32
	consumer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request agenthooks.Request
		require.NoError(t, json.NewDecoder(r.Body).Decode(&request))
		if request.Type != agenthooks.EventPreToolUse {
			_, err := w.Write([]byte(`{}`))
			require.NoError(t, err)
			return
		}
		hookCalls.Add(1)
		decoded, err := request.Decode()
		require.NoError(t, err)
		data, ok := decoded.(*agenthooks.PreToolUseData)
		require.True(t, ok)
		if data.ToolUseID != "call_end_chat_first" {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		_, err = w.Write([]byte(`{"end_chat":true}`))
		require.NoError(t, err)
	}))
	t.Cleanup(consumer.Close)

	ctrl := gomock.NewController(t)
	mockConn := agentconnmock.NewMockAgentConn(ctrl)
	setupToolExecutionAgentConn(t, mockConn)

	server := newTestServer(t, db, ps, uuid.New(), func(cfg *chatd.Config) {
		cfg.AIBridgeTransportFactory = chatAIGatewayTransportFactoryPointer(chattest.NewMockAIBridgeTransport(t, openAIURL))
		cfg.HookDispatcher = newHookDispatcher(t, db, consumer)
		cfg.AgentConn = func(_ context.Context, agentID uuid.UUID) (workspacesdk.AgentConn, func(), error) {
			require.Equal(t, dbAgent.ID, agentID)
			return mockConn, func() {}, nil
		}
	})
	server.Start()

	var archived database.Chat
	testutil.Eventually(ctx, t, func(ctx context.Context) bool {
		var err error
		archived, err = db.GetChatByID(ctx, chatID)
		return err == nil && archived.Archived && archived.Status == database.ChatStatusWaiting
	}, testutil.IntervalFast)
	require.False(t, archived.LastError.Valid)
	require.Equal(t, int32(1), hookCalls.Load())

	rows, err := db.ListChatHookDispatchesByChatID(ctx, chatID)
	require.NoError(t, err)
	preToolUse := 0
	for _, row := range rows {
		if row.Event == string(agenthooks.EventPreToolUse) {
			preToolUse++
			require.Equal(t, "call_end_chat_first", row.ToolUseID.String)
		}
	}
	require.Equal(t, 1, preToolUse)

	results := map[string]bool{}
	for _, part := range chatToolParts(ctx, t, db, chatID) {
		if part.Type == codersdk.ChatMessagePartTypeToolResult {
			results[part.ToolCallID] = part.IsError
		}
	}
	require.Equal(t, map[string]bool{
		"call_end_chat_first":   true,
		"call_never_dispatched": true,
	}, results, "both never-executed calls must persist as synthetic cancellations")
}

func TestPreToolUseHookRepeatedToolCallIDDispatchesFresh(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitLong)
	db, ps := dbtestutil.NewDB(t)
	openAIURL := chattest.NewOpenAI(t, func(req *chattest.OpenAIRequest) chattest.OpenAIResponse {
		if !req.Stream {
			return chattest.OpenAINonStreamingResponse("title")
		}
		return chattest.OpenAIStreamingResponse(chattest.OpenAITextChunks("done")...)
	})
	user, org, model := seedChatDependenciesWithProvider(t, db, "openai-compat", openAIURL)
	ws, dbAgent := seedWorkspaceWithAgent(t, db, user.ID)

	// A later assistant step reuses the tool call ID, name, and input of
	// an already-resolved call from an earlier step in the same turn.
	const toolUseID = "call_repeated"
	toolInput := `{"path":"/tmp/dup.txt"}`
	userContent, err := chatprompt.MarshalParts([]codersdk.ChatMessagePart{codersdk.ChatMessageText("resume")})
	require.NoError(t, err)
	created, err := chatstate.CreateChat(ctx, db, ps, chatstate.CreateChatInput{
		OrganizationID:    org.ID,
		OwnerID:           user.ID,
		WorkspaceID:       uuid.NullUUID{UUID: ws.ID, Valid: true},
		AgentID:           uuid.NullUUID{UUID: dbAgent.ID, Valid: true},
		LastModelConfigID: model.ID,
		Title:             "repeated-tool-call-id",
		ClientType:        database.ChatClientTypeApi,
		InitialMessages: []chatstate.Message{
			{
				Role:           database.ChatMessageRoleUser,
				Content:        userContent,
				Visibility:     database.ChatMessageVisibilityBoth,
				TurnID:         uuid.NullUUID{UUID: uuid.New(), Valid: true},
				ContentVersion: chatprompt.CurrentContentVersion,
				CreatedBy:      uuid.NullUUID{UUID: user.ID, Valid: true},
				ModelConfigID:  uuid.NullUUID{UUID: model.ID, Valid: true},
			},
		},
	})
	require.NoError(t, err)
	chatID := created.Chat.ID
	turnID := created.InitialMessages[0].TurnID
	toolCallContent, err := chatprompt.MarshalParts([]codersdk.ChatMessagePart{{
		Type:       codersdk.ChatMessagePartTypeToolCall,
		ToolCallID: toolUseID,
		ToolName:   "read_file",
		Args:       json.RawMessage(toolInput),
	}})
	require.NoError(t, err)
	toolResultContent, err := chatprompt.MarshalParts([]codersdk.ChatMessagePart{{
		Type:       codersdk.ChatMessagePartTypeToolResult,
		ToolCallID: toolUseID,
		ToolName:   "read_file",
		Result:     json.RawMessage(`{"output":"data"}`),
	}})
	require.NoError(t, err)
	machine := chatstate.NewChatMachine(db, ps, chatID)
	require.NoError(t, machine.Update(ctx, func(tx *chatstate.Tx, _ database.Store) error {
		_, err := tx.CommitStep(chatstate.CommitStepInput{Messages: []chatstate.Message{
			{
				Role:           database.ChatMessageRoleAssistant,
				Content:        toolCallContent,
				Visibility:     database.ChatMessageVisibilityBoth,
				TurnID:         turnID,
				ContentVersion: chatprompt.CurrentContentVersion,
				ModelConfigID:  uuid.NullUUID{UUID: model.ID, Valid: true},
			},
			{
				Role:           database.ChatMessageRoleTool,
				Content:        toolResultContent,
				Visibility:     database.ChatMessageVisibilityBoth,
				TurnID:         turnID,
				ContentVersion: chatprompt.CurrentContentVersion,
				ModelConfigID:  uuid.NullUUID{UUID: model.ID, Valid: true},
			},
			{
				Role:           database.ChatMessageRoleAssistant,
				Content:        toolCallContent,
				Visibility:     database.ChatMessageVisibilityBoth,
				TurnID:         turnID,
				ContentVersion: chatprompt.CurrentContentVersion,
				ModelConfigID:  uuid.NullUUID{UUID: model.ID, Valid: true},
			},
		}})
		return err
	}))

	// The earlier occurrence's recorded allow must not authorize the
	// repeated call; the live consumer denies it.
	recordPreToolUseDecision(ctx, t, db, chatID, user.ID, turnID.UUID, toolUseID, "read_file", json.RawMessage(toolInput), agenthooks.PermissionAllow, "", nil)

	var hookCalls atomic.Int32
	consumer := preToolUseConsumer(t, func(data agenthooks.PreToolUseData) string {
		hookCalls.Add(1)
		require.Equal(t, toolUseID, data.ToolUseID)
		return `{"permission":{"decision":"deny","reason":"repeated call"}}`
	})

	server := newTestServer(t, db, ps, uuid.New(), func(cfg *chatd.Config) {
		cfg.AIBridgeTransportFactory = chatAIGatewayTransportFactoryPointer(chattest.NewMockAIBridgeTransport(t, openAIURL))
		cfg.HookDispatcher = newHookDispatcher(t, db, consumer)
	})
	server.Start()
	waitForChatStatus(ctx, t, db, chatID, database.ChatStatusWaiting)

	require.Equal(t, int32(1), hookCalls.Load(), "the repeated occurrence must dispatch fresh")
	denied := 0
	for _, part := range chatToolParts(ctx, t, db, chatID) {
		if part.Type == codersdk.ChatMessagePartTypeToolResult && part.IsError &&
			strings.Contains(string(part.Result), "repeated call") {
			denied++
		}
	}
	require.Equal(t, 1, denied, "the repeated call must persist the fresh deny result")
}

func TestPreToolUseHookRecordedEndChatSkipsExecution(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitLong)
	db, ps := dbtestutil.NewDB(t)
	openAIURL := chattest.NewOpenAI(t, func(req *chattest.OpenAIRequest) chattest.OpenAIResponse {
		if !req.Stream {
			return chattest.OpenAINonStreamingResponse("title")
		}
		return chattest.OpenAIStreamingResponse(chattest.OpenAITextChunks("done")...)
	})
	user, org, model := seedChatDependenciesWithProvider(t, db, "openai-compat", openAIURL)
	ws, dbAgent := seedWorkspaceWithAgent(t, db, user.ID)
	chatID := seedPendingToolCall(ctx, t, db, ps, pendingToolCallSeed{
		OrganizationID: org.ID,
		OwnerID:        user.ID,
		WorkspaceID:    ws.ID,
		AgentID:        dbAgent.ID,
		ModelConfigID:  model.ID,
		ToolCallID:     "call_recorded_end_chat",
		ToolName:       "read_file",
		ToolInput:      `{"path":"/tmp/end.txt"}`,
	})
	messages, err := db.GetChatMessagesForPromptByChatID(ctx, chatID)
	require.NoError(t, err)
	require.True(t, messages[0].TurnID.Valid)
	turnID := messages[0].TurnID.UUID
	recordPreToolUseResponse(ctx, t, db, chatID, user.ID, turnID, "call_recorded_end_chat", "read_file", json.RawMessage(`{"path":"/tmp/end.txt"}`), agenthooks.PermissionAllow, "", nil, agenthooks.Response{
		EndChat: true,
	})

	var preToolUseCalls, postToolUseCalls atomic.Int32
	consumer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request agenthooks.Request
		require.NoError(t, json.NewDecoder(r.Body).Decode(&request))
		switch request.Type {
		case agenthooks.EventPreToolUse:
			preToolUseCalls.Add(1)
		case agenthooks.EventPostToolUse:
			postToolUseCalls.Add(1)
		}
		_, err := w.Write([]byte(`{}`))
		require.NoError(t, err)
	}))
	t.Cleanup(consumer.Close)

	ctrl := gomock.NewController(t)
	mockConn := agentconnmock.NewMockAgentConn(ctrl)
	setupToolExecutionAgentConn(t, mockConn)

	server := newTestServer(t, db, ps, uuid.New(), func(cfg *chatd.Config) {
		cfg.AIBridgeTransportFactory = chatAIGatewayTransportFactoryPointer(chattest.NewMockAIBridgeTransport(t, openAIURL))
		cfg.HookDispatcher = newHookDispatcher(t, db, consumer)
		cfg.AgentConn = func(_ context.Context, agentID uuid.UUID) (workspacesdk.AgentConn, func(), error) {
			require.Equal(t, dbAgent.ID, agentID)
			return mockConn, func() {}, nil
		}
	})
	server.Start()

	var archived database.Chat
	testutil.Eventually(ctx, t, func(ctx context.Context) bool {
		var err error
		archived, err = db.GetChatByID(ctx, chatID)
		return err == nil && archived.Archived && archived.Status == database.ChatStatusWaiting
	}, testutil.IntervalFast)
	require.False(t, archived.LastError.Valid)
	require.Zero(t, preToolUseCalls.Load(), "recorded decision must not re-dispatch")
	require.Zero(t, postToolUseCalls.Load(), "never-executed call must not dispatch post_tool_use")

	result := requireToolResultPart(t, chatToolParts(ctx, t, db, chatID), "read_file")
	require.Equal(t, "call_recorded_end_chat", result.ToolCallID)
	require.True(t, result.IsError, "never-executed call must persist as a synthetic cancellation")
	row, err := db.GetChatHookDispatchDecision(ctx, database.GetChatHookDispatchDecisionParams{
		ChatID:    chatID,
		ToolUseID: "call_recorded_end_chat",
		ToolName:  "read_file",
		ToolInput: json.RawMessage(`{"path":"/tmp/end.txt"}`),
		TurnID:    uuid.NullUUID{UUID: turnID, Valid: true},
	})
	require.NoError(t, err)
	require.True(t, row.EffectsAppliedAt.Valid, "replayed end_chat effects must be marked applied")
}

func TestPreToolUseHookDispatchFailure(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		statusCode int
		response   string
		result     string
	}{
		{
			name:       "http error",
			statusCode: http.StatusInternalServerError,
			result:     "http_error",
		},
		{
			name:     "ask protocol error",
			response: `{"permission":{"decision":"ask"}}`,
			result:   "protocol_error",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			ctx := testutil.Context(t, testutil.WaitLong)
			db, ps := dbtestutil.NewDB(t)
			openAIURL := chattest.NewOpenAI(t, func(req *chattest.OpenAIRequest) chattest.OpenAIResponse {
				if !req.Stream {
					return chattest.OpenAINonStreamingResponse("title")
				}
				chunk := chattest.OpenAIToolCallChunk("my_dynamic_tool", `{}`)
				chunk.Choices[0].ToolCalls[0].ID = "call_failure"
				return chattest.OpenAIStreamingResponse(chunk)
			})
			user, org, model := seedChatDependenciesWithProvider(t, db, "openai-compat", openAIURL)
			consumer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				var request agenthooks.Request
				require.NoError(t, json.NewDecoder(r.Body).Decode(&request))
				if request.Type != agenthooks.EventPreToolUse {
					_, err := w.Write([]byte(`{}`))
					require.NoError(t, err)
					return
				}
				if tt.statusCode != 0 {
					w.WriteHeader(tt.statusCode)
					return
				}
				_, err := w.Write([]byte(tt.response))
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
				Title:          "pre-tool-use-failure",
				ModelConfigID:  model.ID,
				InitialUserContent: []codersdk.ChatMessagePart{
					codersdk.ChatMessageText("fail before commit"),
				},
				DynamicTools: dynamicToolJSON(t, "my_dynamic_tool"),
			})
			require.NoError(t, err)
			waitForChatStatus(ctx, t, db, chat.ID, database.ChatStatusError)

			failed, err := db.GetChatByID(ctx, chat.ID)
			require.NoError(t, err)
			dispatch := lifecycleDispatch(t, db, chat.ID, agenthooks.EventPreToolUse)
			require.Equal(t, tt.result, dispatch.Result)
			lastError := chatLastErrorMessage(failed.LastError)
			require.Contains(t, lastError, "hook dispatch failed: pre_tool_use: "+tt.result)
			require.Contains(t, lastError, dispatch.ID.String())
			messages := chatMessages(ctx, t, db, chat.ID)
			require.Len(t, messages, 1)
			require.Equal(t, database.ChatMessageRoleUser, messages[0].Role)
		})
	}
}

func TestPreToolUseHookFailureAbortsWholeStep(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitLong)
	db, ps := dbtestutil.NewDB(t)
	openAIURL := chattest.NewOpenAI(t, func(req *chattest.OpenAIRequest) chattest.OpenAIResponse {
		if !req.Stream {
			return chattest.OpenAINonStreamingResponse("title")
		}
		first := chattest.OpenAIToolCallChunk("my_dynamic_tool", `{"value":1}`)
		first.Choices[0].ToolCalls[0].ID = "call_first"
		second := chattest.OpenAIToolCallChunk("my_dynamic_tool", `{"value":2}`).Choices[0].ToolCalls[0]
		second.ID = "call_second"
		second.Index = 1
		first.Choices[0].ToolCalls = append(first.Choices[0].ToolCalls, second)
		return chattest.OpenAIStreamingResponse(first)
	})
	user, org, model := seedChatDependenciesWithProvider(t, db, "openai-compat", openAIURL)
	consumer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request agenthooks.Request
		require.NoError(t, json.NewDecoder(r.Body).Decode(&request))
		if request.Type != agenthooks.EventPreToolUse {
			_, err := w.Write([]byte(`{}`))
			require.NoError(t, err)
			return
		}
		decoded, err := request.Decode()
		require.NoError(t, err)
		data := decoded.(*agenthooks.PreToolUseData)
		if data.ToolUseID == "call_first" {
			_, err = w.Write([]byte(`{"permission":{"decision":"allow","input_override":{"value":1}}}`))
			require.NoError(t, err)
			return
		}
		require.Equal(t, "call_second", data.ToolUseID)
		w.WriteHeader(http.StatusInternalServerError)
	}))
	t.Cleanup(consumer.Close)

	server := newActiveTestServer(t, db, ps, func(cfg *chatd.Config) {
		cfg.AIBridgeTransportFactory = chatAIGatewayTransportFactoryPointer(chattest.NewMockAIBridgeTransport(t, openAIURL))
		cfg.HookDispatcher = newHookDispatcher(t, db, consumer)
	})
	chat, err := server.CreateChat(ctx, chatd.CreateOptions{
		OrganizationID: org.ID,
		OwnerID:        user.ID,
		Title:          "pre-tool-use-atomic-failure",
		ModelConfigID:  model.ID,
		InitialUserContent: []codersdk.ChatMessagePart{
			codersdk.ChatMessageText("call both tools"),
		},
		DynamicTools: dynamicToolJSON(t, "my_dynamic_tool"),
	})
	require.NoError(t, err)
	waitForChatStatus(ctx, t, db, chat.ID, database.ChatStatusError)

	messages := chatMessages(ctx, t, db, chat.ID)
	require.Len(t, messages, 1)
	rows, err := db.ListChatHookDispatchesByChatID(ctx, chat.ID)
	require.NoError(t, err)
	var toolRows []database.ChatHookDispatch
	for _, row := range rows {
		if row.Event == string(agenthooks.EventPreToolUse) {
			toolRows = append(toolRows, row)
		}
	}
	require.Len(t, toolRows, 2)
	require.Equal(t, "call_first", toolRows[0].ToolUseID.String)
	require.Equal(t, "allow", toolRows[0].Decision.String)
	require.Equal(t, "call_second", toolRows[1].ToolUseID.String)
	require.Equal(t, "http_error", toolRows[1].Result)
}

func TestPreToolUseHookResumeRecordedDeny(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitLong)
	db, ps := dbtestutil.NewDB(t)
	var modelCalls atomic.Int32
	openAIURL := chattest.NewOpenAI(t, func(req *chattest.OpenAIRequest) chattest.OpenAIResponse {
		if !req.Stream {
			return chattest.OpenAINonStreamingResponse("title")
		}
		modelCalls.Add(1)
		return chattest.OpenAIStreamingResponse(chattest.OpenAITextChunks("replanned")...)
	})
	user, org, model := seedChatDependenciesWithProvider(t, db, "openai-compat", openAIURL)
	ws, dbAgent := seedWorkspaceWithAgent(t, db, user.ID)
	chatID := seedPendingToolCall(ctx, t, db, ps, pendingToolCallSeed{
		OrganizationID: org.ID,
		OwnerID:        user.ID,
		WorkspaceID:    ws.ID,
		AgentID:        dbAgent.ID,
		ModelConfigID:  model.ID,
		ToolCallID:     "call_recorded_deny",
		ToolName:       "read_file",
		ToolInput:      `{"path":"/tmp/secret.txt"}`,
	})
	messages, err := db.GetChatMessagesForPromptByChatID(ctx, chatID)
	require.NoError(t, err)
	require.True(t, messages[0].TurnID.Valid)
	recordPreToolUseDecision(ctx, t, db, chatID, user.ID, uuid.New(), "call_recorded_deny", "read_file", json.RawMessage(`{"path":"/tmp/secret.txt"}`), agenthooks.PermissionAllow, "", nil)
	recordPreToolUseDecision(ctx, t, db, chatID, user.ID, messages[0].TurnID.UUID, "call_recorded_deny", "read_file", json.RawMessage(`{"path":"/tmp/secret.txt"}`), agenthooks.PermissionDeny, "recorded policy reason", nil)

	var hookCalls atomic.Int32
	consumer := preToolUseConsumer(t, func(agenthooks.PreToolUseData) string {
		hookCalls.Add(1)
		return `{}`
	})
	ctrl := gomock.NewController(t)
	mockConn := agentconnmock.NewMockAgentConn(ctrl)
	setupToolExecutionAgentConn(t, mockConn)
	server := newTestServer(t, db, ps, uuid.New(), func(cfg *chatd.Config) {
		cfg.AIBridgeTransportFactory = chatAIGatewayTransportFactoryPointer(chattest.NewMockAIBridgeTransport(t, openAIURL))
		cfg.HookDispatcher = newHookDispatcher(t, db, consumer)
		cfg.AgentConn = func(_ context.Context, agentID uuid.UUID) (workspacesdk.AgentConn, func(), error) {
			require.Equal(t, dbAgent.ID, agentID)
			return mockConn, func() {}, nil
		}
	})
	server.Start()
	waitForChatStatus(ctx, t, db, chatID, database.ChatStatusWaiting)

	require.Zero(t, hookCalls.Load())
	require.Equal(t, int32(1), modelCalls.Load())
	result := requireToolResultPart(t, chatToolParts(ctx, t, db, chatID), "read_file")
	require.True(t, result.IsError)
	require.Contains(t, string(result.Result), "DENIED: recorded policy reason")
}

func TestPreToolUseHookRecordedDecisionRequiresMatchingToolCall(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name              string
		recordedToolName  string
		recordedToolInput json.RawMessage
	}{
		{
			name:              "InputMismatch",
			recordedToolName:  "read_file",
			recordedToolInput: json.RawMessage(`{"path":"/tmp/secret.txt"}`),
		},
		{
			name:              "ToolNameMismatch",
			recordedToolName:  "execute",
			recordedToolInput: json.RawMessage(`{"path":"/tmp/harmless.txt"}`),
		},
		{
			name:              "MissingToolName",
			recordedToolInput: json.RawMessage(`{"path":"/tmp/harmless.txt"}`),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			ctx := testutil.Context(t, testutil.WaitLong)
			db, ps := dbtestutil.NewDB(t)
			openAIURL := chattest.NewOpenAI(t, func(req *chattest.OpenAIRequest) chattest.OpenAIResponse {
				if !req.Stream {
					return chattest.OpenAINonStreamingResponse("title")
				}
				return chattest.OpenAIStreamingResponse(chattest.OpenAITextChunks("done")...)
			})
			user, org, model := seedChatDependenciesWithProvider(t, db, "openai-compat", openAIURL)
			ws, dbAgent := seedWorkspaceWithAgent(t, db, user.ID)
			toolCallID := "call_binding_mismatch"
			chatID := seedPendingToolCall(ctx, t, db, ps, pendingToolCallSeed{
				OrganizationID: org.ID,
				OwnerID:        user.ID,
				WorkspaceID:    ws.ID,
				AgentID:        dbAgent.ID,
				ModelConfigID:  model.ID,
				ToolCallID:     toolCallID,
				ToolName:       "read_file",
				ToolInput:      `{"path":"/tmp/harmless.txt"}`,
			})
			messages, err := db.GetChatMessagesForPromptByChatID(ctx, chatID)
			require.NoError(t, err)
			require.True(t, messages[0].TurnID.Valid)
			staleDispatchID := recordPreToolUseDecision(ctx, t, db, chatID, user.ID, messages[0].TurnID.UUID, toolCallID, tt.recordedToolName, tt.recordedToolInput, agenthooks.PermissionDeny, "stale decision", nil)

			var hookCalls atomic.Int32
			consumer := preToolUseConsumer(t, func(data agenthooks.PreToolUseData) string {
				hookCalls.Add(1)
				require.Equal(t, "read_file", data.ToolName)
				require.JSONEq(t, `{"path":"/tmp/harmless.txt"}`, string(data.ToolInput))
				return `{}`
			})
			ctrl := gomock.NewController(t)
			mockConn := agentconnmock.NewMockAgentConn(ctrl)
			setupToolExecutionAgentConn(t, mockConn)
			mockConn.EXPECT().ReadFileLines(gomock.Any(), "/tmp/harmless.txt", int64(1), int64(0), gomock.Any()).
				Return(workspacesdk.ReadFileLinesResponse{
					Success: true, FileSize: 4, TotalLines: 1, LinesRead: 1, Content: "data",
				}, nil).
				Times(1)
			server := newTestServer(t, db, ps, uuid.New(), func(cfg *chatd.Config) {
				cfg.AIBridgeTransportFactory = chatAIGatewayTransportFactoryPointer(chattest.NewMockAIBridgeTransport(t, openAIURL))
				cfg.HookDispatcher = newHookDispatcher(t, db, consumer)
				cfg.AgentConn = func(_ context.Context, agentID uuid.UUID) (workspacesdk.AgentConn, func(), error) {
					require.Equal(t, dbAgent.ID, agentID)
					return mockConn, func() {}, nil
				}
			})
			server.Start()
			waitForChatStatus(ctx, t, db, chatID, database.ChatStatusWaiting)

			require.Positive(t, hookCalls.Load())
			result := requireToolResultPart(t, chatToolParts(ctx, t, db, chatID), "read_file")
			require.False(t, result.IsError)

			dispatches, err := db.ListChatHookDispatchesByChatID(ctx, chatID)
			require.NoError(t, err)
			var freshDispatches int
			for _, dispatch := range dispatches {
				if dispatch.Event != string(agenthooks.EventPreToolUse) {
					continue
				}
				if dispatch.ID == staleDispatchID {
					require.False(t, dispatch.EffectsAppliedAt.Valid)
					continue
				}
				freshDispatches++
				require.True(t, dispatch.EffectsAppliedAt.Valid)
			}
			require.Equal(t, 1, freshDispatches)
		})
	}
}

func recordPreToolUseDecision(
	ctx context.Context,
	t *testing.T,
	db database.Store,
	chatID uuid.UUID,
	ownerID uuid.UUID,
	turnID uuid.UUID,
	toolUseID string,
	toolName string,
	toolInput json.RawMessage,
	decision agenthooks.PermissionDecision,
	reason string,
	override json.RawMessage,
) uuid.UUID {
	t.Helper()
	return recordPreToolUseResponse(ctx, t, db, chatID, ownerID, turnID, toolUseID, toolName, toolInput, decision, reason, override, agenthooks.Response{})
}

func recordPreToolUseResponse(
	ctx context.Context,
	t *testing.T,
	db database.Store,
	chatID uuid.UUID,
	ownerID uuid.UUID,
	turnID uuid.UUID,
	toolUseID string,
	toolName string,
	toolInput json.RawMessage,
	decision agenthooks.PermissionDecision,
	reason string,
	override json.RawMessage,
	response agenthooks.Response,
) uuid.UUID {
	t.Helper()
	var allowedTools json.RawMessage
	if response.AllowedTools != nil {
		encoded, err := json.Marshal(response.AllowedTools)
		require.NoError(t, err)
		allowedTools = encoded
	}
	dispatchID := uuid.New()
	_, err := db.InsertChatHookDispatch(ctx, database.InsertChatHookDispatchParams{
		ID:        dispatchID,
		ChatID:    chatID,
		Event:     string(agenthooks.EventPreToolUse),
		TurnID:    uuid.NullUUID{UUID: turnID, Valid: true},
		ToolUseID: sql.NullString{String: toolUseID, Valid: true},
		ToolName:  sql.NullString{String: toolName, Valid: toolName != ""},
		OwnerID:   ownerID,
		StartedAt: time.Now(),
	})
	require.NoError(t, err)
	_, err = db.FinalizeChatHookDispatch(ctx, database.FinalizeChatHookDispatchParams{
		FinishedAt:     time.Now(),
		Result:         "ok",
		Decision:       sql.NullString{String: string(decision), Valid: true},
		DecisionReason: sql.NullString{String: reason, Valid: reason != ""},
		InputOverride:  nullRawMessage(override),
		OriginalInput:  nullRawMessage(toolInput),
		ModelContext:   sql.NullString{String: response.ModelContext, Valid: response.ModelContext != ""},
		UserMessage:    sql.NullString{String: response.UserMessage, Valid: response.UserMessage != ""},
		AllowedTools:   nullRawMessage(allowedTools),
		EndChat:        sql.NullBool{Bool: response.EndChat, Valid: response.EndChat},
		ID:             dispatchID,
		ChatID:         chatID,
		OwnerID:        ownerID,
	})
	require.NoError(t, err)
	return dispatchID
}

func TestPreToolUseHookReplaysOverrideForOriginalInput(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitLong)
	db, ps := dbtestutil.NewDB(t)
	openAIURL := chattest.NewOpenAI(t, func(req *chattest.OpenAIRequest) chattest.OpenAIResponse {
		if !req.Stream {
			return chattest.OpenAINonStreamingResponse("title")
		}
		return chattest.OpenAIStreamingResponse(chattest.OpenAITextChunks("done")...)
	})
	user, org, model := seedChatDependenciesWithProvider(t, db, "openai-compat", openAIURL)
	ws, dbAgent := seedWorkspaceWithAgent(t, db, user.ID)
	chatID := seedPendingToolCall(ctx, t, db, ps, pendingToolCallSeed{
		OrganizationID: org.ID,
		OwnerID:        user.ID,
		WorkspaceID:    ws.ID,
		AgentID:        dbAgent.ID,
		ModelConfigID:  model.ID,
		ToolCallID:     "call_recorded_override",
		ToolName:       "read_file",
		ToolInput:      `{"path":"/tmp/original.txt"}`,
	})
	messages, err := db.GetChatMessagesForPromptByChatID(ctx, chatID)
	require.NoError(t, err)
	require.True(t, messages[0].TurnID.Valid)
	dispatchID := recordPreToolUseDecision(
		ctx,
		t,
		db,
		chatID,
		user.ID,
		messages[0].TurnID.UUID,
		"call_recorded_override",
		"read_file",
		json.RawMessage(`{"path":"/tmp/original.txt"}`),
		agenthooks.PermissionAllow,
		"",
		json.RawMessage(`{"path":"/tmp/reviewed.txt"}`),
	)

	var hookCalls atomic.Int32
	consumer := preToolUseConsumer(t, func(agenthooks.PreToolUseData) string {
		hookCalls.Add(1)
		return `{"permission":{"decision":"allow","input_override":{"path":"/tmp/reviewed.txt"}}}`
	})
	ctrl := gomock.NewController(t)
	mockConn := agentconnmock.NewMockAgentConn(ctrl)
	setupToolExecutionAgentConn(t, mockConn)
	mockConn.EXPECT().ReadFileLines(gomock.Any(), "/tmp/reviewed.txt", int64(1), int64(0), gomock.Any()).
		Return(workspacesdk.ReadFileLinesResponse{
			Success: true, FileSize: 4, TotalLines: 1, LinesRead: 1, Content: "data",
		}, nil).
		Times(1)
	server := newTestServer(t, db, ps, uuid.New(), func(cfg *chatd.Config) {
		cfg.AIBridgeTransportFactory = chatAIGatewayTransportFactoryPointer(chattest.NewMockAIBridgeTransport(t, openAIURL))
		cfg.HookDispatcher = newHookDispatcher(t, db, consumer)
		cfg.AgentConn = func(_ context.Context, agentID uuid.UUID) (workspacesdk.AgentConn, func(), error) {
			require.Equal(t, dbAgent.ID, agentID)
			return mockConn, func() {}, nil
		}
	})
	server.Start()
	waitForChatStatus(ctx, t, db, chatID, database.ChatStatusWaiting)

	require.Zero(t, hookCalls.Load())
	call := requireToolCallPart(t, chatToolParts(ctx, t, db, chatID), "read_file")
	require.JSONEq(t, `{"path":"/tmp/reviewed.txt"}`, string(call.Args))
	result := requireToolResultPart(t, chatToolParts(ctx, t, db, chatID), "read_file")
	require.False(t, result.IsError)

	dispatches, err := db.ListChatHookDispatchesByChatID(ctx, chatID)
	require.NoError(t, err)
	var preToolDispatches int
	var recordedDispatchFound bool
	for _, dispatch := range dispatches {
		if dispatch.Event != string(agenthooks.EventPreToolUse) {
			continue
		}
		preToolDispatches++
		if dispatch.ID == dispatchID {
			recordedDispatchFound = true
			require.True(t, dispatch.EffectsAppliedAt.Valid)
		}
	}
	require.True(t, recordedDispatchFound)
	require.Equal(t, 1, preToolDispatches)
}

func TestPreToolUseHookPendingPolicyNarrowsSpawnedChild(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitLong)
	db, ps := dbtestutil.NewDB(t)
	openAIURL := chattest.NewOpenAI(t, func(req *chattest.OpenAIRequest) chattest.OpenAIResponse {
		if !req.Stream {
			return chattest.OpenAINonStreamingResponse("title")
		}
		return chattest.OpenAIStreamingResponse(chattest.OpenAITextChunks("done")...)
	})
	user, org, model := seedChatDependenciesWithProvider(t, db, "openai-compat", openAIURL)
	chatID := seedPendingToolCall(ctx, t, db, ps, pendingToolCallSeed{
		OrganizationID: org.ID,
		OwnerID:        user.ID,
		ModelConfigID:  model.ID,
		ToolCallID:     "call_spawn_with_policy",
		ToolName:       "spawn_agent",
		ToolInput:      `{"type":"general","prompt":"inspect the workspace","title":"child"}`,
	})
	require.NoError(t, db.UpdateChatHookAllowedTools(ctx, database.UpdateChatHookAllowedToolsParams{
		ID:               chatID,
		HookAllowedTools: nullRawMessage([]byte(`["read_file","spawn_agent"]`)),
	}))

	consumer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request agenthooks.Request
		require.NoError(t, json.NewDecoder(r.Body).Decode(&request))
		response := `{}`
		if request.Type == agenthooks.EventPreToolUse {
			decoded, err := request.Decode()
			require.NoError(t, err)
			data := decoded.(*agenthooks.PreToolUseData)
			require.Equal(t, "call_spawn_with_policy", data.ToolUseID)
			require.Equal(t, "spawn_agent", data.ToolName)
			response = `{"allowed_tools":["read_file"]}`
		}
		_, err := w.Write([]byte(response))
		require.NoError(t, err)
	}))
	t.Cleanup(consumer.Close)

	server := newTestServer(t, db, ps, uuid.New(), func(cfg *chatd.Config) {
		cfg.AIBridgeTransportFactory = chatAIGatewayTransportFactoryPointer(chattest.NewMockAIBridgeTransport(t, openAIURL))
		cfg.HookDispatcher = newHookDispatcher(t, db, consumer)
	})
	server.Start()
	waitForChatStatus(ctx, t, db, chatID, database.ChatStatusWaiting)

	parent, err := db.GetChatByID(ctx, chatID)
	require.NoError(t, err)
	require.JSONEq(t, `["read_file"]`, string(parent.HookAllowedTools.RawMessage))
	children, err := db.GetChildChatsByParentIDs(ctx, database.GetChildChatsByParentIDsParams{
		ParentIds: []uuid.UUID{chatID},
	})
	require.NoError(t, err)
	require.Len(t, children, 1)
	child := children[0].Chat
	require.JSONEq(t, `["read_file"]`, string(child.HookAllowedTools.RawMessage))

	dispatch := lifecycleDispatch(t, db, chatID, agenthooks.EventPreToolUse)
	require.Equal(t, "call_spawn_with_policy", dispatch.ToolUseID.String)
	require.True(t, dispatch.EffectsAppliedAt.Valid)
}

func TestPreToolUseHookReplaysUnappliedEffects(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitLong)
	db, ps := dbtestutil.NewDB(t)
	openAIURL := chattest.NewOpenAI(t, func(req *chattest.OpenAIRequest) chattest.OpenAIResponse {
		if !req.Stream {
			return chattest.OpenAINonStreamingResponse("title")
		}
		return chattest.OpenAIStreamingResponse(chattest.OpenAITextChunks("done")...)
	})
	user, org, model := seedChatDependenciesWithProvider(t, db, "openai-compat", openAIURL)
	ws, dbAgent := seedWorkspaceWithAgent(t, db, user.ID)
	chatID := seedPendingToolCall(ctx, t, db, ps, pendingToolCallSeed{
		OrganizationID: org.ID,
		OwnerID:        user.ID,
		WorkspaceID:    ws.ID,
		AgentID:        dbAgent.ID,
		ModelConfigID:  model.ID,
		ToolCallID:     "call_replay_effects",
		ToolName:       "read_file",
		ToolInput:      `{"path":"/tmp/a.txt"}`,
	})
	messages, err := db.GetChatMessagesForPromptByChatID(ctx, chatID)
	require.NoError(t, err)
	require.True(t, messages[0].TurnID.Valid)
	allowed := []string{"read_file"}
	replayedDispatchID := recordPreToolUseResponse(ctx, t, db, chatID, user.ID, messages[0].TurnID.UUID, "call_replay_effects", "read_file", json.RawMessage(`{"path":"/tmp/a.txt"}`), agenthooks.PermissionAllow, "", nil, agenthooks.Response{
		ModelContext: "recorded context",
		UserMessage:  "recorded notice",
		AllowedTools: &allowed,
	})
	staleDispatchID := recordPreToolUseResponse(ctx, t, db, chatID, user.ID, messages[0].TurnID.UUID, "call_replay_effects", "execute", json.RawMessage(`{"path":"/tmp/a.txt"}`), agenthooks.PermissionDeny, "stale decision", nil, agenthooks.Response{
		ModelContext: "stale context",
	})

	var preToolUseCalls atomic.Int32
	consumer := preToolUseConsumer(t, func(agenthooks.PreToolUseData) string {
		preToolUseCalls.Add(1)
		return `{}`
	})
	ctrl := gomock.NewController(t)
	mockConn := agentconnmock.NewMockAgentConn(ctrl)
	setupToolExecutionAgentConn(t, mockConn)
	mockConn.EXPECT().ReadFileLines(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		Return(workspacesdk.ReadFileLinesResponse{Success: true}, nil).AnyTimes()
	server := newTestServer(t, db, ps, uuid.New(), func(cfg *chatd.Config) {
		cfg.AIBridgeTransportFactory = chatAIGatewayTransportFactoryPointer(chattest.NewMockAIBridgeTransport(t, openAIURL))
		cfg.HookDispatcher = newHookDispatcher(t, db, consumer)
		cfg.AgentConn = func(_ context.Context, agentID uuid.UUID) (workspacesdk.AgentConn, func(), error) {
			require.Equal(t, dbAgent.ID, agentID)
			return mockConn, func() {}, nil
		}
	})
	server.Start()
	waitForChatStatus(ctx, t, db, chatID, database.ChatStatusWaiting)

	require.Zero(t, preToolUseCalls.Load(), "recorded decision must not re-dispatch")
	result := requireToolResultPart(t, chatToolParts(ctx, t, db, chatID), "read_file")
	require.False(t, result.IsError)

	promptRows, err := db.GetChatMessagesForPromptByChatID(ctx, chatID)
	require.NoError(t, err)
	var contextCount, staleContextCount int
	for _, row := range promptRows {
		switch hookMessageText(t, row) {
		case "recorded context":
			contextCount++
		case "stale context":
			staleContextCount++
		}
	}
	var noticeCount int
	for _, row := range chatMessages(ctx, t, db, chatID) {
		if hookMessageText(t, row) == "recorded notice" {
			noticeCount++
		}
	}
	require.Equal(t, 1, contextCount, "replayed model context must commit exactly once")
	require.Zero(t, staleContextCount)
	require.Equal(t, 1, noticeCount, "replayed user notice must commit exactly once")
	updated, err := db.GetChatByID(ctx, chatID)
	require.NoError(t, err)
	require.JSONEq(t, `["read_file"]`, string(updated.HookAllowedTools.RawMessage))
	dispatches, err := db.ListChatHookDispatchesByChatID(ctx, chatID)
	require.NoError(t, err)
	var foundReplayed, foundStale bool
	for _, dispatch := range dispatches {
		switch dispatch.ID {
		case replayedDispatchID:
			foundReplayed = true
			require.True(t, dispatch.EffectsAppliedAt.Valid)
		case staleDispatchID:
			foundStale = true
			require.False(t, dispatch.EffectsAppliedAt.Valid)
		}
	}
	require.True(t, foundReplayed)
	require.True(t, foundStale)
}

func TestPreToolUseHookSkipsAppliedEffects(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitLong)
	db, ps := dbtestutil.NewDB(t)
	openAIURL := chattest.NewOpenAI(t, func(req *chattest.OpenAIRequest) chattest.OpenAIResponse {
		if !req.Stream {
			return chattest.OpenAINonStreamingResponse("title")
		}
		return chattest.OpenAIStreamingResponse(chattest.OpenAITextChunks("done")...)
	})
	user, org, model := seedChatDependenciesWithProvider(t, db, "openai-compat", openAIURL)
	ws, dbAgent := seedWorkspaceWithAgent(t, db, user.ID)
	chatID := seedPendingToolCall(ctx, t, db, ps, pendingToolCallSeed{
		OrganizationID: org.ID,
		OwnerID:        user.ID,
		WorkspaceID:    ws.ID,
		AgentID:        dbAgent.ID,
		ModelConfigID:  model.ID,
		ToolCallID:     "call_applied_effects",
		ToolName:       "read_file",
		ToolInput:      `{"path":"/tmp/a.txt"}`,
	})
	messages, err := db.GetChatMessagesForPromptByChatID(ctx, chatID)
	require.NoError(t, err)
	require.True(t, messages[0].TurnID.Valid)
	allowed := []string{"read_file"}
	dispatchID := recordPreToolUseResponse(ctx, t, db, chatID, user.ID, messages[0].TurnID.UUID, "call_applied_effects", "read_file", json.RawMessage(`{"path":"/tmp/a.txt"}`), agenthooks.PermissionAllow, "", nil, agenthooks.Response{
		ModelContext: "recorded context",
		AllowedTools: &allowed,
	})
	require.NoError(t, db.MarkChatHookDispatchEffectsApplied(ctx, database.MarkChatHookDispatchEffectsAppliedParams{
		ChatID:      chatID,
		DispatchIds: []uuid.UUID{dispatchID},
	}))

	var preToolUseCalls atomic.Int32
	consumer := preToolUseConsumer(t, func(agenthooks.PreToolUseData) string {
		preToolUseCalls.Add(1)
		return `{}`
	})
	ctrl := gomock.NewController(t)
	mockConn := agentconnmock.NewMockAgentConn(ctrl)
	setupToolExecutionAgentConn(t, mockConn)
	mockConn.EXPECT().ReadFileLines(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		Return(workspacesdk.ReadFileLinesResponse{Success: true}, nil).AnyTimes()
	server := newTestServer(t, db, ps, uuid.New(), func(cfg *chatd.Config) {
		cfg.AIBridgeTransportFactory = chatAIGatewayTransportFactoryPointer(chattest.NewMockAIBridgeTransport(t, openAIURL))
		cfg.HookDispatcher = newHookDispatcher(t, db, consumer)
		cfg.AgentConn = func(_ context.Context, agentID uuid.UUID) (workspacesdk.AgentConn, func(), error) {
			require.Equal(t, dbAgent.ID, agentID)
			return mockConn, func() {}, nil
		}
	})
	server.Start()
	waitForChatStatus(ctx, t, db, chatID, database.ChatStatusWaiting)

	require.Zero(t, preToolUseCalls.Load(), "recorded decision must not re-dispatch")
	result := requireToolResultPart(t, chatToolParts(ctx, t, db, chatID), "read_file")
	require.False(t, result.IsError)

	promptRows, err := db.GetChatMessagesForPromptByChatID(ctx, chatID)
	require.NoError(t, err)
	for _, row := range promptRows {
		require.NotEqual(t, "recorded context", hookMessageText(t, row), "applied effects must not replay")
	}
	updated, err := db.GetChatByID(ctx, chatID)
	require.NoError(t, err)
	require.False(t, updated.HookAllowedTools.Valid, "applied allowed_tools must not replay")
}

func TestPreToolUseHookResumeFallback(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitLong)
	db, ps := dbtestutil.NewDB(t)
	var modelCalls atomic.Int32
	openAIURL := chattest.NewOpenAI(t, func(req *chattest.OpenAIRequest) chattest.OpenAIResponse {
		if !req.Stream {
			return chattest.OpenAINonStreamingResponse("title")
		}
		modelCalls.Add(1)
		return chattest.OpenAIStreamingResponse(chattest.OpenAITextChunks("done")...)
	})
	user, org, model := seedChatDependenciesWithProvider(t, db, "openai-compat", openAIURL)
	ws, dbAgent := seedWorkspaceWithAgent(t, db, user.ID)
	chatID := seedPendingToolCall(ctx, t, db, ps, pendingToolCallSeed{
		OrganizationID: org.ID,
		OwnerID:        user.ID,
		WorkspaceID:    ws.ID,
		AgentID:        dbAgent.ID,
		ModelConfigID:  model.ID,
		ToolCallID:     "call_resume_fallback",
		ToolName:       "read_file",
		ToolInput:      `{"path":"/tmp/original.txt"}`,
	})

	var hookCalls atomic.Int32
	consumer := preToolUseConsumer(t, func(data agenthooks.PreToolUseData) string {
		hookCalls.Add(1)
		require.Equal(t, "call_resume_fallback", data.ToolUseID)
		return `{"permission":{"decision":"allow","input_override":{"path":"/tmp/resume.txt"}}}`
	})
	ctrl := gomock.NewController(t)
	mockConn := agentconnmock.NewMockAgentConn(ctrl)
	setupToolExecutionAgentConn(t, mockConn)
	mockConn.EXPECT().ReadFileLines(gomock.Any(), "/tmp/resume.txt", int64(1), int64(0), gomock.Any()).
		Return(workspacesdk.ReadFileLinesResponse{
			Success: true, FileSize: 4, TotalLines: 1, LinesRead: 1, Content: "data",
		}, nil).
		Times(1)

	server := newTestServer(t, db, ps, uuid.New(), func(cfg *chatd.Config) {
		cfg.AIBridgeTransportFactory = chatAIGatewayTransportFactoryPointer(chattest.NewMockAIBridgeTransport(t, openAIURL))
		cfg.HookDispatcher = newHookDispatcher(t, db, consumer)
		cfg.AgentConn = func(_ context.Context, agentID uuid.UUID) (workspacesdk.AgentConn, func(), error) {
			require.Equal(t, dbAgent.ID, agentID)
			return mockConn, func() {}, nil
		}
	})
	server.Start()
	waitForChatStatus(ctx, t, db, chatID, database.ChatStatusWaiting)

	require.Equal(t, int32(1), hookCalls.Load())
	require.Equal(t, int32(1), modelCalls.Load())
	dispatch := lifecycleDispatch(t, db, chatID, agenthooks.EventPreToolUse)
	require.Equal(t, "allow", dispatch.Decision.String)
	call := requireToolCallPart(t, chatToolParts(ctx, t, db, chatID), "read_file")
	require.JSONEq(t, `{"path":"/tmp/resume.txt"}`, string(call.Args))
	result := requireToolResultPart(t, chatToolParts(ctx, t, db, chatID), "read_file")
	require.False(t, result.IsError)
}

type pendingToolCallSeed struct {
	OrganizationID uuid.UUID
	OwnerID        uuid.UUID
	WorkspaceID    uuid.UUID
	AgentID        uuid.UUID
	ModelConfigID  uuid.UUID
	ToolCallID     string
	ToolName       string
	ToolInput      string
	DynamicTools   json.RawMessage
}

func seedPendingToolCall(
	ctx context.Context,
	t *testing.T,
	db database.Store,
	ps dbpubsub.Pubsub,
	seed pendingToolCallSeed,
) uuid.UUID {
	t.Helper()
	userContent, err := chatprompt.MarshalParts([]codersdk.ChatMessagePart{codersdk.ChatMessageText("resume")})
	require.NoError(t, err)
	created, err := chatstate.CreateChat(ctx, db, ps, chatstate.CreateChatInput{
		OrganizationID:    seed.OrganizationID,
		OwnerID:           seed.OwnerID,
		WorkspaceID:       uuid.NullUUID{UUID: seed.WorkspaceID, Valid: seed.WorkspaceID != uuid.Nil},
		AgentID:           uuid.NullUUID{UUID: seed.AgentID, Valid: seed.AgentID != uuid.Nil},
		LastModelConfigID: seed.ModelConfigID,
		Title:             "pending-tool-call",
		DynamicTools:      nullRawMessage(seed.DynamicTools),
		ClientType:        database.ChatClientTypeApi,
		InitialMessages: []chatstate.Message{
			{
				Role:           database.ChatMessageRoleUser,
				Content:        userContent,
				Visibility:     database.ChatMessageVisibilityBoth,
				TurnID:         uuid.NullUUID{UUID: uuid.New(), Valid: true},
				ContentVersion: chatprompt.CurrentContentVersion,
				CreatedBy:      uuid.NullUUID{UUID: seed.OwnerID, Valid: true},
				ModelConfigID:  uuid.NullUUID{UUID: seed.ModelConfigID, Valid: true},
			},
		},
	})
	require.NoError(t, err)
	assistantContent, err := chatprompt.MarshalParts([]codersdk.ChatMessagePart{
		{
			Type:       codersdk.ChatMessagePartTypeToolCall,
			ToolCallID: seed.ToolCallID,
			ToolName:   seed.ToolName,
			Args:       json.RawMessage(seed.ToolInput),
		},
	})
	require.NoError(t, err)
	machine := chatstate.NewChatMachine(db, ps, created.Chat.ID)
	require.NoError(t, machine.Update(ctx, func(tx *chatstate.Tx, _ database.Store) error {
		_, err := tx.CommitStep(chatstate.CommitStepInput{Messages: []chatstate.Message{
			{
				Role:           database.ChatMessageRoleAssistant,
				Content:        assistantContent,
				Visibility:     database.ChatMessageVisibilityBoth,
				TurnID:         created.InitialMessages[0].TurnID,
				ContentVersion: chatprompt.CurrentContentVersion,
				ModelConfigID:  uuid.NullUUID{UUID: seed.ModelConfigID, Valid: true},
			},
		}})
		return err
	}))
	return created.Chat.ID
}

func TestPreToolUseHookDynamicDeny(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitLong)
	db, ps := dbtestutil.NewDB(t)
	var modelCalls atomic.Int32
	openAIURL := chattest.NewOpenAI(t, func(req *chattest.OpenAIRequest) chattest.OpenAIResponse {
		if !req.Stream {
			return chattest.OpenAINonStreamingResponse("title")
		}
		if modelCalls.Add(1) == 1 {
			chunk := chattest.OpenAIToolCallChunk("my_dynamic_tool", `{"query":"test"}`)
			chunk.Choices[0].ToolCalls[0].ID = "call_dynamic_denied"
			return chattest.OpenAIStreamingResponse(chunk)
		}
		return chattest.OpenAIStreamingResponse(chattest.OpenAITextChunks("replanned")...)
	})
	user, org, model := seedChatDependenciesWithProvider(t, db, "openai-compat", openAIURL)
	consumer := preToolUseConsumer(t, func(data agenthooks.PreToolUseData) string {
		require.Equal(t, "call_dynamic_denied", data.ToolUseID)
		return `{"permission":{"decision":"deny","reason":"dynamic denied"}}`
	})

	server := newActiveTestServer(t, db, ps, func(cfg *chatd.Config) {
		cfg.AIBridgeTransportFactory = chatAIGatewayTransportFactoryPointer(chattest.NewMockAIBridgeTransport(t, openAIURL))
		cfg.HookDispatcher = newHookDispatcher(t, db, consumer)
	})
	chat, err := server.CreateChat(ctx, chatd.CreateOptions{
		OrganizationID: org.ID,
		OwnerID:        user.ID,
		Title:          "pre-tool-use-dynamic-deny",
		ModelConfigID:  model.ID,
		InitialUserContent: []codersdk.ChatMessagePart{
			codersdk.ChatMessageText("call the dynamic tool"),
		},
		DynamicTools: dynamicToolJSON(t, "my_dynamic_tool"),
	})
	require.NoError(t, err)
	waitForChatStatus(ctx, t, db, chat.ID, database.ChatStatusWaiting)

	chatResult, err := db.GetChatByID(ctx, chat.ID)
	require.NoError(t, err)
	require.False(t, chatResult.RequiresActionDeadlineAt.Valid)
	require.Equal(t, int32(2), modelCalls.Load())
	result := requireToolResultPart(t, chatToolParts(ctx, t, db, chat.ID), "my_dynamic_tool")
	require.True(t, result.IsError)
	require.Contains(t, string(result.Result), "DENIED: dynamic denied")
}

func TestHookAllowedToolsExcludesDynamicTool(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitLong)
	db, ps := dbtestutil.NewDB(t)
	var modelCalls atomic.Int32
	openAIURL := chattest.NewOpenAI(t, func(req *chattest.OpenAIRequest) chattest.OpenAIResponse {
		if !req.Stream {
			return chattest.OpenAINonStreamingResponse("title")
		}
		if modelCalls.Add(1) == 1 {
			chunk := chattest.OpenAIToolCallChunk("my_dynamic_tool", `{"query":"test"}`)
			chunk.Choices[0].ToolCalls[0].ID = "call_excluded_dynamic"
			return chattest.OpenAIStreamingResponse(chunk)
		}
		return chattest.OpenAIStreamingResponse(chattest.OpenAITextChunks("replanned")...)
	})
	user, org, model := seedChatDependenciesWithProvider(t, db, "openai-compat", openAIURL)
	consumer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request agenthooks.Request
		require.NoError(t, json.NewDecoder(r.Body).Decode(&request))
		body := `{}`
		if request.Type == agenthooks.EventUserPromptSubmit {
			body = `{"allowed_tools":["read_file"]}`
		}
		_, err := w.Write([]byte(body))
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
		Title:          "allowed-tools-excludes-dynamic",
		ModelConfigID:  model.ID,
		InitialUserContent: []codersdk.ChatMessagePart{
			codersdk.ChatMessageText("call the dynamic tool"),
		},
		DynamicTools: dynamicToolJSON(t, "my_dynamic_tool"),
	})
	require.NoError(t, err)
	waitForChatStatus(ctx, t, db, chat.ID, database.ChatStatusWaiting)

	chatResult, err := db.GetChatByID(ctx, chat.ID)
	require.NoError(t, err)
	require.JSONEq(t, `["read_file"]`, string(chatResult.HookAllowedTools.RawMessage))
	require.False(t, chatResult.RequiresActionDeadlineAt.Valid)
	require.Equal(t, int32(2), modelCalls.Load())
	result := requireToolResultPart(t, chatToolParts(ctx, t, db, chat.ID), "my_dynamic_tool")
	require.True(t, result.IsError)
	require.Contains(t, string(result.Result), "Tool not active in this turn")
}

func preToolUseConsumer(t *testing.T, response func(agenthooks.PreToolUseData) string) *httptest.Server {
	t.Helper()
	consumer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request agenthooks.Request
		require.NoError(t, json.NewDecoder(r.Body).Decode(&request))
		if request.Type != agenthooks.EventPreToolUse {
			_, err := w.Write([]byte(`{}`))
			require.NoError(t, err)
			return
		}
		decoded, err := request.Decode()
		require.NoError(t, err)
		data, ok := decoded.(*agenthooks.PreToolUseData)
		require.True(t, ok)
		_, err = w.Write([]byte(response(*data)))
		require.NoError(t, err)
	}))
	t.Cleanup(consumer.Close)
	return consumer
}
