package coderd

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/tailscale/wireguard-go/device"
	"tailscale.com/tailcfg"

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
