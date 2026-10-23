package chatd

import (
	"database/sql"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/x/chatd/chatprompt"
	"github.com/coder/coder/v2/codersdk"
)

func TestContextLadder(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		limit                  int64
		threshold              int32
		handoff, final, forced int64
		forcedPercent          int32
	}{
		{limit: 200_000, threshold: 70, handoff: 140_000, final: 160_000, forced: 180_000, forcedPercent: 90},
		{limit: 1_000_000, threshold: 30, handoff: 300_000, final: 600_000, forced: 900_000, forcedPercent: 90},
		{limit: 64_000, threshold: 70, handoff: 44_800, final: 51_200, forced: 57_600, forcedPercent: 90},
	} {
		l, ok := contextLadder(tc.limit, tc.threshold)
		require.True(t, ok)
		require.Equal(t, ladder{handoff: tc.handoff, final: tc.final, forced: tc.forced, forcedPercent: tc.forcedPercent}, l)
	}
	for _, tc := range []struct {
		limit     int64
		threshold int32
	}{{200_000, 0}, {200_000, 100}, {0, 70}, {200_000, 95}, {200_000, 90}} {
		_, ok := contextLadder(tc.limit, tc.threshold)
		require.False(t, ok, "limit %d threshold %d", tc.limit, tc.threshold)
	}
}

func TestContextNudgeDue(t *testing.T) {
	t.Parallel()

	l, ok := contextLadder(100, 70)
	require.True(t, ok)
	nextID := int64(0)
	row := func(role database.ChatMessageRole, visibility database.ChatMessageVisibility, text string) database.ChatMessage {
		nextID++
		content, err := chatprompt.MarshalParts([]codersdk.ChatMessagePart{codersdk.ChatMessageText(text)})
		require.NoError(t, err)
		return database.ChatMessage{
			ID:             nextID,
			Role:           role,
			Visibility:     visibility,
			Content:        content,
			ContentVersion: chatprompt.CurrentContentVersion,
			CreatedAt:      time.Unix(nextID, 0),
		}
	}
	user := func() database.ChatMessage {
		return row(database.ChatMessageRoleUser, database.ChatMessageVisibilityBoth, "do it")
	}
	assistant := func(usage int64) database.ChatMessage {
		msg := row(database.ChatMessageRoleAssistant, database.ChatMessageVisibilityBoth, "step")
		msg.InputTokens = sql.NullInt64{Int64: usage, Valid: true}
		return msg
	}
	nudge := func(stage contextNudgeStage) database.ChatMessage {
		return row(database.ChatMessageRoleUser, database.ChatMessageVisibilityModel, contextNudgeText(stage, 0, 100, l))
	}
	anchor := func() database.ChatMessage {
		msg := row(database.ChatMessageRoleUser, database.ChatMessageVisibilityModel, "<context_window>")
		msg.Compressed = true
		return msg
	}
	due := func(rows ...database.ChatMessage) contextNudgeStage {
		stage, _, ok := contextNudgeDue(rows, l)
		if !ok {
			return ""
		}
		return stage
	}

	// Under the handoff point: none.
	require.Equal(t, contextNudgeStage(""), due(user(), assistant(60)))
	// The first window may start over handoff; it still gets a handoff nudge.
	require.Equal(t, contextNudgeHandoff, due(user(), assistant(75)))
	// Handoff once.
	require.Equal(t, contextNudgeStage(""), due(user(), assistant(60), assistant(72), nudge(contextNudgeHandoff), assistant(76)))
	// Final once, after handoff.
	require.Equal(t, contextNudgeFinal, due(user(), assistant(72), nudge(contextNudgeHandoff), assistant(85)))
	require.Equal(t, contextNudgeStage(""), due(user(), assistant(72), nudge(contextNudgeHandoff), assistant(85), nudge(contextNudgeFinal), assistant(87)))
	// Final alone when usage jumps past final; it also claims handoff.
	require.Equal(t, contextNudgeFinal, due(user(), assistant(60), assistant(85)))
	require.Equal(t, contextNudgeStage(""), due(user(), assistant(85), nudge(contextNudgeFinal), assistant(75)))
	// None at or past forced.
	require.Equal(t, contextNudgeStage(""), due(user(), assistant(90)))
	require.Equal(t, contextNudgeStage(""), due(user(), assistant(95)))
	// Loop guard: a new window whose first step is past handoff gets none.
	require.Equal(t, contextNudgeStage(""), due(anchor(), user(), assistant(75), assistant(85)))
	// Claims reset in a new window: the prompt rows hold only the new
	// window, so a window that starts low gets its own nudges.
	require.Equal(t, contextNudgeHandoff, due(anchor(), user(), assistant(20), assistant(75)))
	require.Equal(t, contextNudgeFinal, due(anchor(), user(), assistant(20), assistant(75), nudge(contextNudgeHandoff), assistant(82)))
}

func TestContextWindowStart(t *testing.T) {
	t.Parallel()
	created := time.Unix(100, 0)
	start, ok := contextWindowStart([]database.ChatMessage{{Role: database.ChatMessageRoleSystem, Visibility: database.ChatMessageVisibilityModel}}, created)
	require.False(t, ok)
	require.Equal(t, created, start)
	anchorAt := time.Unix(200, 0)
	start, ok = contextWindowStart([]database.ChatMessage{
		{Role: database.ChatMessageRoleSystem, Visibility: database.ChatMessageVisibilityModel},
		{Role: database.ChatMessageRoleUser, Visibility: database.ChatMessageVisibilityModel, Compressed: true, CreatedAt: anchorAt},
	}, created)
	require.True(t, ok)
	require.Equal(t, anchorAt, start)
}
