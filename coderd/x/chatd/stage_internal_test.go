package chatd //nolint:testpackage // Tests unexported stage instrumentation internals.

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
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

// newStageTestTracer returns a stage tracer writing into an in-memory
// span recorder.
func newStageTestTracer(t *testing.T) (*chatloop.StageTracer, *tracetest.SpanRecorder) {
	t.Helper()
	recorder := tracetest.NewSpanRecorder()
	provider := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(recorder))
	t.Cleanup(func() {
		// The test context is already canceled during cleanup, so the
		// flush uses a fresh one.
		require.NoError(t, provider.Shutdown(context.Background()))
	})
	return chatloop.NewStageTracer(provider, chatloop.NopMetrics()), recorder
}

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

func TestStageSpanRoundTripper(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name           string
		base           stubRoundTripper
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
			name:           "client error",
			base:           stubRoundTripper{status: http.StatusTooManyRequests},
			wantStatusCode: codes.Error,
			wantAttribute:  true,
		},
		{
			name:           "server error",
			base:           stubRoundTripper{status: http.StatusInternalServerError},
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
			tracer, recorder := newStageTestTracer(t)
			transport := &stageSpanRoundTripper{base: test.base, stages: tracer}

			req, err := http.NewRequestWithContext(t.Context(), http.MethodPost, "https://provider.example/v1/messages", nil)
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
			require.Equal(t, chatloop.StageProviderAttempt, ended[0].Name())
			require.Equal(t, test.wantStatusCode, ended[0].Status().Code)
			var sawStatusCode bool
			for _, attr := range ended[0].Attributes() {
				if string(attr.Key) == chatloop.AttrHTTPStatusCode {
					sawStatusCode = true
					require.Equal(t, int64(test.base.status), attr.Value.AsInt64())
				}
			}
			require.Equal(t, test.wantAttribute, sawStatusCode)
			require.Contains(t, ended[0].Attributes(),
				attribute.String(chatloop.AttrScope, chatloop.ScopeBackground))
		})
	}
}

func TestStageSpanRoundTripperScope(t *testing.T) {
	t.Parallel()
	tracer, recorder := newStageTestTracer(t)
	transport := &stageSpanRoundTripper{base: stubRoundTripper{status: http.StatusOK}, stages: tracer}

	turnCtx, turn := tracer.StartRoot(t.Context(), chatloop.StageChatTurn, nil)
	turnReq, err := http.NewRequestWithContext(turnCtx, http.MethodPost, "https://provider.example/v1/messages", nil)
	require.NoError(t, err)
	turnResp, err := transport.RoundTrip(turnReq)
	require.NoError(t, err)
	require.NoError(t, turnResp.Body.Close())

	// Background work runs on a context detached from the turn, the same
	// way inflight chatd tasks are.
	backgroundCtx := chatloop.ContextWithScope(
		trace.ContextWithSpanContext(turnCtx, trace.SpanContext{}),
		chatloop.ScopeBackground,
	)
	backgroundReq, err := http.NewRequestWithContext(backgroundCtx, http.MethodPost, "https://provider.example/v1/messages", nil)
	require.NoError(t, err)
	backgroundResp, err := transport.RoundTrip(backgroundReq)
	require.NoError(t, err)
	require.NoError(t, backgroundResp.Body.Close())
	turn.End(nil)

	var scopes []string
	for _, span := range recorder.Ended() {
		if span.Name() != chatloop.StageProviderAttempt {
			continue
		}
		for _, attr := range span.Attributes() {
			if string(attr.Key) == chatloop.AttrScope {
				scopes = append(scopes, attr.Value.AsString())
			}
		}
	}
	require.Equal(t, []string{chatloop.ScopeTurn, chatloop.ScopeBackground}, scopes)
}

func TestStageSpanRoundTripperModel(t *testing.T) {
	t.Parallel()
	tracer, recorder := newStageTestTracer(t)
	model := chatloop.StageModel{Model: "claude-sonnet-4-5", Effort: "medium"}
	transport := &stageSpanRoundTripper{
		base:   stubRoundTripper{status: http.StatusOK},
		stages: tracer,
		model:  model,
	}

	req, err := http.NewRequestWithContext(t.Context(), http.MethodPost, "https://provider.example/v1/messages", nil)
	require.NoError(t, err)
	resp, err := transport.RoundTrip(req)
	require.NoError(t, err)
	require.NoError(t, resp.Body.Close())

	ended := recorder.Ended()
	require.Len(t, ended, 1)
	require.Contains(t, ended[0].Attributes(),
		attribute.String(chatloop.AttrModel, model.Model))
	require.Contains(t, ended[0].Attributes(),
		attribute.String(chatloop.AttrReasoningEffort, model.Effort))
}

func TestRunnerTurnSpanStartsAtTriggerMessage(t *testing.T) {
	t.Parallel()
	tracer, recorder := newStageTestTracer(t)
	turn := newRunnerTurnSpan(tracer)
	chat := database.Chat{ID: uuid.New()}
	triggerAt := time.Now().Add(-2 * time.Second)

	turnCtx := turn.Ensure(t.Context(), chat, triggerAt)
	turn.End(nil)

	ended := recorder.Ended()
	require.Len(t, ended, 2)
	acquisition, chatTurn := ended[0], ended[1]
	require.Equal(t, chatloop.StageAcquisition, acquisition.Name())
	require.Equal(t, chatloop.StageChatTurn, chatTurn.Name())
	require.Equal(t, triggerAt.UTC(), chatTurn.StartTime().UTC())
	require.Equal(t, triggerAt.UTC(), acquisition.StartTime().UTC())
	require.False(t, acquisition.StartTime().Before(chatTurn.StartTime()))
	require.False(t, acquisition.EndTime().After(chatTurn.EndTime()))
	require.Equal(t, chatTurn.SpanContext().SpanID(), acquisition.Parent().SpanID())
	require.Equal(t, chatTurn.SpanContext().TraceID(), trace.SpanContextFromContext(turnCtx).TraceID())
}

func TestRunnerTurnSpanParentsRecordedStages(t *testing.T) {
	t.Parallel()
	tracer, recorder := newStageTestTracer(t)
	turn := newRunnerTurnSpan(tracer)
	chat := database.Chat{ID: uuid.New()}

	turnCtx := turn.Ensure(t.Context(), chat, time.Now().Add(-time.Second))
	stepCtx, step := tracer.Start(turnCtx, chatloop.StageGenerationStep)
	step.End(nil)

	// The step has already ended, so the queue wait is recorded
	// against the turn context the runner keeps.
	tracer.Record(turn.Context(stepCtx), chatloop.StageQueueWait, chatloop.StageModel{},
		time.Now().Add(-500*time.Millisecond), time.Now(), nil)
	turn.End(nil)

	var queueWait, chatTurn, generationStep sdktrace.ReadOnlySpan
	for _, span := range recorder.Ended() {
		switch span.Name() {
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
	tracer, recorder := newStageTestTracer(t)
	server := &Server{stages: tracer}

	// The promoting request has its own span, which the queue wait must
	// not join.
	requestCtx, requestSpan := tracer.Start(t.Context(), chatloop.StageCommit)
	queuedAt := time.Now().Add(-30 * time.Second)
	server.recordQueueWait(requestCtx, uuid.New(), queuedAt, queuedAt.Add(20*time.Second))
	requestSpan.End(nil)

	var queueWait, request sdktrace.ReadOnlySpan
	for _, span := range recorder.Ended() {
		switch span.Name() {
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
	require.Contains(t, queueWait.Attributes(),
		attribute.String(chatloop.AttrScope, chatloop.ScopeTurn))
}

func TestServerInflightContextIsBackgroundScoped(t *testing.T) {
	t.Parallel()
	tracer, recorder := newStageTestTracer(t)
	serverCtx, serverCancel := context.WithCancel(context.Background())
	t.Cleanup(serverCancel)
	server := &Server{ctx: serverCtx, stages: tracer}

	turn := newRunnerTurnSpan(tracer)
	turnCtx := turn.Ensure(t.Context(), database.Chat{ID: uuid.New()}, time.Now().Add(-time.Second))
	inflightCtx, stop := server.inflightContext(turnCtx)
	t.Cleanup(stop)

	_, span := tracer.Start(inflightCtx, chatloop.StageGenerationStep)
	span.End(nil)
	turn.End(nil)

	for _, ended := range recorder.Ended() {
		if ended.Name() != chatloop.StageGenerationStep {
			continue
		}
		require.False(t, ended.Parent().IsValid())
		require.Contains(t, ended.Attributes(),
			attribute.String(chatloop.AttrScope, chatloop.ScopeBackground))
	}
}
