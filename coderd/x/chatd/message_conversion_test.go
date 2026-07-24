package chatd //nolint:testpackage // Uses unexported chatworker helpers.

import (
	"context"
	"database/sql"
	"encoding/json"
	"sync"
	"testing"
	"time"

	"charm.land/fantasy"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/sqlc-dev/pqtype"
	"github.com/stretchr/testify/require"

	"cdr.dev/slog/v3"
	"cdr.dev/slog/v3/sloggers/slogtest"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/x/chatd/chatloop"
	"github.com/coder/coder/v2/coderd/x/chatd/chatprompt"
	"github.com/coder/coder/v2/coderd/x/chatd/messagepartbuffer"
	"github.com/coder/coder/v2/codersdk"
)

func TestBuildCommitStepMessages_AssistantTextAndReasoning(t *testing.T) {
	t.Parallel()

	modelConfigID := uuid.New()
	startedAt := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	completedAt := startedAt.Add(2 * time.Second)
	got, err := buildCommitStepMessages(buildCommitStepMessagesInput{
		modelConfigID:  modelConfigID,
		contentVersion: chatprompt.CurrentContentVersion,
		logger:         slog.Make(),
		step: stepData{
			Content: []fantasy.Content{
				fantasy.ReasoningContent{Text: "thinking"},
				fantasy.TextContent{Text: "hello"},
			},
			ReasoningStartedAt:   []time.Time{startedAt},
			ReasoningCompletedAt: []time.Time{completedAt},
		},
	})
	require.NoError(t, err)
	require.Len(t, got.Messages, 1)
	require.Equal(t, []int{0}, got.VisibleIndexes)

	msg := got.Messages[0]
	require.Equal(t, database.ChatMessageRoleAssistant, msg.Role)
	require.Equal(t, database.ChatMessageVisibilityBoth, msg.Visibility)
	require.Equal(t, uuid.NullUUID{UUID: modelConfigID, Valid: true}, msg.ModelConfigID)
	require.Equal(t, chatprompt.CurrentContentVersion, msg.ContentVersion)
	parts := parseMessageParts(t, msg.Role, msg.Content)
	require.Len(t, parts, 2)
	require.Equal(t, codersdk.ChatMessagePartTypeReasoning, parts[0].Type)
	require.Equal(t, "thinking", parts[0].Text)
	require.Equal(t, startedAt, requireNotNilTime(t, parts[0].CreatedAt))
	require.Equal(t, completedAt, requireNotNilTime(t, parts[0].CompletedAt))
	require.Equal(t, codersdk.ChatMessagePartTypeText, parts[1].Type)
	require.Equal(t, "hello", parts[1].Text)
}

func TestBuildCommitStepMessages_LocalToolResultsBecomeToolMessages(t *testing.T) {
	t.Parallel()

	modelConfigID := uuid.New()
	got, err := buildCommitStepMessages(buildCommitStepMessagesInput{
		modelConfigID:  modelConfigID,
		contentVersion: chatprompt.CurrentContentVersion,
		logger:         slog.Make(),
		step: stepData{Content: []fantasy.Content{
			fantasy.ToolCallContent{ToolCallID: "call-1", ToolName: "execute", Input: `{"cmd":"pwd"}`},
			fantasy.ToolResultContent{
				ToolCallID: "call-1",
				ToolName:   "execute",
				Result:     fantasy.ToolResultOutputContentText{Text: `{"stdout":"/tmp"}`},
			},
		}},
	})
	require.NoError(t, err)
	require.Len(t, got.Messages, 2)
	require.Equal(t, []int{0, 1}, got.VisibleIndexes)

	assistantParts := parseMessageParts(t, got.Messages[0].Role, got.Messages[0].Content)
	require.Len(t, assistantParts, 1)
	require.Equal(t, codersdk.ChatMessagePartTypeToolCall, assistantParts[0].Type)
	require.Equal(t, "call-1", assistantParts[0].ToolCallID)
	require.Equal(t, "execute", assistantParts[0].ToolName)

	toolParts := parseMessageParts(t, got.Messages[1].Role, got.Messages[1].Content)
	require.Len(t, toolParts, 1)
	require.Equal(t, codersdk.ChatMessagePartTypeToolResult, toolParts[0].Type)
	require.Equal(t, "call-1", toolParts[0].ToolCallID)
	require.Equal(t, "execute", toolParts[0].ToolName)
	require.JSONEq(t, `{"stdout":"/tmp"}`, string(toolParts[0].Result))
}

func TestBuildCommitStepMessages_ProviderExecutedResultsStayAssistantContent(t *testing.T) {
	t.Parallel()

	got, err := buildCommitStepMessages(buildCommitStepMessagesInput{
		modelConfigID:  uuid.New(),
		contentVersion: chatprompt.CurrentContentVersion,
		logger:         slog.Make(),
		step: stepData{Content: []fantasy.Content{
			fantasy.ToolCallContent{
				ToolCallID:       "web-1",
				ToolName:         "web_search",
				ProviderExecuted: true,
			},
			fantasy.ToolResultContent{
				ToolCallID:       "web-1",
				ToolName:         "web_search",
				ProviderExecuted: true,
				Result:           fantasy.ToolResultOutputContentText{Text: `{"ok":true}`},
			},
		}},
	})
	require.NoError(t, err)
	require.Len(t, got.Messages, 1)
	parts := parseMessageParts(t, got.Messages[0].Role, got.Messages[0].Content)
	require.Len(t, parts, 2)
	require.Equal(t, codersdk.ChatMessagePartTypeToolCall, parts[0].Type)
	require.True(t, parts[0].ProviderExecuted)
	require.Equal(t, codersdk.ChatMessagePartTypeToolResult, parts[1].Type)
	require.True(t, parts[1].ProviderExecuted)
}

func TestBuildCommitStepMessages_UsageCostRuntime(t *testing.T) {
	t.Parallel()

	inputPrice := decimal.NewFromFloat(2.5)
	outputPrice := decimal.NewFromFloat(7.5)
	got, err := buildCommitStepMessages(buildCommitStepMessagesInput{
		modelConfigID:  uuid.New(),
		contentVersion: chatprompt.CurrentContentVersion,
		logger:         slog.Make(),
		modelCallConfig: codersdk.ChatModelCallConfig{
			Cost: &codersdk.ModelCostConfig{
				InputPricePerMillionTokens:  &inputPrice,
				OutputPricePerMillionTokens: &outputPrice,
			},
		},
		step: stepData{
			Content:      []fantasy.Content{fantasy.TextContent{Text: "usage"}},
			Usage:        fantasy.Usage{InputTokens: 100, OutputTokens: 20, TotalTokens: 120, ReasoningTokens: 3, CacheCreationTokens: 4, CacheReadTokens: 5},
			ContextLimit: sql.NullInt64{Int64: 4096, Valid: true},
			Runtime:      1500 * time.Millisecond,
		},
	})
	require.NoError(t, err)
	require.Len(t, got.Messages, 1)
	msg := got.Messages[0]
	require.Equal(t, sql.NullInt64{Int64: 100, Valid: true}, msg.InputTokens)
	require.Equal(t, sql.NullInt64{Int64: 20, Valid: true}, msg.OutputTokens)
	require.Equal(t, sql.NullInt64{Int64: 120, Valid: true}, msg.TotalTokens)
	require.Equal(t, sql.NullInt64{Int64: 3, Valid: true}, msg.ReasoningTokens)
	require.Equal(t, sql.NullInt64{Int64: 4, Valid: true}, msg.CacheCreationTokens)
	require.Equal(t, sql.NullInt64{Int64: 5, Valid: true}, msg.CacheReadTokens)
	require.Equal(t, sql.NullInt64{Int64: 4096, Valid: true}, msg.ContextLimit)
	require.Equal(t, sql.NullInt64{Int64: 1500, Valid: true}, msg.RuntimeMs)
	require.True(t, msg.TotalCostMicros.Valid)
	require.Greater(t, msg.TotalCostMicros.Int64, int64(0))
}

func TestBuildCommitStepMessages_ToolTimestampsAndMCPConfigIDs(t *testing.T) {
	t.Parallel()

	callAt := time.Date(2026, 2, 3, 4, 5, 6, 0, time.UTC)
	resultAt := callAt.Add(3 * time.Second)
	configID := uuid.New()
	got, err := buildCommitStepMessages(buildCommitStepMessagesInput{
		modelConfigID:  uuid.New(),
		contentVersion: chatprompt.CurrentContentVersion,
		logger:         slog.Make(),
		toolNameToConfigID: map[string]uuid.UUID{
			"mcp_tool": configID,
		},
		step: stepData{Content: []fantasy.Content{
			fantasy.ToolCallContent{ToolCallID: "call-1", ToolName: "mcp_tool", Input: `{}`},
			fantasy.ToolResultContent{ToolCallID: "call-1", ToolName: "mcp_tool", Result: fantasy.ToolResultOutputContentText{Text: `{"ok":true}`}},
		}, ToolCallCreatedAt: map[string]time.Time{
			"call-1": callAt,
		}, ToolResultCreatedAt: map[string]time.Time{
			"call-1": resultAt,
		}},
	})
	require.NoError(t, err)
	require.Len(t, got.Messages, 2)
	callPart := parseMessageParts(t, got.Messages[0].Role, got.Messages[0].Content)[0]
	resultPart := parseMessageParts(t, got.Messages[1].Role, got.Messages[1].Content)[0]
	require.Equal(t, uuid.NullUUID{UUID: configID, Valid: true}, callPart.MCPServerConfigID)
	require.Equal(t, callAt, requireNotNilTime(t, callPart.CreatedAt))
	require.Equal(t, uuid.NullUUID{UUID: configID, Valid: true}, resultPart.MCPServerConfigID)
	require.Equal(t, resultAt, requireNotNilTime(t, resultPart.CreatedAt))
}

func TestBuildCompactionMessages_CompressedSummaryToolCallAndResult(t *testing.T) {
	t.Parallel()

	modelConfigID := uuid.New()
	got, err := buildCompactionMessages(buildCompactionMessagesInput{
		modelConfigID:  modelConfigID,
		contentVersion: chatprompt.CurrentContentVersion,
		toolCallID:     "summary-1",
		toolName:       "chat_summarized",
		compaction: compactionOutcome{
			SystemSummary:    "system summary",
			SummaryReport:    "user report",
			ThresholdPercent: 70,
			UsagePercent:     81.5,
			ContextTokens:    815,
			ContextLimit:     1000,
		},
	})
	require.NoError(t, err)
	require.Equal(t, 1, got.HiddenCount)
	require.Len(t, got.Messages, 3)

	require.Equal(t, database.ChatMessageRoleUser, got.Messages[0].Role)
	require.Equal(t, database.ChatMessageVisibilityModel, got.Messages[0].Visibility)
	require.True(t, got.Messages[0].Compressed)
	require.Equal(t, uuid.NullUUID{UUID: modelConfigID, Valid: true}, got.Messages[0].ModelConfigID)
	require.Equal(t, "system summary", parseMessageParts(t, got.Messages[0].Role, got.Messages[0].Content)[0].Text)

	require.Equal(t, database.ChatMessageRoleAssistant, got.Messages[1].Role)
	require.Equal(t, database.ChatMessageVisibilityUser, got.Messages[1].Visibility)
	require.True(t, got.Messages[1].Compressed)
	callPart := parseMessageParts(t, got.Messages[1].Role, got.Messages[1].Content)[0]
	require.Equal(t, codersdk.ChatMessagePartTypeToolCall, callPart.Type)
	require.Equal(t, "summary-1", callPart.ToolCallID)
	require.JSONEq(t, `{"source":"automatic","threshold_percent":70}`, string(callPart.Args))

	require.Equal(t, database.ChatMessageRoleTool, got.Messages[2].Role)
	require.Equal(t, database.ChatMessageVisibilityBoth, got.Messages[2].Visibility)
	require.True(t, got.Messages[2].Compressed)
	resultPart := parseMessageParts(t, got.Messages[2].Role, got.Messages[2].Content)[0]
	require.Equal(t, codersdk.ChatMessagePartTypeToolResult, resultPart.Type)
	require.Equal(t, "summary-1", resultPart.ToolCallID)
	require.JSONEq(t, `{"summary":"user report","source":"automatic","threshold_percent":70,"usage_percent":81.5,"context_tokens":815,"context_limit_tokens":1000}`, string(resultPart.Result))
}

func TestCurrentTurnStepCount_ExcludesCompressedCompactionMessages(t *testing.T) {
	t.Parallel()

	messages := []database.ChatMessage{
		dbMessage(t, 1, database.ChatMessageRoleUser, false, codersdk.ChatMessageText("start")),
		dbMessage(t, 2, database.ChatMessageRoleAssistant, false, codersdk.ChatMessageText("first")),
		dbMessage(t, 3, database.ChatMessageRoleUser, true, codersdk.ChatMessageText("compressed summary")),
		dbMessage(t, 4, database.ChatMessageRoleAssistant, true, codersdk.ChatMessageToolCall("summary", "chat_summarized", nil)),
		dbMessage(t, 5, database.ChatMessageRoleTool, true, codersdk.ChatMessageToolResult("summary", "chat_summarized", json.RawMessage(`{}`), false, false)),
		dbMessage(t, 6, database.ChatMessageRoleAssistant, false, codersdk.ChatMessageText("second")),
	}
	got := currentTurnStepCount(messages)
	require.Equal(t, 2, got)
}

func TestCurrentTurnStepCount_CountsAssistantMessagesAfterLatestUser(t *testing.T) {
	t.Parallel()

	messages := []database.ChatMessage{
		dbMessage(t, 1, database.ChatMessageRoleUser, false, codersdk.ChatMessageText("old")),
		dbMessage(t, 2, database.ChatMessageRoleAssistant, false, codersdk.ChatMessageText("old answer")),
		dbMessage(t, 3, database.ChatMessageRoleUser, false, codersdk.ChatMessageText("new")),
		dbMessage(t, 4, database.ChatMessageRoleAssistant, false, codersdk.ChatMessageText("one")),
		dbMessage(t, 5, database.ChatMessageRoleTool, false, codersdk.ChatMessageToolResult("call", "tool", json.RawMessage(`{}`), false, false)),
		dbMessage(t, 6, database.ChatMessageRoleAssistant, false, codersdk.ChatMessageText("two")),
	}
	got := currentTurnStepCount(messages)
	require.Equal(t, 2, got)
}

func TestDecisionCompactsAgainAfterPostCompactionTurn(t *testing.T) {
	t.Parallel()

	messages := []database.ChatMessage{
		dbMessage(t, 1, database.ChatMessageRoleUser, false, codersdk.ChatMessageText("initial request")),
		dbMessage(t, 2, database.ChatMessageRoleUser, true, codersdk.ChatMessageText("compacted summary")),
		dbMessage(t, 3, database.ChatMessageRoleAssistant, true, codersdk.ChatMessageToolCall("summary-1", "chat_summarized", nil)),
		dbMessage(t, 4, database.ChatMessageRoleTool, true, codersdk.ChatMessageToolResult("summary-1", "chat_summarized", json.RawMessage(`{}`), false, false)),
		dbMessage(t, 5, database.ChatMessageRoleAssistant, false, codersdk.ChatMessageText("continued after compaction")),
		dbMessage(t, 6, database.ChatMessageRoleUser, false, codersdk.ChatMessageText("next request")),
	}

	decision, err := decideGenerationAction(generationDecisionInput{
		messages:                   messages,
		compactionEnabled:          true,
		compactionNeeded:           true,
		compactionThresholdPercent: 70,
		compactionContextLimit:     100,
	})
	require.NoError(t, err)
	require.Equal(t, generationActionCompact, decision.kind)
}

func TestBuildCompactionMessages_ManualSource(t *testing.T) {
	t.Parallel()

	got, err := buildCompactionMessages(buildCompactionMessagesInput{
		modelConfigID:  uuid.New(),
		contentVersion: chatprompt.CurrentContentVersion,
		toolCallID:     "summary-1",
		toolName:       "chat_summarized",
		compaction: compactionOutcome{
			SystemSummary:    "system summary",
			SummaryReport:    "user report",
			Source:           chatloop.CompactionSourceManual,
			ThresholdPercent: 70,
			UsagePercent:     10,
			ContextTokens:    100,
			ContextLimit:     1000,
		},
	})
	require.NoError(t, err)
	require.Len(t, got.Messages, 3)

	callPart := parseMessageParts(t, got.Messages[1].Role, got.Messages[1].Content)[0]
	require.JSONEq(t, `{"source":"manual","threshold_percent":70}`, string(callPart.Args))
	resultPart := parseMessageParts(t, got.Messages[2].Role, got.Messages[2].Content)[0]
	require.JSONEq(t, `{"summary":"user report","source":"manual","threshold_percent":70,"usage_percent":10,"context_tokens":100,"context_limit_tokens":1000}`, string(resultPart.Result))
}

// TestDecisionForcedCompaction verifies the manual compaction request
// ordering contract: a pending request beats the history-complete
// FinishTurn decision on idle chats, loses to unresolved tool calls,
// and is skipped when nothing after the latest boundary is
// compactable.
func TestDecisionForcedCompaction(t *testing.T) {
	t.Parallel()

	requestedChat := database.Chat{
		CompactionRequestedAt: sql.NullTime{Time: time.Now(), Valid: true},
	}

	t.Run("beats history complete", func(t *testing.T) {
		t.Parallel()

		messages := []database.ChatMessage{
			dbMessage(t, 1, database.ChatMessageRoleUser, false, codersdk.ChatMessageText("question")),
			dbMessage(t, 2, database.ChatMessageRoleAssistant, false, codersdk.ChatMessageText("answer")),
		}
		decision, err := decideGenerationAction(generationDecisionInput{
			chat:     requestedChat,
			messages: messages,
		})
		require.NoError(t, err)
		require.Equal(t, generationActionCompact, decision.kind)
		require.True(t, decision.forced)
	})

	t.Run("loses to unresolved tool calls", func(t *testing.T) {
		t.Parallel()

		messages := []database.ChatMessage{
			dbMessage(t, 1, database.ChatMessageRoleUser, false, codersdk.ChatMessageText("question")),
			dbMessage(t, 2, database.ChatMessageRoleAssistant, false, codersdk.ChatMessageToolCall("read-1", "read_file", json.RawMessage(`{}`))),
		}
		decision, err := decideGenerationAction(generationDecisionInput{
			chat:     requestedChat,
			messages: messages,
		})
		require.NoError(t, err)
		require.Equal(t, generationActionExecuteLocalTools, decision.kind)
		require.False(t, decision.forced)
	})

	t.Run("skipped when nothing compactable", func(t *testing.T) {
		t.Parallel()

		// Everything up to and including the latest boundary is
		// compressed; no uncompressed assistant follows, so the
		// forced compact is skipped and the normal decision applies
		// (after-compaction histories continue with an assistant
		// generation, exactly as if no request were pending).
		messages := []database.ChatMessage{
			dbMessage(t, 1, database.ChatMessageRoleUser, true, codersdk.ChatMessageText("summary")),
			dbMessage(t, 2, database.ChatMessageRoleAssistant, true, codersdk.ChatMessageToolCall("summary-1", "chat_summarized", nil)),
			dbMessage(t, 3, database.ChatMessageRoleTool, true, codersdk.ChatMessageToolResult("summary-1", "chat_summarized", json.RawMessage(`{}`), false, false)),
			dbMessage(t, 4, database.ChatMessageRoleUser, false, codersdk.ChatMessageText("follow-up question")),
			dbMessage(t, 5, database.ChatMessageRoleAssistant, false, codersdk.ChatMessageText("follow-up answer")),
		}
		// Only messages up to the boundary: strip the follow-up.
		requested, err := decideGenerationAction(generationDecisionInput{
			chat:     requestedChat,
			messages: messages[:3],
		})
		require.NoError(t, err)
		unrequested, err := decideGenerationAction(generationDecisionInput{
			chat:     database.Chat{},
			messages: messages[:3],
		})
		require.NoError(t, err)
		require.Equal(t, unrequested.kind, requested.kind,
			"stale request must not change the decision")
		require.False(t, requested.forced)

		// With an uncompressed assistant after the boundary the
		// forced compact fires again.
		decision, err := decideGenerationAction(generationDecisionInput{
			chat:     requestedChat,
			messages: messages,
		})
		require.NoError(t, err)
		require.Equal(t, generationActionCompact, decision.kind)
		require.True(t, decision.forced)
	})

	t.Run("no request follows normal decision", func(t *testing.T) {
		t.Parallel()

		messages := []database.ChatMessage{
			dbMessage(t, 1, database.ChatMessageRoleUser, false, codersdk.ChatMessageText("question")),
			dbMessage(t, 2, database.ChatMessageRoleAssistant, false, codersdk.ChatMessageText("answer")),
		}
		decision, err := decideGenerationAction(generationDecisionInput{
			chat:     database.Chat{},
			messages: messages,
		})
		require.NoError(t, err)
		require.Equal(t, generationActionFinishTurn, decision.kind)
	})
}

func TestCompactionStatusFromHistory(t *testing.T) {
	t.Parallel()

	const thresholdPercent = int32(70)

	t.Run("needed without boundary", func(t *testing.T) {
		t.Parallel()

		messages := []database.ChatMessage{
			dbMessage(t, 1, database.ChatMessageRoleUser, false, codersdk.ChatMessageText("start")),
			dbMessage(t, 2, database.ChatMessageRoleAssistant, false, codersdk.ChatMessageToolCall("read-1", "read_file", json.RawMessage(`{}`))),
		}

		got := compactionStatusFromHistory(messages, compactionRequirementNeeded, thresholdPercent, 100)
		require.Equal(t, compactionStatusNeeded, got)
	})

	t.Run("after compaction without post boundary history", func(t *testing.T) {
		t.Parallel()

		messages := []database.ChatMessage{
			dbMessage(t, 1, database.ChatMessageRoleUser, true, codersdk.ChatMessageText("summary")),
			dbMessage(t, 2, database.ChatMessageRoleAssistant, true, codersdk.ChatMessageToolCall("summary-1", "chat_summarized", nil)),
			dbMessage(t, 3, database.ChatMessageRoleTool, true, codersdk.ChatMessageToolResult("summary-1", "chat_summarized", json.RawMessage(`{}`), false, false)),
		}

		got := compactionStatusFromHistory(messages, compactionRequirementNeeded, thresholdPercent, 100)
		require.Equal(t, compactionStatusAfterCompaction, got)
	})

	t.Run("needed after under limit post compaction assistant", func(t *testing.T) {
		t.Parallel()

		messages := []database.ChatMessage{
			dbMessage(t, 1, database.ChatMessageRoleUser, true, codersdk.ChatMessageText("summary")),
			dbMessage(t, 2, database.ChatMessageRoleAssistant, true, codersdk.ChatMessageToolCall("summary-1", "chat_summarized", nil)),
			dbMessage(t, 3, database.ChatMessageRoleTool, true, codersdk.ChatMessageToolResult("summary-1", "chat_summarized", json.RawMessage(`{}`), false, false)),
			withUsage(dbMessage(t, 4, database.ChatMessageRoleAssistant, false, codersdk.ChatMessageText("continued")), 20, 100),
			dbMessage(t, 5, database.ChatMessageRoleUser, false, codersdk.ChatMessageText("next")),
		}

		got := compactionStatusFromHistory(messages, compactionRequirementNeeded, thresholdPercent, 100)
		require.Equal(t, compactionStatusNeeded, got)
	})

	t.Run("still over limit from first post compaction assistant usage", func(t *testing.T) {
		t.Parallel()

		messages := []database.ChatMessage{
			dbMessage(t, 1, database.ChatMessageRoleUser, true, codersdk.ChatMessageText("summary")),
			dbMessage(t, 2, database.ChatMessageRoleAssistant, true, codersdk.ChatMessageToolCall("summary-1", "chat_summarized", nil)),
			dbMessage(t, 3, database.ChatMessageRoleTool, true, codersdk.ChatMessageToolResult("summary-1", "chat_summarized", json.RawMessage(`{}`), false, false)),
			withUsage(dbMessage(t, 4, database.ChatMessageRoleAssistant, false, codersdk.ChatMessageToolCall("read-1", "read_file", json.RawMessage(`{}`))), 80, 100),
		}

		got := compactionStatusFromHistory(messages, compactionRequirementNeeded, thresholdPercent, 100)
		require.Equal(t, compactionStatusStillOverLimit, got)
	})

	t.Run("still over limit includes prompt cache tokens", func(t *testing.T) {
		t.Parallel()

		messages := []database.ChatMessage{
			dbMessage(t, 1, database.ChatMessageRoleUser, true, codersdk.ChatMessageText("summary")),
			dbMessage(t, 2, database.ChatMessageRoleAssistant, true, codersdk.ChatMessageToolCall("summary-1", "chat_summarized", nil)),
			dbMessage(t, 3, database.ChatMessageRoleTool, true, codersdk.ChatMessageToolResult("summary-1", "chat_summarized", json.RawMessage(`{}`), false, false)),
			withUsageTokens(dbMessage(t, 4, database.ChatMessageRoleAssistant, false, codersdk.ChatMessageToolCall("read-1", "read_file", json.RawMessage(`{}`))), fantasy.Usage{CacheReadTokens: 80}, 100),
		}

		got := compactionStatusFromHistory(messages, compactionRequirementNeeded, thresholdPercent, 100)
		require.Equal(t, compactionStatusStillOverLimit, got)
	})

	t.Run("still over limit uses configured context limit", func(t *testing.T) {
		t.Parallel()

		messages := []database.ChatMessage{
			dbMessage(t, 1, database.ChatMessageRoleUser, true, codersdk.ChatMessageText("summary")),
			dbMessage(t, 2, database.ChatMessageRoleAssistant, true, codersdk.ChatMessageToolCall("summary-1", "chat_summarized", nil)),
			dbMessage(t, 3, database.ChatMessageRoleTool, true, codersdk.ChatMessageToolResult("summary-1", "chat_summarized", json.RawMessage(`{}`), false, false)),
			withUsage(dbMessage(t, 4, database.ChatMessageRoleAssistant, false, codersdk.ChatMessageToolCall("read-1", "read_file", json.RawMessage(`{}`))), 80, 200),
		}

		got := compactionStatusFromHistory(messages, compactionRequirementNeeded, thresholdPercent, 100)
		require.Equal(t, compactionStatusStillOverLimit, got)

		got = compactionStatusFromHistory(messages, compactionRequirementNeeded, thresholdPercent, 200)
		require.Equal(t, compactionStatusNeeded, got)
	})

	t.Run("still over limit includes exact threshold boundary", func(t *testing.T) {
		t.Parallel()

		messages := []database.ChatMessage{
			dbMessage(t, 1, database.ChatMessageRoleUser, true, codersdk.ChatMessageText("summary")),
			dbMessage(t, 2, database.ChatMessageRoleAssistant, true, codersdk.ChatMessageToolCall("summary-1", "chat_summarized", nil)),
			dbMessage(t, 3, database.ChatMessageRoleTool, true, codersdk.ChatMessageToolResult("summary-1", "chat_summarized", json.RawMessage(`{}`), false, false)),
			withUsage(dbMessage(t, 4, database.ChatMessageRoleAssistant, false, codersdk.ChatMessageToolCall("read-1", "read_file", json.RawMessage(`{}`))), 70, 100),
		}

		got := compactionStatusFromHistory(messages, compactionRequirementNeeded, thresholdPercent, 100)
		require.Equal(t, compactionStatusStillOverLimit, got)
	})
}

func TestDecisionDetectsStopAfterToolFromCommittedHistory(t *testing.T) {
	t.Parallel()

	messages := []database.ChatMessage{
		dbMessage(t, 1, database.ChatMessageRoleUser, false, codersdk.ChatMessageText("plan")),
		dbMessage(t, 2, database.ChatMessageRoleAssistant, false, codersdk.ChatMessageToolCall("plan-1", "propose_plan", json.RawMessage(`{}`))),
		dbMessage(t, 3, database.ChatMessageRoleTool, false, codersdk.ChatMessageToolResult("plan-1", "propose_plan", json.RawMessage(`{"ok":true}`), false, false)),
	}
	got, err := historyHasStopAfterToolResult(messages, map[string]struct{}{"propose_plan": {}})
	require.NoError(t, err)
	require.True(t, got)

	messages[2] = dbMessage(t, 3, database.ChatMessageRoleTool, false, codersdk.ChatMessageToolResult("plan-1", "propose_plan", json.RawMessage(`{"error":"no"}`), true, false))
	got, err = historyHasStopAfterToolResult(messages, map[string]struct{}{"propose_plan": {}})
	require.NoError(t, err)
	require.False(t, got)
}

func TestDecisionDetectsCurrentHistoryCompletion(t *testing.T) {
	t.Parallel()

	complete, err := currentHistoryComplete([]database.ChatMessage{
		dbMessage(t, 1, database.ChatMessageRoleUser, false, codersdk.ChatMessageText("hello")),
		dbMessage(t, 2, database.ChatMessageRoleAssistant, false, codersdk.ChatMessageText("done")),
	})
	require.NoError(t, err)
	require.True(t, complete)

	complete, err = currentHistoryComplete([]database.ChatMessage{
		dbMessage(t, 1, database.ChatMessageRoleUser, false, codersdk.ChatMessageText("hello")),
		dbMessage(t, 2, database.ChatMessageRoleAssistant, false, codersdk.ChatMessageToolCall("call-1", "execute", json.RawMessage(`{}`))),
	})
	require.NoError(t, err)
	require.False(t, complete)

	complete, err = currentHistoryComplete([]database.ChatMessage{
		dbMessage(t, 1, database.ChatMessageRoleUser, false, codersdk.ChatMessageText("hello")),
		dbMessage(t, 2, database.ChatMessageRoleAssistant, false, codersdk.ChatMessageToolCall("call-1", "execute", json.RawMessage(`{}`))),
		dbMessage(t, 3, database.ChatMessageRoleTool, false, codersdk.ChatMessageToolResult("call-1", "execute", json.RawMessage(`{"ok":true}`), false, false)),
	})
	require.NoError(t, err)
	require.False(t, complete)
}

func TestBufferedPartsToPartialMessages_NormalizesToolCallDeltasBeforeFinal(t *testing.T) {
	t.Parallel()

	createdAt := time.Date(2026, 3, 4, 5, 6, 7, 0, time.UTC)
	parts := []messagepartbuffer.Part{
		{Seq: 1, Role: codersdk.ChatMessageRoleAssistant, MessagePart: codersdk.ChatMessageText("partial ")},
		{Seq: 2, Role: codersdk.ChatMessageRoleAssistant, MessagePart: codersdk.ChatMessagePart{Type: codersdk.ChatMessagePartTypeToolCall, ToolCallID: "call-1", ToolName: "execute", ArgsDelta: `{"cmd":`}},
		{Seq: 3, Role: codersdk.ChatMessageRoleAssistant, MessagePart: codersdk.ChatMessagePart{Type: codersdk.ChatMessagePartTypeToolCall, ToolCallID: "call-1", ToolName: "execute", ArgsDelta: `"ignored"}`}},
		{Seq: 4, Role: codersdk.ChatMessageRoleAssistant, MessagePart: codersdk.ChatMessageToolCall("call-1", "execute", json.RawMessage(`{"cmd":"pwd"}`))},
	}
	got, err := bufferedPartsToPartialMessages(bufferedPartsToPartialMessagesInput{
		parts:          parts,
		modelConfigID:  uuid.New(),
		contentVersion: chatprompt.CurrentContentVersion,
		logger:         slog.Make(),
		interruptedAt:  createdAt,
	})
	require.NoError(t, err)
	require.Len(t, got, 2)
	assistantParts := parseMessageParts(t, got[0].Role, got[0].Content)
	require.Len(t, assistantParts, 2)
	require.Equal(t, codersdk.ChatMessagePartTypeText, assistantParts[0].Type)
	call := assistantParts[1]
	require.Equal(t, codersdk.ChatMessagePartTypeToolCall, call.Type)
	require.Equal(t, "call-1", call.ToolCallID)
	require.Empty(t, call.ArgsDelta)
	require.JSONEq(t, `{"cmd":"pwd"}`, string(call.Args))
	syntheticParts := parseMessageParts(t, got[1].Role, got[1].Content)
	require.Len(t, syntheticParts, 1)
	require.Equal(t, "call-1", syntheticParts[0].ToolCallID)
}

func TestBufferedPartsToPartialMessages_MergesToolCallDeltasWithoutFinal(t *testing.T) {
	t.Parallel()

	parts := []messagepartbuffer.Part{
		{Seq: 1, Role: codersdk.ChatMessageRoleAssistant, MessagePart: codersdk.ChatMessagePart{Type: codersdk.ChatMessagePartTypeToolCall, ToolCallID: "call-1", ToolName: "execute", ArgsDelta: `{"cmd":`}},
		{Seq: 2, Role: codersdk.ChatMessageRoleAssistant, MessagePart: codersdk.ChatMessagePart{Type: codersdk.ChatMessagePartTypeToolCall, ToolCallID: "call-1", ToolName: "execute", ArgsDelta: `"pwd"}`}},
	}
	got, err := bufferedPartsToPartialMessages(bufferedPartsToPartialMessagesInput{
		parts:          parts,
		modelConfigID:  uuid.New(),
		contentVersion: chatprompt.CurrentContentVersion,
		logger:         slog.Make(),
	})
	require.NoError(t, err)
	require.Len(t, got, 2)
	assistantParts := parseMessageParts(t, got[0].Role, got[0].Content)
	require.Len(t, assistantParts, 1)
	require.Empty(t, assistantParts[0].ArgsDelta)
	require.JSONEq(t, `{"cmd":"pwd"}`, string(assistantParts[0].Args))
	syntheticParts := parseMessageParts(t, got[1].Role, got[1].Content)
	require.Len(t, syntheticParts, 1)
	require.Equal(t, "call-1", syntheticParts[0].ToolCallID)
}

func TestBufferedPartsToPartialMessages_DeltaOnlyToolResultDoesNotAnswer(t *testing.T) {
	t.Parallel()

	logSink := &partialConversionLogSink{}
	logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true}).AppendSinks(logSink)
	parts := []messagepartbuffer.Part{
		{Seq: 1, Role: codersdk.ChatMessageRoleAssistant, MessagePart: codersdk.ChatMessageToolCall("call-1", "advisor", json.RawMessage(`{}`))},
		{Seq: 2, Role: codersdk.ChatMessageRoleTool, MessagePart: codersdk.ChatMessagePart{Type: codersdk.ChatMessagePartTypeToolResult, ToolCallID: "call-1", ToolName: "advisor", ResultDelta: `{"type":"advice"}`}},
	}
	got, err := bufferedPartsToPartialMessages(bufferedPartsToPartialMessagesInput{
		parts:          parts,
		modelConfigID:  uuid.New(),
		contentVersion: chatprompt.CurrentContentVersion,
		logger:         logger,
	})
	require.NoError(t, err)
	require.Len(t, got, 2)
	toolParts := parseMessageParts(t, got[1].Role, got[1].Content)
	require.Len(t, toolParts, 1)
	require.Equal(t, "call-1", toolParts[0].ToolCallID)
	require.True(t, toolParts[0].IsError)
	require.Empty(t, toolParts[0].ResultDelta)
	require.JSONEq(t, `{"error":"tool call was interrupted before it produced a result"}`, string(toolParts[0].Result))
	require.NotEmpty(t, logSink.entriesAtLevelWithMessage(slog.LevelWarn, "skipping buffered chat message part"))
}

func TestBufferedPartsToPartialMessages_LogsMalformedSkippedParts(t *testing.T) {
	t.Parallel()

	logSink := &partialConversionLogSink{}
	logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true}).AppendSinks(logSink)
	parts := []messagepartbuffer.Part{
		{Seq: 1, Role: codersdk.ChatMessageRoleSystem, MessagePart: codersdk.ChatMessageText("bad role")},
		{Seq: 2, Role: codersdk.ChatMessageRoleAssistant, MessagePart: codersdk.ChatMessagePart{}},
		{Seq: 3, Role: codersdk.ChatMessageRoleTool, MessagePart: codersdk.ChatMessagePart{Type: codersdk.ChatMessagePartTypeToolResult, ToolName: "execute", Result: json.RawMessage(`{"ok":true}`)}},
		{Seq: 4, Role: codersdk.ChatMessageRoleAssistant, MessagePart: codersdk.ChatMessagePart{Type: codersdk.ChatMessagePartTypeToolCall, ToolCallID: "bad-args", ToolName: "execute", ArgsDelta: `{"cmd":`}},
	}
	got, err := bufferedPartsToPartialMessages(bufferedPartsToPartialMessagesInput{
		parts:          parts,
		modelConfigID:  uuid.New(),
		contentVersion: chatprompt.CurrentContentVersion,
		logger:         logger,
	})
	require.NoError(t, err)
	require.Empty(t, got)
	require.GreaterOrEqual(t, len(logSink.entriesAtLevelWithMessage(slog.LevelWarn, "skipping buffered chat message part")), 4)
}

func TestBufferedPartsToPartialMessages_SynthesizesMissingToolResults(t *testing.T) {
	t.Parallel()

	modelConfigID := uuid.New()
	createdAt := time.Date(2026, 3, 4, 5, 6, 7, 0, time.UTC)
	reasoningStartedAt := createdAt.Add(-2 * time.Second)
	reasoningPart := codersdk.ChatMessageReasoning("partial thought")
	reasoningPart.CreatedAt = &reasoningStartedAt
	parts := []messagepartbuffer.Part{
		{Seq: 1, Role: codersdk.ChatMessageRoleAssistant, MessagePart: codersdk.ChatMessageText("partial ")},
		{Seq: 2, Role: codersdk.ChatMessageRoleAssistant, MessagePart: reasoningPart},
		{Seq: 3, Role: codersdk.ChatMessageRoleAssistant, MessagePart: codersdk.ChatMessageToolCall("call-1", "execute", json.RawMessage(`{}`))},
		{Seq: 4, Role: codersdk.ChatMessageRoleAssistant, MessagePart: codersdk.ChatMessageToolCall("call-2", "read_file", json.RawMessage(`{}`))},
		{Seq: 5, Role: codersdk.ChatMessageRoleTool, MessagePart: withCreatedAt(codersdk.ChatMessageToolResult("call-2", "read_file", json.RawMessage(`{"ok":true}`), false, false), createdAt)},
	}
	got, err := bufferedPartsToPartialMessages(bufferedPartsToPartialMessagesInput{
		parts:          parts,
		modelConfigID:  modelConfigID,
		contentVersion: chatprompt.CurrentContentVersion,
		logger:         slog.Make(),
		interruptedAt:  createdAt,
	})
	require.NoError(t, err)
	require.Len(t, got, 3)
	require.Equal(t, database.ChatMessageRoleAssistant, got[0].Role)
	assistantParts := parseMessageParts(t, got[0].Role, got[0].Content)
	require.Len(t, assistantParts, 4)
	require.Equal(t, codersdk.ChatMessagePartTypeReasoning, assistantParts[1].Type)
	require.Equal(t, "partial thought", assistantParts[1].Text)
	require.Equal(t, reasoningStartedAt, requireNotNilTime(t, assistantParts[1].CreatedAt))
	require.Equal(t, createdAt, requireNotNilTime(t, assistantParts[1].CompletedAt))
	require.Equal(t, codersdk.ChatMessagePartTypeToolCall, assistantParts[2].Type)
	require.Equal(t, codersdk.ChatMessagePartTypeToolCall, assistantParts[3].Type)

	require.Equal(t, database.ChatMessageRoleTool, got[1].Role)
	toolParts := parseMessageParts(t, got[1].Role, got[1].Content)
	require.Equal(t, "call-2", toolParts[0].ToolCallID)
	require.Equal(t, createdAt, requireNotNilTime(t, toolParts[0].CreatedAt))

	require.Equal(t, database.ChatMessageRoleTool, got[2].Role)
	syntheticParts := parseMessageParts(t, got[2].Role, got[2].Content)
	require.Len(t, syntheticParts, 1)
	require.Equal(t, "call-1", syntheticParts[0].ToolCallID)
	require.Equal(t, "execute", syntheticParts[0].ToolName)
	require.True(t, syntheticParts[0].IsError)
	require.JSONEq(t, `{"error":"tool call was interrupted before it produced a result"}`, string(syntheticParts[0].Result))
	require.Equal(t, createdAt, requireNotNilTime(t, syntheticParts[0].CreatedAt))
	require.Equal(t, uuid.NullUUID{UUID: modelConfigID, Valid: true}, got[2].ModelConfigID)
}

func parseMessageParts(t *testing.T, role database.ChatMessageRole, raw pqtype.NullRawMessage) []codersdk.ChatMessagePart {
	t.Helper()
	parts, err := chatprompt.ParseContent(database.ChatMessage{
		Role:    role,
		Content: raw,
	})
	require.NoError(t, err)
	return parts
}

func dbMessage(t *testing.T, id int64, role database.ChatMessageRole, compressed bool, parts ...codersdk.ChatMessagePart) database.ChatMessage {
	t.Helper()
	raw, err := chatprompt.MarshalParts(parts)
	require.NoError(t, err)
	return database.ChatMessage{
		ID:             id,
		Role:           role,
		Content:        raw,
		ContentVersion: chatprompt.CurrentContentVersion,
		Visibility:     database.ChatMessageVisibilityBoth,
		Compressed:     compressed,
	}
}

func withUsage(msg database.ChatMessage, inputTokens int64, contextLimit int64) database.ChatMessage {
	return withUsageTokens(msg, fantasy.Usage{InputTokens: inputTokens, TotalTokens: inputTokens}, contextLimit)
}

func withUsageTokens(msg database.ChatMessage, usage fantasy.Usage, contextLimit int64) database.ChatMessage {
	msg.InputTokens = sql.NullInt64{Int64: usage.InputTokens, Valid: usage.InputTokens != 0}
	msg.OutputTokens = sql.NullInt64{Int64: usage.OutputTokens, Valid: usage.OutputTokens != 0}
	msg.TotalTokens = sql.NullInt64{Int64: usage.TotalTokens, Valid: usage.TotalTokens != 0}
	msg.ReasoningTokens = sql.NullInt64{Int64: usage.ReasoningTokens, Valid: usage.ReasoningTokens != 0}
	msg.CacheCreationTokens = sql.NullInt64{Int64: usage.CacheCreationTokens, Valid: usage.CacheCreationTokens != 0}
	msg.CacheReadTokens = sql.NullInt64{Int64: usage.CacheReadTokens, Valid: usage.CacheReadTokens != 0}
	msg.ContextLimit = sql.NullInt64{Int64: contextLimit, Valid: contextLimit != 0}
	return msg
}

func requireNotNilTime(t *testing.T, value *time.Time) time.Time {
	t.Helper()
	require.NotNil(t, value)
	return *value
}

func withCreatedAt(part codersdk.ChatMessagePart, createdAt time.Time) codersdk.ChatMessagePart {
	part.CreatedAt = &createdAt
	return part
}

type partialConversionLogSink struct {
	mu      sync.Mutex
	entries []slog.SinkEntry
}

func (s *partialConversionLogSink) LogEntry(_ context.Context, entry slog.SinkEntry) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.entries = append(s.entries, entry)
}

func (*partialConversionLogSink) Sync() {}

func (s *partialConversionLogSink) entriesAtLevelWithMessage(level slog.Level, message string) []slog.SinkEntry {
	s.mu.Lock()
	defer s.mu.Unlock()

	entries := make([]slog.SinkEntry, 0, len(s.entries))
	for _, entry := range s.entries {
		if entry.Level == level && entry.Message == message {
			entries = append(entries, entry)
		}
	}
	return entries
}
