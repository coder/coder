package codersdk_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"tailscale.com/tailcfg"

	"cdr.dev/slog/sloggers/slogtest"
	"github.com/coder/coder/coderd/httpapi"
	"github.com/coder/coder/codersdk/agentsdk"
	"github.com/coder/coder/testutil"
)

func TestWorkspaceAgentMetadata(t *testing.T) {
	t.Parallel()
	// This test ensures that the DERP map returned properly
	// mutates built-in DERPs with the client access URL.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		httpapi.Write(context.Background(), w, http.StatusOK, agentsdk.Manifest{
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
	client := agentsdk.New(parsed)
	manifest, err := client.Manifest(context.Background())
	require.NoError(t, err)
	region := manifest.DERPMap.Regions[1]
	require.True(t, region.EmbeddedRelay)
	require.Len(t, region.Nodes, 1)
	node := region.Nodes[0]
	require.Equal(t, parsed.Hostname(), node.HostName)
	require.Equal(t, parsed.Port(), strconv.Itoa(node.DERPPort))
}

func TestAgentReportStats(t *testing.T) {
	t.Parallel()

	var (
		numReports       atomic.Int64
		numIntervalCalls atomic.Int64
		wantInterval     = 5 * time.Millisecond
	)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		numReports.Add(1)
		httpapi.Write(context.Background(), w, http.StatusOK, agentsdk.StatsResponse{
			ReportInterval: wantInterval,
		})
	}))
	parsed, err := url.Parse(srv.URL)
	require.NoError(t, err)
	client := agentsdk.New(parsed)

	assertStatInterval := func(interval time.Duration) {
		numIntervalCalls.Add(1)
		assert.Equal(t, wantInterval, interval)
	}

	chanLen := 3
	statCh := make(chan *agentsdk.Stats, chanLen)
	for i := 0; i < chanLen; i++ {
		statCh <- &agentsdk.Stats{ConnectionsByProto: map[string]int64{}}
	}

	ctx := context.Background()
	closeStream, err := client.ReportStats(ctx, slogtest.Make(t, nil), statCh, assertStatInterval)
	require.NoError(t, err)
	defer closeStream.Close()

	require.Eventually(t,
		func() bool { return numReports.Load() >= 3 },
		testutil.WaitMedium, testutil.IntervalFast,
	)
	closeStream.Close()
	require.Equal(t, int64(1), numIntervalCalls.Load())
}
