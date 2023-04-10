package coderd_test

import (
	"context"
	"net"
	"net/url"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/cli/clibase"
	"github.com/coder/coder/coderd/coderdtest"
	"github.com/coder/coder/coderd/httpmw"
	"github.com/coder/coder/coderd/workspaceapps/apptest"
	"github.com/coder/coder/testutil"
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

func TestWorkspaceApps(t *testing.T) {
	t.Parallel()

	apptest.Run(t, func(t *testing.T, opts *apptest.DeploymentOptions) *apptest.Deployment {
		deploymentValues := coderdtest.DeploymentValues(t)
		deploymentValues.DisablePathApps = clibase.Bool(opts.DisablePathApps)
		deploymentValues.Dangerous.AllowPathAppSharing = clibase.Bool(opts.DangerousAllowPathAppSharing)
		deploymentValues.Dangerous.AllowPathAppSiteOwnerAccess = clibase.Bool(opts.DangerousAllowPathAppSiteOwnerAccess)

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
		})

		user := coderdtest.CreateFirstUser(t, client)

		return &apptest.Deployment{
			Options:        opts,
			Client:         client,
			FirstUser:      user,
			PathAppBaseURL: client.URL,
		}
	})
}
