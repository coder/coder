package derphealth_test

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

	"github.com/coder/coder/v2/coderd/healthcheck/derphealth"
	"github.com/coder/coder/v2/tailnet"
	"github.com/coder/coder/v2/testutil"
)

//nolint:tparallel
func TestDERP(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping healthcheck test in short mode, they reach out over the network.")
	}

	t.Run("OK", func(t *testing.T) {
		t.Parallel()

		derpSrv := derp.NewServer(key.NewNode(), func(format string, args ...any) { t.Logf(format, args...) })
		defer derpSrv.Close()
		srv := httptest.NewServer(derphttp.Handler(derpSrv))
		defer srv.Close()

		var (
			ctx        = context.Background()
			report     = derphealth.Report{}
			derpURL, _ = url.Parse(srv.URL)
			opts       = &derphealth.ReportOptions{
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

		assert.True(t, report.Healthy)
		for _, region := range report.Regions {
			assert.True(t, region.Healthy)
			for _, node := range region.NodeReports {
				assert.True(t, node.Healthy)
				assert.True(t, node.CanExchangeMessages)
				assert.NotEmpty(t, node.RoundTripPing)
				assert.Len(t, node.ClientLogs, 2)
				assert.Len(t, node.ClientLogs[0], 3)
				assert.Len(t, node.ClientErrs[0], 0)
				assert.Len(t, node.ClientLogs[1], 3)
				assert.Len(t, node.ClientErrs[1], 0)

				assert.False(t, node.STUN.Enabled)
				assert.False(t, node.STUN.CanSTUN)
				assert.Nil(t, node.STUN.Error)
			}
		}
	})

	t.Run("Tailscale/Dallas/OK", func(t *testing.T) {
		t.Parallel()

		if testutil.InCI() {
			t.Skip("This test depends on reaching out over the network to Tailscale servers, which is inherently flaky.")
		}

		derpSrv := derp.NewServer(key.NewNode(), func(format string, args ...any) { t.Logf(format, args...) })
		defer derpSrv.Close()
		srv := httptest.NewServer(derphttp.Handler(derpSrv))
		defer srv.Close()

		var (
			ctx    = context.Background()
			report = derphealth.Report{}
			opts   = &derphealth.ReportOptions{
				DERPMap: tsDERPMap(ctx, t),
			}
		)
		// Only include the Dallas region
		opts.DERPMap.Regions = map[int]*tailcfg.DERPRegion{9: opts.DERPMap.Regions[9]}

		report.Run(ctx, opts)

		assert.True(t, report.Healthy)
		for _, region := range report.Regions {
			assert.True(t, region.Healthy)
			for _, node := range region.NodeReports {
				assert.True(t, node.Healthy)
				assert.True(t, node.CanExchangeMessages)
				assert.NotEmpty(t, node.RoundTripPing)
				assert.Len(t, node.ClientLogs, 2)
				// the exact number of logs depends on the certificates, which we don't control.
				assert.GreaterOrEqual(t, len(node.ClientLogs[0]), 1)
				assert.Len(t, node.ClientErrs[0], 0)
				// the exact number of logs depends on the certificates, which we don't control.
				assert.GreaterOrEqual(t, len(node.ClientLogs[1]), 1)
				assert.Len(t, node.ClientErrs[1], 0)

				assert.True(t, node.STUN.Enabled)
				assert.True(t, node.STUN.CanSTUN)
				assert.Nil(t, node.STUN.Error)
			}
		}
	})

	t.Run("FailoverToWebsockets", func(t *testing.T) {
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
			report     = derphealth.Report{}
			derpURL, _ = url.Parse(srv.URL)
			opts       = &derphealth.ReportOptions{
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

		assert.True(t, report.Healthy)
		assert.NotEmpty(t, report.Warnings)
		for _, region := range report.Regions {
			assert.True(t, region.Healthy)
			assert.NotEmpty(t, region.Warnings)
			for _, node := range region.NodeReports {
				assert.True(t, node.Healthy)
				assert.NotEmpty(t, node.Warnings)
				assert.True(t, node.CanExchangeMessages)
				assert.NotEmpty(t, node.RoundTripPing)
				assert.Len(t, node.ClientLogs, 2)
				assert.Len(t, node.ClientLogs[0], 5)
				assert.Len(t, node.ClientLogs[1], 5)
				assert.Len(t, node.ClientErrs, 2)
				assert.Len(t, node.ClientErrs[0], 1) // this
				assert.Len(t, node.ClientErrs[1], 1)
				assert.True(t, node.UsesWebsocket)

				assert.False(t, node.STUN.Enabled)
				assert.False(t, node.STUN.CanSTUN)
				assert.Nil(t, node.STUN.Error)
			}
		}
	})

	t.Run("STUNOnly/OK", func(t *testing.T) {
		t.Parallel()

		var (
			ctx    = context.Background()
			report = derphealth.Report{}
			opts   = &derphealth.ReportOptions{
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

		report.Run(ctx, opts)

		assert.True(t, report.Healthy)
		for _, region := range report.Regions {
			assert.True(t, region.Healthy)
			for _, node := range region.NodeReports {
				assert.True(t, node.Healthy)
				assert.False(t, node.CanExchangeMessages)
				assert.Len(t, node.ClientLogs, 0)

				assert.True(t, node.STUN.Enabled)
				assert.True(t, node.STUN.CanSTUN)
				assert.Nil(t, node.STUN.Error)
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
