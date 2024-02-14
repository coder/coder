package coderd_test

import (
	"context"
	"net"
	"net/http"
	"net/url"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/cli/clibase"
	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbgen"
	"github.com/coder/coder/v2/coderd/database/dbtestutil"
	"github.com/coder/coder/v2/coderd/httpmw"
	"github.com/coder/coder/v2/coderd/workspaceapps"
	"github.com/coder/coder/v2/coderd/workspaceapps/apptest"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/testutil"
)

func TestGetAppHost(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name        string
		accessURL   string
		appHostname string
		expected    string
	}{
		{
			name:        "OK",
			accessURL:   "https://test.coder.com",
			appHostname: "*.test.coder.com",
			expected:    "*.test.coder.com",
		},
		{
			name:        "None",
			accessURL:   "https://test.coder.com",
			appHostname: "",
			expected:    "",
		},
		{
			name:        "OKWithPort",
			accessURL:   "https://test.coder.com:8443",
			appHostname: "*.test.coder.com",
			expected:    "*.test.coder.com:8443",
		},
		{
			name:        "OKWithSuffix",
			accessURL:   "https://test.coder.com:8443",
			appHostname: "*--suffix.test.coder.com",
			expected:    "*--suffix.test.coder.com:8443",
		},
	}
	for _, c := range cases {
		c := c
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()

			accessURL, err := url.Parse(c.accessURL)
			require.NoError(t, err)

			client := coderdtest.New(t, &coderdtest.Options{
				AccessURL:   accessURL,
				AppHostname: c.appHostname,
			})

			ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
			defer cancel()

			// Should not leak to unauthenticated users.
			host, err := client.AppHost(ctx)
			require.Error(t, err)
			require.Equal(t, "", host.Host)

			_ = coderdtest.CreateFirstUser(t, client)
			host, err = client.AppHost(ctx)
			require.NoError(t, err)
			require.Equal(t, c.expected, host.Host)
		})
	}
}

func TestWorkspaceApplicationAuth(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name             string
		accessURL        string
		appHostname      string
		proxyURL         string
		proxyAppHostname string

		redirectURI    string
		expectRedirect string
	}{
		{
			name:             "OK",
			accessURL:        "https://test.coder.com",
			appHostname:      "*.test.coder.com",
			proxyURL:         "https://proxy.test.coder.com",
			proxyAppHostname: "*.proxy.test.coder.com",
			redirectURI:      "https://something.test.coder.com",
			expectRedirect:   "https://something.test.coder.com",
		},
		{
			name:             "ProxyPathOK",
			accessURL:        "https://test.coder.com",
			appHostname:      "*.test.coder.com",
			proxyURL:         "https://proxy.test.coder.com",
			proxyAppHostname: "*.proxy.test.coder.com",
			redirectURI:      "https://proxy.test.coder.com/path",
			expectRedirect:   "https://proxy.test.coder.com/path",
		},
		{
			name:             "ProxySubdomainOK",
			accessURL:        "https://test.coder.com",
			appHostname:      "*.test.coder.com",
			proxyURL:         "https://proxy.test.coder.com",
			proxyAppHostname: "*.proxy.test.coder.com",
			redirectURI:      "https://something.proxy.test.coder.com/path?yeah=true",
			expectRedirect:   "https://something.proxy.test.coder.com/path?yeah=true",
		},
		{
			name:             "ProxySubdomainSuffixOK",
			accessURL:        "https://test.coder.com",
			appHostname:      "*.test.coder.com",
			proxyURL:         "https://proxy.test.coder.com",
			proxyAppHostname: "*--suffix.proxy.test.coder.com",
			redirectURI:      "https://something--suffix.proxy.test.coder.com/",
			expectRedirect:   "https://something--suffix.proxy.test.coder.com/",
		},
		{
			name:             "NormalizeSchemePrimaryAppHostname",
			accessURL:        "https://test.coder.com",
			appHostname:      "*.test.coder.com",
			proxyURL:         "https://proxy.test.coder.com",
			proxyAppHostname: "*.proxy.test.coder.com",
			redirectURI:      "http://x.test.coder.com",
			expectRedirect:   "https://x.test.coder.com",
		},
		{
			name:             "NormalizeSchemeProxyAppHostname",
			accessURL:        "https://test.coder.com",
			appHostname:      "*.test.coder.com",
			proxyURL:         "https://proxy.test.coder.com",
			proxyAppHostname: "*.proxy.test.coder.com",
			redirectURI:      "http://x.proxy.test.coder.com",
			expectRedirect:   "https://x.proxy.test.coder.com",
		},
		{
			name:             "NoneError",
			accessURL:        "https://test.coder.com",
			appHostname:      "*.test.coder.com",
			proxyURL:         "https://proxy.test.coder.com",
			proxyAppHostname: "*.proxy.test.coder.com",
			redirectURI:      "",
			expectRedirect:   "",
		},
		{
			name:             "PrimaryAccessURLError",
			accessURL:        "https://test.coder.com",
			appHostname:      "*.test.coder.com",
			proxyURL:         "https://proxy.test.coder.com",
			proxyAppHostname: "*.proxy.test.coder.com",
			redirectURI:      "https://test.coder.com/",
			expectRedirect:   "",
		},
		{
			name:             "OtherError",
			accessURL:        "https://test.coder.com",
			appHostname:      "*.test.coder.com",
			proxyURL:         "https://proxy.test.coder.com",
			proxyAppHostname: "*.proxy.test.coder.com",
			redirectURI:      "https://example.com/",
			expectRedirect:   "",
		},
	}

	for _, c := range cases {
		c := c
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()

			db, pubsub := dbtestutil.NewDB(t)

			accessURL, err := url.Parse(c.accessURL)
			require.NoError(t, err)

			client := coderdtest.New(t, &coderdtest.Options{
				Database:    db,
				Pubsub:      pubsub,
				AccessURL:   accessURL,
				AppHostname: c.appHostname,
			})
			_ = coderdtest.CreateFirstUser(t, client)

			// Disable redirects.
			client.HTTPClient.CheckRedirect = func(_ *http.Request, _ []*http.Request) error {
				return http.ErrUseLastResponse
			}

			_, _ = dbgen.WorkspaceProxy(t, db, database.WorkspaceProxy{
				Url:              c.proxyURL,
				WildcardHostname: c.proxyAppHostname,
			})

			ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
			defer cancel()

			resp, err := client.Request(ctx, http.MethodGet, "/api/v2/applications/auth-redirect", nil, func(req *http.Request) {
				q := req.URL.Query()
				q.Set("redirect_uri", c.redirectURI)
				req.URL.RawQuery = q.Encode()
			})
			require.NoError(t, err)
			defer resp.Body.Close()
			if resp.StatusCode != http.StatusSeeOther {
				err = codersdk.ReadBodyAsError(resp)
				if c.expectRedirect == "" {
					require.Error(t, err)
					return
				}
				require.NoError(t, err)
				return
			}
			if c.expectRedirect == "" {
				t.Fatal("expected a failure but got a success")
			}

			loc, err := resp.Location()
			require.NoError(t, err)
			q := loc.Query()

			// Verify the API key is set.
			encryptedAPIKey := loc.Query().Get(workspaceapps.SubdomainProxyAPIKeyParam)
			require.NotEmpty(t, encryptedAPIKey, "no API key was set in the query parameters")

			// Strip the API key from the actual redirect URI and compare.
			q.Del(workspaceapps.SubdomainProxyAPIKeyParam)
			loc.RawQuery = q.Encode()
			require.Equal(t, c.expectRedirect, loc.String())

			// The decrypted key is verified in the apptest test suite.
		})
	}
}

func TestWorkspaceApps(t *testing.T) {
	t.Parallel()

	apptest.Run(t, true, func(t *testing.T, opts *apptest.DeploymentOptions) *apptest.Deployment {
		deploymentValues := coderdtest.DeploymentValues(t)
		deploymentValues.DisablePathApps = clibase.Bool(opts.DisablePathApps)
		deploymentValues.Dangerous.AllowPathAppSharing = clibase.Bool(opts.DangerousAllowPathAppSharing)
		deploymentValues.Dangerous.AllowPathAppSiteOwnerAccess = clibase.Bool(opts.DangerousAllowPathAppSiteOwnerAccess)
		deploymentValues.Experiments = append(deploymentValues.Experiments, string(codersdk.ExperimentSharedPorts))

		if opts.DisableSubdomainApps {
			opts.AppHost = ""
		}

		flushStatsCollectorCh := make(chan chan<- struct{}, 1)
		opts.StatsCollectorOptions.Flush = flushStatsCollectorCh
		flushStats := func() {
			flushStatsCollectorDone := make(chan struct{}, 1)
			flushStatsCollectorCh <- flushStatsCollectorDone
			<-flushStatsCollectorDone
		}
		client := coderdtest.New(t, &coderdtest.Options{
			DeploymentValues:         deploymentValues,
			AppHostname:              opts.AppHost,
			IncludeProvisionerDaemon: true,
			RealIPConfig: &httpmw.RealIPConfig{
				TrustedOrigins: []*net.IPNet{{
					IP:   net.ParseIP("127.0.0.1"),
					Mask: net.CIDRMask(8, 32),
				}},
				TrustedHeaders: []string{
					"CF-Connecting-IP",
				},
			},
			WorkspaceAppsStatsCollectorOptions: opts.StatsCollectorOptions,
		})

		user := coderdtest.CreateFirstUser(t, client)

		return &apptest.Deployment{
			Options:        opts,
			SDKClient:      client,
			FirstUser:      user,
			PathAppBaseURL: client.URL,
			FlushStats:     flushStats,
		}
	})
}
