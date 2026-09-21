package chatd

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"charm.land/fantasy"
	"github.com/google/uuid"
	"github.com/prometheus/client_golang/prometheus"
	promtestutil "github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/x/chatd/chathooks"
	"github.com/coder/coder/v2/coderd/x/chatd/chatloop"
	"github.com/coder/coder/v2/coderd/x/chatd/chatprompt"
	"github.com/coder/coder/v2/coderd/x/chatd/chatstate"
	"github.com/coder/coder/v2/codersdk"
)

func runContextTool(t *testing.T, tool fantasy.AgentTool, callID string, args map[string]any) fantasy.ToolResponse {
	t.Helper()
	input, err := json.Marshal(args)
	require.NoError(t, err)
	response, err := tool.Run(context.Background(), fantasy.ToolCall{
		ID:    callID,
		Name:  tool.Info().Name,
		Input: string(input),
	})
	require.NoError(t, err)
	return response
}

// contextToolHistory returns a decision view with one user prompt and
// the assistant row that carries the context tool call callID.
func contextToolHistory(t *testing.T, toolName, callID string) []database.ChatMessage {
	t.Helper()
	return []database.ChatMessage{
		dbMessage(t, 1, database.ChatMessageRoleUser, false, codersdk.ChatMessageText("do the work")),
		dbMessage(t, 2, database.ChatMessageRoleAssistant, false, codersdk.ChatMessageToolCall(callID, toolName, json.RawMessage(`{"follow_up":"next"}`))),
	}
}

func TestContextTools_ArgumentValidation(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name     string
		followUp string
		wantErr  string
	}{
		{name: "blank", followUp: "", wantErr: "follow_up is required"},
		{name: "whitespace", followUp: " \n\t", wantErr: "follow_up is required"},
		{name: "too long", followUp: strings.Repeat("a", contextFollowUpMaxRunes+1), wantErr: "exceeds 8000 characters"},
		{name: "too long multibyte", followUp: strings.Repeat("é", contextFollowUpMaxRunes+1), wantErr: "exceeds 8000 characters"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			tool := clearContextTool(contextToolHistory(t, clearContextToolName, "clear-1"))
			response := runContextTool(t, tool, "clear-1", map[string]any{"follow_up": tc.followUp})
			require.True(t, response.IsError)
			require.Contains(t, response.Content, tc.wantErr)
		})
	}

	t.Run("max length multibyte accepted", func(t *testing.T) {
		t.Parallel()
		followUp := strings.Repeat("é", contextFollowUpMaxRunes)
		tool := clearContextTool(contextToolHistory(t, clearContextToolName, "clear-1"))
		response := runContextTool(t, tool, "clear-1", map[string]any{"follow_up": followUp})
		require.False(t, response.IsError)
		require.Equal(t, "Context cleared. Follow-up: "+followUp, response.Content)
	})
}

func TestHasContextSinceLastBoundary(t *testing.T) {
	t.Parallel()

	t.Run("only the calling assistant row after a boundary", func(t *testing.T) {
		t.Parallel()
		messages := append(clearBoundaryTriplet(t, 1),
			dbMessage(t, 4, database.ChatMessageRoleAssistant, false, codersdk.ChatMessageToolCall("clear-2", clearContextToolName, json.RawMessage(`{"follow_up":"again"}`))),
		)
		require.False(t, hasContextSinceLastBoundary(messages, "clear-2"))
	})

	t.Run("only a system hook notice after a boundary", func(t *testing.T) {
		t.Parallel()
		notice := dbMessage(t, 4, database.ChatMessageRoleSystem, false, codersdk.ChatMessagePart{Type: codersdk.ChatMessagePartTypeHookNotice, Text: "hook"})
		notice.Visibility = database.ChatMessageVisibilityUser
		messages := append(clearBoundaryTriplet(t, 1),
			notice,
			dbMessage(t, 5, database.ChatMessageRoleAssistant, false, codersdk.ChatMessageToolCall("clear-2", clearContextToolName, json.RawMessage(`{"follow_up":"again"}`))),
		)
		require.False(t, hasContextSinceLastBoundary(messages, "clear-2"))
	})

	t.Run("user prompt after a boundary", func(t *testing.T) {
		t.Parallel()
		messages := append(clearBoundaryTriplet(t, 1),
			dbMessage(t, 4, database.ChatMessageRoleUser, false, codersdk.ChatMessageText("more work")),
			dbMessage(t, 5, database.ChatMessageRoleAssistant, false, codersdk.ChatMessageToolCall("clear-2", clearContextToolName, json.RawMessage(`{"follow_up":"again"}`))),
		)
		require.True(t, hasContextSinceLastBoundary(messages, "clear-2"))
	})

	t.Run("earlier assistant step after a boundary", func(t *testing.T) {
		t.Parallel()
		messages := append(clearBoundaryTriplet(t, 1),
			dbMessage(t, 4, database.ChatMessageRoleAssistant, false, codersdk.ChatMessageToolCall("read-1", "read_file", json.RawMessage(`{}`))),
			dbMessage(t, 5, database.ChatMessageRoleTool, false, codersdk.ChatMessageToolResult("read-1", "read_file", json.RawMessage(`{}`), false, false)),
			dbMessage(t, 6, database.ChatMessageRoleAssistant, false, codersdk.ChatMessageToolCall("clear-2", clearContextToolName, json.RawMessage(`{"follow_up":"again"}`))),
		)
		require.True(t, hasContextSinceLastBoundary(messages, "clear-2"))
	})

	t.Run("no boundary at all", func(t *testing.T) {
		t.Parallel()
		require.True(t, hasContextSinceLastBoundary(contextToolHistory(t, clearContextToolName, "clear-1"), "clear-1"))
	})

	t.Run("deleted rows do not count", func(t *testing.T) {
		t.Parallel()
		deleted := dbMessage(t, 4, database.ChatMessageRoleUser, false, codersdk.ChatMessageText("gone"))
		deleted.Deleted = true
		messages := append(clearBoundaryTriplet(t, 1),
			deleted,
			dbMessage(t, 5, database.ChatMessageRoleAssistant, false, codersdk.ChatMessageToolCall("clear-2", clearContextToolName, json.RawMessage(`{"follow_up":"again"}`))),
		)
		require.False(t, hasContextSinceLastBoundary(messages, "clear-2"))
	})
}

func TestClearContextTool_NothingNewGuard(t *testing.T) {
	t.Parallel()

	messages := append(clearBoundaryTriplet(t, 1),
		dbMessage(t, 4, database.ChatMessageRoleAssistant, false, codersdk.ChatMessageToolCall("clear-2", clearContextToolName, json.RawMessage(`{"follow_up":"again"}`))),
	)
	response := runContextTool(t, clearContextTool(messages), "clear-2", map[string]any{"follow_up": "again"})
	require.True(t, response.IsError)
	require.Contains(t, response.Content, "nothing has happened since the last context boundary")
}

func TestCompactContextTool(t *testing.T) {
	t.Parallel()

	successfulCompactResult := func(id int64, callID string) database.ChatMessage {
		return dbMessage(t, id, database.ChatMessageRoleTool, false, codersdk.ChatMessageToolResult(callID, compactContextToolName, json.RawMessage(`{"output":"Compaction scheduled."}`), false, false))
	}

	t.Run("succeeds with work after the boundary", func(t *testing.T) {
		t.Parallel()
		response := runContextTool(t, compactContextTool(contextToolHistory(t, compactContextToolName, "compact-1")), "compact-1", map[string]any{"follow_up": "resume"})
		require.False(t, response.IsError)
		require.Equal(t, "Compaction scheduled. Follow-up: resume", response.Content)
	})

	t.Run("blank follow-up rejected", func(t *testing.T) {
		t.Parallel()
		response := runContextTool(t, compactContextTool(contextToolHistory(t, compactContextToolName, "compact-1")), "compact-1", map[string]any{"follow_up": ""})
		require.True(t, response.IsError)
		require.Contains(t, response.Content, "follow_up is required")
	})

	t.Run("nothing new since boundary rejected", func(t *testing.T) {
		t.Parallel()
		messages := append(clearBoundaryTriplet(t, 1),
			dbMessage(t, 4, database.ChatMessageRoleAssistant, false, codersdk.ChatMessageToolCall("compact-2", compactContextToolName, json.RawMessage(`{"follow_up":"again"}`))),
		)
		response := runContextTool(t, compactContextTool(messages), "compact-2", map[string]any{"follow_up": "again"})
		require.True(t, response.IsError)
		require.Contains(t, response.Content, "nothing has happened since the last context boundary")
	})

	t.Run("failed earlier request in the same segment rejected", func(t *testing.T) {
		t.Parallel()
		messages := []database.ChatMessage{
			dbMessage(t, 1, database.ChatMessageRoleUser, false, codersdk.ChatMessageText("do the work")),
			dbMessage(t, 2, database.ChatMessageRoleAssistant, false, codersdk.ChatMessageToolCall("compact-1", compactContextToolName, json.RawMessage(`{"follow_up":"first"}`))),
			successfulCompactResult(3, "compact-1"),
			dbMessage(t, 4, database.ChatMessageRoleAssistant, false, codersdk.ChatMessageToolCall("compact-2", compactContextToolName, json.RawMessage(`{"follow_up":"second"}`))),
		}
		response := runContextTool(t, compactContextTool(messages), "compact-2", map[string]any{"follow_up": "second"})
		require.True(t, response.IsError)
		require.Contains(t, response.Content, "did not produce a boundary")
	})

	t.Run("earlier request followed by a boundary accepted", func(t *testing.T) {
		t.Parallel()
		messages := []database.ChatMessage{
			dbMessage(t, 1, database.ChatMessageRoleUser, false, codersdk.ChatMessageText("do the work")),
			dbMessage(t, 2, database.ChatMessageRoleAssistant, false, codersdk.ChatMessageToolCall("compact-1", compactContextToolName, json.RawMessage(`{"follow_up":"first"}`))),
			successfulCompactResult(3, "compact-1"),
		}
		messages = append(messages, clearBoundaryTriplet(t, 4)...)
		messages = append(messages,
			dbMessage(t, 7, database.ChatMessageRoleAssistant, false, codersdk.ChatMessageText("more work")),
			dbMessage(t, 8, database.ChatMessageRoleAssistant, false, codersdk.ChatMessageToolCall("compact-2", compactContextToolName, json.RawMessage(`{"follow_up":"second"}`))),
		)
		response := runContextTool(t, compactContextTool(messages), "compact-2", map[string]any{"follow_up": "second"})
		require.False(t, response.IsError)
	})

	t.Run("earlier rejected request does not block", func(t *testing.T) {
		t.Parallel()
		messages := []database.ChatMessage{
			dbMessage(t, 1, database.ChatMessageRoleUser, false, codersdk.ChatMessageText("do the work")),
			dbMessage(t, 2, database.ChatMessageRoleAssistant, false, codersdk.ChatMessageToolCall("compact-1", compactContextToolName, json.RawMessage(`{"follow_up":""}`))),
			dbMessage(t, 3, database.ChatMessageRoleTool, false, codersdk.ChatMessageToolResult("compact-1", compactContextToolName, json.RawMessage(`{"error":"blank"}`), true, false)),
			dbMessage(t, 4, database.ChatMessageRoleAssistant, false, codersdk.ChatMessageToolCall("compact-2", compactContextToolName, json.RawMessage(`{"follow_up":"second"}`))),
		}
		response := runContextTool(t, compactContextTool(messages), "compact-2", map[string]any{"follow_up": "second"})
		require.False(t, response.IsError)
	})
}

func TestCompactionSourceFromHistory(t *testing.T) {
	t.Parallel()

	t.Run("agent when the last tool row is a successful compact_context result", func(t *testing.T) {
		t.Parallel()
		messages := []database.ChatMessage{
			dbMessage(t, 1, database.ChatMessageRoleUser, false, codersdk.ChatMessageText("question")),
			dbMessage(t, 2, database.ChatMessageRoleAssistant, false, codersdk.ChatMessageToolCall("compact-1", compactContextToolName, json.RawMessage(`{"follow_up":"resume"}`))),
			dbMessage(t, 3, database.ChatMessageRoleTool, false, codersdk.ChatMessageToolResult("compact-1", compactContextToolName, json.RawMessage(`{"output":"ok"}`), false, false)),
		}
		require.Equal(t, chatloop.CompactionSourceAgent, compactionSourceFromHistory(messages))
	})

	t.Run("manual when the last tool row is another tool", func(t *testing.T) {
		t.Parallel()
		messages := []database.ChatMessage{
			dbMessage(t, 1, database.ChatMessageRoleUser, false, codersdk.ChatMessageText("question")),
			dbMessage(t, 2, database.ChatMessageRoleAssistant, false, codersdk.ChatMessageToolCall("read-1", "read_file", json.RawMessage(`{}`))),
			dbMessage(t, 3, database.ChatMessageRoleTool, false, codersdk.ChatMessageToolResult("read-1", "read_file", json.RawMessage(`{}`), false, false)),
			dbMessage(t, 4, database.ChatMessageRoleAssistant, false, codersdk.ChatMessageText("answer")),
		}
		require.Equal(t, chatloop.CompactionSourceManual, compactionSourceFromHistory(messages))
	})

	t.Run("agent when only system rows follow the compact_context result", func(t *testing.T) {
		t.Parallel()
		notice := dbMessage(t, 4, database.ChatMessageRoleSystem, false, codersdk.ChatMessagePart{Type: codersdk.ChatMessagePartTypeHookNotice, Text: "hook"})
		notice.Visibility = database.ChatMessageVisibilityUser
		messages := []database.ChatMessage{
			dbMessage(t, 1, database.ChatMessageRoleUser, false, codersdk.ChatMessageText("question")),
			dbMessage(t, 2, database.ChatMessageRoleAssistant, false, codersdk.ChatMessageToolCall("compact-1", compactContextToolName, json.RawMessage(`{"follow_up":"resume"}`))),
			dbMessage(t, 3, database.ChatMessageRoleTool, false, codersdk.ChatMessageToolResult("compact-1", compactContextToolName, json.RawMessage(`{"output":"ok"}`), false, false)),
			notice,
		}
		require.Equal(t, chatloop.CompactionSourceAgent, compactionSourceFromHistory(messages))
	})

	t.Run("manual when a user prompt or assistant row follows the compact_context result", func(t *testing.T) {
		t.Parallel()
		base := []database.ChatMessage{
			dbMessage(t, 1, database.ChatMessageRoleUser, false, codersdk.ChatMessageText("question")),
			dbMessage(t, 2, database.ChatMessageRoleAssistant, false, codersdk.ChatMessageToolCall("compact-1", compactContextToolName, json.RawMessage(`{"follow_up":"resume"}`))),
			dbMessage(t, 3, database.ChatMessageRoleTool, false, codersdk.ChatMessageToolResult("compact-1", compactContextToolName, json.RawMessage(`{"output":"ok"}`), false, false)),
		}
		withPrompt := append(append([]database.ChatMessage{}, base...), dbMessage(t, 4, database.ChatMessageRoleUser, false, codersdk.ChatMessageText("new prompt")))
		require.Equal(t, chatloop.CompactionSourceManual, compactionSourceFromHistory(withPrompt))
		withReply := append(append([]database.ChatMessage{}, base...), dbMessage(t, 4, database.ChatMessageRoleAssistant, false, codersdk.ChatMessageText("reply")))
		require.Equal(t, chatloop.CompactionSourceManual, compactionSourceFromHistory(withReply))
	})

	t.Run("manual when the compact_context result is behind a later boundary", func(t *testing.T) {
		t.Parallel()
		messages := []database.ChatMessage{
			dbMessage(t, 1, database.ChatMessageRoleUser, false, codersdk.ChatMessageText("question")),
			dbMessage(t, 2, database.ChatMessageRoleAssistant, false, codersdk.ChatMessageToolCall("compact-1", compactContextToolName, json.RawMessage(`{"follow_up":"resume"}`))),
			dbMessage(t, 3, database.ChatMessageRoleTool, false, codersdk.ChatMessageToolResult("compact-1", compactContextToolName, json.RawMessage(`{"output":"ok"}`), false, false)),
			dbMessage(t, 4, database.ChatMessageRoleAssistant, true, codersdk.ChatMessageToolCall("summary-1", "chat_summarized", json.RawMessage(`{"source":"agent"}`))),
			dbMessage(t, 5, database.ChatMessageRoleTool, true, codersdk.ChatMessageToolResult("summary-1", "chat_summarized", json.RawMessage(`{"source":"agent"}`), false, false)),
			dbMessage(t, 6, database.ChatMessageRoleAssistant, false, codersdk.ChatMessageText("answer after compaction")),
		}
		require.Equal(t, chatloop.CompactionSourceManual, compactionSourceFromHistory(messages))
	})

	t.Run("manual when the compact_context result is an error or absent", func(t *testing.T) {
		t.Parallel()
		messages := []database.ChatMessage{
			dbMessage(t, 1, database.ChatMessageRoleUser, false, codersdk.ChatMessageText("question")),
			dbMessage(t, 2, database.ChatMessageRoleAssistant, false, codersdk.ChatMessageToolCall("compact-1", compactContextToolName, json.RawMessage(`{}`))),
			dbMessage(t, 3, database.ChatMessageRoleTool, false, codersdk.ChatMessageToolResult("compact-1", compactContextToolName, json.RawMessage(`{"error":"blank"}`), true, false)),
		}
		require.Equal(t, chatloop.CompactionSourceManual, compactionSourceFromHistory(messages))
		require.Equal(t, chatloop.CompactionSourceManual, compactionSourceFromHistory(messages[:1]))
	})
}

func TestBuildAgentCompactionFailureMessages(t *testing.T) {
	t.Parallel()

	modelConfigID := uuid.New()
	rows, err := buildAgentCompactionFailureMessages(modelConfigID, "provider returned 400")
	require.NoError(t, err)
	require.Len(t, rows, 2)

	notice := rows[0]
	require.Equal(t, database.ChatMessageRoleSystem, notice.Role)
	require.Equal(t, database.ChatMessageVisibilityUser, notice.Visibility)
	require.False(t, notice.Compressed)
	noticeParts := parseRowParts(t, notice)
	require.Equal(t, codersdk.ChatMessagePartTypeText, noticeParts[0].Type)
	require.Equal(t, "Assistant-requested compaction failed: provider returned 400. Continuing without compaction.", noticeParts[0].Text)

	note := rows[1]
	require.Equal(t, database.ChatMessageRoleUser, note.Role)
	require.Equal(t, database.ChatMessageVisibilityModel, note.Visibility)
	noteText := parseRowParts(t, note)[0].Text
	require.Contains(t, noteText, "compact_context failed (provider returned 400)")
	require.Contains(t, noteText, "Do not call compact_context again in this context segment")

	trailingPeriod, err := buildAgentCompactionFailureMessages(modelConfigID, "The request was canceled before it completed.")
	require.NoError(t, err)
	require.Equal(t, "Assistant-requested compaction failed: The request was canceled before it completed. Continuing without compaction.", parseRowParts(t, trailingPeriod[0])[0].Text)

	blankRows, err := buildAgentCompactionFailureMessages(modelConfigID, "  ")
	require.NoError(t, err)
	require.Contains(t, parseRowParts(t, blankRows[0])[0].Text, "unknown error")
}

func TestContextBoundaryEffectFromStep(t *testing.T) {
	t.Parallel()

	calls := []fantasy.ToolCallContent{
		{ToolCallID: "read-1", ToolName: "read_file", Input: `{"path":"a"}`},
		{ToolCallID: "clear-1", ToolName: clearContextToolName, Input: `{"follow_up":"resume from PLAN.md"}`},
	}

	t.Run("successful result yields effect from matching call args", func(t *testing.T) {
		t.Parallel()
		effect, ok, err := contextBoundaryEffectFromStep(calls, []fantasy.Content{
			fantasy.ToolResultContent{ToolCallID: "read-1", ToolName: "read_file", Result: fantasy.ToolResultOutputContentText{Text: "file"}},
			fantasy.ToolResultContent{ToolCallID: "clear-1", ToolName: clearContextToolName, Result: fantasy.ToolResultOutputContentText{Text: "Context cleared."}},
		})
		require.NoError(t, err)
		require.True(t, ok)
		require.Equal(t, contextBoundaryEffect{tool: clearContextToolName, followUp: "resume from PLAN.md"}, effect)
	})

	t.Run("error result yields no effect", func(t *testing.T) {
		t.Parallel()
		_, ok, err := contextBoundaryEffectFromStep(calls, []fantasy.Content{
			fantasy.ToolResultContent{ToolCallID: "clear-1", ToolName: clearContextToolName, Result: fantasy.ToolResultOutputContentError{Error: stubError("rejected")}},
		})
		require.NoError(t, err)
		require.False(t, ok)
	})

	t.Run("other tools yield no effect", func(t *testing.T) {
		t.Parallel()
		_, ok, err := contextBoundaryEffectFromStep(calls, []fantasy.Content{
			fantasy.ToolResultContent{ToolCallID: "read-1", ToolName: "read_file", Result: fantasy.ToolResultOutputContentText{Text: "file"}},
		})
		require.NoError(t, err)
		require.False(t, ok)
	})

	t.Run("successful result without matching call is an error", func(t *testing.T) {
		t.Parallel()
		_, _, err := contextBoundaryEffectFromStep(nil, []fantasy.Content{
			fantasy.ToolResultContent{ToolCallID: "clear-9", ToolName: clearContextToolName, Result: fantasy.ToolResultOutputContentText{Text: "Context cleared."}},
		})
		require.Error(t, err)
	})

	t.Run("compact_context result yields a compact effect", func(t *testing.T) {
		t.Parallel()
		effect, ok, err := contextBoundaryEffectFromStep(
			[]fantasy.ToolCallContent{{ToolCallID: "compact-1", ToolName: compactContextToolName, Input: `{"follow_up":"resume"}`}},
			[]fantasy.Content{fantasy.ToolResultContent{ToolCallID: "compact-1", ToolName: compactContextToolName, Result: fantasy.ToolResultOutputContentText{Text: "Compaction scheduled."}}},
		)
		require.NoError(t, err)
		require.True(t, ok)
		require.Equal(t, contextBoundaryEffect{tool: compactContextToolName, followUp: "resume"}, effect)
	})
}

type stubError string

func (e stubError) Error() string { return string(e) }

// parseRowParts parses a row to be committed using its own content
// version, so system-role rows written as parts decode correctly.
func parseRowParts(t *testing.T, row chatstate.Message) []codersdk.ChatMessagePart {
	t.Helper()
	parts, err := chatprompt.ParseContent(database.ChatMessage{
		Role:           row.Role,
		Content:        row.Content,
		ContentVersion: row.ContentVersion,
	})
	require.NoError(t, err)
	return parts
}

func TestApplyContextBoundaryEffect_ClearRowOrder(t *testing.T) {
	t.Parallel()

	modelConfigID := uuid.New()
	toolResult, err := modelOnlyUserRow(modelConfigID, "placeholder")
	require.NoError(t, err)
	toolResult.Role = database.ChatMessageRoleTool
	toolResult.Visibility = database.ChatMessageVisibilityBoth
	step := stepMessagesForCommit{Messages: []chatstate.Message{toolResult}}

	committed, err := applyContextBoundaryEffect(step, contextBoundaryEffect{
		tool:     clearContextToolName,
		followUp: "resume from PLAN.md",
	}, []*chathooks.Result{{ModelContext: "hook context", UserMessage: "hook notice"}}, modelConfigID)
	require.NoError(t, err)

	rows := committed.Messages
	require.Len(t, rows, 1+3+2+1)
	require.Equal(t, database.ChatMessageRoleTool, rows[0].Role)
	for _, row := range rows[1:4] {
		require.True(t, row.Compressed, "triplet rows are compressed")
	}
	sentinel := parseMessageParts(t, rows[1].Role, rows[1].Content)
	require.Equal(t, agentClearSentinel, sentinel[0].Text)
	callParts := parseMessageParts(t, rows[2].Role, rows[2].Content)
	require.JSONEq(t, `{"source":"agent"}`, string(callParts[0].Args))

	require.False(t, rows[4].Compressed)
	require.Equal(t, database.ChatMessageRoleUser, rows[4].Role)
	require.Equal(t, database.ChatMessageVisibilityModel, rows[4].Visibility)
	require.Equal(t, "hook context", parseMessageParts(t, rows[4].Role, rows[4].Content)[0].Text)
	require.Equal(t, database.ChatMessageRoleSystem, rows[5].Role)
	require.Equal(t, "hook notice", parseRowParts(t, rows[5])[0].Text)

	followUp := rows[6]
	require.False(t, followUp.Compressed)
	require.Equal(t, database.ChatMessageRoleUser, followUp.Role)
	require.Equal(t, database.ChatMessageVisibilityModel, followUp.Visibility)
	require.Equal(t, "resume from PLAN.md", parseMessageParts(t, followUp.Role, followUp.Content)[0].Text)
}

func TestApplyContextBoundaryEffect_CompactRowOrder(t *testing.T) {
	t.Parallel()

	modelConfigID := uuid.New()
	toolResult, err := modelOnlyUserRow(modelConfigID, "placeholder")
	require.NoError(t, err)
	toolResult.Role = database.ChatMessageRoleTool
	toolResult.Visibility = database.ChatMessageVisibilityBoth
	usage, ok, err := batchUsageMessage(modelConfigID, chatprompt.CurrentContentVersion, 5*time.Millisecond, 1)
	require.NoError(t, err)
	require.True(t, ok)
	step := stepMessagesForCommit{Messages: []chatstate.Message{toolResult, usage}}

	committed, err := applyContextBoundaryEffect(step, contextBoundaryEffect{
		tool:     compactContextToolName,
		followUp: "resume from PLAN.md",
	}, []*chathooks.Result{{ModelContext: "hook context"}}, modelConfigID)
	require.NoError(t, err)
	require.True(t, committed.RequestCompaction)
	require.False(t, committed.ConsumeCompactionRequest)

	rows := committed.Messages
	require.Len(t, rows, 4)
	require.Equal(t, database.ChatMessageRoleTool, rows[0].Role)
	require.Equal(t, database.ChatMessageRoleTool, rows[1].Role, "batch usage row stays before the follow-up")
	require.Equal(t, database.ChatMessageRoleUser, rows[2].Role)
	require.Equal(t, "hook context", parseMessageParts(t, rows[2].Role, rows[2].Content)[0].Text)
	followUp := rows[3]
	require.Equal(t, database.ChatMessageRoleUser, followUp.Role)
	require.Equal(t, database.ChatMessageVisibilityModel, followUp.Visibility)
	require.False(t, followUp.Compressed)
	require.Equal(t, "resume from PLAN.md", parseMessageParts(t, followUp.Role, followUp.Content)[0].Text)
	for _, row := range rows {
		require.False(t, row.Compressed, "a compact request commits no boundary rows")
	}
}

func TestRecordContextToolOutcomes(t *testing.T) {
	t.Parallel()

	results := []fantasy.ToolResultContent{
		{ToolCallID: "clear-1", ToolName: clearContextToolName, Result: fantasy.ToolResultOutputContentText{Text: "ok"}},
		{ToolCallID: "clear-2", ToolName: clearContextToolName, Result: fantasy.ToolResultOutputContentError{Error: stubError("blank")}},
		{ToolCallID: "clear-3", ToolName: clearContextToolName, Result: fantasy.ToolResultOutputContentError{Error: stubError("denied")}},
		{ToolCallID: "read-1", ToolName: "read_file", Result: fantasy.ToolResultOutputContentText{Text: "ignored"}},
	}

	metrics := chatloop.NewMetrics(prometheus.NewPedanticRegistry())
	recordContextToolOutcomes(metrics, results, map[string]bool{"clear-3": true})
	require.Equal(t, 1.0, promtestutil.ToFloat64(metrics.ContextToolCallsTotal.WithLabelValues(clearContextToolName, "success")))
	require.Equal(t, 1.0, promtestutil.ToFloat64(metrics.ContextToolCallsTotal.WithLabelValues(clearContextToolName, "rejected")))
	require.Equal(t, 1.0, promtestutil.ToFloat64(metrics.ContextToolCallsTotal.WithLabelValues(clearContextToolName, "denied")))
	recordContextToolOutcomes(metrics, results[:1], toolCallIDSet(results[:1]))
	require.Equal(t, 2.0, promtestutil.ToFloat64(metrics.ContextToolCallsTotal.WithLabelValues(clearContextToolName, "denied")))
}
