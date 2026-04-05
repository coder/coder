package chatdebug_test

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/x/chatd/chatdebug"
)

func TestContextWithRunRoundTrip(t *testing.T) {
	t.Parallel()

	rc := &chatdebug.RunContext{
		RunID:               uuid.New(),
		ChatID:              uuid.New(),
		RootChatID:          uuid.New(),
		ParentChatID:        uuid.New(),
		ModelConfigID:       uuid.New(),
		TriggerMessageID:    11,
		HistoryTipMessageID: 22,
		Kind:                chatdebug.KindChatTurn,
		Provider:            "anthropic",
		Model:               "claude-sonnet",
	}

	ctx := chatdebug.ContextWithRun(context.Background(), rc)
	got, ok := chatdebug.RunFromContext(ctx)
	require.True(t, ok)
	require.Same(t, rc, got)
	require.Equal(t, *rc, *got)
}

func TestRunFromContextAbsent(t *testing.T) {
	t.Parallel()

	got, ok := chatdebug.RunFromContext(context.Background())
	require.False(t, ok)
	require.Nil(t, got)
}

func TestContextWithStepRoundTrip(t *testing.T) {
	t.Parallel()

	sc := &chatdebug.StepContext{
		StepID:              uuid.New(),
		RunID:               uuid.New(),
		ChatID:              uuid.New(),
		StepNumber:          7,
		Operation:           chatdebug.OperationStream,
		HistoryTipMessageID: 33,
	}

	ctx := chatdebug.ContextWithStep(context.Background(), sc)
	got, ok := chatdebug.StepFromContext(ctx)
	require.True(t, ok)
	require.Same(t, sc, got)
	require.Equal(t, *sc, *got)
}

func TestStepFromContextAbsent(t *testing.T) {
	t.Parallel()

	got, ok := chatdebug.StepFromContext(context.Background())
	require.False(t, ok)
	require.Nil(t, got)
}

func TestContextWithRunAndStep(t *testing.T) {
	t.Parallel()

	rc := &chatdebug.RunContext{RunID: uuid.New(), ChatID: uuid.New()}
	sc := &chatdebug.StepContext{StepID: uuid.New(), RunID: rc.RunID, ChatID: rc.ChatID}

	ctx := chatdebug.ContextWithStep(
		chatdebug.ContextWithRun(context.Background(), rc),
		sc,
	)

	gotRun, ok := chatdebug.RunFromContext(ctx)
	require.True(t, ok)
	require.Same(t, rc, gotRun)

	gotStep, ok := chatdebug.StepFromContext(ctx)
	require.True(t, ok)
	require.Same(t, sc, gotStep)
}

func TestContextWithRunPanicsOnNil(t *testing.T) {
	t.Parallel()

	require.Panics(t, func() {
		_ = chatdebug.ContextWithRun(context.Background(), nil)
	})
}

func TestContextWithStepPanicsOnNil(t *testing.T) {
	t.Parallel()

	require.Panics(t, func() {
		_ = chatdebug.ContextWithStep(context.Background(), nil)
	})
}
