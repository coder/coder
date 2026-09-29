package chatd

import (
	"context"
	"database/sql"
	"encoding/json"
	"slices"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/sqlc-dev/pqtype"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"go.opentelemetry.io/otel/trace"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd/database"
	coderdpubsub "github.com/coder/coder/v2/coderd/pubsub"
	"github.com/coder/coder/v2/coderd/x/chatd/chatloop"
	"github.com/coder/coder/v2/coderd/x/chatd/chatprompt"
	"github.com/coder/coder/v2/coderd/x/chatd/chatstate"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/testutil"
	"github.com/coder/quartz"
)

// turnSpansByStart returns the chat_turn spans the recorder saw,
// ordered by start time.
func turnSpansByStart(t *testing.T, recorder *tracetest.SpanRecorder) []sdktrace.ReadOnlySpan {
	t.Helper()
	return stageSpansByStart(t, recorder, chatloop.StageChatTurn)
}

// stageSpansByStart returns the stage spans the recorder saw, ordered
// by start time.
func stageSpansByStart(t *testing.T, recorder *tracetest.SpanRecorder, stage chatloop.Stage) []sdktrace.ReadOnlySpan {
	t.Helper()
	var spans []sdktrace.ReadOnlySpan
	for _, span := range recorder.Ended() {
		if chatloop.Stage(span.Name()) == stage {
			spans = append(spans, span)
		}
	}
	slices.SortFunc(spans, func(a, b sdktrace.ReadOnlySpan) int {
		return a.StartTime().Compare(b.StartTime())
	})
	return spans
}

func TestRunnerTurnSpanStartsAtTriggerMessage(t *testing.T) {
	t.Parallel()
	tracer, recorder := newStageTestTracer(t)
	turn := newRunnerTurnSpan(tracer, false)
	chat := database.Chat{ID: uuid.New()}
	triggerAt := time.Now().Add(-2 * time.Second)

	turnCtx, _ := turn.Ensure(t.Context(), uuid.Nil, chat, triggerAt)
	turn.End(nil)

	ended := recorder.Ended()
	require.Len(t, ended, 2)
	acquisition, chatTurn := ended[0], ended[1]
	require.Equal(t, string(chatloop.StageAcquisition), acquisition.Name())
	require.Equal(t, string(chatloop.StageChatTurn), chatTurn.Name())
	require.Equal(t, triggerAt.UTC(), chatTurn.StartTime().UTC())
	require.Equal(t, triggerAt.UTC(), acquisition.StartTime().UTC())
	require.False(t, acquisition.StartTime().Before(chatTurn.StartTime()))
	require.False(t, acquisition.EndTime().After(chatTurn.EndTime()))
	require.Equal(t, chatTurn.SpanContext().SpanID(), acquisition.Parent().SpanID())
	require.Equal(t, chatTurn.SpanContext().TraceID(), trace.SpanContextFromContext(turnCtx).TraceID())
}

func TestRunnerTurnSpanCarriesChatIdentity(t *testing.T) {
	t.Parallel()
	tracer, recorder := newStageTestTracer(t)
	turn := newRunnerTurnSpan(tracer, false)
	chat := database.Chat{
		ID:           uuid.New(),
		ParentChatID: uuid.NullUUID{UUID: uuid.New(), Valid: true},
	}

	turnCtx, _ := turn.Ensure(t.Context(), uuid.Nil, chat, time.Now().Add(-time.Second))
	_, step := tracer.Start(turnCtx, chatloop.StageGenerationStep)
	step.End(nil)
	start := time.Now().Add(-500 * time.Millisecond)
	tracer.Record(turnCtx, chatloop.StageCommit, chatloop.StageModel{}, start, time.Now(), nil)
	turn.End(nil)

	ended := recorder.Ended()
	require.Len(t, ended, 4)
	for _, span := range ended {
		require.Contains(t, span.Attributes(),
			attribute.String(chatloop.AttrChatKind, string(chatloop.ChatKindSubagent)), span.Name())
	}
}

func TestRunnerTurnSpanEnsureReusesTurnForSameTrigger(t *testing.T) {
	t.Parallel()
	tracer, recorder := newStageTestTracer(t)
	turn := newRunnerTurnSpan(tracer, false)
	chat := database.Chat{ID: uuid.New()}

	trigger := time.Now().Add(-time.Minute)
	_, first := turn.Ensure(t.Context(), uuid.Nil, chat, trigger)
	_, again := turn.Ensure(t.Context(), uuid.Nil, chat, trigger)
	require.Equal(t, first, again)
	require.Empty(t, turnSpansByStart(t, recorder))
	turn.End(nil)
	require.Len(t, turnSpansByStart(t, recorder), 1)
	require.Len(t, stageSpansByStart(t, recorder, chatloop.StageAcquisition), 1)
}

func TestRunnerTurnSpanRotatesOnNewerTrigger(t *testing.T) {
	t.Parallel()
	tracer, recorder := newStageTestTracer(t)
	turn := newRunnerTurnSpan(tracer, false)
	chat := database.Chat{ID: uuid.New()}

	firstTrigger := time.Now().Add(-time.Minute)
	_, first := turn.Ensure(t.Context(), uuid.Nil, chat, firstTrigger)
	// A newer prompt lands while the first turn is still open, and its
	// task reaches Ensure before the canceled task records anything.
	secondTrigger := time.Now().Add(-10 * time.Second)
	_, second := turn.Ensure(t.Context(), uuid.Nil, chat, secondTrigger)
	require.NotEqual(t, first, second)
	turn.Invalidate(first, chatloop.TurnOutcomeInterrupted, xerrors.New("canceled"))
	// An older trigger on the open turn does not rotate it.
	_, again := turn.Ensure(t.Context(), uuid.Nil, chat, firstTrigger)
	require.Equal(t, second, again)
	turn.Complete(second)
	turn.Settle(second)

	turns := turnSpansByStart(t, recorder)
	require.Len(t, turns, 2)
	outcome, _ := spanAttribute(t, turns[0], chatloop.AttrTurnOutcome)
	require.Equal(t, chatloop.TurnOutcomeAbandoned, chatloop.TurnOutcome(outcome.AsString()))
	require.Equal(t, codes.Unset, turns[0].Status().Code)
	require.Equal(t, secondTrigger.UTC(), turns[1].StartTime().UTC())
	outcome, _ = spanAttribute(t, turns[1], chatloop.AttrTurnOutcome)
	require.Equal(t, chatloop.TurnOutcomeCompleted, chatloop.TurnOutcome(outcome.AsString()))

	acquisitions := stageSpansByStart(t, recorder, chatloop.StageAcquisition)
	require.Len(t, acquisitions, 2)
	require.Equal(t, secondTrigger.UTC(), acquisitions[1].StartTime().UTC())
	require.Equal(t, turns[1].SpanContext().SpanID(), acquisitions[1].Parent().SpanID())
}

func TestRunnerTurnSpanClampsStaleAnchor(t *testing.T) {
	t.Parallel()
	ctx := testutil.Context(t, testutil.WaitShort)
	clock := quartz.NewMock(t)
	tracer, recorder, registry := newStageMetricsTracer(t, chatloop.WithClock(clock))
	turn := newRunnerTurnSpan(tracer, false)
	chat := database.Chat{ID: uuid.New()}
	staleAnchors := func() float64 {
		return anomalyCount(t, registry, chatloop.StageAnomalyStaleAnchor)
	}

	firstTrigger := clock.Now().Add(-time.Minute)
	_, first := turn.Ensure(ctx, uuid.Nil, chat, firstTrigger)
	turn.Invalidate(first, chatloop.TurnOutcomeError, xerrors.New("provider refused"))
	turn.Settle(first)
	require.Zero(t, staleAnchors())

	// The same prompt reopening the turn after an invalidation starts
	// at now and records no second acquisition.
	clock.Advance(time.Second).MustWait(ctx)
	reopenedAt := clock.Now()
	_, reopened := turn.Ensure(ctx, uuid.Nil, chat, firstTrigger)
	require.NotEqual(t, first, reopened)
	require.Equal(t, float64(1), staleAnchors())
	turn.Complete(reopened)
	turn.Settle(reopened)

	// So does a trigger before the previous anchor.
	clock.Advance(time.Second).MustWait(ctx)
	olderAt := clock.Now()
	_, older := turn.Ensure(ctx, uuid.Nil, chat, firstTrigger.Add(-30*time.Second))
	require.NotEqual(t, reopened, older)
	require.Equal(t, float64(2), staleAnchors())
	turn.Complete(older)
	turn.Settle(older)

	// A trigger after the previous anchor is kept.
	thirdTrigger := olderAt.Add(time.Millisecond)
	clock.Advance(time.Second).MustWait(ctx)
	_, third := turn.Ensure(ctx, uuid.Nil, chat, thirdTrigger)
	require.NotEqual(t, older, third)
	require.Equal(t, float64(2), staleAnchors())
	turn.End(nil)

	turns := turnSpansByStart(t, recorder)
	require.Len(t, turns, 4)
	require.Equal(t, firstTrigger.UTC(), turns[0].StartTime().UTC())
	require.Equal(t, reopenedAt.UTC(), turns[1].StartTime().UTC())
	require.Equal(t, olderAt.UTC(), turns[2].StartTime().UTC())
	require.Equal(t, thirdTrigger.UTC(), turns[3].StartTime().UTC())

	acquisitions := stageSpansByStart(t, recorder, chatloop.StageAcquisition)
	require.Len(t, acquisitions, 2)
	require.Equal(t, turns[0].SpanContext().SpanID(), acquisitions[0].Parent().SpanID())
	require.Equal(t, turns[3].SpanContext().SpanID(), acquisitions[1].Parent().SpanID())
}

func TestRunnerTurnSpanKeepsAnchorBeforePreviousClose(t *testing.T) {
	t.Parallel()
	tracer, recorder, registry := newStageMetricsTracer(t)
	turn := newRunnerTurnSpan(tracer, false)
	chat := database.Chat{ID: uuid.New()}

	firstTrigger := time.Now().Add(-time.Minute)
	_, first := turn.Ensure(t.Context(), uuid.Nil, chat, firstTrigger)
	// The next prompt lands while the first turn is still running.
	secondTrigger := time.Now().Add(-10 * time.Second)
	turn.Complete(first)
	turn.Settle(first)

	_, second := turn.Ensure(t.Context(), uuid.Nil, chat, secondTrigger)
	require.NotEqual(t, first, second)
	turn.End(nil)

	require.Zero(t, anomalyCount(t, registry, chatloop.StageAnomalyStaleAnchor))
	turns := turnSpansByStart(t, recorder)
	require.Len(t, turns, 2)
	require.Equal(t, secondTrigger.UTC(), turns[1].StartTime().UTC())

	acquisitions := stageSpansByStart(t, recorder, chatloop.StageAcquisition)
	require.Len(t, acquisitions, 2)
	require.Equal(t, secondTrigger.UTC(), acquisitions[1].StartTime().UTC())
	require.Equal(t, turns[1].SpanContext().SpanID(), acquisitions[1].Parent().SpanID())
}

func TestRunnerTurnSpanTakenOverAnchorsAtNow(t *testing.T) {
	t.Parallel()
	ctx := testutil.Context(t, testutil.WaitShort)
	clock := quartz.NewMock(t)
	tracer, recorder, registry := newStageMetricsTracer(t, chatloop.WithClock(clock))
	turn := newRunnerTurnSpan(tracer, true)
	chat := database.Chat{ID: uuid.New()}

	// The previous owner already ran part of this turn.
	staleTrigger := clock.Now().Add(-time.Minute)
	takenOverAt := clock.Now()
	_, first := turn.Ensure(ctx, uuid.Nil, chat, staleTrigger)
	_, again := turn.Ensure(ctx, uuid.Nil, chat, staleTrigger)
	require.Equal(t, first, again)
	turn.Complete(first)
	turn.Settle(first)

	nextTrigger := takenOverAt.Add(time.Millisecond)
	clock.Advance(time.Second).MustWait(ctx)
	_, second := turn.Ensure(ctx, uuid.Nil, chat, nextTrigger)
	require.NotEqual(t, first, second)
	turn.End(nil)

	require.Zero(t, anomalyCount(t, registry, chatloop.StageAnomalyStaleAnchor))
	turns := turnSpansByStart(t, recorder)
	require.Len(t, turns, 2)
	require.Equal(t, takenOverAt.UTC(), turns[0].StartTime().UTC())
	require.Equal(t, nextTrigger.UTC(), turns[1].StartTime().UTC())
	acquisitions := stageSpansByStart(t, recorder, chatloop.StageAcquisition)
	require.Len(t, acquisitions, 1)
	require.Equal(t, turns[1].SpanContext().SpanID(), acquisitions[0].Parent().SpanID())
}

func TestRunnerTurnSpanZeroTriggerRecordsNoAcquisition(t *testing.T) {
	t.Parallel()
	tracer, recorder, registry := newStageMetricsTracer(t)
	turn := newRunnerTurnSpan(tracer, false)

	_, token := turn.Ensure(t.Context(), uuid.Nil, database.Chat{ID: uuid.New()}, time.Time{})
	turn.Complete(token)
	turn.Settle(token)

	require.Len(t, turnSpansByStart(t, recorder), 1)
	require.Empty(t, stageSpansByStart(t, recorder, chatloop.StageAcquisition))
	require.Zero(t, anomalyCount(t, registry, chatloop.StageAnomalyInvertedWindow))
}

// TestRunnerTurnSpanEnsureClosesUnsettledTurn covers an Ensure for the
// same trigger that arrives after the turn finished or was invalidated
// but before Settle closed it.
func TestRunnerTurnSpanEnsureClosesUnsettledTurn(t *testing.T) {
	t.Parallel()

	failure := xerrors.New("provider refused")
	tests := []struct {
		name        string
		close       func(*runnerTurnSpan, turnToken)
		wantOutcome chatloop.TurnOutcome
		wantStatus  codes.Code
	}{
		{
			name:        "Completed",
			close:       func(turn *runnerTurnSpan, token turnToken) { turn.Complete(token) },
			wantOutcome: chatloop.TurnOutcomeCompleted,
			wantStatus:  codes.Unset,
		},
		{
			name: "Invalidated",
			close: func(turn *runnerTurnSpan, token turnToken) {
				turn.Invalidate(token, chatloop.TurnOutcomeError, failure)
			},
			wantOutcome: chatloop.TurnOutcomeError,
			wantStatus:  codes.Error,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			tracer, recorder := newStageTestTracer(t)
			turn := newRunnerTurnSpan(tracer, false)
			chat := database.Chat{ID: uuid.New()}

			trigger := time.Now().Add(-time.Minute)
			_, first := turn.Ensure(t.Context(), uuid.Nil, chat, trigger)
			tt.close(turn, first)
			_, second := turn.Ensure(t.Context(), uuid.Nil, chat, trigger)
			require.NotEqual(t, first, second)
			turn.Complete(second)
			turn.Settle(second)

			turns := turnSpansByStart(t, recorder)
			require.Len(t, turns, 2)
			require.Equal(t, tt.wantStatus, turns[0].Status().Code)
			outcome, _ := spanAttribute(t, turns[0], chatloop.AttrTurnOutcome)
			require.Equal(t, tt.wantOutcome, chatloop.TurnOutcome(outcome.AsString()))
			outcome, _ = spanAttribute(t, turns[1], chatloop.AttrTurnOutcome)
			require.Equal(t, chatloop.TurnOutcomeCompleted, chatloop.TurnOutcome(outcome.AsString()))
		})
	}
}

func TestTurnTriggerTime(t *testing.T) {
	t.Parallel()
	promptAt := time.Now().Add(-time.Minute)
	prompt := database.ChatMessage{
		Role:      database.ChatMessageRoleUser,
		CreatedAt: promptAt,
	}
	messages := []database.ChatMessage{prompt}
	toolResult := func(t *testing.T, toolName string, createdAt time.Time) database.ChatMessage {
		t.Helper()
		content, err := chatprompt.MarshalParts([]codersdk.ChatMessagePart{
			codersdk.ChatMessageToolResult("call-"+toolName, toolName, json.RawMessage(`"ok"`), false, false),
		})
		require.NoError(t, err)
		return database.ChatMessage{
			Role:           database.ChatMessageRoleTool,
			Content:        content,
			ContentVersion: chatprompt.CurrentContentVersion,
			CreatedAt:      createdAt,
		}
	}
	dynamicChat := database.Chat{DynamicTools: pqtype.NullRawMessage{
		RawMessage: json.RawMessage(`[{"name":"approve"}]`),
		Valid:      true,
	}}

	t.Run("PromptOnly", func(t *testing.T) {
		t.Parallel()
		require.Equal(t, promptAt, turnTriggerTime(database.Chat{}, messages))
	})
	t.Run("CompactionLater", func(t *testing.T) {
		t.Parallel()
		compactionAt := promptAt.Add(30 * time.Second)
		chat := database.Chat{CompactionRequestedAt: sql.NullTime{Time: compactionAt, Valid: true}}
		require.Equal(t, compactionAt, turnTriggerTime(chat, messages))
	})
	t.Run("CompactionEarlier", func(t *testing.T) {
		t.Parallel()
		chat := database.Chat{CompactionRequestedAt: sql.NullTime{Time: promptAt.Add(-30 * time.Second), Valid: true}}
		require.Equal(t, promptAt, turnTriggerTime(chat, messages))
	})
	t.Run("CompactionWithoutPrompt", func(t *testing.T) {
		t.Parallel()
		compactionAt := time.Now()
		chat := database.Chat{CompactionRequestedAt: sql.NullTime{Time: compactionAt, Valid: true}}
		require.Equal(t, compactionAt, turnTriggerTime(chat, nil))
	})
	t.Run("Neither", func(t *testing.T) {
		t.Parallel()
		require.True(t, turnTriggerTime(database.Chat{}, nil).IsZero())
	})
	t.Run("DynamicToolResultAfterPrompt", func(t *testing.T) {
		t.Parallel()
		submittedAt := promptAt.Add(time.Minute)
		history := []database.ChatMessage{prompt, toolResult(t, "approve", submittedAt)}
		require.Equal(t, submittedAt, turnTriggerTime(dynamicChat, history))
	})
	t.Run("LocalToolResultAfterPrompt", func(t *testing.T) {
		t.Parallel()
		history := []database.ChatMessage{prompt, toolResult(t, "read_file", promptAt.Add(time.Minute))}
		require.Equal(t, promptAt, turnTriggerTime(dynamicChat, history))
	})
	t.Run("DynamicToolResultBeforePrompt", func(t *testing.T) {
		t.Parallel()
		history := []database.ChatMessage{toolResult(t, "approve", promptAt.Add(-time.Minute)), prompt}
		require.Equal(t, promptAt, turnTriggerTime(dynamicChat, history))
	})
}

func TestRunnerTurnSpanOutcome(t *testing.T) {
	t.Parallel()

	turnSpanFor := func(t *testing.T, recorder *tracetest.SpanRecorder) sdktrace.ReadOnlySpan {
		t.Helper()
		turns := turnSpansByStart(t, recorder)
		require.Len(t, turns, 1)
		return turns[0]
	}
	outcome := func(t *testing.T, span sdktrace.ReadOnlySpan) chatloop.TurnOutcome {
		t.Helper()
		value, ok := spanAttribute(t, span, chatloop.AttrTurnOutcome)
		require.True(t, ok)
		return chatloop.TurnOutcome(value.AsString())
	}

	// Complete follows the committed finishing transition, so a failure
	// after it cannot change how the turn is counted.
	t.Run("InvalidateAfterCompleteIgnored", func(t *testing.T) {
		t.Parallel()
		tracer, recorder := newStageTestTracer(t)
		turn := newRunnerTurnSpan(tracer, false)
		_, token := turn.Ensure(t.Context(), uuid.Nil, database.Chat{ID: uuid.New()}, time.Now())
		turn.Complete(token)
		turn.Invalidate(token, chatloop.TurnOutcomeError, xerrors.New("publish watch"))
		turn.Settle(token)

		span := turnSpanFor(t, recorder)
		require.Equal(t, codes.Unset, span.Status().Code)
		require.Equal(t, chatloop.TurnOutcomeCompleted, outcome(t, span))
	})

	t.Run("Error", func(t *testing.T) {
		t.Parallel()
		tracer, recorder := newStageTestTracer(t)
		turn := newRunnerTurnSpan(tracer, false)
		_, token := turn.Ensure(t.Context(), uuid.Nil, database.Chat{ID: uuid.New()}, time.Now())
		firstErr := xerrors.New("provider refused")
		turn.Invalidate(token, chatloop.TurnOutcomeError, firstErr)
		// The first invalidation wins over later ones.
		turn.Invalidate(token, chatloop.TurnOutcomeInterrupted, xerrors.New("later"))
		turn.End(nil)

		span := turnSpanFor(t, recorder)
		require.Equal(t, codes.Error, span.Status().Code)
		require.Equal(t, firstErr.Error(), span.Status().Description)
		require.Equal(t, chatloop.TurnOutcomeError, outcome(t, span))
	})

	t.Run("SettleClosesInvalidatedTurn", func(t *testing.T) {
		t.Parallel()
		tracer, recorder := newStageTestTracer(t)
		turn := newRunnerTurnSpan(tracer, false)
		_, token := turn.Ensure(t.Context(), uuid.Nil, database.Chat{ID: uuid.New()}, time.Now())
		turn.Invalidate(token, chatloop.TurnOutcomeError, xerrors.New("provider refused"))
		turn.Settle(token)

		span := turnSpanFor(t, recorder)
		require.Equal(t, codes.Error, span.Status().Code)
		require.Equal(t, chatloop.TurnOutcomeError, outcome(t, span))
		// Runner exit after Settle records no second turn.
		turn.End(nil)
		require.Len(t, turnSpansByStart(t, recorder), 1)
	})

	t.Run("SettleLeavesRunningTurnOpen", func(t *testing.T) {
		t.Parallel()
		tracer, recorder := newStageTestTracer(t)
		turn := newRunnerTurnSpan(tracer, false)
		_, token := turn.Ensure(t.Context(), uuid.Nil, database.Chat{ID: uuid.New()}, time.Now())
		turn.Settle(token)
		require.Empty(t, turnSpansByStart(t, recorder))
		// End closes the unfinished turn as abandoned.
		turn.End(nil)
		span := turnSpanFor(t, recorder)
		require.Equal(t, codes.Unset, span.Status().Code)
		require.Equal(t, chatloop.TurnOutcomeAbandoned, outcome(t, span))
	})

	t.Run("ErrorThenSettled", func(t *testing.T) {
		t.Parallel()
		tracer, recorder := newStageTestTracer(t)
		turn := newRunnerTurnSpan(tracer, false)
		_, token := turn.Ensure(t.Context(), uuid.Nil, database.Chat{ID: uuid.New()}, time.Now())
		turn.Invalidate(token, chatloop.TurnOutcomeError, xerrors.New("provider refused"))
		turn.Complete(token)
		turn.Settle(token)

		span := turnSpanFor(t, recorder)
		require.Equal(t, codes.Error, span.Status().Code)
		require.Equal(t, chatloop.TurnOutcomeError, outcome(t, span))
	})

	t.Run("StaleTokenIgnored", func(t *testing.T) {
		t.Parallel()
		tracer, recorder := newStageTestTracer(t)
		turn := newRunnerTurnSpan(tracer, false)
		_, token := turn.Ensure(t.Context(), uuid.Nil, database.Chat{ID: uuid.New()}, time.Now())
		turn.Invalidate(token+1, chatloop.TurnOutcomeError, xerrors.New("not this turn"))
		turn.Complete(token)
		turn.End(nil)

		span := turnSpanFor(t, recorder)
		require.Equal(t, codes.Unset, span.Status().Code)
		require.Equal(t, chatloop.TurnOutcomeCompleted, outcome(t, span))
	})

	t.Run("StaleTokenCannotCloseReplacement", func(t *testing.T) {
		t.Parallel()
		tracer, recorder := newStageTestTracer(t)
		turn := newRunnerTurnSpan(tracer, false)
		chat := database.Chat{ID: uuid.New()}
		_, first := turn.Ensure(t.Context(), uuid.Nil, chat, time.Now().Add(-time.Minute))
		_, second := turn.Ensure(t.Context(), uuid.Nil, chat, time.Now().Add(-time.Second))
		require.NotEqual(t, first, second)
		turn.Complete(first)
		turn.Settle(first)
		require.Len(t, turnSpansByStart(t, recorder), 1)
		require.Equal(t, second, turn.OpenToken(t.Context()))

		turn.End(nil)
		turns := turnSpansByStart(t, recorder)
		require.Len(t, turns, 2)
		require.Equal(t, chatloop.TurnOutcomeAbandoned, outcome(t, turns[1]))
	})
}

// TestRunnerTurnSpanCanceledEnsure runs a canceled task's Ensure after
// its replacement opened the turn for a newer prompt. The canceled task
// gets no token, so its failure cannot close the replacement's turn.
func TestRunnerTurnSpanCanceledEnsure(t *testing.T) {
	t.Parallel()
	tracer, recorder := newStageTestTracer(t)
	turn := newRunnerTurnSpan(tracer, false)
	chat := database.Chat{ID: uuid.New()}
	firstTrigger := time.Now().Add(-time.Minute)
	secondTrigger := time.Now().Add(-10 * time.Second)

	_, first := turn.Ensure(t.Context(), uuid.Nil, chat, firstTrigger)
	_, second := turn.Ensure(t.Context(), uuid.Nil, chat, secondTrigger)
	canceled, cancel := context.WithCancel(t.Context())
	cancel()
	canceledCtx, stale := turn.Ensure(canceled, uuid.Nil, chat, firstTrigger)
	require.Zero(t, stale)
	require.False(t, trace.SpanContextFromContext(canceledCtx).IsValid())
	turn.Invalidate(stale, chatloop.TurnOutcomeError, xerrors.New("canceled"))
	turn.Settle(stale)
	require.Equal(t, second, turn.OpenToken(t.Context()))

	_, again := turn.Ensure(t.Context(), uuid.Nil, chat, secondTrigger)
	require.Equal(t, second, again)
	turn.Complete(again)
	turn.Settle(again)

	turns := turnSpansByStart(t, recorder)
	require.Len(t, turns, 2)
	require.NotEqual(t, first, second)
	for i, want := range []chatloop.TurnOutcome{chatloop.TurnOutcomeAbandoned, chatloop.TurnOutcomeCompleted} {
		value, _ := spanAttribute(t, turns[i], chatloop.AttrTurnOutcome)
		require.Equal(t, want, chatloop.TurnOutcome(value.AsString()))
		require.Equal(t, codes.Unset, turns[i].Status().Code)
	}
}

// TestRunnerTurnSpanCanceledOpenToken runs a canceled interrupt task's
// OpenToken after an edited prompt opened a new turn. The canceled task
// gets no token, so the edited turn completes and the stopped turn
// closes as abandoned.
func TestRunnerTurnSpanCanceledOpenToken(t *testing.T) {
	t.Parallel()
	tracer, recorder := newStageTestTracer(t)
	turn := newRunnerTurnSpan(tracer, false)
	chat := database.Chat{ID: uuid.New()}

	turn.Ensure(t.Context(), uuid.Nil, chat, time.Now().Add(-time.Minute))
	interruptCtx, cancel := context.WithCancel(t.Context())
	cancel()
	_, edited := turn.Ensure(t.Context(), uuid.Nil, chat, time.Now().Add(-time.Second))
	stopped := turn.OpenToken(interruptCtx)
	require.Zero(t, stopped)
	turn.Invalidate(stopped, chatloop.TurnOutcomeInterrupted, nil)
	turn.Complete(edited)
	turn.Settle(edited)

	turns := turnSpansByStart(t, recorder)
	require.Len(t, turns, 2)
	for i, want := range []chatloop.TurnOutcome{chatloop.TurnOutcomeAbandoned, chatloop.TurnOutcomeCompleted} {
		value, _ := spanAttribute(t, turns[i], chatloop.AttrTurnOutcome)
		require.Equal(t, want, chatloop.TurnOutcome(value.AsString()))
	}
}

// TestRunnerTurnSpanObservesOnlyCompletedTurns closes one turn with each
// outcome and checks that only the completed one is observed on the
// stage histogram.
func TestRunnerTurnSpanObservesOnlyCompletedTurns(t *testing.T) {
	t.Parallel()
	tracer, recorder, registry := newStageMetricsTracer(t)
	turn := newRunnerTurnSpan(tracer, false)
	chat := database.Chat{ID: uuid.New()}
	base := time.Now().Add(-time.Hour)

	_, token := turn.Ensure(t.Context(), uuid.Nil, chat, base)
	turn.Complete(token)
	turn.Settle(token)
	for i, outcome := range []chatloop.TurnOutcome{chatloop.TurnOutcomeInterrupted, chatloop.TurnOutcomeError} {
		_, token = turn.Ensure(t.Context(), uuid.Nil, chat, base.Add(time.Duration(i+1)*time.Minute))
		turn.Invalidate(token, outcome, xerrors.New(string(outcome)))
		turn.Settle(token)
	}
	// Runner exit closes the last turn as abandoned.
	turn.Ensure(t.Context(), uuid.Nil, chat, base.Add(10*time.Minute))
	turn.End(nil)

	require.Len(t, turnSpansByStart(t, recorder), 4)
	require.Equal(t, uint64(1), stageObservationCount(t, registry, chatloop.StageChatTurn))
}

// TestRunnerTurnSpanSupersededTurnWaitsForHolder replaces a turn with a
// newer prompt's turn while the task that ran it still holds it. The
// replaced turn ends at the replacement time, and the outcome its task
// records before releasing it is the one emitted.
func TestRunnerTurnSpanSupersededTurnWaitsForHolder(t *testing.T) {
	t.Parallel()

	failure := xerrors.New("provider refused")
	tests := []struct {
		name        string
		mark        func(*runnerTurnSpan, turnToken)
		wantOutcome chatloop.TurnOutcome
		wantStatus  codes.Code
	}{
		{
			name:        "Completed",
			mark:        func(turn *runnerTurnSpan, token turnToken) { turn.Complete(token) },
			wantOutcome: chatloop.TurnOutcomeCompleted,
			wantStatus:  codes.Unset,
		},
		{
			name: "Error",
			mark: func(turn *runnerTurnSpan, token turnToken) {
				turn.Invalidate(token, chatloop.TurnOutcomeError, failure)
			},
			wantOutcome: chatloop.TurnOutcomeError,
			wantStatus:  codes.Error,
		},
		{
			name:        "Unmarked",
			mark:        func(*runnerTurnSpan, turnToken) {},
			wantOutcome: chatloop.TurnOutcomeAbandoned,
			wantStatus:  codes.Unset,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			ctx := testutil.Context(t, testutil.WaitShort)
			clock := quartz.NewMock(t)
			tracer, recorder, _ := newStageMetricsTracer(t, chatloop.WithClock(clock))
			turn := newRunnerTurnSpan(tracer, false)
			chat := database.Chat{ID: uuid.New()}
			finishing, promoted := uuid.New(), uuid.New()

			_, first := turn.Ensure(ctx, finishing, chat, clock.Now().Add(-time.Minute))
			clock.Advance(time.Second).MustWait(ctx)
			supersededAt := clock.Now()
			_, second := turn.Ensure(ctx, promoted, chat, clock.Now().Add(-time.Millisecond))
			require.NotEqual(t, first, second)
			require.Equal(t, second, turn.OpenToken(ctx))
			require.Empty(t, turnSpansByStart(t, recorder))

			tt.mark(turn, first)
			clock.Advance(time.Second).MustWait(ctx)
			turn.Release(finishing)

			turns := turnSpansByStart(t, recorder)
			require.Len(t, turns, 1)
			require.Equal(t, supersededAt.UTC(), turns[0].EndTime().UTC())
			require.Equal(t, tt.wantStatus, turns[0].Status().Code)
			outcome, _ := spanAttribute(t, turns[0], chatloop.AttrTurnOutcome)
			require.Equal(t, tt.wantOutcome, chatloop.TurnOutcome(outcome.AsString()))

			// Marks after emission are ignored.
			turn.Invalidate(first, chatloop.TurnOutcomeError, failure)
			turn.Settle(first)
			require.Len(t, turnSpansByStart(t, recorder), 1)
		})
	}
}

// TestRunnerTurnSpanSettleWaitsForHolders settles a turn that two tasks
// joined. The turn is no longer current once settled, ends at the
// settle time, and is emitted when the last holder releases it.
func TestRunnerTurnSpanSettleWaitsForHolders(t *testing.T) {
	t.Parallel()
	ctx := testutil.Context(t, testutil.WaitShort)
	clock := quartz.NewMock(t)
	tracer, recorder, _ := newStageMetricsTracer(t, chatloop.WithClock(clock))
	turn := newRunnerTurnSpan(tracer, false)
	chat := database.Chat{ID: uuid.New()}
	firstStep, secondStep := uuid.New(), uuid.New()
	trigger := clock.Now().Add(-time.Minute)

	_, token := turn.Ensure(ctx, firstStep, chat, trigger)
	_, joined := turn.Ensure(ctx, secondStep, chat, trigger)
	require.Equal(t, token, joined)
	turn.Invalidate(token, chatloop.TurnOutcomeInterrupted, nil)
	clock.Advance(time.Second).MustWait(ctx)
	settledAt := clock.Now()
	turn.Settle(token)
	require.Zero(t, turn.OpenToken(ctx))

	clock.Advance(time.Second).MustWait(ctx)
	turn.Release(secondStep)
	require.Empty(t, turnSpansByStart(t, recorder))
	turn.Release(firstStep)

	turns := turnSpansByStart(t, recorder)
	require.Len(t, turns, 1)
	require.Equal(t, settledAt.UTC(), turns[0].EndTime().UTC())
	outcome, _ := spanAttribute(t, turns[0], chatloop.AttrTurnOutcome)
	require.Equal(t, chatloop.TurnOutcomeInterrupted, chatloop.TurnOutcome(outcome.AsString()))
}

// TestRunnerTurnSpanReleasedTurnEmitsOnClose releases the only holder
// of the current turn before it closes; closing it emits it at once.
func TestRunnerTurnSpanReleasedTurnEmitsOnClose(t *testing.T) {
	t.Parallel()
	tracer, recorder := newStageTestTracer(t)
	turn := newRunnerTurnSpan(tracer, false)
	task := uuid.New()

	_, token := turn.Ensure(t.Context(), task, database.Chat{ID: uuid.New()}, time.Now())
	turn.Release(task)
	turn.Release(uuid.New())
	require.Empty(t, turnSpansByStart(t, recorder))
	turn.Complete(token)
	turn.Settle(token)
	require.Len(t, turnSpansByStart(t, recorder), 1)
}

// TestRunnerTurnSpanEndEmitsHeldTurns ends the runner while a replaced
// turn and the current turn are both still held. End emits both, and
// no turn opens afterwards.
func TestRunnerTurnSpanEndEmitsHeldTurns(t *testing.T) {
	t.Parallel()
	tracer, recorder := newStageTestTracer(t)
	turn := newRunnerTurnSpan(tracer, false)
	chat := database.Chat{ID: uuid.New()}
	stale, current := uuid.New(), uuid.New()

	_, first := turn.Ensure(t.Context(), stale, chat, time.Now().Add(-time.Minute))
	turn.Ensure(t.Context(), current, chat, time.Now().Add(-time.Second))
	turn.Complete(first)
	turn.End(nil)

	turns := turnSpansByStart(t, recorder)
	require.Len(t, turns, 2)
	for i, want := range []chatloop.TurnOutcome{chatloop.TurnOutcomeCompleted, chatloop.TurnOutcomeAbandoned} {
		outcome, _ := spanAttribute(t, turns[i], chatloop.AttrTurnOutcome)
		require.Equal(t, want, chatloop.TurnOutcome(outcome.AsString()))
	}

	turn.Release(stale)
	turn.Release(current)
	_, token := turn.Ensure(t.Context(), uuid.New(), chat, time.Now())
	require.Zero(t, token)
	require.Len(t, turnSpansByStart(t, recorder), 2)
}

// stageObservationCount returns how many observations the stage
// duration histogram recorded for stage.
func stageObservationCount(t *testing.T, registry *prometheus.Registry, stage chatloop.Stage) uint64 {
	t.Helper()
	families, err := registry.Gather()
	require.NoError(t, err)
	var count uint64
	for _, family := range families {
		if family.GetName() != "coderd_chatd_stage_duration_seconds" {
			continue
		}
		for _, metric := range family.GetMetric() {
			for _, label := range metric.GetLabel() {
				if label.GetName() == "stage" && label.GetValue() == string(stage) {
					count += metric.GetHistogram().GetSampleCount()
				}
			}
		}
	}
	return count
}

// TestFinishGenerationErrorOutcomeSurvivesCommitCancel cancels the task
// context as soon as the finishing commit publishes its state change.
// The turn still closes as an error, because the committed FinishError
// decides the outcome.
func TestFinishGenerationErrorOutcomeSurvivesCommitCancel(t *testing.T) {
	t.Parallel()
	f := newTaskTestFixture(t)
	chat := f.createRunningChat(t)
	workerID, runnerID := uuid.New(), uuid.New()
	acquired := f.acquireChat(t, chat.ID, workerID, runnerID)
	starter := newTestTaskStarter(t, f, newTaskSideEffectRecorder())
	tracer, recorder := newStageTestTracer(t)
	turn := newRunnerTurnSpan(tracer, false)

	taskCtx, cancel := context.WithCancel(testutil.Context(t, testutil.WaitLong))
	defer cancel()
	f.pubsub.mu.Lock()
	f.pubsub.onPublish = func(channel string) {
		if channel == coderdpubsub.ChatStateUpdateChannel(chat.ID) {
			cancel()
		}
	}
	f.pubsub.mu.Unlock()

	_, token := turn.Ensure(taskCtx, uuid.Nil, acquired, acquired.CreatedAt)
	input := chatWorkerTaskStartInput{
		ChatID:            chat.ID,
		WorkerID:          workerID,
		RunnerID:          runnerID,
		HistoryVersion:    acquired.HistoryVersion,
		GenerationAttempt: acquired.GenerationAttempt,
		Status:            database.ChatStatusRunning,
		TurnSpan:          turn,
		TurnToken:         token,
	}
	machine := chatstate.NewChatMachine(f.db, f.pubsub, chat.ID)
	// The error returned by post-commit side effects on the canceled
	// context is irrelevant here.
	_ = starter.finishGenerationError(taskCtx, machine, input, xerrors.New("provider refused"), generationAttemptNotRequired)
	require.ErrorIs(t, taskCtx.Err(), context.Canceled)
	latest, err := f.db.GetChatByID(testutil.Context(t, testutil.WaitShort), chat.ID)
	require.NoError(t, err)
	require.Equal(t, database.ChatStatusError, latest.Status)
	turn.Settle(token)

	turns := turnSpansByStart(t, recorder)
	require.Len(t, turns, 1)
	outcome, _ := spanAttribute(t, turns[0], chatloop.AttrTurnOutcome)
	require.Equal(t, chatloop.TurnOutcomeError, chatloop.TurnOutcome(outcome.AsString()))
}

// TestInterruptTaskInterruptsOpenTurn interrupts a chat whose turn is
// open but held by no generation task, as between two steps. The
// interrupt task closes that turn as interrupted.
func TestInterruptTaskInterruptsOpenTurn(t *testing.T) {
	t.Parallel()
	f := newTaskTestFixture(t)
	chat := f.createRunningChat(t)
	workerID, runnerID := uuid.New(), uuid.New()
	acquired := f.acquireChat(t, chat.ID, workerID, runnerID)
	starter := newTestTaskStarter(t, f, newTaskSideEffectRecorder())
	tracer, recorder := newStageTestTracer(t)
	turn := newRunnerTurnSpan(tracer, false)
	_, token := turn.Ensure(t.Context(), uuid.Nil, acquired, acquired.CreatedAt)
	require.NotZero(t, token)

	interrupting := f.interruptChat(t, chat.ID)
	require.Equal(t, database.ChatStatusInterrupting, interrupting.Status)
	require.NoError(t, starter.StartInterrupt(testutil.Context(t, testutil.WaitLong), chatWorkerTaskStartInput{
		ChatID:            chat.ID,
		WorkerID:          workerID,
		RunnerID:          runnerID,
		HistoryVersion:    interrupting.HistoryVersion,
		GenerationAttempt: interrupting.GenerationAttempt,
		Status:            database.ChatStatusInterrupting,
		TurnSpan:          turn,
	}))

	turns := turnSpansByStart(t, recorder)
	require.Len(t, turns, 1)
	outcome, _ := spanAttribute(t, turns[0], chatloop.AttrTurnOutcome)
	require.Equal(t, chatloop.TurnOutcomeInterrupted, chatloop.TurnOutcome(outcome.AsString()))
	require.Equal(t, codes.Unset, turns[0].Status().Code)
	require.Zero(t, turn.OpenToken(t.Context()))
}

// TestInterruptTaskMarksTurnBeforeCommit opens a newer turn while the
// interrupt task's FinishInterruption commit publishes, as a promoted
// prompt's task can. The stopped turn was marked before the commit, so
// the newer turn closes it as interrupted.
func TestInterruptTaskMarksTurnBeforeCommit(t *testing.T) {
	t.Parallel()
	f := newTaskTestFixture(t)
	chat := f.createRunningChat(t)
	workerID, runnerID := uuid.New(), uuid.New()
	acquired := f.acquireChat(t, chat.ID, workerID, runnerID)
	starter := newTestTaskStarter(t, f, newTaskSideEffectRecorder())
	tracer, recorder := newStageTestTracer(t)
	turn := newRunnerTurnSpan(tracer, false)
	_, token := turn.Ensure(t.Context(), uuid.Nil, acquired, acquired.CreatedAt)
	require.NotZero(t, token)

	interrupting := f.interruptChat(t, chat.ID)
	var promoted turnToken
	f.pubsub.mu.Lock()
	f.pubsub.onPublish = func(channel string) {
		if channel == coderdpubsub.ChatStateUpdateChannel(chat.ID) && promoted == 0 {
			_, promoted = turn.Ensure(t.Context(), uuid.Nil, acquired, acquired.CreatedAt.Add(time.Minute))
		}
	}
	f.pubsub.mu.Unlock()
	require.NoError(t, starter.StartInterrupt(testutil.Context(t, testutil.WaitLong), chatWorkerTaskStartInput{
		ChatID:            chat.ID,
		WorkerID:          workerID,
		RunnerID:          runnerID,
		HistoryVersion:    interrupting.HistoryVersion,
		GenerationAttempt: interrupting.GenerationAttempt,
		Status:            database.ChatStatusInterrupting,
		TurnSpan:          turn,
	}))
	require.NotZero(t, promoted)
	require.NotEqual(t, token, promoted)

	turns := turnSpansByStart(t, recorder)
	require.Len(t, turns, 1)
	outcome, _ := spanAttribute(t, turns[0], chatloop.AttrTurnOutcome)
	require.Equal(t, chatloop.TurnOutcomeInterrupted, chatloop.TurnOutcome(outcome.AsString()))
	require.Equal(t, promoted, turn.OpenToken(t.Context()))
}
