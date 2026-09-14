package coderd_test

import (
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/aibridge"
	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/testutil"
)

// stubTransportFactory wires a deterministic handler through the
// AIBridgeTransportFactory hook so the AGPL side of the in-memory pipe can be
// exercised without pulling coderd/aibridged in.
type stubTransportFactory struct {
	handler http.Handler
	calls   chan callRecord
}

type callRecord struct {
	providerName string
	source       aibridge.Source
}

func (f *stubTransportFactory) TransportFor(providerName string, source aibridge.Source) (http.RoundTripper, error) {
	f.calls <- callRecord{providerName: providerName, source: source}
	return &handlerRoundTripper{handler: f.handler}, nil
}

// handlerRoundTripper is a minimal http.RoundTripper for the AGPL test. It
// does not stream; coderd/aibridged.transport_test.go already covers
// streaming semantics.
type handlerRoundTripper struct{ handler http.Handler }

func (h *handlerRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	rec := httptest.NewRecorder()
	h.handler.ServeHTTP(rec, req)
	resp := rec.Result()
	resp.Request = req
	return resp, nil
}

// Verify that a factory stored on coderd.API.AIBridgeTransportFactory is
// observable through the normal API lifecycle: cli/server.go registers it
// when the bridge daemon starts (see RegisterInMemoryAIBridgedHTTPHandler).
func TestAIBridgeTransportFactory_Registration(t *testing.T) {
	t.Parallel()

	_, _, api := coderdtest.NewWithAPI(t, nil)

	require.Nil(t, api.AIBridgeTransportFactory.Load(),
		"AGPL coderd must not pre-populate the factory")

	stub := &stubTransportFactory{
		handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"bridged":true}`))
		}),
		calls: make(chan callRecord, 4),
	}

	var asInterface aibridge.TransportFactory = stub
	api.AIBridgeTransportFactory.Store(&asInterface)

	loaded := api.AIBridgeTransportFactory.Load()
	require.NotNil(t, loaded)

	providerName := "openai"
	rt, err := (*loaded).TransportFor(providerName, aibridge.SourceAgents)
	require.NoError(t, err)
	require.NotNil(t, rt)

	select {
	case got := <-stub.calls:
		require.Equal(t, providerName, got.providerName)
		require.Equal(t, aibridge.SourceAgents, got.source)
	default:
		t.Fatal("factory was not invoked")
	}

	ctx := testutil.Context(t, testutil.WaitShort)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "http://aibridge/v1/messages", nil)
	require.NoError(t, err)

	client := &http.Client{Transport: rt}
	resp, err := client.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	require.Equal(t, `{"bridged":true}`, string(body))
	require.Equal(t, "application/json", resp.Header.Get("Content-Type"))
}
