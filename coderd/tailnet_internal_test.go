package coderd

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"tailscale.com/derp"
	"tailscale.com/tailcfg"
	"tailscale.com/types/key"

	"github.com/coder/coder/v2/testutil"
	"github.com/coder/quartz"
)

func TestEmbeddedDERPRegionDialer(t *testing.T) {
	t.Parallel()

	srv := derp.NewServer(key.NewNode(), func(format string, args ...any) { t.Logf(format, args...) })
	t.Cleanup(func() { _ = srv.Close() })
	ctx, cancel := context.WithCancel(testutil.Context(t, testutil.WaitShort))
	dial := embeddedDERPRegionDialer(ctx, testutil.Logger(t), srv)

	require.Nil(t, dial(ctx, &tailcfg.DERPRegion{EmbeddedRelay: false}))

	cancel()
	conn := dial(ctx, &tailcfg.DERPRegion{EmbeddedRelay: true})
	require.NotNil(t, conn, "an embedded region must never fall back to a network dial")
	_, err := conn.Read(make([]byte, 1))
	require.Error(t, err)
}

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
