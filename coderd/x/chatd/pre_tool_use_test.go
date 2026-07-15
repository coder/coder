package chatd_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"cdr.dev/slog/v3/sloggers/slogtest"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbtestutil"
	"github.com/coder/coder/v2/coderd/x/chatd"
	"github.com/coder/coder/v2/coderd/x/chatd/chathooks"
	"github.com/coder/coder/v2/coderd/x/chatd/chatprompt"
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
		},
		{
			name:         "override",
			response:     `{"permission":{"decision":"allow","input_override":{"path":"/tmp/after.txt"}}}`,
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

			consumer := preToolUseConsumer(t, func(data agenthooks.PreToolUseData) string {
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
				cfg.HookDispatcher = newPreToolUseDispatcher(t, db, consumer)
				cfg.AgentConn = func(_ context.Context, agentID uuid.UUID) (workspacesdk.AgentConn, func(), error) {
					require.Equal(t, dbAgent.ID, agentID)
					return mockConn, func() {}, nil
				}
			})
			chat, err := server.CreateChat(ctx, chatd.CreateOptions{
				OrganizationID: org.ID,
				OwnerID:        user.ID,
				APIKeyID:       testAPIKeyID(t, db, user.ID),
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

			call := requireToolCallPart(t, chatToolParts(ctx, t, db, chat.ID), "read_file")
			require.JSONEq(t, `{"path":"`+tt.expectedPath+`"}`, string(call.Args))
			dispatch := preToolUseDispatch(t, db, chat.ID)
			require.Equal(t, "call_non_uuid", dispatch.ToolUseID.String)
			require.Equal(t, tt.decision != "", dispatch.Decision.Valid)
			if tt.decision != "" {
				require.Equal(t, tt.decision, dispatch.Decision.String)
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
		cfg.HookDispatcher = newPreToolUseDispatcher(t, db, consumer)
		cfg.AgentConn = func(_ context.Context, agentID uuid.UUID) (workspacesdk.AgentConn, func(), error) {
			require.Equal(t, dbAgent.ID, agentID)
			return mockConn, func() {}, nil
		}
	})
	chat, err := server.CreateChat(ctx, chatd.CreateOptions{
		OrganizationID: org.ID,
		OwnerID:        user.ID,
		APIKeyID:       testAPIKeyID(t, db, user.ID),
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
	dispatch := preToolUseDispatch(t, db, chat.ID)
	require.Equal(t, "deny", dispatch.Decision.String)
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

func newPreToolUseDispatcher(t *testing.T, db database.Store, consumer *httptest.Server) *chathooks.Dispatcher {
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

func preToolUseDispatch(t *testing.T, db database.Store, chatID uuid.UUID) database.ChatHookDispatch {
	t.Helper()
	rows, err := db.ListChatHookDispatchesByChatID(t.Context(), chatID)
	require.NoError(t, err)
	for _, row := range rows {
		if row.Event == string(agenthooks.EventPreToolUse) {
			return row
		}
	}
	require.FailNow(t, "pre_tool_use dispatch not found")
	return database.ChatHookDispatch{}
}
