package chatd //nolint:testpackage // Tests the unexported interrupt result payload builder.

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbgen"
	"github.com/coder/coder/v2/coderd/database/dbtestutil"
	"github.com/coder/coder/v2/coderd/database/dbtime"
	"github.com/coder/coder/v2/coderd/x/chatd/chattool"
	"github.com/coder/coder/v2/testutil"
)

// TestInterruptedToolResultPayload asserts that the synthetic result
// committed at interrupt carries the process handle for a background
// execute whose handle already landed, and the generic cancellation
// error for everything else.
func TestInterruptedToolResultPayload(t *testing.T) {
	t.Parallel()
	db, _ := dbtestutil.NewDB(t)
	ctx := testutil.Context(t, testutil.WaitLong)

	user := dbgen.User(t, db, database.User{})
	org := dbgen.Organization(t, db, database.Organization{})
	_ = dbgen.OrganizationMember(t, db, database.OrganizationMember{UserID: user.ID, OrganizationID: org.ID})
	_ = dbgen.ChatProvider(t, db, database.ChatProvider{Provider: "openai", DisplayName: "OpenAI"})
	mc := dbgen.ChatModelConfig(t, db, database.ChatModelConfig{Model: "test-model", ContextLimit: 8192})
	chat := dbgen.Chat(t, db, database.Chat{
		OrganizationID:    org.ID,
		OwnerID:           user.ID,
		LastModelConfigID: mc.ID,
		Title:             "interrupt-payload",
	})
	msg := dbgen.ChatMessage(t, db, database.ChatMessage{
		ChatID: chat.ID,
		Role:   database.ChatMessageRoleAssistant,
	})
	agent := seedSweepWorkspaceAgent(t, db, user.ID, org.ID)

	claim := func(toolCallID string, background bool) database.ChatToolCallExecution {
		row, err := db.ClaimChatToolCallExecution(ctx, database.ClaimChatToolCallExecutionParams{
			ID:                 uuid.New(),
			ChatID:             chat.ID,
			AssistantMessageID: msg.ID,
			ToolCallID:         toolCallID,
			InputSha256:        "hash",
			Command:            "sleep 600",
			Background:         background,
			TimeoutSecs:        600,
			Now:                dbtime.Now(),
			StaleBefore:        time.Time{},
		})
		require.NoError(t, err)
		return row
	}

	bg := claim("bg-call", true)
	_, err := db.UpdateChatToolCallExecutionProcess(ctx, database.UpdateChatToolCallExecutionProcessParams{
		ChatID:             chat.ID,
		AssistantMessageID: msg.ID,
		ToolCallID:         "bg-call",
		ClaimEpoch:         bg.ClaimEpoch,
		ProcessID:          "proc-bg",
		WorkspaceAgentID:   agent.ID,
		StartedAt:          dbtime.Now(),
	})
	require.NoError(t, err)

	payload, isError, err := interruptedToolResultPayload(ctx, db, chat.ID, msg.ID, "bg-call")
	require.NoError(t, err)
	require.False(t, isError)
	var result chattool.ExecuteResult
	require.NoError(t, json.Unmarshal(payload, &result))
	require.True(t, result.Success)
	require.Equal(t, "proc-bg", result.BackgroundProcessID)
	require.NotEmpty(t, result.Note)

	// A foreground row with a handle still cancels: the interrupt
	// reconciler kills its process.
	fg := claim("fg-call", false)
	_, err = db.UpdateChatToolCallExecutionProcess(ctx, database.UpdateChatToolCallExecutionProcessParams{
		ChatID:             chat.ID,
		AssistantMessageID: msg.ID,
		ToolCallID:         "fg-call",
		ClaimEpoch:         fg.ClaimEpoch,
		ProcessID:          "proc-fg",
		WorkspaceAgentID:   agent.ID,
		StartedAt:          dbtime.Now(),
	})
	require.NoError(t, err)
	payload, isError, err = interruptedToolResultPayload(ctx, db, chat.ID, msg.ID, "fg-call")
	require.NoError(t, err)
	require.True(t, isError)
	require.Contains(t, string(payload), "error")

	// A call without a ledger row gets the generic cancellation.
	payload, isError, err = interruptedToolResultPayload(ctx, db, chat.ID, msg.ID, "no-row")
	require.NoError(t, err)
	require.True(t, isError)
	require.Contains(t, string(payload), "error")
}
