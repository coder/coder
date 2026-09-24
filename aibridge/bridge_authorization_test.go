package aibridge_test

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel"

	"cdr.dev/slog/v3/sloggers/slogtest"
	"github.com/coder/coder/v2/aibridge"
	"github.com/coder/coder/v2/aibridge/config"
	"github.com/coder/coder/v2/aibridge/intercept"
	"github.com/coder/coder/v2/aibridge/internal/testutil"
	"github.com/coder/coder/v2/aibridge/provider"
)

func TestRequestBridgeAuthorizationDeniedBeforeRecordingAndCredentialResolution(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name   string
		stream bool
	}{
		{name: "blocking"},
		{name: "streaming", stream: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			var upstreamCalls atomic.Int32
			upstream := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
				upstreamCalls.Add(1)
			}))
			t.Cleanup(upstream.Close)

			rec := &testutil.MockRecorder{}
			bridge, err := aibridge.NewRequestBridge(
				t.Context(),
				[]provider.Provider{aibridge.NewOpenAIProvider(config.OpenAI{BaseURL: upstream.URL})},
				rec,
				nil,
				slogtest.Make(t, &slogtest.Options{IgnoreErrors: true}),
				nil,
				otel.Tracer("authorization-test"),
			)
			require.NoError(t, err)

			var gotProvider, gotModel string
			ctx := intercept.WithRequestAuthorizer(context.Background(), func(_ context.Context, providerName, invocationModel string) error {
				gotProvider = providerName
				gotModel = invocationModel
				return &intercept.AuthorizationError{Kind: intercept.AuthorizationErrorPolicy, Err: context.Canceled}
			})
			body := []byte(`{"model":"gpt-4o","messages":[{"role":"user","content":"hello"}],"stream":false}`)
			if tc.stream {
				body = bytes.Replace(body, []byte(`"stream":false`), []byte(`"stream":true`), 1)
			}
			req := httptest.NewRequest(http.MethodPost, "/openai/v1/chat/completions", bytes.NewReader(body)).WithContext(aibridge.AsActor(ctx, "actor", nil))
			resp := httptest.NewRecorder()

			bridge.ServeHTTP(resp, req)

			require.Equal(t, http.StatusForbidden, resp.Code)
			require.Empty(t, rec.RecordedInterceptions())
			require.Zero(t, upstreamCalls.Load())
			require.Equal(t, "openai", gotProvider)
			require.Equal(t, "gpt-4o", gotModel)
		})
	}
}
