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

func TestContextWithRunPanicsOnNil(t *testing.T) {
	t.Parallel()

	require.Panics(t, func() {
		_ = chatdebug.ContextWithRun(context.Background(), nil)
	})
}
