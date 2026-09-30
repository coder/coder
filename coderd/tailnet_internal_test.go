package coderd

import (
	"net"
	"net/http"
	"strconv"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"golang.org/x/xerrors"
	"tailscale.com/tailcfg"

	"github.com/coder/coder/v2/codersdk/workspacesdk"
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

func TestAgentRoundTripper_RejectsOtherHosts(t *testing.T) {
	t.Parallel()

	agentID := uuid.New()
	apiPort := strconv.Itoa(workspacesdk.AgentHTTPAPIServerPort)
	for name, host := range map[string]string{
		"OtherAgent": net.JoinHostPort(tailnet.TailscaleServicePrefix.AddrFromUUID(uuid.New()).String(), apiPort),
		"OtherPort":  net.JoinHostPort(tailnet.TailscaleServicePrefix.AddrFromUUID(agentID).String(), "80"),
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			ctx := testutil.Context(t, testutil.WaitShort)
			forwarded := false
			rt := agentRoundTripper{
				agentID: agentID,
				transport: testutil.RoundTripperFunc(func(*http.Request) (*http.Response, error) {
					forwarded = true
					return nil, xerrors.New("forwarded")
				}),
			}
			req, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://"+host+"/api/v0/listening-ports", nil)
			require.NoError(t, err)

			_, err = rt.RoundTrip(req) //nolint:bodyclose // Rejected requests return no response.
			require.Error(t, err)
			require.False(t, forwarded, "request must not reach the pooled transport")
		})
	}
}
