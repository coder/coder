package aibridged_test

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/google/uuid"
	"github.com/prometheus/client_golang/prometheus"
	promtest "github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
	"storj.io/drpc/drpcerr"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/aibridge"
	"github.com/coder/coder/v2/aibridge/config"
	"github.com/coder/coder/v2/aibridge/intercept"
	agplaibridge "github.com/coder/coder/v2/coderd/aibridge"
	"github.com/coder/coder/v2/coderd/aibridged"
	mock "github.com/coder/coder/v2/coderd/aibridged/aibridgedmock"
	"github.com/coder/coder/v2/coderd/aibridged/proto"
	"github.com/coder/coder/v2/testutil"
)

func TestServeHTTP_ModelAuthorization(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name         string
		delegated    bool
		stream       bool
		noCredential bool
		code         uint64
		status       int
		wantCalls    int
		kind         string
	}{
		{name: "full_key_allowed", status: http.StatusTeapot, wantCalls: 1},
		{name: "full_key_authentication", code: proto.AuthorizationErrorAuthentication, status: http.StatusForbidden, kind: "authentication"},
		{name: "full_key_policy", code: proto.AuthorizationErrorPolicy, status: http.StatusForbidden, kind: "policy"},
		{name: "full_key_evaluation", code: proto.AuthorizationErrorEvaluation, status: http.StatusInternalServerError, kind: "evaluation"},
		{name: "full_key_malformed", code: proto.AuthorizationErrorMalformed, status: http.StatusInternalServerError, kind: "malformed"},
		{name: "full_key_unknown_error", code: 9999, status: http.StatusInternalServerError, kind: "evaluation"},
		{name: "delegated_allowed", delegated: true, status: http.StatusTeapot, wantCalls: 1},
		{name: "delegated_policy", delegated: true, code: proto.AuthorizationErrorPolicy, status: http.StatusForbidden, kind: "policy"},
		{name: "delegated_evaluation", delegated: true, code: proto.AuthorizationErrorEvaluation, status: http.StatusInternalServerError, kind: "evaluation"},
		{name: "denied_before_credentials_blocking", noCredential: true, code: proto.AuthorizationErrorPolicy, status: http.StatusForbidden, kind: "policy"},
		{name: "denied_before_credentials_streaming", noCredential: true, stream: true, code: proto.AuthorizationErrorPolicy, status: http.StatusForbidden, kind: "policy"},
		{name: "allowed_without_credentials", noCredential: true, status: http.StatusInternalServerError},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			var upstreamCalls atomic.Int32
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				upstreamCalls.Add(1)
				if r.Header.Get("Authorization") != "Bearer test-provider-key" {
					t.Error("upstream request did not use the configured provider credential")
				}
				w.WriteHeader(http.StatusTeapot)
			}))
			t.Cleanup(upstream.Close)

			ctrl := gomock.NewController(t)
			client := mock.NewMockDRPCClient(ctrl)
			conn := &mockDRPCConn{}
			client.EXPECT().DRPCConn().AnyTimes().Return(conn)
			client.EXPECT().GetMCPServerConfigs(gomock.Any(), gomock.Any()).AnyTimes().Return(&proto.GetMCPServerConfigsResponse{}, nil)

			ownerID := uuid.NewString()
			client.EXPECT().IsAuthorized(gomock.Any(), gomock.Any()).Times(2).DoAndReturn(
				func(_ context.Context, in *proto.IsAuthorizedRequest) (*proto.IsAuthorizedResponse, error) {
					if tc.delegated {
						require.Equal(t, "delegated-key-id", in.GetKeyId())
						require.Empty(t, in.GetKey())
					} else {
						require.Equal(t, "full-key-secret", in.GetKey())
						require.Empty(t, in.GetKeyId())
					}
					if in.ProviderName == nil && in.Model == nil {
						return &proto.IsAuthorizedResponse{OwnerId: ownerID, ApiKeyId: "api-key-id", Username: "user"}, nil
					}
					require.NotNil(t, in.ProviderName)
					require.NotNil(t, in.Model)
					require.Equal(t, "openai", in.GetProviderName())
					require.Equal(t, "gpt-4o", in.GetModel())
					if tc.code == 0 {
						return &proto.IsAuthorizedResponse{}, nil
					}
					return nil, drpcerr.WithCode(context.Canceled, tc.code)
				})
			client.EXPECT().IsBudgetExceeded(gomock.Any(), gomock.Any()).Return(&proto.IsBudgetExceededResponse{}, nil)
			client.EXPECT().RecordInterception(gomock.Any(), gomock.Any()).Times(tc.wantCalls).Return(&proto.RecordInterceptionResponse{}, nil)
			client.EXPECT().RecordInterceptionEnded(gomock.Any(), gomock.Any()).Times(tc.wantCalls)

			logs := testutil.NewFakeSink(t)
			metrics := aibridge.NewMetrics(prometheus.NewRegistry())
			srv, err := aibridged.New(t.Context(), func(context.Context) (aibridged.DRPCClient, error) {
				return client, nil
			}, logs.Logger(), testTracer, nil, metrics)
			require.NoError(t, err)
			t.Cleanup(func() { _ = srv.Shutdown(testutil.Context(t, testutil.WaitShort)) })
			require.Eventually(t, srv.Ready, testutil.WaitShort, testutil.IntervalFast)
			cfg := config.OpenAI{Name: "openai", BaseURL: upstream.URL}
			if !tc.noCredential {
				cfg.KeyPool = singleKeyPool(t, "openai", "test-provider-key")
			}
			require.NoError(t, srv.ReplaceProviders(t.Context(), []aibridge.Provider{
				aibridge.NewOpenAIProvider(cfg),
			}))

			ctx := testutil.Context(t, testutil.WaitShort)
			if tc.delegated {
				ctx = agplaibridge.WithDelegatedAPIKeyID(ctx, "delegated-key-id")
			}
			body := fmt.Sprintf(`{"model":"gpt-4o","messages":[{"role":"user","content":"hello"}],"stream":%t}`, tc.stream)
			req := httptest.NewRequest(http.MethodPost, "/openai/v1/chat/completions", bytes.NewBufferString(body)).WithContext(ctx)
			if !tc.delegated {
				req.Header.Set("Authorization", "Bearer full-key-secret")
			}
			rw := httptest.NewRecorder()

			srv.ServeHTTP(rw, req)

			require.Equal(t, tc.status, rw.Code)
			require.EqualValues(t, tc.wantCalls, upstreamCalls.Load())
			if tc.noCredential && tc.code == 0 {
				require.Contains(t, rw.Body.String(), "failed to resolve credential:")
			}
			outcome := "allowed"
			if tc.code != 0 {
				outcome = "error"
				if tc.status == http.StatusForbidden {
					outcome = "denied"
					require.Equal(t, "unauthorized\n", rw.Body.String())
				}
			}
			for _, label := range []string{"allowed", "denied", "error"} {
				want := 0.0
				if label == outcome {
					want = 1
				}
				require.Equal(t, want, promtest.ToFloat64(metrics.AuthorizationCount.WithLabelValues(label)))
			}
			if tc.kind != "" {
				entries := logs.Entries(func(entry slog.SinkEntry) bool {
					return entry.Message == "request authorization denied" || entry.Message == "request authorization failed"
				})
				require.Len(t, entries, 1)
				require.Contains(t, entries[0].Fields, slog.F("authorization_error_kind", intercept.AuthorizationErrorKind(tc.kind)))
			}
		})
	}
}
