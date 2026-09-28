//nolint:testpackage // These tests exercise package-private task seams.
package chatd

import (
	"context"
	"encoding/json"
	"testing"

	"charm.land/fantasy"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/x/chatd/chatprompt"
	"github.com/coder/coder/v2/coderd/x/chatd/chatstate"
	"github.com/coder/coder/v2/coderd/x/chatd/chattool"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/codersdk/workspacesdk"
	"github.com/coder/coder/v2/codersdk/workspacesdk/agentconnmock"
	"github.com/coder/coder/v2/testutil"
)

// executeLocalToolBatch runs the chat's unresolved tool calls once
// through executeLocalTools with tools, as one generation attempt does.
// isCapableAgent is the agent capability check of the turn.
func executeLocalToolBatch(ctx context.Context, t *testing.T, f *taskTestFixture, batch interruptedBatch, tools []fantasy.AgentTool, isCapableAgent func(context.Context) bool) error {
	t.Helper()
	chat, err := f.db.GetChatByID(ctx, batch.chat.ID)
	require.NoError(t, err)
	decision, err := decideGenerationAction(generationDecisionInput{chat: chat, messages: chatMessages(ctx, t, f, chat.ID), maxSteps: 100})
	require.NoError(t, err)
	require.Equal(t, generationActionExecuteLocalTools, decision.kind)
	activeTools := make([]string, 0, len(tools))
	for _, tool := range tools {
		activeTools = append(activeTools, tool.Info().Name)
	}
	return batch.starter.executeLocalTools(ctx, chatstate.NewChatMachine(f.db, f.pubsub, chat.ID), chatWorkerTaskStartInput{
		ChatID:            chat.ID,
		WorkerID:          batch.workerID,
		RunnerID:          batch.runnerID,
		HistoryVersion:    chat.HistoryVersion,
		GenerationAttempt: chat.GenerationAttempt,
		Status:            database.ChatStatusRunning,
	}, generationPrepared{
		Chat:           chat,
		Tools:          tools,
		ActiveTools:    activeTools,
		ModelConfigID:  f.model.ID,
		IsCapableAgent: isCapableAgent,
	}, decision)
}

// chatMessages returns the chat's committed messages.
func chatMessages(ctx context.Context, t *testing.T, f *taskTestFixture, chatID uuid.UUID) []database.ChatMessage {
	t.Helper()
	messages, err := f.db.GetChatMessagesByChatID(ctx, database.GetChatMessagesByChatIDParams{ChatID: chatID})
	require.NoError(t, err)
	return messages
}

// latestAssistantMessage returns the chat's latest message, which must be
// the assistant message interruptedBatchFixture committed.
func latestAssistantMessage(ctx context.Context, t *testing.T, f *taskTestFixture, chatID uuid.UUID) database.ChatMessage {
	t.Helper()
	messages := chatMessages(ctx, t, f, chatID)
	require.NotEmpty(t, messages)
	assistant := messages[len(messages)-1]
	require.Equal(t, database.ChatMessageRoleAssistant, assistant.Role)
	return assistant
}

func TestExecuteLocalTools_ToolCallIdentity(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		// toolName is the name of the probe tool the batch calls.
		toolName string
		capable  bool
		// wantCheck is whether the batch asks for the capability.
		wantCheck    bool
		wantIdentity bool
	}{
		{name: "CapableAgent", toolName: chattool.ExecuteToolName, capable: true, wantCheck: true, wantIdentity: true},
		{name: "AgentNotCapable", toolName: chattool.ExecuteToolName, capable: false, wantCheck: true},
		{name: "NoToolSendsIdentity", toolName: "probe", capable: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			f := newTaskTestFixture(t)
			callID := "call_" + uuid.NewString()
			batch := interruptedBatchFixture(t, f, []codersdk.ChatMessagePart{
				{Type: codersdk.ChatMessagePartTypeToolCall, ToolCallID: callID, ToolName: tt.toolName, Args: json.RawMessage(`{}`)},
			})
			ctx := testutil.Context(t, testutil.WaitLong)
			assistant := latestAssistantMessage(ctx, t, f, batch.chat.ID)

			var (
				identity chattool.ToolCallIdentity
				found    bool
				checks   int
			)
			probe := fantasy.NewAgentTool(tt.toolName, "records its tool call identity",
				func(ctx context.Context, _ struct{}, _ fantasy.ToolCall) (fantasy.ToolResponse, error) {
					identity, found = chattool.ToolCallIdentityFromContext(ctx)
					return fantasy.NewTextResponse("ok"), nil
				})
			require.NoError(t, executeLocalToolBatch(ctx, t, f, batch, []fantasy.AgentTool{probe}, func(context.Context) bool {
				checks++
				return tt.capable
			}))

			if tt.wantCheck {
				assert.Equal(t, 1, checks, "the capability is checked once per batch")
			} else {
				assert.Zero(t, checks)
			}
			require.Equal(t, tt.wantIdentity, found)
			if tt.wantIdentity {
				assert.Equal(t, chattool.ToolCallIdentity{
					ChatID:     batch.chat.ID,
					MessageID:  assistant.ID,
					ToolCallID: callID,
					ToolName:   tt.toolName,
				}, identity)
			}
		})
	}
}

// TestExecuteLocalTools_ExecuteTaskRetry runs one unresolved execute
// call in two attempts, as a task retry does. Both attempts send the
// same start request with the same tool call, and the second attempt
// commits the result of the process the first one started.
func TestExecuteLocalTools_ExecuteTaskRetry(t *testing.T) {
	t.Parallel()

	f := newTaskTestFixture(t)
	callID := "call_" + uuid.NewString()
	batch := interruptedBatchFixture(t, f, []codersdk.ChatMessagePart{{
		Type:       codersdk.ChatMessagePartTypeToolCall,
		ToolCallID: callID,
		ToolName:   chattool.ExecuteToolName,
		Args:       json.RawMessage(`{"command":"make test","timeout":"10m"}`),
	}})
	ctx := testutil.Context(t, testutil.WaitLong)
	assistant := latestAssistantMessage(ctx, t, f, batch.chat.ID)
	processID := workspacesdk.ToolCallUUID(batch.chat.ID, assistant.ID, chattool.ExecuteToolName, callID).String()

	conn := agentconnmock.NewMockAgentConn(gomock.NewController(t))
	type startRequest struct {
		toolCall workspacesdk.ToolCall
		req      workspacesdk.StartProcessRequest
	}
	var starts []startRequest
	recordStart := func(ctx context.Context, req workspacesdk.StartProcessRequest) (workspacesdk.StartProcessResponse, error) {
		tc, ok := workspacesdk.ToolCallFromContext(ctx)
		assert.True(t, ok, "start request must carry the tool call")
		starts = append(starts, startRequest{toolCall: tc, req: req})
		return workspacesdk.StartProcessResponse{ID: processID, Started: true}, nil
	}
	firstAttemptCtx, endFirstAttempt := context.WithCancel(ctx)
	defer endFirstAttempt()
	exitCode := 0
	wait := &workspacesdk.ProcessOutputOptions{Wait: true}
	gomock.InOrder(
		conn.EXPECT().StartProcess(gomock.Any(), gomock.Any()).DoAndReturn(recordStart),
		// The first attempt ends while it waits, as on the attempt
		// timeout.
		conn.EXPECT().ProcessOutput(gomock.Any(), processID, wait).
			DoAndReturn(func(ctx context.Context, _ string, _ *workspacesdk.ProcessOutputOptions) (workspacesdk.ProcessOutputResponse, error) {
				endFirstAttempt()
				<-ctx.Done()
				return workspacesdk.ProcessOutputResponse{}, ctx.Err()
			}),
		conn.EXPECT().StartProcess(gomock.Any(), gomock.Any()).DoAndReturn(recordStart),
		conn.EXPECT().ProcessOutput(gomock.Any(), processID, wait).
			Return(workspacesdk.ProcessOutputResponse{ExitCode: &exitCode, Output: "PASS", DurationMs: 7000}, nil),
	)
	tools := []fantasy.AgentTool{chattool.NewToolCallTools(chattool.ToolCallToolsOptions{
		GetWorkspaceConn: func(context.Context) (workspacesdk.AgentConn, error) { return conn, nil },
		Clock:            batch.clock,
	}).Execute}
	capable := func(context.Context) bool { return true }

	require.ErrorIs(t, executeLocalToolBatch(firstAttemptCtx, t, f, batch, tools, capable), context.Canceled)
	require.NoError(t, executeLocalToolBatch(ctx, t, f, batch, tools, capable))

	require.Len(t, starts, 2)
	assert.Equal(t, workspacesdk.ToolCall{MessageID: assistant.ID, ID: callID, Name: chattool.ExecuteToolName}, starts[0].toolCall)
	assert.Equal(t, int64(600_000), starts[0].req.TimeoutMs)
	assert.Equal(t, starts[0], starts[1], "a task retry sends the same request")

	parts, err := chatprompt.ParseContent(findToolResultMessage(t, chatMessages(ctx, t, f, batch.chat.ID), callID))
	require.NoError(t, err)
	require.Len(t, parts, 1)
	var result chattool.ExecuteResult
	require.NoError(t, json.Unmarshal(parts[0].Result, &result), string(parts[0].Result))
	assert.True(t, result.Success)
	assert.Equal(t, "PASS", result.Output)
	assert.Equal(t, int64(7000), result.WallDurationMs)
}
