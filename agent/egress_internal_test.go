package agent

import (
	"bufio"
	"context"
	"io"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"slices"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/agent/agentegress"
	"github.com/coder/coder/v2/codersdk/agentsdk"
	"github.com/coder/coder/v2/tailnet"
	"github.com/coder/coder/v2/testutil"
)

func TestEgressChangeRequiresRestart(t *testing.T) {
	t.Parallel()

	base := agentsdk.EgressConfig{
		ExitNodes:         []agentsdk.EgressExitNode{{ID: uuid.New(), ReplicaIDs: []uuid.UUID{uuid.New()}}},
		ExitNodePort:      3128,
		Enforce:           true,
		ControlPlaneHosts: []string{"tcp/control.example:443"},
	}
	for _, tt := range []struct {
		name string
		next agentsdk.EgressConfig
		want egressChange
	}{
		{name: "unchanged", next: base},
		{
			name: "selector",
			next: agentsdk.EgressConfig{
				ExitNodes:         []agentsdk.EgressExitNode{{ID: uuid.New(), ReplicaIDs: []uuid.UUID{uuid.New()}}},
				ExitNodePort:      4128,
				Enforce:           true,
				ControlPlaneHosts: base.ControlPlaneHosts,
			},
			want: egressChange{applyProxyState: true},
		},
		{
			name: "enforcement",
			next: agentsdk.EgressConfig{
				ExitNodes:         base.ExitNodes,
				ExitNodePort:      base.ExitNodePort,
				Enforce:           false,
				ControlPlaneHosts: base.ControlPlaneHosts,
			},
			want: egressChange{applyProxyState: true, deferEnforcement: true},
		},
		{
			name: "control plane hosts",
			next: agentsdk.EgressConfig{
				ExitNodes:         base.ExitNodes,
				ExitNodePort:      base.ExitNodePort,
				Enforce:           true,
				ControlPlaneHosts: []string{"tcp/other.example:443"},
			},
			want: egressChange{applyProxyState: true, deferNetfilterExemptions: true},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, tt.want, egressChangeRequiresRestart(base, tt.next))
		})
	}
}

func TestUpdateEgressLockedEnforcementAppliesReplicaChurn(t *testing.T) {
	t.Parallel()

	first, second, third := uuid.New(), uuid.New(), uuid.New()
	seen := make(chan netip.AddrPort, 1)
	dialer := agentegress.DialerFunc(func(_ context.Context, addr netip.AddrPort) (net.Conn, error) {
		seen <- addr
		client, server := net.Pipe()
		go func() {
			defer server.Close()
			req, err := http.ReadRequest(bufio.NewReader(server))
			if err == nil {
				_, _ = io.WriteString(server, "HTTP/1.1 200 Connection Established\r\n\r\n")
				_ = req.Body.Close()
			}
		}()
		return client, nil
	})
	base := agentsdk.EgressConfig{
		ExitNodes:         []agentsdk.EgressExitNode{{ID: uuid.New(), ReplicaIDs: []uuid.UUID{first}}},
		ExitNodePort:      3128,
		Enforce:           true,
		ControlPlaneHosts: []string{"udp/198.51.100.1:41641"},
	}
	proxy, err := agentegress.New(testutil.Logger(t), agentegress.Options{
		Dialer:     dialer,
		Config:     base,
		ListenAddr: "127.0.0.1:0",
	})
	require.NoError(t, err)
	require.NoError(t, proxy.Start(t.Context()))
	defer func() { require.NoError(t, proxy.Close()) }()

	sink := testutil.NewFakeSink(t)
	a := &agent{
		logger:                    sink.Logger(),
		egressProxy:               proxy,
		egressEnforcer:            &agentegress.Enforcer{},
		egressEnforcerExemptHosts: slices.Clone(base.ControlPlaneHosts),
	}
	next := base
	next.ExitNodes = []agentsdk.EgressExitNode{{ID: uuid.New(), ReplicaIDs: []uuid.UUID{second}}}
	next.ControlPlaneHosts = []string{"udp/198.51.100.2:41641"}
	a.updateEgress(t.Context(), &next)

	connectExplicit(t, proxy.Addr())
	want := netip.AddrPortFrom(tailnet.TailscaleServicePrefix.AddrFromUUID(second), 3128)
	require.Equal(t, want, testutil.RequireReceive(t.Context(), t, seen))

	again := next
	again.ExitNodes = []agentsdk.EgressExitNode{{ID: uuid.New(), ReplicaIDs: []uuid.UUID{third}}}
	a.updateEgress(t.Context(), &again)
	connectExplicit(t, proxy.Addr())
	want = netip.AddrPortFrom(tailnet.TailscaleServicePrefix.AddrFromUUID(third), 3128)
	require.Equal(t, want, testutil.RequireReceive(t.Context(), t, seen))

	entries := sink.Entries(func(e slog.SinkEntry) bool {
		return e.Message == "egress enforcement exemptions deferred until restart; new WireGuard endpoints are not exempt and connectivity to those replicas will use DERP relay"
	})
	require.Len(t, entries, 1)
	require.Equal(t, slog.LevelInfo, entries[0].Level)
}

func connectExplicit(t *testing.T, addr netip.AddrPort) {
	t.Helper()
	conn, err := net.Dial("tcp", addr.String())
	require.NoError(t, err)
	defer conn.Close()
	req := &http.Request{
		Method: http.MethodConnect,
		URL:    &url.URL{Opaque: "example.com:443"},
		Host:   "example.com:443",
	}
	require.NoError(t, req.Write(conn))
	resp, err := http.ReadResponse(bufio.NewReader(conn), req)
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)
}
