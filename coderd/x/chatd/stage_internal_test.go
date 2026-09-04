package chatd //nolint:testpackage // Tests unexported stage instrumentation internals.

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"go.opentelemetry.io/otel/trace"
	"golang.org/x/xerrors"

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
