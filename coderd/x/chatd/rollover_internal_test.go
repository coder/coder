package chatd //nolint:testpackage // Exercises unexported rollover helpers.

import (
	"encoding/json"
	"fmt"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/x/chatd/chatprompt"
	"github.com/coder/coder/v2/codersdk"
)

func TestNewContextWindowBudget(t *testing.T) {
	t.Parallel()

	type points struct{ usable, final, handoff int64 }
	tests := []struct {
		window    int64
		threshold int32
		want      points
	}{
		{1_000_000, 30, points{968_000, 952_000, 300_000}},
		{1_000_000, 70, points{968_000, 952_000, 700_000}},
		{1_000_000, 95, points{968_000, 952_000, 950_000}},
		{1_000_000, 100, points{968_000, 952_000, 952_000}},
		{200_000, 30, points{168_000, 152_000, 60_000}},
		{200_000, 70, points{168_000, 152_000, 140_000}},
		{200_000, 95, points{168_000, 152_000, 152_000}},
		{200_000, 100, points{168_000, 152_000, 152_000}},
		{128_000, 30, points{96_000, 80_000, 38_400}},
		{128_000, 70, points{96_000, 80_000, 80_000}},
		{128_000, 95, points{96_000, 80_000, 80_000}},
		{128_000, 100, points{96_000, 80_000, 80_000}},
		// From here the reserve is a quarter of the window and the final
		// zone opens at half the usable limit.
		{32_000, 30, points{24_000, 12_000, 9_600}},
		{32_000, 70, points{24_000, 12_000, 12_000}},
		{32_000, 95, points{24_000, 12_000, 12_000}},
		{32_000, 100, points{24_000, 12_000, 12_000}},
		{8_000, 30, points{6_000, 3_000, 2_400}},
		{8_000, 70, points{6_000, 3_000, 3_000}},
		{8_000, 95, points{6_000, 3_000, 3_000}},
		{8_000, 100, points{6_000, 3_000, 3_000}},
		// The handoff target never drops below 10% of the window, and
		// the final zone rounds half the usable limit up.
		{200_000, 0, points{168_000, 152_000, 20_000}},
		{8_001, 5, points{6_001, 3_001, 800}},
	}
	for _, tc := range tests {
		t.Run(fmt.Sprintf("W%d/T%d", tc.window, tc.threshold), func(t *testing.T) {
			t.Parallel()
			got := newContextWindowBudget(tc.window, 32_000, tc.threshold)
			require.Equal(t, tc.want, points{got.usableLimit, got.finalPoint, got.handoffPoint})
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

	// W = 200k: C = 168k, F = 152k, H = 140k.
	budget := newContextWindowBudget(200_000, 32_000, 70)
	handoff := rolloverDecision{action: rolloverActionHandoff}
	final := rolloverDecision{action: rolloverActionFinal}
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
		{name: "LimitBeforeHandoff", tokens: 168_000, budget: budget, want: limit},
		{name: "HandoffAtTarget", tokens: 140_000, budget: budget, want: handoff},
		{name: "BelowHandoff", tokens: 139_999, budget: budget},
		{name: "NoUsage", budget: budget},
		{name: "UnknownWindow", tokens: 1_000_000, budget: newContextWindowBudget(0, 32_000, 70)},
		{name: "HandoffClaimedBelowFinal", tokens: 151_999, budget: budget, state: rolloverStageState{handoffClaimed: true, newContextOffered: true}},
		{name: "FinalAfterHandoff", tokens: 152_000, budget: budget, state: rolloverStageState{handoffClaimed: true, newContextOffered: true}, want: final},
		{name: "FinalNeedsNewContext", tokens: 152_000, budget: budget, state: rolloverStageState{handoffClaimed: true}},
		{name: "FinalOncePerWindow", tokens: 160_000, budget: budget, state: rolloverStageState{handoffClaimed: true, finalClaimed: true, newContextOffered: true}},
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

	// W = 100 with a 32k output cap: C = 75 and H = F = 38.
	budget := newContextWindowBudget(100, 32_000, 70)
	toolStep := func(t *testing.T, id int64, inputTokens int64) []database.ChatMessage {
		callID := fmt.Sprintf("call-%d", id)
		return []database.ChatMessage{
			withUsage(dbMessage(t, id, database.ChatMessageRoleAssistant, false, codersdk.ChatMessageToolCall(callID, "tool", json.RawMessage(`{}`))), inputTokens, 100),
			dbMessage(t, id+1, database.ChatMessageRoleTool, false, codersdk.ChatMessageToolResult(callID, "tool", json.RawMessage(`{}`), false, false)),
		}
	}
	rolloverBoundary := func(t *testing.T, id int64) []database.ChatMessage {
		anchor := dbMessage(t, id, database.ChatMessageRoleUser, true, codersdk.ChatMessageText("lead-in"))
		anchor.Visibility = database.ChatMessageVisibilityModel
		return []database.ChatMessage{
			anchor,
			dbMessage(t, id+1, database.ChatMessageRoleAssistant, true, codersdk.ChatMessageToolCall("r", rolloverToolName, json.RawMessage(`{}`))),
			dbMessage(t, id+2, database.ChatMessageRoleTool, true, codersdk.ChatMessageToolResult("r", rolloverToolName, json.RawMessage(`{}`), false, false)),
		}
	}
	history := func(rows ...[]database.ChatMessage) []database.ChatMessage {
		var out []database.ChatMessage
		for _, r := range rows {
			out = append(out, r...)
		}
		return out
	}
	prompt := func(t *testing.T, id int64, text string) []database.ChatMessage {
		return []database.ChatMessage{dbMessage(t, id, database.ChatMessageRoleUser, false, codersdk.ChatMessageText(text))}
	}

	tests := []struct {
		name     string
		messages []database.ChatMessage
		budget   *contextWindowBudget
		maxSteps int
		want     generationActionKind
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
				require.Equal(t, rolloverReasonLimit, decision.rolloverReason)
			}
		})
	}
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
			requestItemID: 42,
		})
		require.NoError(t, err)
		require.Len(t, messages, 3)
		parts, err := chatprompt.ParseContent(database.ChatMessage{Content: messages[0].Content, ContentVersion: messages[0].ContentVersion})
		require.NoError(t, err)
		require.Len(t, parts, 1)
		require.Equal(t, "A context window rollover started a fresh context window. Previous window: w:12.\n"+
			"Your previous turn was interrupted by a context rollover; continue the task.\n"+
			"The window reached its usable limit.\n"+
			"Completed tool results remain in the previous window: retrieve them with chat_history rather than re-executing their side effects. The request in progress is item 42.\n"+
			"No checkpoint was saved; recover the task with chat_history, starting with item 42.", parts[0].Text)
	})

	t.Run("ReplaysPendingUserRows", func(t *testing.T) {
		t.Parallel()
		pending := dbMessage(t, 50, database.ChatMessageRoleUser, false, codersdk.ChatMessageText("next"))
		messages, err := buildRolloverMessages(buildRolloverMessagesInput{
			modelConfigID:       modelConfigID,
			toolCallID:          "call",
			payload:             payload,
			requestItemID:       50,
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
