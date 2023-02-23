package coderd_test

import (
	"context"
	"io"
	"net/http"
	"net/netip"
	"strconv"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/goleak"
	"tailscale.com/tailcfg"

	"cdr.dev/slog"
	"cdr.dev/slog/sloggers/slogtest"

	"github.com/coder/coder/buildinfo"
	"github.com/coder/coder/coderd/coderdtest"
	"github.com/coder/coder/tailnet"
	"github.com/coder/coder/testutil"
)

func TestMain(m *testing.M) {
	goleak.VerifyTestMain(m)
}

func TestBuildInfo(t *testing.T) {
	t.Parallel()
	client := coderdtest.New(t, nil)

	ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
	defer cancel()

	buildInfo, err := client.BuildInfo(ctx)
	require.NoError(t, err)
	require.Equal(t, buildinfo.ExternalURL(), buildInfo.ExternalURL, "external URL")
	require.Equal(t, buildinfo.Version(), buildInfo.Version, "version")
}

func TestDERP(t *testing.T) {
	t.Parallel()
	client := coderdtest.New(t, nil)

	logger := slogtest.Make(t, nil).Leveled(slog.LevelDebug)

	derpPort, err := strconv.Atoi(client.URL.Port())
	require.NoError(t, err)
	derpMap := &tailcfg.DERPMap{
		Regions: map[int]*tailcfg.DERPRegion{
			1: {
				RegionID:   1,
				RegionCode: "cdr",
				RegionName: "Coder",
				Nodes: []*tailcfg.DERPNode{{
					Name:      "1a",
					RegionID:  1,
					HostName:  client.URL.Hostname(),
					DERPPort:  derpPort,
					STUNPort:  -1,
					ForceHTTP: true,
				}},
			},
		},
	}
	w1IP := tailnet.IP()
	w1, err := tailnet.NewConn(&tailnet.Options{
		Addresses: []netip.Prefix{netip.PrefixFrom(w1IP, 128)},
		Logger:    logger.Named("w1"),
		DERPMap:   derpMap,
	})
	require.NoError(t, err)

	w2, err := tailnet.NewConn(&tailnet.Options{
		Addresses: []netip.Prefix{netip.PrefixFrom(tailnet.IP(), 128)},
		Logger:    logger.Named("w2"),
		DERPMap:   derpMap,
	})
	require.NoError(t, err)

	w2Ready := make(chan struct{})
	w2ReadyOnce := sync.Once{}
	w1.SetNodeCallback(func(node *tailnet.Node) {
		w2.UpdateNodes([]*tailnet.Node{node}, false)
		w2ReadyOnce.Do(func() {
			close(w2Ready)
		})
	})
	w2.SetNodeCallback(func(node *tailnet.Node) {
		w1.UpdateNodes([]*tailnet.Node{node}, false)
	})

	conn := make(chan struct{})
	go func() {
		listener, err := w1.Listen("tcp", ":35565")
		assert.NoError(t, err)
		defer listener.Close()
		conn <- struct{}{}
		nc, err := listener.Accept()
		assert.NoError(t, err)
		_ = nc.Close()
		conn <- struct{}{}
	}()

	<-conn
	<-w2Ready
	nc, err := w2.DialContextTCP(context.Background(), netip.AddrPortFrom(w1IP, 35565))
	require.NoError(t, err)
	_ = nc.Close()
	<-conn

	w1.Close()
	w2.Close()
}

func TestDERPLatencyCheck(t *testing.T) {
	t.Parallel()
	client := coderdtest.New(t, nil)
	res, err := client.Request(context.Background(), http.MethodGet, "/derp/latency-check", nil)
	require.NoError(t, err)
	defer res.Body.Close()
	require.Equal(t, http.StatusOK, res.StatusCode)
}

func TestHealthz(t *testing.T) {
	t.Parallel()
	client := coderdtest.New(t, nil)

	res, err := client.Request(context.Background(), http.MethodGet, "/healthz", nil)
	require.NoError(t, err)
	defer res.Body.Close()

	require.Equal(t, http.StatusOK, res.StatusCode)
	body, err := io.ReadAll(res.Body)
	require.NoError(t, err)

	assert.Equal(t, "OK", string(body))
}

func TestSwagger(t *testing.T) {
	t.Parallel()

	const swaggerEndpoint = "/swagger"
	t.Run("endpoint enabled", func(t *testing.T) {
		t.Parallel()

		client := coderdtest.New(t, &coderdtest.Options{
			SwaggerEndpoint: true,
		})

		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitMedium)
		defer cancel()

		resp, err := requestWithRetries(ctx, t, client, http.MethodGet, swaggerEndpoint, nil)
		require.NoError(t, err)

		body, err := io.ReadAll(resp.Body)
		require.NoError(t, err)
		defer resp.Body.Close()

		require.Contains(t, string(body), "Swagger UI")
	})
	t.Run("doc.json exposed", func(t *testing.T) {
		t.Parallel()

		client := coderdtest.New(t, &coderdtest.Options{
			SwaggerEndpoint: true,
		})

		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitMedium)
		defer cancel()

		resp, err := requestWithRetries(ctx, t, client, http.MethodGet, swaggerEndpoint+"/doc.json", nil)
		require.NoError(t, err)

		body, err := io.ReadAll(resp.Body)
		require.NoError(t, err)
		defer resp.Body.Close()

		require.Contains(t, string(body), `"swagger": "2.0"`)
	})
	t.Run("endpoint disabled by default", func(t *testing.T) {
		t.Parallel()

		client := coderdtest.New(t, nil)

		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitMedium)
		defer cancel()

		resp, err := requestWithRetries(ctx, t, client, http.MethodGet, swaggerEndpoint, nil)
		require.NoError(t, err)

		body, err := io.ReadAll(resp.Body)
		require.NoError(t, err)
		defer resp.Body.Close()

		require.Equal(t, "<pre>\n</pre>\n", string(body))
	})
	t.Run("doc.json disabled by default", func(t *testing.T) {
		t.Parallel()

		client := coderdtest.New(t, nil)

		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitMedium)
		defer cancel()

		resp, err := requestWithRetries(ctx, t, client, http.MethodGet, swaggerEndpoint+"/doc.json", nil)
		require.NoError(t, err)

		body, err := io.ReadAll(resp.Body)
		require.NoError(t, err)
		defer resp.Body.Close()

		require.Equal(t, "<pre>\n</pre>\n", string(body))
	})
}
