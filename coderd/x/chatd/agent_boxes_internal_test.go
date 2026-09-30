package chatd

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/sqlc-dev/pqtype"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/x/chatd/agentbox"
	"github.com/coder/coder/v2/coderd/x/chatd/chattool"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/testutil"
)

func newTestBoxEngine(t *testing.T) *agentbox.Engine {
	t.Helper()
	engine, err := agentbox.NewEngine(t.Context(), agentbox.Options{
		Logger:  testutil.Logger(t),
		RootDir: t.TempDir(),
	})
	require.NoError(t, err)
	t.Cleanup(func() { _ = engine.Close(context.Background()) })
	return engine
}

func TestTurnBoxTracker(t *testing.T) {
	t.Parallel()
	engine := newTestBoxEngine(t)

	t.Run("SameKeyReturnsSameBox", func(t *testing.T) {
		t.Parallel()
		var tracker turnBoxTracker
		first, created, err := tracker.acquire(engine, 10)
		require.NoError(t, err)
		require.True(t, created)
		second, created, err := tracker.acquire(engine, 10)
		require.NoError(t, err)
		require.False(t, created)
		require.Same(t, first, second)
		require.NoError(t, tracker.take().Close())
	})

	t.Run("NewKeyReplacesAndClosesOld", func(t *testing.T) {
		t.Parallel()
		var tracker turnBoxTracker
		first, _, err := tracker.acquire(engine, 10)
		require.NoError(t, err)
		second, created, err := tracker.acquire(engine, 11)
		require.NoError(t, err)
		require.True(t, created)
		require.NotSame(t, first, second)
		require.ErrorIs(t, first.WriteFile("x", nil), agentbox.ErrClosed)
		require.NoError(t, tracker.take().Close())
	})

	t.Run("TakeClosesKey", func(t *testing.T) {
		t.Parallel()
		var tracker turnBoxTracker
		box, _, err := tracker.acquire(engine, 10)
		require.NoError(t, err)
		require.Same(t, box, tracker.take())
		require.Nil(t, tracker.take())
		require.NoError(t, box.Close())

		_, _, err = tracker.acquire(engine, 10)
		require.ErrorContains(t, err, "closed")
		_, _, err = tracker.acquire(engine, 9)
		require.ErrorContains(t, err, "closed")
		next, _, err := tracker.acquire(engine, 11)
		require.NoError(t, err)
		require.NoError(t, next.Close())
		_ = tracker.take()
	})

	t.Run("KeyZeroErrors", func(t *testing.T) {
		t.Parallel()
		var tracker turnBoxTracker
		_, _, err := tracker.acquire(engine, 0)
		require.ErrorContains(t, err, "user prompt")
		require.Nil(t, tracker.take())
	})
}

func boxTurnMessages(t *testing.T, results ...codersdk.ChatMessagePart) []database.ChatMessage {
	t.Helper()
	messages := []database.ChatMessage{
		{ID: 1, Role: database.ChatMessageRoleUser, Content: pqtype.NullRawMessage{}},
	}
	for i, part := range results {
		raw, err := json.Marshal([]codersdk.ChatMessagePart{part})
		require.NoError(t, err)
		messages = append(messages, database.ChatMessage{
			ID:      int64(i + 2),
			Role:    database.ChatMessageRoleTool,
			Content: pqtype.NullRawMessage{RawMessage: raw, Valid: true},
		})
	}
	return messages
}

func boxResultPart(toolName string, result string, isError bool) codersdk.ChatMessagePart {
	return codersdk.ChatMessagePart{
		Type:       codersdk.ChatMessagePartTypeToolResult,
		ToolCallID: uuid.NewString(),
		ToolName:   toolName,
		Result:     json.RawMessage(result),
		IsError:    isError,
	}
}

func TestLastTurnBoxID(t *testing.T) {
	t.Parallel()

	assert.Empty(t, lastTurnBoxID(boxTurnMessages(t)))
	assert.Equal(t, "b2", lastTurnBoxID(boxTurnMessages(t,
		boxResultPart(chattool.BoxRunToolName, `{"box_id":"b1","stdout":""}`, false),
		boxResultPart("read_file", `{"box_id":"other"}`, false),
		boxResultPart(chattool.BoxWriteFileToolName, `{"box_id":"b2"}`, false),
		boxResultPart(chattool.BoxRunToolName, `{"error":"busy"}`, false),
		boxResultPart(chattool.BoxRunToolName, `{"box_id":"b3"}`, true),
	)))

	// Only the current turn counts.
	messages := boxTurnMessages(t, boxResultPart(chattool.BoxRunToolName, `{"box_id":"old"}`, false))
	messages = append(messages, database.ChatMessage{ID: 50, Role: database.ChatMessageRoleUser})
	assert.Empty(t, lastTurnBoxID(messages))
}

func TestTurnBoxGetterReset(t *testing.T) {
	t.Parallel()
	engine := newTestBoxEngine(t)

	t.Run("FirstUseNoReset", func(t *testing.T) {
		t.Parallel()
		var tracker turnBoxTracker
		getBox := newTurnBoxGetter(engine, &tracker, boxTurnMessages(t))
		box, reset, err := getBox(t.Context())
		require.NoError(t, err)
		assert.False(t, reset)
		_, reset, err = getBox(t.Context())
		require.NoError(t, err)
		assert.False(t, reset)
		require.NoError(t, box.Close())
		_ = tracker.take()
	})

	t.Run("SameBoxNoReset", func(t *testing.T) {
		t.Parallel()
		var tracker turnBoxTracker
		box, _, err := tracker.acquire(engine, 1)
		require.NoError(t, err)
		getBox := newTurnBoxGetter(engine, &tracker, boxTurnMessages(t,
			boxResultPart(chattool.BoxRunToolName, `{"box_id":"`+box.ID()+`"}`, false),
		))
		got, reset, err := getBox(t.Context())
		require.NoError(t, err)
		require.Same(t, box, got)
		assert.False(t, reset)
		require.NoError(t, tracker.take().Close())
	})

	t.Run("DifferentBoxResetsOnce", func(t *testing.T) {
		t.Parallel()
		var tracker turnBoxTracker
		getBox := newTurnBoxGetter(engine, &tracker, boxTurnMessages(t,
			boxResultPart(chattool.BoxRunToolName, `{"box_id":"lost-on-other-replica"}`, false),
		))
		_, reset, err := getBox(t.Context())
		require.NoError(t, err)
		assert.True(t, reset)
		_, reset, err = getBox(t.Context())
		require.NoError(t, err)
		assert.False(t, reset)
		require.NoError(t, tracker.take().Close())
	})

	t.Run("NoPromptRow", func(t *testing.T) {
		t.Parallel()
		var tracker turnBoxTracker
		getBox := newTurnBoxGetter(engine, &tracker, nil)
		_, _, err := getBox(t.Context())
		require.ErrorContains(t, err, "user prompt")
	})
}

func TestAgentBoxSystemPrompt(t *testing.T) {
	t.Parallel()

	text := systemPromptText(t, buildSystemPrompt(nil, "", "", nil, "user instructions", systemPromptBehaviorContext{agentBoxes: true}))
	require.Contains(t, text, "<agent-box>")
	require.Less(t, strings.Index(text, "<agent-box>"), strings.Index(text, "user instructions"), "user instructions come last")

	text = systemPromptText(t, buildSystemPrompt(nil, "", "", nil, "", systemPromptBehaviorContext{}))
	require.NotContains(t, text, "<agent-box>")

	text = systemPromptText(t, buildSystemPrompt(nil, "", "", nil, "", systemPromptBehaviorContext{
		agentBoxes: true,
		chatMode:   database.NullChatMode{ChatMode: database.ChatModeExplore, Valid: true},
	}))
	require.NotContains(t, text, "<agent-box>")
}

func TestBuiltinPlanToolAllowedBoxTools(t *testing.T) {
	t.Parallel()
	for _, name := range chattool.BoxToolNames {
		assert.True(t, builtinPlanToolAllowed(name, true), name)
		assert.True(t, builtinPlanToolAllowed(name, false), name)
	}
}
