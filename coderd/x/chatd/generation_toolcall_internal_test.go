//nolint:testpackage // These tests exercise package-private task seams.
package chatd

import (
	"context"
	"encoding/json"
	"testing"
	"time"

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

// runLocalToolBatch runs the chat's unresolved tool calls once through
// executeLocalTools with tools, as one generation attempt does.
func runLocalToolBatch(t *testing.T, f *taskTestFixture, batch interruptedBatch, tools []fantasy.AgentTool) {
	t.Helper()
	require.NoError(t, executeLocalToolBatch(testutil.Context(t, testutil.WaitLong), t, f, batch, tools))
}

// executeLocalToolBatch is runLocalToolBatch with the attempt's context
// and error.
func executeLocalToolBatch(ctx context.Context, t *testing.T, f *taskTestFixture, batch interruptedBatch, tools []fantasy.AgentTool) error {
	t.Helper()
	chat, err := f.db.GetChatByID(ctx, batch.chat.ID)
	require.NoError(t, err)
	messages, err := f.db.GetChatMessagesByChatID(ctx, database.GetChatMessagesByChatIDParams{ChatID: chat.ID})
	require.NoError(t, err)
	decision, err := decideGenerationAction(generationDecisionInput{chat: chat, messages: messages})
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
		Chat:          chat,
		Tools:         tools,
		ActiveTools:   activeTools,
		ModelConfigID: f.model.ID,
	}, decision)
}

func TestExecuteLocalTools_ToolCallIdentity(t *testing.T) {
	t.Parallel()

	f := newTaskTestFixture(t)
	callID := "call_" + uuid.NewString()
	batch := interruptedBatchFixture(t, f, []codersdk.ChatMessagePart{
		{Type: codersdk.ChatMessagePartTypeToolCall, ToolCallID: callID, ToolName: "probe", Args: json.RawMessage(`{}`)},
	})
	ctx := testutil.Context(t, testutil.WaitLong)
	// Backdate the assistant message so its age comes from the database
	// clock and cannot be confused with the starter's mock clock.
	_, err := f.sqlDB.ExecContext(ctx,
		`UPDATE chat_messages SET created_at = created_at - interval '1 hour' WHERE chat_id = $1 AND role = 'assistant'`,
		batch.chat.ID)
	require.NoError(t, err)
	messages, err := f.db.GetChatMessagesByChatID(ctx, database.GetChatMessagesByChatIDParams{ChatID: batch.chat.ID})
	require.NoError(t, err)
	assistant := messages[len(messages)-1]
	require.Equal(t, database.ChatMessageRoleAssistant, assistant.Role)

	var (
		identity chattool.ToolCallIdentity
		found    bool
	)
	probe := fantasy.NewAgentTool("probe", "records its tool call identity",
		func(ctx context.Context, _ struct{}, _ fantasy.ToolCall) (fantasy.ToolResponse, error) {
			identity, found = chattool.ToolCallIdentityFromContext(ctx)
			return fantasy.NewTextResponse("ok"), nil
		})

	dbBefore, err := f.db.GetDatabaseNow(ctx)
	require.NoError(t, err)
	runLocalToolBatch(t, f, batch, []fantasy.AgentTool{probe})
	dbAfter, err := f.db.GetDatabaseNow(ctx)
	require.NoError(t, err)

	require.True(t, found)
	assert.Equal(t, batch.chat.ID, identity.ChatID)
	assert.Equal(t, assistant.ID, identity.MessageID)
	assert.Equal(t, callID, identity.ToolCallID)
	// The mock clock has not moved, so the age is the database clock
	// read during the batch minus the committed CreatedAt.
	age := identity.Age.Now()
	assert.GreaterOrEqual(t, age, dbBefore.Sub(assistant.CreatedAt))
	assert.LessOrEqual(t, age, dbAfter.Sub(assistant.CreatedAt))
	assert.GreaterOrEqual(t, age, time.Hour)
	batch.clock.Advance(5 * time.Second)
	assert.Equal(t, age+5*time.Second, identity.Age.Now())
}

// TestExecuteLocalTools_ExecuteTaskRetry runs one unresolved execute
// call in two attempts, as a task retry does. Both start requests carry
// the same tool call, the second with a larger age, and the second
// attempt commits the result of the process the first one started.
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
	messages, err := f.db.GetChatMessagesByChatID(ctx, database.GetChatMessagesByChatIDParams{ChatID: batch.chat.ID})
	require.NoError(t, err)
	assistant := messages[len(messages)-1]
	require.Equal(t, database.ChatMessageRoleAssistant, assistant.Role)
	processID := workspacesdk.ToolCallUUID(batch.chat.ID, assistant.ID, callID).String()

	ctrl := gomock.NewController(t)
	conn := agentconnmock.NewMockAgentConn(ctrl)
	var toolCalls []workspacesdk.ToolCall
	recordStart := func(resp workspacesdk.StartProcessResponse) func(context.Context, workspacesdk.StartProcessRequest) (workspacesdk.StartProcessResponse, error) {
		return func(ctx context.Context, _ workspacesdk.StartProcessRequest) (workspacesdk.StartProcessResponse, error) {
			tc, ok := workspacesdk.ToolCallFromContext(ctx)
			assert.True(t, ok, "start request must carry the tool call")
			toolCalls = append(toolCalls, tc)
			return resp, nil
		}
	}
	firstAttemptCtx, endFirstAttempt := context.WithCancel(ctx)
	defer endFirstAttempt()
	exitCode := 0
	gomock.InOrder(
		conn.EXPECT().StartProcess(gomock.Any(), gomock.Any()).
			DoAndReturn(recordStart(workspacesdk.StartProcessResponse{ID: processID, Started: true})),
		// The first attempt ends while it waits, as on the attempt
		// timeout, and its snapshot fails on the ended context.
		conn.EXPECT().ProcessOutput(gomock.Any(), processID, &workspacesdk.ProcessOutputOptions{Wait: true}).
			DoAndReturn(func(ctx context.Context, _ string, _ *workspacesdk.ProcessOutputOptions) (workspacesdk.ProcessOutputResponse, error) {
				endFirstAttempt()
				<-ctx.Done()
				return workspacesdk.ProcessOutputResponse{}, ctx.Err()
			}),
		conn.EXPECT().ProcessOutput(gomock.Any(), processID, nil).
			DoAndReturn(func(ctx context.Context, _ string, _ *workspacesdk.ProcessOutputOptions) (workspacesdk.ProcessOutputResponse, error) {
				return workspacesdk.ProcessOutputResponse{}, ctx.Err()
			}),
		// The agent returns the process the first attempt started.
		conn.EXPECT().StartProcess(gomock.Any(), gomock.Any()).
			DoAndReturn(recordStart(workspacesdk.StartProcessResponse{ID: processID, Started: true, AgeMs: 7000})),
		conn.EXPECT().ProcessOutput(gomock.Any(), processID, &workspacesdk.ProcessOutputOptions{Wait: true}).
			Return(workspacesdk.ProcessOutputResponse{ExitCode: &exitCode, Output: "PASS"}, nil),
	)
	tools := []fantasy.AgentTool{chattool.Execute(chattool.ExecuteOptions{
		GetWorkspaceConn: func(context.Context) (workspacesdk.AgentConn, error) { return conn, nil },
	})}

	err = executeLocalToolBatch(firstAttemptCtx, t, f, batch, tools)
	require.ErrorIs(t, err, context.Canceled)
	runLocalToolBatch(t, f, batch, tools)

	require.Len(t, toolCalls, 2)
	assert.Equal(t, assistant.ID, toolCalls[0].MessageID)
	assert.Equal(t, callID, toolCalls[0].ID)
	assert.Equal(t, toolCalls[0].MessageID, toolCalls[1].MessageID)
	assert.Equal(t, toolCalls[0].ID, toolCalls[1].ID)
	assert.Greater(t, toolCalls[1].Age, toolCalls[0].Age)

	messages, err = f.db.GetChatMessagesByChatID(ctx, database.GetChatMessagesByChatIDParams{ChatID: batch.chat.ID})
	require.NoError(t, err)
	parts, err := chatprompt.ParseContent(findToolResultMessage(t, messages, callID))
	require.NoError(t, err)
	require.Len(t, parts, 1)
	var result chattool.ExecuteResult
	require.NoError(t, json.Unmarshal(parts[0].Result, &result), string(parts[0].Result))
	assert.True(t, result.Success)
	assert.Equal(t, "PASS", result.Output)
	assert.GreaterOrEqual(t, result.WallDurationMs, int64(7000))
}
