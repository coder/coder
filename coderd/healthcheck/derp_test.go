package healthcheck_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"tailscale.com/derp"
	"tailscale.com/derp/derphttp"
	"tailscale.com/ipn"
	"tailscale.com/tailcfg"
	"tailscale.com/types/key"

	"github.com/coder/coder/coderd/healthcheck"
	"github.com/coder/coder/tailnet"
)

func TestDERP(t *testing.T) {
	t.Parallel()

	t.Run("OK", func(t *testing.T) {
		t.Parallel()

		derpSrv := derp.NewServer(key.NewNode(), func(format string, args ...any) { t.Logf(format, args...) })
		defer derpSrv.Close()
		srv := httptest.NewServer(derphttp.Handler(derpSrv))
		defer srv.Close()

		var (
			ctx        = context.Background()
			report     = healthcheck.DERPReport{}
			derpURL, _ = url.Parse(srv.URL)
			opts       = &healthcheck.DERPReportOptions{
				DERPMap: &tailcfg.DERPMap{Regions: map[int]*tailcfg.DERPRegion{
					1: {
						EmbeddedRelay: true,
						RegionID:      999,
						Nodes: []*tailcfg.DERPNode{{
							Name:             "1a",
							RegionID:         999,
							HostName:         derpURL.Host,
							IPv4:             derpURL.Host,
							STUNPort:         -1,
							InsecureForTests: true,
							ForceHTTP:        true,
						}},
					},
				}},
			}
		)

		err := report.Run(ctx, opts)
		require.NoError(t, err)

		assert.True(t, report.Healthy)
		for _, region := range report.Regions {
			assert.True(t, region.Healthy)
			for _, node := range region.NodeReports {
				assert.True(t, node.Healthy)
				assert.True(t, node.CanExchangeMessages)
				// TODO: test this without serializing time.Time over the wire.
				// assert.Positive(t, node.RoundTripPing)
				assert.Len(t, node.ClientLogs, 2)
				assert.Len(t, node.ClientLogs[0], 1)
				assert.Len(t, node.ClientErrs[0], 0)
				assert.Len(t, node.ClientLogs[1], 1)
				assert.Len(t, node.ClientErrs[1], 0)

				assert.False(t, node.STUN.Enabled)
				assert.False(t, node.STUN.CanSTUN)
				assert.NoError(t, node.STUN.Error)
			}
		}
	})

	t.Run("OK/Tailscale/Dallas", func(t *testing.T) {
		t.Parallel()

		derpSrv := derp.NewServer(key.NewNode(), func(format string, args ...any) { t.Logf(format, args...) })
		defer derpSrv.Close()
		srv := httptest.NewServer(derphttp.Handler(derpSrv))
		defer srv.Close()

		var (
			ctx    = context.Background()
			report = healthcheck.DERPReport{}
			opts   = &healthcheck.DERPReportOptions{
				DERPMap: tsDERPMap(ctx, t),
			}
		)
		// Only include the Dallas region
		opts.DERPMap.Regions = map[int]*tailcfg.DERPRegion{9: opts.DERPMap.Regions[9]}

		err := report.Run(ctx, opts)
		require.NoError(t, err)

		assert.True(t, report.Healthy)
		for _, region := range report.Regions {
			assert.True(t, region.Healthy)
			for _, node := range region.NodeReports {
				assert.True(t, node.Healthy)
				assert.True(t, node.CanExchangeMessages)
				// TODO: test this without serializing time.Time over the wire.
				// assert.Positive(t, node.RoundTripPing)
				assert.Len(t, node.ClientLogs, 2)
				assert.Len(t, node.ClientLogs[0], 1)
				assert.Len(t, node.ClientErrs[0], 0)
				assert.Len(t, node.ClientLogs[1], 1)
				assert.Len(t, node.ClientErrs[1], 0)

				assert.True(t, node.STUN.Enabled)
				assert.True(t, node.STUN.CanSTUN)
				assert.NoError(t, node.STUN.Error)
			}
		}
	})

	t.Run("ForceWebsockets", func(t *testing.T) {
		t.Parallel()

		derpSrv := derp.NewServer(key.NewNode(), func(format string, args ...any) { t.Logf(format, args...) })
		defer derpSrv.Close()
		handler, closeHandler := tailnet.WithWebsocketSupport(derpSrv, derphttp.Handler(derpSrv))
		defer closeHandler()

		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Header.Get("Upgrade") == "DERP" {
				w.WriteHeader(http.StatusBadRequest)
				w.Write([]byte("bad request"))
				return
			}

			handler.ServeHTTP(w, r)
		}))

		var (
			ctx        = context.Background()
			report     = healthcheck.DERPReport{}
			derpURL, _ = url.Parse(srv.URL)
			opts       = &healthcheck.DERPReportOptions{
				DERPMap: &tailcfg.DERPMap{Regions: map[int]*tailcfg.DERPRegion{
					1: {
						EmbeddedRelay: true,
						RegionID:      999,
						Nodes: []*tailcfg.DERPNode{{
							Name:             "1a",
							RegionID:         999,
							HostName:         derpURL.Host,
							IPv4:             derpURL.Host,
							STUNPort:         -1,
							InsecureForTests: true,
							ForceHTTP:        true,
						}},
					},
				}},
			}
		)

		report.Run(ctx, opts)

		assert.False(t, report.Healthy)
		for _, region := range report.Regions {
			assert.False(t, region.Healthy)
			for _, node := range region.NodeReports {
				assert.False(t, node.Healthy)
				assert.True(t, node.CanExchangeMessages)
				// TODO: test this without serializing time.Time over the wire.
				// assert.Positive(t, node.RoundTripPing)
				assert.Len(t, node.ClientLogs, 2)
				assert.Len(t, node.ClientLogs[0], 3)
				assert.Len(t, node.ClientLogs[1], 3)
				assert.Len(t, node.ClientErrs, 2)
				assert.Len(t, node.ClientErrs[0], 1)
				assert.Len(t, node.ClientErrs[1], 1)
				assert.True(t, node.UsesWebsocket)

				assert.False(t, node.STUN.Enabled)
				assert.False(t, node.STUN.CanSTUN)
				assert.NoError(t, node.STUN.Error)
			}
		}
	})

	t.Run("OK/STUNOnly", func(t *testing.T) {
		t.Parallel()

		var (
			ctx    = context.Background()
			report = healthcheck.DERPReport{}
			opts   = &healthcheck.DERPReportOptions{
				DERPMap: &tailcfg.DERPMap{Regions: map[int]*tailcfg.DERPRegion{
					1: {
						EmbeddedRelay: true,
						RegionID:      999,
						Nodes: []*tailcfg.DERPNode{{
							Name:             "999stun0",
							RegionID:         999,
							HostName:         "stun.l.google.com",
							STUNPort:         19302,
							STUNOnly:         true,
							InsecureForTests: true,
							ForceHTTP:        true,
						}},
					},
				}},
			}
		)

		err := report.Run(ctx, opts)
		require.NoError(t, err)

		assert.True(t, report.Healthy)
		for _, region := range report.Regions {
			assert.True(t, region.Healthy)
			for _, node := range region.NodeReports {
				assert.True(t, node.Healthy)
				assert.False(t, node.CanExchangeMessages)
				assert.Len(t, node.ClientLogs, 0)

				assert.True(t, node.STUN.Enabled)
				assert.True(t, node.STUN.CanSTUN)
				assert.NoError(t, node.STUN.Error)
			}
		}
	})
}

func tsDERPMap(ctx context.Context, t testing.TB) *tailcfg.DERPMap {
	req, err := http.NewRequestWithContext(ctx, "GET", ipn.DefaultControlURL+"/derpmap/default", nil)
	require.NoError(t, err)

	res, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer res.Body.Close()
	require.Equal(t, http.StatusOK, res.StatusCode)

	var derpMap tailcfg.DERPMap
	err = json.NewDecoder(io.LimitReader(res.Body, 1<<20)).Decode(&derpMap)
	require.NoError(t, err)

	return &derpMap
}
