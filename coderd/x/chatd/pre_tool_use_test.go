package chatd_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

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
	"github.com/coder/coder/v2/codersdk/workspacesdk"
	"github.com/coder/coder/v2/codersdk/workspacesdk/agentconnmock"
	"github.com/coder/coder/v2/codersdk/x/agenthooks"
	"github.com/coder/coder/v2/testutil"
)

func TestPreToolUseHookAllow(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		response     string
		expectedPath string
	}{
		{
			name:         "passthrough",
			response:     `{}`,
			expectedPath: "/tmp/before.txt",
		},
		{
			name:         "override",
			response:     `{"permission":{"decision":"allow","input_override":{"path":"/tmp/after.txt"}},"user_message":"tool approved"}`,
			expectedPath: "/tmp/after.txt",
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
		})
	}
}

func TestPreToolUseHookOverrideIsPersistedOnceNeverTheOriginal(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitLong)
	db, ps := dbtestutil.NewDB(t)
	var modelCalls atomic.Int32
	openAIURL := chattest.NewOpenAI(t, func(req *chattest.OpenAIRequest) chattest.OpenAIResponse {
		if !req.Stream {
			return chattest.OpenAINonStreamingResponse("title")
		}
		if modelCalls.Add(1) == 1 {
			chunk := chattest.OpenAIToolCallChunk("read_file", `{"path":"/tmp/secret.txt"}`)
			chunk.Choices[0].ToolCalls[0].ID = "call_override"
			return chattest.OpenAIStreamingResponse(chunk)
		}
		return chattest.OpenAIStreamingResponse(chattest.OpenAITextChunks("done")...)
	})
	user, org, model := seedChatDependenciesWithProvider(t, db, "openai-compat", openAIURL)
	ws, dbAgent := seedWorkspaceWithAgent(t, db, user.ID)

	consumer := preToolUseConsumer(t, func(data agenthooks.PreToolUseData) string {
		require.JSONEq(t, `{"path":"/tmp/secret.txt"}`, string(data.ToolInput))
		return `{"permission":{"decision":"allow","input_override":{"path":"/tmp/public.txt"}}}`
	})

	ctrl := gomock.NewController(t)
	mockConn := agentconnmock.NewMockAgentConn(ctrl)
	setupToolExecutionAgentConn(t, mockConn)
	mockConn.EXPECT().ReadFileLines(gomock.Any(), "/tmp/public.txt", int64(1), int64(0), gomock.Any()).
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
		Title:          "pre-tool-use-override-persisted-once",
		ModelConfigID:  model.ID,
		InitialUserContent: []codersdk.ChatMessagePart{
			codersdk.ChatMessageText("read the file"),
		},
	})
	require.NoError(t, err)
	waitForChatStatus(ctx, t, db, chat.ID, database.ChatStatusWaiting)

	call := requireToolCallPart(t, chatToolParts(ctx, t, db, chat.ID), "read_file")
	require.JSONEq(t, `{"path":"/tmp/public.txt"}`, string(call.Args))

	messages := chatMessages(ctx, t, db, chat.ID)
	promptMessages, err := db.GetChatMessagesForPromptByChatID(ctx, chat.ID)
	require.NoError(t, err)
	for _, message := range append(messages, promptMessages...) {
		require.NotContains(t, string(message.Content.RawMessage), "/tmp/secret.txt")
	}
}

// Malformed tool input cannot be represented in a hook payload, so it must
// become a tool error the model can retry rather than failing the turn.
func TestPreToolUseHookMalformedToolInputStaysRecoverable(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitLong)
	db, ps := dbtestutil.NewDB(t)
	var modelCalls atomic.Int32
	openAIURL := chattest.NewOpenAI(t, func(req *chattest.OpenAIRequest) chattest.OpenAIResponse {
		if !req.Stream {
			return chattest.OpenAINonStreamingResponse("title")
		}
		if modelCalls.Add(1) == 1 {
			chunk := chattest.OpenAIToolCallChunk("read_file", `{"path":`)
			chunk.Choices[0].ToolCalls[0].ID = "call_malformed"
			return chattest.OpenAIStreamingResponse(chunk)
		}
		return chattest.OpenAIStreamingResponse(chattest.OpenAITextChunks("recovered")...)
	})
	user, org, model := seedChatDependenciesWithProvider(t, db, "openai-compat", openAIURL)
	ws, dbAgent := seedWorkspaceWithAgent(t, db, user.ID)

	var hookCalls atomic.Int32
	consumer := preToolUseConsumer(t, func(agenthooks.PreToolUseData) string {
		hookCalls.Add(1)
		return `{}`
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
		Title:          "pre-tool-use-malformed-input",
		ModelConfigID:  model.ID,
		InitialUserContent: []codersdk.ChatMessagePart{
			codersdk.ChatMessageText("read the file"),
		},
	})
	require.NoError(t, err)
	waitForChatStatus(ctx, t, db, chat.ID, database.ChatStatusWaiting)

	require.Zero(t, hookCalls.Load(), "unrepresentable input must not reach the consumer")
	result := requireToolResultPart(t, chatToolParts(ctx, t, db, chat.ID), "read_file")
	require.True(t, result.IsError)

	failed, err := db.GetChatByID(ctx, chat.ID)
	require.NoError(t, err)
	require.False(t, failed.LastError.Valid, "the turn must not fail as a hook dispatch error")
}

// Duplicate tool-use IDs make a hook decision unattributable, so the batch
// must be rejected even when filtering would otherwise hide the duplication.
func TestPreToolUseHookDuplicateToolUseIDWithMalformedSiblingFailsClosed(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitLong)
	db, ps := dbtestutil.NewDB(t)
	openAIURL := chattest.NewOpenAI(t, func(req *chattest.OpenAIRequest) chattest.OpenAIResponse {
		if !req.Stream {
			return chattest.OpenAINonStreamingResponse("title")
		}
		first := chattest.OpenAIToolCallChunk("read_file", `{"path":`)
		first.Choices[0].ToolCalls[0].ID = "call_duplicate"
		second := chattest.OpenAIToolCallChunk("read_file", `{"path":"/tmp/allowed.txt"}`).Choices[0].ToolCalls[0]
		second.ID = "call_duplicate"
		second.Index = 1
		first.Choices[0].ToolCalls = append(first.Choices[0].ToolCalls, second)
		return chattest.OpenAIStreamingResponse(first)
	})
	user, org, model := seedChatDependenciesWithProvider(t, db, "openai-compat", openAIURL)
	ws, dbAgent := seedWorkspaceWithAgent(t, db, user.ID)

	var hookCalls atomic.Int32
	consumer := preToolUseConsumer(t, func(agenthooks.PreToolUseData) string {
		hookCalls.Add(1)
		return `{}`
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
		Title:          "pre-tool-use-duplicate-id",
		ModelConfigID:  model.ID,
		InitialUserContent: []codersdk.ChatMessagePart{
			codersdk.ChatMessageText("read the files"),
		},
	})
	require.NoError(t, err)
	failed := waitForChatStatus(ctx, t, db, chat.ID, database.ChatStatusError)

	require.Zero(t, hookCalls.Load(), "a duplicated ID must be rejected before any dispatch")
	var chatErr codersdk.ChatError
	require.NoError(t, json.Unmarshal(failed.LastError.RawMessage, &chatErr))
	require.Contains(t, chatErr.Message, "duplicate tool use ID")
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
	require.Contains(t, string(result.Result), "blocked by an external policy")
	require.Contains(t, string(result.Result), "Reason: blocked by policy.")
	require.NotContains(t, string(result.Result), "Do not read secrets.")

	requireNoClientVisibleText(ctx, t, db, chat.ID, "Do not read secrets.")
	requireModelOnlyTextCount(ctx, t, db, chat.ID, "Do not read secrets.", 1)

	messagesMu.Lock()
	modelMessages := append([]chattest.OpenAIMessage(nil), secondMessages...)
	messagesMu.Unlock()
	deniedIndex, contextIndex := -1, -1
	for i, msg := range modelMessages {
		if strings.Contains(msg.Content, "Reason: blocked by policy.") {
			deniedIndex = i
			require.Equal(t, "tool", msg.Role)
		}
		if strings.Contains(msg.Content, "Do not read secrets.") {
			contextIndex = i
			require.Equal(t, "user", msg.Role)
		}
	}
	// The model still receives the denial reason and the hook context,
	// with the context after the tool result so results stay adjacent
	// to the assistant tool calls.
	require.NotEqual(t, -1, deniedIndex)
	require.Greater(t, contextIndex, deniedIndex)
}

func TestPreToolUseHookDenyMixedWithAllowed(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitLong)
	db, ps := dbtestutil.NewDB(t)
	var modelCalls atomic.Int32
	openAIURL := chattest.NewOpenAI(t, func(req *chattest.OpenAIRequest) chattest.OpenAIResponse {
		if !req.Stream {
			return chattest.OpenAINonStreamingResponse("title")
		}
		if modelCalls.Add(1) == 1 {
			first := chattest.OpenAIToolCallChunk("read_file", `{"path":"/tmp/secret.txt"}`)
			first.Choices[0].ToolCalls[0].ID = "call_denied"
			second := chattest.OpenAIToolCallChunk("read_file", `{"path":"/tmp/allowed.txt"}`).Choices[0].ToolCalls[0]
			second.ID = "call_allowed"
			second.Index = 1
			first.Choices[0].ToolCalls = append(first.Choices[0].ToolCalls, second)
			return chattest.OpenAIStreamingResponse(first)
		}
		return chattest.OpenAIStreamingResponse(chattest.OpenAITextChunks("done")...)
	})
	user, org, model := seedChatDependenciesWithProvider(t, db, "openai-compat", openAIURL)
	ws, dbAgent := seedWorkspaceWithAgent(t, db, user.ID)

	consumer := preToolUseConsumer(t, func(data agenthooks.PreToolUseData) string {
		if data.ToolUseID == "call_denied" {
			return `{"permission":{"decision":"deny","reason":"blocked by policy"},"model_context":"Do not read secrets."}`
		}
		return `{}`
	})

	ctrl := gomock.NewController(t)
	mockConn := agentconnmock.NewMockAgentConn(ctrl)
	setupToolExecutionAgentConn(t, mockConn)
	mockConn.EXPECT().ReadFileLines(gomock.Any(), "/tmp/allowed.txt", int64(1), int64(0), gomock.Any()).
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
		Title:          "pre-tool-use-deny-mixed",
		ModelConfigID:  model.ID,
		InitialUserContent: []codersdk.ChatMessagePart{
			codersdk.ChatMessageText("read both files"),
		},
	})
	require.NoError(t, err)
	waitForChatStatus(ctx, t, db, chat.ID, database.ChatStatusWaiting)

	var results []codersdk.ChatMessagePart
	for _, part := range chatToolParts(ctx, t, db, chat.ID) {
		if part.Type == codersdk.ChatMessagePartTypeToolResult {
			results = append(results, part)
		}
	}
	require.Len(t, results, 2)
	// Persisted results keep the assistant call order even though the
	// denied result is synthesized after the executed one.
	require.Equal(t, "call_denied", results[0].ToolCallID)
	require.Equal(t, "call_allowed", results[1].ToolCallID)
	require.True(t, results[0].IsError)
	require.Contains(t, string(results[0].Result), "Reason: blocked by policy.")
	require.NotContains(t, string(results[0].Result), "Do not read secrets.")
	require.False(t, results[1].IsError)

	requireNoClientVisibleText(ctx, t, db, chat.ID, "Do not read secrets.")
	requireModelOnlyTextCount(ctx, t, db, chat.ID, "Do not read secrets.", 1)
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
		return `{"permission":{"decision":"allow","input_override":{"query":"redacted"}},"model_context":"dynamic context","user_message":"dynamic notice"}`
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
			lastError := chatLastErrorMessage(failed.LastError)
			require.Contains(t, lastError, "hook dispatch failed: pre_tool_use: "+tt.result)
			messages := chatMessages(ctx, t, db, chat.ID)
			require.Len(t, messages, 1)
			require.Equal(t, database.ChatMessageRoleUser, messages[0].Role)
		})
	}
}

func TestPreToolUseHookErrorRetryRedispatchesSiblings(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitLong)
	db, ps := dbtestutil.NewDB(t)
	var modelCalls atomic.Int32
	openAIURL := chattest.NewOpenAI(t, func(req *chattest.OpenAIRequest) chattest.OpenAIResponse {
		if !req.Stream {
			return chattest.OpenAINonStreamingResponse("title")
		}
		if modelCalls.Add(1) <= 2 {
			first := chattest.OpenAIToolCallChunk("read_file", `{"path":"/tmp/first.txt"}`)
			first.Choices[0].ToolCalls[0].ID = "call_first"
			second := chattest.OpenAIToolCallChunk("read_file", `{"path":"/tmp/second.txt"}`).Choices[0].ToolCalls[0]
			second.ID = "call_second"
			second.Index = 1
			first.Choices[0].ToolCalls = append(first.Choices[0].ToolCalls, second)
			return chattest.OpenAIStreamingResponse(first)
		}
		return chattest.OpenAIStreamingResponse(chattest.OpenAITextChunks("done")...)
	})
	user, org, model := seedChatDependenciesWithProvider(t, db, "openai-compat", openAIURL)
	ws, dbAgent := seedWorkspaceWithAgent(t, db, user.ID)

	var firstCalls atomic.Int32
	var secondCalls atomic.Int32
	var failSecond atomic.Bool
	failSecond.Store(true)
	consumer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request agenthooks.Request
		require.NoError(t, json.NewDecoder(r.Body).Decode(&request))
		if request.Type != agenthooks.EventPreToolUse {
			_, err := w.Write([]byte(`{}`))
			require.NoError(t, err)
			return
		}
		data := decodeHookData[agenthooks.PreToolUseData](t, request)
		switch data.ToolUseID {
		case "call_first":
			firstCalls.Add(1)
			_, err := w.Write([]byte(`{"model_context":"first context","user_message":"first notice"}`))
			require.NoError(t, err)
		case "call_second":
			secondCalls.Add(1)
			if failSecond.Load() {
				w.WriteHeader(http.StatusInternalServerError)
				return
			}
			_, err := w.Write([]byte(`{}`))
			require.NoError(t, err)
		default:
			t.Fatalf("unexpected tool use ID %q", data.ToolUseID)
		}
	}))
	t.Cleanup(consumer.Close)

	ctrl := gomock.NewController(t)
	mockConn := agentconnmock.NewMockAgentConn(ctrl)
	setupToolExecutionAgentConn(t, mockConn)
	mockConn.EXPECT().ReadFileLines(gomock.Any(), gomock.Any(), int64(1), int64(0), gomock.Any()).
		Return(workspacesdk.ReadFileLinesResponse{Success: true, Content: "data"}, nil).
		Times(2)

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
		Title:          "pre-tool-use-error-retry",
		ModelConfigID:  model.ID,
		InitialUserContent: []codersdk.ChatMessagePart{
			codersdk.ChatMessageText("read both files"),
		},
	})
	require.NoError(t, err)
	waitForChatStatus(ctx, t, db, chat.ID, database.ChatStatusError)
	require.Equal(t, int32(1), firstCalls.Load())
	require.Equal(t, int32(1), secondCalls.Load())
	require.Len(t, chatMessages(ctx, t, db, chat.ID), 1)

	failSecond.Store(false)
	_, err = server.SendMessage(ctx, chatd.SendMessageOptions{
		ChatID:        chat.ID,
		CreatedBy:     user.ID,
		ModelConfigID: model.ID,
		Content:       []codersdk.ChatMessagePart{codersdk.ChatMessageText("retry")},
	})
	require.NoError(t, err)
	waitForChatStatus(ctx, t, db, chat.ID, database.ChatStatusWaiting)

	require.Equal(t, int32(2), firstCalls.Load())
	require.Equal(t, int32(2), secondCalls.Load())
	promptMessages, err := db.GetChatMessagesForPromptByChatID(ctx, chat.ID)
	require.NoError(t, err)
	var contextCount, noticeCount int
	for _, message := range append(promptMessages, chatMessages(ctx, t, db, chat.ID)...) {
		parts, err := chatprompt.ParseContent(message)
		require.NoError(t, err)
		for _, part := range parts {
			switch part.Text {
			case "first context":
				contextCount++
			case "first notice":
				noticeCount++
			}
		}
	}
	require.Equal(t, 1, contextCount)
	require.Equal(t, 1, noticeCount)
}

func TestPreToolUseHookSettledDecisionDispatchesFresh(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitLong)
	db, ps := dbtestutil.NewDB(t)
	var modelCalls atomic.Int32
	openAIURL := chattest.NewOpenAI(t, func(req *chattest.OpenAIRequest) chattest.OpenAIResponse {
		if !req.Stream {
			return chattest.OpenAINonStreamingResponse("title")
		}
		switch modelCalls.Add(1) {
		case 1, 3:
			chunk := chattest.OpenAIToolCallChunk("read_file", `{"path":"/tmp/file.txt"}`)
			chunk.Choices[0].ToolCalls[0].ID = "call_reused_after_settle"
			return chattest.OpenAIStreamingResponse(chunk)
		default:
			return chattest.OpenAIStreamingResponse(chattest.OpenAITextChunks("done")...)
		}
	})
	user, org, model := seedChatDependenciesWithProvider(t, db, "openai-compat", openAIURL)
	ws, dbAgent := seedWorkspaceWithAgent(t, db, user.ID)
	var hookCalls atomic.Int32
	consumer := preToolUseConsumer(t, func(data agenthooks.PreToolUseData) string {
		hookCalls.Add(1)
		require.Equal(t, "call_reused_after_settle", data.ToolUseID)
		return `{}`
	})

	ctrl := gomock.NewController(t)
	mockConn := agentconnmock.NewMockAgentConn(ctrl)
	setupToolExecutionAgentConn(t, mockConn)
	mockConn.EXPECT().ReadFileLines(gomock.Any(), "/tmp/file.txt", int64(1), int64(0), gomock.Any()).
		Return(workspacesdk.ReadFileLinesResponse{Success: true, Content: "data"}, nil).
		Times(2)

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
		Title:          "pre-tool-use-settled",
		ModelConfigID:  model.ID,
		InitialUserContent: []codersdk.ChatMessagePart{
			codersdk.ChatMessageText("read once"),
		},
	})
	require.NoError(t, err)
	waitForChatStatus(ctx, t, db, chat.ID, database.ChatStatusWaiting)
	require.Equal(t, int32(1), hookCalls.Load())

	_, err = server.SendMessage(ctx, chatd.SendMessageOptions{
		ChatID:        chat.ID,
		CreatedBy:     user.ID,
		ModelConfigID: model.ID,
		Content:       []codersdk.ChatMessagePart{codersdk.ChatMessageText("read again")},
	})
	require.NoError(t, err)
	waitForChatStatus(ctx, t, db, chat.ID, database.ChatStatusWaiting)
	require.Equal(t, int32(2), hookCalls.Load())
}

func TestPreToolUseHookHistoryPendingCallExecutesAsAdmitted(t *testing.T) {
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
		ToolCallID:     "call_resume_fallback",
		ToolName:       "read_file",
		ToolInput:      `{"path":"/tmp/original.txt"}`,
	})

	var hookCalls atomic.Int32
	consumer := preToolUseConsumer(t, func(agenthooks.PreToolUseData) string {
		hookCalls.Add(1)
		return `{"permission":{"decision":"allow","input_override":{"path":"/tmp/resume.txt"}}}`
	})
	ctrl := gomock.NewController(t)
	mockConn := agentconnmock.NewMockAgentConn(ctrl)
	setupToolExecutionAgentConn(t, mockConn)
	mockConn.EXPECT().ReadFileLines(gomock.Any(), "/tmp/original.txt", int64(1), int64(0), gomock.Any()).
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
	require.JSONEq(t, `{"path":"/tmp/original.txt"}`, string(call.Args))
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
		LastModelConfigID: uuid.NullUUID{UUID: seed.ModelConfigID, Valid: true},
		Title:             "pending-tool-call",
		DynamicTools:      nullRawMessage(seed.DynamicTools),
		ClientType:        database.ChatClientTypeApi,
		InitialMessages: []chatstate.Message{
			{
				Role:           database.ChatMessageRoleUser,
				Content:        userContent,
				Visibility:     database.ChatMessageVisibilityBoth,
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
	require.Contains(t, string(result.Result), "Reason: dynamic denied.")
}

func TestPreToolUseHookRejectsAmbiguousToolInput(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitLong)
	db, ps := dbtestutil.NewDB(t)
	var modelCalls atomic.Int32
	openAIURL := chattest.NewOpenAI(t, func(req *chattest.OpenAIRequest) chattest.OpenAIResponse {
		if !req.Stream {
			return chattest.OpenAINonStreamingResponse("title")
		}
		if modelCalls.Add(1) == 1 {
			chunk := chattest.OpenAIToolCallChunk("read_file",
				`{"path":"/tmp/allowed.txt","PATH":"/tmp/secret.txt"}`)
			chunk.Choices[0].ToolCalls[0].ID = "call_ambiguous"
			return chattest.OpenAIStreamingResponse(chunk)
		}
		return chattest.OpenAIStreamingResponse(chattest.OpenAITextChunks("done")...)
	})
	user, org, model := seedChatDependenciesWithProvider(t, db, "openai-compat", openAIURL)
	ws, dbAgent := seedWorkspaceWithAgent(t, db, user.ID)

	var hookCalls atomic.Int32
	consumer := preToolUseConsumer(t, func(agenthooks.PreToolUseData) string {
		hookCalls.Add(1)
		return `{}`
	})

	ctrl := gomock.NewController(t)
	mockConn := agentconnmock.NewMockAgentConn(ctrl)
	setupToolExecutionAgentConn(t, mockConn)
	mockConn.EXPECT().ReadFileLines(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		Times(0)

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
		Title:          "pre-tool-use-ambiguous",
		ModelConfigID:  model.ID,
		InitialUserContent: []codersdk.ChatMessagePart{
			codersdk.ChatMessageText("read the file"),
		},
	})
	require.NoError(t, err)
	waitForChatStatus(ctx, t, db, chat.ID, database.ChatStatusWaiting)

	require.Zero(t, hookCalls.Load())
	result := requireToolResultPart(t, chatToolParts(ctx, t, db, chat.ID), "read_file")
	require.True(t, result.IsError)
	require.Contains(t, string(result.Result), "input is ambiguous")
	require.Contains(t, string(result.Result), "only by case")
}

func TestPreToolUseHookRejectsAmbiguousInputOverride(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitLong)
	db, ps := dbtestutil.NewDB(t)
	openAIURL := chattest.NewOpenAI(t, func(req *chattest.OpenAIRequest) chattest.OpenAIResponse {
		if !req.Stream {
			return chattest.OpenAINonStreamingResponse("title")
		}
		chunk := chattest.OpenAIToolCallChunk("read_file", `{"path":"/tmp/before.txt"}`)
		chunk.Choices[0].ToolCalls[0].ID = "call_override"
		return chattest.OpenAIStreamingResponse(chunk)
	})
	user, org, model := seedChatDependenciesWithProvider(t, db, "openai-compat", openAIURL)
	ws, dbAgent := seedWorkspaceWithAgent(t, db, user.ID)

	consumer := preToolUseConsumer(t, func(agenthooks.PreToolUseData) string {
		return `{"permission":{"decision":"allow","input_override":{"path":"/tmp/after.txt","PATH":"/tmp/secret.txt"}}}`
	})

	ctrl := gomock.NewController(t)
	mockConn := agentconnmock.NewMockAgentConn(ctrl)
	setupToolExecutionAgentConn(t, mockConn)
	mockConn.EXPECT().ReadFileLines(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		Times(0)

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
		Title:          "pre-tool-use-ambiguous-override",
		ModelConfigID:  model.ID,
		InitialUserContent: []codersdk.ChatMessagePart{
			codersdk.ChatMessageText("read the file"),
		},
	})
	require.NoError(t, err)
	waitForChatStatus(ctx, t, db, chat.ID, database.ChatStatusError)

	failed, err := db.GetChatByID(ctx, chat.ID)
	require.NoError(t, err)
	require.Contains(t, chatLastErrorMessage(failed.LastError),
		`hook input override for tool read_file: input key "PATH" differs from schema property "path" only by case`)
}

func requireNoClientVisibleText(ctx context.Context, t *testing.T, db database.Store, chatID uuid.UUID, text string) {
	t.Helper()
	for _, message := range chatMessages(ctx, t, db, chatID) {
		parsed, err := chatprompt.ParseContent(message)
		require.NoError(t, err)
		for _, part := range parsed {
			require.NotContains(t, part.Text, text)
			require.NotContains(t, string(part.Result), text)
			require.NotContains(t, string(part.Args), text)
		}
	}
}

func requireModelOnlyTextCount(ctx context.Context, t *testing.T, db database.Store, chatID uuid.UUID, text string, count int) {
	t.Helper()
	messages, err := db.GetChatMessagesForPromptByChatID(ctx, chatID)
	require.NoError(t, err)
	found := 0
	for _, message := range messages {
		if message.Visibility != database.ChatMessageVisibilityModel {
			continue
		}
		parsed, err := chatprompt.ParseContent(message)
		require.NoError(t, err)
		for _, part := range parsed {
			if strings.Contains(part.Text, text) {
				found++
			}
		}
	}
	require.Equal(t, count, found)
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
		data := decodeHookData[agenthooks.PreToolUseData](t, request)
		var err error
		_, err = w.Write([]byte(response(data)))
		require.NoError(t, err)
	}))
	t.Cleanup(consumer.Close)
	return consumer
}
