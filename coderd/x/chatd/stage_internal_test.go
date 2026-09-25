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
