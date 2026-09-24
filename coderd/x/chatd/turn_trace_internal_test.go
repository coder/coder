package chatd //nolint:testpackage // Tests unexported turn span internals.

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"slices"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/sqlc-dev/pqtype"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"go.opentelemetry.io/otel/trace"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/x/chatd/chatloop"
	"github.com/coder/coder/v2/coderd/x/chatd/chatprompt"
	"github.com/coder/coder/v2/codersdk"
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

func TestRunnerTurnSpanParentsRecordedStages(t *testing.T) {
	t.Parallel()
	tracer, recorder := newStageTestTracer(t)
	turn := newRunnerTurnSpan(tracer, nil, false)
	chat := database.Chat{ID: uuid.New()}

	turnCtx, _ := turn.Ensure(t.Context(), chat, time.Now().Add(-time.Second))
	_, step := tracer.Start(turnCtx, chatloop.StageGenerationStep)
	step.End(nil)

	// The step has ended, so the stage is recorded on the turn context
	// rather than the step context.
	tracer.Record(turnCtx, chatloop.StageCommit, chatloop.StageModel{},
		time.Now().Add(-500*time.Millisecond), time.Now(), nil)
	turn.End(nil)

	var commit, chatTurn, generationStep sdktrace.ReadOnlySpan
	for _, span := range recorder.Ended() {
		switch chatloop.Stage(span.Name()) {
		case chatloop.StageCommit:
			commit = span
		case chatloop.StageChatTurn:
			chatTurn = span
		case chatloop.StageGenerationStep:
			generationStep = span
		}
	}
	require.NotNil(t, commit)
	require.NotNil(t, chatTurn)
	require.NotNil(t, generationStep)
	require.Equal(t, chatTurn.SpanContext().SpanID(), commit.Parent().SpanID())
	require.NotEqual(t, generationStep.SpanContext().SpanID(), commit.Parent().SpanID())
}

func TestRunnerTurnSpanEnsureOpensTurnPerPrompt(t *testing.T) {
	t.Parallel()
	tracer, recorder := newStageTestTracer(t)
	turn := newRunnerTurnSpan(tracer, nil, false)
	chat := database.Chat{ID: uuid.New()}

	firstTrigger := time.Now().Add(-2 * time.Minute)
	_, first := turn.Ensure(t.Context(), chat, firstTrigger)
	// Another step for the same trigger reuses the open turn.
	_, again := turn.Ensure(t.Context(), chat, firstTrigger)
	require.Equal(t, first, again)
	require.Len(t, turnSpansByStart(t, recorder), 0)

	turn.Complete(first)
	turn.Settle(first)
	secondTrigger := time.Now()
	_, second := turn.Ensure(t.Context(), chat, secondTrigger)
	require.NotEqual(t, first, second)
	turn.End(nil)

	turns := turnSpansByStart(t, recorder)
	require.Len(t, turns, 2)
	require.Equal(t, firstTrigger.UTC(), turns[0].StartTime().UTC())
	require.Equal(t, secondTrigger.UTC(), turns[1].StartTime().UTC())
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
	tracer, recorder, registry := newStageMetricsTracer(t)
	turn := newRunnerTurnSpan(tracer, nil, false)
	chat := database.Chat{ID: uuid.New()}
	staleAnchors := func() float64 {
		return anomalyCount(t, registry, chatloop.StageAnomalyStaleAnchor)
	}

	firstTrigger := time.Now().Add(-time.Minute)
	_, first := turn.Ensure(t.Context(), chat, firstTrigger)
	turn.Invalidate(first, chatloop.TurnOutcomeError, xerrors.New("provider refused"))
	turn.Settle(first)
	require.Zero(t, staleAnchors())

	// The same prompt reopening the turn after an invalidation starts
	// at now and records no second acquisition.
	beforeReopen := time.Now()
	_, reopened := turn.Ensure(t.Context(), chat, firstTrigger)
	require.NotEqual(t, first, reopened)
	require.Equal(t, float64(1), staleAnchors())
	turn.Complete(reopened)
	turn.Settle(reopened)

	// So does a trigger before the previous anchor.
	_, older := turn.Ensure(t.Context(), chat, firstTrigger.Add(-30*time.Second))
	require.NotEqual(t, reopened, older)
	require.Equal(t, float64(2), staleAnchors())
	turn.Complete(older)
	turn.Settle(older)

	// A trigger after the previous anchor is kept.
	thirdTrigger := time.Now()
	_, third := turn.Ensure(t.Context(), chat, thirdTrigger)
	require.NotEqual(t, older, third)
	require.Equal(t, float64(2), staleAnchors())
	turn.End(nil)

	turns := turnSpansByStart(t, recorder)
	require.Len(t, turns, 4)
	require.Equal(t, firstTrigger.UTC(), turns[0].StartTime().UTC())
	require.False(t, turns[1].StartTime().Before(beforeReopen))
	require.False(t, turns[1].StartTime().Before(turns[0].EndTime()))
	require.False(t, turns[2].StartTime().Before(turns[1].StartTime()))
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
	tracer, recorder, registry := newStageMetricsTracer(t)
	turn := newRunnerTurnSpan(tracer, nil, true)
	chat := database.Chat{ID: uuid.New()}

	// The previous owner already ran part of this turn.
	staleTrigger := time.Now().Add(-time.Minute)
	beforeEnsure := time.Now()
	_, first := turn.Ensure(t.Context(), chat, staleTrigger)
	_, again := turn.Ensure(t.Context(), chat, staleTrigger)
	require.Equal(t, first, again)
	turn.Complete(first)
	turn.Settle(first)

	nextTrigger := time.Now()
	_, second := turn.Ensure(t.Context(), chat, nextTrigger)
	require.NotEqual(t, first, second)
	turn.End(nil)

	require.Zero(t, anomalyCount(t, registry, chatloop.StageAnomalyStaleAnchor))
	turns := turnSpansByStart(t, recorder)
	require.Len(t, turns, 2)
	require.False(t, turns[0].StartTime().Before(beforeEnsure))
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

func TestRunnerTurnSpanEnsureRotatesInvalidatedTurn(t *testing.T) {
	t.Parallel()
	tracer, recorder := newStageTestTracer(t)
	turn := newRunnerTurnSpan(tracer, nil, false)
	chat := database.Chat{ID: uuid.New()}

	firstTrigger := time.Now().Add(-time.Minute)
	_, first := turn.Ensure(t.Context(), chat, firstTrigger)
	failure := xerrors.New("provider refused")
	turn.Invalidate(first, chatloop.TurnOutcomeError, failure)

	secondTrigger := time.Now()
	_, second := turn.Ensure(t.Context(), chat, secondTrigger)
	require.NotEqual(t, first, second)
	turn.Complete(second)
	turn.Settle(second)

	turns := turnSpansByStart(t, recorder)
	require.Len(t, turns, 2)
	require.Equal(t, codes.Error, turns[0].Status().Code)
	require.Equal(t, failure.Error(), turns[0].Status().Description)
	outcome, _ := spanAttribute(t, turns[0], chatloop.AttrTurnOutcome)
	require.Equal(t, chatloop.TurnOutcomeError, chatloop.TurnOutcome(outcome.AsString()))
	require.Equal(t, secondTrigger.UTC(), turns[1].StartTime().UTC())
	require.Equal(t, codes.Unset, turns[1].Status().Code)
	outcome, _ = spanAttribute(t, turns[1], chatloop.AttrTurnOutcome)
	require.Equal(t, chatloop.TurnOutcomeCompleted, chatloop.TurnOutcome(outcome.AsString()))
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

	t.Run("Completed", func(t *testing.T) {
		t.Parallel()
		tracer, recorder := newStageTestTracer(t)
		turn := newRunnerTurnSpan(tracer, nil, false)
		_, token := turn.Ensure(t.Context(), database.Chat{ID: uuid.New()}, time.Now())
		turn.Complete(token)
		turn.Settle(token)

		span := turnSpanFor(t, recorder)
		require.Equal(t, codes.Unset, span.Status().Code)
		require.Equal(t, chatloop.TurnOutcomeCompleted, outcome(t, span))
	})

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
		// A later abandon task on the runner does not extend the span.
		afterSettle := time.Now()
		turn.End(nil)
		require.False(t, span.EndTime().After(afterSettle))
	})

	t.Run("SettleLeavesRunningTurnOpen", func(t *testing.T) {
		t.Parallel()
		tracer, recorder := newStageTestTracer(t)
		turn := newRunnerTurnSpan(tracer, nil, false)
		_, token := turn.Ensure(t.Context(), database.Chat{ID: uuid.New()}, time.Now())
		turn.Settle(token)
		require.Empty(t, turnSpansByStart(t, recorder))
		turn.End(nil)
		require.Equal(t, chatloop.TurnOutcomeAbandoned, outcome(t, turnSpanFor(t, recorder)))
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

	t.Run("Interrupted", func(t *testing.T) {
		t.Parallel()
		tracer, recorder := newStageTestTracer(t)
		turn := newRunnerTurnSpan(tracer, nil, false)
		_, token := turn.Ensure(t.Context(), database.Chat{ID: uuid.New()}, time.Now())
		turn.Invalidate(token, chatloop.TurnOutcomeInterrupted, xerrors.Errorf("generation action: %w", context.Canceled))
		turn.End(nil)

		span := turnSpanFor(t, recorder)
		require.Equal(t, codes.Error, span.Status().Code)
		require.Equal(t, chatloop.TurnOutcomeInterrupted, outcome(t, span))
	})

	t.Run("AbandonedOnEndOfUnfinishedTurn", func(t *testing.T) {
		t.Parallel()
		tracer, recorder := newStageTestTracer(t)
		turn := newRunnerTurnSpan(tracer, nil, false)
		turn.Ensure(t.Context(), database.Chat{ID: uuid.New()}, time.Now())
		turn.End(nil)

		span := turnSpanFor(t, recorder)
		require.Equal(t, codes.Unset, span.Status().Code)
		require.Equal(t, chatloop.TurnOutcomeAbandoned, outcome(t, span))
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
}

func TestTurnContinues(t *testing.T) {
	t.Parallel()

	t.Run("Retryable", func(t *testing.T) {
		t.Parallel()
		require.True(t, turnContinues(t.Context(), taskRetryableError{err: xerrors.New("transient")}))
	})
	t.Run("AttemptTimeout", func(t *testing.T) {
		t.Parallel()
		ctx, cancel := context.WithCancelCause(t.Context())
		cancel(errTaskTimeout)
		require.True(t, turnContinues(ctx, xerrors.Errorf("stream: %w", context.Canceled)))
	})
	t.Run("Interrupted", func(t *testing.T) {
		t.Parallel()
		ctx, cancel := context.WithCancel(t.Context())
		cancel()
		require.False(t, turnContinues(ctx, xerrors.Errorf("stream: %w", context.Canceled)))
	})
	t.Run("Error", func(t *testing.T) {
		t.Parallel()
		require.False(t, turnContinues(t.Context(), xerrors.New("provider refused")))
	})
}

func TestTurnOutcomeForError(t *testing.T) {
	t.Parallel()

	t.Run("Interrupted", func(t *testing.T) {
		t.Parallel()
		ctx, cancel := context.WithCancel(t.Context())
		cancel()
		err := errors.Join(errTaskExpectedExit, context.Canceled)
		require.Equal(t, chatloop.TurnOutcomeInterrupted, turnOutcomeForError(ctx, err))
	})
	t.Run("Abandoned", func(t *testing.T) {
		t.Parallel()
		err := errors.Join(errTaskExpectedExit, xerrors.New("generation fence mismatch"))
		require.Equal(t, chatloop.TurnOutcomeAbandoned, turnOutcomeForError(t.Context(), err))
	})
	t.Run("Error", func(t *testing.T) {
		t.Parallel()
		require.Equal(t, chatloop.TurnOutcomeError, turnOutcomeForError(t.Context(), xerrors.New("provider refused")))
	})
}
