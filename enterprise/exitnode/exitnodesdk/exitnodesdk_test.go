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
	agents      [][]uuid.UUID
	calls       atomic.Int32
	registers   chan codersdk.RegisterExitNodeRequest
	deregisters chan codersdk.DeregisterExitNodeRequest
	flows       chan codersdk.ReportExitNodeFlowsRequest
	tokens      chan string
	wsHeaders   chan http.Header
	wsQueries   chan url.Values
}

func newFakeCoderd(t *testing.T, agents ...[]uuid.UUID) (*fakeCoderd, *url.URL) {
	t.Helper()
	f := &fakeCoderd{agents: agents, registers: make(chan codersdk.RegisterExitNodeRequest, 16),
		deregisters: make(chan codersdk.DeregisterExitNodeRequest, 4), flows: make(chan codersdk.ReportExitNodeFlowsRequest, 8),
		tokens: make(chan string, 16), wsHeaders: make(chan http.Header, 1), wsQueries: make(chan url.Values, 1)}
	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/v2/exitnodes/me/register", func(w http.ResponseWriter, r *http.Request) {
		f.tokens <- r.Header.Get(codersdk.ExitNodeTokenHeader)
		var req codersdk.RegisterExitNodeRequest
		require.NoError(t, json.NewDecoder(r.Body).Decode(&req))
		f.registers <- req
		n := int(f.calls.Add(1)) - 1
		var ids []uuid.UUID
		if len(agents) > 0 {
			ids = agents[min(n, len(agents)-1)]
		}
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(codersdk.RegisterExitNodeResponse{DERPMap: &tailcfg.DERPMap{Regions: map[int]*tailcfg.DERPRegion{}}, AgentIDs: ids})
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
	return f, serverURL(t, mux)
}
func TestClient(t *testing.T) {
	t.Parallel()
	ctx := testutil.Context(t, testutil.WaitShort)
	agent, replica := uuid.New(), uuid.New()
	fake, u := newFakeCoderd(t, []uuid.UUID{agent})
	client := exitnodesdk.New(u, testToken)
	resp, err := client.Register(ctx, codersdk.RegisterExitNodeRequest{ReplicaID: replica})
	require.NoError(t, err)
	require.Equal(t, []uuid.UUID{agent}, resp.AgentIDs)
	require.NotNil(t, resp.DERPMap)
	require.Equal(t, testToken, testutil.RequireReceive(ctx, t, fake.tokens))
	report := codersdk.ExitNodeFlowReport{FlowID: uuid.New(), AgentID: agent, Decision: codersdk.ExitNodeFlowDeny}
	require.NoError(t, client.ReportFlows(ctx, codersdk.ReportExitNodeFlowsRequest{Flows: []codersdk.ExitNodeFlowReport{report}}))
	require.Equal(t, testToken, testutil.RequireReceive(ctx, t, fake.tokens))
	require.Equal(t, report.FlowID, testutil.RequireReceive(ctx, t, fake.flows).Flows[0].FlowID)
	dialer, err := client.TailnetDialer(replica)
	require.NoError(t, err)
	_, err = dialer.Dial(ctx, nil)
	require.Error(t, err)
	require.Equal(t, testToken, testutil.RequireReceive(ctx, t, fake.wsHeaders).Get(codersdk.ExitNodeTokenHeader))
	require.Equal(t, replica.String(), testutil.RequireReceive(ctx, t, fake.wsQueries).Get(codersdk.ExitNodeCoordinateReplicaIDParam))
}
func TestRegisterLoopHeartbeatAndMutation(t *testing.T) {
	t.Parallel()
	ctx := testutil.Context(t, testutil.WaitShort)
	first, second := []uuid.UUID{uuid.New()}, []uuid.UUID{uuid.New(), uuid.New()}
	fake, u := newFakeCoderd(t, first, second)
	clock := quartz.NewMock(t)
	trap := clock.Trap().NewTicker("exitnodesdk", "register")
	defer trap.Close()
	replica, hash := uuid.New(), "first"
	callbacks := make(chan codersdk.RegisterExitNodeResponse, 1)
	loop, initial, err := exitnodesdk.New(u, testToken).RegisterLoop(ctx, exitnodesdk.RegisterLoopOpts{
		Logger: testutil.Logger(t), Clock: clock, Request: codersdk.RegisterExitNodeRequest{ReplicaID: replica},
		MutateFn:   func(req *codersdk.RegisterExitNodeRequest) { req.PolicyHash = hash },
		CallbackFn: func(res codersdk.RegisterExitNodeResponse) error { callbacks <- res; return nil },
	})
	require.NoError(t, err)
	require.Equal(t, first, initial.AgentIDs)
	require.Equal(t, "first", testutil.RequireReceive(ctx, t, fake.registers).PolicyHash)
	hash = "second"
	trap.MustWait(ctx).MustRelease(ctx)
	clock.Advance(5 * time.Second).MustWait(ctx)
	require.Equal(t, second, testutil.RequireReceive(ctx, t, callbacks).AgentIDs)
	require.Equal(t, "second", testutil.RequireReceive(ctx, t, fake.registers).PolicyHash)
	loop.Close()
	require.Equal(t, replica, testutil.RequireReceive(ctx, t, fake.deregisters).ReplicaID)
}
func TestRegisterLoopFailuresDeregister(t *testing.T) {
	t.Parallel()
	for _, tt := range []struct {
		name, want         string
		status, max, ticks int
	}{
		{"permanent", "permanent registration failure", http.StatusBadRequest, 10, 1},
		{"deleted", "permanent registration failure", http.StatusUnauthorized, 10, 1},
		{"max failures", "exceeded re-registration failure count of 2", http.StatusServiceUnavailable, 2, 3},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			ctx := testutil.Context(t, testutil.WaitShort)
			replica := uuid.New()
			var calls atomic.Int32
			attempts := make(chan struct{}, tt.ticks+1)
			deregisters := make(chan codersdk.DeregisterExitNodeRequest, 1)
			mux := http.NewServeMux()
			mux.HandleFunc("POST /api/v2/exitnodes/me/register", func(w http.ResponseWriter, _ *http.Request) {
				attempts <- struct{}{}
				if calls.Add(1) == 1 {
					_ = json.NewEncoder(w).Encode(codersdk.RegisterExitNodeResponse{})
					return
				}
				w.WriteHeader(tt.status)
				_ = json.NewEncoder(w).Encode(codersdk.Response{Message: "failed"})
			})
			mux.HandleFunc("POST /api/v2/exitnodes/me/deregister", func(w http.ResponseWriter, r *http.Request) {
				var req codersdk.DeregisterExitNodeRequest
				require.NoError(t, json.NewDecoder(r.Body).Decode(&req))
				deregisters <- req
				w.WriteHeader(http.StatusNoContent)
			})
			clock := quartz.NewMock(t)
			trap := clock.Trap().NewTicker("exitnodesdk", "register")
			defer trap.Close()
			failures := make(chan error, 1)
			loop, _, err := exitnodesdk.New(serverURL(t, mux), testToken).RegisterLoop(ctx, exitnodesdk.RegisterLoopOpts{
				Logger: testutil.Logger(t), Clock: clock, MaxFailureCount: tt.max,
				Request: codersdk.RegisterExitNodeRequest{ReplicaID: replica}, FailureFn: func(err error) { failures <- err },
			})
			require.NoError(t, err)
			testutil.RequireReceive(ctx, t, attempts)
			trap.MustWait(ctx).MustRelease(ctx)
			for range tt.ticks {
				clock.Advance(5 * time.Second).MustWait(ctx)
				testutil.RequireReceive(ctx, t, attempts)
			}
			require.ErrorContains(t, testutil.RequireReceive(ctx, t, failures), tt.want)
			require.Equal(t, replica, testutil.RequireReceive(ctx, t, deregisters).ReplicaID)
			loop.Close()
			require.EqualValues(t, tt.ticks+1, calls.Load())
		})
	}
}
func TestRegisterLoopInitialFailure(t *testing.T) {
	t.Parallel()
	u := serverURL(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_ = json.NewEncoder(w).Encode(codersdk.Response{Message: "bad token"})
	}))
	_, _, err := exitnodesdk.New(u, testToken).RegisterLoop(t.Context(), exitnodesdk.RegisterLoopOpts{
		Logger: testutil.Logger(t), Request: codersdk.RegisterExitNodeRequest{ReplicaID: uuid.New()}})
	require.ErrorContains(t, err, "initial registration")
	require.NotContains(t, err.Error(), "supersecret")
}
func serverURL(t *testing.T, handler http.Handler) *url.URL {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	u, err := url.Parse(srv.URL)
	require.NoError(t, err)
	return u
}
