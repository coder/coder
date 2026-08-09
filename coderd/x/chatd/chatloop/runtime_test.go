package chatloop_test

import (
	"context"
	"testing"
	"time"

	"charm.land/fantasy"
	"github.com/stretchr/testify/require"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd/x/chatd/chatloop"
	"github.com/coder/coder/v2/coderd/x/chatd/chattest"
	"github.com/coder/quartz"
)

func TestGenerateAssistant_RecordsModelInvocationRuntime(t *testing.T) {
	t.Parallel()

	clock := quartz.NewMock(t)
	model := &chattest.FakeModel{
		ProviderName: "test-provider",
		ModelName:    "test-model",
		StreamFn: func(_ context.Context, _ fantasy.Call) (fantasy.StreamResponse, error) {
			return func(yield func(fantasy.StreamPart) bool) {
				if !yield(fantasy.StreamPart{Type: fantasy.StreamPartTypeTextStart, ID: "t"}) {
					return
				}
				if !yield(fantasy.StreamPart{Type: fantasy.StreamPartTypeTextDelta, ID: "t", Delta: "hello"}) {
					return
				}
				if !yield(fantasy.StreamPart{Type: fantasy.StreamPartTypeTextEnd, ID: "t"}) {
					return
				}
				clock.Advance(1500 * time.Millisecond)
				yield(fantasy.StreamPart{
					Type:         fantasy.StreamPartTypeFinish,
					FinishReason: fantasy.FinishReasonStop,
				})
			}, nil
		},
	}

	outcome, err := chatloop.GenerateAssistant(context.Background(), chatloop.GenerateAssistantOptions{
		Model: model,
		Clock: clock,
	})
	require.NoError(t, err)
	require.Equal(t, 1500*time.Millisecond, outcome.Step.Runtime)
}

// The interrupt path bills the window OnModelStreamStart opens, so that
// hook must fire at the instant PersistedStep.Runtime starts measuring.
func TestGenerateAssistant_ModelStreamStartMatchesRuntimeWindow(t *testing.T) {
	t.Parallel()

	clock := quartz.NewMock(t)
	model := &chattest.FakeModel{
		ProviderName: "test-provider",
		ModelName:    "test-model",
		StreamFn: func(_ context.Context, _ fantasy.Call) (fantasy.StreamResponse, error) {
			return func(yield func(fantasy.StreamPart) bool) {
				if !yield(fantasy.StreamPart{Type: fantasy.StreamPartTypeTextStart, ID: "t"}) {
					return
				}
				if !yield(fantasy.StreamPart{Type: fantasy.StreamPartTypeTextDelta, ID: "t", Delta: "hello"}) {
					return
				}
				if !yield(fantasy.StreamPart{Type: fantasy.StreamPartTypeTextEnd, ID: "t"}) {
					return
				}
				clock.Advance(1500 * time.Millisecond)
				yield(fantasy.StreamPart{
					Type:         fantasy.StreamPartTypeFinish,
					FinishReason: fantasy.FinishReasonStop,
				})
			}, nil
		},
	}

	var startedAt []time.Time
	outcome, err := chatloop.GenerateAssistant(context.Background(), chatloop.GenerateAssistantOptions{
		Model: model,
		Clock: clock,
		OnModelStreamStart: func() {
			startedAt = append(startedAt, clock.Now())
		},
	})
	require.NoError(t, err)
	require.Len(t, startedAt, 1)
	require.Equal(t, outcome.Step.Runtime, clock.Since(startedAt[0]))
}

func TestGenerateAssistant_ErroredStreamReturnsNoStep(t *testing.T) {
	t.Parallel()

	clock := quartz.NewMock(t)
	model := &chattest.FakeModel{
		ProviderName: "test-provider",
		ModelName:    "test-model",
		StreamFn: func(_ context.Context, _ fantasy.Call) (fantasy.StreamResponse, error) {
			return func(yield func(fantasy.StreamPart) bool) {
				if !yield(fantasy.StreamPart{Type: fantasy.StreamPartTypeTextStart, ID: "t"}) {
					return
				}
				clock.Advance(1500 * time.Millisecond)
				yield(fantasy.StreamPart{
					Type:  fantasy.StreamPartTypeError,
					Error: xerrors.New("stream blew up"),
				})
			}, nil
		},
	}

	outcome, err := chatloop.GenerateAssistant(context.Background(), chatloop.GenerateAssistantOptions{
		Model: model,
		Clock: clock,
	})
	require.Error(t, err)
	require.Zero(t, outcome.Step.Runtime)
	require.Empty(t, outcome.Step.Content)
}

func TestGenerateCompaction_RecordsRuntime(t *testing.T) {
	t.Parallel()

	clock := quartz.NewMock(t)
	model := &chattest.FakeModel{
		ProviderName: "test-provider",
		ModelName:    "test-model",
		GenerateFn: func(_ context.Context, _ fantasy.Call) (*fantasy.Response, error) {
			clock.Advance(1500 * time.Millisecond)
			return &fantasy.Response{
				Content: []fantasy.Content{
					fantasy.TextContent{Text: "summary"},
				},
			}, nil
		},
	}

	var startedAt []time.Time
	result, err := chatloop.GenerateCompaction(context.Background(), chatloop.GenerateCompactionOptions{
		Model: model,
		Messages: []fantasy.Message{{
			Role:    fantasy.MessageRoleUser,
			Content: []fantasy.MessagePart{fantasy.TextPart{Text: "hello"}},
		}},
		ThresholdPercent: 70,
		ContextLimit:     100,
		StepUsage:        fantasy.Usage{InputTokens: 90},
		Clock:            clock,
		OnModelStreamStart: func() {
			startedAt = append(startedAt, clock.Now())
		},
	})
	require.NoError(t, err)
	require.Equal(t, "summary", result.SummaryReport)
	require.Equal(t, 1500*time.Millisecond, result.Runtime)
	require.Len(t, startedAt, 1)
	require.Equal(t, result.Runtime, clock.Since(startedAt[0]))
}
