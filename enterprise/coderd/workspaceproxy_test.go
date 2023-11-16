package coderd_test

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/http/httputil"
	"net/url"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/moby/moby/pkg/namesgenerator"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/agent/agenttest"
	"github.com/coder/coder/v2/buildinfo"
	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbtestutil"
	"github.com/coder/coder/v2/coderd/workspaceapps"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/enterprise/coderd/coderdenttest"
	"github.com/coder/coder/v2/enterprise/coderd/license"
	"github.com/coder/coder/v2/enterprise/wsproxy/wsproxysdk"
	"github.com/coder/coder/v2/provisioner/echo"
	"github.com/coder/coder/v2/testutil"
)

func TestRegions(t *testing.T) {
	t.Parallel()

	const appHostname = "*.apps.coder.test"

	t.Run("OK", func(t *testing.T) {
		t.Parallel()

		dv := coderdtest.DeploymentValues(t)
		dv.Experiments = []string{
			string(codersdk.ExperimentMoons),
			"*",
		}

		db, pubsub := dbtestutil.NewDB(t)

		client, _ := coderdenttest.New(t, &coderdenttest.Options{
			Options: &coderdtest.Options{
				AppHostname:      appHostname,
				Database:         db,
				Pubsub:           pubsub,
				DeploymentValues: dv,
			},
		})

		ctx := testutil.Context(t, testutil.WaitLong)
		deploymentID, err := db.GetDeploymentID(ctx)
		require.NoError(t, err, "get deployment ID")

		regions, err := client.Regions(ctx)
		require.NoError(t, err)

		require.Len(t, regions, 1)
		require.NotEqual(t, uuid.Nil, regions[0].ID)
		require.Equal(t, regions[0].ID.String(), deploymentID)
		require.Equal(t, "primary", regions[0].Name)
		require.Equal(t, "Default", regions[0].DisplayName)
		require.NotEmpty(t, regions[0].IconURL)
		require.True(t, regions[0].Healthy)
		require.Equal(t, client.URL.String(), regions[0].PathAppURL)
		require.Equal(t, appHostname, regions[0].WildcardHostname)

		// Ensure the primary region ID is constant.
		regions2, err := client.Regions(ctx)
		require.NoError(t, err)
		require.Equal(t, regions[0].ID, regions2[0].ID)
	})

	t.Run("WithProxies", func(t *testing.T) {
		t.Parallel()

		dv := coderdtest.DeploymentValues(t)
		dv.Experiments = []string{
			string(codersdk.ExperimentMoons),
			"*",
		}

		db, pubsub := dbtestutil.NewDB(t)

		client, closer, api, _ := coderdenttest.NewWithAPI(t, &coderdenttest.Options{
			Options: &coderdtest.Options{
				AppHostname:      appHostname,
				Database:         db,
				Pubsub:           pubsub,
				DeploymentValues: dv,
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
		ctx := testutil.Context(t, testutil.WaitLong)
		deploymentID, err := db.GetDeploymentID(ctx)
		require.NoError(t, err, "get deployment ID")

		const proxyName = "hello"
		_ = coderdenttest.NewWorkspaceProxy(t, api, client, &coderdenttest.ProxyOptions{
			Name:        proxyName,
			AppHostname: appHostname + ".proxy",
		})
		proxy, err := db.GetWorkspaceProxyByName(ctx, proxyName)
		require.NoError(t, err)

		// Wait for the proxy to become healthy.
		require.Eventually(t, func() bool {
			healthCtx := testutil.Context(t, testutil.WaitLong)
			err := api.ProxyHealth.ForceUpdate(healthCtx)
			if !assert.NoError(t, err) {
				return false
			}

			wps, err := client.WorkspaceProxies(ctx)
			if !assert.NoError(t, err) {
				return false
			}
			if !assert.Len(t, wps.Regions, 2) {
				return false
			}
			for _, wp := range wps.Regions {
				if !wp.Healthy {
					t.Logf("region %q is not healthy yet, retrying healthcheck", wp.Name)
					for _, errMsg := range wp.Status.Report.Errors {
						t.Logf(" - error: %s", errMsg)
					}
					for _, warnMsg := range wp.Status.Report.Warnings {
						t.Logf(" - warning: %s", warnMsg)
					}
					return false
				}
			}
			return true
		}, testutil.WaitLong, testutil.IntervalMedium)

		regions, err := client.Regions(ctx)
		require.NoError(t, err)
		require.Len(t, regions, 2)

		// Region 0 is the primary	require.Len(t, regions, 1)
		require.NotEqual(t, uuid.Nil, regions[0].ID)
		require.Equal(t, regions[0].ID.String(), deploymentID)
		require.Equal(t, "primary", regions[0].Name)
		require.Equal(t, "Default", regions[0].DisplayName)
		require.NotEmpty(t, regions[0].IconURL)
		require.True(t, regions[0].Healthy)
		require.Equal(t, client.URL.String(), regions[0].PathAppURL)
		require.Equal(t, appHostname, regions[0].WildcardHostname)

		// Region 1 is the proxy.
		require.NotEqual(t, uuid.Nil, regions[1].ID)
		require.Equal(t, proxy.ID, regions[1].ID)
		require.Equal(t, proxy.Name, regions[1].Name)
		require.Equal(t, proxy.DisplayName, regions[1].DisplayName)
		require.Equal(t, proxy.Icon, regions[1].IconURL)
		require.True(t, regions[1].Healthy)
		require.Equal(t, proxy.Url, regions[1].PathAppURL)
		require.Equal(t, proxy.WildcardHostname, regions[1].WildcardHostname)
	})

	t.Run("RequireAuth", func(t *testing.T) {
		t.Parallel()

		dv := coderdtest.DeploymentValues(t)
		dv.Experiments = []string{
			string(codersdk.ExperimentMoons),
			"*",
		}

		ctx := testutil.Context(t, testutil.WaitLong)
		client, _ := coderdenttest.New(t, &coderdenttest.Options{
			Options: &coderdtest.Options{
				AppHostname:      appHostname,
				DeploymentValues: dv,
			},
		})

		unauthedClient := codersdk.New(client.URL)
		regions, err := unauthedClient.Regions(ctx)
		require.Error(t, err)
		require.Empty(t, regions)
	})
}

func TestWorkspaceProxyCRUD(t *testing.T) {
	t.Parallel()

	t.Run("CreateAndUpdate", func(t *testing.T) {
		t.Parallel()

		dv := coderdtest.DeploymentValues(t)
		dv.Experiments = []string{
			string(codersdk.ExperimentMoons),
			"*",
		}
		client, _ := coderdenttest.New(t, &coderdenttest.Options{
			Options: &coderdtest.Options{
				DeploymentValues: dv,
			},
			LicenseOptions: &coderdenttest.LicenseOptions{
				Features: license.Features{
					codersdk.FeatureWorkspaceProxy: 1,
				},
			},
		})
		ctx := testutil.Context(t, testutil.WaitLong)
		proxyRes, err := client.CreateWorkspaceProxy(ctx, codersdk.CreateWorkspaceProxyRequest{
			Name: namesgenerator.GetRandomName(1),
			Icon: "/emojis/flag.png",
		})
		require.NoError(t, err)

		found, err := client.WorkspaceProxyByID(ctx, proxyRes.Proxy.ID)
		require.NoError(t, err)
		// This will be different, so set it to the same
		found.Status = proxyRes.Proxy.Status
		require.Equal(t, proxyRes.Proxy, found, "expected proxy")
		require.NotEmpty(t, proxyRes.ProxyToken)

		// Update the proxy
		expName := namesgenerator.GetRandomName(1)
		expDisplayName := namesgenerator.GetRandomName(1)
		expIcon := namesgenerator.GetRandomName(1)
		_, err = client.PatchWorkspaceProxy(ctx, codersdk.PatchWorkspaceProxy{
			ID:          proxyRes.Proxy.ID,
			Name:        expName,
			DisplayName: expDisplayName,
			Icon:        expIcon,
		})
		require.NoError(t, err, "expected no error updating proxy")

		found, err = client.WorkspaceProxyByID(ctx, proxyRes.Proxy.ID)
		require.NoError(t, err)
		require.Equal(t, expName, found.Name, "name")
		require.Equal(t, expDisplayName, found.DisplayName, "display name")
		require.Equal(t, expIcon, found.IconURL, "icon")
	})

	t.Run("Delete", func(t *testing.T) {
		t.Parallel()

		dv := coderdtest.DeploymentValues(t)
		dv.Experiments = []string{
			string(codersdk.ExperimentMoons),
			"*",
		}
		client, _ := coderdenttest.New(t, &coderdenttest.Options{
			Options: &coderdtest.Options{
				DeploymentValues: dv,
			},
			LicenseOptions: &coderdenttest.LicenseOptions{
				Features: license.Features{
					codersdk.FeatureWorkspaceProxy: 1,
				},
			},
		})
		ctx := testutil.Context(t, testutil.WaitLong)
		proxyRes, err := client.CreateWorkspaceProxy(ctx, codersdk.CreateWorkspaceProxyRequest{
			Name: namesgenerator.GetRandomName(1),
			Icon: "/emojis/flag.png",
		})
		require.NoError(t, err)

		err = client.DeleteWorkspaceProxyByID(ctx, proxyRes.Proxy.ID)
		require.NoError(t, err, "failed to delete workspace proxy")

		proxies, err := client.WorkspaceProxies(ctx)
		require.NoError(t, err)
		// Default proxy is always there
		require.Len(t, proxies.Regions, 1)
	})
}

func TestProxyRegisterDeregister(t *testing.T) {
	t.Parallel()

	setup := func(t *testing.T) (*codersdk.Client, database.Store) {
		dv := coderdtest.DeploymentValues(t)
		dv.Experiments = []string{
			string(codersdk.ExperimentMoons),
			"*",
		}

		db, pubsub := dbtestutil.NewDB(t)
		client, _ := coderdenttest.New(t, &coderdenttest.Options{
			Options: &coderdtest.Options{
				DeploymentValues:         dv,
				Database:                 db,
				Pubsub:                   pubsub,
				IncludeProvisionerDaemon: true,
			},
			ReplicaSyncUpdateInterval: time.Minute,
			LicenseOptions: &coderdenttest.LicenseOptions{
				Features: license.Features{
					codersdk.FeatureWorkspaceProxy: 1,
				},
			},
		})

		return client, db
	}

	t.Run("OK", func(t *testing.T) {
		t.Parallel()

		client, db := setup(t)

		ctx := testutil.Context(t, testutil.WaitLong)
		const (
			proxyName        = "hello"
			proxyDisplayName = "Hello World"
			proxyIcon        = "/emojis/flag.png"
		)
		createRes, err := client.CreateWorkspaceProxy(ctx, codersdk.CreateWorkspaceProxyRequest{
			Name:        proxyName,
			DisplayName: proxyDisplayName,
			Icon:        proxyIcon,
		})
		require.NoError(t, err)

		proxyClient := wsproxysdk.New(client.URL)
		proxyClient.SetSessionToken(createRes.ProxyToken)

		// Register
		req := wsproxysdk.RegisterWorkspaceProxyRequest{
			AccessURL:           "https://proxy.coder.test",
			WildcardHostname:    "*.proxy.coder.test",
			DerpEnabled:         true,
			ReplicaID:           uuid.New(),
			ReplicaHostname:     "mars",
			ReplicaError:        "",
			ReplicaRelayAddress: "http://127.0.0.1:8080",
			Version:             buildinfo.Version(),
		}
		registerRes1, err := proxyClient.RegisterWorkspaceProxy(ctx, req)
		require.NoError(t, err)
		require.NotEmpty(t, registerRes1.AppSecurityKey)
		require.NotEmpty(t, registerRes1.DERPMeshKey)
		require.EqualValues(t, 10001, registerRes1.DERPRegionID)
		require.Empty(t, registerRes1.SiblingReplicas)

		proxy, err := client.WorkspaceProxyByID(ctx, createRes.Proxy.ID)
		require.NoError(t, err)
		require.Equal(t, createRes.Proxy.ID, proxy.ID)
		require.Equal(t, proxyName, proxy.Name)
		require.Equal(t, proxyDisplayName, proxy.DisplayName)
		require.Equal(t, proxyIcon, proxy.IconURL)
		require.Equal(t, req.AccessURL, proxy.PathAppURL)
		require.Equal(t, req.AccessURL, proxy.PathAppURL)
		require.Equal(t, req.WildcardHostname, proxy.WildcardHostname)
		require.Equal(t, req.DerpEnabled, proxy.DerpEnabled)
		require.False(t, proxy.Deleted)

		// Get the replica from the DB.
		replica, err := db.GetReplicaByID(ctx, req.ReplicaID)
		require.NoError(t, err)
		require.Equal(t, req.ReplicaID, replica.ID)
		require.Equal(t, req.ReplicaHostname, replica.Hostname)
		require.Equal(t, req.ReplicaError, replica.Error)
		require.Equal(t, req.ReplicaRelayAddress, replica.RelayAddress)
		require.Equal(t, req.Version, replica.Version)
		require.EqualValues(t, 10001, replica.RegionID)
		require.False(t, replica.StoppedAt.Valid)
		require.Zero(t, replica.DatabaseLatency)
		require.False(t, replica.Primary)

		// Re-register with most fields changed.
		req = wsproxysdk.RegisterWorkspaceProxyRequest{
			AccessURL:           "https://cool.proxy.coder.test",
			WildcardHostname:    "*.cool.proxy.coder.test",
			DerpEnabled:         false,
			ReplicaID:           req.ReplicaID,
			ReplicaHostname:     "venus",
			ReplicaError:        "error",
			ReplicaRelayAddress: "http://127.0.0.1:9090",
			Version:             buildinfo.Version(),
		}
		registerRes2, err := proxyClient.RegisterWorkspaceProxy(ctx, req)
		require.NoError(t, err)
		require.Equal(t, registerRes1, registerRes2)

		// Get the proxy to ensure nothing has changed except updated_at.
		proxyNew, err := client.WorkspaceProxyByID(ctx, createRes.Proxy.ID)
		require.NoError(t, err)
		require.Equal(t, createRes.Proxy.ID, proxyNew.ID)
		require.Equal(t, proxyName, proxyNew.Name)
		require.Equal(t, proxyDisplayName, proxyNew.DisplayName)
		require.Equal(t, proxyIcon, proxyNew.IconURL)
		require.Equal(t, req.AccessURL, proxyNew.PathAppURL)
		require.Equal(t, req.AccessURL, proxyNew.PathAppURL)
		require.Equal(t, req.WildcardHostname, proxyNew.WildcardHostname)
		require.Equal(t, req.DerpEnabled, proxyNew.DerpEnabled)
		require.False(t, proxyNew.Deleted)

		// Get the replica from the DB and ensure the fields have been updated,
		// especially the updated_at.
		replica, err = db.GetReplicaByID(ctx, req.ReplicaID)
		require.NoError(t, err)
		require.Equal(t, req.ReplicaID, replica.ID)
		require.Equal(t, req.ReplicaHostname, replica.Hostname)
		require.Equal(t, req.ReplicaError, replica.Error)
		require.Equal(t, req.ReplicaRelayAddress, replica.RelayAddress)
		require.Equal(t, req.Version, replica.Version)
		require.EqualValues(t, 10001, replica.RegionID)
		require.False(t, replica.StoppedAt.Valid)
		require.Zero(t, replica.DatabaseLatency)
		require.False(t, replica.Primary)

		// Deregister
		err = proxyClient.DeregisterWorkspaceProxy(ctx, wsproxysdk.DeregisterWorkspaceProxyRequest{
			ReplicaID: req.ReplicaID,
		})
		require.NoError(t, err)

		// Ensure the replica has been fully stopped.
		replica, err = db.GetReplicaByID(ctx, req.ReplicaID)
		require.NoError(t, err)
		require.Equal(t, req.ReplicaID, replica.ID)
		require.True(t, replica.StoppedAt.Valid)

		// Re-register should fail
		_, err = proxyClient.RegisterWorkspaceProxy(ctx, wsproxysdk.RegisterWorkspaceProxyRequest{})
		require.Error(t, err)
	})

	t.Run("ReregisterUpdateReplica", func(t *testing.T) {
		t.Parallel()

		client, db := setup(t)

		ctx := testutil.Context(t, testutil.WaitLong)
		createRes, err := client.CreateWorkspaceProxy(ctx, codersdk.CreateWorkspaceProxyRequest{
			Name: "hi",
		})
		require.NoError(t, err)

		proxyClient := wsproxysdk.New(client.URL)
		proxyClient.SetSessionToken(createRes.ProxyToken)

		req := wsproxysdk.RegisterWorkspaceProxyRequest{
			AccessURL:           "https://proxy.coder.test",
			WildcardHostname:    "*.proxy.coder.test",
			DerpEnabled:         true,
			ReplicaID:           uuid.New(),
			ReplicaHostname:     "mars",
			ReplicaError:        "",
			ReplicaRelayAddress: "http://127.0.0.1:8080",
			Version:             buildinfo.Version(),
		}
		_, err = proxyClient.RegisterWorkspaceProxy(ctx, req)
		require.NoError(t, err)

		// Get the replica from the DB.
		replica, err := db.GetReplicaByID(ctx, req.ReplicaID)
		require.NoError(t, err)
		require.Equal(t, req.ReplicaID, replica.ID)

		time.Sleep(time.Millisecond)

		// Re-register with no changed fields.
		_, err = proxyClient.RegisterWorkspaceProxy(ctx, req)
		require.NoError(t, err)

		// Get the replica from the DB and make sure updated_at has changed.
		replica, err = db.GetReplicaByID(ctx, req.ReplicaID)
		require.NoError(t, err)
		require.Equal(t, req.ReplicaID, replica.ID)
		require.Greater(t, replica.UpdatedAt.UnixNano(), replica.CreatedAt.UnixNano())
	})

	t.Run("DeregisterNonExistentReplica", func(t *testing.T) {
		t.Parallel()

		client, _ := setup(t)

		ctx := testutil.Context(t, testutil.WaitLong)
		createRes, err := client.CreateWorkspaceProxy(ctx, codersdk.CreateWorkspaceProxyRequest{
			Name: "hi",
		})
		require.NoError(t, err)

		proxyClient := wsproxysdk.New(client.URL)
		proxyClient.SetSessionToken(createRes.ProxyToken)

		err = proxyClient.DeregisterWorkspaceProxy(ctx, wsproxysdk.DeregisterWorkspaceProxyRequest{
			ReplicaID: uuid.New(),
		})
		require.Error(t, err)
		var sdkErr *codersdk.Error
		require.ErrorAs(t, err, &sdkErr)
		require.Equal(t, http.StatusNotFound, sdkErr.StatusCode())
	})

	t.Run("ReturnSiblings", func(t *testing.T) {
		t.Parallel()

		client, _ := setup(t)

		ctx := testutil.Context(t, testutil.WaitLong)
		createRes1, err := client.CreateWorkspaceProxy(ctx, codersdk.CreateWorkspaceProxyRequest{
			Name: "one",
		})
		require.NoError(t, err)
		createRes2, err := client.CreateWorkspaceProxy(ctx, codersdk.CreateWorkspaceProxyRequest{
			Name: "two",
		})
		require.NoError(t, err)

		// Register a replica on proxy 2. This shouldn't be returned by replicas
		// for proxy 1.
		proxyClient2 := wsproxysdk.New(client.URL)
		proxyClient2.SetSessionToken(createRes2.ProxyToken)
		_, err = proxyClient2.RegisterWorkspaceProxy(ctx, wsproxysdk.RegisterWorkspaceProxyRequest{
			AccessURL:           "https://other.proxy.coder.test",
			WildcardHostname:    "*.other.proxy.coder.test",
			DerpEnabled:         true,
			ReplicaID:           uuid.New(),
			ReplicaHostname:     "venus",
			ReplicaError:        "",
			ReplicaRelayAddress: "http://127.0.0.1:9090",
			Version:             buildinfo.Version(),
		})
		require.NoError(t, err)

		// Register replica 1.
		proxyClient1 := wsproxysdk.New(client.URL)
		proxyClient1.SetSessionToken(createRes1.ProxyToken)
		req1 := wsproxysdk.RegisterWorkspaceProxyRequest{
			AccessURL:           "https://one.proxy.coder.test",
			WildcardHostname:    "*.one.proxy.coder.test",
			DerpEnabled:         true,
			ReplicaID:           uuid.New(),
			ReplicaHostname:     "mars1",
			ReplicaError:        "",
			ReplicaRelayAddress: "http://127.0.0.1:8081",
			Version:             buildinfo.Version(),
		}
		registerRes1, err := proxyClient1.RegisterWorkspaceProxy(ctx, req1)
		require.NoError(t, err)
		require.Empty(t, registerRes1.SiblingReplicas)

		// Register replica 2 and expect to get replica 1 as a sibling.
		req2 := wsproxysdk.RegisterWorkspaceProxyRequest{
			AccessURL:           "https://two.proxy.coder.test",
			WildcardHostname:    "*.two.proxy.coder.test",
			DerpEnabled:         true,
			ReplicaID:           uuid.New(),
			ReplicaHostname:     "mars2",
			ReplicaError:        "",
			ReplicaRelayAddress: "http://127.0.0.1:8082",
			Version:             buildinfo.Version(),
		}
		registerRes2, err := proxyClient1.RegisterWorkspaceProxy(ctx, req2)
		require.NoError(t, err)
		require.Len(t, registerRes2.SiblingReplicas, 1)
		require.Equal(t, req1.ReplicaID, registerRes2.SiblingReplicas[0].ID)
		require.Equal(t, req1.ReplicaHostname, registerRes2.SiblingReplicas[0].Hostname)
		require.Equal(t, req1.ReplicaRelayAddress, registerRes2.SiblingReplicas[0].RelayAddress)
		require.EqualValues(t, 10001, registerRes2.SiblingReplicas[0].RegionID)

		// Re-register replica 1 and expect to get replica 2 as a sibling.
		registerRes1, err = proxyClient1.RegisterWorkspaceProxy(ctx, req1)
		require.NoError(t, err)
		require.Len(t, registerRes1.SiblingReplicas, 1)
		require.Equal(t, req2.ReplicaID, registerRes1.SiblingReplicas[0].ID)
		require.Equal(t, req2.ReplicaHostname, registerRes1.SiblingReplicas[0].Hostname)
		require.Equal(t, req2.ReplicaRelayAddress, registerRes1.SiblingReplicas[0].RelayAddress)
		require.EqualValues(t, 10001, registerRes1.SiblingReplicas[0].RegionID)
	})

	// ReturnSiblings2 tries to create 100 proxy replicas and ensures that they
	// all return the correct number of siblings.
	t.Run("ReturnSiblings2", func(t *testing.T) {
		t.Parallel()

		client, _ := setup(t)
		ctx := testutil.Context(t, testutil.WaitLong)

		createRes, err := client.CreateWorkspaceProxy(ctx, codersdk.CreateWorkspaceProxyRequest{
			Name: "proxy",
		})
		require.NoError(t, err)

		proxyClient := wsproxysdk.New(client.URL)
		proxyClient.SetSessionToken(createRes.ProxyToken)

		for i := 0; i < 100; i++ {
			ok := false
			for j := 0; j < 2; j++ {
				registerRes, err := proxyClient.RegisterWorkspaceProxy(ctx, wsproxysdk.RegisterWorkspaceProxyRequest{
					AccessURL:           "https://proxy.coder.test",
					WildcardHostname:    "*.proxy.coder.test",
					DerpEnabled:         true,
					ReplicaID:           uuid.New(),
					ReplicaHostname:     "venus",
					ReplicaError:        "",
					ReplicaRelayAddress: fmt.Sprintf("http://127.0.0.1:%d", 8080+i),
					Version:             buildinfo.Version(),
				})
				require.NoErrorf(t, err, "register proxy %d", i)

				// If the sibling replica count is wrong, try again. The impact
				// of this not being immediate is that proxies may not function
				// as DERP relays until they register again in 30 seconds.
				//
				// In the real world, replicas will not be registering this
				// quickly. Kubernetes rolls out gradually in practice.
				if len(registerRes.SiblingReplicas) != i {
					t.Logf("%d: expected %d siblings, got %d", i, i, len(registerRes.SiblingReplicas))
					time.Sleep(100 * time.Millisecond)
					continue
				}

				ok = true
				break
			}

			require.True(t, ok, "expected to register replica %d", i)
		}
	})
}

func TestIssueSignedAppToken(t *testing.T) {
	t.Parallel()

	dv := coderdtest.DeploymentValues(t)
	dv.Experiments = []string{
		string(codersdk.ExperimentMoons),
		"*",
	}

	db, pubsub := dbtestutil.NewDB(t)
	client, user := coderdenttest.New(t, &coderdenttest.Options{
		Options: &coderdtest.Options{
			DeploymentValues:         dv,
			Database:                 db,
			Pubsub:                   pubsub,
			IncludeProvisionerDaemon: true,
		},
		LicenseOptions: &coderdenttest.LicenseOptions{
			Features: license.Features{
				codersdk.FeatureWorkspaceProxy: 1,
			},
		},
	})

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

	// Connect an agent to the workspace
	_ = agenttest.New(t, client.URL, authToken)
	_ = coderdtest.AwaitWorkspaceAgents(t, client, workspace.ID)

	createProxyCtx := testutil.Context(t, testutil.WaitLong)
	proxyRes, err := client.CreateWorkspaceProxy(createProxyCtx, codersdk.CreateWorkspaceProxyRequest{
		Name: namesgenerator.GetRandomName(1),
		Icon: "/emojis/flag.png",
	})
	require.NoError(t, err)

	t.Run("BadAppRequest", func(t *testing.T) {
		t.Parallel()
		proxyClient := wsproxysdk.New(client.URL)
		proxyClient.SetSessionToken(proxyRes.ProxyToken)

		ctx := testutil.Context(t, testutil.WaitLong)
		_, err := proxyClient.IssueSignedAppToken(ctx, workspaceapps.IssueTokenRequest{
			// Invalid request.
			AppRequest:   workspaceapps.Request{},
			SessionToken: client.SessionToken(),
		})
		require.Error(t, err)
	})

	goodRequest := workspaceapps.IssueTokenRequest{
		AppRequest: workspaceapps.Request{
			BasePath:      "/app",
			AccessMethod:  workspaceapps.AccessMethodTerminal,
			AgentNameOrID: build.Resources[0].Agents[0].ID.String(),
		},
		SessionToken: client.SessionToken(),
	}
	t.Run("OK", func(t *testing.T) {
		t.Parallel()
		proxyClient := wsproxysdk.New(client.URL)
		proxyClient.SetSessionToken(proxyRes.ProxyToken)

		ctx := testutil.Context(t, testutil.WaitLong)
		_, err := proxyClient.IssueSignedAppToken(ctx, goodRequest)
		require.NoError(t, err)
	})

	t.Run("OKHTML", func(t *testing.T) {
		t.Parallel()
		proxyClient := wsproxysdk.New(client.URL)
		proxyClient.SetSessionToken(proxyRes.ProxyToken)

		rw := httptest.NewRecorder()
		ctx := testutil.Context(t, testutil.WaitLong)
		_, ok := proxyClient.IssueSignedAppTokenHTML(ctx, rw, goodRequest)
		if !assert.True(t, ok, "expected true") {
			resp := rw.Result()
			defer resp.Body.Close()
			dump, err := httputil.DumpResponse(resp, true)
			require.NoError(t, err)
			t.Log(string(dump))
		}
	})
}

func TestReconnectingPTYSignedToken(t *testing.T) {
	t.Parallel()

	dv := coderdtest.DeploymentValues(t)
	dv.Experiments = []string{
		string(codersdk.ExperimentMoons),
		"*",
	}

	db, pubsub := dbtestutil.NewDB(t)
	client, closer, api, user := coderdenttest.NewWithAPI(t, &coderdenttest.Options{
		Options: &coderdtest.Options{
			DeploymentValues:         dv,
			Database:                 db,
			Pubsub:                   pubsub,
			IncludeProvisionerDaemon: true,
		},
		LicenseOptions: &coderdenttest.LicenseOptions{
			Features: license.Features{
				codersdk.FeatureWorkspaceProxy: 1,
			},
		},
	})
	t.Cleanup(func() {
		closer.Close()
	})

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

	// Connect an agent to the workspace
	agentID := build.Resources[0].Agents[0].ID
	_ = agenttest.New(t, client.URL, authToken)
	_ = coderdtest.AwaitWorkspaceAgents(t, client, workspace.ID)

	proxyURL, err := url.Parse(fmt.Sprintf("https://%s.com", namesgenerator.GetRandomName(1)))
	require.NoError(t, err)

	_ = coderdenttest.NewWorkspaceProxy(t, api, client, &coderdenttest.ProxyOptions{
		Name:        namesgenerator.GetRandomName(1),
		ProxyURL:    proxyURL,
		AppHostname: "*.sub.example.com",
	})

	u, err := url.Parse(proxyURL.String())
	require.NoError(t, err)
	if u.Scheme == "https" {
		u.Scheme = "wss"
	} else {
		u.Scheme = "ws"
	}
	u.Path = fmt.Sprintf("/api/v2/workspaceagents/%s/pty", agentID.String())

	t.Run("Validate", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)
		res, err := client.IssueReconnectingPTYSignedToken(ctx, codersdk.IssueReconnectingPTYSignedTokenRequest{
			URL:     "",
			AgentID: uuid.Nil,
		})
		require.Error(t, err)
		require.Empty(t, res)
		var sdkErr *codersdk.Error
		require.ErrorAs(t, err, &sdkErr)
		require.Equal(t, http.StatusBadRequest, sdkErr.StatusCode())
	})

	t.Run("BadURL", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)
		res, err := client.IssueReconnectingPTYSignedToken(ctx, codersdk.IssueReconnectingPTYSignedTokenRequest{
			URL:     ":",
			AgentID: agentID,
		})
		require.Error(t, err)
		require.Empty(t, res)
		var sdkErr *codersdk.Error
		require.ErrorAs(t, err, &sdkErr)
		require.Equal(t, http.StatusBadRequest, sdkErr.StatusCode())
		require.Contains(t, sdkErr.Response.Message, "Invalid URL")
	})

	t.Run("BadURL", func(t *testing.T) {
		t.Parallel()

		u := *u
		u.Scheme = "ftp"

		ctx := testutil.Context(t, testutil.WaitLong)
		res, err := client.IssueReconnectingPTYSignedToken(ctx, codersdk.IssueReconnectingPTYSignedTokenRequest{
			URL:     u.String(),
			AgentID: agentID,
		})
		require.Error(t, err)
		require.Empty(t, res)
		var sdkErr *codersdk.Error
		require.ErrorAs(t, err, &sdkErr)
		require.Equal(t, http.StatusBadRequest, sdkErr.StatusCode())
		require.Contains(t, sdkErr.Response.Message, "Invalid URL")
		require.Contains(t, sdkErr.Response.Detail, "scheme")
	})

	t.Run("BadURLPath", func(t *testing.T) {
		t.Parallel()

		u := *u
		u.Path = "/hello"

		ctx := testutil.Context(t, testutil.WaitLong)
		res, err := client.IssueReconnectingPTYSignedToken(ctx, codersdk.IssueReconnectingPTYSignedTokenRequest{
			URL:     u.String(),
			AgentID: agentID,
		})
		require.Error(t, err)
		require.Empty(t, res)
		var sdkErr *codersdk.Error
		require.ErrorAs(t, err, &sdkErr)
		require.Equal(t, http.StatusBadRequest, sdkErr.StatusCode())
		require.Contains(t, sdkErr.Response.Message, "Invalid URL")
		require.Contains(t, sdkErr.Response.Detail, "The provided URL is not a valid reconnecting PTY endpoint URL")
	})

	t.Run("BadHostname", func(t *testing.T) {
		t.Parallel()

		u := *u
		u.Host = "badhostname.com"

		ctx := testutil.Context(t, testutil.WaitLong)
		res, err := client.IssueReconnectingPTYSignedToken(ctx, codersdk.IssueReconnectingPTYSignedTokenRequest{
			URL:     u.String(),
			AgentID: agentID,
		})
		require.Error(t, err)
		require.Empty(t, res)
		var sdkErr *codersdk.Error
		require.ErrorAs(t, err, &sdkErr)
		require.Equal(t, http.StatusBadRequest, sdkErr.StatusCode())
		require.Contains(t, sdkErr.Response.Message, "Invalid hostname in URL")
	})

	t.Run("NoToken", func(t *testing.T) {
		t.Parallel()

		unauthedClient := codersdk.New(client.URL)

		ctx := testutil.Context(t, testutil.WaitLong)
		res, err := unauthedClient.IssueReconnectingPTYSignedToken(ctx, codersdk.IssueReconnectingPTYSignedTokenRequest{
			URL:     u.String(),
			AgentID: agentID,
		})
		require.Error(t, err)
		require.Empty(t, res)
		var sdkErr *codersdk.Error
		require.ErrorAs(t, err, &sdkErr)
		require.Equal(t, http.StatusUnauthorized, sdkErr.StatusCode())
	})

	t.Run("NoPermissions", func(t *testing.T) {
		t.Parallel()

		userClient, _ := coderdtest.CreateAnotherUser(t, client, user.OrganizationID)

		ctx := testutil.Context(t, testutil.WaitLong)
		res, err := userClient.IssueReconnectingPTYSignedToken(ctx, codersdk.IssueReconnectingPTYSignedTokenRequest{
			URL:     u.String(),
			AgentID: agentID,
		})
		require.Error(t, err)
		require.Empty(t, res)
		var sdkErr *codersdk.Error
		require.ErrorAs(t, err, &sdkErr)
		require.Equal(t, http.StatusNotFound, sdkErr.StatusCode())
	})

	t.Run("OK", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)
		res, err := client.IssueReconnectingPTYSignedToken(ctx, codersdk.IssueReconnectingPTYSignedTokenRequest{
			URL:     u.String(),
			AgentID: agentID,
		})
		require.NoError(t, err)
		require.NotEmpty(t, res.SignedToken)

		// The token is validated in the apptest suite, so we don't need to
		// validate it here.
	})
}
