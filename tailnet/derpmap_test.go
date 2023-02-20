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
	"tailscale.com/tailcfg"

	"github.com/coder/coder/tailnet"
)

func TestNewDERPMap(t *testing.T) {
	t.Parallel()
	t.Run("WithoutRemoteURL", func(t *testing.T) {
		t.Parallel()
		derpMap, err := tailnet.NewDERPMap(context.Background(), &tailcfg.DERPRegion{
			RegionID: 1,
			Nodes:    []*tailcfg.DERPNode{{}},
		}, []string{"stun.google.com:2345"}, "", "")
		require.NoError(t, err)
		require.Len(t, derpMap.Regions[1].Nodes, 2)
	})
	t.Run("RemoteURL", func(t *testing.T) {
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
		derpMap, err := tailnet.NewDERPMap(context.Background(), &tailcfg.DERPRegion{
			RegionID: 2,
		}, []string{}, server.URL, "")
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
		}, []string{}, server.URL, "")
		require.Error(t, err)
	})
	t.Run("LocalPath", func(t *testing.T) {
		t.Parallel()
		localPath := filepath.Join(t.TempDir(), "derp.json")
		content, err := json.Marshal(&tailcfg.DERPMap{
			Regions: map[int]*tailcfg.DERPRegion{
				1: {},
			},
		})
		require.NoError(t, err)
		err = os.WriteFile(localPath, content, 0o600)
		require.NoError(t, err)
		derpMap, err := tailnet.NewDERPMap(context.Background(), &tailcfg.DERPRegion{
			RegionID: 2,
		}, []string{}, "", localPath)
		require.NoError(t, err)
		require.Len(t, derpMap.Regions, 2)
	})
}
