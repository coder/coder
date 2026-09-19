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

type fakeCoderd struct {
	t                *testing.T
	agentSets        [][]uuid.UUID
	registers        atomic.Int32
	registerRequests chan codersdk.RegisterExitNodeRequest
	deregisters      chan codersdk.DeregisterExitNodeRequest
	flows            chan codersdk.ReportExitNodeFlowsRequest
	tokens           chan string
	wsHeaders        chan http.Header
	wsQueries        chan url.Values
}

func newFakeCoderd(t *testing.T, agentSets ...[]uuid.UUID) (*fakeCoderd, *url.URL) {
	f := &fakeCoderd{
		t:                t,
		agentSets:        agentSets,
		registerRequests: make(chan codersdk.RegisterExitNodeRequest, 16),
		deregisters:      make(chan codersdk.DeregisterExitNodeRequest, 4),
		flows:            make(chan codersdk.ReportExitNodeFlowsRequest, 8),
		tokens:           make(chan string, 16),
		wsHeaders:        make(chan http.Header, 1),
		wsQueries:        make(chan url.Values, 1),
	}
	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/v2/exitnodes/me/register", func(w http.ResponseWriter, r *http.Request) {
		f.tokens <- r.Header.Get(codersdk.ExitNodeTokenHeader)
		var req codersdk.RegisterExitNodeRequest
		require.NoError(t, json.NewDecoder(r.Body).Decode(&req))
		f.registerRequests <- req
		n := int(f.registers.Add(1)) - 1
		var agents []uuid.UUID
		if len(f.agentSets) > 0 {
			agents = f.agentSets[min(n, len(f.agentSets)-1)]
		}
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(codersdk.RegisterExitNodeResponse{
			DERPMap:  &tailcfg.DERPMap{Regions: map[int]*tailcfg.DERPRegion{}},
			AgentIDs: agents,
		})
	})
	mux.HandleFunc("POST /api/v2/exitnodes/me/deregister", func(w http.ResponseWriter, r *http.Request) {
		var req codersdk.DeregisterExitNodeRequest
		require.NoError(t, json.NewDecoder(r.Body).Decode(&req))
		f.deregisters <- req
		w.WriteHeader(http.StatusNoContent)
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
		f.wsQueries <- r.URL.Query()
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
	replicaID := uuid.New()

	resp, err := client.Register(ctx, codersdk.RegisterExitNodeRequest{ReplicaID: replicaID, Version: "test"})
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

	dialer, err := client.TailnetDialer(replicaID)
	require.NoError(t, err)
	_, err = dialer.Dial(ctx, nil)
	require.Error(t, err)
	hdr := testutil.RequireReceive(ctx, t, fake.wsHeaders)
	require.Equal(t, testToken, hdr.Get(codersdk.ExitNodeTokenHeader))
	query := testutil.RequireReceive(ctx, t, fake.wsQueries)
	require.Equal(t, replicaID.String(), query.Get(codersdk.ExitNodeCoordinateReplicaIDParam))
}

func TestClient_RegisterLoopDeliversAgentChanges(t *testing.T) {
	t.Parallel()
	ctx := testutil.Context(t, testutil.WaitShort)
	first := []uuid.UUID{uuid.New()}
	second := []uuid.UUID{uuid.New(), uuid.New()}
	fake, u := newFakeCoderd(t, first, second)
	mClock := quartz.NewMock(t)
	tickerTrap := mClock.Trap().NewTicker("exitnodesdk", "register")
	defer tickerTrap.Close()

	replicaID := uuid.New()
	client := exitnodesdk.New(u, testToken)
	callbacks := make(chan codersdk.RegisterExitNodeResponse, 4)
	loop, initial, err := client.RegisterLoop(ctx, exitnodesdk.RegisterLoopOpts{
		Logger:   testutil.Logger(t),
		Interval: 5 * time.Second,
		Clock:    mClock,
		Request:  codersdk.RegisterExitNodeRequest{ReplicaID: replicaID},
		CallbackFn: func(res codersdk.RegisterExitNodeResponse) error {
			callbacks <- res
			return nil
		},
	})
	require.NoError(t, err)
	require.Equal(t, first, initial.AgentIDs)

	call := tickerTrap.MustWait(ctx)
	call.MustRelease(ctx)
	mClock.Advance(5 * time.Second).MustWait(ctx)
	res := testutil.RequireReceive(ctx, t, callbacks)
	require.Equal(t, second, res.AgentIDs)
	loop.Close()
	require.Equal(t, replicaID, testutil.RequireReceive(ctx, t, fake.deregisters).ReplicaID)
}

func TestClient_RegisterLoopRequestAndMutation(t *testing.T) {
	t.Parallel()
	ctx := testutil.Context(t, testutil.WaitShort)
	fake, u := newFakeCoderd(t)
	mClock := quartz.NewMock(t)
	tickerTrap := mClock.Trap().NewTicker("exitnodesdk", "register")
	defer tickerTrap.Close()

	replicaID := uuid.New()
	policyHash := "first"
	client := exitnodesdk.New(u, testToken)
	loop, _, err := client.RegisterLoop(ctx, exitnodesdk.RegisterLoopOpts{
		Logger:  testutil.Logger(t),
		Clock:   mClock,
		Request: codersdk.RegisterExitNodeRequest{ReplicaID: replicaID, PolicyHash: "stale"},
		MutateFn: func(req *codersdk.RegisterExitNodeRequest) {
			req.PolicyHash = policyHash
		},
	})
	require.NoError(t, err)

	first := testutil.RequireReceive(ctx, t, fake.registerRequests)
	require.Equal(t, replicaID, first.ReplicaID)
	require.Equal(t, "first", first.PolicyHash)

	policyHash = "second"
	call := tickerTrap.MustWait(ctx)
	call.MustRelease(ctx)
	mClock.Advance(5 * time.Second).MustWait(ctx)
	second := testutil.RequireReceive(ctx, t, fake.registerRequests)
	require.Equal(t, replicaID, second.ReplicaID)
	require.Equal(t, "second", second.PolicyHash)
	loop.Close()
}

func TestClient_RegisterLoopPermanentFailureDeregisters(t *testing.T) {
	t.Parallel()
	ctx := testutil.Context(t, testutil.WaitShort)
	replicaID := uuid.New()
	var calls atomic.Int32
	deregisters := make(chan codersdk.DeregisterExitNodeRequest, 1)
	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/v2/exitnodes/me/register", func(w http.ResponseWriter, _ *http.Request) {
		if calls.Add(1) == 1 {
			_ = json.NewEncoder(w).Encode(codersdk.RegisterExitNodeResponse{})
			return
		}
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(codersdk.Response{Message: "replica was stopped"})
	})
	mux.HandleFunc("POST /api/v2/exitnodes/me/deregister", func(w http.ResponseWriter, r *http.Request) {
		var req codersdk.DeregisterExitNodeRequest
		require.NoError(t, json.NewDecoder(r.Body).Decode(&req))
		deregisters <- req
		w.WriteHeader(http.StatusNoContent)
	})
	u := newTestServerURL(t, mux)
	mClock := quartz.NewMock(t)
	tickerTrap := mClock.Trap().NewTicker("exitnodesdk", "register")
	defer tickerTrap.Close()
	failures := make(chan error, 1)

	client := exitnodesdk.New(u, testToken)
	loop, _, err := client.RegisterLoop(ctx, exitnodesdk.RegisterLoopOpts{
		Logger:          testutil.Logger(t),
		Clock:           mClock,
		MaxFailureCount: 10,
		Request:         codersdk.RegisterExitNodeRequest{ReplicaID: replicaID},
		FailureFn: func(err error) {
			failures <- err
		},
	})
	require.NoError(t, err)
	call := tickerTrap.MustWait(ctx)
	call.MustRelease(ctx)
	mClock.Advance(5 * time.Second).MustWait(ctx)
	require.ErrorContains(t, testutil.RequireReceive(ctx, t, failures), "permanent registration failure")
	require.Equal(t, replicaID, testutil.RequireReceive(ctx, t, deregisters).ReplicaID)
	loop.Close()
	require.EqualValues(t, 2, calls.Load())
}

func TestClient_RegisterLoopMaxFailuresDeregisters(t *testing.T) {
	t.Parallel()
	ctx := testutil.Context(t, testutil.WaitShort)
	replicaID := uuid.New()
	var calls atomic.Int32
	attempts := make(chan struct{}, 4)
	deregisters := make(chan codersdk.DeregisterExitNodeRequest, 1)
	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/v2/exitnodes/me/register", func(w http.ResponseWriter, _ *http.Request) {
		attempts <- struct{}{}
		if calls.Add(1) == 1 {
			_ = json.NewEncoder(w).Encode(codersdk.RegisterExitNodeResponse{})
			return
		}
		w.WriteHeader(http.StatusServiceUnavailable)
		_ = json.NewEncoder(w).Encode(codersdk.Response{Message: "unavailable"})
	})
	mux.HandleFunc("POST /api/v2/exitnodes/me/deregister", func(w http.ResponseWriter, r *http.Request) {
		var req codersdk.DeregisterExitNodeRequest
		require.NoError(t, json.NewDecoder(r.Body).Decode(&req))
		deregisters <- req
		w.WriteHeader(http.StatusNoContent)
	})
	u := newTestServerURL(t, mux)
	mClock := quartz.NewMock(t)
	tickerTrap := mClock.Trap().NewTicker("exitnodesdk", "register")
	defer tickerTrap.Close()
	failures := make(chan error, 1)

	client := exitnodesdk.New(u, testToken)
	loop, _, err := client.RegisterLoop(ctx, exitnodesdk.RegisterLoopOpts{
		Logger:          testutil.Logger(t),
		Clock:           mClock,
		MaxFailureCount: 2,
		Request:         codersdk.RegisterExitNodeRequest{ReplicaID: replicaID},
		FailureFn: func(err error) {
			failures <- err
		},
	})
	require.NoError(t, err)
	testutil.RequireReceive(ctx, t, attempts)
	call := tickerTrap.MustWait(ctx)
	call.MustRelease(ctx)
	for range 3 {
		mClock.Advance(5 * time.Second).MustWait(ctx)
		testutil.RequireReceive(ctx, t, attempts)
	}
	require.ErrorContains(t, testutil.RequireReceive(ctx, t, failures), "exceeded re-registration failure count of 2")
	require.Equal(t, replicaID, testutil.RequireReceive(ctx, t, deregisters).ReplicaID)
	loop.Close()
	require.EqualValues(t, 4, calls.Load())
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
	_, _, err = client.RegisterLoop(ctx, exitnodesdk.RegisterLoopOpts{
		Logger:  testutil.Logger(t),
		Request: codersdk.RegisterExitNodeRequest{ReplicaID: uuid.New()},
	})
	require.ErrorContains(t, err, "initial registration")
	require.NotContains(t, err.Error(), "supersecret")
}

func newTestServerURL(t *testing.T, handler http.Handler) *url.URL {
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	u, err := url.Parse(srv.URL)
	require.NoError(t, err)
	return u
}
