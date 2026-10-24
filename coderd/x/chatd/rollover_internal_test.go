package chatd //nolint:testpackage // Exercises unexported rollover helpers.

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"charm.land/fantasy"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"cdr.dev/slog/v3/sloggers/slogtest"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbmock"
	"github.com/coder/coder/v2/coderd/x/chatd/chatprompt"
	"github.com/coder/coder/v2/coderd/x/chatd/chattool"
	"github.com/coder/coder/v2/codersdk"
)

func TestNewContextWindowBudget(t *testing.T) {
	t.Parallel()

	tests := []struct {
		window int64
		want   int64
	}{
		{1_000_000, 968_000},
		{200_000, 168_000},
		{128_000, 96_000},
		// From here the reserve is a quarter of the window.
		{32_000, 24_000},
		{8_000, 6_000},
	}
	for _, tc := range tests {
		t.Run(fmt.Sprintf("W%d", tc.window), func(t *testing.T) {
			t.Parallel()
			got := newContextWindowBudget(tc.window, 32_000, 70)
			require.Equal(t, tc.want, got.usableLimit)
		})
	}

	t.Run("SmallerOutputCapShrinksReserve", func(t *testing.T) {
		t.Parallel()
		got := newContextWindowBudget(200_000, 8_192, 70)
		require.Equal(t, int64(191_808), got.usableLimit)
	})
	t.Run("UnknownWindowHasNoPoints", func(t *testing.T) {
		t.Parallel()
		got := newContextWindowBudget(0, 32_000, 70)
		require.Equal(t, contextWindowBudget{thresholdPercent: 70}, got)
	})
}

func TestDecideRollover(t *testing.T) {
	t.Parallel()

	// W = 200k: C = 168k.
	budget := newContextWindowBudget(200_000, 32_000, 70)
	limit := rolloverDecision{action: rolloverActionRollover, reason: rolloverReasonLimit}
	model := rolloverDecision{action: rolloverActionRollover, reason: rolloverReasonModel}
	tests := []struct {
		name   string
		tokens int64
		budget contextWindowBudget
		state  rolloverStageState
		want   rolloverDecision
	}{
		{name: "DisabledAtHundredEvenWithReceipt", tokens: 199_000, budget: newContextWindowBudget(200_000, 32_000, 100), state: rolloverStageState{receipt: true}},
		{name: "ReceiptWithUnknownWindow", budget: newContextWindowBudget(0, 32_000, 70), state: rolloverStageState{receipt: true}, want: model},
		{name: "ReceiptBeforeLimit", tokens: 170_000, budget: budget, state: rolloverStageState{receipt: true}, want: model},
		{name: "LimitAtUsableLimit", tokens: 168_000, budget: budget, want: limit},
		{name: "BelowLimit", tokens: 167_999, budget: budget},
		{name: "NoUsage", budget: budget},
		{name: "UnknownWindow", tokens: 1_000_000, budget: newContextWindowBudget(0, 32_000, 70)},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, tc.want, decideRollover(tc.tokens, tc.budget, tc.state))
		})
	}
}

func TestDecideGenerationActionContextRollover(t *testing.T) {
	t.Parallel()

	// W = 100 with a 32k output cap: C = 75.
	budget := newContextWindowBudget(100, 32_000, 70)
	toolStep := rolloverTestToolStep
	rolloverBoundary := func(t *testing.T, id int64) []database.ChatMessage {
		return rolloverTestBoundary(t, id, rolloverToolName)
	}
	history := rolloverTestHistory
	prompt := rolloverTestPrompt
	scheduled := json.RawMessage(`{"status":"scheduled","message":"A new context window will start without summarizing conversation history."}`)
	// checkpointStep calls save_checkpoint and new_context in one step and
	// records the given results.
	checkpointStep := func(t *testing.T, id int64, results ...codersdk.ChatMessagePart) []database.ChatMessage {
		rows := []database.ChatMessage{withUsage(dbMessage(t, id, database.ChatMessageRoleAssistant, false,
			codersdk.ChatMessageToolCall("save", chattool.SaveCheckpointToolName, json.RawMessage(`{"checkpoint":"goal"}`)),
			codersdk.ChatMessageToolCall("new", chattool.NewContextToolName, json.RawMessage(`{}`)),
		), 10, 100)}
		for i, result := range results {
			rows = append(rows, dbMessage(t, id+1+int64(i), database.ChatMessageRoleTool, false, result))
		}
		return rows
	}
	saveResult := codersdk.ChatMessageToolResult("save", chattool.SaveCheckpointToolName, json.RawMessage(`{"saved":true,"bytes":4}`), false, false)
	newContextResult := codersdk.ChatMessageToolResult("new", chattool.NewContextToolName, scheduled, false, false)
	unknownWindow := newContextWindowBudget(0, 32_000, 70)

	tests := []struct {
		name     string
		messages []database.ChatMessage
		budget   *contextWindowBudget
		maxSteps int
		want     generationActionKind
		// wantReason defaults to the usable-limit reason.
		wantReason rolloverReason
	}{
		{name: "RollsOverAtUsableLimit", messages: history(prompt(t, 1, "go"), toolStep(t, 2, 75)), budget: &budget, want: generationActionContextRollover},
		{name: "GeneratesBelowUsableLimit", messages: history(prompt(t, 1, "go"), toolStep(t, 2, 74)), budget: &budget, want: generationActionGenerateAssistant},
		{name: "RollsOverBeforeAnsweringNewPrompt", messages: history(
			prompt(t, 1, "go"),
			[]database.ChatMessage{withUsage(dbMessage(t, 2, database.ChatMessageRoleAssistant, false, codersdk.ChatMessageText("done")), 80, 100)},
			prompt(t, 3, "next"),
		), budget: &budget, want: generationActionContextRollover},
		{name: "IgnoresUsageBeforeRolloverBoundary", messages: history(prompt(t, 1, "go"), toolStep(t, 2, 80), rolloverBoundary(t, 4)), budget: &budget, want: generationActionGenerateAssistant},
		{name: "RollsOverWhenNewWindowFills", messages: history(prompt(t, 1, "go"), toolStep(t, 2, 80), rolloverBoundary(t, 4), toolStep(t, 7, 80)), budget: &budget, want: generationActionContextRollover},
		{name: "DisabledAtHundredPercent", messages: history(prompt(t, 1, "go"), toolStep(t, 2, 99)), budget: new(newContextWindowBudget(100, 32_000, 100)), want: generationActionGenerateAssistant},
		{name: "SummarizeStrategyIgnoresLimit", messages: history(prompt(t, 1, "go"), toolStep(t, 2, 99)), want: generationActionGenerateAssistant},
		{name: "NewContextReceiptWithUnknownWindow", messages: history(prompt(t, 1, "go"), checkpointStep(t, 2, saveResult, newContextResult)), budget: &unknownWindow, want: generationActionContextRollover, wantReason: rolloverReasonModel},
		{name: "SiblingToolsSettleBeforeReceipt", messages: history(prompt(t, 1, "go"), checkpointStep(t, 2, newContextResult)), budget: &budget, want: generationActionExecuteLocalTools},
		{name: "ReceiptHonoredOncePerWindow", messages: history(prompt(t, 1, "go"), checkpointStep(t, 2, saveResult, newContextResult), rolloverBoundary(t, 5)), budget: &budget, want: generationActionGenerateAssistant},
		{name: "RefusedNewContextIsNoReceipt", messages: history(prompt(t, 1, "go"), checkpointStep(t, 2, saveResult,
			codersdk.ChatMessageToolResult("new", chattool.NewContextToolName, json.RawMessage(`{"code":"rollover_disabled","error":"disabled"}`), false, false),
		)), budget: &budget, want: generationActionGenerateAssistant},
		{name: "FailedNewContextIsNoReceipt", messages: history(prompt(t, 1, "go"), checkpointStep(t, 2, saveResult,
			codersdk.ChatMessageToolResult("new", chattool.NewContextToolName, scheduled, true, false),
		)), budget: &budget, want: generationActionGenerateAssistant},
		// A dynamic tool of the same name may have returned any JSON value.
		{name: "LegacyDynamicResultIsNoReceipt", messages: history(prompt(t, 1, "go"), checkpointStep(t, 2, saveResult,
			codersdk.ChatMessageToolResult("new", chattool.NewContextToolName, json.RawMessage(`"ok"`), false, false),
		)), budget: &budget, want: generationActionGenerateAssistant},
		{name: "MaxStepsFinishesFirst", messages: history(prompt(t, 1, "go"), toolStep(t, 2, 80)), budget: &budget, maxSteps: 1, want: generationActionFinishTurn},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			maxSteps := tc.maxSteps
			if maxSteps == 0 {
				maxSteps = codersdk.DefaultChatMaxStepsPerTurn
			}
			decision, err := decideGenerationAction(generationDecisionInput{
				messages: tc.messages,
				maxSteps: maxSteps,
				rollover: tc.budget,
			})
			require.NoError(t, err)
			require.Equal(t, tc.want, decision.kind)
			if tc.want == generationActionContextRollover {
				wantReason := tc.wantReason
				if wantReason == "" {
					wantReason = rolloverReasonLimit
				}
				require.Equal(t, wantReason, decision.rolloverReason)
			}
		})
	}
}

func rolloverTestToolStep(t *testing.T, id int64, inputTokens int64) []database.ChatMessage {
	callID := fmt.Sprintf("call-%d", id)
	return []database.ChatMessage{
		withUsage(dbMessage(t, id, database.ChatMessageRoleAssistant, false, codersdk.ChatMessageToolCall(callID, "tool", json.RawMessage(`{}`))), inputTokens, 100),
		dbMessage(t, id+1, database.ChatMessageRoleTool, false, codersdk.ChatMessageToolResult(callID, "tool", json.RawMessage(`{}`), false, false)),
	}
}

// rolloverTestBoundary returns a context boundary triplet of the given
// kind: chat_summarized, chat_cleared, or chat_context_rollover.
func rolloverTestBoundary(t *testing.T, id int64, toolName string) []database.ChatMessage {
	anchor := dbMessage(t, id, database.ChatMessageRoleUser, true, codersdk.ChatMessageText("anchor"))
	anchor.Visibility = database.ChatMessageVisibilityModel
	call := dbMessage(t, id+1, database.ChatMessageRoleAssistant, true, codersdk.ChatMessageToolCall("boundary", toolName, json.RawMessage(`{}`)))
	call.Visibility = database.ChatMessageVisibilityUser
	return []database.ChatMessage{
		anchor,
		call,
		dbMessage(t, id+2, database.ChatMessageRoleTool, true, codersdk.ChatMessageToolResult("boundary", toolName, json.RawMessage(`{}`), false, false)),
	}
}

func rolloverTestHistory(rows ...[]database.ChatMessage) []database.ChatMessage {
	var out []database.ChatMessage
	for _, r := range rows {
		out = append(out, r...)
	}
	return out
}

func rolloverTestPrompt(t *testing.T, id int64, text string) []database.ChatMessage {
	return []database.ChatMessage{dbMessage(t, id, database.ChatMessageRoleUser, false, codersdk.ChatMessageText(text))}
}

func TestContextWindowID(t *testing.T) {
	t.Parallel()

	first := dbMessage(t, 1, database.ChatMessageRoleUser, false, codersdk.ChatMessageText("go"))
	require.Equal(t, "w:0", contextWindowID([]database.ChatMessage{first}))

	anchor := dbMessage(t, 10, database.ChatMessageRoleUser, true, codersdk.ChatMessageText("anchor"))
	anchor.Visibility = database.ChatMessageVisibilityModel
	replay := dbMessage(t, 13, database.ChatMessageRoleUser, false, codersdk.ChatMessageText("pending"))
	replay.Visibility = database.ChatMessageVisibilityModel
	next := dbMessage(t, 14, database.ChatMessageRoleAssistant, false, codersdk.ChatMessageText("answer"))
	require.Equal(t, "w:10", contextWindowID([]database.ChatMessage{anchor, replay, next}))
}

func TestContextWindowToolsAllowedInRestrictedModes(t *testing.T) {
	t.Parallel()

	names := []string{chattool.SaveCheckpointToolName, chattool.NewContextToolName, chattool.ChatHistoryToolName}
	tools := make([]fantasy.AgentTool, 0, len(names))
	for _, name := range names {
		require.True(t, builtinPlanToolAllowed(name, true), name)
		require.True(t, builtinPlanToolAllowed(name, false), "%s is offered to child chats too", name)
		tools = append(tools, newTestAgentTool(name))
	}
	require.Equal(t, names, allowedExploreToolNames(tools))
}

func TestBuildRolloverMessages(t *testing.T) {
	t.Parallel()

	modelConfigID := uuid.New()
	payload := contextRolloverPayload{
		Reason:           rolloverReasonLimit,
		RequestedBy:      rolloverRequestedBySystem,
		PreviousWindowID: "w:12",
		ContextTokens:    80,
		BudgetTokens:     75,
	}

	t.Run("MidTurn", func(t *testing.T) {
		t.Parallel()
		messages, err := buildRolloverMessages(buildRolloverMessagesInput{
			modelConfigID: modelConfigID,
			toolCallID:    "call",
			payload:       payload,
		})
		require.NoError(t, err)
		require.Len(t, messages, 3)
		parts, err := chatprompt.ParseContent(database.ChatMessage{Content: messages[0].Content, ContentVersion: messages[0].ContentVersion})
		require.NoError(t, err)
		require.Len(t, parts, 1)
		require.Equal(t, "A context window rollover started a fresh context window. Previous window: w:12.\n"+
			"Your previous turn was interrupted by a context rollover; continue the task.\n"+
			"The window reached its usable limit.\n"+
			"No checkpoint was saved.", parts[0].Text)
	})

	t.Run("ReplaysPendingUserRows", func(t *testing.T) {
		t.Parallel()
		pending := dbMessage(t, 50, database.ChatMessageRoleUser, false, codersdk.ChatMessageText("next"))
		messages, err := buildRolloverMessages(buildRolloverMessagesInput{
			modelConfigID:       modelConfigID,
			toolCallID:          "call",
			payload:             payload,
			pendingUserMessages: []database.ChatMessage{pending},
		})
		require.NoError(t, err)
		require.Len(t, messages, 4)
		for _, msg := range messages[:3] {
			require.True(t, msg.Compressed)
		}
		replay := messages[3]
		require.False(t, replay.Compressed)
		require.Equal(t, database.ChatMessageRoleUser, replay.Role)
		require.Equal(t, database.ChatMessageVisibilityModel, replay.Visibility)
		require.Equal(t, pending.Content, replay.Content)
		parts, err := chatprompt.ParseContent(database.ChatMessage{Content: messages[0].Content, ContentVersion: messages[0].ContentVersion})
		require.NoError(t, err)
		require.NotContains(t, parts[0].Text, "interrupted", "a rollover before answering a new prompt interrupted nothing")
	})
}

func TestLatestChatCheckpoint(t *testing.T) {
	t.Parallel()

	oversized, err := json.Marshal(map[string]string{"checkpoint": strings.Repeat("a", chattool.MaxCheckpointBytes+1)})
	require.NoError(t, err)
	tests := []struct {
		name string
		args json.RawMessage
		want string
	}{
		{name: "Saved", args: json.RawMessage(`{"checkpoint":"goal"}`), want: "goal"},
		// A dynamic tool of the same name may have taken other arguments.
		{name: "IncompatibleLegacyArgs", args: json.RawMessage(`{"checkpoint":123}`)},
		{name: "OversizedLegacyArgs", args: oversized},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			db := dbmock.NewMockStore(gomock.NewController(t))
			db.EXPECT().GetLatestSuccessfulChatToolCallArgs(gomock.Any(), gomock.Any()).Return(tc.args, nil)
			server := &Server{db: db, logger: slogtest.Make(t, &slogtest.Options{IgnoreErrors: true})}
			got, err := server.latestChatCheckpoint(t.Context(), uuid.New())
			require.NoError(t, err)
			require.Equal(t, tc.want, got)
		})
	}
}
