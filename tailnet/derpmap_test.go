package tailnet_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"tailscale.com/ipn/ipnstate"
	"tailscale.com/tailcfg"

	"github.com/coder/coder/v2/tailnet"
)

func TestNewDERPMap(t *testing.T) {
	t.Parallel()
	t.Run("WithoutRemoteURL", func(t *testing.T) {
		t.Parallel()
		derpMap, err := tailnet.NewDERPMap(context.Background(), &tailcfg.DERPRegion{
			RegionID: 1,
			Nodes:    []*tailcfg.DERPNode{{}},
		}, []string{"stun.google.com:2345"}, "", "", false)
		require.NoError(t, err)
		require.Len(t, derpMap.Regions, 2)
		require.Len(t, derpMap.Regions[1].Nodes, 1)
		require.Len(t, derpMap.Regions[2].Nodes, 1)
	})
	t.Run("RemoteURL", func(t *testing.T) {
		t.Parallel()
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			data, _ := json.Marshal(&tailcfg.DERPMap{
				Regions: map[int]*tailcfg.DERPRegion{
					1: {
						RegionID: 1,
						Nodes:    []*tailcfg.DERPNode{{}},
					},
				},
			})
			_, _ = w.Write(data)
		}))
		t.Cleanup(server.Close)
		derpMap, err := tailnet.NewDERPMap(context.Background(), &tailcfg.DERPRegion{
			RegionID: 2,
		}, []string{}, server.URL, "", false)
		require.NoError(t, err)
		require.Len(t, derpMap.Regions, 2)
	})
	t.Run("RemoteConflicts", func(t *testing.T) {
		t.Parallel()
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			data, _ := json.Marshal(&tailcfg.DERPMap{
				Regions: map[int]*tailcfg.DERPRegion{
					1: {},
				},
			})
			_, _ = w.Write(data)
		}))
		t.Cleanup(server.Close)
		_, err := tailnet.NewDERPMap(context.Background(), &tailcfg.DERPRegion{
			RegionID: 1,
		}, []string{}, server.URL, "", false)
		require.Error(t, err)
	})
	t.Run("LocalPath", func(t *testing.T) {
		t.Parallel()
		localPath := filepath.Join(t.TempDir(), "derp.json")
		content, err := json.Marshal(&tailcfg.DERPMap{
			Regions: map[int]*tailcfg.DERPRegion{
				1: {
					Nodes: []*tailcfg.DERPNode{{}},
				},
			},
		})
		require.NoError(t, err)
		err = os.WriteFile(localPath, content, 0o600)
		require.NoError(t, err)
		derpMap, err := tailnet.NewDERPMap(context.Background(), &tailcfg.DERPRegion{
			RegionID: 2,
		}, []string{}, "", localPath, false)
		require.NoError(t, err)
		require.Len(t, derpMap.Regions, 2)
	})
	t.Run("DisableSTUN", func(t *testing.T) {
		t.Parallel()
		localPath := filepath.Join(t.TempDir(), "derp.json")
		content, err := json.Marshal(&tailcfg.DERPMap{
			Regions: map[int]*tailcfg.DERPRegion{
				1: {
					Nodes: []*tailcfg.DERPNode{{
						STUNPort: 1234,
					}},
				},
				2: {
					Nodes: []*tailcfg.DERPNode{
						{
							STUNPort: 1234,
						},
						{
							STUNPort: 12345,
						},
						{
							STUNOnly: true,
							STUNPort: 54321,
						},
					},
				},
			},
		})
		require.NoError(t, err)
		err = os.WriteFile(localPath, content, 0o600)
		require.NoError(t, err)
		region := &tailcfg.DERPRegion{
			RegionID: 3,
			Nodes: []*tailcfg.DERPNode{{
				STUNPort: 1234,
			}},
		}
		derpMap, err := tailnet.NewDERPMap(context.Background(), region, []string{"127.0.0.1:54321"}, "", localPath, true)
		require.NoError(t, err)
		require.Len(t, derpMap.Regions, 3)

		require.Len(t, derpMap.Regions[1].Nodes, 1)
		require.EqualValues(t, -1, derpMap.Regions[1].Nodes[0].STUNPort)
		// The STUNOnly node should get removed.
		require.Len(t, derpMap.Regions[2].Nodes, 2)
		require.EqualValues(t, -1, derpMap.Regions[2].Nodes[0].STUNPort)
		require.False(t, derpMap.Regions[2].Nodes[0].STUNOnly)
		require.EqualValues(t, -1, derpMap.Regions[2].Nodes[1].STUNPort)
		require.False(t, derpMap.Regions[2].Nodes[1].STUNOnly)
		// We don't add any nodes ourselves if STUN is disabled.
		require.Len(t, derpMap.Regions[3].Nodes, 1)
		// ... but we still remove the STUN port from existing nodes in the
		// region.
		require.EqualValues(t, -1, derpMap.Regions[3].Nodes[0].STUNPort)
	})
	t.Run("RequireRegions", func(t *testing.T) {
		t.Parallel()
		_, err := tailnet.NewDERPMap(context.Background(), nil, nil, "", "", false)
		require.Error(t, err)
		require.ErrorContains(t, err, "DERP map has no regions")
	})
	t.Run("RequireDERPNodes", func(t *testing.T) {
		t.Parallel()

		// No nodes.
		_, err := tailnet.NewDERPMap(context.Background(), &tailcfg.DERPRegion{}, nil, "", "", false)
		require.Error(t, err)
		require.ErrorContains(t, err, "DERP map has no DERP nodes")

		// No DERP nodes.
		_, err = tailnet.NewDERPMap(context.Background(), &tailcfg.DERPRegion{
			Nodes: []*tailcfg.DERPNode{
				{
					STUNOnly: true,
				},
			},
		}, nil, "", "", false)
		require.Error(t, err)
		require.ErrorContains(t, err, "DERP map has no DERP nodes")
	})
}

func TestExtractDERPLatency(t *testing.T) {
	t.Parallel()

	derpMap := &tailcfg.DERPMap{
		Regions: map[int]*tailcfg.DERPRegion{
			1: {
				RegionID:   1,
				RegionName: "Region One",
				Nodes: []*tailcfg.DERPNode{
					{Name: "node1", RegionID: 1},
				},
			},
			2: {
				RegionID:   2,
				RegionName: "Region Two",
				Nodes: []*tailcfg.DERPNode{
					{Name: "node2", RegionID: 2},
				},
			},
		},
	}

	t.Run("Basic", func(t *testing.T) {
		t.Parallel()
		node := &tailnet.Node{
			DERPLatency: map[string]float64{
				"1-node1": 0.05,
				"2-node2": 0.1,
			},
		}
		latencyMs := tailnet.ExtractDERPLatency(node, derpMap)
		require.EqualValues(t, 50, latencyMs["Region One"].Milliseconds())
		require.EqualValues(t, 100, latencyMs["Region Two"].Milliseconds())
		require.Len(t, latencyMs, 2)
	})

	t.Run("UnknownRegion", func(t *testing.T) {
		t.Parallel()
		node := &tailnet.Node{
			DERPLatency: map[string]float64{
				"999-node999": 0.2,
			},
		}
		latencyMs := tailnet.ExtractDERPLatency(node, derpMap)
		require.EqualValues(t, 200, latencyMs["Unnamed 999"].Milliseconds())
		require.Len(t, latencyMs, 1)
	})

	t.Run("InvalidRegionFormat", func(t *testing.T) {
		t.Parallel()
		node := &tailnet.Node{
			DERPLatency: map[string]float64{
				"invalid":  0.3,
				"1-node1":  0.05,
				"abc-node": 0.15,
			},
		}
		latencyMs := tailnet.ExtractDERPLatency(node, derpMap)
		require.EqualValues(t, 50, latencyMs["Region One"].Milliseconds())
		require.Len(t, latencyMs, 1)
		require.NotContains(t, latencyMs, "invalid")
		require.NotContains(t, latencyMs, "abc-node")
	})

	t.Run("EmptyInput", func(t *testing.T) {
		t.Parallel()
		node := &tailnet.Node{
			DERPLatency: map[string]float64{},
		}
		latencyMs := tailnet.ExtractDERPLatency(node, derpMap)
		require.Empty(t, latencyMs)
	})
}

func TestExtractPreferredDERPName(t *testing.T) {
	t.Parallel()
	derpMap := &tailcfg.DERPMap{
		Regions: map[int]*tailcfg.DERPRegion{
			1: {RegionName: "New York"},
			2: {RegionName: "London"},
		},
	}

	t.Run("UsesPingRegion", func(t *testing.T) {
		t.Parallel()
		pingResult := &ipnstate.PingResult{DERPRegionID: 2}
		node := &tailnet.Node{PreferredDERP: 1}
		result := tailnet.ExtractPreferredDERPName(pingResult, node, derpMap)
		require.Equal(t, "London", result)
	})

	t.Run("UsesNodePreferred", func(t *testing.T) {
		t.Parallel()
		pingResult := &ipnstate.PingResult{DERPRegionID: 0}
		node := &tailnet.Node{PreferredDERP: 1}
		result := tailnet.ExtractPreferredDERPName(pingResult, node, derpMap)
		require.Equal(t, "New York", result)
	})

	t.Run("UnknownRegion", func(t *testing.T) {
		t.Parallel()
		pingResult := &ipnstate.PingResult{DERPRegionID: 99}
		node := &tailnet.Node{PreferredDERP: 1}
		result := tailnet.ExtractPreferredDERPName(pingResult, node, derpMap)
		require.Equal(t, "Unnamed 99", result)
	})
}
