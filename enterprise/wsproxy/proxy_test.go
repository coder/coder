package wsproxy_test

import (
	"context"
	"net"
	"testing"

	"github.com/coder/coder/coderd/httpapi"
	"github.com/coder/coder/enterprise/wsproxy"

	"github.com/moby/moby/pkg/namesgenerator"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/enterprise/coderd/license"

	"github.com/coder/coder/codersdk"

	"github.com/coder/coder/enterprise/coderd/coderdenttest"

	"github.com/coder/coder/cli/clibase"
	"github.com/coder/coder/coderd/coderdtest"
	"github.com/coder/coder/coderd/httpmw"
	"github.com/coder/coder/coderd/workspaceapps/apptest"
)

func TestExternalProxyWorkspaceApps(t *testing.T) {
	t.Parallel()

	apptest.Run(t, func(t *testing.T, opts *apptest.DeploymentOptions) *apptest.Deployment {
		deploymentValues := coderdtest.DeploymentValues(t)
		deploymentValues.DisablePathApps = clibase.Bool(opts.DisablePathApps)
		deploymentValues.Dangerous.AllowPathAppSharing = clibase.Bool(opts.DangerousAllowPathAppSharing)
		deploymentValues.Dangerous.AllowPathAppSiteOwnerAccess = clibase.Bool(opts.DangerousAllowPathAppSiteOwnerAccess)
		deploymentValues.Experiments = []string{
			string(codersdk.ExperimentMoons),
			"*",
		}

		client, _, api := coderdenttest.NewWithAPI(t, &coderdenttest.Options{
			Options: &coderdtest.Options{
				DeploymentValues: deploymentValues,
				// TODO: @emyrk Should we give a hostname here too?
				AppHostname:              "",
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
			},
		})

		user := coderdtest.CreateFirstUser(t, client)
		_ = coderdenttest.AddLicense(t, client, coderdenttest.LicenseOptions{
			Features: license.Features{
				codersdk.FeatureWorkspaceProxy: 1,
			},
		})

		// Create the external proxy
		// TODO: @emyrk this code will probably change as we create a better
		// 		method of creating external proxies.
		ctx := context.Background()
		proxyRes, err := client.CreateWorkspaceProxy(ctx, codersdk.CreateWorkspaceProxyRequest{
			Name:             namesgenerator.GetRandomName(1),
			Icon:             "/emojis/flag.png",
			URL:              "https://" + namesgenerator.GetRandomName(1) + ".com",
			WildcardHostname: opts.AppHost,
		})
		require.NoError(t, err)

		appHostRegex, err := httpapi.CompileHostnamePattern(opts.AppHost)
		require.NoError(t, err, "app host regex should compile")

		// Make the external proxy service
		proxy, err := wsproxy.New(&wsproxy.Options{
			Logger:           api.Logger,
			PrimaryAccessURL: api.AccessURL,
			// TODO: @emyrk give this an access url
			AccessURL:          nil,
			AppHostname:        opts.AppHost,
			AppHostnameRegex:   appHostRegex,
			RealIPConfig:       api.RealIPConfig,
			AppSecurityKey:     api.AppSecurityKey,
			Tracing:            api.TracerProvider,
			PrometheusRegistry: api.PrometheusRegistry,
			APIRateLimit:       api.APIRateLimit,
			SecureAuthCookie:   api.SecureAuthCookie,
			ProxySessionToken:  proxyRes.ProxyToken,
		})
		require.NoError(t, err, "wsproxy should be created")

		// TODO: Run the wsproxy, http.Serve
		_ = proxy

		return &apptest.Deployment{
			Options:        opts,
			Client:         client,
			FirstUser:      user,
			PathAppBaseURL: client.URL,
		}
	})
}
