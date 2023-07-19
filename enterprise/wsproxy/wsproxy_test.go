package wsproxy_test

import (
	"net"
	"testing"

	"github.com/coder/coder/cli/clibase"
	"github.com/coder/coder/coderd"
	"github.com/coder/coder/coderd/coderdtest"
	"github.com/coder/coder/coderd/httpmw"
	"github.com/coder/coder/coderd/workspaceapps/apptest"
	"github.com/coder/coder/codersdk"
	"github.com/coder/coder/enterprise/coderd/coderdenttest"
	"github.com/coder/coder/enterprise/coderd/license"
)

func TestWorkspaceProxyWorkspaceApps_Wsconncache(t *testing.T) {
	t.Parallel()

	apptest.Run(t, false, func(t *testing.T, opts *apptest.DeploymentOptions) *apptest.Deployment {
		deploymentValues := coderdtest.DeploymentValues(t)
		deploymentValues.DisablePathApps = clibase.Bool(opts.DisablePathApps)
		deploymentValues.Dangerous.AllowPathAppSharing = clibase.Bool(opts.DangerousAllowPathAppSharing)
		deploymentValues.Dangerous.AllowPathAppSiteOwnerAccess = clibase.Bool(opts.DangerousAllowPathAppSiteOwnerAccess)
		deploymentValues.Experiments = []string{
			string(codersdk.ExperimentMoons),
			"*",
		}

		client, _, api, user := coderdenttest.NewWithAPI(t, &coderdenttest.Options{
			Options: &coderdtest.Options{
				DeploymentValues:         deploymentValues,
				AppHostname:              "*.primary.test.coder.com",
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
			LicenseOptions: &coderdenttest.LicenseOptions{
				Features: license.Features{
					codersdk.FeatureWorkspaceProxy: 1,
				},
			},
		})

		// Create the external proxy
		if opts.DisableSubdomainApps {
			opts.AppHost = ""
		}
		proxyAPI := coderdenttest.NewWorkspaceProxy(t, api, client, &coderdenttest.ProxyOptions{
			Name:            "best-proxy",
			AppHostname:     opts.AppHost,
			DisablePathApps: opts.DisablePathApps,
		})

		return &apptest.Deployment{
			Options:        opts,
			SDKClient:      client,
			FirstUser:      user,
			PathAppBaseURL: proxyAPI.Options.AccessURL,
		}
	})
}

func TestWorkspaceProxyWorkspaceApps_SingleTailnet(t *testing.T) {
	t.Parallel()

	apptest.Run(t, false, func(t *testing.T, opts *apptest.DeploymentOptions) *apptest.Deployment {
		deploymentValues := coderdtest.DeploymentValues(t)
		deploymentValues.DisablePathApps = clibase.Bool(opts.DisablePathApps)
		deploymentValues.Dangerous.AllowPathAppSharing = clibase.Bool(opts.DangerousAllowPathAppSharing)
		deploymentValues.Dangerous.AllowPathAppSiteOwnerAccess = clibase.Bool(opts.DangerousAllowPathAppSiteOwnerAccess)
		deploymentValues.Experiments = []string{
			string(codersdk.ExperimentMoons),
			string(codersdk.ExperimentSingleTailnet),
			"*",
		}

		client, _, api, user := coderdenttest.NewWithAPI(t, &coderdenttest.Options{
			Options: &coderdtest.Options{
				DeploymentValues:         deploymentValues,
				AppHostname:              "*.primary.test.coder.com",
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
			LicenseOptions: &coderdenttest.LicenseOptions{
				Features: license.Features{
					codersdk.FeatureWorkspaceProxy: 1,
				},
			},
		})

		// Create the external proxy
		if opts.DisableSubdomainApps {
			opts.AppHost = ""
		}
		proxyAPI := coderdenttest.NewWorkspaceProxy(t, api, client, &coderdenttest.ProxyOptions{
			Name:            "best-proxy",
			Experiments:     coderd.ReadExperiments(api.Logger, deploymentValues.Experiments.Value()),
			AppHostname:     opts.AppHost,
			DisablePathApps: opts.DisablePathApps,
		})

		return &apptest.Deployment{
			Options:        opts,
			SDKClient:      client,
			FirstUser:      user,
			PathAppBaseURL: proxyAPI.Options.AccessURL,
		}
	})
}
