package aibridged_test

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
	"golang.org/x/xerrors"
	"storj.io/drpc"

	"cdr.dev/slog/sloggers/slogtest"
	"github.com/coder/aibridge"
	agplaibridge "github.com/coder/coder/v2/coderd/aibridge"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/enterprise/aibridged"
	mock "github.com/coder/coder/v2/enterprise/aibridged/aibridgedmock"
	"github.com/coder/coder/v2/enterprise/aibridged/proto"
	"github.com/coder/coder/v2/testutil"
)

func newTestServer(t *testing.T) (*aibridged.Server, *mock.MockDRPCClient, *mock.MockPooler) {
	t.Helper()

	logger := slogtest.Make(t, nil)
	ctrl := gomock.NewController(t)
	client := mock.NewMockDRPCClient(ctrl)
	pool := mock.NewMockPooler(ctrl)

	conn := &mockDRPCConn{}
	client.EXPECT().DRPCConn().AnyTimes().Return(conn)
	pool.EXPECT().Shutdown(gomock.Any()).MinTimes(1).Return(nil)

	srv, err := aibridged.New(
		t.Context(),
		pool,
		func(ctx context.Context) (aibridged.DRPCClient, error) {
			return client, nil
		}, logger, testTracer)
	require.NoError(t, err, "create new aibridged")
	t.Cleanup(func() {
		srv.Shutdown(context.Background())
	})

	return srv, client, pool
}

// mockDRPCConn is a mock implementation of drpc.Conn
type mockDRPCConn struct{}

func (*mockDRPCConn) Close() error              { return nil }
func (*mockDRPCConn) Closed() <-chan struct{}   { ch := make(chan struct{}); return ch }
func (*mockDRPCConn) Transport() drpc.Transport { return nil }
func (*mockDRPCConn) Invoke(ctx context.Context, rpc string, enc drpc.Encoding, in, out drpc.Message) error {
	return nil
}

func (*mockDRPCConn) NewStream(ctx context.Context, rpc string, enc drpc.Encoding) (drpc.Stream, error) {
	// nolint:nilnil // Chillchill.
	return nil, nil
}

func TestServeHTTP_FailureModes(t *testing.T) {
	t.Parallel()

	defaultHeaders := map[string]string{"Authorization": "Bearer key"}
	httpClient := &http.Client{}

	cases := []struct {
		name           string
		reqHeaders     map[string]string
		applyMocksFn   func(client *mock.MockDRPCClient, pool *mock.MockPooler)
		dialerFn       aibridged.Dialer
		contextFn      func() context.Context
		expectedErr    error
		expectedStatus int
	}{
		// Authnz-related failures.
		{
			name:           "no auth key",
			reqHeaders:     make(map[string]string),
			expectedErr:    aibridged.ErrNoAuthKey,
			expectedStatus: http.StatusBadRequest,
		},
		{
			name: "unrecognized header",
			reqHeaders: map[string]string{
				codersdk.SessionTokenHeader: "key", // Coder-Session-Token is not supported; requests originate with AI clients, not coder CLI.
			},
			applyMocksFn:   func(client *mock.MockDRPCClient, _ *mock.MockPooler) {},
			expectedErr:    aibridged.ErrNoAuthKey,
			expectedStatus: http.StatusBadRequest,
		},
		{
			name: "unauthorized",
			applyMocksFn: func(client *mock.MockDRPCClient, _ *mock.MockPooler) {
				client.EXPECT().IsAuthorized(gomock.Any(), gomock.Any()).AnyTimes().Return(nil, xerrors.New("not authorized"))
			},
			expectedErr:    aibridged.ErrUnauthorized,
			expectedStatus: http.StatusForbidden,
		},
		{
			name: "invalid key owner ID",
			applyMocksFn: func(client *mock.MockDRPCClient, _ *mock.MockPooler) {
				client.EXPECT().IsAuthorized(gomock.Any(), gomock.Any()).AnyTimes().Return(&proto.IsAuthorizedResponse{OwnerId: "oops"}, nil)
			},
			expectedErr:    aibridged.ErrUnauthorized,
			expectedStatus: http.StatusForbidden,
		},

		// TODO: coderd connection-related failures.

		// Pool-related failures.
		{
			name: "pool instance",
			applyMocksFn: func(client *mock.MockDRPCClient, pool *mock.MockPooler) {
				// Should pass authorization.
				client.EXPECT().IsAuthorized(gomock.Any(), gomock.Any()).AnyTimes().Return(&proto.IsAuthorizedResponse{OwnerId: uuid.NewString()}, nil)
				// But fail when acquiring a pool instance.
				pool.EXPECT().Acquire(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).AnyTimes().Return(nil, xerrors.New("oops"))
			},
			expectedErr:    aibridged.ErrAcquireRequestHandler,
			expectedStatus: http.StatusInternalServerError,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			srv, client, pool := newTestServer(t)
			conn := &mockDRPCConn{}
			client.EXPECT().DRPCConn().AnyTimes().Return(conn)

			if tc.applyMocksFn != nil {
				tc.applyMocksFn(client, pool)
			}

			httpSrv := httptest.NewServer(srv)

			ctx := testutil.Context(t, testutil.WaitShort)
			req, err := http.NewRequestWithContext(ctx, http.MethodPost, httpSrv.URL+"/openai/v1/chat/completions", nil)
			require.NoError(t, err, "make request to test server")

			headers := defaultHeaders
			if tc.reqHeaders != nil {
				headers = tc.reqHeaders
			}
			for k, v := range headers {
				req.Header.Set(k, v)
			}

			resp, err := httpClient.Do(req)
			t.Cleanup(func() {
				if resp == nil || resp.Body == nil {
					return
				}
				resp.Body.Close()
			})
			require.NoError(t, err)

			body, err := io.ReadAll(resp.Body)
			require.NoError(t, err, "read response body")
			require.Contains(t, string(body), tc.expectedErr.Error())
			require.Equal(t, tc.expectedStatus, resp.StatusCode)
		})
	}
}

func TestExtractAuthToken(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name        string
		headers     map[string]string
		expectedKey string
	}{
		{
			name: "none",
		},
		{
			name:    "authorization/invalid",
			headers: map[string]string{"authorization": "invalid"},
		},
		{
			name:    "authorization/bearer empty",
			headers: map[string]string{"authorization": "bearer"},
		},
		{
			name:        "authorization/bearer ok",
			headers:     map[string]string{"authorization": "bearer key"},
			expectedKey: "key",
		},
		{
			name:        "authorization/case",
			headers:     map[string]string{"AUTHORIZATION": "BEARer key"},
			expectedKey: "key",
		},
		{
			name:    "x-api-key/empty",
			headers: map[string]string{"X-Api-Key": ""},
		},
		{
			name:        "x-api-key/ok",
			headers:     map[string]string{"X-Api-Key": "key"},
			expectedKey: "key",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			headers := make(http.Header, len(tc.headers))
			for k, v := range tc.headers {
				headers.Add(k, v)
			}
			key := agplaibridge.ExtractAuthToken(headers)
			require.Equal(t, tc.expectedKey, key)
		})
	}
}

var _ http.Handler = &mockHandler{}

type mockHandler struct{}

func (*mockHandler) ServeHTTP(rw http.ResponseWriter, r *http.Request) {
	rw.WriteHeader(http.StatusOK)
	_, _ = rw.Write([]byte(r.URL.Path))
}

// TestRouting validates that a request which originates with aibridged will be handled
// by coder/aibridge's handling logic in a provider-specific manner.
// We must validate that logic that pertains to coder/coder is exercised.
// aibridge will only handle certain routes; we don't need to test these exhaustively
// (that's coder/aibridge's responsibility), but we do need to validate that it handles
// requests correctly.
func TestRouting(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name           string
		path           string
		expectedStatus int
		expectedHits   int // Expected hits to the upstream server.
	}{
		{
			name:           "unsupported",
			path:           "/this-route-does-not-exist",
			expectedStatus: http.StatusNotFound,
			expectedHits:   0,
		},
		{
			name:           "openai chat completions",
			path:           "/openai/v1/chat/completions",
			expectedStatus: http.StatusTeapot, // Nonsense status to indicate server was hit.
			expectedHits:   1,
		},
		{
			name:           "anthropic messages",
			path:           "/anthropic/v1/messages",
			expectedStatus: http.StatusTeapot, // Nonsense status to indicate server was hit.
			expectedHits:   1,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			// Setup mock upstream AI server.
			upstreamSrv := &mockAIUpstreamServer{}
			openaiSrv := httptest.NewServer(upstreamSrv)
			antSrv := httptest.NewServer(upstreamSrv)
			t.Cleanup(openaiSrv.Close)
			t.Cleanup(antSrv.Close)

			// Setup.
			logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true})
			ctrl := gomock.NewController(t)
			client := mock.NewMockDRPCClient(ctrl)

			providers := []aibridge.Provider{
				aibridge.NewOpenAIProvider(aibridge.OpenAIConfig{BaseURL: openaiSrv.URL}),
				aibridge.NewAnthropicProvider(aibridge.AnthropicConfig{BaseURL: antSrv.URL}, nil),
			}
			pool, err := aibridged.NewCachedBridgePool(aibridged.DefaultPoolOptions, providers, logger, nil, testTracer)
			require.NoError(t, err)
			conn := &mockDRPCConn{}
			client.EXPECT().DRPCConn().AnyTimes().Return(conn)

			client.EXPECT().IsAuthorized(gomock.Any(), gomock.Any()).AnyTimes().Return(&proto.IsAuthorizedResponse{OwnerId: uuid.NewString()}, nil)
			client.EXPECT().GetMCPServerConfigs(gomock.Any(), gomock.Any()).AnyTimes().Return(&proto.GetMCPServerConfigsResponse{}, nil)
			// This is the only recording we really care about in this test. This is called before the provider-specific logic processes
			// the incoming request, and anything beyond that is the responsibility of coder/aibridge to test.
			var interceptionID string
			client.EXPECT().RecordInterception(gomock.Any(), gomock.Any()).Times(tc.expectedHits).DoAndReturn(func(ctx context.Context, in *proto.RecordInterceptionRequest) (*proto.RecordInterceptionResponse, error) {
				interceptionID = in.GetId()
				return &proto.RecordInterceptionResponse{}, nil
			})
			client.EXPECT().RecordInterceptionEnded(gomock.Any(), gomock.Any()).Times(tc.expectedHits)

			// Given: aibridged is started.
			srv, err := aibridged.New(t.Context(), pool, func(ctx context.Context) (aibridged.DRPCClient, error) {
				return client, nil
			}, logger, testTracer)
			require.NoError(t, err, "create new aibridged")
			t.Cleanup(func() {
				_ = srv.Shutdown(testutil.Context(t, testutil.WaitShort))
			})

			// When: a request is made to aibridged.
			ctx := testutil.Context(t, testutil.WaitShort)
			req, err := http.NewRequestWithContext(ctx, http.MethodPost, tc.path, bytes.NewBufferString(`{}`))
			require.NoError(t, err, "make request to test server")
			req.Header.Add("Authorization", "Bearer key")
			req.Header.Add("Accept", "application/json")

			// When: aibridged handles the request.
			rec := httptest.NewRecorder()
			srv.ServeHTTP(rec, req)

			// Then: the upstream server will have received a number of hits.
			// NOTE: we *expect* the interceptions to fail because [mockAIUpstreamServer] returns a nonsense status code.
			// We only need to test that the request was routed, NOT processed.
			require.Equal(t, tc.expectedStatus, rec.Code)
			assert.EqualValues(t, tc.expectedHits, upstreamSrv.Hits())
			if tc.expectedHits > 0 {
				_, err = uuid.Parse(interceptionID)
				require.NoError(t, err, "parse interception ID")
			}
		})
	}
}
