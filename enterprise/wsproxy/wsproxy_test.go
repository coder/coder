package wsproxy_test

import (
	"fmt"
	"net"
	"testing"

	"github.com/davecgh/go-spew/spew"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"tailscale.com/derp/derphttp"
	"tailscale.com/tailcfg"
	"tailscale.com/types/key"

	"cdr.dev/slog"
	"cdr.dev/slog/sloggers/slogtest"
	"github.com/coder/coder/v2/agent/agenttest"
	"github.com/coder/coder/v2/cli/clibase"
	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/coderd/healthcheck/derphealth"
	"github.com/coder/coder/v2/coderd/httpmw"
	"github.com/coder/coder/v2/coderd/workspaceapps/apptest"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/enterprise/coderd/coderdenttest"
	"github.com/coder/coder/v2/enterprise/coderd/license"
	"github.com/coder/coder/v2/provisioner/echo"
	"github.com/coder/coder/v2/testutil"
)

func TestDERPOnly(t *testing.T) {
	t.Parallel()

	deploymentValues := coderdtest.DeploymentValues(t)
	deploymentValues.Experiments = []string{
		"*",
	}

	client, closer, api, _ := coderdenttest.NewWithAPI(t, &coderdenttest.Options{
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
	t.Cleanup(func() {
		_ = closer.Close()
	})

	// Create an external proxy.
	_ = coderdenttest.NewWorkspaceProxy(t, api, client, &coderdenttest.ProxyOptions{
		Name:     "best-proxy",
		DerpOnly: true,
	})

	// Should not show up in the regions list.
	ctx := testutil.Context(t, testutil.WaitLong)
	regions, err := client.Regions(ctx)
	require.NoError(t, err)
	require.Len(t, regions, 1)
	require.Equal(t, api.Options.AccessURL.String(), regions[0].PathAppURL)
}

func TestDERP(t *testing.T) {
	t.Parallel()

	deploymentValues := coderdtest.DeploymentValues(t)
	deploymentValues.Experiments = []string{
		"*",
	}

	client, closer, api, user := coderdenttest.NewWithAPI(t, &coderdenttest.Options{
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
	t.Cleanup(func() {
		_ = closer.Close()
	})

	// Create two running external proxies.
	proxyAPI1 := coderdenttest.NewWorkspaceProxy(t, api, client, &coderdenttest.ProxyOptions{
		Name: "best-proxy",
	})
	proxyAPI2 := coderdenttest.NewWorkspaceProxy(t, api, client, &coderdenttest.ProxyOptions{
		Name: "worst-proxy",
	})

	// Create a running external proxy with DERP disabled.
	proxyAPI3 := coderdenttest.NewWorkspaceProxy(t, api, client, &coderdenttest.ProxyOptions{
		Name:         "no-derp-proxy",
		DerpDisabled: true,
	})

	// Create a proxy that is never started.
	createProxyCtx := testutil.Context(t, testutil.WaitLong)
	_, err := client.CreateWorkspaceProxy(createProxyCtx, codersdk.CreateWorkspaceProxyRequest{
		Name: "never-started-proxy",
	})
	require.NoError(t, err)

	// Wait for both running proxies to become healthy.
	require.Eventually(t, func() bool {
		healthCtx := testutil.Context(t, testutil.WaitLong)
		err := api.ProxyHealth.ForceUpdate(healthCtx)
		if !assert.NoError(t, err) {
			return false
		}

		regions, err := client.Regions(healthCtx)
		if !assert.NoError(t, err) {
			return false
		}
		if !assert.Len(t, regions, 5) {
			return false
		}

		// The first 3 regions should be healthy.
		for _, r := range regions[:4] {
			if !r.Healthy {
				return false
			}
		}

		// The last region should never be healthy.
		assert.False(t, regions[4].Healthy)
		return true
	}, testutil.WaitLong, testutil.IntervalMedium)

	// Create a workspace + apps
	authToken := uuid.NewString()
	version := coderdtest.CreateTemplateVersion(t, client, user.OrganizationID, &echo.Responses{
		Parse:          echo.ParseComplete,
		ProvisionApply: echo.ProvisionApplyWithAgent(authToken),
	})
	template := coderdtest.CreateTemplate(t, client, user.OrganizationID, version.ID)
	coderdtest.AwaitTemplateVersionJobCompleted(t, client, version.ID)
	workspace := coderdtest.CreateWorkspace(t, client, user.OrganizationID, template.ID)
	build := coderdtest.AwaitWorkspaceBuildJobCompleted(t, client, workspace.LatestBuild.ID)
	workspace.LatestBuild = build

	agentID := uuid.Nil
resourceLoop:
	for _, res := range build.Resources {
		for _, agnt := range res.Agents {
			agentID = agnt.ID
			break resourceLoop
		}
	}
	require.NotEqual(t, uuid.Nil, agentID)

	// Connect an agent to the workspace
	_ = agenttest.New(t, client.URL, authToken)
	_ = coderdtest.AwaitWorkspaceAgents(t, client, workspace.ID)

	t.Run("ReturnedInDERPMap", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)
		connInfo, err := client.WorkspaceAgentConnectionInfo(ctx, agentID)
		require.NoError(t, err)

		// There should be three DERP regions in the map: the primary, and each
		// of the two running proxies. Also the STUN-only regions.
		require.NotNil(t, connInfo.DERPMap)
		require.Len(t, connInfo.DERPMap.Regions, 3+len(api.DeploymentValues.DERP.Server.STUNAddresses.Value()))

		var (
			primaryRegion *tailcfg.DERPRegion
			proxy1Region  *tailcfg.DERPRegion
			proxy2Region  *tailcfg.DERPRegion
		)
		for _, r := range connInfo.DERPMap.Regions {
			if r.EmbeddedRelay {
				primaryRegion = r
				continue
			}
			if r.RegionName == "best-proxy" {
				proxy1Region = r
				continue
			}
			if r.RegionName == "worst-proxy" {
				proxy2Region = r
				continue
			}
			// The no-derp-proxy shouldn't show up in the map.
			// The last region is never started, which means it's never healthy,
			// which means it's never added to the DERP map.

			if len(r.Nodes) == 1 && r.Nodes[0].STUNOnly {
				// Skip STUN-only regions.
				continue
			}

			t.Fatalf("unexpected region: %+v", r)
		}

		// The primary region:
		require.Equal(t, "Coder Embedded Relay", primaryRegion.RegionName)
		require.Equal(t, "coder", primaryRegion.RegionCode)
		require.Equal(t, 999, primaryRegion.RegionID)
		require.True(t, primaryRegion.EmbeddedRelay)

		// The first proxy region:
		require.Equal(t, "best-proxy", proxy1Region.RegionName)
		require.Equal(t, "coder_best-proxy", proxy1Region.RegionCode)
		require.Equal(t, 10001, proxy1Region.RegionID)
		require.False(t, proxy1Region.EmbeddedRelay)
		require.Len(t, proxy1Region.Nodes, 1)
		require.Equal(t, "10001a", proxy1Region.Nodes[0].Name)
		require.Equal(t, 10001, proxy1Region.Nodes[0].RegionID)
		require.Equal(t, proxyAPI1.Options.AccessURL.Hostname(), proxy1Region.Nodes[0].HostName)
		require.Equal(t, proxyAPI1.Options.AccessURL.Port(), fmt.Sprint(proxy1Region.Nodes[0].DERPPort))
		require.Equal(t, proxyAPI1.Options.AccessURL.Scheme == "http", proxy1Region.Nodes[0].ForceHTTP)

		// The second proxy region:
		require.Equal(t, "worst-proxy", proxy2Region.RegionName)
		require.Equal(t, "coder_worst-proxy", proxy2Region.RegionCode)
		require.Equal(t, 10002, proxy2Region.RegionID)
		require.False(t, proxy2Region.EmbeddedRelay)
		require.Len(t, proxy2Region.Nodes, 1)
		require.Equal(t, "10002a", proxy2Region.Nodes[0].Name)
		require.Equal(t, 10002, proxy2Region.Nodes[0].RegionID)
		require.Equal(t, proxyAPI2.Options.AccessURL.Hostname(), proxy2Region.Nodes[0].HostName)
		require.Equal(t, proxyAPI2.Options.AccessURL.Port(), fmt.Sprint(proxy2Region.Nodes[0].DERPPort))
		require.Equal(t, proxyAPI2.Options.AccessURL.Scheme == "http", proxy2Region.Nodes[0].ForceHTTP)
	})

	t.Run("ConnectDERP", func(t *testing.T) {
		t.Parallel()

		connInfo, err := client.WorkspaceAgentConnectionInfo(testutil.Context(t, testutil.WaitLong), agentID)
		require.NoError(t, err)
		require.NotNil(t, connInfo.DERPMap)
		require.Len(t, connInfo.DERPMap.Regions, 3+len(api.DeploymentValues.DERP.Server.STUNAddresses.Value()))

		// Connect to each region.
		for _, r := range connInfo.DERPMap.Regions {
			r := r
			if len(r.Nodes) == 1 && r.Nodes[0].STUNOnly {
				// Skip STUN-only regions.
				continue
			}

			t.Run(r.RegionName, func(t *testing.T) {
				t.Parallel()

				derpMap := &tailcfg.DERPMap{
					Regions: map[int]*tailcfg.DERPRegion{
						r.RegionID: r,
					},
					OmitDefaultRegions: true,
				}

				ctx := testutil.Context(t, testutil.WaitLong)
				report := derphealth.Report{}
				report.Run(ctx, &derphealth.ReportOptions{
					DERPMap: derpMap,
				})

				t.Log("healthcheck report: " + spew.Sdump(&report))
				require.True(t, report.Healthy, "healthcheck failed, see report dump")
			})
		}
	})

	t.Run("DERPDisabled", func(t *testing.T) {
		t.Parallel()

		// Try to connect to the DERP server on the no-derp-proxy region.
		client, err := derphttp.NewClient(key.NewNode(), proxyAPI3.Options.AccessURL.String(), func(format string, args ...any) {})
		require.NoError(t, err)

		ctx := testutil.Context(t, testutil.WaitLong)
		err = client.Connect(ctx)
		require.Error(t, err)
	})
}

func TestDERPEndToEnd(t *testing.T) {
	t.Parallel()

	deploymentValues := coderdtest.DeploymentValues(t)
	deploymentValues.Experiments = []string{
		"*",
	}

	client, closer, api, user := coderdenttest.NewWithAPI(t, &coderdenttest.Options{
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
	t.Cleanup(func() {
		_ = closer.Close()
	})

	coderdenttest.NewWorkspaceProxy(t, api, client, &coderdenttest.ProxyOptions{
		Name: "best-proxy",
	})

	// Wait for the proxy to become healthy.
	require.Eventually(t, func() bool {
		healthCtx := testutil.Context(t, testutil.WaitLong)
		err := api.ProxyHealth.ForceUpdate(healthCtx)
		if !assert.NoError(t, err) {
			return false
		}

		regions, err := client.Regions(healthCtx)
		if !assert.NoError(t, err) {
			return false
		}
		if !assert.Len(t, regions, 2) {
			return false
		}
		for _, r := range regions {
			if !r.Healthy {
				return false
			}
		}
		return true
	}, testutil.WaitLong, testutil.IntervalMedium)

	// Swap out the DERPMapper for a fake one that only returns the proxy. This
	// allows us to force the agent to pick the proxy as its preferred region.
	oldDERPMapper := *api.AGPL.DERPMapper.Load()
	newDERPMapper := func(derpMap *tailcfg.DERPMap) *tailcfg.DERPMap {
		derpMap = oldDERPMapper(derpMap)
		// Strip everything but the proxy, which is region ID 10001.
		derpMap.Regions = map[int]*tailcfg.DERPRegion{
			10001: derpMap.Regions[10001],
		}
		derpMap.OmitDefaultRegions = true
		return derpMap
	}
	api.AGPL.DERPMapper.Store(&newDERPMapper)

	// Create a workspace + apps
	authToken := uuid.NewString()
	version := coderdtest.CreateTemplateVersion(t, client, user.OrganizationID, &echo.Responses{
		Parse:          echo.ParseComplete,
		ProvisionApply: echo.ProvisionApplyWithAgent(authToken),
	})
	template := coderdtest.CreateTemplate(t, client, user.OrganizationID, version.ID)
	coderdtest.AwaitTemplateVersionJobCompleted(t, client, version.ID)
	workspace := coderdtest.CreateWorkspace(t, client, user.OrganizationID, template.ID)
	build := coderdtest.AwaitWorkspaceBuildJobCompleted(t, client, workspace.LatestBuild.ID)
	workspace.LatestBuild = build

	agentID := uuid.Nil
resourceLoop:
	for _, res := range build.Resources {
		for _, agnt := range res.Agents {
			agentID = agnt.ID
			break resourceLoop
		}
	}
	require.NotEqual(t, uuid.Nil, agentID)

	// Connect an agent to the workspace
	_ = agenttest.New(t, client.URL, authToken)
	_ = coderdtest.AwaitWorkspaceAgents(t, client, workspace.ID)

	// Connect to the workspace agent.
	ctx := testutil.Context(t, testutil.WaitLong)
	conn, err := client.DialWorkspaceAgent(ctx, agentID, &codersdk.DialWorkspaceAgentOptions{
		Logger: slogtest.Make(t, &slogtest.Options{
			IgnoreErrors: true,
		}).Named("client").Leveled(slog.LevelDebug),
		// Force DERP.
		BlockEndpoints: true,
	})
	require.NoError(t, err)
	t.Cleanup(func() {
		err := conn.Close()
		assert.NoError(t, err)
	})

	ok := conn.AwaitReachable(ctx)
	require.True(t, ok)

	_, p2p, _, err := conn.Ping(ctx)
	require.NoError(t, err)
	require.False(t, p2p)
}

func TestWorkspaceProxyWorkspaceApps(t *testing.T) {
	t.Parallel()

	apptest.Run(t, false, func(t *testing.T, opts *apptest.DeploymentOptions) *apptest.Deployment {
		deploymentValues := coderdtest.DeploymentValues(t)
		deploymentValues.DisablePathApps = clibase.Bool(opts.DisablePathApps)
		deploymentValues.Dangerous.AllowPathAppSharing = clibase.Bool(opts.DangerousAllowPathAppSharing)
		deploymentValues.Dangerous.AllowPathAppSiteOwnerAccess = clibase.Bool(opts.DangerousAllowPathAppSiteOwnerAccess)
		deploymentValues.Experiments = []string{
			"*",
		}

		proxyStatsCollectorFlushCh := make(chan chan<- struct{}, 1)
		flushStats := func() {
			proxyStatsCollectorFlushDone := make(chan struct{}, 1)
			proxyStatsCollectorFlushCh <- proxyStatsCollectorFlushDone
			<-proxyStatsCollectorFlushDone
		}

		if opts.PrimaryAppHost == "" {
			opts.PrimaryAppHost = "*.primary.test.coder.com"
		}
		client, closer, api, user := coderdenttest.NewWithAPI(t, &coderdenttest.Options{
			Options: &coderdtest.Options{
				DeploymentValues:         deploymentValues,
				AppHostname:              opts.PrimaryAppHost,
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
			},
			LicenseOptions: &coderdenttest.LicenseOptions{
				Features: license.Features{
					codersdk.FeatureWorkspaceProxy: 1,
				},
			},
		})
		t.Cleanup(func() {
			_ = closer.Close()
		})

		// Create the external proxy
		if opts.DisableSubdomainApps {
			opts.AppHost = ""
		}
		proxyAPI := coderdenttest.NewWorkspaceProxy(t, api, client, &coderdenttest.ProxyOptions{
			Name:            "best-proxy",
			AppHostname:     opts.AppHost,
			DisablePathApps: opts.DisablePathApps,
			FlushStats:      proxyStatsCollectorFlushCh,
		})

		return &apptest.Deployment{
			Options:        opts,
			SDKClient:      client,
			FirstUser:      user,
			PathAppBaseURL: proxyAPI.Options.AccessURL,
			FlushStats:     flushStats,
		}
	})
}
