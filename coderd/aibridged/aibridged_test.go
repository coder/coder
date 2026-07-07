package aibridged_test

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
	"golang.org/x/xerrors"
	"storj.io/drpc"

	"cdr.dev/slog/v3/sloggers/slogtest"
	"github.com/coder/coder/v2/aibridge"
	"github.com/coder/coder/v2/aibridge/aibridgetest"
	"github.com/coder/coder/v2/aibridge/intercept"
	"github.com/coder/coder/v2/aibridge/keypool"
	agplaibridge "github.com/coder/coder/v2/coderd/aibridge"
	"github.com/coder/coder/v2/coderd/aibridged"
	mock "github.com/coder/coder/v2/coderd/aibridged/aibridgedmock"
	"github.com/coder/coder/v2/coderd/aibridged/proto"
	"github.com/coder/coder/v2/coderd/util/ptr"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/testutil"
	"github.com/coder/quartz"
)

// singleKeyPool builds a centralized key pool containing a single key.
func singleKeyPool(t *testing.T, name, key string) *keypool.Pool {
	t.Helper()
	pool, err := keypool.New(name, []string{key}, quartz.NewReal(), nil)
	require.NoError(t, err)
	return pool
}

func newTestServer(t *testing.T) (*aibridged.Server, *mock.MockDRPCClient, *mock.MockPooler) {
	t.Helper()
	return newTestServerWithDialer(t, nil, nil)
}

func newTestServerWithDialer(t *testing.T, dialer aibridged.Dialer, loggerOptions *slogtest.Options) (*aibridged.Server, *mock.MockDRPCClient, *mock.MockPooler) {
	t.Helper()

	logger := slogtest.Make(t, loggerOptions)
	ctrl := gomock.NewController(t)
	client := mock.NewMockDRPCClient(ctrl)
	pool := mock.NewMockPooler(ctrl)

	conn := &mockDRPCConn{}
	client.EXPECT().DRPCConn().AnyTimes().Return(conn)
	pool.EXPECT().Shutdown(gomock.Any()).MinTimes(1).Return(nil)

	if dialer == nil {
		dialer = func(ctx context.Context) (aibridged.DRPCClient, error) {
			return client, nil
		}
	}
	srv, err := aibridged.New(t.Context(), pool, dialer, logger, testTracer)
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

func sdkError(status int, message string) error {
	return codersdk.ReadBodyAsError(&http.Response{
		StatusCode: status,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       io.NopCloser(bytes.NewBufferString(`{"message":"` + message + `"}`)),
	})
}

func TestClient_TransientDialErrorRetries(t *testing.T) {
	t.Parallel()

	var calls atomic.Int32
	ctrl := gomock.NewController(t)
	client := mock.NewMockDRPCClient(ctrl)
	client.EXPECT().DRPCConn().AnyTimes().Return(&mockDRPCConn{})
	pool := mock.NewMockPooler(ctrl)
	pool.EXPECT().Shutdown(gomock.Any()).MinTimes(1).Return(nil)
	dialFc := func(context.Context) (aibridged.DRPCClient, error) {
		if calls.Add(1) == 1 {
			return nil, sdkError(http.StatusInternalServerError, "internal error")
		}
		return client, nil
	}

	srv, err := aibridged.New(t.Context(), pool, dialFc, slogtest.Make(t, nil), testTracer)
	require.NoError(t, err)
	t.Cleanup(func() { _ = srv.Shutdown(context.Background()) })

	_, err = srv.ClientContext(testutil.Context(t, testutil.WaitShort))
	require.NoError(t, err)
	require.Equal(t, int32(2), calls.Load())
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
		ignoreLogs     bool
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

		// Coderd connection-related failures.
		{
			name: "fatal bad request dial error",
			dialerFn: func(context.Context) (aibridged.DRPCClient, error) {
				return nil, sdkError(http.StatusBadRequest, "bad request")
			},
			ignoreLogs:     true,
			expectedErr:    aibridged.ErrConnect,
			expectedStatus: http.StatusServiceUnavailable,
		},
		{
			name: "fatal unauthorized dial error",
			dialerFn: func(context.Context) (aibridged.DRPCClient, error) {
				return nil, sdkError(http.StatusUnauthorized, "unauthorized")
			},
			ignoreLogs:     true,
			expectedErr:    aibridged.ErrConnect,
			expectedStatus: http.StatusServiceUnavailable,
		},
		{
			name: "fatal forbidden dial error",
			dialerFn: func(context.Context) (aibridged.DRPCClient, error) {
				return nil, sdkError(http.StatusForbidden, "forbidden")
			},
			ignoreLogs:     true,
			expectedErr:    aibridged.ErrConnect,
			expectedStatus: http.StatusServiceUnavailable,
		},

		// Budget-related failures.
		{
			name: "budget exceeded",
			applyMocksFn: func(client *mock.MockDRPCClient, _ *mock.MockPooler) {
				// Authorization passes.
				client.EXPECT().IsAuthorized(gomock.Any(), gomock.Any()).AnyTimes().Return(&proto.IsAuthorizedResponse{OwnerId: uuid.NewString()}, nil)
				client.EXPECT().IsBudgetExceeded(gomock.Any(), gomock.Any()).AnyTimes().Return(&proto.IsBudgetExceededResponse{
					Exceeded:         true,
					SpendLimitMicros: ptr.Ref(int64(1_000)),
				}, nil)
			},
			expectedErr:    xerrors.New("AI budget of"),
			expectedStatus: http.StatusForbidden,
		},
		{
			name: "budget check failed",
			applyMocksFn: func(client *mock.MockDRPCClient, _ *mock.MockPooler) {
				// Authorization passes.
				client.EXPECT().IsAuthorized(gomock.Any(), gomock.Any()).AnyTimes().Return(&proto.IsAuthorizedResponse{OwnerId: uuid.NewString()}, nil)
				client.EXPECT().IsBudgetExceeded(gomock.Any(), gomock.Any()).AnyTimes().Return(nil, xerrors.New("oops"))
			},
			expectedErr:    aibridged.ErrBudgetCheck,
			expectedStatus: http.StatusInternalServerError,
		},

		// Pool-related failures.
		{
			name: "pool instance",
			applyMocksFn: func(client *mock.MockDRPCClient, pool *mock.MockPooler) {
				// Should pass authorization and budget check.
				client.EXPECT().IsAuthorized(gomock.Any(), gomock.Any()).AnyTimes().Return(&proto.IsAuthorizedResponse{OwnerId: uuid.NewString()}, nil)
				client.EXPECT().IsBudgetExceeded(gomock.Any(), gomock.Any()).AnyTimes().Return(&proto.IsBudgetExceededResponse{}, nil)
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

			var loggerOptions *slogtest.Options
			if tc.ignoreLogs {
				loggerOptions = &slogtest.Options{IgnoreErrors: true}
			}
			srv, client, pool := newTestServerWithDialer(t, tc.dialerFn, loggerOptions)
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

// When the request context carries a delegated API key ID (set by the
// in-process transport on behalf of a trusted caller like chatd), the handler
// must authenticate via the key_id field, skipping the header-based key
// extraction entirely. Validation succeeds or fails exactly as it would for a
// real API key. Delegation is orthogonal to BYOK: in BYOK mode the user's own
// LLM credentials must still be forwarded upstream while the Coder governance
// token is stripped.
func TestServeHTTP_DelegatedAPIKey(t *testing.T) {
	t.Parallel()

	const testKeyID = "abcdef1234"

	tests := []struct {
		name          string
		reqHeaders    map[string]string
		applyMocks    func(t *testing.T, client *mock.MockDRPCClient, pool *mock.MockPooler, mockH *mockHandler)
		expectStatus  int
		expectHandled bool
		expectPresent map[string]string
		expectAbsent  []string
	}{
		{
			// Delegated + centralized: identity comes from the
			// api key ID on the context, in lieu of a session
			// token. No header credentials are sent and SessionKey
			// is empty downstream.
			name: "valid centralized",
			applyMocks: func(t *testing.T, client *mock.MockDRPCClient, pool *mock.MockPooler, mockH *mockHandler) {
				client.EXPECT().IsAuthorized(gomock.Any(), gomock.Any()).DoAndReturn(
					func(_ context.Context, in *proto.IsAuthorizedRequest) (*proto.IsAuthorizedResponse, error) {
						assert.Equal(t, testKeyID, in.GetKeyId(), "handler must use KeyId for delegated requests")
						assert.Empty(t, in.GetKey(), "handler must not set Key for delegated requests")
						return &proto.IsAuthorizedResponse{
							OwnerId:  uuid.NewString(),
							ApiKeyId: testKeyID,
							Username: "u",
						}, nil
					})
				client.EXPECT().IsBudgetExceeded(gomock.Any(), gomock.Any()).Return(&proto.IsBudgetExceededResponse{}, nil)
				pool.EXPECT().Acquire(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).DoAndReturn(
					func(_ context.Context, req aibridged.Request, _ aibridged.ClientFunc, _ aibridged.MCPProxyBuilder) (http.Handler, error) {
						assert.Empty(t, req.SessionKey,
							"delegated centralized request carries no session token")
						return mockH, nil
					})
			},
			expectStatus:  http.StatusOK,
			expectHandled: true,
			expectAbsent: []string{
				"Authorization",
				"X-Api-Key",
				agplaibridge.HeaderCoderToken,
			},
		},
		{
			name: "valid BYOK preserves user credentials",
			reqHeaders: map[string]string{
				// Marks BYOK; this header must be stripped before
				// forwarding upstream. Its value is what gets
				// surfaced downstream as the SessionKey because
				// ExtractAuthToken prefers HeaderCoderToken.
				agplaibridge.HeaderCoderToken: "coder-token-byok",
				// The user's own LLM credential; must be preserved.
				"Authorization": "Bearer sk-ant-oat01-user-token",
			},
			applyMocks: func(t *testing.T, client *mock.MockDRPCClient, pool *mock.MockPooler, mockH *mockHandler) {
				client.EXPECT().IsAuthorized(gomock.Any(), gomock.Any()).Return(&proto.IsAuthorizedResponse{
					OwnerId:  uuid.NewString(),
					ApiKeyId: testKeyID,
					Username: "u",
				}, nil)
				client.EXPECT().IsBudgetExceeded(gomock.Any(), gomock.Any()).Return(&proto.IsBudgetExceededResponse{}, nil)
				pool.EXPECT().Acquire(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).DoAndReturn(
					func(_ context.Context, req aibridged.Request, _ aibridged.ClientFunc, _ aibridged.MCPProxyBuilder) (http.Handler, error) {
						assert.Equal(t, "coder-token-byok", req.SessionKey,
							"BYOK delegated request must still surface the extracted Coder token as SessionKey")
						return mockH, nil
					})
			},
			expectStatus:  http.StatusOK,
			expectHandled: true,
			expectPresent: map[string]string{
				"Authorization": "Bearer sk-ant-oat01-user-token",
			},
			expectAbsent: []string{
				agplaibridge.HeaderCoderToken,
			},
		},
		{
			name: "invalid",
			applyMocks: func(_ *testing.T, client *mock.MockDRPCClient, _ *mock.MockPooler, _ *mockHandler) {
				client.EXPECT().IsAuthorized(gomock.Any(), gomock.Any()).Return(nil, xerrors.New("unknown key"))
			},
			expectStatus: http.StatusForbidden,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			srv, client, pool := newTestServer(t)
			conn := &mockDRPCConn{}
			client.EXPECT().DRPCConn().AnyTimes().Return(conn)
			mockH := &mockHandler{}
			tc.applyMocks(t, client, pool, mockH)

			ctx := agplaibridge.WithDelegatedAPIKeyID(testutil.Context(t, testutil.WaitShort), testKeyID)
			req, err := http.NewRequestWithContext(ctx, http.MethodPost, "http://aibridge/openai/v1/chat/completions", nil)
			require.NoError(t, err)
			for k, v := range tc.reqHeaders {
				req.Header.Set(k, v)
			}

			rw := httptest.NewRecorder()
			srv.ServeHTTP(rw, req)

			require.Equal(t, tc.expectStatus, rw.Code)
			if tc.expectHandled {
				require.NotNil(t, mockH.headersReceived, "downstream handler must be invoked")
				for h, v := range tc.expectPresent {
					require.Equal(t, v, mockH.headersReceived.Get(h), "header %q must be preserved", h)
				}
				for _, h := range tc.expectAbsent {
					require.Empty(t, mockH.headersReceived.Get(h), "header %q must be stripped", h)
				}
			} else {
				require.Nil(t, mockH.headersReceived, "downstream handler must not be invoked on auth failure")
			}
		})
	}
}

// End-to-end: a real transport factory wired to a real server, with BYOK in
// effect. The delegated key ID identifies the user (no Coder token over the
// wire) while the user's own LLM credentials in Authorization must flow
// through to the downstream handler. The Coder governance token, if set by
// the caller, must be stripped.
func TestServeHTTP_DelegatedAPIKey_BYOK_Integration(t *testing.T) {
	t.Parallel()

	const (
		testKeyID = "abcdef1234"
		// nolint:gosec // Fake LLM credential for assertion comparison.
		userLLMToken = "Bearer sk-ant-oat01-user-byok-token"
	)

	srv, client, pool := newTestServer(t)
	conn := &mockDRPCConn{}
	client.EXPECT().DRPCConn().AnyTimes().Return(conn)
	mockH := &mockHandler{}

	client.EXPECT().IsAuthorized(gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, in *proto.IsAuthorizedRequest) (*proto.IsAuthorizedResponse, error) {
			assert.Equal(t, testKeyID, in.GetKeyId(), "delegated identity must be carried in KeyId")
			assert.Empty(t, in.GetKey(), "Key must not be set on delegated requests")
			return &proto.IsAuthorizedResponse{
				OwnerId:  uuid.NewString(),
				ApiKeyId: testKeyID,
				Username: "u",
			}, nil
		})
	client.EXPECT().IsBudgetExceeded(gomock.Any(), gomock.Any()).Return(&proto.IsBudgetExceededResponse{}, nil)
	pool.EXPECT().Acquire(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(mockH, nil)

	factory := aibridged.NewTransportFactory(srv)
	rt, err := factory.TransportFor("openai", agplaibridge.SourceAgents)
	require.NoError(t, err)

	ctx := agplaibridge.WithDelegatedAPIKeyID(testutil.Context(t, testutil.WaitShort), testKeyID)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "http://aibridge/anthropic/v1/messages", nil)
	require.NoError(t, err)
	// HeaderCoderToken marks the request as BYOK. Its value is irrelevant on
	// the delegated path (identity comes from context) and it must be
	// stripped before forwarding upstream.
	req.Header.Set(agplaibridge.HeaderCoderToken, "ignored-on-delegated-path")
	// The user's own LLM credential; must reach the downstream handler.
	req.Header.Set("Authorization", userLLMToken)

	resp, err := rt.RoundTrip(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)

	require.NotNil(t, mockH.headersReceived, "downstream handler must be invoked")
	require.Equal(t, userLLMToken, mockH.headersReceived.Get("Authorization"),
		"user's BYOK credential must be preserved end-to-end")
	require.Empty(t, mockH.headersReceived.Get(agplaibridge.HeaderCoderToken),
		"Coder governance token must be stripped before forwarding upstream")
}

// End-to-end: a real transport factory wired to a real server. Verifies the
// delegated key ID survives the in-memory round-trip and is treated as the
// authoritative caller identity by the handler, without any HTTP-layer header
// extraction.
func TestServeHTTP_DelegatedAPIKey_Integration(t *testing.T) {
	t.Parallel()

	const testKeyID = "abcdef1234"

	srv, client, pool := newTestServer(t)
	conn := &mockDRPCConn{}
	client.EXPECT().DRPCConn().AnyTimes().Return(conn)
	mockH := &mockHandler{}

	client.EXPECT().IsAuthorized(gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, in *proto.IsAuthorizedRequest) (*proto.IsAuthorizedResponse, error) {
			assert.Equal(t, testKeyID, in.GetKeyId())
			assert.Empty(t, in.GetKey())
			return &proto.IsAuthorizedResponse{
				OwnerId:  uuid.NewString(),
				ApiKeyId: testKeyID,
				Username: "u",
			}, nil
		})
	client.EXPECT().IsBudgetExceeded(gomock.Any(), gomock.Any()).Return(&proto.IsBudgetExceededResponse{}, nil)
	pool.EXPECT().Acquire(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(mockH, nil)

	factory := aibridged.NewTransportFactory(srv)
	rt, err := factory.TransportFor("openai", agplaibridge.SourceAgents)
	require.NoError(t, err)

	ctx := agplaibridge.WithDelegatedAPIKeyID(testutil.Context(t, testutil.WaitShort), testKeyID)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "http://aibridge/openai/v1/chat/completions", nil)
	require.NoError(t, err)

	resp, err := rt.RoundTrip(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	require.Equal(t, http.StatusOK, resp.StatusCode)
	require.NotNil(t, mockH.headersReceived, "downstream handler must observe the delegated request")
}

func TestServeHTTP_StripCoderToken(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name          string
		reqHeaders    map[string]string
		expectPresent map[string]string // header → expected value
		expectAbsent  []string          // headers that must be gone
	}{
		{
			// Centralized: the client sets Authorization and X-Api-Key,
			// but does not include HeaderCoderToken.
			// All auth headers are stripped.
			name: "centralized",
			reqHeaders: map[string]string{
				"Authorization": "Bearer coder-token",
				"X-Api-Key":     "sk-ant-api03-user-key",
			},
			expectAbsent: []string{
				"Authorization",
				"X-Api-Key",
				agplaibridge.HeaderCoderToken,
			},
		},
		{
			// BYOK with access token: Coder token in BYOK header,
			// user's access token in Authorization. Only the
			// BYOK header is stripped.
			name: "byok bearer token",
			reqHeaders: map[string]string{
				agplaibridge.HeaderCoderToken: "coder-token",
				"Authorization":               "Bearer sk-ant-oat01-user-oauth-token",
			},
			expectPresent: map[string]string{
				"Authorization": "Bearer sk-ant-oat01-user-oauth-token",
			},
			expectAbsent: []string{
				agplaibridge.HeaderCoderToken,
			},
		},
		{
			// BYOK with personal API key: Coder token in BYOK header,
			// user's API key in X-Api-Key. Only the BYOK header is
			// stripped.
			name: "byok api key",
			reqHeaders: map[string]string{
				agplaibridge.HeaderCoderToken: "coder-token",
				"X-Api-Key":                   "sk-ant-api03-user-key",
			},
			expectPresent: map[string]string{
				"X-Api-Key": "sk-ant-api03-user-key",
			},
			expectAbsent: []string{
				agplaibridge.HeaderCoderToken,
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			mockH := &mockHandler{}

			srv, client, pool := newTestServer(t)
			conn := &mockDRPCConn{}
			client.EXPECT().DRPCConn().AnyTimes().Return(conn)
			client.EXPECT().IsAuthorized(gomock.Any(), gomock.Any()).AnyTimes().Return(&proto.IsAuthorizedResponse{OwnerId: uuid.NewString()}, nil)
			client.EXPECT().IsBudgetExceeded(gomock.Any(), gomock.Any()).AnyTimes().Return(&proto.IsBudgetExceededResponse{}, nil)
			pool.EXPECT().Acquire(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).AnyTimes().Return(mockH, nil)

			httpSrv := httptest.NewServer(srv)
			t.Cleanup(httpSrv.Close)

			ctx := testutil.Context(t, testutil.WaitShort)
			req, err := http.NewRequestWithContext(ctx, http.MethodPost, httpSrv.URL+"/openai/v1/chat/completions", nil)
			require.NoError(t, err)

			for k, v := range tc.reqHeaders {
				req.Header.Set(k, v)
			}

			resp, err := http.DefaultClient.Do(req)
			require.NoError(t, err)
			defer resp.Body.Close()

			require.Equal(t, http.StatusOK, resp.StatusCode)
			require.NotNil(t, mockH.headersReceived)

			for header, expected := range tc.expectPresent {
				require.Equal(t, expected, mockH.headersReceived.Get(header),
					"header %q should be preserved with value %q", header, expected)
			}
			for _, header := range tc.expectAbsent {
				require.Empty(t, mockH.headersReceived.Get(header),
					"header %q should be stripped", header)
			}
			// HeaderCoderToken should always be stripped
			require.Empty(t, mockH.headersReceived.Get(agplaibridge.HeaderCoderToken),
				"header %q should be stripped", agplaibridge.HeaderCoderToken)
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
			name: "authorization/priority over x-api-key",
			headers: map[string]string{
				"Authorization": "Bearer auth-token",
				"X-Api-Key":     "api-key",
			},
			expectedKey: "auth-token",
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

		// BYOK: X-Coder-AI-Governance-Token carries the Coder
		// token and has the highest priority.
		{
			name:    "byok/empty",
			headers: map[string]string{agplaibridge.HeaderCoderToken: ""},
		},
		{
			name:        "byok/ok",
			headers:     map[string]string{agplaibridge.HeaderCoderToken: "coder-token"},
			expectedKey: "coder-token",
		},
		{
			name: "byok/priority over all",
			headers: map[string]string{
				agplaibridge.HeaderCoderToken: "coder-token",
				"Authorization":               "Bearer oauth-token",
				"X-Api-Key":                   "api-key",
			},
			expectedKey: "coder-token",
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

type mockHandler struct {
	headersReceived http.Header
}

func (h *mockHandler) ServeHTTP(rw http.ResponseWriter, r *http.Request) {
	h.headersReceived = r.Header.Clone()
	rw.WriteHeader(http.StatusOK)
	_, _ = rw.Write([]byte(r.URL.Path))
}

// TestServeHTTP_ActorHeaders validates that actor headers are correctly forwarded to
// upstream AI providers when SendActorHeaders is enabled in the provider configuration.
// These headers allow upstream providers to identify the user making the request for
// tracking and auditing purposes.
func TestServeHTTP_ActorHeaders(t *testing.T) {
	t.Parallel()

	testUsername := "testuser"
	testUserID := uuid.New()

	cases := []struct {
		path string
	}{
		// Not a complete set of paths; we're not testing the specific APIs - just the provider configs.
		{
			path: "/openai/v1/chat/completions",
		},
		{
			path: "/anthropic/v1/messages",
		},
	}

	for _, tc := range cases {
		t.Run(tc.path, func(t *testing.T) {
			t.Parallel()

			// Setup mock upstream AI server that captures headers.
			var receivedHeaders http.Header
			upstreamSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				receivedHeaders = r.Header.Clone()
				w.WriteHeader(http.StatusTeapot)
				_, _ = w.Write([]byte(`i am a teapot`))
			}))
			t.Cleanup(upstreamSrv.Close)

			// Setup with SendActorHeaders enabled.
			logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true})
			ctrl := gomock.NewController(t)
			client := mock.NewMockDRPCClient(ctrl)

			// Create providers with SendActorHeaders=true.
			providers := []aibridge.Provider{
				aibridge.NewOpenAIProvider(aibridge.OpenAIConfig{
					BaseURL:          upstreamSrv.URL,
					KeyPool:          singleKeyPool(t, "openai", "test-key"),
					SendActorHeaders: true,
				}),
				aibridgetest.NewAnthropicProvider(t, aibridge.AnthropicConfig{
					BaseURL:          upstreamSrv.URL,
					KeyPool:          singleKeyPool(t, "anthropic", "test-key"),
					SendActorHeaders: true,
				}, nil),
			}

			pool, err := aibridged.NewCachedBridgePool(aibridged.DefaultPoolOptions, providers, logger, nil, testTracer)
			require.NoError(t, err)
			conn := &mockDRPCConn{}
			client.EXPECT().DRPCConn().AnyTimes().Return(conn)

			// Return authorization response with user ID and username.
			client.EXPECT().IsAuthorized(gomock.Any(), gomock.Any()).AnyTimes().Return(&proto.IsAuthorizedResponse{
				OwnerId:  testUserID.String(),
				Username: testUsername,
			}, nil)
			client.EXPECT().IsBudgetExceeded(gomock.Any(), gomock.Any()).AnyTimes().Return(&proto.IsBudgetExceededResponse{}, nil)
			client.EXPECT().GetMCPServerConfigs(gomock.Any(), gomock.Any()).AnyTimes().Return(&proto.GetMCPServerConfigsResponse{}, nil)
			client.EXPECT().RecordInterception(gomock.Any(), gomock.Any()).AnyTimes().Return(&proto.RecordInterceptionResponse{}, nil)
			client.EXPECT().RecordInterceptionEnded(gomock.Any(), gomock.Any()).AnyTimes()

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

			// Then: the actor headers should be present in the upstream request.
			require.NotEmpty(t, receivedHeaders, "upstream server should have received headers")

			// Verify the actor ID header is present with the correct value.
			actorIDHeader := receivedHeaders.Get(intercept.ActorIDHeader())
			assert.Equal(t, testUserID.String(), actorIDHeader, "actor ID header should contain user ID")
			// Verify the actor metadata header for username is present.
			usernameHeader := receivedHeaders.Get(intercept.ActorMetadataHeader("Username"))
			assert.Equal(t, testUsername, usernameHeader, "actor metadata username header should contain username")
		})
	}
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
				aibridge.NewOpenAIProvider(aibridge.OpenAIConfig{BaseURL: openaiSrv.URL, KeyPool: singleKeyPool(t, "openai", "test-key")}),
				aibridgetest.NewAnthropicProvider(t, aibridge.AnthropicConfig{BaseURL: antSrv.URL, KeyPool: singleKeyPool(t, "anthropic", "test-key")}, nil),
			}
			pool, err := aibridged.NewCachedBridgePool(aibridged.DefaultPoolOptions, providers, logger, nil, testTracer)
			require.NoError(t, err)
			conn := &mockDRPCConn{}
			client.EXPECT().DRPCConn().AnyTimes().Return(conn)

			client.EXPECT().IsAuthorized(gomock.Any(), gomock.Any()).AnyTimes().Return(&proto.IsAuthorizedResponse{OwnerId: uuid.NewString()}, nil)
			client.EXPECT().IsBudgetExceeded(gomock.Any(), gomock.Any()).AnyTimes().Return(&proto.IsBudgetExceededResponse{}, nil)
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

// TestServeHTTP_StripInternalHeaders verifies that internal X-Coder-*
// headers are never forwarded to upstream LLM providers.
func TestServeHTTP_StripInternalHeaders(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name   string
		header string
		value  string
	}{
		{
			name:   "X-Coder-AI-Governance-Token",
			header: agplaibridge.HeaderCoderToken,
			value:  "coder-token",
		},
		{
			name:   "X-Coder-AI-Governance-Request-Id",
			header: agplaibridge.HeaderCoderRequestID,
			value:  uuid.NewString(),
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			mockH := &mockHandler{}

			srv, client, pool := newTestServer(t)
			conn := &mockDRPCConn{}
			client.EXPECT().DRPCConn().AnyTimes().Return(conn)
			client.EXPECT().IsAuthorized(gomock.Any(), gomock.Any()).AnyTimes().Return(&proto.IsAuthorizedResponse{OwnerId: uuid.NewString()}, nil)
			client.EXPECT().IsBudgetExceeded(gomock.Any(), gomock.Any()).AnyTimes().Return(&proto.IsBudgetExceededResponse{}, nil)
			pool.EXPECT().Acquire(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).AnyTimes().Return(mockH, nil)

			httpSrv := httptest.NewServer(srv)
			t.Cleanup(httpSrv.Close)

			ctx := testutil.Context(t, testutil.WaitShort)
			req, err := http.NewRequestWithContext(ctx, http.MethodPost, httpSrv.URL+"/anthropic/v1/messages", nil)
			require.NoError(t, err)

			// Always set a valid auth token so the request reaches
			// the upstream handler.
			req.Header.Set("Authorization", "Bearer coder-token")
			req.Header.Set(tc.header, tc.value)

			resp, err := http.DefaultClient.Do(req)
			require.NoError(t, err)
			defer resp.Body.Close()

			require.Equal(t, http.StatusOK, resp.StatusCode)
			require.NotNil(t, mockH.headersReceived)

			// Assert no X-Coder-* headers were forwarded upstream.
			for name := range mockH.headersReceived {
				require.NotContains(t, name, "X-Coder-",
					"internal header %q must not be forwarded to upstream providers", name)
			}
		})
	}
}
