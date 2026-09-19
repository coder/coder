package exitnodesdk_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"tailscale.com/tailcfg"

	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/enterprise/exitnode/exitnodesdk"
	"github.com/coder/coder/v2/testutil"
	"github.com/coder/quartz"
)

//nolint:gosec // Test fixture, not a real credential.
const testToken = "7d9b3d9c-6d7e-4d3e-9d1a-2f3a4b5c6d7e:supersecret"

// fakeCoderd records the token it saw on each route and returns canned
// responses. Registration responses rotate through agentSets.
type fakeCoderd struct {
	t         *testing.T
	agentSets [][]uuid.UUID
	registers atomic.Int32
	flows     chan codersdk.ReportExitNodeFlowsRequest
	tokens    chan string
	wsHeaders chan http.Header
}

func newFakeCoderd(t *testing.T, agentSets ...[]uuid.UUID) (*fakeCoderd, *url.URL) {
	f := &fakeCoderd{
		t:         t,
		agentSets: agentSets,
		flows:     make(chan codersdk.ReportExitNodeFlowsRequest, 8),
		tokens:    make(chan string, 16),
		wsHeaders: make(chan http.Header, 1),
	}
	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/v2/exitnodes/me/register", func(w http.ResponseWriter, r *http.Request) {
		f.tokens <- r.Header.Get(codersdk.ExitNodeTokenHeader)
		n := int(f.registers.Add(1)) - 1
		agents := f.agentSets[min(n, len(f.agentSets)-1)]
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(codersdk.RegisterExitNodeResponse{
			DERPMap:  &tailcfg.DERPMap{Regions: map[int]*tailcfg.DERPRegion{}},
			AgentIDs: agents,
		})
	})
	mux.HandleFunc("POST /api/v2/exitnodes/me/flows", func(w http.ResponseWriter, r *http.Request) {
		f.tokens <- r.Header.Get(codersdk.ExitNodeTokenHeader)
		var req codersdk.ReportExitNodeFlowsRequest
		require.NoError(t, json.NewDecoder(r.Body).Decode(&req))
		f.flows <- req
		w.WriteHeader(http.StatusNoContent)
	})
	mux.HandleFunc("GET /api/v2/exitnodes/me/coordinate", func(w http.ResponseWriter, r *http.Request) {
		f.wsHeaders <- r.Header.Clone()
		// Refusing the upgrade is enough: the test only checks that the
		// token traveled with the websocket handshake.
		w.WriteHeader(http.StatusUnauthorized)
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	u, err := url.Parse(srv.URL)
	require.NoError(t, err)
	return f, u
}

func TestClient_SendsTokenHeader(t *testing.T) {
	t.Parallel()
	ctx := testutil.Context(t, testutil.WaitShort)
	agent := uuid.New()
	fake, u := newFakeCoderd(t, []uuid.UUID{agent})

	client := exitnodesdk.New(u, testToken)

	resp, err := client.Register(ctx, codersdk.RegisterExitNodeRequest{Version: "test"})
	require.NoError(t, err)
	require.Equal(t, []uuid.UUID{agent}, resp.AgentIDs)
	require.NotNil(t, resp.DERPMap)
	require.Equal(t, testToken, testutil.RequireReceive(ctx, t, fake.tokens))

	report := codersdk.ExitNodeFlowReport{FlowID: uuid.New(), AgentID: agent, Decision: codersdk.ExitNodeFlowDeny}
	err = client.ReportFlows(ctx, codersdk.ReportExitNodeFlowsRequest{Flows: []codersdk.ExitNodeFlowReport{report}})
	require.NoError(t, err)
	require.Equal(t, testToken, testutil.RequireReceive(ctx, t, fake.tokens))
	got := testutil.RequireReceive(ctx, t, fake.flows)
	require.Len(t, got.Flows, 1)
	require.Equal(t, report.FlowID, got.Flows[0].FlowID)

	dialer, err := client.TailnetDialer()
	require.NoError(t, err)
	_, err = dialer.Dial(ctx, nil)
	require.Error(t, err)
	hdr := testutil.RequireReceive(ctx, t, fake.wsHeaders)
	require.Equal(t, testToken, hdr.Get(codersdk.ExitNodeTokenHeader))
}

func TestClient_RegisterLoopDeliversAgentChanges(t *testing.T) {
	t.Parallel()
	ctx := testutil.Context(t, testutil.WaitShort)
	first := []uuid.UUID{uuid.New()}
	second := []uuid.UUID{uuid.New(), uuid.New()}
	_, u := newFakeCoderd(t, first, second)

	mClock := quartz.NewMock(t)
	tickerTrap := mClock.Trap().NewTicker("exitnodesdk", "register")
	defer tickerTrap.Close()

	client := exitnodesdk.New(u, testToken)
	callbacks := make(chan codersdk.RegisterExitNodeResponse, 4)
	loop, initial, err := client.RegisterLoop(ctx, exitnodesdk.RegisterLoopOpts{
		Logger:   testutil.Logger(t),
		Interval: 5 * time.Second,
		Clock:    mClock,
		CallbackFn: func(res codersdk.RegisterExitNodeResponse) error {
			callbacks <- res
			return nil
		},
	})
	require.NoError(t, err)
	defer loop.Close()
	require.Equal(t, first, initial.AgentIDs)

	call := tickerTrap.MustWait(ctx)
	require.Equal(t, 5*time.Second, call.Duration)
	call.MustRelease(ctx)

	mClock.Advance(5 * time.Second).MustWait(ctx)
	res := testutil.RequireReceive(ctx, t, callbacks)
	require.Equal(t, second, res.AgentIDs)
}

func TestClient_RegisterLoopInitialFailure(t *testing.T) {
	t.Parallel()
	ctx := testutil.Context(t, testutil.WaitShort)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_ = json.NewEncoder(w).Encode(codersdk.Response{Message: "bad token"})
	}))
	t.Cleanup(srv.Close)
	u, err := url.Parse(srv.URL)
	require.NoError(t, err)

	client := exitnodesdk.New(u, testToken)
	_, _, err = client.RegisterLoop(ctx, exitnodesdk.RegisterLoopOpts{Logger: testutil.Logger(t)})
	require.ErrorContains(t, err, "initial registration")
	// The secret must never surface in errors.
	require.NotContains(t, err.Error(), "supersecret")
}
