package chatd //nolint:testpackage // Exercises unexported rollover helpers.

import (
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
	tests := []struct {
		name   string
		tokens int64
		budget contextWindowBudget
		want   rolloverDecision
	}{
		{name: "DisabledAtHundred", tokens: 199_000, budget: newContextWindowBudget(200_000, 32_000, 100)},
		{name: "LimitAtUsableLimit", tokens: 168_000, budget: budget, want: limit},
		{name: "BelowLimit", tokens: 167_999, budget: budget},
		{name: "NoUsage", budget: budget},
		{name: "UnknownWindow", tokens: 1_000_000, budget: newContextWindowBudget(0, 32_000, 70)},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, tc.want, decideRollover(tc.tokens, tc.budget))
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
		})
		require.NoError(t, err)
		require.Len(t, messages, 3)
		parts, err := chatprompt.ParseContent(database.ChatMessage{Content: messages[0].Content, ContentVersion: messages[0].ContentVersion})
		require.NoError(t, err)
		require.Len(t, parts, 1)
		require.Equal(t, "A context window rollover started a fresh context window. Previous window: w:12.\n"+
			"Your previous turn was interrupted by a context rollover; continue the task.\n"+
			"The window reached its usable limit.", parts[0].Text)
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
