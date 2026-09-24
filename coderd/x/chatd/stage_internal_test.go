package chatd //nolint:testpackage // Tests unexported stage instrumentation internals.

import (
	"context"
	"io"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"

	"charm.land/fantasy"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd/database"
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
			tracer, recorder := newStageTestTracer(t)
			transport := &stageSpanRoundTripper{base: test.base, stages: tracer, stageModel: model}

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
			require.Contains(t, span.Attributes(), attribute.String(chatloop.AttrProviderType, model.ProviderType))
			require.Contains(t, span.Attributes(), attribute.String(chatloop.AttrModel, model.Model))
			require.Contains(t, span.Attributes(), attribute.String(chatloop.AttrReasoningEffort, model.Effort))
		})
	}
}

// The provider SDK is built without retries, so a refused call
// surfaces as an error and the retry is a second call.
func TestNewModelTracesEachProviderAttempt(t *testing.T) {
	t.Parallel()

	tracer, recorder := newStageTestTracer(t)
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
