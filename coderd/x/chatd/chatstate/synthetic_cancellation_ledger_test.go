package chatstate_test

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbgen"
	"github.com/coder/coder/v2/coderd/database/dbtime"
	"github.com/coder/coder/v2/coderd/x/chatd/chatprompt"
	"github.com/coder/coder/v2/coderd/x/chatd/chatstate"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/testutil"
)

// seedLedgerWorkspaceAgent creates the workspace scaffolding needed
// for an execution row's workspace_agent_id foreign key.
func seedLedgerWorkspaceAgent(t *testing.T, f *testFixture) database.WorkspaceAgent {
	t.Helper()
	tv := dbgen.TemplateVersion(t, f.DB, database.TemplateVersion{OrganizationID: f.Org.ID, CreatedBy: f.User.ID})
	tpl := dbgen.Template(t, f.DB, database.Template{CreatedBy: f.User.ID, OrganizationID: f.Org.ID, ActiveVersionID: tv.ID})
	ws := dbgen.Workspace(t, f.DB, database.WorkspaceTable{TemplateID: tpl.ID, OwnerID: f.User.ID, OrganizationID: f.Org.ID})
	pj := dbgen.ProvisionerJob(t, f.DB, nil, database.ProvisionerJob{InitiatorID: f.User.ID, OrganizationID: f.Org.ID})
	_ = dbgen.WorkspaceBuild(t, f.DB, database.WorkspaceBuild{TemplateVersionID: tv.ID, WorkspaceID: ws.ID, JobID: pj.ID})
	res := dbgen.WorkspaceResource(t, f.DB, database.WorkspaceResource{Transition: database.WorkspaceTransitionStart, JobID: pj.ID})
	return dbgen.WorkspaceAgent(t, f.DB, database.WorkspaceAgent{ResourceID: res.ID})
}

// TestEditMessage_MapsExecutionLedgerBeforeHistoryDelete asserts
// that editing a message whose deleted suffix carries in-flight
// execute calls still maps the ledger: the deleted turn gets no
// synthetic history results, so nothing can carry a background
// handle back to the user and both the foreground and background
// rows must become cancel_requested for the sweep to kill, with
// result_committed_at stamped.
func TestEditMessage_MapsExecutionLedgerBeforeHistoryDelete(t *testing.T) {
	t.Parallel()
	f := newTestFixture(t)
	ctx := testutil.Context(t, testutil.WaitShort)
	created := createTestChat(t, f)
	m := chatstate.NewChatMachine(f.DB, f.Pub, created.Chat.ID)
	agent := seedLedgerWorkspaceAgent(t, f)

	fgCallID := "call_fg_" + uuid.NewString()
	bgCallID := "call_bg_" + uuid.NewString()
	raw, err := chatprompt.MarshalParts([]codersdk.ChatMessagePart{
		{
			Type:       codersdk.ChatMessagePartTypeToolCall,
			ToolCallID: fgCallID,
			ToolName:   "execute",
			Args:       json.RawMessage(`{}`),
		},
		{
			Type:       codersdk.ChatMessagePartTypeToolCall,
			ToolCallID: bgCallID,
			ToolName:   "execute",
			Args:       json.RawMessage(`{}`),
		},
	})
	require.NoError(t, err)
	// The edited user message precedes the in-flight assistant
	// message, so the edit soft-deletes the assistant message too.
	var step chatstate.CommitStepResult
	require.NoError(t, m.Update(ctx, func(tx *chatstate.Tx, store database.Store) error {
		var err error
		step, err = tx.CommitStep(chatstate.CommitStepInput{Messages: []chatstate.Message{
			userTextMessage("first user", f.User.ID, f.Model.ID),
			{
				Role:           database.ChatMessageRoleAssistant,
				Content:        raw,
				Visibility:     database.ChatMessageVisibilityBoth,
				ContentVersion: chatprompt.CurrentContentVersion,
				ModelConfigID:  uuid.NullUUID{UUID: f.Model.ID, Valid: true},
			},
		}})
		return err
	}))
	require.Len(t, step.InsertedMessages, 2)
	firstUserID := step.InsertedMessages[0].ID
	assistantID := step.InsertedMessages[1].ID

	claim := func(toolCallID string, background bool) database.ChatToolCallExecution {
		row, err := f.DB.ClaimChatToolCallExecution(ctx, database.ClaimChatToolCallExecutionParams{
			ID:                 uuid.New(),
			ChatID:             created.Chat.ID,
			AssistantMessageID: assistantID,
			ToolCallID:         toolCallID,
			InputSha256:        "hash-" + toolCallID,
			Command:            "sleep 600",
			Background:         background,
			TimeoutSecs:        600,
			Now:                dbtime.Now(),
			StaleBefore:        time.Time{},
		})
		require.NoError(t, err)
		return row
	}
	recordProcess := func(row database.ChatToolCallExecution, processID string) {
		_, err := f.DB.UpdateChatToolCallExecutionProcess(ctx, database.UpdateChatToolCallExecutionProcessParams{
			ChatID:             created.Chat.ID,
			AssistantMessageID: assistantID,
			ToolCallID:         row.ToolCallID,
			ClaimEpoch:         row.ClaimEpoch,
			ProcessID:          processID,
			WorkspaceAgentID:   agent.ID,
			StartedAt:          dbtime.Now(),
		})
		require.NoError(t, err)
	}
	recordProcess(claim(fgCallID, false), "proc-fg")
	recordProcess(claim(bgCallID, true), "proc-bg")

	var edit chatstate.EditMessageResult
	editedContent := mustMarshalParts(t, []codersdk.ChatMessagePart{
		codersdk.ChatMessageText("edited"),
	})
	require.NoError(t, m.Update(ctx, func(tx *chatstate.Tx, store database.Store) error {
		var err error
		edit, err = tx.EditMessage(chatstate.EditMessageInput{
			MessageID: firstUserID,
			CreatedBy: f.User.ID,
			Content:   editedContent,
			APIKeyID:  f.apiKeyID(),
		})
		return err
	}))
	require.Empty(t, edit.CancellationMessages,
		"the deleted turn gets no synthetic history results")

	fgRow, err := f.DB.GetChatToolCallExecution(ctx, database.GetChatToolCallExecutionParams{
		ChatID:             created.Chat.ID,
		AssistantMessageID: assistantID,
		ToolCallID:         fgCallID,
	})
	require.NoError(t, err)
	require.Equal(t, database.ChatToolCallExecutionStatusCancelRequested, fgRow.Status,
		"the dispatched foreground row is left for the sweep to kill")
	require.True(t, fgRow.ResultCommittedAt.Valid)

	bgRow, err := f.DB.GetChatToolCallExecution(ctx, database.GetChatToolCallExecutionParams{
		ChatID:             created.Chat.ID,
		AssistantMessageID: assistantID,
		ToolCallID:         bgCallID,
	})
	require.NoError(t, err)
	require.Equal(t, database.ChatToolCallExecutionStatusCancelRequested, bgRow.Status,
		"no committed result can carry the handle, so the background row is killed, not spared")
	require.True(t, bgRow.ResultCommittedAt.Valid)
}

// TestEditMessage_KillsPriorDetachedExecutions asserts that editing
// a message routes previously detached rows anchored in the deleted
// suffix to cancel_requested: the interrupt-spared synthetic result
// was the only carrier of the process handle and the edit deletes
// it, so the process must be killed, not left alive and
// unaddressable. A detached row anchored before the edited message
// keeps its carrier and must survive.
func TestEditMessage_KillsPriorDetachedExecutions(t *testing.T) {
	t.Parallel()

	// Builds a chat whose first turn holds a background execute
	// spared to detached by a new-user-message cancellation, exactly
	// the CODAGT-757 orphan shape: user1, assistant(bg call),
	// process recorded, turn finished, user2 sent (maps the row
	// detached and commits the handle-carrying synthetic result).
	setup := func(t *testing.T) (f *testFixture, m *chatstate.ChatMachine, chatID uuid.UUID, user1ID, assistantID, user2ID int64, bgCallID string) {
		f = newTestFixture(t)
		ctx := testutil.Context(t, testutil.WaitShort)
		created := createTestChat(t, f)
		m = chatstate.NewChatMachine(f.DB, f.Pub, created.Chat.ID)
		agent := seedLedgerWorkspaceAgent(t, f)

		bgCallID = "call_bg_" + uuid.NewString()
		raw, err := chatprompt.MarshalParts([]codersdk.ChatMessagePart{{
			Type:       codersdk.ChatMessagePartTypeToolCall,
			ToolCallID: bgCallID,
			ToolName:   "execute",
			Args:       json.RawMessage(`{}`),
		}})
		require.NoError(t, err)
		var step chatstate.CommitStepResult
		require.NoError(t, m.Update(ctx, func(tx *chatstate.Tx, store database.Store) error {
			var err error
			step, err = tx.CommitStep(chatstate.CommitStepInput{Messages: []chatstate.Message{
				userTextMessage("first user", f.User.ID, f.Model.ID),
				{
					Role:           database.ChatMessageRoleAssistant,
					Content:        raw,
					Visibility:     database.ChatMessageVisibilityBoth,
					ContentVersion: chatprompt.CurrentContentVersion,
					ModelConfigID:  uuid.NullUUID{UUID: f.Model.ID, Valid: true},
				},
			}})
			return err
		}))
		require.Len(t, step.InsertedMessages, 2)
		user1ID = step.InsertedMessages[0].ID
		assistantID = step.InsertedMessages[1].ID

		row, err := f.DB.ClaimChatToolCallExecution(ctx, database.ClaimChatToolCallExecutionParams{
			ID:                 uuid.New(),
			ChatID:             created.Chat.ID,
			AssistantMessageID: assistantID,
			ToolCallID:         bgCallID,
			InputSha256:        "hash-" + bgCallID,
			Command:            "sleep 600",
			Background:         true,
			TimeoutSecs:        600,
			Now:                dbtime.Now(),
			StaleBefore:        time.Time{},
		})
		require.NoError(t, err)
		_, err = f.DB.UpdateChatToolCallExecutionProcess(ctx, database.UpdateChatToolCallExecutionProcessParams{
			ChatID:             created.Chat.ID,
			AssistantMessageID: assistantID,
			ToolCallID:         bgCallID,
			ClaimEpoch:         row.ClaimEpoch,
			ProcessID:          "proc-bg",
			WorkspaceAgentID:   agent.ID,
			StartedAt:          dbtime.Now(),
		})
		require.NoError(t, err)

		landInW(t, f, m)

		var send chatstate.SendMessageResult
		require.NoError(t, m.Update(ctx, func(tx *chatstate.Tx, store database.Store) error {
			var err error
			send, err = tx.SendMessage(chatstate.SendMessageInput{
				Message:      userTextMessage("second user", f.User.ID, f.Model.ID),
				BusyBehavior: chatstate.BusyBehaviorQueue,
			})
			return err
		}))
		require.Len(t, send.InsertedMessages, 2, "synthetic spare + new user")
		for _, msg := range send.InsertedMessages {
			if msg.Role == database.ChatMessageRoleUser {
				user2ID = msg.ID
			}
		}
		require.NotZero(t, user2ID)

		bgRow, err := f.DB.GetChatToolCallExecution(ctx, database.GetChatToolCallExecutionParams{
			ChatID:             created.Chat.ID,
			AssistantMessageID: assistantID,
			ToolCallID:         bgCallID,
		})
		require.NoError(t, err)
		require.Equal(t, database.ChatToolCallExecutionStatusDetached, bgRow.Status,
			"precondition: the background row was spared to detached")
		return f, m, created.Chat.ID, user1ID, assistantID, user2ID, bgCallID
	}

	edit := func(t *testing.T, f *testFixture, m *chatstate.ChatMachine, messageID int64) {
		ctx := testutil.Context(t, testutil.WaitShort)
		require.NoError(t, m.Update(ctx, func(tx *chatstate.Tx, store database.Store) error {
			_, err := tx.EditMessage(chatstate.EditMessageInput{
				MessageID: messageID,
				CreatedBy: f.User.ID,
				Content:   mustMarshalParts(t, []codersdk.ChatMessagePart{codersdk.ChatMessageText("edited")}),
				APIKeyID:  f.apiKeyID(),
			})
			return err
		}))
	}

	t.Run("DeletedCarrierIsKilled", func(t *testing.T) {
		t.Parallel()
		f, m, chatID, user1ID, assistantID, _, bgCallID := setup(t)
		ctx := testutil.Context(t, testutil.WaitShort)

		// Editing user1 deletes the whole suffix including the
		// synthetic result that carried the detached row's handle.
		edit(t, f, m, user1ID)

		bgRow, err := f.DB.GetChatToolCallExecution(ctx, database.GetChatToolCallExecutionParams{
			ChatID:             chatID,
			AssistantMessageID: assistantID,
			ToolCallID:         bgCallID,
		})
		require.NoError(t, err)
		require.Equal(t, database.ChatToolCallExecutionStatusCancelRequested, bgRow.Status,
			"the deleted suffix carried the handle, so the detached row is killed")
	})

	t.Run("SurvivingCarrierIsSpared", func(t *testing.T) {
		t.Parallel()
		f, m, chatID, _, assistantID, user2ID, bgCallID := setup(t)
		ctx := testutil.Context(t, testutil.WaitShort)

		// Editing user2 keeps the first turn, including the
		// synthetic result carrying the detached row's handle.
		edit(t, f, m, user2ID)

		bgRow, err := f.DB.GetChatToolCallExecution(ctx, database.GetChatToolCallExecutionParams{
			ChatID:             chatID,
			AssistantMessageID: assistantID,
			ToolCallID:         bgCallID,
		})
		require.NoError(t, err)
		require.Equal(t, database.ChatToolCallExecutionStatusDetached, bgRow.Status,
			"the handle carrier survives the edit, so the detached row stays spared")
	})
}

// TestSyntheticCancellation_MapsExecutionLedger asserts that a
// non-interrupt cancellation transition (a new user message during a
// run) maps the execution ledger exactly like the interrupt commit:
// a dispatched foreground row becomes cancel_requested for the sweep
// to kill, a spared background row's synthetic result carries its
// process handle, and both get result_committed_at stamped so the
// purge can reap them.
func TestSyntheticCancellation_MapsExecutionLedger(t *testing.T) {
	t.Parallel()
	f := newTestFixture(t)
	ctx := testutil.Context(t, testutil.WaitShort)
	created := createTestChat(t, f)
	m := chatstate.NewChatMachine(f.DB, f.Pub, created.Chat.ID)
	agent := seedLedgerWorkspaceAgent(t, f)

	fgCallID := "call_fg_" + uuid.NewString()
	bgCallID := "call_bg_" + uuid.NewString()
	raw, err := chatprompt.MarshalParts([]codersdk.ChatMessagePart{
		{
			Type:       codersdk.ChatMessagePartTypeToolCall,
			ToolCallID: fgCallID,
			ToolName:   "execute",
			Args:       json.RawMessage(`{}`),
		},
		{
			Type:       codersdk.ChatMessagePartTypeToolCall,
			ToolCallID: bgCallID,
			ToolName:   "execute",
			Args:       json.RawMessage(`{}`),
		},
	})
	require.NoError(t, err)
	assistant := commitAssistantToolCall(t, f, m, chatstate.Message{
		Role:           database.ChatMessageRoleAssistant,
		Content:        raw,
		Visibility:     database.ChatMessageVisibilityBoth,
		ContentVersion: chatprompt.CurrentContentVersion,
		ModelConfigID:  uuid.NullUUID{UUID: f.Model.ID, Valid: true},
	})

	claim := func(toolCallID string, background bool) database.ChatToolCallExecution {
		row, err := f.DB.ClaimChatToolCallExecution(ctx, database.ClaimChatToolCallExecutionParams{
			ID:                 uuid.New(),
			ChatID:             created.Chat.ID,
			AssistantMessageID: assistant.ID,
			ToolCallID:         toolCallID,
			InputSha256:        "hash-" + toolCallID,
			Command:            "sleep 600",
			Background:         background,
			TimeoutSecs:        600,
			Now:                dbtime.Now(),
			StaleBefore:        time.Time{},
		})
		require.NoError(t, err)
		return row
	}
	recordProcess := func(row database.ChatToolCallExecution, processID string) {
		_, err := f.DB.UpdateChatToolCallExecutionProcess(ctx, database.UpdateChatToolCallExecutionProcessParams{
			ChatID:             created.Chat.ID,
			AssistantMessageID: assistant.ID,
			ToolCallID:         row.ToolCallID,
			ClaimEpoch:         row.ClaimEpoch,
			ProcessID:          processID,
			WorkspaceAgentID:   agent.ID,
			StartedAt:          dbtime.Now(),
		})
		require.NoError(t, err)
	}
	recordProcess(claim(fgCallID, false), "proc-fg")
	recordProcess(claim(bgCallID, true), "proc-bg")

	landInW(t, f, m)

	var send chatstate.SendMessageResult
	require.NoError(t, m.Update(ctx, func(tx *chatstate.Tx, store database.Store) error {
		var err error
		send, err = tx.SendMessage(chatstate.SendMessageInput{
			Message:      userTextMessage("after-cancel", f.User.ID, f.Model.ID),
			BusyBehavior: chatstate.BusyBehaviorQueue,
		})
		return err
	}))
	require.Len(t, send.InsertedMessages, 3, "two synthetic cancels + new user")

	fgRow, err := f.DB.GetChatToolCallExecution(ctx, database.GetChatToolCallExecutionParams{
		ChatID:             created.Chat.ID,
		AssistantMessageID: assistant.ID,
		ToolCallID:         fgCallID,
	})
	require.NoError(t, err)
	require.Equal(t, database.ChatToolCallExecutionStatusCancelRequested, fgRow.Status,
		"the dispatched foreground row is left for the sweep to kill")
	require.True(t, fgRow.ResultCommittedAt.Valid, "the synthetic result committed")

	bgRow, err := f.DB.GetChatToolCallExecution(ctx, database.GetChatToolCallExecutionParams{
		ChatID:             created.Chat.ID,
		AssistantMessageID: assistant.ID,
		ToolCallID:         bgCallID,
	})
	require.NoError(t, err)
	require.Equal(t, database.ChatToolCallExecutionStatusDetached, bgRow.Status,
		"the background row with a handle is spared")
	require.True(t, bgRow.ResultCommittedAt.Valid)

	// The spared background call's synthetic result carries its
	// process handle instead of the generic cancellation error.
	var sawBackgroundHandle bool
	for _, msg := range send.InsertedMessages {
		if msg.Role != database.ChatMessageRoleTool {
			continue
		}
		parts, err := chatprompt.ParseContent(msg)
		require.NoError(t, err)
		for _, p := range parts {
			if p.Type != codersdk.ChatMessagePartTypeToolResult {
				continue
			}
			switch p.ToolCallID {
			case fgCallID:
				require.True(t, p.IsError)
			case bgCallID:
				require.False(t, p.IsError)
				require.Contains(t, string(p.Result), "proc-bg")
				sawBackgroundHandle = true
			}
		}
	}
	require.True(t, sawBackgroundHandle, "expected a background result carrying the handle")
}
