package coderd

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/http/httptrace"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/require"
	"github.com/tailscale/wireguard-go/device"
	"golang.org/x/xerrors"
	"tailscale.com/tailcfg"

	"cdr.dev/slog/v3"
	"cdr.dev/slog/v3/sloggers/slogjson"
	"github.com/coder/coder/v2/coderd/workspaceapps"
	"github.com/coder/coder/v2/tailnet"
	"github.com/coder/coder/v2/testutil"
	"github.com/coder/quartz"
)

func TestPollingDERPClient_FirstRecvDoesNotWaitForTick(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitShort)
	derpMap := &tailcfg.DERPMap{Regions: map[int]*tailcfg.DERPRegion{1: {RegionID: 1}}}
	// The mock clock never advances, so a poll loop that waits for the
	// first tick would block forever.
	client := newPollingDERPClient(func() *tailcfg.DERPMap { return derpMap }, testutil.Logger(t), quartz.NewMock(t))
	defer client.Close()

	got := make(chan *tailcfg.DERPMap, 1)
	go func() {
		dm, err := client.Recv()
		if err != nil {
			t.Error("recv derp map:", err)
			return
		}
		got <- dm
	}()
	require.Equal(t, derpMap, testutil.RequireReceive(ctx, t, got))
}

func TestUnreachableReason(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 1, 7, 12, 0, 0, 0, time.UTC)
	node := &tailcfg.Node{}
	for _, tc := range []struct {
		name string
		diag tailnet.PeerDiagnostics
		want string
	}{
		{name: "NoNode", diag: tailnet.PeerDiagnostics{}, want: "no_node"},
		{name: "NoHandshake", diag: tailnet.PeerDiagnostics{ReceivedNode: node}, want: "no_handshake"},
		{
			name: "Lost",
			diag: tailnet.PeerDiagnostics{ReceivedNode: node, Lost: true, LastWireguardHandshake: now.Add(-time.Minute)},
			want: "peer_lost",
		},
		{
			name: "Stale",
			diag: tailnet.PeerDiagnostics{ReceivedNode: node, LastWireguardHandshake: now.Add(-device.RejectAfterTime - time.Second)},
			want: "handshake_stale",
		},
		{
			name: "OK",
			diag: tailnet.PeerDiagnostics{ReceivedNode: node, LastWireguardHandshake: now.Add(-time.Minute)},
			want: "handshake_ok",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, tc.want, unreachableReason(tc.diag, now))
		})
	}
}

func fieldMap(fields []slog.Field) map[string]any {
	m := make(map[string]any, len(fields))
	for _, f := range fields {
		m[f.Name] = f.Value
	}
	return m
}

func TestUnreachableFields(t *testing.T) {
	t.Parallel()

	t.Run("NoNode", func(t *testing.T) {
		t.Parallel()
		got := fieldMap(unreachableFields(tailnet.PeerDiagnostics{PreferredDERP: 1}))
		require.Equal(t, false, got["peer_node_received"])
		require.Equal(t, 1, got["server_preferred_derp"])
		require.NotContains(t, got, "peer_last_handshake")
		require.NotContains(t, got, "peer_preferred_derp")
		require.NotContains(t, got, "peer_tx_bytes")
		require.NotContains(t, got, "peer_rx_bytes")
	})

	t.Run("NodeReceived", func(t *testing.T) {
		t.Parallel()
		handshake := time.Date(2026, 1, 7, 12, 0, 0, 0, time.UTC)
		got := fieldMap(unreachableFields(tailnet.PeerDiagnostics{
			PreferredDERP:          1,
			ReceivedNode:           &tailcfg.Node{DERP: "127.3.3.40:2"},
			LastWireguardHandshake: handshake,
			TxBytes:                148,
			RxBytes:                0,
		}))
		require.Equal(t, true, got["peer_node_received"])
		require.Equal(t, false, got["peer_lost"])
		require.Equal(t, handshake, got["peer_last_handshake"])
		require.Equal(t, "2", got["peer_preferred_derp"])
		require.EqualValues(t, 148, got["peer_tx_bytes"])
		require.EqualValues(t, 0, got["peer_rx_bytes"])
	})
}

// newTestServerTailnet returns a ServerTailnet with only the fields that
// recordAgentUnreachable uses. read stands in for the tailnet connection.
func newTestServerTailnet(t *testing.T, read func(uuid.UUID) tailnet.PeerDiagnostics) (*ServerTailnet, *prometheus.Registry) {
	t.Helper()
	counter := prometheus.NewCounterVec(prometheus.CounterOpts{Name: "agent_unreachable_total"}, []string{"reason"})
	registry := prometheus.NewRegistry()
	require.NoError(t, registry.Register(counter))
	return &ServerTailnet{
		clock:               quartz.NewReal(),
		agentUnreachable:    counter,
		readPeerDiagnostics: read,
		peerDiagnosticsSlot: make(chan struct{}, 1),
	}, registry
}

func noPeerDiagnostics(uuid.UUID) tailnet.PeerDiagnostics {
	return tailnet.PeerDiagnostics{}
}

func TestRecordAgentUnreachable_DiagnosticsWait(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitShort)
	readStarted := make(chan struct{}, 2)
	release := make(chan struct{})
	s, registry := newTestServerTailnet(t, func(uuid.UUID) tailnet.PeerDiagnostics {
		readStarted <- struct{}{}
		<-release
		return tailnet.PeerDiagnostics{ReceivedNode: &tailcfg.Node{}}
	})
	// The mock clock never fires the timeout, so the second failure can only
	// finish by getting the slot.
	clock := quartz.NewMock(t)
	s.clock = clock
	trap := clock.Trap().NewTimer("peerDiagnostics")
	defer trap.Close()

	errCh := make(chan error, 2)
	go func() {
		errCh <- s.recordAgentUnreachable(ctx, uuid.New(), time.Second)
	}()
	trap.MustWait(ctx).MustRelease(ctx)
	testutil.RequireReceive(ctx, t, readStarted)

	go func() {
		errCh <- s.recordAgentUnreachable(ctx, uuid.New(), time.Second)
	}()
	trap.MustWait(ctx).MustRelease(ctx)
	close(release)

	for range 2 {
		err := testutil.RequireReceive(ctx, t, errCh)
		var unreachable *workspaceapps.AgentUnreachableError
		require.ErrorAs(t, err, &unreachable)
		require.Equal(t, "no_handshake", fieldMap(unreachable.Fields)["reason"])
	}

	metrics, err := registry.Gather()
	require.NoError(t, err)
	require.True(t, testutil.PromCounterHasValue(t, metrics, 2, "agent_unreachable_total", "no_handshake"))
}

func TestRecordAgentUnreachable_NoRequestLogger(t *testing.T) {
	t.Parallel()

	s, _ := newTestServerTailnet(t, noPeerDiagnostics)
	var logs bytes.Buffer
	s.logger = slog.Make(slogjson.Sink(&logs))

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_ = s.recordAgentUnreachable(ctx, uuid.New(), 100*time.Millisecond)
	require.Empty(t, logs.String())

	agentID := uuid.New()
	_ = s.recordAgentUnreachable(testutil.Context(t, testutil.WaitShort), agentID, time.Second)
	var entry struct {
		Level  string         `json:"level"`
		Msg    string         `json:"msg"`
		Fields map[string]any `json:"fields"`
	}
	require.NoError(t, json.Unmarshal(logs.Bytes(), &entry))
	require.Equal(t, "WARN", entry.Level)
	require.Equal(t, "agent is unreachable", entry.Msg)
	require.Equal(t, agentID.String(), entry.Fields["agent_id"])
	require.Equal(t, "no_node", entry.Fields["reason"])
}

func TestRecordAgentUnreachable_ClientCanceled(t *testing.T) {
	t.Parallel()

	s, registry := newTestServerTailnet(t, noPeerDiagnostics)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	// A short wait ended by the client is counted but is not an unreachable
	// agent, so the PTY handler logs it at debug like any other dial error.
	err := s.recordAgentUnreachable(ctx, uuid.New(), 100*time.Millisecond)
	require.Error(t, err)
	var unreachable *workspaceapps.AgentUnreachableError
	require.NotErrorAs(t, err, &unreachable)

	// A long wait counts however the context ended.
	err = s.recordAgentUnreachable(ctx, uuid.New(), clientCanceledWait)
	require.ErrorAs(t, err, &unreachable)
	require.Equal(t, "no_node", fieldMap(unreachable.Fields)["reason"])

	metrics, err := registry.Gather()
	require.NoError(t, err)
	require.True(t, testutil.PromCounterHasValue(t, metrics, 1, "agent_unreachable_total", "client_canceled"))
	require.True(t, testutil.PromCounterHasValue(t, metrics, 1, "agent_unreachable_total", "no_node"))
}

func TestReportProxyDialFailure_PooledConn(t *testing.T) {
	t.Parallel()

	s, registry := newTestServerTailnet(t, noPeerDiagnostics)

	req := httptest.NewRequest(http.MethodGet, "http://127.0.0.1", nil)
	s.director(uuid.New(), func(*http.Request) {})(req)
	ctx, cancel := context.WithCancel(req.Context())
	ds := dialStateFromContext(ctx)
	require.NotNil(t, ds)

	// The dial started for this request is still waiting on the agent, but
	// the transport gave the request a pooled connection and the error is
	// from that connection.
	ds.startDial()
	httptrace.ContextClientTrace(ctx).GotConn(httptrace.GotConnInfo{Reused: true})
	require.True(t, ds.hadConn())
	cancel()
	err := s.reportProxyDialFailure(ctx, uuid.New(), context.Canceled)
	require.Same(t, context.Canceled, err)

	metrics, err := registry.Gather()
	require.NoError(t, err)
	require.Empty(t, metrics)
}

func TestReportProxyDialFailure_RetryDial(t *testing.T) {
	t.Parallel()

	s, registry := newTestServerTailnet(t, noPeerDiagnostics)

	req := httptest.NewRequest(http.MethodGet, "http://127.0.0.1", nil)
	s.director(uuid.New(), func(*http.Request) {})(req)
	ctx, cancel := context.WithCancel(req.Context())
	ds := dialStateFromContext(ctx)
	require.NotNil(t, ds)

	// The pooled connection failed and the transport retried on a new dial,
	// which is still waiting on the agent when the request ends.
	httptrace.ContextClientTrace(ctx).GotConn(httptrace.GotConnInfo{Reused: true})
	ds.startDial()
	require.False(t, ds.hadConn())
	cancel()
	err := s.reportProxyDialFailure(ctx, uuid.New(), context.Canceled)
	require.ErrorIs(t, err, context.Canceled)

	metrics, err := registry.Gather()
	require.NoError(t, err)
	require.True(t, testutil.PromCounterHasValue(t, metrics, 1, "agent_unreachable_total", "client_canceled"))
}

func TestReportProxyDialFailure_DialTimedOut(t *testing.T) {
	t.Parallel()

	s, registry := newTestServerTailnet(t, noPeerDiagnostics)

	// The dial hit proxyDialTimeout before the request ended, so the error
	// already names the unreachable agent and must not be prefixed again.
	ds := &dialState{}
	ds.set(dialPhaseAwaitReachable)
	ctx := context.WithValue(testutil.Context(t, testutil.WaitShort), dialStateKey{}, ds)
	dialErr := xerrors.Errorf("acquire agent conn: %w", errAgentUnreachable)
	err := s.reportProxyDialFailure(ctx, uuid.New(), dialErr)
	require.Same(t, dialErr, err)

	metrics, err := registry.Gather()
	require.NoError(t, err)
	require.True(t, testutil.PromCounterHasValue(t, metrics, 1, "agent_unreachable_total", "no_node"))
}

func TestReportProxyDialFailure_CoordinatorError(t *testing.T) {
	t.Parallel()

	s, registry := newTestServerTailnet(t, noPeerDiagnostics)

	// The request is still waiting and the dial failed before the first
	// ping, so the agent was never asked.
	ds := &dialState{}
	ds.set(dialPhaseAwaitReachable)
	ctx := context.WithValue(testutil.Context(t, testutil.WaitShort), dialStateKey{}, ds)
	dialErr := xerrors.New("acquire agent conn: ensure agent: send failed")
	err := s.reportProxyDialFailure(ctx, uuid.New(), dialErr)
	require.Same(t, dialErr, err)

	metrics, err := registry.Gather()
	require.NoError(t, err)
	require.Empty(t, metrics)
}

func TestRecordAgentUnreachable_DiagnosticsSlot(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitShort)
	release := make(chan struct{})
	var reads atomic.Int32
	s, registry := newTestServerTailnet(t, func(uuid.UUID) tailnet.PeerDiagnostics {
		reads.Add(1)
		<-release
		return tailnet.PeerDiagnostics{ReceivedNode: &tailcfg.Node{}}
	})
	var unreachable *workspaceapps.AgentUnreachableError

	// The first read waits out the timeout while the read stays in flight.
	err := s.recordAgentUnreachable(ctx, uuid.New(), time.Second)
	require.ErrorAs(t, err, &unreachable)
	require.Equal(t, "diagnostics_timeout", fieldMap(unreachable.Fields)["reason"])

	// A second failure during that read waits for the slot, times out, and
	// does not start another read.
	err = s.recordAgentUnreachable(ctx, uuid.New(), time.Second)
	require.ErrorAs(t, err, &unreachable)
	require.Equal(t, "diagnostics_timeout", fieldMap(unreachable.Fields)["reason"])
	require.EqualValues(t, 1, reads.Load())

	// Once the read returns, the slot frees and later failures read again.
	close(release)
	err = s.recordAgentUnreachable(ctx, uuid.New(), time.Second)
	require.ErrorAs(t, err, &unreachable)
	got := fieldMap(unreachable.Fields)
	require.Equal(t, "no_handshake", got["reason"])
	require.Equal(t, true, got["peer_node_received"])

	metrics, err := registry.Gather()
	require.NoError(t, err)
	require.True(t, testutil.PromCounterHasValue(t, metrics, 2, "agent_unreachable_total", "diagnostics_timeout"))
	require.True(t, testutil.PromCounterHasValue(t, metrics, 1, "agent_unreachable_total", "no_handshake"))
}
