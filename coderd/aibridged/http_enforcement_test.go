package aibridged_test

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
	"storj.io/drpc/drpcerr"

	"cdr.dev/slog/v3/sloggers/slogtest"
	"github.com/coder/coder/v2/aibridge"
	"github.com/coder/coder/v2/aibridge/config"
	agplaibridge "github.com/coder/coder/v2/coderd/aibridge"
	"github.com/coder/coder/v2/coderd/aibridged"
	mock "github.com/coder/coder/v2/coderd/aibridged/aibridgedmock"
	"github.com/coder/coder/v2/coderd/aibridged/proto"
	"github.com/coder/coder/v2/testutil"
)

func TestServeHTTP_ModelAuthorization(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name      string
		delegated bool
		code      uint64
		status    int
		wantCalls int
	}{
		{name: "full_key_allowed", status: http.StatusTeapot, wantCalls: 1},
		{name: "full_key_policy", code: proto.AuthorizationErrorPolicy, status: http.StatusForbidden},
		{name: "full_key_evaluation", code: proto.AuthorizationErrorEvaluation, status: http.StatusInternalServerError},
		{name: "full_key_unknown_error", code: 9999, status: http.StatusInternalServerError},
		{name: "delegated_allowed", delegated: true, status: http.StatusTeapot, wantCalls: 1},
		{name: "delegated_policy", delegated: true, code: proto.AuthorizationErrorPolicy, status: http.StatusForbidden},
		{name: "delegated_evaluation", delegated: true, code: proto.AuthorizationErrorEvaluation, status: http.StatusInternalServerError},
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

			srv, err := aibridged.New(t.Context(), func(context.Context) (aibridged.DRPCClient, error) {
				return client, nil
			}, slogtest.Make(t, &slogtest.Options{IgnoreErrors: true}), testTracer, nil, nil)
			require.NoError(t, err)
			t.Cleanup(func() { _ = srv.Shutdown(testutil.Context(t, testutil.WaitShort)) })
			require.Eventually(t, srv.Ready, testutil.WaitShort, testutil.IntervalFast)
			require.NoError(t, srv.ReplaceProviders(t.Context(), []aibridge.Provider{
				aibridge.NewOpenAIProvider(config.OpenAI{
					Name: "openai", BaseURL: upstream.URL,
					KeyPool: singleKeyPool(t, "openai", "test-provider-key"),
				}),
			}))

			ctx := testutil.Context(t, testutil.WaitShort)
			if tc.delegated {
				ctx = agplaibridge.WithDelegatedAPIKeyID(ctx, "delegated-key-id")
			}
			req := httptest.NewRequest(http.MethodPost, "/openai/v1/chat/completions", bytes.NewBufferString(`{"model":"gpt-4o","messages":[{"role":"user","content":"hello"}]}`)).WithContext(ctx)
			if !tc.delegated {
				req.Header.Set("Authorization", "Bearer full-key-secret")
			}
			rw := httptest.NewRecorder()

			srv.ServeHTTP(rw, req)

			require.Equal(t, tc.status, rw.Code)
			require.EqualValues(t, tc.wantCalls, upstreamCalls.Load())
		})
	}
}
