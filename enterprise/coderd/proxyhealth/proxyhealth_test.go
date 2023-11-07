package proxyhealth_test

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"
	"golang.org/x/xerrors"

	"cdr.dev/slog/sloggers/slogtest"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbgen"
	"github.com/coder/coder/v2/coderd/database/dbmem"
	"github.com/coder/coder/v2/coderd/httpapi"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/enterprise/coderd/proxyhealth"
	"github.com/coder/coder/v2/testutil"
)

func insertProxy(t *testing.T, db database.Store, url string) database.WorkspaceProxy {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitShort)
	defer cancel()

	proxy, _ := dbgen.WorkspaceProxy(t, db, database.WorkspaceProxy{})
	_, err := db.RegisterWorkspaceProxy(ctx, database.RegisterWorkspaceProxyParams{
		Url:              url,
		WildcardHostname: "",
		ID:               proxy.ID,
	})
	require.NoError(t, err, "failed to update proxy")
	return proxy
}

// Test the nil guard for experiment off cases.
func TestProxyHealth_Nil(t *testing.T) {
	t.Parallel()
	var ph *proxyhealth.ProxyHealth

	require.NotNil(t, ph.HealthStatus())
}

func TestProxyHealth_Unregistered(t *testing.T) {
	t.Parallel()
	db := dbmem.New()

	proxies := []database.WorkspaceProxy{
		insertProxy(t, db, ""),
		insertProxy(t, db, ""),
	}

	ph, err := proxyhealth.New(&proxyhealth.Options{
		Interval: 0,
		DB:       db,
		Logger:   slogtest.Make(t, nil),
	})
	require.NoError(t, err, "failed to create proxy health")

	ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitShort)
	defer cancel()

	err = ph.ForceUpdate(ctx)
	require.NoError(t, err, "failed to force update")
	for _, p := range proxies {
		require.Equal(t, ph.HealthStatus()[p.ID].Status, proxyhealth.Unregistered, "expect unregistered proxy")
	}
}

func TestProxyHealth_Unhealthy(t *testing.T) {
	t.Parallel()
	db := dbmem.New()

	srvBadReport := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		httpapi.Write(context.Background(), w, http.StatusOK, codersdk.ProxyHealthReport{
			Errors: []string{"We have a problem!"},
		})
	}))
	defer srvBadReport.Close()

	srvBadCode := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
	}))
	defer srvBadCode.Close()

	proxies := []database.WorkspaceProxy{
		// Same url for both, just checking multiple proxies are checked.
		insertProxy(t, db, srvBadReport.URL),
		insertProxy(t, db, srvBadCode.URL),
	}

	ph, err := proxyhealth.New(&proxyhealth.Options{
		Interval: 0,
		DB:       db,
		Logger:   slogtest.Make(t, nil),
		Client:   srvBadReport.Client(),
	})
	require.NoError(t, err, "failed to create proxy health")

	ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitShort)
	defer cancel()

	err = ph.ForceUpdate(ctx)
	require.NoError(t, err, "failed to force update")
	for _, p := range proxies {
		require.Equal(t, ph.HealthStatus()[p.ID].Status, proxyhealth.Unhealthy, "expect reachable proxy")
	}
}

func TestProxyHealth_Reachable(t *testing.T) {
	t.Parallel()
	db := dbmem.New()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		httpapi.Write(context.Background(), w, http.StatusOK, codersdk.ProxyHealthReport{
			Warnings: []string{"No problems, just a warning"},
		})
	}))
	defer srv.Close()

	proxies := []database.WorkspaceProxy{
		// Same url for both, just checking multiple proxies are checked.
		insertProxy(t, db, srv.URL),
		insertProxy(t, db, srv.URL),
	}

	ph, err := proxyhealth.New(&proxyhealth.Options{
		Interval: 0,
		DB:       db,
		Logger:   slogtest.Make(t, nil),
		Client:   srv.Client(),
	})
	require.NoError(t, err, "failed to create proxy health")

	ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitShort)
	defer cancel()

	err = ph.ForceUpdate(ctx)
	require.NoError(t, err, "failed to force update")
	for _, p := range proxies {
		require.Equal(t, ph.HealthStatus()[p.ID].Status, proxyhealth.Healthy, "expect reachable proxy")
	}
}

func TestProxyHealth_Unreachable(t *testing.T) {
	t.Parallel()
	db := dbmem.New()

	cli := &http.Client{
		Transport: &http.Transport{
			DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
				return nil, xerrors.New("Always fail")
			},
		},
	}

	proxies := []database.WorkspaceProxy{
		// example.com is a real domain, but the client should always fail.
		insertProxy(t, db, "https://example.com"),
		insertProxy(t, db, "https://random.example.com"),
	}

	ph, err := proxyhealth.New(&proxyhealth.Options{
		Interval: 0,
		DB:       db,
		Logger:   slogtest.Make(t, nil),
		Client:   cli,
	})
	require.NoError(t, err, "failed to create proxy health")

	ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitShort)
	defer cancel()

	err = ph.ForceUpdate(ctx)
	require.NoError(t, err, "failed to force update")
	for _, p := range proxies {
		require.Equal(t, ph.HealthStatus()[p.ID].Status, proxyhealth.Unreachable, "expect unreachable proxy")
	}
}
