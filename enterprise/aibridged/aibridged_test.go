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

	"cdr.dev/slog/v3/sloggers/slogtest"
	"github.com/coder/aibridge"
	"github.com/coder/aibridge/intercept"
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

func TestServeHTTP_CoderTokenRemoved(t *testing.T) {
	t.Parallel()

	mockHandler := &mockHandler{}

	srv, client, pool := newTestServer(t)
	conn := &mockDRPCConn{}
	client.EXPECT().DRPCConn().AnyTimes().Return(conn)
	client.EXPECT().IsAuthorized(gomock.Any(), gomock.Any()).AnyTimes().Return(&proto.IsAuthorizedResponse{OwnerId: uuid.NewString()}, nil)
	pool.EXPECT().Acquire(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).AnyTimes().Return(mockHandler, nil)

	httpSrv := httptest.NewServer(srv)
	t.Cleanup(httpSrv.Close)

	ctx := testutil.Context(t, testutil.WaitShort)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, httpSrv.URL+"/openai/v1/chat/completions", nil)
	require.NoError(t, err)

	// X-Coder-Token is used for authentication and should be stripped.
	// Other authorization headers should be preserved.
	req.Header.Set(agplaibridge.HeaderCoderAuth, "coder-token")
	req.Header.Set("Authorization", "Bearer some-token")
	req.Header.Set("X-Api-Key", "some-api-key")

	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	require.Equal(t, http.StatusOK, resp.StatusCode)

	// Verify X-Coder-Token was removed before forwarding to handler.
	require.NotNil(t, mockHandler.headersReceived)
	require.Empty(t, mockHandler.headersReceived.Get(agplaibridge.HeaderCoderAuth),
		"X-Coder-Token should be removed before forwarding to handler")

	// Verify other headers were preserved.
	require.Equal(t, "Bearer some-token", mockHandler.headersReceived.Get("Authorization"))
	require.Equal(t, "some-api-key", mockHandler.headersReceived.Get("X-Api-Key"))
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
			name:    "x-coder-token/empty",
			headers: map[string]string{agplaibridge.HeaderCoderAuth: ""},
		},
		{
			name:        "x-coder-token/ok",
			headers:     map[string]string{agplaibridge.HeaderCoderAuth: "coder-token"},
			expectedKey: "coder-token",
		},
		{
			name: "x-coder-token/priority over authorization",
			headers: map[string]string{
				agplaibridge.HeaderCoderAuth: "coder-token",
				"Authorization":              "Bearer other-token",
			},
			expectedKey: "coder-token",
		},
		{
			name: "x-coder-token/priority over x-api-key",
			headers: map[string]string{
				agplaibridge.HeaderCoderAuth: "coder-token",
				"X-Api-Key":                  "api-key",
			},
			expectedKey: "coder-token",
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
					SendActorHeaders: true,
				}),
				aibridge.NewAnthropicProvider(aibridge.AnthropicConfig{
					BaseURL:          upstreamSrv.URL,
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
