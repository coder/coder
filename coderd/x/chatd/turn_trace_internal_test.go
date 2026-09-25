package chatd

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
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

	"github.com/coder/coder/v2/coderd/coderdtest/promhelp"
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
	turn := newRunnerTurnSpan(tracer, nil, false)
	chat := database.Chat{ID: uuid.New()}
	triggerAt := time.Now().Add(-2 * time.Second)

	turnCtx, _ := turn.Ensure(t.Context(), chat, triggerAt)
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
	orgID := uuid.New()
	var resolved []uuid.UUID
	turn := newRunnerTurnSpan(tracer, func(_ context.Context, id uuid.UUID) string {
		resolved = append(resolved, id)
		return "acme"
	}, false)
	chat := database.Chat{
		ID:             uuid.New(),
		OrganizationID: orgID,
		ParentChatID:   uuid.NullUUID{UUID: uuid.New(), Valid: true},
	}

	turnCtx, _ := turn.Ensure(t.Context(), chat, time.Now().Add(-time.Second))
	_, step := tracer.Start(turnCtx, chatloop.StageGenerationStep)
	step.End(nil)
	start := time.Now().Add(-500 * time.Millisecond)
	tracer.Record(turnCtx, chatloop.StageCommit, chatloop.StageModel{}, start, time.Now(), nil)
	turn.End(nil)

	require.Equal(t, []uuid.UUID{orgID}, resolved)
	ended := recorder.Ended()
	require.Len(t, ended, 4)
	for _, span := range ended {
		require.Contains(t, span.Attributes(),
			attribute.String(chatloop.AttrOrganizationName, "acme"), span.Name())
		require.Contains(t, span.Attributes(),
			attribute.String(chatloop.AttrChatKind, string(chatloop.ChatKindSubagent)), span.Name())
	}
}

func TestRunnerTurnSpanEnsureReusesTurnForSameTrigger(t *testing.T) {
	t.Parallel()
	tracer, recorder := newStageTestTracer(t)
	turn := newRunnerTurnSpan(tracer, nil, false)
	chat := database.Chat{ID: uuid.New()}

	trigger := time.Now().Add(-time.Minute)
	_, first := turn.Ensure(t.Context(), chat, trigger)
	_, again := turn.Ensure(t.Context(), chat, trigger)
	require.Equal(t, first, again)
	require.Empty(t, turnSpansByStart(t, recorder))
	turn.End(nil)
	require.Len(t, turnSpansByStart(t, recorder), 1)
	require.Len(t, stageSpansByStart(t, recorder, chatloop.StageAcquisition), 1)
}

func TestRunnerTurnSpanRotatesOnNewerTrigger(t *testing.T) {
	t.Parallel()
	tracer, recorder := newStageTestTracer(t)
	turn := newRunnerTurnSpan(tracer, nil, false)
	chat := database.Chat{ID: uuid.New()}

	firstTrigger := time.Now().Add(-time.Minute)
	_, first := turn.Ensure(t.Context(), chat, firstTrigger)
	// A newer prompt lands while the first turn is still open, and its
	// task reaches Ensure before the canceled task records anything.
	secondTrigger := time.Now().Add(-10 * time.Second)
	_, second := turn.Ensure(t.Context(), chat, secondTrigger)
	require.NotEqual(t, first, second)
	turn.Invalidate(first, chatloop.TurnOutcomeInterrupted, xerrors.New("canceled"))
	// An older trigger on the open turn does not rotate it.
	_, again := turn.Ensure(t.Context(), chat, firstTrigger)
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
	turn := newRunnerTurnSpan(tracer, nil, false)
	chat := database.Chat{ID: uuid.New()}
	staleAnchors := func() float64 {
		return anomalyCount(t, registry, chatloop.StageAnomalyStaleAnchor)
	}

	firstTrigger := clock.Now().Add(-time.Minute)
	_, first := turn.Ensure(ctx, chat, firstTrigger)
	turn.Invalidate(first, chatloop.TurnOutcomeError, xerrors.New("provider refused"))
	turn.Settle(first)
	require.Zero(t, staleAnchors())

	// The same prompt reopening the turn after an invalidation starts
	// at now and records no second acquisition.
	clock.Advance(time.Second).MustWait(ctx)
	reopenedAt := clock.Now()
	_, reopened := turn.Ensure(ctx, chat, firstTrigger)
	require.NotEqual(t, first, reopened)
	require.Equal(t, float64(1), staleAnchors())
	turn.Complete(reopened)
	turn.Settle(reopened)

	// So does a trigger before the previous anchor.
	clock.Advance(time.Second).MustWait(ctx)
	olderAt := clock.Now()
	_, older := turn.Ensure(ctx, chat, firstTrigger.Add(-30*time.Second))
	require.NotEqual(t, reopened, older)
	require.Equal(t, float64(2), staleAnchors())
	turn.Complete(older)
	turn.Settle(older)

	// A trigger after the previous anchor is kept.
	thirdTrigger := olderAt.Add(time.Millisecond)
	clock.Advance(time.Second).MustWait(ctx)
	_, third := turn.Ensure(ctx, chat, thirdTrigger)
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
	turn := newRunnerTurnSpan(tracer, nil, false)
	chat := database.Chat{ID: uuid.New()}

	firstTrigger := time.Now().Add(-time.Minute)
	_, first := turn.Ensure(t.Context(), chat, firstTrigger)
	// The next prompt lands while the first turn is still running.
	secondTrigger := time.Now().Add(-10 * time.Second)
	turn.Complete(first)
	turn.Settle(first)

	_, second := turn.Ensure(t.Context(), chat, secondTrigger)
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
	turn := newRunnerTurnSpan(tracer, nil, true)
	chat := database.Chat{ID: uuid.New()}

	// The previous owner already ran part of this turn.
	staleTrigger := clock.Now().Add(-time.Minute)
	takenOverAt := clock.Now()
	_, first := turn.Ensure(ctx, chat, staleTrigger)
	_, again := turn.Ensure(ctx, chat, staleTrigger)
	require.Equal(t, first, again)
	turn.Complete(first)
	turn.Settle(first)

	nextTrigger := takenOverAt.Add(time.Millisecond)
	clock.Advance(time.Second).MustWait(ctx)
	_, second := turn.Ensure(ctx, chat, nextTrigger)
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
	turn := newRunnerTurnSpan(tracer, nil, false)

	_, token := turn.Ensure(t.Context(), database.Chat{ID: uuid.New()}, time.Time{})
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
			turn := newRunnerTurnSpan(tracer, nil, false)
			chat := database.Chat{ID: uuid.New()}

			trigger := time.Now().Add(-time.Minute)
			_, first := turn.Ensure(t.Context(), chat, trigger)
			tt.close(turn, first)
			_, second := turn.Ensure(t.Context(), chat, trigger)
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
		turn := newRunnerTurnSpan(tracer, nil, false)
		_, token := turn.Ensure(t.Context(), database.Chat{ID: uuid.New()}, time.Now())
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
		turn := newRunnerTurnSpan(tracer, nil, false)
		_, token := turn.Ensure(t.Context(), database.Chat{ID: uuid.New()}, time.Now())
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
		turn := newRunnerTurnSpan(tracer, nil, false)
		_, token := turn.Ensure(t.Context(), database.Chat{ID: uuid.New()}, time.Now())
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
		turn := newRunnerTurnSpan(tracer, nil, false)
		_, token := turn.Ensure(t.Context(), database.Chat{ID: uuid.New()}, time.Now())
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
		turn := newRunnerTurnSpan(tracer, nil, false)
		_, token := turn.Ensure(t.Context(), database.Chat{ID: uuid.New()}, time.Now())
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
		turn := newRunnerTurnSpan(tracer, nil, false)
		_, token := turn.Ensure(t.Context(), database.Chat{ID: uuid.New()}, time.Now())
		turn.Invalidate(token+1, chatloop.TurnOutcomeError, xerrors.New("not this turn"))
		turn.Complete(token)
		turn.End(nil)

		span := turnSpanFor(t, recorder)
		require.Equal(t, codes.Unset, span.Status().Code)
		require.Equal(t, chatloop.TurnOutcomeCompleted, outcome(t, span))
	})

	// A task holding the token of a replaced turn cannot finish or close
	// the turn that replaced it.
	t.Run("StaleTokenCannotCloseReplacement", func(t *testing.T) {
		t.Parallel()
		tracer, recorder := newStageTestTracer(t)
		turn := newRunnerTurnSpan(tracer, nil, false)
		chat := database.Chat{ID: uuid.New()}
		_, first := turn.Ensure(t.Context(), chat, time.Now().Add(-time.Minute))
		_, second := turn.Ensure(t.Context(), chat, time.Now().Add(-time.Second))
		require.NotEqual(t, first, second)
		turn.Complete(first)
		turn.Settle(first)
		// Only the rotated first turn has closed.
		require.Len(t, turnSpansByStart(t, recorder), 1)
		require.Equal(t, second, turn.OpenToken())

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
	turn := newRunnerTurnSpan(tracer, nil, false)
	chat := database.Chat{ID: uuid.New()}
	firstTrigger := time.Now().Add(-time.Minute)
	secondTrigger := time.Now().Add(-10 * time.Second)

	_, first := turn.Ensure(t.Context(), chat, firstTrigger)
	_, second := turn.Ensure(t.Context(), chat, secondTrigger)
	canceled, cancel := context.WithCancel(t.Context())
	cancel()
	canceledCtx, stale := turn.Ensure(canceled, chat, firstTrigger)
	require.Zero(t, stale)
	require.False(t, trace.SpanContextFromContext(canceledCtx).IsValid())
	turn.Invalidate(stale, chatloop.TurnOutcomeError, xerrors.New("canceled"))
	turn.Settle(stale)
	require.Equal(t, second, turn.OpenToken())

	_, again := turn.Ensure(t.Context(), chat, secondTrigger)
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

// TestRunnerTurnSpanObservesOnlyCompletedTurns closes one turn with each
// outcome and checks that only the completed one is observed on the
// stage histogram.
func TestRunnerTurnSpanObservesOnlyCompletedTurns(t *testing.T) {
	t.Parallel()
	tracer, recorder, registry := newStageMetricsTracer(t)
	turn := newRunnerTurnSpan(tracer, nil, false)
	chat := database.Chat{ID: uuid.New()}
	base := time.Now().Add(-time.Hour)

	_, token := turn.Ensure(t.Context(), chat, base)
	turn.Complete(token)
	turn.Settle(token)
	for i, outcome := range []chatloop.TurnOutcome{chatloop.TurnOutcomeInterrupted, chatloop.TurnOutcomeError} {
		_, token = turn.Ensure(t.Context(), chat, base.Add(time.Duration(i+1)*time.Minute))
		turn.Invalidate(token, outcome, xerrors.New(string(outcome)))
		turn.Settle(token)
	}
	// Runner exit closes the last turn as abandoned.
	turn.Ensure(t.Context(), chat, base.Add(10*time.Minute))
	turn.End(nil)

	require.Len(t, turnSpansByStart(t, recorder), 4)
	require.Equal(t, uint64(1), stageObservationCount(t, registry, chatloop.StageChatTurn))
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

func TestStepAbandonsTurn(t *testing.T) {
	t.Parallel()

	canceled, cancel := context.WithCancel(t.Context())
	cancel()
	timedOut, cancelTimeout := context.WithCancelCause(t.Context())
	cancelTimeout(errTaskTimeout)
	expectedExit := errors.Join(errTaskExpectedExit, xerrors.New("generation fence mismatch"))

	for _, tc := range []struct {
		name string
		ctx  context.Context
		err  error
		want bool
	}{
		{name: "NoError", ctx: t.Context(), err: nil, want: false},
		{name: "ExpectedExit", ctx: t.Context(), err: expectedExit, want: true},
		{name: "RetryableExpectedExit", ctx: t.Context(), err: taskRetryableError{err: expectedExit}, want: false},
		{name: "Retryable", ctx: t.Context(), err: taskRetryableError{err: xerrors.New("transient")}, want: false},
		// The task runner retries a transition error that is neither
		// retryable nor an expected exit.
		{name: "TransitionError", ctx: t.Context(), err: normalizeTaskTransitionError(chatstate.ErrTransitionNotAllowed, "finish generation turn"), want: false},
		{name: "Error", ctx: t.Context(), err: xerrors.New("provider refused"), want: false},
		{name: "Canceled", ctx: canceled, err: errors.Join(errTaskExpectedExit, context.Canceled), want: false},
		{name: "TimedOut", ctx: timedOut, err: errors.Join(errTaskExpectedExit, context.Canceled), want: false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, tc.want, stepAbandonsTurn(tc.ctx, tc.err))
		})
	}
}

// TestFinishGenerationErrorIgnoresCanceledContext cancels the task
// context as soon as the finishing commit publishes its state change.
// The turn still closes as an error, because the committed FinishError
// decides the outcome.
func TestFinishGenerationErrorIgnoresCanceledContext(t *testing.T) {
	t.Parallel()
	f := newTaskTestFixture(t)
	chat := f.createRunningChat(t)
	workerID, runnerID := uuid.New(), uuid.New()
	acquired := f.acquireChat(t, chat.ID, workerID, runnerID)
	starter := newTestTaskStarter(t, f, newTaskSideEffectRecorder())
	tracer, recorder := newStageTestTracer(t)
	turn := newRunnerTurnSpan(tracer, nil, false)

	taskCtx, cancel := context.WithCancel(testutil.Context(t, testutil.WaitLong))
	defer cancel()
	f.pubsub.mu.Lock()
	f.pubsub.onPublish = func(channel string) {
		if channel == coderdpubsub.ChatStateUpdateChannel(chat.ID) {
			cancel()
		}
	}
	f.pubsub.mu.Unlock()

	_, token := turn.Ensure(taskCtx, acquired, acquired.CreatedAt)
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
	turn := newRunnerTurnSpan(tracer, nil, false)
	_, token := turn.Ensure(t.Context(), acquired, acquired.CreatedAt)
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
	require.Equal(t, codes.Error, turns[0].Status().Code)
	require.Equal(t, errChatInterrupted.Error(), turns[0].Status().Description)
	require.Zero(t, turn.OpenToken())
}

// turnCategorySeconds returns the turn_time_seconds_total value of a
// root chat's turns for category and outcome, zero without a series.
func turnCategorySeconds(t *testing.T, registry *prometheus.Registry, category chatloop.TurnCategory, outcome chatloop.TurnOutcome) float64 {
	t.Helper()
	return promhelp.MetricValue(t, registry, "coderd_chatd_turn_time_seconds_total", prometheus.Labels{
		"category":  string(category),
		"chat_kind": string(chatloop.ChatKindRoot),
		"outcome":   string(outcome),
	}).GetCounter().GetValue()
}

// turnOutcomeCount returns the turn_outcomes_total value of a root
// chat's turns for outcome, zero without a series.
func turnOutcomeCount(t *testing.T, registry *prometheus.Registry, outcome chatloop.TurnOutcome) float64 {
	t.Helper()
	return promhelp.MetricValue(t, registry, "coderd_chatd_turn_outcomes_total", prometheus.Labels{
		"chat_kind": string(chatloop.ChatKindRoot),
		"outcome":   string(outcome),
	}).GetCounter().GetValue()
}

func TestRunnerTurnSpanCountsFinishingStep(t *testing.T) {
	t.Parallel()
	clock := quartz.NewMock(t)
	tracer, recorder, registry := newStageMetricsTracer(t, chatloop.WithClock(clock))
	turn := newRunnerTurnSpan(tracer, nil, false)
	chat := database.Chat{ID: uuid.New()}

	turnCtx, token := turn.Ensure(t.Context(), chat, clock.Now().Add(-2*time.Second))
	// The finishing transition runs inside the step, so Complete
	// arrives while the step's stage is still open.
	stepCtx, step := tracer.Start(turnCtx, chatloop.StageGenerationStep)
	step.SetGenerationAction(chatloop.GenerationActionExecuteLocalTools)
	clock.Advance(3 * time.Second)
	_, commit := tracer.Start(stepCtx, chatloop.StageCommit)
	clock.Advance(time.Second)
	commit.End(nil)
	turn.Complete(token)
	require.Zero(t, turnOutcomeCount(t, registry, chatloop.TurnOutcomeCompleted), "the turn must stay open until the step ends")
	clock.Advance(time.Second)
	step.End(nil)
	turn.Settle(token)

	require.Equal(t, 1.0, turnOutcomeCount(t, registry, chatloop.TurnOutcomeCompleted))
	completed := func(category chatloop.TurnCategory) float64 {
		return turnCategorySeconds(t, registry, category, chatloop.TurnOutcomeCompleted)
	}
	require.Equal(t, 2.0, completed(chatloop.TurnCategoryScheduling))
	require.Equal(t, 4.0, completed(chatloop.TurnCategoryToolExecution))
	require.Equal(t, 1.0, completed(chatloop.TurnCategoryPersistence))
	require.Zero(t, completed(chatloop.TurnCategoryUnattributed))

	turns := turnSpansByStart(t, recorder)
	require.Len(t, turns, 1)
	for _, span := range recorder.Ended() {
		if chatloop.Stage(span.Name()) == chatloop.StageGenerationStep {
			require.False(t, span.EndTime().After(turns[0].EndTime()),
				"the step must end inside its turn span")
		}
	}

	// Settle closed the turn; the runner's End has nothing left.
	turn.End(nil)
	require.Len(t, turnSpansByStart(t, recorder), 1)
}

func TestRunnerTurnSpanRetryContinuesTurn(t *testing.T) {
	t.Parallel()
	clock := quartz.NewMock(t)
	tracer, recorder, registry := newStageMetricsTracer(t, chatloop.WithClock(clock))
	turn := newRunnerTurnSpan(tracer, nil, false)
	chat := database.Chat{ID: uuid.New()}
	triggerAt := clock.Now().Add(-time.Minute)

	// A retryable failure does not invalidate the turn; the task only
	// settles, which leaves an unfinished turn open.
	_, token := turn.Ensure(t.Context(), chat, triggerAt)
	turn.Settle(token)

	// The retried task runs the same prompt, so it continues the same
	// turn and records no second acquisition.
	_, retried := turn.Ensure(t.Context(), chat, triggerAt)
	require.Equal(t, token, retried)
	turn.Complete(retried)
	turn.Settle(retried)
	turn.End(nil)

	require.Len(t, turnSpansByStart(t, recorder), 1)
	require.Len(t, stageSpansByStart(t, recorder, chatloop.StageAcquisition), 1)
	require.Equal(t, 1.0, turnOutcomeCount(t, registry, chatloop.TurnOutcomeCompleted))
	require.Equal(t, 60.0, turnCategorySeconds(t, registry, chatloop.TurnCategoryScheduling, chatloop.TurnOutcomeCompleted))
}

func TestRunnerTurnSpanCountsOutcomes(t *testing.T) {
	t.Parallel()
	clock := quartz.NewMock(t)
	tracer, recorder, registry := newStageMetricsTracer(t, chatloop.WithClock(clock))
	turn := newRunnerTurnSpan(tracer, nil, false)
	chat := database.Chat{ID: uuid.New()}

	turnCtx, completed := turn.Ensure(t.Context(), chat, clock.Now())
	_, stream := tracer.Start(turnCtx, chatloop.StageStream)
	clock.Advance(time.Second)
	stream.End(nil)
	turn.Complete(completed)
	turn.Settle(completed)

	// An errored turn is counted as an error even though it was later
	// finished.
	_, errored := turn.Ensure(t.Context(), chat, clock.Now())
	clock.Advance(time.Second)
	turn.Invalidate(errored, chatloop.TurnOutcomeError, xerrors.New("provider refused"))
	turn.Complete(errored)
	turn.Settle(errored)

	// An interrupted turn is closed by the next Ensure, which counts it
	// once; the turn that Ensure opens is later counted as completed.
	_, interrupted := turn.Ensure(t.Context(), chat, clock.Now())
	clock.Advance(time.Second)
	turn.Invalidate(interrupted, chatloop.TurnOutcomeInterrupted, xerrors.Errorf("generation action: %w", context.Canceled))
	_, afterInterrupt := turn.Ensure(t.Context(), chat, clock.Now())
	require.NotEqual(t, interrupted, afterInterrupt)
	require.Equal(t, 1.0, turnOutcomeCount(t, registry, chatloop.TurnOutcomeInterrupted))
	clock.Advance(time.Second)
	turn.Complete(afterInterrupt)
	turn.Settle(afterInterrupt)

	// A turn that ends before it finished is abandoned, and keeps the
	// time of the step it ran.
	abandonedCtx, _ := turn.Ensure(t.Context(), chat, clock.Now())
	_, step := tracer.Start(abandonedCtx, chatloop.StageGenerationStep)
	clock.Advance(time.Second)
	step.End(nil)
	turn.End(xerrors.New("runner torn down"))

	for outcome, count := range map[chatloop.TurnOutcome]float64{
		chatloop.TurnOutcomeCompleted:   2,
		chatloop.TurnOutcomeError:       1,
		chatloop.TurnOutcomeInterrupted: 1,
		chatloop.TurnOutcomeAbandoned:   1,
	} {
		require.Equal(t, count, turnOutcomeCount(t, registry, outcome), outcome)
	}
	require.Len(t, turnSpansByStart(t, recorder), 5, "outcomes sum to closed turns")
	// Every turn lasted one second and recorded its partition.
	require.Equal(t, 1.0, turnCategorySeconds(t, registry, chatloop.TurnCategoryStreaming, chatloop.TurnOutcomeCompleted))
	require.Equal(t, 1.0, turnCategorySeconds(t, registry, chatloop.TurnCategoryUnattributed, chatloop.TurnOutcomeCompleted))
	require.Equal(t, 1.0, turnCategorySeconds(t, registry, chatloop.TurnCategoryUnattributed, chatloop.TurnOutcomeError))
	require.Equal(t, 1.0, turnCategorySeconds(t, registry, chatloop.TurnCategoryUnattributed, chatloop.TurnOutcomeInterrupted))
	require.Equal(t, 1.0, turnCategorySeconds(t, registry, chatloop.TurnCategoryChatdOverhead, chatloop.TurnOutcomeAbandoned))
	require.Zero(t, turnCategorySeconds(t, registry, chatloop.TurnCategoryUnattributed, chatloop.TurnOutcomeAbandoned))
	require.Zero(t, anomalyCount(t, registry, chatloop.StageAnomalyNonPositiveTurn))
}

func TestRunnerTurnSpanLabelsSubagentTurns(t *testing.T) {
	t.Parallel()
	clock := quartz.NewMock(t)
	tracer, _, registry := newStageMetricsTracer(t, chatloop.WithClock(clock))
	turn := newRunnerTurnSpan(tracer, nil, false)
	chat := database.Chat{ID: uuid.New(), ParentChatID: uuid.NullUUID{UUID: uuid.New(), Valid: true}}

	turnCtx, token := turn.Ensure(t.Context(), chat, clock.Now())
	_, stream := tracer.Start(turnCtx, chatloop.StageStream)
	clock.Advance(time.Second)
	stream.End(nil)
	turn.Complete(token)
	turn.Settle(token)

	subagent := string(chatloop.ChatKindSubagent)
	completed := string(chatloop.TurnOutcomeCompleted)
	require.Equal(t, 1.0, promhelp.MetricValue(t, registry, "coderd_chatd_turn_outcomes_total", prometheus.Labels{
		"chat_kind": subagent,
		"outcome":   completed,
	}).GetCounter().GetValue())
	require.Equal(t, 1.0, promhelp.MetricValue(t, registry, "coderd_chatd_turn_time_seconds_total", prometheus.Labels{
		"category":  string(chatloop.TurnCategoryStreaming),
		"chat_kind": subagent,
		"outcome":   completed,
	}).GetCounter().GetValue())
	require.Zero(t, turnOutcomeCount(t, registry, chatloop.TurnOutcomeCompleted), "no root series")
}
