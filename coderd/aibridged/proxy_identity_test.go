package aibridged_test

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
	"golang.org/x/xerrors"

	"cdr.dev/slog/v3/sloggers/slogtest"
	"github.com/coder/coder/v2/aibridge"
	"github.com/coder/coder/v2/aibridge/config"
	agplaibridge "github.com/coder/coder/v2/coderd/aibridge"
	"github.com/coder/coder/v2/coderd/aibridged"
	mock "github.com/coder/coder/v2/coderd/aibridged/aibridgedmock"
	"github.com/coder/coder/v2/coderd/aibridged/proto"
	"github.com/coder/coder/v2/testutil"
)

func TestProxyRecorderUsesAuthenticatedAPIKeyIDPerRequest(t *testing.T) {
	t.Parallel()

	var upstreamMu sync.Mutex
	upstreamHeaders := map[string]http.Header{}
	shutdownUpstreamStarted := make(chan struct{})
	shutdownUpstreamReleaseCh := make(chan struct{})
	shutdownUpstreamRelease := sync.OnceFunc(func() { close(shutdownUpstreamReleaseCh) })
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		upstreamMu.Lock()
		upstreamHeaders[r.Header.Get("X-Test-ID")] = r.Header.Clone()
		upstreamMu.Unlock()
		if r.Header.Get("X-Test-ID") == "canceled" {
			w.WriteHeader(http.StatusOK)
			if f, ok := w.(http.Flusher); ok {
				f.Flush()
			}
			<-r.Context().Done()
			return
		}
		if r.Header.Get("X-Test-ID") == "shutdown" {
			close(shutdownUpstreamStarted)
			<-shutdownUpstreamReleaseCh
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	t.Cleanup(upstream.Close)

	ctrl := gomock.NewController(t)
	client := mock.NewMockDRPCClient(ctrl)
	client.EXPECT().DRPCConn().AnyTimes().Return(&mockDRPCConn{})
	client.EXPECT().GetMCPServerConfigs(gomock.Any(), gomock.Any()).Return(&proto.GetMCPServerConfigsResponse{}, nil)
	client.EXPECT().IsBudgetExceeded(gomock.Any(), gomock.Any()).AnyTimes().Return(&proto.IsBudgetExceededResponse{}, nil)

	owners := map[string]string{
		"direct-secret":   uuid.NewString(),
		"delegated-id":    uuid.NewString(),
		"canceled-id":     uuid.NewString(),
		"shutdown-secret": uuid.NewString(),
		"missing-id-key":  uuid.NewString(),
	}
	directWorkspaceID := uuid.New()
	delegatedWorkspaceID := uuid.New()
	canceledWorkspaceID := uuid.New()
	shutdownWorkspaceID := uuid.New()
	client.EXPECT().IsAuthorized(gomock.Any(), gomock.Any()).AnyTimes().DoAndReturn(func(_ context.Context, req *proto.IsAuthorizedRequest) (*proto.IsAuthorizedResponse, error) {
		apiKeyID := req.GetKeyId()
		if apiKeyID == "" {
			switch req.GetKey() {
			case "missing-id-key":
			case "shutdown-secret":
				apiKeyID = "shutdown-id"
			default:
				apiKeyID = "direct-id"
			}
		}
		lookup := req.GetKey()
		if lookup == "" {
			lookup = apiKeyID
		}
		response := &proto.IsAuthorizedResponse{OwnerId: owners[lookup], ApiKeyId: apiKeyID}
		switch lookup {
		case "direct-secret":
			response.WorkspaceId = directWorkspaceID.String()
		case "shutdown-secret":
			response.WorkspaceId = shutdownWorkspaceID.String()
		default:
			response.WorkspaceId = uuid.NewString()
		}
		return response, nil
	})

	var mu sync.Mutex
	starts := map[string]*proto.RecordInterceptionRequest{}
	ends := map[string]*proto.RecordInterceptionEndedRequest{}
	canceledStarted := make(chan struct{})
	var canceledEndContextErr error
	var canceledEndWorkspaceID uuid.UUID
	client.EXPECT().RecordInterception(gomock.Any(), gomock.Any()).Times(4).DoAndReturn(func(_ context.Context, req *proto.RecordInterceptionRequest) (*proto.RecordInterceptionResponse, error) {
		mu.Lock()
		starts[req.GetId()] = req
		if req.GetApiKeyId() == "canceled-id" {
			close(canceledStarted)
		}
		mu.Unlock()
		return &proto.RecordInterceptionResponse{}, nil
	})
	client.EXPECT().RecordInterceptionEnded(gomock.Any(), gomock.Any()).Times(4).DoAndReturn(func(ctx context.Context, req *proto.RecordInterceptionEndedRequest) (*proto.RecordInterceptionEndedResponse, error) {
		mu.Lock()
		ends[req.GetId()] = req
		if start := starts[req.GetId()]; start != nil && start.GetApiKeyId() == "canceled-id" {
			canceledEndContextErr = ctx.Err()
			if attr, ok := agplaibridge.AttributionFromContext(ctx); ok {
				canceledEndWorkspaceID = attr.WorkspaceID
			}
		}
		mu.Unlock()
		return &proto.RecordInterceptionEndedResponse{}, nil
	})

	srv, err := aibridged.New(t.Context(), func(context.Context) (aibridged.DRPCClient, error) { return client, nil }, slogtest.Make(t, nil), testTracer, proxyExperiments(), nil)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, srv.Shutdown(context.Background())) })
	// Registered after server shutdown so LIFO cleanup releases the blocked
	// request before shutdown waits for it.
	t.Cleanup(shutdownUpstreamRelease)
	waitReady(t, srv)
	require.NoError(t, srv.ReplaceProviders(t.Context(), []aibridge.Provider{
		aibridge.NewOpenAIProvider(config.OpenAI{BaseURL: upstream.URL}),
	}))

	requests := []*http.Request{}
	direct := httptest.NewRequest(http.MethodPost, "/openai/v1/chat/completions", bytes.NewBufferString(`{}`))
	direct.Header.Set(agplaibridge.HeaderCoderToken, "direct-secret")
	direct.Header.Set("Authorization", "Bearer provider-secret")
	direct.Header.Set("X-Test-ID", "direct")
	requests = append(requests, direct)
	delegatedCtx := agplaibridge.WithDelegatedAPIKeyID(t.Context(), "delegated-id")
	delegatedCtx = agplaibridge.WithDelegatedAttribution(delegatedCtx, agplaibridge.Attribution{WorkspaceID: delegatedWorkspaceID})
	delegated := httptest.NewRequest(http.MethodPost, "/openai/v1/chat/completions", bytes.NewBufferString(`{}`)).WithContext(delegatedCtx)
	delegated.Header.Set("X-Test-ID", "delegated")
	requests = append(requests, delegated)

	var wg sync.WaitGroup
	requestErrors := make(chan error, len(requests))
	for _, req := range requests {
		wg.Go(func() {
			rec := httptest.NewRecorder()
			srv.ServeHTTP(rec, req)
			if rec.Code != http.StatusNoContent {
				requestErrors <- xerrors.Errorf("request returned status %d", rec.Code)
				return
			}
			requestErrors <- nil
		})
	}
	wg.Wait()
	close(requestErrors)
	for err := range requestErrors {
		require.NoError(t, err)
	}

	transport, err := aibridged.NewTransportFactory(http.StripPrefix(agplaibridge.AIGatewayRootPath, srv)).TransportFor("openai", agplaibridge.SourceAgents)
	require.NoError(t, err)
	cancelCtx, cancel := context.WithCancel(t.Context())
	defer cancel()
	cancelCtx = agplaibridge.WithDelegatedAPIKeyID(cancelCtx, "canceled-id")
	cancelCtx = agplaibridge.WithDelegatedAttribution(cancelCtx, agplaibridge.Attribution{WorkspaceID: canceledWorkspaceID})
	canceled, err := http.NewRequestWithContext(cancelCtx, http.MethodPost, "http://upstream/v1/chat/completions", bytes.NewBufferString(`{}`))
	require.NoError(t, err)
	canceled.Header.Set("X-Test-ID", "canceled")
	canceledDone := make(chan error, 1)
	go func() {
		resp, err := transport.RoundTrip(canceled)
		if resp != nil {
			_, err = io.ReadAll(resp.Body)
			_ = resp.Body.Close()
		}
		canceledDone <- err
	}()
	testCtx := testutil.Context(t, testutil.WaitShort)
	testutil.TryReceive(testCtx, t, canceledStarted)
	cancel()
	require.ErrorIs(t, testutil.TryReceive(testCtx, t, canceledDone), context.Canceled)

	missingID := httptest.NewRequest(http.MethodPost, "/openai/v1/chat/completions", bytes.NewBufferString(`{}`))
	missingID.Header.Set(agplaibridge.HeaderCoderToken, "missing-id-key")
	missingID.Header.Set("Authorization", "Bearer provider-secret")
	missingID.Header.Set("X-Test-ID", "missing-id")
	missingResponse := httptest.NewRecorder()
	srv.ServeHTTP(missingResponse, missingID)
	require.Equal(t, http.StatusInternalServerError, missingResponse.Code)

	shutdownRequest := httptest.NewRequest(http.MethodPost, "/openai/v1/chat/completions", bytes.NewBufferString(`{}`))
	shutdownRequest.Header.Set(agplaibridge.HeaderCoderToken, "shutdown-secret")
	shutdownRequest.Header.Set("Authorization", "Bearer provider-secret")
	shutdownRequest.Header.Set("X-Test-ID", "shutdown")
	shutdownResponse := make(chan int, 1)
	go func() {
		rec := httptest.NewRecorder()
		srv.ServeHTTP(rec, shutdownRequest)
		shutdownResponse <- rec.Code
	}()
	testutil.TryReceive(testCtx, t, shutdownUpstreamStarted)

	shutdownDone := make(chan error, 1)
	go func() {
		shutdownDone <- srv.Shutdown(testCtx)
	}()
	require.Eventually(t, func() bool {
		probe := httptest.NewRequest(http.MethodPost, "/openai/v1/models", nil)
		probe.Header.Set(agplaibridge.HeaderCoderToken, "direct-secret")
		probe.Header.Set("Authorization", "Bearer provider-secret")
		rec := httptest.NewRecorder()
		srv.ServeHTTP(rec, probe)
		return rec.Code == http.StatusServiceUnavailable && rec.Body.String() == "AI Gateway is shutting down\n"
	}, testutil.WaitShort, testutil.IntervalFast)
	select {
	case err := <-shutdownDone:
		t.Fatalf("shutdown returned before the admitted request completed: %v", err)
	default:
	}
	shutdownUpstreamRelease()
	require.Equal(t, http.StatusNoContent, testutil.TryReceive(testCtx, t, shutdownResponse))
	require.NoError(t, testutil.TryReceive(testCtx, t, shutdownDone))

	require.Eventually(t, func() bool {
		mu.Lock()
		defer mu.Unlock()
		return len(starts) == 4 && len(ends) == 4
	}, testutil.WaitShort, testutil.IntervalFast)

	mu.Lock()
	defer mu.Unlock()
	seenKeys := map[string]*proto.RecordInterceptionRequest{}
	for id, start := range starts {
		seenKeys[start.GetApiKeyId()] = start
		require.Contains(t, ends, id)
	}
	require.Equal(t, owners["direct-secret"], seenKeys["direct-id"].GetInitiatorId())
	require.Equal(t, directWorkspaceID.String(), seenKeys["direct-id"].GetWorkspaceId())
	require.Equal(t, owners["delegated-id"], seenKeys["delegated-id"].GetInitiatorId())
	require.Equal(t, delegatedWorkspaceID.String(), seenKeys["delegated-id"].GetWorkspaceId())
	require.Equal(t, owners["canceled-id"], seenKeys["canceled-id"].GetInitiatorId())
	require.Equal(t, canceledWorkspaceID.String(), seenKeys["canceled-id"].GetWorkspaceId())
	require.Equal(t, owners["shutdown-secret"], seenKeys["shutdown-id"].GetInitiatorId())
	require.Equal(t, shutdownWorkspaceID.String(), seenKeys["shutdown-id"].GetWorkspaceId())
	require.NoError(t, canceledEndContextErr)
	require.Equal(t, canceledWorkspaceID, canceledEndWorkspaceID)
	upstreamMu.Lock()
	require.Equal(t, "Bearer provider-secret", upstreamHeaders["direct"].Get("Authorization"))
	require.Empty(t, upstreamHeaders["direct"].Get(agplaibridge.HeaderCoderToken))
	require.NotContains(t, upstreamHeaders, "missing-id")
	upstreamMu.Unlock()
}
