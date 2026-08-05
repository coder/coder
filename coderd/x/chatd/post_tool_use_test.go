package chatd_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbtestutil"
	"github.com/coder/coder/v2/coderd/x/agenthooks/dispatch"
	"github.com/coder/coder/v2/coderd/x/chatd"
	"github.com/coder/coder/v2/coderd/x/chatd/chatprompt"
	"github.com/coder/coder/v2/coderd/x/chatd/chattest"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/codersdk/workspacesdk"
	"github.com/coder/coder/v2/codersdk/workspacesdk/agentconnmock"
	"github.com/coder/coder/v2/codersdk/x/agenthooks"
	"github.com/coder/coder/v2/testutil"
)

func TestPostToolUseHookResponsesCommitWithResults(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitLong)
	db, ps := dbtestutil.NewDB(t)
	var modelCalls atomic.Int32
	openAIURL := chattest.NewOpenAI(t, func(req *chattest.OpenAIRequest) chattest.OpenAIResponse {
		if !req.Stream {
			return chattest.OpenAINonStreamingResponse("title")
		}
		if modelCalls.Add(1) == 1 {
			first := chattest.OpenAIToolCallChunk("read_file", `{"path":"/tmp/first.txt"}`)
			first.Choices[0].ToolCalls[0].ID = "call_first"
			second := chattest.OpenAIToolCallChunk("read_file", `{"path":"/tmp/second.txt"}`).Choices[0].ToolCalls[0]
			second.ID = "call_second"
			second.Index = 1
			first.Choices[0].ToolCalls = append(first.Choices[0].ToolCalls, second)
			return chattest.OpenAIStreamingResponse(first)
		}
		toolResultIndex := -1
		contextIndex := -1
		for i, message := range req.Messages {
			if message.Role == "tool" && strings.Contains(message.Content, "data") {
				toolResultIndex = i
			}
			if strings.Contains(message.Content, "lint feedback") {
				contextIndex = i
			}
		}
		require.NotEqual(t, -1, toolResultIndex)
		require.NotEqual(t, -1, contextIndex)
		require.Less(t, toolResultIndex, contextIndex)
		return chattest.OpenAIStreamingResponse(chattest.OpenAITextChunks("done")...)
	})
	user, org, model := seedChatDependenciesWithProvider(t, db, "openai-compat", openAIURL)
	ws, dbAgent := seedWorkspaceWithAgent(t, db, user.ID)

	var mu sync.Mutex
	var received []agenthooks.PostToolUseData
	consumer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request agenthooks.Request
		require.NoError(t, json.NewDecoder(r.Body).Decode(&request))
		if request.Type != agenthooks.EventPostToolUse {
			_, err := w.Write([]byte(`{}`))
			require.NoError(t, err)
			return
		}
		data := decodeHookData[agenthooks.PostToolUseData](t, request)
		mu.Lock()
		received = append(received, data)
		index := len(received)
		mu.Unlock()

		messages := chatMessages(ctx, t, db, request.Meta.ChatID)
		for _, message := range messages {
			require.NotEqual(t, database.ChatMessageRoleTool, message.Role)
		}
		var err error
		if index == 1 {
			_, err = w.Write([]byte(`{"model_context":"lint feedback","user_message":"tool notice"}`))
		} else {
			_, err = w.Write([]byte(`{}`))
		}
		require.NoError(t, err)
	}))
	t.Cleanup(consumer.Close)

	ctrl := gomock.NewController(t)
	mockConn := agentconnmock.NewMockAgentConn(ctrl)
	setupToolExecutionAgentConn(t, mockConn)
	mockConn.EXPECT().ReadFileLines(gomock.Any(), gomock.Any(), int64(1), int64(0), gomock.Any()).
		Return(workspacesdk.ReadFileLinesResponse{
			Success: true, FileSize: 4, TotalLines: 1, LinesRead: 1, Content: "data",
		}, nil).
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
		Title:          "post-tool-use-responses",
		ModelConfigID:  model.ID,
		InitialUserContent: []codersdk.ChatMessagePart{
			codersdk.ChatMessageText("read both files"),
		},
	})
	require.NoError(t, err)
	waitForChatStatus(ctx, t, db, chat.ID, database.ChatStatusWaiting)

	mu.Lock()
	receivedSnapshot := append([]agenthooks.PostToolUseData(nil), received...)
	mu.Unlock()
	require.Len(t, receivedSnapshot, 2)
	require.Equal(t, "call_first", receivedSnapshot[0].ToolUseID)
	require.Equal(t, "call_second", receivedSnapshot[1].ToolUseID)
	require.Equal(t, "read_file", receivedSnapshot[0].ToolName)
	require.Empty(t, receivedSnapshot[0].ToolError)
	require.Contains(t, string(receivedSnapshot[0].ToolResponse), "data")

	var toolResults, userMessages int
	for _, message := range chatMessages(ctx, t, db, chat.ID) {
		parts, err := chatprompt.ParseContent(message)
		require.NoError(t, err)
		if message.Role == database.ChatMessageRoleTool {
			toolResults++
		}
		if len(parts) == 1 && parts[0].Text == "tool notice" {
			userMessages++
			require.Equal(t, database.ChatMessageVisibilityUser, message.Visibility)
		}
	}
	promptMessages, err := db.GetChatMessagesForPromptByChatID(ctx, chat.ID)
	require.NoError(t, err)
	var modelContexts int
	for _, message := range promptMessages {
		parts, err := chatprompt.ParseContent(message)
		require.NoError(t, err)
		if len(parts) == 1 && parts[0].Text == "lint feedback" {
			modelContexts++
			require.Equal(t, database.ChatMessageVisibilityModel, message.Visibility)
		}
	}
	require.Equal(t, 2, toolResults)
	require.Equal(t, 1, modelContexts)
	require.Equal(t, 1, userMessages)
}

func TestPostToolUseHookDynamicResult(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitLong)
	db, ps := dbtestutil.NewDB(t)
	var modelCalls atomic.Int32
	openAIURL := chattest.NewOpenAI(t, func(req *chattest.OpenAIRequest) chattest.OpenAIResponse {
		if !req.Stream {
			return chattest.OpenAINonStreamingResponse("title")
		}
		if modelCalls.Add(1) == 1 {
			chunk := chattest.OpenAIToolCallChunk("my_dynamic_tool", `{"query":"value"}`)
			chunk.Choices[0].ToolCalls[0].ID = "call_dynamic_result"
			return chattest.OpenAIStreamingResponse(chunk)
		}
		resultIndex := -1
		contextIndex := -1
		for i, message := range req.Messages {
			if message.Role == "tool" && strings.Contains(message.Content, "answer") {
				resultIndex = i
			}
			if strings.Contains(message.Content, "dynamic feedback") {
				contextIndex = i
			}
		}
		require.NotEqual(t, -1, resultIndex)
		require.NotEqual(t, -1, contextIndex)
		require.Less(t, resultIndex, contextIndex)
		return chattest.OpenAIStreamingResponse(chattest.OpenAITextChunks("done")...)
	})
	user, org, model := seedChatDependenciesWithProvider(t, db, "openai-compat", openAIURL)
	var postCalls atomic.Int32
	consumer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request agenthooks.Request
		require.NoError(t, json.NewDecoder(r.Body).Decode(&request))
		if request.Type != agenthooks.EventPostToolUse {
			_, err := w.Write([]byte(`{}`))
			require.NoError(t, err)
			return
		}
		postCalls.Add(1)
		data := decodeHookData[agenthooks.PostToolUseData](t, request)
		require.Equal(t, "call_dynamic_result", data.ToolUseID)
		require.Equal(t, "my_dynamic_tool", data.ToolName)
		require.JSONEq(t, `{"answer":42}`, string(data.ToolResponse))
		_, err := w.Write([]byte(`{"model_context":"dynamic feedback","user_message":"dynamic notice"}`))
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
		Title:          "post-tool-use-dynamic",
		ModelConfigID:  model.ID,
		InitialUserContent: []codersdk.ChatMessagePart{
			codersdk.ChatMessageText("call the dynamic tool"),
		},
		DynamicTools: dynamicToolJSON(t, "my_dynamic_tool"),
	})
	require.NoError(t, err)
	testutil.Eventually(ctx, t, func(context.Context) bool {
		updated, err := db.GetChatByID(ctx, chat.ID)
		return err == nil && updated.Status == database.ChatStatusRequiresAction
	}, testutil.IntervalFast)

	err = server.SubmitToolResults(ctx, chatd.SubmitToolResultsOptions{
		ChatID:        chat.ID,
		UserID:        user.ID,
		ModelConfigID: model.ID,
		Results: []codersdk.ToolResult{{
			ToolCallID: "call_dynamic_result",
			Output:     json.RawMessage(`{"answer":42}`),
		}},
	})
	require.NoError(t, err)
	waitForChatStatus(ctx, t, db, chat.ID, database.ChatStatusWaiting)
	require.Equal(t, int32(1), postCalls.Load())

	err = server.SubmitToolResults(ctx, chatd.SubmitToolResultsOptions{
		ChatID:        chat.ID,
		UserID:        user.ID,
		ModelConfigID: model.ID,
		Results: []codersdk.ToolResult{{
			ToolCallID: "call_dynamic_result",
			Output:     json.RawMessage(`{"answer":42}`),
		}},
	})
	require.Error(t, err)
	require.Equal(t, int32(1), postCalls.Load())
	var notices int
	for _, message := range chatMessages(ctx, t, db, chat.ID) {
		if hookMessageText(t, message) == "dynamic notice" {
			notices++
		}
	}
	require.Equal(t, 1, notices)
}

func TestPostToolUseHookDynamicFailureRejectsSubmission(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitLong)
	db, ps := dbtestutil.NewDB(t)
	openAIURL := chattest.NewOpenAI(t, func(req *chattest.OpenAIRequest) chattest.OpenAIResponse {
		if !req.Stream {
			return chattest.OpenAINonStreamingResponse("title")
		}
		chunk := chattest.OpenAIToolCallChunk("my_dynamic_tool", `{}`)
		chunk.Choices[0].ToolCalls[0].ID = "call_dynamic_failure"
		return chattest.OpenAIStreamingResponse(chunk)
	})
	user, org, model := seedChatDependenciesWithProvider(t, db, "openai-compat", openAIURL)
	var postCalls atomic.Int32
	var failPostToolUse atomic.Bool
	failPostToolUse.Store(true)
	consumer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request agenthooks.Request
		require.NoError(t, json.NewDecoder(r.Body).Decode(&request))
		if request.Type == agenthooks.EventPostToolUse {
			postCalls.Add(1)
			if failPostToolUse.Load() {
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
		Title:          "post-tool-use-dynamic-failure",
		ModelConfigID:  model.ID,
		InitialUserContent: []codersdk.ChatMessagePart{
			codersdk.ChatMessageText("call the dynamic tool"),
		},
		DynamicTools: dynamicToolJSON(t, "my_dynamic_tool"),
	})
	require.NoError(t, err)
	testutil.Eventually(ctx, t, func(context.Context) bool {
		updated, err := db.GetChatByID(ctx, chat.ID)
		return err == nil && updated.Status == database.ChatStatusRequiresAction
	}, testutil.IntervalFast)

	results := []codersdk.ToolResult{{
		ToolCallID: "call_dynamic_failure",
		Output:     json.RawMessage(`{"answer":42}`),
	}}
	err = server.SubmitToolResults(ctx, chatd.SubmitToolResultsOptions{
		ChatID:        chat.ID,
		UserID:        user.ID,
		ModelConfigID: model.ID,
		Results:       results,
	})
	var dispatchErr *dispatch.Error
	require.ErrorAs(t, err, &dispatchErr)

	unchanged, err := db.GetChatByID(ctx, chat.ID)
	require.NoError(t, err)
	require.Equal(t, database.ChatStatusRequiresAction, unchanged.Status)
	require.False(t, unchanged.LastError.Valid)
	for _, part := range chatToolParts(ctx, t, db, chat.ID) {
		require.NotEqual(t, codersdk.ChatMessagePartTypeToolResult, part.Type,
			"rejected submission must not commit tool results")
	}
	require.Equal(t, int32(1), postCalls.Load())

	failPostToolUse.Store(false)
	require.NoError(t, server.SubmitToolResults(ctx, chatd.SubmitToolResultsOptions{
		ChatID:        chat.ID,
		UserID:        user.ID,
		ModelConfigID: model.ID,
		Results:       results,
	}))
	result := requireToolResultPart(t, chatToolParts(ctx, t, db, chat.ID), "my_dynamic_tool")
	require.JSONEq(t, `{"answer":42}`, string(result.Result))
	require.Equal(t, int32(2), postCalls.Load())
}

func TestPostToolUseHookFailureCommitsResultThenErrors(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitLong)
	db, ps := dbtestutil.NewDB(t)
	openAIURL := chattest.NewOpenAI(t, func(req *chattest.OpenAIRequest) chattest.OpenAIResponse {
		if !req.Stream {
			return chattest.OpenAINonStreamingResponse("title")
		}
		chunk := chattest.OpenAIToolCallChunk("read_file", `{"path":"/tmp/file.txt"}`)
		chunk.Choices[0].ToolCalls[0].ID = "call_failure"
		return chattest.OpenAIStreamingResponse(chunk)
	})
	user, org, model := seedChatDependenciesWithProvider(t, db, "openai-compat", openAIURL)
	ws, dbAgent := seedWorkspaceWithAgent(t, db, user.ID)
	var postCalls atomic.Int32
	consumer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request agenthooks.Request
		require.NoError(t, json.NewDecoder(r.Body).Decode(&request))
		if request.Type == agenthooks.EventPostToolUse {
			postCalls.Add(1)
			data := decodeHookData[agenthooks.PostToolUseData](t, request)
			require.Equal(t, "call_failure", data.ToolUseID)
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		_, err := w.Write([]byte(`{}`))
		require.NoError(t, err)
	}))
	t.Cleanup(consumer.Close)

	ctrl := gomock.NewController(t)
	mockConn := agentconnmock.NewMockAgentConn(ctrl)
	setupToolExecutionAgentConn(t, mockConn)
	mockConn.EXPECT().ReadFileLines(gomock.Any(), "/tmp/file.txt", int64(1), int64(0), gomock.Any()).
		Return(workspacesdk.ReadFileLinesResponse{
			Success: true, FileSize: 4, TotalLines: 1, LinesRead: 1, Content: "data",
		}, nil)

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
		Title:          "post-tool-use-failure",
		ModelConfigID:  model.ID,
		InitialUserContent: []codersdk.ChatMessagePart{
			codersdk.ChatMessageText("read the file"),
		},
	})
	require.NoError(t, err)
	failed := waitForChatStatus(ctx, t, db, chat.ID, database.ChatStatusError)

	result := requireToolResultPart(t, chatToolParts(ctx, t, db, chat.ID), "read_file")
	require.Contains(t, string(result.Result), "data")
	require.Equal(t, int32(1), postCalls.Load())
	lastError := chatLastErrorMessage(failed.LastError)
	require.Contains(t, lastError, "hook dispatch failed: post_tool_use: http_error")
}

func TestSubagentSpawnHookDispatchFailureCommitsSiblingResults(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitLong)
	db, ps := dbtestutil.NewDB(t)
	openAIURL := chattest.NewOpenAI(t, func(req *chattest.OpenAIRequest) chattest.OpenAIResponse {
		if !req.Stream {
			return chattest.OpenAINonStreamingResponse("title")
		}
		chunk := chattest.OpenAIToolCallChunk("read_file", `{"path":"/tmp/sibling.txt"}`)
		chunk.Choices[0].ToolCalls[0].ID = "call_sibling"
		spawn := chattest.OpenAIToolCallChunk("spawn_agent", `{"type":"general","prompt":"child admission prompt"}`).Choices[0].ToolCalls[0]
		spawn.ID = "call_spawn"
		spawn.Index = 1
		chunk.Choices[0].ToolCalls = append(chunk.Choices[0].ToolCalls, spawn)
		return chattest.OpenAIStreamingResponse(chunk)
	})
	user, org, model := seedChatDependenciesWithProvider(t, db, "openai-compat", openAIURL)
	ws, dbAgent := seedWorkspaceWithAgent(t, db, user.ID)

	var mu sync.Mutex
	var postToolUseIDs []string
	consumer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request agenthooks.Request
		require.NoError(t, json.NewDecoder(r.Body).Decode(&request))
		switch request.Type {
		case agenthooks.EventUserPromptSubmit:
			data := decodeHookData[agenthooks.UserPromptSubmitData](t, request)
			if data.Prompt == "child admission prompt" {
				w.WriteHeader(http.StatusInternalServerError)
				return
			}
		case agenthooks.EventPostToolUse:
			data := decodeHookData[agenthooks.PostToolUseData](t, request)
			mu.Lock()
			postToolUseIDs = append(postToolUseIDs, data.ToolUseID)
			mu.Unlock()
		}
		_, err := w.Write([]byte(`{}`))
		require.NoError(t, err)
	}))
	t.Cleanup(consumer.Close)

	ctrl := gomock.NewController(t)
	mockConn := agentconnmock.NewMockAgentConn(ctrl)
	setupToolExecutionAgentConn(t, mockConn)
	mockConn.EXPECT().ReadFileLines(gomock.Any(), gomock.Any(), int64(1), int64(0), gomock.Any()).
		Return(workspacesdk.ReadFileLinesResponse{
			Success: true, FileSize: 7, TotalLines: 1, LinesRead: 1, Content: "sibling",
		}, nil)

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
		Title:          "spawn-hook-failure-siblings",
		ModelConfigID:  model.ID,
		InitialUserContent: []codersdk.ChatMessagePart{
			codersdk.ChatMessageText("read a file and spawn a child"),
		},
	})
	require.NoError(t, err)
	failed := waitForChatStatus(ctx, t, db, chat.ID, database.ChatStatusError)
	require.Contains(t, chatLastErrorMessage(failed.LastError), "hook dispatch failed: user_prompt_submit: http_error")

	messages := chatMessages(ctx, t, db, chat.ID)
	require.Len(t, messages, 4)
	require.Equal(t, database.ChatMessageRoleTool, messages[2].Role)
	require.Equal(t, database.ChatMessageRoleTool, messages[3].Role)
	sibling := string(messages[2].Content.RawMessage)
	require.Contains(t, sibling, "call_sibling")
	require.Contains(t, sibling, "sibling")
	spawn := string(messages[3].Content.RawMessage)
	require.Contains(t, spawn, "call_spawn")
	require.Contains(t, spawn, "lifecycle hook returned HTTP status 500")

	mu.Lock()
	dispatched := slices.Clone(postToolUseIDs)
	mu.Unlock()
	require.Equal(t, []string{"call_sibling"}, dispatched,
		"the spawn tool never ran, so it has no use to post-process")
}

func TestPostToolUseHookFailureDispatchesRemainingResults(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitLong)
	db, ps := dbtestutil.NewDB(t)
	openAIURL := chattest.NewOpenAI(t, func(req *chattest.OpenAIRequest) chattest.OpenAIResponse {
		if !req.Stream {
			return chattest.OpenAINonStreamingResponse("title")
		}
		first := chattest.OpenAIToolCallChunk("read_file", `{"path":"/tmp/first.txt"}`)
		first.Choices[0].ToolCalls[0].ID = "call_first"
		second := chattest.OpenAIToolCallChunk("read_file", `{"path":"/tmp/second.txt"}`).Choices[0].ToolCalls[0]
		second.ID = "call_second"
		second.Index = 1
		first.Choices[0].ToolCalls = append(first.Choices[0].ToolCalls, second)
		return chattest.OpenAIStreamingResponse(first)
	})
	user, org, model := seedChatDependenciesWithProvider(t, db, "openai-compat", openAIURL)
	ws, dbAgent := seedWorkspaceWithAgent(t, db, user.ID)

	var mu sync.Mutex
	results := map[string]string{}
	consumer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request agenthooks.Request
		require.NoError(t, json.NewDecoder(r.Body).Decode(&request))
		if request.Type != agenthooks.EventPostToolUse {
			_, err := w.Write([]byte(`{}`))
			require.NoError(t, err)
			return
		}
		data := decodeHookData[agenthooks.PostToolUseData](t, request)
		result := "ok"
		if data.ToolUseID == "call_first" {
			result = "http_error"
		}
		mu.Lock()
		results[data.ToolUseID] = result
		mu.Unlock()
		if result == "http_error" {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		_, err := w.Write([]byte(`{}`))
		require.NoError(t, err)
	}))
	t.Cleanup(consumer.Close)

	ctrl := gomock.NewController(t)
	mockConn := agentconnmock.NewMockAgentConn(ctrl)
	setupToolExecutionAgentConn(t, mockConn)
	mockConn.EXPECT().ReadFileLines(gomock.Any(), gomock.Any(), int64(1), int64(0), gomock.Any()).
		Return(workspacesdk.ReadFileLinesResponse{
			Success: true, FileSize: 4, TotalLines: 1, LinesRead: 1, Content: "data",
		}, nil).
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
		Title:          "post-tool-use-continue",
		ModelConfigID:  model.ID,
		InitialUserContent: []codersdk.ChatMessagePart{
			codersdk.ChatMessageText("read both files"),
		},
	})
	require.NoError(t, err)
	failed := waitForChatStatus(ctx, t, db, chat.ID, database.ChatStatusError)

	mu.Lock()
	received := make(map[string]string, len(results))
	for toolUseID, result := range results {
		received[toolUseID] = result
	}
	mu.Unlock()
	require.Equal(t, map[string]string{
		"call_first":  "http_error",
		"call_second": "ok",
	}, received)
	require.Contains(t, chatLastErrorMessage(failed.LastError), "hook dispatch failed: post_tool_use: http_error")
}
