package chatd //nolint:testpackage // Exercises unexported rollover helpers.

import (
	"encoding/json"
	"fmt"
	"slices"
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
	"github.com/coder/coder/v2/coderd/x/chatd/chatstate"
	"github.com/coder/coder/v2/coderd/x/chatd/chattool"
	"github.com/coder/coder/v2/codersdk"
)

func TestNewContextWindowBudget(t *testing.T) {
	t.Parallel()

	type points struct{ usable, handoff int64 }
	tests := []struct {
		window    int64
		threshold int32
		want      points
	}{
		{1_000_000, 30, points{968_000, 300_000}},
		{1_000_000, 70, points{968_000, 700_000}},
		{1_000_000, 95, points{968_000, 950_000}},
		{1_000_000, 100, points{968_000, 968_000}},
		{200_000, 30, points{168_000, 60_000}},
		{200_000, 70, points{168_000, 140_000}},
		{200_000, 95, points{168_000, 168_000}},
		{200_000, 100, points{168_000, 168_000}},
		{128_000, 30, points{96_000, 38_400}},
		{128_000, 70, points{96_000, 89_600}},
		{128_000, 95, points{96_000, 96_000}},
		{128_000, 100, points{96_000, 96_000}},
		// From here the reserve is a quarter of the window.
		{32_000, 30, points{24_000, 9_600}},
		{32_000, 70, points{24_000, 22_400}},
		{32_000, 95, points{24_000, 24_000}},
		{32_000, 100, points{24_000, 24_000}},
		{8_000, 30, points{6_000, 2_400}},
		{8_000, 70, points{6_000, 5_600}},
		{8_000, 95, points{6_000, 6_000}},
		{8_000, 100, points{6_000, 6_000}},
		// The handoff target never drops below 10% of the window.
		{200_000, 0, points{168_000, 20_000}},
		{8_001, 5, points{6_001, 800}},
	}
	for _, tc := range tests {
		t.Run(fmt.Sprintf("W%d/T%d", tc.window, tc.threshold), func(t *testing.T) {
			t.Parallel()
			got := newContextWindowBudget(tc.window, 32_000, tc.threshold)
			require.Equal(t, tc.want, points{got.usableLimit, got.handoffPoint})
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

	// W = 200k: C = 168k, H = 140k.
	budget := newContextWindowBudget(200_000, 32_000, 70)
	handoff := rolloverDecision{action: rolloverActionHandoff}
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
		{name: "HandoffOncePerWindow", tokens: 167_999, budget: budget, state: rolloverStageState{handoffClaimed: true}},
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

	// W = 100 with a 32k output cap: C = 75 and H = 70.
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
		{name: "GeneratesBelowHandoffTarget", messages: history(prompt(t, 1, "go"), toolStep(t, 2, 37)), budget: &budget, want: generationActionGenerateAssistant},
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

func TestDecideGenerationActionContextHandoff(t *testing.T) {
	t.Parallel()

	// W = 100 at 20% with a 32k output cap: H = 20 and C = 75.
	budget := newContextWindowBudget(100, 32_000, 20)
	toolStep, history, prompt := rolloverTestToolStep, rolloverTestHistory, rolloverTestPrompt
	handoffRows := func(t *testing.T, id int64, stage rolloverAction) []database.ChatMessage {
		messages, err := buildContextHandoffMessages(buildContextHandoffMessagesInput{
			toolCallID:    fmt.Sprintf("handoff-%d", id),
			stage:         stage,
			contextTokens: 25,
			usableLimit:   budget.usableLimit,
		})
		require.NoError(t, err)
		rows := make([]database.ChatMessage, 0, len(messages))
		for i, msg := range messages {
			rows = append(rows, database.ChatMessage{
				ID:             id + int64(i),
				Role:           msg.Role,
				Content:        msg.Content,
				ContentVersion: msg.ContentVersion,
				Visibility:     msg.Visibility,
				Compressed:     msg.Compressed,
			})
		}
		return rows
	}
	answer := func(t *testing.T, id int64, inputTokens int64) []database.ChatMessage {
		return []database.ChatMessage{withUsage(dbMessage(t, id, database.ChatMessageRoleAssistant, false, codersdk.ChatMessageText("done")), inputTokens, 100)}
	}

	tests := []struct {
		name      string
		messages  []database.ChatMessage
		budget    contextWindowBudget
		maxSteps  int
		want      generationActionKind
		wantStage rolloverAction
	}{
		{name: "HandoffAtTarget", messages: history(prompt(t, 1, "go"), toolStep(t, 2, 20)), want: generationActionContextHandoff, wantStage: rolloverActionHandoff},
		{name: "HandoffBeforeAnsweringNewPrompt", messages: history(prompt(t, 1, "go"), answer(t, 2, 25), prompt(t, 3, "next")), want: generationActionContextHandoff, wantStage: rolloverActionHandoff},
		{name: "HandoffOncePerWindow", messages: history(prompt(t, 1, "go"), toolStep(t, 2, 20), handoffRows(t, 4, rolloverActionHandoff), toolStep(t, 7, 37)), want: generationActionGenerateAssistant},
		{name: "ClaimsResetAfterRollover", messages: history(prompt(t, 1, "go"), toolStep(t, 2, 20), handoffRows(t, 4, rolloverActionHandoff), rolloverTestBoundary(t, 7, rolloverToolName), toolStep(t, 10, 25)), want: generationActionContextHandoff, wantStage: rolloverActionHandoff},
		{name: "ClaimsResetAfterSummary", messages: history(prompt(t, 1, "go"), toolStep(t, 2, 20), handoffRows(t, 4, rolloverActionHandoff), rolloverTestBoundary(t, 7, "chat_summarized"), toolStep(t, 10, 25)), want: generationActionContextHandoff, wantStage: rolloverActionHandoff},
		{name: "ClaimsResetAfterClear", messages: history(prompt(t, 1, "go"), toolStep(t, 2, 20), handoffRows(t, 4, rolloverActionHandoff), rolloverTestBoundary(t, 7, "chat_cleared"), toolStep(t, 10, 25)), want: generationActionContextHandoff, wantStage: rolloverActionHandoff},
		{name: "DisabledAtHundredPercent", messages: history(prompt(t, 1, "go"), toolStep(t, 2, 50)), budget: newContextWindowBudget(100, 32_000, 100), want: generationActionGenerateAssistant},
		// Handoff rows add a compressed assistant call and a model-only
		// user prompt; neither is a step of the turn nor a new turn.
		{name: "HandoffRowsAreNotSteps", messages: history(prompt(t, 1, "go"), toolStep(t, 2, 20), handoffRows(t, 4, rolloverActionHandoff)), maxSteps: 2, want: generationActionGenerateAssistant},
		{name: "HandoffPromptDoesNotStartTurn", messages: history(prompt(t, 1, "go"), toolStep(t, 2, 20), handoffRows(t, 4, rolloverActionHandoff)), maxSteps: 1, want: generationActionFinishTurn},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			maxSteps := tc.maxSteps
			if maxSteps == 0 {
				maxSteps = codersdk.DefaultChatMaxStepsPerTurn
			}
			caseBudget := tc.budget
			if caseBudget == (contextWindowBudget{}) {
				caseBudget = budget
			}
			decision, err := decideGenerationAction(generationDecisionInput{
				messages: tc.messages,
				maxSteps: maxSteps,
				rollover: &caseBudget,
			})
			require.NoError(t, err)
			require.Equal(t, tc.want, decision.kind)
			require.Equal(t, tc.wantStage, decision.handoffStage)
		})
	}
}

func TestBuildContextHandoffMessages(t *testing.T) {
	t.Parallel()

	modelConfigID := uuid.New()
	usage := "Context budget ~33% used (25 of 75 tokens before Coder forces a rollover at the usable limit). "
	tests := []struct {
		stage      rolloverAction
		wantPrompt string
	}{
		{
			stage: rolloverActionHandoff,
			wantPrompt: usage + "The context handoff target has been reached. Finish the current small unit of work and start no substantial new work in this window. " +
				"Before starting a new context window, save a concise checkpoint with save_checkpoint: the goal, decisions, progress, learnings, next steps, and the window ID and item ID of every relevant user request still being solved, as well as important actions and tool calls. " +
				"Future context windows will not include the current conversation. After saving your checkpoint, call new_context in a later step to continue in a fresh context window. " +
				"If the task is already complete, finish the reply instead.",
		},
	}
	for _, tc := range tests {
		t.Run(string(tc.stage), func(t *testing.T) {
			t.Parallel()
			messages, err := buildContextHandoffMessages(buildContextHandoffMessagesInput{
				modelConfigID: modelConfigID,
				toolCallID:    "call",
				stage:         tc.stage,
				contextTokens: 25,
				usableLimit:   75,
			})
			require.NoError(t, err)
			require.Len(t, messages, 3)
			parse := func(msg chatstate.Message) codersdk.ChatMessagePart {
				parts, err := chatprompt.ParseContent(database.ChatMessage{Content: msg.Content, ContentVersion: msg.ContentVersion})
				require.NoError(t, err)
				require.Len(t, parts, 1)
				return parts[0]
			}

			call, result, prompt := messages[0], messages[1], messages[2]
			require.Equal(t, []any{database.ChatMessageRoleAssistant, database.ChatMessageVisibilityUser, true}, []any{call.Role, call.Visibility, call.Compressed})
			callPart := parse(call)
			require.Equal(t, codersdk.ChatMessagePartTypeToolCall, callPart.Type)
			require.Equal(t, "chat_context_handoff", callPart.ToolName)
			require.JSONEq(t, fmt.Sprintf(`{"stage":%q,"context_tokens":25,"budget_tokens":75}`, tc.stage), string(callPart.Args))

			require.Equal(t, []any{database.ChatMessageRoleTool, database.ChatMessageVisibilityUser, true}, []any{result.Role, result.Visibility, result.Compressed})
			resultPart := parse(result)
			require.Equal(t, codersdk.ChatMessagePartTypeToolResult, resultPart.Type)
			require.Equal(t, callPart.ToolCallID, resultPart.ToolCallID)
			wantResult, err := json.Marshal(map[string]string{"stage": string(tc.stage), "prompt": tc.wantPrompt})
			require.NoError(t, err)
			require.JSONEq(t, string(wantResult), string(resultPart.Result))

			require.Equal(t, []any{database.ChatMessageRoleUser, database.ChatMessageVisibilityModel, false}, []any{prompt.Role, prompt.Visibility, prompt.Compressed})
			require.Equal(t, fmt.Sprintf(`<context-handoff stage=%q>%s</context-handoff>`, tc.stage, tc.wantPrompt), parse(prompt).Text)
		})
	}
}

func TestReplayablePendingUserRows(t *testing.T) {
	t.Parallel()

	messages, err := buildContextHandoffMessages(buildContextHandoffMessagesInput{
		toolCallID:    "call",
		stage:         rolloverActionHandoff,
		contextTokens: 25,
		usableLimit:   75,
	})
	require.NoError(t, err)
	handoffPrompt := database.ChatMessage{
		ID:             5,
		Role:           messages[2].Role,
		Content:        messages[2].Content,
		ContentVersion: messages[2].ContentVersion,
		Visibility:     messages[2].Visibility,
	}
	request := dbMessage(t, 2, database.ChatMessageRoleUser, false, codersdk.ChatMessageText("next"))

	rows, err := replayablePendingUserRows([]database.ChatMessage{request, handoffPrompt})
	require.NoError(t, err)
	require.Equal(t, []database.ChatMessage{request}, rows, "a stale handoff prompt must not reach the next window")

	rows, err = replayablePendingUserRows([]database.ChatMessage{handoffPrompt})
	require.NoError(t, err)
	require.Empty(t, rows, "a handoff prompt alone is no pending user request")
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

func TestPreviousContextWindowID(t *testing.T) {
	t.Parallel()

	anchorRow := func(msg chatstate.Message) database.ChatMessage {
		return database.ChatMessage{ID: 20, Role: msg.Role, Visibility: msg.Visibility, Compressed: msg.Compressed, Content: msg.Content, ContentVersion: msg.ContentVersion}
	}
	rollover, err := buildRolloverMessages(buildRolloverMessagesInput{
		toolCallID: "call",
		payload:    contextRolloverPayload{Reason: rolloverReasonModel, PreviousWindowID: "w:12"},
	})
	require.NoError(t, err)
	previous, err := previousContextWindowID([]database.ChatMessage{anchorRow(rollover[0])})
	require.NoError(t, err)
	require.Equal(t, "w:12", previous)

	summary := dbMessage(t, 20, database.ChatMessageRoleUser, true, codersdk.ChatMessageText("summary of the earlier conversation"))
	summary.Visibility = database.ChatMessageVisibilityModel
	previous, err = previousContextWindowID([]database.ChatMessage{summary})
	require.NoError(t, err)
	require.Empty(t, previous, "only a rollover names the window it left")
}

func TestWithoutChatHistoryLeadIn(t *testing.T) {
	t.Parallel()

	anchor := func(t *testing.T, checkpoint string) database.ChatMessage {
		messages, err := buildRolloverMessages(buildRolloverMessagesInput{
			toolCallID:    "call",
			payload:       contextRolloverPayload{Reason: rolloverReasonLimit, PreviousWindowID: "w:0", Checkpoint: checkpoint},
			requestItemID: 7,
		})
		require.NoError(t, err)
		return database.ChatMessage{ID: 10, Role: messages[0].Role, Visibility: messages[0].Visibility, Compressed: messages[0].Compressed, Content: messages[0].Content, ContentVersion: messages[0].ContentVersion}
	}
	text := func(t *testing.T, msg database.ChatMessage) string {
		parts, err := chatprompt.ParseContent(msg)
		require.NoError(t, err)
		require.Len(t, parts, 1)
		return parts[0].Text
	}

	t.Run("NoCheckpoint", func(t *testing.T) {
		t.Parallel()
		rows := []database.ChatMessage{anchor(t, "")}
		original := slices.Clone(rows)
		got, err := withoutChatHistoryLeadIn(rows)
		require.NoError(t, err)
		require.Equal(t, original, rows, "the input rows are persisted elsewhere and must stay unchanged")
		leadIn := text(t, got[0])
		require.NotContains(t, leadIn, "chat_history")
		require.Contains(t, leadIn, "No checkpoint was saved.")
		require.Contains(t, leadIn, "The window reached its usable limit.")
	})
	t.Run("CheckpointIsVerbatim", func(t *testing.T) {
		t.Parallel()
		checkpoint := rolloverRecoveryLine + "\nNo checkpoint was saved; this line is the agent's own text."
		got, err := withoutChatHistoryLeadIn([]database.ChatMessage{anchor(t, checkpoint)})
		require.NoError(t, err)
		leadIn := text(t, got[0])
		require.Contains(t, leadIn, "<checkpoint>\n"+checkpoint+"\n</checkpoint>")
		require.Equal(t, 1, strings.Count(leadIn, "chat_history"), "only the checkpoint keeps its chat_history mention")
	})
	t.Run("OtherAnchorsUnchanged", func(t *testing.T) {
		t.Parallel()
		summary := dbMessage(t, 10, database.ChatMessageRoleUser, true, codersdk.ChatMessageText("Summary: "+rolloverRecoveryLine))
		summary.Visibility = database.ChatMessageVisibilityModel
		got, err := withoutChatHistoryLeadIn([]database.ChatMessage{summary})
		require.NoError(t, err)
		require.Equal(t, []database.ChatMessage{summary}, got)
	})
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

func TestTagUserRowsWithItemIDs(t *testing.T) {
	t.Parallel()

	request := dbMessage(t, 1, database.ChatMessageRoleUser, false, codersdk.ChatMessageText("first"), codersdk.ChatMessageText("second"))
	fileOnly := dbMessage(t, 2, database.ChatMessageRoleUser, false, codersdk.ChatMessageFile(uuid.New(), "image/png", "screenshot.png"))
	answer := dbMessage(t, 3, database.ChatMessageRoleAssistant, false, codersdk.ChatMessageText("answer"))
	anchor := dbMessage(t, 4, database.ChatMessageRoleUser, true, codersdk.ChatMessageText("lead-in"))
	anchor.Visibility = database.ChatMessageVisibilityModel
	replay := dbMessage(t, 5, database.ChatMessageRoleUser, false, codersdk.ChatMessageText("pending"))
	replay.Visibility = database.ChatMessageVisibilityModel
	rows := []database.ChatMessage{request, fileOnly, answer, anchor, replay}
	original := slices.Clone(rows)

	tagged, err := tagUserRowsWithItemIDs(rows)
	require.NoError(t, err)
	require.Equal(t, original, rows, "the input rows are persisted elsewhere and must stay untagged")

	texts := func(msg database.ChatMessage) []string {
		parts, err := chatprompt.ParseContent(msg)
		require.NoError(t, err)
		var out []string
		for _, part := range parts {
			if part.Type == codersdk.ChatMessagePartTypeText {
				out = append(out, part.Text)
			}
		}
		return out
	}
	require.Equal(t, []string{"first", "second\n[id: 1]"}, texts(tagged[0]))
	require.Equal(t, []string{"[id: 2]"}, texts(tagged[1]))
	require.Equal(t, []string{"pending\n[id: 5]"}, texts(tagged[4]), "a replayed pending request keeps an ID")
	for _, i := range []int{2, 3} {
		require.Equal(t, rows[i], tagged[i], "row %d is not a user request", rows[i].ID)
	}
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
