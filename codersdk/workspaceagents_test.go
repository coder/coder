package codersdk_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"testing"

	"github.com/stretchr/testify/require"
	"tailscale.com/tailcfg"

	"github.com/coder/coder/coderd/httpapi"
	"github.com/coder/coder/codersdk"
)

func TestWorkspaceAgentMetadata(t *testing.T) {
	t.Parallel()
	// This test ensures that the DERP map returned properly
	// mutates built-in DERPs with the client access URL.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		httpapi.Write(context.Background(), w, http.StatusOK, codersdk.WorkspaceAgentMetadata{
			DERPMap: &tailcfg.DERPMap{
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
			},
		})
	}))
	parsed, err := url.Parse(srv.URL)
	require.NoError(t, err)
	client := codersdk.New(parsed)
	metadata, err := client.WorkspaceAgentMetadata(context.Background())
	require.NoError(t, err)
	region := metadata.DERPMap.Regions[1]
	require.True(t, region.EmbeddedRelay)
	require.Len(t, region.Nodes, 1)
	node := region.Nodes[0]
	require.Equal(t, parsed.Hostname(), node.HostName)
	require.Equal(t, parsed.Port(), strconv.Itoa(node.DERPPort))
}
