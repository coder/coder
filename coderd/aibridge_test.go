package coderd_test

import (
	"io"
	"net/http"
	"testing"

	"github.com/google/uuid"
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
	providerID   uuid.UUID
	isCoderAgent bool
}

//nolint:nilnil,revive // matches aibridge.TransportFactory contract.
func (f *stubTransportFactory) TransportFor(providerID uuid.UUID, isCoderAgent bool) (http.RoundTripper, error) {
	f.calls <- callRecord{providerID: providerID, isCoderAgent: isCoderAgent}
	if !isCoderAgent {
		return nil, nil
	}
	return &handlerRoundTripper{handler: f.handler}, nil
}

// handlerRoundTripper is a minimal http.RoundTripper for the AGPL test. It
// does not stream; coderd/aibridged.transport_test.go already covers
// streaming semantics.
type handlerRoundTripper struct{ handler http.Handler }

func (h *handlerRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	rec := &captureWriter{header: http.Header{}, status: http.StatusOK}
	h.handler.ServeHTTP(rec, req)
	pr, pw := io.Pipe()
	go func() {
		defer pw.Close()
		_, _ = pw.Write(rec.body)
	}()
	return &http.Response{
		StatusCode: rec.status,
		Header:     rec.header,
		Body:       pr,
		Request:    req,
	}, nil
}

type captureWriter struct {
	header http.Header
	status int
	body   []byte
}

func (c *captureWriter) Header() http.Header    { return c.header }
func (c *captureWriter) WriteHeader(status int) { c.status = status }
func (c *captureWriter) Write(p []byte) (int, error) {
	c.body = append(c.body, p...)
	return len(p), nil
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

	providerID := uuid.New()
	rt, err := (*loaded).TransportFor(providerID, true)
	require.NoError(t, err)
	require.NotNil(t, rt)

	select {
	case got := <-stub.calls:
		require.Equal(t, providerID, got.providerID)
		require.True(t, got.isCoderAgent)
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

// External (non-coder-agent) traffic must fall through so chatd keeps calling
// upstream directly. This is the carve-out's licensing posture: the in-memory
// path is reserved for coder-agent traffic only.
func TestAIBridgeTransportFactory_NonCoderAgentFallsThrough(t *testing.T) {
	t.Parallel()

	_, _, api := coderdtest.NewWithAPI(t, nil)

	stub := &stubTransportFactory{
		handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			t.Fatal("handler must not be invoked for non-coder-agent traffic")
		}),
		calls: make(chan callRecord, 1),
	}
	var asInterface aibridge.TransportFactory = stub
	api.AIBridgeTransportFactory.Store(&asInterface)

	loaded := api.AIBridgeTransportFactory.Load()
	require.NotNil(t, loaded)

	rt, err := (*loaded).TransportFor(uuid.New(), false)
	require.NoError(t, err)
	require.Nil(t, rt, "external traffic must not get an in-memory transport")
}
