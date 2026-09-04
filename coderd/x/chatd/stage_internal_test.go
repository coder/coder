package chatd //nolint:testpackage // Tests unexported stage instrumentation internals.

import (
	"context"
	"database/sql"
	"errors"
	"io"
	"net/http"
	"sort"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"charm.land/fantasy"
	"github.com/google/uuid"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"go.opentelemetry.io/otel/trace"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/x/chatd/chatloop"
)

type stubRoundTripper struct {
	status int
	err    error
}

func (s stubRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	if s.err != nil {
		return nil, s.err
	}
	return &http.Response{
		StatusCode: s.status,
		Body:       io.NopCloser(strings.NewReader("")),
		Request:    req,
	}, nil
}

// spanAttribute returns the value of the span attribute named key.
func spanAttribute(t *testing.T, span sdktrace.ReadOnlySpan, key string) (attribute.Value, bool) {
	t.Helper()
	for _, attr := range span.Attributes() {
		if string(attr.Key) == key {
			return attr.Value, true
		}
	}
	return attribute.Value{}, false
}

func TestStageSpanRoundTripper(t *testing.T) {
	t.Parallel()

	model := chatloop.StageModel{ProviderType: "bedrock", Model: "claude-sonnet-4-5", Effort: "medium"}
	tests := []struct {
		name           string
		base           stubRoundTripper
		turnScoped     bool
		wantStatusCode codes.Code
		wantAttribute  bool
		wantErr        bool
	}{
		{
			name:           "success",
			base:           stubRoundTripper{status: http.StatusOK},
			wantStatusCode: codes.Unset,
			wantAttribute:  true,
		},
		{
			name:           "turn scoped",
			base:           stubRoundTripper{status: http.StatusOK},
			turnScoped:     true,
			wantStatusCode: codes.Unset,
			wantAttribute:  true,
		},
		{
			name:           "client error",
			base:           stubRoundTripper{status: http.StatusTooManyRequests},
			wantStatusCode: codes.Error,
			wantAttribute:  true,
		},
		{
			name:           "transport error",
			base:           stubRoundTripper{err: xerrors.New("dial failed")},
			wantStatusCode: codes.Error,
			wantErr:        true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			tracer, recorder, _ := newStageMetricsTracer(t)
			transport := &stageSpanRoundTripper{base: test.base, stages: tracer, model: model}

			ctx := t.Context()
			wantScope := chatloop.ScopeBackground
			if test.turnScoped {
				var turn *chatloop.StageSpan
				ctx, turn = tracer.StartRoot(ctx, chatloop.StageChatTurn, nil)
				defer turn.End(nil)
				wantScope = chatloop.ScopeTurn
			}
			req, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://provider.example/v1/messages", nil)
			require.NoError(t, err)
			resp, err := transport.RoundTrip(req)
			if test.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
				require.NotNil(t, resp)
				require.Equal(t, test.base.status, resp.StatusCode)
				require.NoError(t, resp.Body.Close())
			}

			ended := recorder.Ended()
			require.Len(t, ended, 1)
			span := ended[0]
			require.Equal(t, string(chatloop.StageProviderAttempt), span.Name())
			require.Equal(t, test.wantStatusCode, span.Status().Code)
			statusCode, sawStatusCode := spanAttribute(t, span, chatloop.AttrHTTPStatusCode)
			require.Equal(t, test.wantAttribute, sawStatusCode)
			if sawStatusCode {
				require.Equal(t, int64(test.base.status), statusCode.AsInt64())
			}
			require.Contains(t, span.Attributes(), attribute.String(chatloop.AttrScope, string(wantScope)))
			require.Contains(t, span.Attributes(), attribute.String(chatloop.AttrHTTPMethod, http.MethodPost))
			require.Contains(t, span.Attributes(), attribute.String(chatloop.AttrHTTPHost, "provider.example"))
			require.Contains(t, span.Attributes(), attribute.String(chatloop.AttrProviderType, model.ProviderType))
			require.Contains(t, span.Attributes(), attribute.String(chatloop.AttrModel, model.Model))
			require.Contains(t, span.Attributes(), attribute.String(chatloop.AttrReasoningEffort, model.Effort))
		})
	}
}

// TestNewModelTracesEachProviderAttempt drives requests through a
// model built by newModel and checks that every HTTP round trip
// produces its own provider_attempt span. The provider SDK is built
// without retries, so a refused call surfaces as an error and the
// retry is a second call.
func TestNewModelTracesEachProviderAttempt(t *testing.T) {
	t.Parallel()

	tracer, recorder, _ := newStageMetricsTracer(t)
	var attempts atomic.Int32
	factory := &aibridgeTestFactory{rt: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		if attempts.Add(1) == 1 {
			return &http.Response{
				StatusCode: http.StatusTooManyRequests,
				Body:       io.NopCloser(strings.NewReader(`{"error":{"message":"slow down"}}`)),
				Request:    req,
			}, nil
		}
		body := `{"id":"resp_test","object":"response","created_at":0,"status":"completed","model":"gpt-4","output":[{"id":"msg_test","type":"message","role":"assistant","content":[{"type":"output_text","text":"hello"}]}],"usage":{"input_tokens":1,"output_tokens":1,"total_tokens":2}}`
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": []string{"application/json"}},
			Body:       io.NopCloser(strings.NewReader(body)),
			Request:    req,
		}, nil
	})}
	server := &Server{
		aibridgeTransportFactory: aibridgeTestFactoryPointer(factory),
		stages:                   tracer,
	}
	provider := aibridgeTestAIProvider(uuid.New(), "primary-openai", database.AIProviderTypeOpenai)
	route := newAIGatewayModelRoute(provider, string(provider.Type), aiGatewayProviderAuth{})
	stageModel := chatloop.StageModel{ProviderType: "openai", Model: "gpt-4", Effort: "low"}

	model, err := server.newModel(t.Context(),
		aibridgeTestRequest(database.Chat{ID: uuid.New(), OwnerID: uuid.New()}, "gpt-4"),
		route, modelBuildOptions{ActiveAPIKeyID: uuid.NewString(), StageModel: stageModel})
	require.NoError(t, err)
	call := fantasy.Call{Prompt: []fantasy.Message{{
		Role: fantasy.MessageRoleUser, Content: []fantasy.MessagePart{fantasy.TextPart{Text: "hello"}},
	}}}
	_, err = model.LanguageModel().Generate(t.Context(), call)
	require.Error(t, err)
	_, err = model.LanguageModel().Generate(t.Context(), call)
	require.NoError(t, err)
	require.Equal(t, int32(2), attempts.Load())

	ended := recorder.Ended()
	require.Len(t, ended, 2)
	for _, span := range ended {
		require.Equal(t, string(chatloop.StageProviderAttempt), span.Name())
		require.Contains(t, span.Attributes(), attribute.String(chatloop.AttrProviderType, stageModel.ProviderType))
		require.Contains(t, span.Attributes(), attribute.String(chatloop.AttrModel, stageModel.Model))
		require.Contains(t, span.Attributes(), attribute.String(chatloop.AttrReasoningEffort, stageModel.Effort))
	}
	first, _ := spanAttribute(t, ended[0], chatloop.AttrHTTPStatusCode)
	second, _ := spanAttribute(t, ended[1], chatloop.AttrHTTPStatusCode)
	require.Equal(t, int64(http.StatusTooManyRequests), first.AsInt64())
	require.Equal(t, codes.Error, ended[0].Status().Code)
	require.Equal(t, int64(http.StatusOK), second.AsInt64())
	require.Equal(t, codes.Unset, ended[1].Status().Code)
}

// newStageMetricsTracer returns a stage tracer writing spans into an
// in-memory recorder and metrics into a private registry.
func newStageMetricsTracer(t *testing.T) (*chatloop.StageTracer, *tracetest.SpanRecorder, *prometheus.Registry) {
	t.Helper()
	recorder := tracetest.NewSpanRecorder()
	provider := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(recorder))
	t.Cleanup(func() {
		require.NoError(t, provider.Shutdown(context.Background()))
	})
	registry := prometheus.NewRegistry()
	return chatloop.NewStageTracer(provider, chatloop.NewMetrics(registry)), recorder, registry
}

// turnStageTurns returns, per stage, the number of turns that
// reported the stage on coderd_chatd_turn_stage_seconds.
func turnStageTurns(t *testing.T, registry *prometheus.Registry) map[chatloop.Stage]uint64 {
	t.Helper()
	families, err := registry.Gather()
	require.NoError(t, err)
	out := map[chatloop.Stage]uint64{}
	for _, family := range families {
		if family.GetName() != "coderd_chatd_turn_stage_seconds" {
			continue
		}
		for _, metric := range family.GetMetric() {
			var stage chatloop.Stage
			for _, label := range metric.GetLabel() {
				if label.GetName() == "stage" {
					stage = chatloop.Stage(label.GetValue())
				}
			}
			out[stage] = metric.GetHistogram().GetSampleCount()
		}
	}
	return out
}

// anomalyCount returns the stage anomaly count recorded for reason.
func anomalyCount(t *testing.T, registry *prometheus.Registry, reason string) float64 {
	t.Helper()
	families, err := registry.Gather()
	require.NoError(t, err)
	for _, family := range families {
		if family.GetName() != "coderd_chatd_stage_anomalies_total" {
			continue
		}
		for _, metric := range family.GetMetric() {
			for _, label := range metric.GetLabel() {
				if label.GetName() == "reason" && label.GetValue() == reason {
					return metric.GetCounter().GetValue()
				}
			}
		}
	}
	return 0
}

// emittedTurns returns how many turns reported their category
// partition, summed across label sets.
func emittedTurns(t *testing.T, registry *prometheus.Registry) uint64 {
	t.Helper()
	families, err := registry.Gather()
	require.NoError(t, err)
	var total uint64
	for _, family := range families {
		if family.GetName() != "coderd_chatd_turn_time_seconds" {
			continue
		}
		for _, metric := range family.GetMetric() {
			for _, label := range metric.GetLabel() {
				if label.GetName() == "category" && label.GetValue() == string(chatloop.CategoryUnattributed) {
					total += metric.GetHistogram().GetSampleCount()
				}
			}
		}
	}
	return total
}

func TestRunnerTurnSpanStartsAtTriggerMessage(t *testing.T) {
	t.Parallel()
	tracer, recorder, _ := newStageMetricsTracer(t)
	turn := newRunnerTurnSpan(tracer, nil)
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
	tracer, recorder, _ := newStageMetricsTracer(t)
	orgID := uuid.New()
	var resolved []uuid.UUID
	turn := newRunnerTurnSpan(tracer, func(_ context.Context, id uuid.UUID) string {
		resolved = append(resolved, id)
		return "acme"
	})
	chat := database.Chat{
		ID:             uuid.New(),
		OrganizationID: orgID,
		ParentChatID:   uuid.NullUUID{UUID: uuid.New(), Valid: true},
	}

	turnCtx, _ := turn.Ensure(t.Context(), chat, time.Now().Add(-time.Second))
	_, step := tracer.Start(turnCtx, chatloop.StageGenerationStep)
	step.End(nil)
	start := time.Now().Add(-500 * time.Millisecond)
	tracer.Record(turn.Context(turnCtx), chatloop.StageCommit, chatloop.StageModel{}, start, time.Now(), nil)
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
	tracer, recorder, _ := newStageMetricsTracer(t)
	turn := newRunnerTurnSpan(tracer, nil)
	chat := database.Chat{ID: uuid.New()}

	turnCtx, _ := turn.Ensure(t.Context(), chat, time.Now().Add(-time.Second))
	stepCtx, step := tracer.Start(turnCtx, chatloop.StageGenerationStep)
	step.End(nil)

	// The step has ended, so the queue wait is recorded on turn.Context
	// rather than the step context.
	tracer.Record(turn.Context(stepCtx), chatloop.StageQueueWait, chatloop.StageModel{},
		time.Now().Add(-500*time.Millisecond), time.Now(), nil)
	turn.End(nil)

	var queueWait, chatTurn, generationStep sdktrace.ReadOnlySpan
	for _, span := range recorder.Ended() {
		switch chatloop.Stage(span.Name()) {
		case chatloop.StageQueueWait:
			queueWait = span
		case chatloop.StageChatTurn:
			chatTurn = span
		case chatloop.StageGenerationStep:
			generationStep = span
		}
	}
	require.NotNil(t, queueWait)
	require.NotNil(t, chatTurn)
	require.NotNil(t, generationStep)
	require.Equal(t, chatTurn.SpanContext().SpanID(), queueWait.Parent().SpanID())
	require.NotEqual(t, generationStep.SpanContext().SpanID(), queueWait.Parent().SpanID())
}

func TestServerRecordQueueWaitIsStandalone(t *testing.T) {
	t.Parallel()
	tracer, recorder, _ := newStageMetricsTracer(t)
	server := &Server{stages: tracer}

	// The promoting request has its own span, which the queue wait must
	// not join.
	requestCtx, requestSpan := tracer.Start(t.Context(), chatloop.StageCommit)
	queuedAt := time.Now().Add(-30 * time.Second)
	promotedAt := queuedAt.Add(20 * time.Second)
	server.recordQueueWait(requestCtx, uuid.New(), chatloop.ChatKindSubagent, "acme", queuedAt, promotedAt)
	requestSpan.End(nil)

	var queueWait, request sdktrace.ReadOnlySpan
	for _, span := range recorder.Ended() {
		switch chatloop.Stage(span.Name()) {
		case chatloop.StageQueueWait:
			queueWait = span
		case chatloop.StageCommit:
			request = span
		}
	}
	require.NotNil(t, queueWait)
	require.NotNil(t, request)
	require.False(t, queueWait.Parent().IsValid())
	require.NotEqual(t, request.SpanContext().TraceID(), queueWait.SpanContext().TraceID())
	require.Equal(t, queuedAt.UTC(), queueWait.StartTime().UTC())
	require.Equal(t, promotedAt.UTC(), queueWait.EndTime().UTC())
	require.Contains(t, queueWait.Attributes(),
		attribute.String(chatloop.AttrScope, string(chatloop.ScopeTurn)))
	require.Contains(t, queueWait.Attributes(),
		attribute.String(chatloop.AttrChatKind, string(chatloop.ChatKindSubagent)))
	require.Contains(t, queueWait.Attributes(),
		attribute.String(chatloop.AttrOrganizationName, "acme"))
}

func TestServerInflightContextIsBackgroundScoped(t *testing.T) {
	t.Parallel()
	tracer, recorder, _ := newStageMetricsTracer(t)
	serverCtx, serverCancel := context.WithCancel(context.Background())
	t.Cleanup(serverCancel)
	server := &Server{ctx: serverCtx, stages: tracer}

	chat := database.Chat{ID: uuid.New()}
	turn := newRunnerTurnSpan(tracer, nil)
	turnCtx, _ := turn.Ensure(t.Context(), chat, time.Now().Add(-time.Second))
	inflightCtx, stop := server.inflightChatContext(turnCtx, chat)
	t.Cleanup(stop)

	_, span := tracer.Start(inflightCtx, chatloop.StageGenerationStep)
	span.End(nil)
	turn.End(nil)

	var step sdktrace.ReadOnlySpan
	for _, ended := range recorder.Ended() {
		if chatloop.Stage(ended.Name()) == chatloop.StageGenerationStep {
			step = ended
		}
	}
	require.NotNil(t, step, "no %s span was recorded", chatloop.StageGenerationStep)
	require.False(t, step.Parent().IsValid())
	require.Contains(t, step.Attributes(),
		attribute.String(chatloop.AttrScope, string(chatloop.ScopeBackground)))
	require.Contains(t, step.Attributes(),
		attribute.String(chatloop.AttrChatKind, string(chatloop.ChatKindRoot)))
}

// turnSpansByStart returns the chat_turn spans the recorder saw,
// ordered by start time.
func turnSpansByStart(t *testing.T, recorder *tracetest.SpanRecorder) []sdktrace.ReadOnlySpan {
	t.Helper()
	var turns []sdktrace.ReadOnlySpan
	for _, span := range recorder.Ended() {
		if chatloop.Stage(span.Name()) == chatloop.StageChatTurn {
			turns = append(turns, span)
		}
	}
	sort.Slice(turns, func(i, j int) bool {
		return turns[i].StartTime().Before(turns[j].StartTime())
	})
	return turns
}

func TestRunnerTurnSpanEndOfUnfinishedTurnDropsAccounting(t *testing.T) {
	t.Parallel()
	tracer, recorder, registry := newStageMetricsTracer(t)
	turn := newRunnerTurnSpan(tracer, nil)
	chat := database.Chat{ID: uuid.New()}

	turnCtx, _ := turn.Ensure(t.Context(), chat, time.Now().Add(-time.Minute))
	_, step := tracer.Start(turnCtx, chatloop.StageGenerationStep)
	step.End(nil)
	turn.End(xerrors.New("runner torn down"))

	require.Len(t, turnSpansByStart(t, recorder), 1)
	require.EqualValues(t, 0, emittedTurns(t, registry))
	require.Empty(t, turnStageTurns(t, registry))
	require.Equal(t, map[chatloop.TurnOutcome]float64{
		chatloop.TurnOutcomeAbandoned: 1,
	}, turnOutcomeCounts(t, registry))
}

func TestRunnerTurnSpanCountsFinishingStep(t *testing.T) {
	t.Parallel()
	tracer, recorder, registry := newStageMetricsTracer(t)
	turn := newRunnerTurnSpan(tracer, nil)
	chat := database.Chat{ID: uuid.New()}

	turnCtx, token := turn.Ensure(t.Context(), chat, time.Now().Add(-time.Minute))
	// The finishing transition runs inside the step, so Complete
	// arrives while the step's stage is still open.
	_, step := tracer.Start(turnCtx, chatloop.StageGenerationStep)
	turn.Complete(token, time.Time{})
	require.EqualValues(t, 0, emittedTurns(t, registry), "the turn must stay open until the step ends")
	step.End(nil)
	turn.Settle(t.Context(), token)

	require.EqualValues(t, 1, emittedTurns(t, registry))
	require.EqualValues(t, 1, turnStageTurns(t, registry)[chatloop.StageGenerationStep])

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

func TestRunnerTurnSpanSettleRotatesOnPromotion(t *testing.T) {
	t.Parallel()
	tracer, recorder, registry := newStageMetricsTracer(t)
	turn := newRunnerTurnSpan(tracer, nil)
	chat := database.Chat{ID: uuid.New()}

	turnCtx, token := turn.Ensure(t.Context(), chat, time.Now().Add(-time.Minute))
	_, step := tracer.Start(turnCtx, chatloop.StageGenerationStep)
	queuedAt := time.Now().Add(-30 * time.Second)
	turn.Complete(token, queuedAt)
	promotedBy := time.Now()
	step.End(nil)
	turn.Settle(t.Context(), token)

	require.EqualValues(t, 1, emittedTurns(t, registry))
	require.EqualValues(t, 1, turnStageTurns(t, registry)[chatloop.StageGenerationStep])

	// The next turn is open, anchored at the promoted message.
	_, nextToken := turn.Ensure(t.Context(), chat, queuedAt)
	require.NotEqual(t, token, nextToken)
	turn.End(nil)

	turns := turnSpansByStart(t, recorder)
	require.Len(t, turns, 2)
	require.Equal(t, queuedAt.UTC(), turns[1].StartTime().UTC())
	require.NotEqual(t, turns[0].SpanContext().TraceID(), turns[1].SpanContext().TraceID())
	require.Contains(t, turns[1].Attributes(),
		attribute.String(chatloop.AttrChatKind, string(chatloop.ChatKindRoot)))

	// The promoted message's wait belongs to the turn it opens and ends
	// when the promotion happened, not when the turn settled.
	var queueWait sdktrace.ReadOnlySpan
	for _, span := range recorder.Ended() {
		if chatloop.Stage(span.Name()) == chatloop.StageQueueWait {
			queueWait = span
		}
	}
	require.NotNil(t, queueWait)
	require.Equal(t, turns[1].SpanContext().SpanID(), queueWait.Parent().SpanID())
	require.Equal(t, queuedAt.UTC(), queueWait.StartTime().UTC())
	require.False(t, queueWait.EndTime().After(promotedBy))

	// The rotated turn's head is the queue wait, so it records no
	// acquisition: both windows start at the same instant and both
	// would count as scheduling time.
	var acquisitions int
	for _, span := range recorder.Ended() {
		if chatloop.Stage(span.Name()) == chatloop.StageAcquisition {
			acquisitions++
			require.Equal(t, turns[0].SpanContext().SpanID(), span.Parent().SpanID())
		}
	}
	require.Equal(t, 1, acquisitions)
}

func TestRunnerTurnSpanInvalidateAfterPromotionDropsFinishedTurn(t *testing.T) {
	t.Parallel()
	tracer, recorder, registry := newStageMetricsTracer(t)
	turn := newRunnerTurnSpan(tracer, nil)
	chat := database.Chat{ID: uuid.New()}

	_, token := turn.Ensure(t.Context(), chat, time.Now().Add(-time.Minute))
	queuedAt := time.Now().Add(-30 * time.Second)
	turn.Complete(token, queuedAt)
	// Post-commit work of the finished turn failed. The failure belongs
	// to the finished turn, not to the one the promotion opens.
	turn.Invalidate(token, chatloop.TurnOutcomeError, xerrors.New("step failed"))
	turn.Settle(t.Context(), token)
	require.EqualValues(t, 0, emittedTurns(t, registry))

	_, nextToken := turn.Ensure(t.Context(), chat, queuedAt)
	turn.Complete(nextToken, time.Time{})
	turn.Settle(t.Context(), nextToken)
	require.EqualValues(t, 1, emittedTurns(t, registry))
	require.Len(t, turnSpansByStart(t, recorder), 2)
}

func TestRunnerTurnSpanIgnoresStaleToken(t *testing.T) {
	t.Parallel()
	tracer, _, registry := newStageMetricsTracer(t)
	turn := newRunnerTurnSpan(tracer, nil)
	chat := database.Chat{ID: uuid.New()}

	_, first := turn.Ensure(t.Context(), chat, time.Now().Add(-time.Minute))
	turn.Complete(first, time.Time{})
	// A task for the next prompt arrives before the finishing task has
	// settled, and opens the next turn itself.
	_, second := turn.Ensure(t.Context(), chat, time.Now().Add(-time.Second))
	require.NotEqual(t, first, second)
	require.EqualValues(t, 1, emittedTurns(t, registry))

	// The finishing task's late calls address a turn that is gone.
	turn.Invalidate(first, chatloop.TurnOutcomeError, xerrors.New("step failed"))
	turn.Settle(t.Context(), first)
	turn.Complete(second, time.Time{})
	turn.Settle(t.Context(), second)
	require.EqualValues(t, 2, emittedTurns(t, registry))
}

func TestRunnerTurnSpanRetryContinuesTurn(t *testing.T) {
	t.Parallel()
	tracer, recorder, registry := newStageMetricsTracer(t)
	turn := newRunnerTurnSpan(tracer, nil)
	chat := database.Chat{ID: uuid.New()}
	triggerAt := time.Now().Add(-time.Minute)

	// A retryable failure does not invalidate the turn; the task only
	// settles, which leaves an unfinished turn open.
	_, token := turn.Ensure(t.Context(), chat, triggerAt)
	turn.Settle(t.Context(), token)

	// The retried task runs the same prompt, so it continues the same
	// turn and records no second acquisition.
	_, retried := turn.Ensure(t.Context(), chat, triggerAt)
	require.Equal(t, token, retried)
	turn.Complete(retried, time.Time{})
	turn.Settle(t.Context(), retried)
	turn.End(nil)

	require.Len(t, turnSpansByStart(t, recorder), 1)
	var acquisitions int
	for _, span := range recorder.Ended() {
		if chatloop.Stage(span.Name()) == chatloop.StageAcquisition {
			acquisitions++
		}
	}
	require.Equal(t, 1, acquisitions)
	require.EqualValues(t, 1, emittedTurns(t, registry))
	require.Equal(t, map[chatloop.TurnOutcome]float64{
		chatloop.TurnOutcomeCompleted: 1,
	}, turnOutcomeCounts(t, registry))
}

// turnOutcomeCounts returns the turn outcome counter values by
// outcome.
func turnOutcomeCounts(t *testing.T, registry *prometheus.Registry) map[chatloop.TurnOutcome]float64 {
	t.Helper()
	families, err := registry.Gather()
	require.NoError(t, err)
	out := map[chatloop.TurnOutcome]float64{}
	for _, family := range families {
		if family.GetName() != "coderd_chatd_turn_outcomes_total" {
			continue
		}
		for _, metric := range family.GetMetric() {
			for _, label := range metric.GetLabel() {
				if label.GetName() == "outcome" {
					out[chatloop.TurnOutcome(label.GetValue())] = metric.GetCounter().GetValue()
				}
			}
		}
	}
	return out
}

func TestRunnerTurnSpanCountsOutcomes(t *testing.T) {
	t.Parallel()
	tracer, recorder, registry := newStageMetricsTracer(t)
	turn := newRunnerTurnSpan(tracer, nil)
	chat := database.Chat{ID: uuid.New()}

	// A completed turn is counted once its partition is emitted.
	turnCtx, completed := turn.Ensure(t.Context(), chat, time.Now().Add(-time.Minute))
	_, stream := tracer.Start(turnCtx, chatloop.StageStream)
	stream.End(nil)
	turn.Complete(completed, time.Time{})
	turn.Settle(t.Context(), completed)

	// An errored turn is counted when it closes, and not as completed
	// even though it was later finished.
	_, errored := turn.Ensure(t.Context(), chat, time.Now())
	turn.Invalidate(errored, chatloop.TurnOutcomeError, xerrors.New("provider refused"))
	turn.Complete(errored, time.Time{})
	turn.Settle(t.Context(), errored)

	// An interrupted turn is closed by the next Ensure, which counts it
	// once; the turn that Ensure opens is later counted as completed.
	_, interrupted := turn.Ensure(t.Context(), chat, time.Now())
	turn.Invalidate(interrupted, chatloop.TurnOutcomeInterrupted, xerrors.Errorf("generation action: %w", context.Canceled))
	_, afterInterrupt := turn.Ensure(t.Context(), chat, time.Now())
	require.NotEqual(t, interrupted, afterInterrupt)
	require.Equal(t, float64(1), turnOutcomeCounts(t, registry)[chatloop.TurnOutcomeInterrupted])
	turn.Complete(afterInterrupt, time.Time{})
	turn.Settle(t.Context(), afterInterrupt)

	// A turn that ends before it finished is abandoned.
	turn.Ensure(t.Context(), chat, time.Now())
	turn.End(nil)

	require.Equal(t, map[chatloop.TurnOutcome]float64{
		chatloop.TurnOutcomeCompleted:   2,
		chatloop.TurnOutcomeError:       1,
		chatloop.TurnOutcomeInterrupted: 1,
		chatloop.TurnOutcomeAbandoned:   1,
	}, turnOutcomeCounts(t, registry))
	require.EqualValues(t, 2, emittedTurns(t, registry))
	require.Len(t, turnSpansByStart(t, recorder), 5, "outcomes sum to closed turns")
}

func TestRunnerTurnSpanEnsureOpensTurnPerPrompt(t *testing.T) {
	t.Parallel()
	tracer, recorder, _ := newStageMetricsTracer(t)
	turn := newRunnerTurnSpan(tracer, nil)
	chat := database.Chat{ID: uuid.New()}

	firstTrigger := time.Now().Add(-2 * time.Minute)
	_, first := turn.Ensure(t.Context(), chat, firstTrigger)
	// A second prompt on the same runner reuses the open turn until it
	// finishes.
	_, again := turn.Ensure(t.Context(), chat, firstTrigger)
	require.Equal(t, first, again)
	require.Len(t, turnSpansByStart(t, recorder), 0)

	turn.Complete(first, time.Time{})
	// The second trigger is taken after the first turn closes so it is
	// a valid anchor.
	turn.Settle(t.Context(), first)
	secondTrigger := time.Now()
	_, second := turn.Ensure(t.Context(), chat, secondTrigger)
	require.NotEqual(t, first, second)
	turn.End(nil)

	turns := turnSpansByStart(t, recorder)
	require.Len(t, turns, 2)
	require.Equal(t, firstTrigger.UTC(), turns[0].StartTime().UTC())
	require.Equal(t, secondTrigger.UTC(), turns[1].StartTime().UTC())
}

func TestRunnerTurnSpanClampsStaleAnchor(t *testing.T) {
	t.Parallel()
	tracer, recorder, registry := newStageMetricsTracer(t)
	turn := newRunnerTurnSpan(tracer, nil)
	chat := database.Chat{ID: uuid.New()}
	staleAnchors := func() float64 {
		return anomalyCount(t, registry, chatloop.StageAnomalyStaleAnchor)
	}

	firstTrigger := time.Now().Add(-time.Minute)
	_, first := turn.Ensure(t.Context(), chat, firstTrigger)
	turn.Complete(first, time.Time{})
	turn.Settle(t.Context(), first)
	require.Zero(t, staleAnchors())

	// A trigger older than the previous turn's anchor is clamped to it.
	_, second := turn.Ensure(t.Context(), chat, firstTrigger.Add(-30*time.Second))
	require.NotEqual(t, first, second)
	require.Equal(t, float64(1), staleAnchors())
	turn.Complete(second, time.Time{})
	turn.Settle(t.Context(), second)

	// A trigger after the previous anchor is kept.
	thirdTrigger := time.Now()
	_, third := turn.Ensure(t.Context(), chat, thirdTrigger)
	require.NotEqual(t, second, third)
	require.Equal(t, float64(1), staleAnchors())
	turn.End(nil)

	turns := turnSpansByStart(t, recorder)
	require.Len(t, turns, 3)
	require.Equal(t, firstTrigger.UTC(), turns[1].StartTime().UTC())
	require.Equal(t, thirdTrigger.UTC(), turns[2].StartTime().UTC())
}

func TestRunnerTurnSpanKeepsAnchorBeforePreviousClose(t *testing.T) {
	t.Parallel()
	tracer, recorder, registry := newStageMetricsTracer(t)
	turn := newRunnerTurnSpan(tracer, nil)
	chat := database.Chat{ID: uuid.New()}

	firstTrigger := time.Now().Add(-time.Minute)
	_, first := turn.Ensure(t.Context(), chat, firstTrigger)
	// The next prompt lands while the first turn is still running.
	secondTrigger := time.Now().Add(-10 * time.Second)
	turn.Complete(first, time.Time{})
	turn.Settle(t.Context(), first)

	_, second := turn.Ensure(t.Context(), chat, secondTrigger)
	require.NotEqual(t, first, second)
	turn.End(nil)

	require.Zero(t, anomalyCount(t, registry, chatloop.StageAnomalyStaleAnchor))
	turns := turnSpansByStart(t, recorder)
	require.Len(t, turns, 2)
	require.Equal(t, secondTrigger.UTC(), turns[1].StartTime().UTC())

	var acquisitions []sdktrace.ReadOnlySpan
	for _, span := range recorder.Ended() {
		if chatloop.Stage(span.Name()) == chatloop.StageAcquisition {
			acquisitions = append(acquisitions, span)
		}
	}
	require.Len(t, acquisitions, 2)
	sort.Slice(acquisitions, func(i, j int) bool {
		return acquisitions[i].StartTime().Before(acquisitions[j].StartTime())
	})
	require.Equal(t, secondTrigger.UTC(), acquisitions[1].StartTime().UTC())
	require.Equal(t, turns[1].SpanContext().SpanID(), acquisitions[1].Parent().SpanID())
}

func TestRunnerTurnSpanZeroTriggerRecordsNoAcquisition(t *testing.T) {
	t.Parallel()
	tracer, recorder, registry := newStageMetricsTracer(t)
	turn := newRunnerTurnSpan(tracer, nil)

	_, token := turn.Ensure(t.Context(), database.Chat{ID: uuid.New()}, time.Time{})
	turn.Complete(token, time.Time{})
	turn.Settle(t.Context(), token)

	require.Len(t, turnSpansByStart(t, recorder), 1)
	for _, span := range recorder.Ended() {
		require.NotEqual(t, chatloop.StageAcquisition, span.Name())
	}
	require.Zero(t, anomalyCount(t, registry, chatloop.StageAnomalyInvertedWindow))
}

func TestRunnerTurnSpanEnsureRotatesInvalidatedTurn(t *testing.T) {
	t.Parallel()
	tracer, recorder, _ := newStageMetricsTracer(t)
	turn := newRunnerTurnSpan(tracer, nil)
	chat := database.Chat{ID: uuid.New()}

	firstTrigger := time.Now().Add(-time.Minute)
	_, first := turn.Ensure(t.Context(), chat, firstTrigger)
	failure := xerrors.New("provider refused")
	turn.Invalidate(first, chatloop.TurnOutcomeError, failure)

	secondTrigger := time.Now()
	_, second := turn.Ensure(t.Context(), chat, secondTrigger)
	require.NotEqual(t, first, second)
	turn.Complete(second, time.Time{})
	turn.Settle(t.Context(), second)

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
	messages := []database.ChatMessage{{
		Role:      database.ChatMessageRoleUser,
		CreatedAt: promptAt,
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
		tracer, recorder, _ := newStageMetricsTracer(t)
		turn := newRunnerTurnSpan(tracer, nil)
		_, token := turn.Ensure(t.Context(), database.Chat{ID: uuid.New()}, time.Now())
		turn.Complete(token, time.Time{})
		turn.Settle(t.Context(), token)

		span := turnSpanFor(t, recorder)
		require.Equal(t, codes.Unset, span.Status().Code)
		require.Equal(t, chatloop.TurnOutcomeCompleted, outcome(t, span))
	})

	t.Run("Error", func(t *testing.T) {
		t.Parallel()
		tracer, recorder, _ := newStageMetricsTracer(t)
		turn := newRunnerTurnSpan(tracer, nil)
		_, token := turn.Ensure(t.Context(), database.Chat{ID: uuid.New()}, time.Now())
		firstErr := xerrors.New("provider refused")
		turn.Invalidate(token, chatloop.TurnOutcomeError, firstErr)
		// The first invalidation wins over later ones and over the End
		// error.
		turn.Invalidate(token, chatloop.TurnOutcomeInterrupted, xerrors.New("later"))
		turn.End(nil)

		span := turnSpanFor(t, recorder)
		require.Equal(t, codes.Error, span.Status().Code)
		require.Equal(t, firstErr.Error(), span.Status().Description)
		require.Equal(t, chatloop.TurnOutcomeError, outcome(t, span))
	})

	t.Run("ErrorThenSettled", func(t *testing.T) {
		t.Parallel()
		tracer, recorder, _ := newStageMetricsTracer(t)
		turn := newRunnerTurnSpan(tracer, nil)
		_, token := turn.Ensure(t.Context(), database.Chat{ID: uuid.New()}, time.Now())
		turn.Invalidate(token, chatloop.TurnOutcomeError, xerrors.New("provider refused"))
		turn.Complete(token, time.Time{})
		turn.Settle(t.Context(), token)

		span := turnSpanFor(t, recorder)
		require.Equal(t, codes.Error, span.Status().Code)
		require.Equal(t, chatloop.TurnOutcomeError, outcome(t, span))
	})

	t.Run("Interrupted", func(t *testing.T) {
		t.Parallel()
		tracer, recorder, _ := newStageMetricsTracer(t)
		turn := newRunnerTurnSpan(tracer, nil)
		_, token := turn.Ensure(t.Context(), database.Chat{ID: uuid.New()}, time.Now())
		turn.Invalidate(token, chatloop.TurnOutcomeInterrupted, xerrors.Errorf("generation action: %w", context.Canceled))
		turn.End(nil)

		span := turnSpanFor(t, recorder)
		require.Equal(t, codes.Error, span.Status().Code)
		require.Equal(t, chatloop.TurnOutcomeInterrupted, outcome(t, span))
	})

	t.Run("AbandonedOnEndOfUnfinishedTurn", func(t *testing.T) {
		t.Parallel()
		tracer, recorder, _ := newStageMetricsTracer(t)
		turn := newRunnerTurnSpan(tracer, nil)
		turn.Ensure(t.Context(), database.Chat{ID: uuid.New()}, time.Now())
		turn.End(nil)

		span := turnSpanFor(t, recorder)
		require.Equal(t, codes.Unset, span.Status().Code)
		require.Equal(t, chatloop.TurnOutcomeAbandoned, outcome(t, span))
	})

	t.Run("StaleTokenIgnored", func(t *testing.T) {
		t.Parallel()
		tracer, recorder, _ := newStageMetricsTracer(t)
		turn := newRunnerTurnSpan(tracer, nil)
		_, token := turn.Ensure(t.Context(), database.Chat{ID: uuid.New()}, time.Now())
		turn.Invalidate(token+1, chatloop.TurnOutcomeError, xerrors.New("not this turn"))
		turn.Complete(token, time.Time{})
		turn.End(nil)

		span := turnSpanFor(t, recorder)
		require.Equal(t, codes.Unset, span.Status().Code)
		require.Equal(t, chatloop.TurnOutcomeCompleted, outcome(t, span))
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
