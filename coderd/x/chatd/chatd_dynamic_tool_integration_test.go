package chatd_test

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/sqlc-dev/pqtype"
	"github.com/stretchr/testify/require"

	"cdr.dev/slog/v3/sloggers/slogtest"
	agentproto "github.com/coder/coder/v2/agent/proto"
	"github.com/coder/coder/v2/coderd/agentapi"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/database/dbtestutil"
	coderdpubsub "github.com/coder/coder/v2/coderd/pubsub"
	"github.com/coder/coder/v2/coderd/x/chatd"
	"github.com/coder/coder/v2/coderd/x/chatd/chatprompt"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/testutil"
)

func TestAgentDynamicToolLoopIntegration(t *testing.T) {
	t.Parallel()

	db, ps := dbtestutil.NewDB(t)
	ctx := testutil.Context(t, testutil.WaitLong)
	logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true})

	server := chatd.New(chatd.Config{
		Logger:                     logger,
		Database:                   db,
		ReplicaID:                  uuid.New(),
		Pubsub:                     ps,
		PendingChatAcquireInterval: testutil.WaitLong,
		Experiments:                codersdk.Experiments{codersdk.ExperimentAgentChatRunner},
	})
	t.Cleanup(func() {
		require.NoError(t, server.Close())
	})

	user, org, model := seedChatDependencies(t, db)
	_, _, agent := seedWorkspaceWithChatAgent(t, db, user.ID, org.ID)

	err := db.UpdateWorkspaceAgentChatRunnerStatus(dbauthz.AsSystemRestricted(ctx), database.UpdateWorkspaceAgentChatRunnerStatusParams{
		AgentID:         agent.ID,
		ChatRunnerReady: true,
	})
	require.NoError(t, err)

	api := &agentapi.ChatRunnerAPI{
		AgentID:     agent.ID,
		Database:    db,
		Log:         logger,
		Experiments: codersdk.Experiments{codersdk.ExperimentAgentChatRunner},
		OnRequiresAction: func(ctx context.Context, chat database.Chat) error {
			return server.PublishRequiresAction(ctx, chat)
		},
	}

	dynamicToolsJSON, err := json.Marshal([]codersdk.DynamicTool{{
		Name:        "my_tool",
		Description: "A test dynamic tool.",
		InputSchema: json.RawMessage(`{"type":"object","properties":{"input":{"type":"string"}}}`),
	}})
	require.NoError(t, err)

	chat, err := db.InsertChat(ctx, database.InsertChatParams{
		OrganizationID:    org.ID,
		Status:            database.ChatStatusPending,
		ClientType:        database.ChatClientTypeApi,
		OwnerID:           user.ID,
		Title:             "agent-dynamic-tool-loop",
		LastModelConfigID: model.ID,
		AgentID:           uuid.NullUUID{UUID: agent.ID, Valid: true},
		DynamicTools: pqtype.NullRawMessage{
			RawMessage: dynamicToolsJSON,
			Valid:      true,
		},
	})
	require.NoError(t, err)

	_, streamEvents, cancelStream, ok := server.Subscribe(ctx, chat.ID, nil, 0)
	require.True(t, ok)
	t.Cleanup(cancelStream)

	watchEvents := make(chan chatWatchEventResult, 1)
	cancelWatch, err := ps.SubscribeWithErr(
		coderdpubsub.ChatWatchEventChannel(user.ID),
		coderdpubsub.HandleChatWatchEvent(func(_ context.Context, payload codersdk.ChatWatchEvent, err error) {
			watchEvents <- chatWatchEventResult{payload: payload, err: err}
		}),
	)
	require.NoError(t, err)
	t.Cleanup(cancelWatch)

	pendingBeforeAcquire, err := db.GetPendingChatsForAgent(ctx, database.GetPendingChatsForAgentParams{
		AgentID:  agent.ID,
		MaxChats: 10,
	})
	require.NoError(t, err)
	require.Len(t, pendingBeforeAcquire, 1)
	require.Equal(t, chat.ID, pendingBeforeAcquire[0].ID)

	acquireResp, err := api.AcquireChatLease(ctx, &agentproto.AcquireChatLeaseRequest{ChatId: chat.ID[:]})
	require.NoError(t, err)
	require.Positive(t, acquireResp.LeaseEpoch)
	require.Equal(t, string(database.ChatStatusRunning), acquireResp.Status)

	runningChat, err := db.GetChatByID(ctx, chat.ID)
	require.NoError(t, err)
	require.Equal(t, database.ChatStatusRunning, runningChat.Status)
	require.True(t, runningChat.WorkerID.Valid)
	require.Equal(t, agent.ID, runningChat.WorkerID.UUID)
	require.True(t, runningChat.RunnerType.Valid)
	require.Equal(t, database.ChatRunnerTypeWorkspaceAgent, runningChat.RunnerType.ChatRunnerType)
	require.Equal(t, acquireResp.LeaseEpoch, runningChat.LeaseEpoch)

	toolCallArgs := json.RawMessage(`{"input":"hello"}`)
	assistantContent, err := chatprompt.MarshalParts([]codersdk.ChatMessagePart{
		codersdk.ChatMessageText("Calling my_tool."),
		{
			Type:       codersdk.ChatMessagePartTypeToolCall,
			ToolCallID: "call_123",
			ToolName:   "my_tool",
			Args:       toolCallArgs,
		},
	})
	require.NoError(t, err)

	insertSingleChatMessage(
		ctx,
		t,
		db,
		chat.ID,
		uuid.Nil,
		model.ID,
		database.ChatMessageRoleAssistant,
		assistantContent,
	)

	assistantMessage, err := db.GetLastChatMessageByRole(ctx, database.GetLastChatMessageByRoleParams{
		ChatID: chat.ID,
		Role:   database.ChatMessageRoleAssistant,
	})
	require.NoError(t, err)

	assistantParts, err := chatprompt.ParseContent(assistantMessage)
	require.NoError(t, err)
	foundPersistedToolCall := false
	for _, part := range assistantParts {
		if part.Type != codersdk.ChatMessagePartTypeToolCall {
			continue
		}
		require.Equal(t, "call_123", part.ToolCallID)
		require.Equal(t, "my_tool", part.ToolName)
		require.JSONEq(t, string(toolCallArgs), string(part.Args))
		foundPersistedToolCall = true
	}
	require.True(t, foundPersistedToolCall, "expected to find the persisted assistant tool call")

	_, err = api.ReleaseChatLease(ctx, &agentproto.ReleaseChatLeaseRequest{
		ChatId:      chat.ID[:],
		LeaseEpoch:  acquireResp.LeaseEpoch,
		FinalStatus: string(database.ChatStatusRequiresAction),
	})
	require.NoError(t, err)

	requiresActionChat, err := db.GetChatByID(ctx, chat.ID)
	require.NoError(t, err)
	require.Equal(t, database.ChatStatusRequiresAction, requiresActionChat.Status)
	require.False(t, requiresActionChat.WorkerID.Valid)
	require.False(t, requiresActionChat.RunnerType.Valid)

	streamEvent := requireActionRequiredStreamEvent(t, streamEvents)
	require.Equal(t, chat.ID, streamEvent.ChatID)
	require.Len(t, streamEvent.ActionRequired.ToolCalls, 1)
	require.Equal(t, "call_123", streamEvent.ActionRequired.ToolCalls[0].ToolCallID)
	require.Equal(t, "my_tool", streamEvent.ActionRequired.ToolCalls[0].ToolName)
	require.JSONEq(t, string(toolCallArgs), streamEvent.ActionRequired.ToolCalls[0].Args)

	watchEvent := requireActionRequiredWatchEvent(t, watchEvents)
	require.Equal(t, chat.ID, watchEvent.Chat.ID)
	require.Len(t, watchEvent.ToolCalls, 1)
	require.Equal(t, "call_123", watchEvent.ToolCalls[0].ToolCallID)
	require.Equal(t, "my_tool", watchEvent.ToolCalls[0].ToolName)
	require.JSONEq(t, string(toolCallArgs), watchEvent.ToolCalls[0].Args)

	toolResultOutput := json.RawMessage(`{"ok":true}`)
	err = server.SubmitToolResults(ctx, chatd.SubmitToolResultsOptions{
		ChatID:        chat.ID,
		UserID:        user.ID,
		ModelConfigID: model.ID,
		Results: []codersdk.ToolResult{{
			ToolCallID: "call_123",
			Output:     toolResultOutput,
		}},
		DynamicTools: dynamicToolsJSON,
	})
	require.NoError(t, err)

	pendingChat, err := db.GetChatByID(ctx, chat.ID)
	require.NoError(t, err)
	require.Equal(t, database.ChatStatusPending, pendingChat.Status)

	messages, err := db.GetChatMessagesByChatID(ctx, database.GetChatMessagesByChatIDParams{
		ChatID:  chat.ID,
		AfterID: 0,
	})
	require.NoError(t, err)

	foundToolResult := false
	for _, message := range messages {
		if message.Role != database.ChatMessageRoleTool {
			continue
		}
		parts, parseErr := chatprompt.ParseContent(message)
		require.NoError(t, parseErr)
		for _, part := range parts {
			if part.Type != codersdk.ChatMessagePartTypeToolResult {
				continue
			}
			require.Equal(t, "call_123", part.ToolCallID)
			require.Equal(t, "my_tool", part.ToolName)
			require.JSONEq(t, string(toolResultOutput), string(part.Result))
			foundToolResult = true
		}
	}
	require.True(t, foundToolResult, "expected to find the submitted tool result message")

	pendingAfterSubmit, err := db.GetPendingChatsForAgent(ctx, database.GetPendingChatsForAgentParams{
		AgentID:  agent.ID,
		MaxChats: 10,
	})
	require.NoError(t, err)
	require.Len(t, pendingAfterSubmit, 1)
	require.Equal(t, chat.ID, pendingAfterSubmit[0].ID)
}

type chatWatchEventResult struct {
	payload codersdk.ChatWatchEvent
	err     error
}

func insertSingleChatMessage(
	ctx context.Context,
	t *testing.T,
	db database.Store,
	chatID uuid.UUID,
	createdBy uuid.UUID,
	modelConfigID uuid.UUID,
	role database.ChatMessageRole,
	content pqtype.NullRawMessage,
) database.ChatMessage {
	t.Helper()

	messages, err := db.InsertChatMessages(ctx, database.InsertChatMessagesParams{
		ChatID:              chatID,
		CreatedBy:           []uuid.UUID{createdBy},
		ModelConfigID:       []uuid.UUID{modelConfigID},
		Role:                []database.ChatMessageRole{role},
		ContentVersion:      []int16{chatprompt.CurrentContentVersion},
		Content:             []string{string(content.RawMessage)},
		Visibility:          []database.ChatMessageVisibility{database.ChatMessageVisibilityBoth},
		InputTokens:         []int64{0},
		OutputTokens:        []int64{0},
		TotalTokens:         []int64{0},
		ReasoningTokens:     []int64{0},
		CacheCreationTokens: []int64{0},
		CacheReadTokens:     []int64{0},
		ContextLimit:        []int64{0},
		Compressed:          []bool{false},
		TotalCostMicros:     []int64{0},
		RuntimeMs:           []int64{0},
	})
	require.NoError(t, err)
	require.Len(t, messages, 1)
	return messages[0]
}

func requireActionRequiredStreamEvent(t *testing.T, events <-chan codersdk.ChatStreamEvent) codersdk.ChatStreamEvent {
	t.Helper()

	select {
	case event, ok := <-events:
		require.True(t, ok, "chat stream closed before delivering an event")
		require.Equal(t, codersdk.ChatStreamEventTypeActionRequired, event.Type)
		require.NotNil(t, event.ActionRequired)
		return event
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for chat stream action_required event")
		return codersdk.ChatStreamEvent{}
	}
}

func requireActionRequiredWatchEvent(t *testing.T, events <-chan chatWatchEventResult) codersdk.ChatWatchEvent {
	t.Helper()

	select {
	case event := <-events:
		require.NoError(t, event.err)
		require.Equal(t, codersdk.ChatWatchEventKindActionRequired, event.payload.Kind)
		return event.payload
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for chat watch action_required event")
		return codersdk.ChatWatchEvent{}
	}
}
