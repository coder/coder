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

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/x/chatd/chatstate"
	"github.com/coder/coder/v2/coderd/x/chatd/chattool"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/testutil"
)

// runLocalToolBatch runs the chat's unresolved tool calls once through
// executeLocalTools with tools, as one generation attempt does.
func runLocalToolBatch(t *testing.T, f *taskTestFixture, batch interruptedBatch, tools []fantasy.AgentTool) {
	t.Helper()
	ctx := testutil.Context(t, testutil.WaitLong)
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
	err = batch.starter.executeLocalTools(ctx, chatstate.NewChatMachine(f.db, f.pubsub, chat.ID), chatWorkerTaskStartInput{
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
	require.NoError(t, err)
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
