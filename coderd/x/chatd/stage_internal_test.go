package chatd

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/x/chatd/chatloop"
	"github.com/coder/coder/v2/coderd/x/chatd/mcpclient"
	"github.com/coder/quartz"
)

func newStageTestTracer(t *testing.T) (*chatloop.StageTracer, *tracetest.SpanRecorder) {
	t.Helper()
	tracer, recorder, _ := newStageMetricsTracer(t)
	return tracer, recorder
}

// newStageMetricsTracer returns a stage tracer writing spans into an
// in-memory recorder and metrics into a private registry.
func newStageMetricsTracer(t *testing.T, opts ...chatloop.StageTracerOption) (*chatloop.StageTracer, *tracetest.SpanRecorder, *prometheus.Registry) {
	t.Helper()
	recorder := tracetest.NewSpanRecorder()
	provider := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(recorder))
	t.Cleanup(func() {
		// t.Context is canceled before Cleanup runs.
		require.NoError(t, provider.Shutdown(context.Background()))
	})
	registry := prometheus.NewRegistry()
	return chatloop.NewStageTracer(provider, chatloop.NewMetricsWithOptions(registry, chatloop.MetricsOptions{StageMetrics: true}), opts...), recorder, registry
}

// anomalyCount returns the stage anomaly count recorded for reason.
func anomalyCount(t *testing.T, registry *prometheus.Registry, reason chatloop.StageAnomaly) float64 {
	t.Helper()
	families, err := registry.Gather()
	require.NoError(t, err)
	for _, family := range families {
		if family.GetName() != "coderd_chatd_stage_anomalies_total" {
			continue
		}
		for _, metric := range family.GetMetric() {
			for _, label := range metric.GetLabel() {
				if label.GetName() == "reason" && label.GetValue() == string(reason) {
					return metric.GetCounter().GetValue()
				}
			}
		}
	}
	return 0
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
		wantStatusCode codes.Code
		wantErr        bool
	}{
		{
			name:           "success",
			base:           stubRoundTripper{status: http.StatusOK},
			wantStatusCode: codes.Unset,
		},
		{
			name:           "client error",
			base:           stubRoundTripper{status: http.StatusTooManyRequests},
			wantStatusCode: codes.Error,
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
			transport := &stageSpanRoundTripper{base: test.base, stages: tracer, stageModel: model}

			req, err := http.NewRequestWithContext(t.Context(), http.MethodPost, "https://provider.example/v1/messages", nil)
			require.NoError(t, err)
			resp, err := transport.RoundTrip(req)
			if test.wantErr {
				require.Error(t, err)
			} else {
				// An HTTP error status still returns the response with a
				// nil error.
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
			require.Equal(t, !test.wantErr, sawStatusCode)
			if sawStatusCode {
				require.Equal(t, int64(test.base.status), statusCode.AsInt64())
			}
			require.Subset(t, span.Attributes(), []attribute.KeyValue{
				attribute.String(chatloop.AttrHTTPMethod, http.MethodPost),
				attribute.String(chatloop.AttrProviderType, model.ProviderType),
				attribute.String(chatloop.AttrModel, model.Model),
				attribute.String(chatloop.AttrReasoningEffort, model.Effort),
			})
		})
	}
}

func TestServerRecordQueueWaitIsStandalone(t *testing.T) {
	t.Parallel()
	clock := quartz.NewMock(t)
	tracer, recorder, _ := newStageMetricsTracer(t, chatloop.WithClock(clock))
	server := &Server{stages: tracer}
	chat := database.Chat{
		ID:             uuid.New(),
		OrganizationID: uuid.New(),
		ParentChatID:   uuid.NullUUID{UUID: uuid.New(), Valid: true},
	}
	server.organizationNames.Store(chat.OrganizationID, "acme")

	// The promoting request has its own span, which the queue wait must
	// not join.
	requestCtx, requestSpan := tracer.Start(t.Context(), chatloop.StageCommit)
	queuedAt := clock.Now().Add(-30 * time.Second)
	server.recordQueueWait(requestCtx, chat, queuedAt)
	// A zero queue time means nothing was promoted.
	server.recordQueueWait(requestCtx, chat, time.Time{})
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
	require.Len(t, recorder.Ended(), 2)
	require.NotNil(t, queueWait)
	require.NotNil(t, request)
	require.False(t, queueWait.Parent().IsValid())
	require.NotEqual(t, request.SpanContext().TraceID(), queueWait.SpanContext().TraceID())
	require.Equal(t, queuedAt.UTC(), queueWait.StartTime().UTC())
	// The wait ends at the tracer's now.
	require.Equal(t, clock.Now().UTC(), queueWait.EndTime().UTC())
	require.Contains(t, queueWait.Attributes(),
		attribute.String(chatloop.AttrScope, string(chatloop.ScopeTurn)))
	require.Contains(t, queueWait.Attributes(),
		attribute.String(chatloop.AttrChatKind, string(chatloop.ChatKindSubagent)))
	require.Contains(t, queueWait.Attributes(),
		attribute.String(chatloop.AttrOrganizationName, "acme"))
}

func TestServerInflightContextIsBackgroundScoped(t *testing.T) {
	t.Parallel()
	tracer, recorder := newStageTestTracer(t)
	server := &Server{ctx: t.Context(), stages: tracer}

	// The turn is for a root chat, so the subagent kind and organization
	// on the stage come from the chat passed to inflightChatContext.
	turn := newRunnerTurnSpan(tracer, nil, false)
	turnCtx, _ := turn.Ensure(t.Context(), database.Chat{ID: uuid.New()}, time.Now().Add(-time.Second))
	chat := database.Chat{
		ID:             uuid.New(),
		OrganizationID: uuid.New(),
		ParentChatID:   uuid.NullUUID{UUID: uuid.New(), Valid: true},
	}
	server.organizationNames.Store(chat.OrganizationID, "acme")
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
		attribute.String(chatloop.AttrChatKind, string(chatloop.ChatKindSubagent)))
	require.Contains(t, step.Attributes(),
		attribute.String(chatloop.AttrOrganizationName, "acme"))
}

func TestWaitGenerationRetryStage(t *testing.T) {
	t.Parallel()

	newStarter := func(t *testing.T) (*taskStarter, *quartz.Mock, *tracetest.SpanRecorder) {
		t.Helper()
		tracer, recorder := newStageTestTracer(t)
		clock := quartz.NewMock(t)
		return &taskStarter{
			server: &Server{stages: tracer},
			opts:   chatWorkerOptions{Clock: clock},
		}, clock, recorder
	}

	t.Run("ElapsedEndsWithoutError", func(t *testing.T) {
		t.Parallel()
		starter, clock, recorder := newStarter(t)
		trap := clock.Trap().NewTimer("chatworker", "generation-retry")
		defer trap.Close()

		done := make(chan error, 1)
		go func() {
			done <- starter.waitGenerationRetry(t.Context(), time.Minute)
		}()
		trap.MustWait(t.Context()).MustRelease(t.Context())
		clock.Advance(time.Minute).MustWait(t.Context())
		require.NoError(t, <-done)

		ended := recorder.Ended()
		require.Len(t, ended, 1)
		require.Equal(t, string(chatloop.StageRetryBackoff), ended[0].Name())
		require.Equal(t, codes.Unset, ended[0].Status().Code)
	})

	t.Run("CanceledEndsWithError", func(t *testing.T) {
		t.Parallel()
		starter, clock, recorder := newStarter(t)
		trap := clock.Trap().NewTimer("chatworker", "generation-retry")
		defer trap.Close()

		ctx, cancel := context.WithCancel(t.Context())
		done := make(chan error, 1)
		go func() {
			done <- starter.waitGenerationRetry(ctx, time.Minute)
		}()
		trap.MustWait(t.Context()).MustRelease(t.Context())
		cancel()
		err := <-done
		require.ErrorIs(t, err, errTaskExpectedExit)
		require.ErrorIs(t, err, context.Canceled)

		ended := recorder.Ended()
		require.Len(t, ended, 1)
		require.Equal(t, string(chatloop.StageRetryBackoff), ended[0].Name())
		require.Equal(t, codes.Error, ended[0].Status().Code)
	})
}

func TestMCPConnectOutcome(t *testing.T) {
	t.Parallel()

	connected := mcpclient.ConnectSummary{Slug: "docs", Outcome: mcpclient.ConnectOutcomeConnected}
	noTools := mcpclient.ConnectSummary{Slug: "empty", Outcome: mcpclient.ConnectOutcomeNoTools}
	timedOut := mcpclient.ConnectSummary{Slug: "github", Outcome: mcpclient.ConnectOutcomeTimeout, Error: "connect timeout"}
	failed := mcpclient.ConnectSummary{Slug: "linear", Outcome: mcpclient.ConnectOutcomeError}

	tests := []struct {
		name          string
		summaries     []mcpclient.ConnectSummary
		wantErr       string
		wantConnected int
		wantFailed    int
	}{
		{name: "AllConnected", summaries: []mcpclient.ConnectSummary{connected, noTools}, wantConnected: 2},
		{name: "PartialFailure", summaries: []mcpclient.ConnectSummary{connected, timedOut}, wantConnected: 1, wantFailed: 1},
		{
			name:       "AllFailed",
			summaries:  []mcpclient.ConnectSummary{timedOut, failed},
			wantErr:    "no MCP server connected: github: connect timeout; linear: error",
			wantFailed: 2,
		},
		{name: "NoServers"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			gotConnected, gotFailed, err := mcpConnectOutcome(tt.summaries)
			if tt.wantErr != "" {
				require.EqualError(t, err, tt.wantErr)
			} else {
				require.NoError(t, err)
			}
			require.Equal(t, tt.wantConnected, gotConnected)
			require.Equal(t, tt.wantFailed, gotFailed)
		})
	}
}
