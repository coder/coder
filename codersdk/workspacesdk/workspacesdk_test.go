package workspacesdk_test

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/stretchr/testify/require"
	"tailscale.com/tailcfg"

	"github.com/coder/websocket"

	"github.com/coder/coder/v2/coderd/httpapi"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/codersdk/agentsdk"
	"github.com/coder/coder/v2/codersdk/workspacesdk"
	"github.com/coder/coder/v2/testutil"
)

func TestWorkspaceRewriteDERPMap(t *testing.T) {
	t.Parallel()
	// This test ensures that RewriteDERPMap mutates built-in DERPs with the
	// client access URL.
	dm := &tailcfg.DERPMap{
		Regions: map[int]*tailcfg.DERPRegion{
			1: {
				EmbeddedRelay: true,
				RegionID:      1,
				Nodes: []*tailcfg.DERPNode{{
					HostName: "bananas.org",
					DERPPort: 1,
				}},
			},
		},
	}
	parsed, err := url.Parse("https://coconuts.org:44558")
	require.NoError(t, err)
	client := agentsdk.New(parsed)
	client.RewriteDERPMap(dm)
	region := dm.Regions[1]
	require.True(t, region.EmbeddedRelay)
	require.Len(t, region.Nodes, 1)
	node := region.Nodes[0]
	require.Equal(t, "coconuts.org", node.HostName)
	require.Equal(t, 44558, node.DERPPort)
}

func TestWorkspaceDialerFailure(t *testing.T) {
	t.Parallel()

	// Setup.
	ctx := testutil.Context(t, testutil.WaitShort)
	logger := testutil.Logger(t)

	// Given: a mock HTTP server which mimicks an unreachable database when calling the coordination endpoint.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		httpapi.Write(ctx, w, http.StatusInternalServerError, codersdk.Response{
			Message: codersdk.DatabaseNotReachable,
			Detail:  "oops",
		})
	}))
	t.Cleanup(srv.Close)

	u, err := url.Parse(srv.URL)
	require.NoError(t, err)

	// When: calling the coordination endpoint.
	dialer := workspacesdk.NewWebsocketDialer(logger, u, &websocket.DialOptions{})
	_, err = dialer.Dial(ctx, nil)

	// Then: an error indicating a database issue is returned, to conditionalize the behavior of the caller.
	require.ErrorIs(t, err, codersdk.ErrDatabaseNotReachable)
}
