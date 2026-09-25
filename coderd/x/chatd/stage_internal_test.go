package chatd

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

func newStageTestTracer(t *testing.T) (*chatloop.StageTracer, *tracetest.SpanRecorder) {
	t.Helper()
	recorder := tracetest.NewSpanRecorder()
	provider := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(recorder))
	t.Cleanup(func() {
		// t.Context is canceled before Cleanup runs.
		require.NoError(t, provider.Shutdown(context.Background()))
	})
	return chatloop.NewStageTracer(provider, chatloop.NopMetrics()), recorder
}

type stubRoundTripper struct {
	status int
	err    error
	// seen, when set, receives the span context of each request.
	seen *trace.SpanContext
}

func (s stubRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	if s.seen != nil {
		*s.seen = trace.SpanContextFromContext(req.Context())
	}
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

	stageModel := chatloop.StageModel{ProviderType: "bedrock", Model: "claude-sonnet-4-5", Effort: "medium"}
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
			var seen trace.SpanContext
			base := test.base
			base.seen = &seen
			transport := &stageSpanRoundTripper{base: base, stages: tracer, stageModel: stageModel}

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
			// The base transport runs under the provider_attempt span.
			require.Equal(t, span.SpanContext(), seen)
			statusCode, sawStatusCode := spanAttribute(t, span, chatloop.AttrHTTPStatusCode)
			require.Equal(t, !test.wantErr, sawStatusCode)
			if sawStatusCode {
				require.Equal(t, int64(test.base.status), statusCode.AsInt64())
			}
			require.Subset(t, span.Attributes(), []attribute.KeyValue{
				attribute.String(chatloop.AttrHTTPMethod, http.MethodPost),
				attribute.String(chatloop.AttrProviderType, stageModel.ProviderType),
				attribute.String(chatloop.AttrModel, stageModel.Model),
				attribute.String(chatloop.AttrReasoningEffort, stageModel.Effort),
			})
		})
	}
}
