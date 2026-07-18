package chatstate

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/sqlc-dev/pqtype"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbgen"
	"github.com/coder/coder/v2/coderd/database/dbtestutil"
	"github.com/coder/coder/v2/coderd/database/dbtime"
	"github.com/coder/coder/v2/coderd/x/chatd/chatprompt"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/testutil"
)

// TestMapExecutionsForHistoryDeleteMalformedAssistant asserts that
// an unparseable last assistant message cannot wedge the deleting
// edit: the pending-call scan is skipped and the broad status-based
// mapping still routes dispatched rows to the sweep.
func TestMapExecutionsForHistoryDeleteMalformedAssistant(t *testing.T) {
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
		Title:             "malformed-edit",
	})
	msg := dbgen.ChatMessage(t, db, database.ChatMessage{
		ChatID:  chat.ID,
		Role:    database.ChatMessageRoleAssistant,
		Content: pqtype.NullRawMessage{RawMessage: json.RawMessage(`{"not": "parts"}`), Valid: true},
	})

	claimed, err := db.ClaimChatToolCallExecution(ctx, database.ClaimChatToolCallExecutionParams{
		ID:                 uuid.New(),
		ChatID:             chat.ID,
		AssistantMessageID: msg.ID,
		ToolCallID:         "edit-call",
		InputSha256:        "hash",
		Command:            "sleep 600",
		TimeoutSecs:        600,
		Now:                dbtime.Now(),
		StaleBefore:        time.Time{},
	})
	require.NoError(t, err)
	require.Equal(t, database.ChatToolCallExecutionStatusStarting, claimed.Status)

	require.NoError(t, mapExecutionsForHistoryDelete(ctx, db, chat, msg.ID))

	row, err := db.GetChatToolCallExecution(ctx, database.GetChatToolCallExecutionParams{
		ChatID:             chat.ID,
		AssistantMessageID: msg.ID,
		ToolCallID:         "edit-call",
	})
	require.NoError(t, err)
	require.Equal(t, database.ChatToolCallExecutionStatusCancelRequested, row.Status)
}

// TestMapExecutionsForHistoryDeleteSurvivingAssistant asserts that
// arm 1 skips a last assistant message that survives the deletion:
// its outstanding calls belong to the post-delete synthetic
// cancellation, which can still spare recorded background handles.
func TestMapExecutionsForHistoryDeleteSurvivingAssistant(t *testing.T) {
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
		Title:             "surviving-assistant-edit",
	})
	assistantParts := []codersdk.ChatMessagePart{{
		Type:       codersdk.ChatMessagePartTypeToolCall,
		ToolCallID: "surviving-call",
		ToolName:   "execute",
		Args:       json.RawMessage(`{"command":"sleep 600","run_in_background":true}`),
	}}
	assistantContent, err := chatprompt.MarshalParts(assistantParts)
	require.NoError(t, err)
	assistant := dbgen.ChatMessage(t, db, database.ChatMessage{
		ChatID:         chat.ID,
		Role:           database.ChatMessageRoleAssistant,
		Content:        assistantContent,
		ContentVersion: chatprompt.CurrentContentVersion,
	})
	// The edit targets a later user message; the assistant above
	// survives the deletion.
	edited := dbgen.ChatMessage(t, db, database.ChatMessage{
		ChatID: chat.ID,
		Role:   database.ChatMessageRoleUser,
	})

	claimed, err := db.ClaimChatToolCallExecution(ctx, database.ClaimChatToolCallExecutionParams{
		ID:                 uuid.New(),
		ChatID:             chat.ID,
		AssistantMessageID: assistant.ID,
		ToolCallID:         "surviving-call",
		InputSha256:        "hash",
		Command:            "sleep 600",
		Background:         true,
		TimeoutSecs:        0,
		Now:                dbtime.Now(),
		StaleBefore:        time.Time{},
	})
	require.NoError(t, err)
	require.Equal(t, database.ChatToolCallExecutionStatusStarting, claimed.Status)

	require.NoError(t, mapExecutionsForHistoryDelete(ctx, db, chat, edited.ID))

	// The surviving assistant's row is untouched: the post-delete
	// synthetic cancellation owns its resolution.
	row, err := db.GetChatToolCallExecution(ctx, database.GetChatToolCallExecutionParams{
		ChatID:             chat.ID,
		AssistantMessageID: assistant.ID,
		ToolCallID:         "surviving-call",
	})
	require.NoError(t, err)
	require.Equal(t, database.ChatToolCallExecutionStatusStarting, row.Status)
}
