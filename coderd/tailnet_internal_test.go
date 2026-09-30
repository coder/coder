package coderd

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/http/httptrace"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/require"
	"github.com/tailscale/wireguard-go/device"
	"golang.org/x/xerrors"
	"tailscale.com/tailcfg"

	"cdr.dev/slog/v3"
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
		require.Equal(t, handshake, got["peer_last_handshake"])
		require.Equal(t, "2", got["peer_preferred_derp"])
		require.EqualValues(t, 148, got["peer_tx_bytes"])
		require.EqualValues(t, 0, got["peer_rx_bytes"])
	})
}

func TestRecordAgentUnreachable_DiagnosticsBusy(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitShort)
	counter := prometheus.NewCounterVec(prometheus.CounterOpts{Name: "agent_unreachable_total"}, []string{"reason"})
	registry := prometheus.NewRegistry()
	require.NoError(t, registry.Register(counter))

	// A read is already in flight, so this call must not start another one
	// and must return without waiting for the timeout.
	s := &ServerTailnet{agentUnreachable: counter}
	s.peerDiagnosticsBusy.Store(true)

	start := time.Now()
	err := s.recordAgentUnreachable(ctx, uuid.New(), time.Second)
	require.Less(t, time.Since(start), peerDiagnosticsTimeout)

	var unreachable *workspaceapps.AgentUnreachableError
	require.ErrorAs(t, err, &unreachable)
	got := fieldMap(unreachable.Fields)
	require.Equal(t, true, got["peer_diagnostics_skipped"])
	require.NotContains(t, got, "peer_node_received")

	metrics, err := registry.Gather()
	require.NoError(t, err)
	require.True(t, testutil.PromCounterHasValue(t, metrics, 1, "agent_unreachable_total", "diagnostics_timeout"))
}

func TestRecordAgentUnreachable_ClientCanceled(t *testing.T) {
	t.Parallel()

	counter := prometheus.NewCounterVec(prometheus.CounterOpts{Name: "agent_unreachable_total"}, []string{"reason"})
	registry := prometheus.NewRegistry()
	require.NoError(t, registry.Register(counter))
	// The gate is held so a diagnostics read, if attempted, reports
	// diagnostics_timeout instead of touching the nil conn.
	s := &ServerTailnet{agentUnreachable: counter}
	s.peerDiagnosticsBusy.Store(true)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	// A short wait ended by the client is not an unreachable agent.
	err := s.recordAgentUnreachable(ctx, uuid.New(), 100*time.Millisecond)
	var unreachable *workspaceapps.AgentUnreachableError
	require.ErrorAs(t, err, &unreachable)
	require.Equal(t, "client_canceled", fieldMap(unreachable.Fields)["reason"])

	// A long wait counts however the context ended.
	err = s.recordAgentUnreachable(ctx, uuid.New(), clientCanceledWait)
	require.ErrorAs(t, err, &unreachable)
	require.Equal(t, "diagnostics_timeout", fieldMap(unreachable.Fields)["reason"])

	metrics, err := registry.Gather()
	require.NoError(t, err)
	require.True(t, testutil.PromCounterHasValue(t, metrics, 1, "agent_unreachable_total", "client_canceled"))
	require.True(t, testutil.PromCounterHasValue(t, metrics, 1, "agent_unreachable_total", "diagnostics_timeout"))
}

func TestReportProxyDialFailure_PooledConn(t *testing.T) {
	t.Parallel()

	counter := prometheus.NewCounterVec(prometheus.CounterOpts{Name: "agent_unreachable_total"}, []string{"reason"})
	registry := prometheus.NewRegistry()
	require.NoError(t, registry.Register(counter))
	s := &ServerTailnet{agentUnreachable: counter}
	s.peerDiagnosticsBusy.Store(true)

	// The director installs the trace, and the transport calls GotConn once
	// the request is on a connection, pooled or not.
	req := httptest.NewRequest(http.MethodGet, "http://127.0.0.1", nil)
	s.director(uuid.New(), func(*http.Request) {})(req)
	ctx := req.Context()
	ds := dialStateFromContext(ctx)
	require.NotNil(t, ds)
	require.False(t, ds.hadConn())
	httptrace.ContextClientTrace(ctx).GotConn(httptrace.GotConnInfo{Reused: true})
	require.True(t, ds.hadConn())

	// The dial started for this request is still waiting on the agent, but
	// the error is from the pooled connection.
	ds.set(dialPhaseAwaitReachable)
	connErr := xerrors.New("connection reset by peer")
	err := s.reportProxyDialFailure(ctx, uuid.New(), connErr)
	require.Same(t, connErr, err)

	metrics, err := registry.Gather()
	require.NoError(t, err)
	require.Empty(t, metrics)
}
