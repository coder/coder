package coderdenttest

import (
	"context"
	"crypto/tls"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"regexp"
	"sync"
	"testing"

	"github.com/moby/moby/pkg/namesgenerator"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"cdr.dev/slog"
	"cdr.dev/slog/sloggers/slogtest"
	"github.com/coder/coder/v2/coderd/httpapi"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/enterprise/coderd"
	"github.com/coder/coder/v2/enterprise/wsproxy"
)

type ProxyOptions struct {
	Name        string
	Experiments codersdk.Experiments

	TLSCertificates []tls.Certificate
	AppHostname     string
	DisablePathApps bool
	DerpDisabled    bool
	DerpOnly        bool

	// ProxyURL is optional
	ProxyURL *url.URL

	// FlushStats is optional
	FlushStats chan chan<- struct{}
}

// NewWorkspaceProxy will configure a wsproxy.Server with the given options.
// The new wsproxy will register itself with the given coderd.API instance.
// The first user owner client is required to create the wsproxy on the coderd
// api server.
func NewWorkspaceProxy(t *testing.T, coderdAPI *coderd.API, owner *codersdk.Client, options *ProxyOptions) *wsproxy.Server {
	ctx, cancelFunc := context.WithCancel(context.Background())
	t.Cleanup(cancelFunc)

	if options == nil {
		options = &ProxyOptions{}
	}

	// HTTP Server. We have to start this once to get the access URL to start
	// the workspace proxy with. The workspace proxy has the handler, so the
	// http server will start with a 503 until the proxy is started.
	var mutex sync.RWMutex
	var handler http.Handler
	srv := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mutex.RLock()
		defer mutex.RUnlock()
		if handler == nil {
			http.Error(w, "handler not set", http.StatusServiceUnavailable)
			return
		}

		handler.ServeHTTP(w, r)
	}))
	srv.Config.BaseContext = func(_ net.Listener) context.Context {
		return ctx
	}
	if options.TLSCertificates != nil {
		srv.TLS = &tls.Config{
			Certificates: options.TLSCertificates,
			MinVersion:   tls.VersionTLS12,
		}
		srv.StartTLS()
	} else {
		srv.Start()
	}
	t.Cleanup(srv.Close)

	tcpAddr, ok := srv.Listener.Addr().(*net.TCPAddr)
	require.True(t, ok)

	serverURL, err := url.Parse(srv.URL)
	require.NoError(t, err)

	serverURL.Host = fmt.Sprintf("localhost:%d", tcpAddr.Port)

	accessURL := options.ProxyURL
	if accessURL == nil {
		accessURL = serverURL
	}

	var appHostnameRegex *regexp.Regexp
	if options.AppHostname != "" {
		var err error
		appHostnameRegex, err = httpapi.CompileHostnamePattern(options.AppHostname)
		require.NoError(t, err)
	}

	if options.Name == "" {
		options.Name = namesgenerator.GetRandomName(1)
	}

	proxyRes, err := owner.CreateWorkspaceProxy(ctx, codersdk.CreateWorkspaceProxyRequest{
		Name: options.Name,
		Icon: "/emojis/flag.png",
	})
	require.NoError(t, err, "failed to create workspace proxy")

	// Inherit collector options from coderd, but keep the wsproxy reporter.
	statsCollectorOptions := coderdAPI.Options.WorkspaceAppsStatsCollectorOptions
	statsCollectorOptions.Reporter = nil
	if options.FlushStats != nil {
		statsCollectorOptions.Flush = options.FlushStats
	}

	wssrv, err := wsproxy.New(ctx, &wsproxy.Options{
		Logger:            slogtest.Make(t, nil).Leveled(slog.LevelDebug),
		Experiments:       options.Experiments,
		DashboardURL:      coderdAPI.AccessURL,
		AccessURL:         accessURL,
		AppHostname:       options.AppHostname,
		AppHostnameRegex:  appHostnameRegex,
		RealIPConfig:      coderdAPI.RealIPConfig,
		Tracing:           coderdAPI.TracerProvider,
		APIRateLimit:      coderdAPI.APIRateLimit,
		SecureAuthCookie:  coderdAPI.SecureAuthCookie,
		ProxySessionToken: proxyRes.ProxyToken,
		DisablePathApps:   options.DisablePathApps,
		// We need a new registry to not conflict with the coderd internal
		// proxy metrics.
		PrometheusRegistry:     prometheus.NewRegistry(),
		DERPEnabled:            !options.DerpDisabled,
		DERPOnly:               options.DerpOnly,
		DERPServerRelayAddress: accessURL.String(),
		StatsCollectorOptions:  statsCollectorOptions,
	})
	require.NoError(t, err)
	t.Cleanup(func() {
		err := wssrv.Close()
		assert.NoError(t, err)
	})

	mutex.Lock()
	handler = wssrv.Handler
	mutex.Unlock()

	return wssrv
}
