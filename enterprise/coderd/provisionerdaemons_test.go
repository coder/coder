package coderd_test

import (
	"bytes"
	"context"
	"database/sql"
	"fmt"
	"io"
	"net/http"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"cdr.dev/slog/sloggers/slogtest"
	"github.com/coder/coder/v2/apiversion"
	"github.com/coder/coder/v2/buildinfo"
	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/db2sdk"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/provisionerkey"
	"github.com/coder/coder/v2/coderd/rbac"
	"github.com/coder/coder/v2/coderd/util/ptr"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/codersdk/drpc"
	"github.com/coder/coder/v2/enterprise/coderd/coderdenttest"
	"github.com/coder/coder/v2/enterprise/coderd/license"
	"github.com/coder/coder/v2/provisioner/echo"
	"github.com/coder/coder/v2/provisionerd"
	"github.com/coder/coder/v2/provisionerd/proto"
	"github.com/coder/coder/v2/provisionersdk"
	sdkproto "github.com/coder/coder/v2/provisionersdk/proto"
	"github.com/coder/coder/v2/testutil"
)

func TestProvisionerDaemonServe(t *testing.T) {
	t.Parallel()
	t.Run("OK", func(t *testing.T) {
		t.Parallel()
		client, user := coderdenttest.New(t, &coderdenttest.Options{LicenseOptions: &coderdenttest.LicenseOptions{
			Features: license.Features{
				codersdk.FeatureExternalProvisionerDaemons: 1,
			},
		}})
		templateAdminClient, _ := coderdtest.CreateAnotherUser(t, client, user.OrganizationID, rbac.RoleTemplateAdmin())
		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
		defer cancel()
		daemonName := testutil.MustRandString(t, 63)
		srv, err := templateAdminClient.ServeProvisionerDaemon(ctx, codersdk.ServeProvisionerDaemonRequest{
			ID:           uuid.New(),
			Name:         daemonName,
			Organization: user.OrganizationID,
			Provisioners: []codersdk.ProvisionerType{
				codersdk.ProvisionerTypeEcho,
			},
			Tags: map[string]string{},
		})
		require.NoError(t, err)
		srv.DRPCConn().Close()

		daemons, err := client.ProvisionerDaemons(ctx) //nolint:gocritic // Test assertion.
		require.NoError(t, err)
		if assert.Len(t, daemons, 1) {
			assert.Equal(t, daemonName, daemons[0].Name)
			assert.Equal(t, buildinfo.Version(), daemons[0].Version)
			assert.Equal(t, proto.CurrentVersion.String(), daemons[0].APIVersion)
		}
	})

	t.Run("NoVersion", func(t *testing.T) {
		t.Parallel()
		// In this test, we just send a HTTP request with minimal parameters to the provisionerdaemons
		// endpoint. We do not pass the required machinery to start a websocket connection, so we expect a
		// WebSocket protocol violation. This just means the pre-flight checks have passed though.

		// Sending a HTTP request triggers an error log, which would otherwise fail the test.
		logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true})
		client, user := coderdenttest.New(t, &coderdenttest.Options{
			LicenseOptions: &coderdenttest.LicenseOptions{
				Features: license.Features{
					codersdk.FeatureExternalProvisionerDaemons: 1,
				},
			},
			ProvisionerDaemonPSK: "provisionersftw",
			Options: &coderdtest.Options{
				Logger: &logger,
			},
		})
		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
		defer cancel()

		// Formulate the correct URL for provisionerd server.
		srvURL, err := client.URL.Parse(fmt.Sprintf("/api/v2/organizations/%s/provisionerdaemons/serve", user.OrganizationID))
		require.NoError(t, err)
		q := srvURL.Query()
		// Set required query parameters.
		q.Add("provisioner", "echo")
		// Note: Explicitly not setting API version.
		q.Add("version", "")
		srvURL.RawQuery = q.Encode()

		// Set PSK header for auth.
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, srvURL.String(), nil)
		require.NoError(t, err)
		req.Header.Set(codersdk.ProvisionerDaemonPSK, "provisionersftw")

		// Do the request!
		resp, err := client.HTTPClient.Do(req)
		require.NoError(t, err)
		defer resp.Body.Close()
		b, err := io.ReadAll(resp.Body)
		require.NoError(t, err)
		// The below means that provisionerd tried to serve us!
		require.Contains(t, string(b), "Internal error accepting websocket connection.")

		daemons, err := client.ProvisionerDaemons(ctx) //nolint:gocritic // Test assertion.
		require.NoError(t, err)
		if assert.Len(t, daemons, 1) {
			assert.Equal(t, "1.0", daemons[0].APIVersion) // The whole point of this test is here.
		}
	})

	t.Run("OldVersion", func(t *testing.T) {
		t.Parallel()
		// In this test, we just send a HTTP request with minimal parameters to the provisionerdaemons
		// endpoint. We do not pass the required machinery to start a websocket connection, but we pass a
		// version header that should cause provisionerd to refuse to serve us, so no websocket for you!

		// Sending a HTTP request triggers an error log, which would otherwise fail the test.
		logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true})
		client, user := coderdenttest.New(t, &coderdenttest.Options{
			LicenseOptions: &coderdenttest.LicenseOptions{
				Features: license.Features{
					codersdk.FeatureExternalProvisionerDaemons: 1,
				},
			},
			ProvisionerDaemonPSK: "provisionersftw",
			Options: &coderdtest.Options{
				Logger: &logger,
			},
		})
		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
		defer cancel()

		// Formulate the correct URL for provisionerd server.
		srvURL, err := client.URL.Parse(fmt.Sprintf("/api/v2/organizations/%s/provisionerdaemons/serve", user.OrganizationID))
		require.NoError(t, err)
		q := srvURL.Query()
		// Set required query parameters.
		q.Add("provisioner", "echo")

		// Set a different (newer) version than the current.
		v := apiversion.New(proto.CurrentMajor+1, proto.CurrentMinor+1)
		q.Add("version", v.String())
		srvURL.RawQuery = q.Encode()

		// Set PSK header for auth.
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, srvURL.String(), nil)
		require.NoError(t, err)
		req.Header.Set(codersdk.ProvisionerDaemonPSK, "provisionersftw")

		// Do the request!
		resp, err := client.HTTPClient.Do(req)
		require.NoError(t, err)
		defer resp.Body.Close()
		b, err := io.ReadAll(resp.Body)
		require.NoError(t, err)
		// The below means that provisionerd tried to serve us, checked our api version, and said nope.
		require.Contains(t, string(b), fmt.Sprintf("server is at version %s, behind requested major version %s", proto.CurrentVersion.String(), v.String()))
	})

	t.Run("NoLicense", func(t *testing.T) {
		t.Parallel()
		client, user := coderdenttest.New(t, &coderdenttest.Options{DontAddLicense: true})
		templateAdminClient, _ := coderdtest.CreateAnotherUser(t, client, user.OrganizationID, rbac.RoleTemplateAdmin())
		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
		defer cancel()
		daemonName := testutil.MustRandString(t, 63)
		_, err := templateAdminClient.ServeProvisionerDaemon(ctx, codersdk.ServeProvisionerDaemonRequest{
			ID:           uuid.New(),
			Name:         daemonName,
			Organization: user.OrganizationID,
			Provisioners: []codersdk.ProvisionerType{
				codersdk.ProvisionerTypeEcho,
			},
			Tags: map[string]string{},
		})
		require.Error(t, err)
		var apiError *codersdk.Error
		require.ErrorAs(t, err, &apiError)
		require.Equal(t, http.StatusForbidden, apiError.StatusCode())
	})

	t.Run("Organization", func(t *testing.T) {
		t.Parallel()
		client, user := coderdenttest.New(t, &coderdenttest.Options{LicenseOptions: &coderdenttest.LicenseOptions{
			Features: license.Features{
				codersdk.FeatureExternalProvisionerDaemons: 1,
			},
		}})
		another, _ := coderdtest.CreateAnotherUser(t, client, user.OrganizationID, rbac.ScopedRoleOrgAdmin(user.OrganizationID))
		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
		defer cancel()
		_, err := another.ServeProvisionerDaemon(ctx, codersdk.ServeProvisionerDaemonRequest{
			ID:           uuid.New(),
			Name:         testutil.MustRandString(t, 63),
			Organization: user.OrganizationID,
			Provisioners: []codersdk.ProvisionerType{
				codersdk.ProvisionerTypeEcho,
			},
			Tags: map[string]string{
				provisionersdk.TagScope: provisionersdk.ScopeOrganization,
			},
		})
		require.NoError(t, err)
	})

	t.Run("OrganizationNoPerms", func(t *testing.T) {
		t.Parallel()
		client, user := coderdenttest.New(t, &coderdenttest.Options{LicenseOptions: &coderdenttest.LicenseOptions{
			Features: license.Features{
				codersdk.FeatureExternalProvisionerDaemons: 1,
			},
		}})
		another, _ := coderdtest.CreateAnotherUser(t, client, user.OrganizationID)
		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
		defer cancel()
		_, err := another.ServeProvisionerDaemon(ctx, codersdk.ServeProvisionerDaemonRequest{
			ID:           uuid.New(),
			Name:         testutil.MustRandString(t, 63),
			Organization: user.OrganizationID,
			Provisioners: []codersdk.ProvisionerType{
				codersdk.ProvisionerTypeEcho,
			},
			Tags: map[string]string{
				provisionersdk.TagScope: provisionersdk.ScopeOrganization,
			},
		})
		require.Error(t, err)
		var apiError *codersdk.Error
		require.ErrorAs(t, err, &apiError)
		require.Equal(t, http.StatusForbidden, apiError.StatusCode())
	})

	t.Run("UserLocal", func(t *testing.T) {
		t.Parallel()
		client, user := coderdenttest.New(t, &coderdenttest.Options{LicenseOptions: &coderdenttest.LicenseOptions{
			Features: license.Features{
				codersdk.FeatureExternalProvisionerDaemons: 1,
			},
		}})
		closer := coderdenttest.NewExternalProvisionerDaemon(t, client, user.OrganizationID, map[string]string{
			provisionersdk.TagScope: provisionersdk.ScopeUser,
		})
		defer closer.Close()

		authToken := uuid.NewString()
		data, err := echo.Tar(&echo.Responses{
			Parse: echo.ParseComplete,
			ProvisionPlan: []*sdkproto.Response{{
				Type: &sdkproto.Response_Plan{
					Plan: &sdkproto.PlanComplete{
						Resources: []*sdkproto.Resource{{
							Name: "example",
							Type: "aws_instance",
							Agents: []*sdkproto.Agent{{
								Id:   uuid.NewString(),
								Name: "example",
							}},
						}},
					},
				},
			}},
			ProvisionApply: echo.ProvisionApplyWithAgent(authToken),
		})
		require.NoError(t, err)
		//nolint:gocritic // Not testing file upload in this test.
		file, err := client.Upload(context.Background(), codersdk.ContentTypeTar, bytes.NewReader(data))
		require.NoError(t, err)

		require.Eventually(t, func() bool {
			daemons, err := client.ProvisionerDaemons(context.Background())
			assert.NoError(t, err, "failed to get provisioner daemons")
			return len(daemons) > 0 &&
				assert.NotEmpty(t, daemons[0].Name) &&
				assert.Equal(t, provisionersdk.ScopeUser, daemons[0].Tags[provisionersdk.TagScope]) &&
				assert.Equal(t, user.UserID.String(), daemons[0].Tags[provisionersdk.TagOwner])
		}, testutil.WaitShort, testutil.IntervalMedium)

		version, err := client.CreateTemplateVersion(context.Background(), user.OrganizationID, codersdk.CreateTemplateVersionRequest{
			Name:          "example",
			StorageMethod: codersdk.ProvisionerStorageMethodFile,
			FileID:        file.ID,
			Provisioner:   codersdk.ProvisionerTypeEcho,
			ProvisionerTags: map[string]string{
				provisionersdk.TagScope: provisionersdk.ScopeUser,
			},
		})
		require.NoError(t, err)
		coderdtest.AwaitTemplateVersionJobCompleted(t, client, version.ID)
		template := coderdtest.CreateTemplate(t, client, user.OrganizationID, version.ID)
		another, _ := coderdtest.CreateAnotherUser(t, client, user.OrganizationID)
		_ = closer.Close()
		closer = coderdenttest.NewExternalProvisionerDaemon(t, another, user.OrganizationID, map[string]string{
			provisionersdk.TagScope: provisionersdk.ScopeUser,
		})
		defer closer.Close()
		workspace := coderdtest.CreateWorkspace(t, another, template.ID)
		coderdtest.AwaitWorkspaceBuildJobCompleted(t, client, workspace.LatestBuild.ID)
	})

	t.Run("PSK", func(t *testing.T) {
		t.Parallel()
		client, user := coderdenttest.New(t, &coderdenttest.Options{
			LicenseOptions: &coderdenttest.LicenseOptions{
				Features: license.Features{
					codersdk.FeatureExternalProvisionerDaemons: 1,
				},
			},
			ProvisionerDaemonPSK: "provisionersftw",
		})
		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
		defer cancel()
		another := codersdk.New(client.URL)
		daemonName := testutil.MustRandString(t, 63)
		srv, err := another.ServeProvisionerDaemon(ctx, codersdk.ServeProvisionerDaemonRequest{
			Name:         daemonName,
			Organization: user.OrganizationID,
			Provisioners: []codersdk.ProvisionerType{
				codersdk.ProvisionerTypeEcho,
			},
			Tags: map[string]string{
				provisionersdk.TagScope: provisionersdk.ScopeOrganization,
			},
			PreSharedKey: "provisionersftw",
		})
		require.NoError(t, err)
		err = srv.DRPCConn().Close()
		require.NoError(t, err)

		daemons, err := client.ProvisionerDaemons(ctx) //nolint:gocritic // Test assertion.
		require.NoError(t, err)
		if assert.Len(t, daemons, 1) {
			assert.Equal(t, daemonName, daemons[0].Name)
			assert.Equal(t, provisionersdk.ScopeOrganization, daemons[0].Tags[provisionersdk.TagScope])
		}
	})

	t.Run("ChangeTags", func(t *testing.T) {
		t.Parallel()
		client, user := coderdenttest.New(t, &coderdenttest.Options{LicenseOptions: &coderdenttest.LicenseOptions{
			Features: license.Features{
				codersdk.FeatureExternalProvisionerDaemons: 1,
			},
		}})
		another, _ := coderdtest.CreateAnotherUser(t, client, user.OrganizationID, rbac.ScopedRoleOrgAdmin(user.OrganizationID))
		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
		defer cancel()
		req := codersdk.ServeProvisionerDaemonRequest{
			ID:           uuid.New(),
			Name:         testutil.MustRandString(t, 63),
			Organization: user.OrganizationID,
			Provisioners: []codersdk.ProvisionerType{
				codersdk.ProvisionerTypeEcho,
			},
			Tags: map[string]string{
				provisionersdk.TagScope: provisionersdk.ScopeOrganization,
			},
		}
		_, err := another.ServeProvisionerDaemon(ctx, req)
		require.NoError(t, err)

		// add tag
		req.Tags["new"] = "tag"
		_, err = another.ServeProvisionerDaemon(ctx, req)
		require.NoError(t, err)

		// remove tag
		delete(req.Tags, "new")
		_, err = another.ServeProvisionerDaemon(ctx, req)
		require.NoError(t, err)
	})

	t.Run("PSK_daily_cost", func(t *testing.T) {
		t.Parallel()
		const provPSK = `provisionersftw`
		client, user := coderdenttest.New(t, &coderdenttest.Options{
			UserWorkspaceQuota: 10,
			LicenseOptions: &coderdenttest.LicenseOptions{
				Features: license.Features{
					codersdk.FeatureExternalProvisionerDaemons: 1,
					codersdk.FeatureTemplateRBAC:               1,
				},
			},
			ProvisionerDaemonPSK: provPSK,
		})
		logger := testutil.Logger(t)
		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
		defer cancel()

		terraformClient, terraformServer := drpc.MemTransportPipe()
		go func() {
			<-ctx.Done()
			_ = terraformClient.Close()
			_ = terraformServer.Close()
		}()

		tempDir := t.TempDir()
		errCh := make(chan error)
		go func() {
			err := echo.Serve(ctx, &provisionersdk.ServeOptions{
				Listener:      terraformServer,
				Logger:        logger.Named("echo"),
				WorkDirectory: tempDir,
			})
			errCh <- err
		}()

		connector := provisionerd.LocalProvisioners{
			string(database.ProvisionerTypeEcho): sdkproto.NewDRPCProvisionerClient(terraformClient),
		}
		another := codersdk.New(client.URL)
		pd := provisionerd.New(func(ctx context.Context) (proto.DRPCProvisionerDaemonClient, error) {
			return another.ServeProvisionerDaemon(ctx, codersdk.ServeProvisionerDaemonRequest{
				ID:           uuid.New(),
				Name:         testutil.MustRandString(t, 63),
				Organization: user.OrganizationID,
				Provisioners: []codersdk.ProvisionerType{
					codersdk.ProvisionerTypeEcho,
				},
				Tags: map[string]string{
					provisionersdk.TagScope: provisionersdk.ScopeOrganization,
				},
				PreSharedKey: provPSK,
			})
		}, &provisionerd.Options{
			Logger:    logger.Named("provisionerd"),
			Connector: connector,
		})
		defer pd.Close()

		// Patch the 'Everyone' group to give the user quota to build their workspace.
		//nolint:gocritic // Not testing RBAC here.
		_, err := client.PatchGroup(ctx, user.OrganizationID, codersdk.PatchGroupRequest{
			QuotaAllowance: ptr.Ref(1),
		})
		require.NoError(t, err)

		authToken := uuid.NewString()
		version := coderdtest.CreateTemplateVersion(t, client, user.OrganizationID, &echo.Responses{
			Parse: echo.ParseComplete,
			ProvisionApply: []*sdkproto.Response{{
				Type: &sdkproto.Response_Apply{
					Apply: &sdkproto.ApplyComplete{
						Resources: []*sdkproto.Resource{{
							Name:      "example",
							Type:      "aws_instance",
							DailyCost: 1,
							Agents: []*sdkproto.Agent{{
								Id:   uuid.NewString(),
								Name: "example",
								Auth: &sdkproto.Agent_Token{
									Token: authToken,
								},
							}},
						}},
					},
				},
			}},
		})
		coderdtest.AwaitTemplateVersionJobCompleted(t, client, version.ID)
		template := coderdtest.CreateTemplate(t, client, user.OrganizationID, version.ID)
		workspace := coderdtest.CreateWorkspace(t, client, template.ID)
		build := coderdtest.AwaitWorkspaceBuildJobCompleted(t, client, workspace.LatestBuild.ID)
		require.Equal(t, codersdk.WorkspaceStatusRunning, build.Status)

		err = pd.Shutdown(ctx, false)
		require.NoError(t, err)
		err = terraformServer.Close()
		require.NoError(t, err)
		select {
		case <-ctx.Done():
			t.Error("timeout waiting for server to shut down")
		case err := <-errCh:
			require.NoError(t, err)
		}
	})

	t.Run("BadPSK", func(t *testing.T) {
		t.Parallel()
		client, user := coderdenttest.New(t, &coderdenttest.Options{
			LicenseOptions: &coderdenttest.LicenseOptions{
				Features: license.Features{
					codersdk.FeatureExternalProvisionerDaemons: 1,
				},
			},
			ProvisionerDaemonPSK: "provisionersftw",
		})
		another := codersdk.New(client.URL)
		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
		defer cancel()
		_, err := another.ServeProvisionerDaemon(ctx, codersdk.ServeProvisionerDaemonRequest{
			ID:           uuid.New(),
			Name:         testutil.MustRandString(t, 32),
			Organization: user.OrganizationID,
			Provisioners: []codersdk.ProvisionerType{
				codersdk.ProvisionerTypeEcho,
			},
			Tags: map[string]string{
				provisionersdk.TagScope: provisionersdk.ScopeOrganization,
			},
			PreSharedKey: "the wrong key",
		})
		require.Error(t, err)
		var apiError *codersdk.Error
		require.ErrorAs(t, err, &apiError)
		require.Equal(t, http.StatusUnauthorized, apiError.StatusCode())

		daemons, err := client.ProvisionerDaemons(ctx) //nolint:gocritic // Test assertion.
		require.NoError(t, err)
		require.Len(t, daemons, 0)
	})

	t.Run("NoAuth", func(t *testing.T) {
		t.Parallel()
		client, user := coderdenttest.New(t, &coderdenttest.Options{
			LicenseOptions: &coderdenttest.LicenseOptions{
				Features: license.Features{
					codersdk.FeatureExternalProvisionerDaemons: 1,
				},
			},
			ProvisionerDaemonPSK: "provisionersftw",
		})
		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
		defer cancel()
		another := codersdk.New(client.URL)
		_, err := another.ServeProvisionerDaemon(ctx, codersdk.ServeProvisionerDaemonRequest{
			ID:           uuid.New(),
			Name:         testutil.MustRandString(t, 63),
			Organization: user.OrganizationID,
			Provisioners: []codersdk.ProvisionerType{
				codersdk.ProvisionerTypeEcho,
			},
			Tags: map[string]string{
				provisionersdk.TagScope: provisionersdk.ScopeOrganization,
			},
		})
		require.Error(t, err)
		var apiError *codersdk.Error
		require.ErrorAs(t, err, &apiError)
		require.Equal(t, http.StatusUnauthorized, apiError.StatusCode())

		daemons, err := client.ProvisionerDaemons(ctx) //nolint:gocritic // Test assertion.
		require.NoError(t, err)
		require.Len(t, daemons, 0)
	})

	t.Run("NoPSK", func(t *testing.T) {
		t.Parallel()
		client, user := coderdenttest.New(t, &coderdenttest.Options{
			LicenseOptions: &coderdenttest.LicenseOptions{
				Features: license.Features{
					codersdk.FeatureExternalProvisionerDaemons: 1,
				},
			},
		})
		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
		defer cancel()
		another := codersdk.New(client.URL)
		_, err := another.ServeProvisionerDaemon(ctx, codersdk.ServeProvisionerDaemonRequest{
			ID:           uuid.New(),
			Name:         testutil.MustRandString(t, 63),
			Organization: user.OrganizationID,
			Provisioners: []codersdk.ProvisionerType{
				codersdk.ProvisionerTypeEcho,
			},
			Tags: map[string]string{
				provisionersdk.TagScope: provisionersdk.ScopeOrganization,
			},
			PreSharedKey: "provisionersftw",
		})
		require.Error(t, err)
		var apiError *codersdk.Error
		require.ErrorAs(t, err, &apiError)
		require.Equal(t, http.StatusUnauthorized, apiError.StatusCode())

		daemons, err := client.ProvisionerDaemons(ctx) //nolint:gocritic // Test assertion.
		require.NoError(t, err)
		require.Len(t, daemons, 0)
	})

	t.Run("ProvisionerKeyAuth", func(t *testing.T) {
		t.Parallel()

		insertParams, token, err := provisionerkey.New(uuid.Nil, "dont-TEST-me", nil)
		require.NoError(t, err)

		tcs := []struct {
			name                   string
			psk                    string
			multiOrgFeatureEnabled bool
			insertParams           database.InsertProvisionerKeyParams
			requestProvisionerKey  string
			requestPSK             string
			errStatusCode          int
		}{
			{
				name:       "PSKAuthOK",
				psk:        "provisionersftw",
				requestPSK: "provisionersftw",
			},
			{
				name:                   "MultiOrgExperimentDisabledPSKAuthOK",
				multiOrgFeatureEnabled: true,
				psk:                    "provisionersftw",
				requestPSK:             "provisionersftw",
			},
			{
				name:       "MultiOrgFeatureDisabledPSKAuthOK",
				psk:        "provisionersftw",
				requestPSK: "provisionersftw",
			},
			{
				name:                   "MultiOrgEnabledPSKAuthOK",
				psk:                    "provisionersftw",
				multiOrgFeatureEnabled: true,
				requestPSK:             "provisionersftw",
			},
			{
				name:                   "MultiOrgEnabledKeyAuthOK",
				psk:                    "provisionersftw",
				multiOrgFeatureEnabled: true,
				insertParams:           insertParams,
				requestProvisionerKey:  token,
			},
			{
				name:                   "MultiOrgEnabledPSKAuthDisabled",
				multiOrgFeatureEnabled: true,
				requestPSK:             "provisionersftw",
				errStatusCode:          http.StatusUnauthorized,
			},
			{
				name:                   "InvalidKey",
				multiOrgFeatureEnabled: true,
				insertParams:           insertParams,
				requestProvisionerKey:  "provisionersftw",
				errStatusCode:          http.StatusBadRequest,
			},
			{
				name:                   "KeyAndPSK",
				multiOrgFeatureEnabled: true,
				psk:                    "provisionersftw",
				insertParams:           insertParams,
				requestProvisionerKey:  token,
				requestPSK:             "provisionersftw",
				errStatusCode:          http.StatusUnauthorized,
			},
			{
				name:                   "None",
				multiOrgFeatureEnabled: true,
				psk:                    "provisionersftw",
				insertParams:           insertParams,
				errStatusCode:          http.StatusUnauthorized,
			},
		}

		for _, tc := range tcs {
			t.Run(tc.name, func(t *testing.T) {
				t.Parallel()
				features := license.Features{
					codersdk.FeatureExternalProvisionerDaemons: 1,
				}
				if tc.multiOrgFeatureEnabled {
					features[codersdk.FeatureMultipleOrganizations] = 1
				}
				dv := coderdtest.DeploymentValues(t)
				client, db, user := coderdenttest.NewWithDatabase(t, &coderdenttest.Options{
					LicenseOptions: &coderdenttest.LicenseOptions{
						Features: features,
					},
					ProvisionerDaemonPSK: tc.psk,
					Options: &coderdtest.Options{
						DeploymentValues: dv,
					},
				})
				ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
				defer cancel()

				if tc.insertParams.Name != "" {
					tc.insertParams.OrganizationID = user.OrganizationID
					// nolint:gocritic // test
					_, err := db.InsertProvisionerKey(dbauthz.AsSystemRestricted(ctx), tc.insertParams)
					require.NoError(t, err)
				}

				another := codersdk.New(client.URL)
				srv, err := another.ServeProvisionerDaemon(ctx, codersdk.ServeProvisionerDaemonRequest{
					ID:           uuid.New(),
					Name:         testutil.MustRandString(t, 63),
					Organization: user.OrganizationID,
					Provisioners: []codersdk.ProvisionerType{
						codersdk.ProvisionerTypeEcho,
					},
					PreSharedKey:   tc.requestPSK,
					ProvisionerKey: tc.requestProvisionerKey,
				})
				if tc.errStatusCode != 0 {
					require.Error(t, err)
					var apiError *codersdk.Error
					require.ErrorAs(t, err, &apiError)
					require.Equal(t, http.StatusUnauthorized, apiError.StatusCode())
					return
				}

				require.NoError(t, err)
				err = srv.DRPCConn().Close()
				require.NoError(t, err)
			})
		}
	})
}

func TestGetProvisionerDaemons(t *testing.T) {
	t.Parallel()

	t.Run("OK", func(t *testing.T) {
		t.Parallel()
		dv := coderdtest.DeploymentValues(t)
		client, first := coderdenttest.New(t, &coderdenttest.Options{
			Options: &coderdtest.Options{
				DeploymentValues: dv,
			},
			ProvisionerDaemonPSK: "provisionersftw",
			LicenseOptions: &coderdenttest.LicenseOptions{
				Features: license.Features{
					codersdk.FeatureExternalProvisionerDaemons: 1,
					codersdk.FeatureMultipleOrganizations:      1,
				},
			},
		})
		org := coderdenttest.CreateOrganization(t, client, coderdenttest.CreateOrganizationOptions{})
		orgAdmin, _ := coderdtest.CreateAnotherUser(t, client, org.ID, rbac.ScopedRoleOrgAdmin(org.ID))
		outsideOrg, _ := coderdtest.CreateAnotherUser(t, client, first.OrganizationID)

		res, err := orgAdmin.CreateProvisionerKey(context.Background(), org.ID, codersdk.CreateProvisionerKeyRequest{
			Name: "my-key",
		})
		require.NoError(t, err)

		keys, err := orgAdmin.ListProvisionerKeys(context.Background(), org.ID)
		require.NoError(t, err)
		require.Len(t, keys, 1)

		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
		defer cancel()
		daemonName := testutil.MustRandString(t, 63)
		srv, err := orgAdmin.ServeProvisionerDaemon(ctx, codersdk.ServeProvisionerDaemonRequest{
			ID:           uuid.New(),
			Name:         daemonName,
			Organization: org.ID,
			Provisioners: []codersdk.ProvisionerType{
				codersdk.ProvisionerTypeEcho,
			},
			Tags:           map[string]string{},
			ProvisionerKey: res.Key,
		})
		require.NoError(t, err)
		srv.DRPCConn().Close()

		daemons, err := orgAdmin.OrganizationProvisionerDaemons(ctx, org.ID, nil)
		require.NoError(t, err)
		require.Len(t, daemons, 1)

		assert.Equal(t, daemonName, daemons[0].Name)
		assert.Equal(t, buildinfo.Version(), daemons[0].Version)
		assert.Equal(t, proto.CurrentVersion.String(), daemons[0].APIVersion)
		assert.Equal(t, keys[0].ID, daemons[0].KeyID)

		pkDaemons, err := orgAdmin.ListProvisionerKeyDaemons(ctx, org.ID)
		require.NoError(t, err)

		require.Len(t, pkDaemons, 2)
		require.Len(t, pkDaemons[0].Daemons, 1)
		assert.Equal(t, keys[0].ID, pkDaemons[0].Key.ID)
		assert.Equal(t, keys[0].Name, pkDaemons[0].Key.Name)
		// user-auth provisioners
		require.Len(t, pkDaemons[1].Daemons, 0)
		assert.Equal(t, codersdk.ProvisionerKeyUUIDUserAuth, pkDaemons[1].Key.ID)
		assert.Equal(t, codersdk.ProvisionerKeyNameUserAuth, pkDaemons[1].Key.Name)

		assert.Equal(t, daemonName, pkDaemons[0].Daemons[0].Name)
		assert.Equal(t, buildinfo.Version(), pkDaemons[0].Daemons[0].Version)
		assert.Equal(t, proto.CurrentVersion.String(), pkDaemons[0].Daemons[0].APIVersion)
		assert.Equal(t, keys[0].ID, pkDaemons[0].Daemons[0].KeyID)

		// Verify user outside the org cannot read the provisioners
		_, err = outsideOrg.ListProvisionerKeyDaemons(ctx, org.ID)
		require.Error(t, err)
	})

	t.Run("filtered by tags", func(t *testing.T) {
		t.Parallel()

		testCases := []struct {
			name                  string
			tagsToFilterBy        map[string]string
			provisionerDaemonTags map[string]string
			expectToGetDaemon     bool
		}{
			{
				name:                  "only an empty tagset finds an untagged provisioner",
				tagsToFilterBy:        map[string]string{"scope": "organization", "owner": ""},
				provisionerDaemonTags: map[string]string{"scope": "organization", "owner": ""},
				expectToGetDaemon:     true,
			},
			{
				name:                  "an exact match with a single optional tag finds a provisioner daemon",
				tagsToFilterBy:        map[string]string{"scope": "organization", "owner": "", "environment": "on-prem"},
				provisionerDaemonTags: map[string]string{"scope": "organization", "owner": "", "environment": "on-prem"},
				expectToGetDaemon:     true,
			},
			{
				name:                  "a subset of filter tags finds a daemon with a superset of tags",
				tagsToFilterBy:        map[string]string{"scope": "organization", "owner": "", "environment": "on-prem"},
				provisionerDaemonTags: map[string]string{"scope": "organization", "owner": "", "environment": "on-prem", "datacenter": "chicago"},
				expectToGetDaemon:     true,
			},
			{
				name:                  "an exact match with two additional tags finds a provisioner daemon",
				tagsToFilterBy:        map[string]string{"scope": "organization", "owner": "", "environment": "on-prem", "datacenter": "chicago"},
				provisionerDaemonTags: map[string]string{"scope": "organization", "owner": "", "environment": "on-prem", "datacenter": "chicago"},
				expectToGetDaemon:     true,
			},
			{
				name:                  "a user scoped filter tag set finds a user scoped provisioner daemon",
				tagsToFilterBy:        map[string]string{"scope": "user", "owner": "aaa"},
				provisionerDaemonTags: map[string]string{"scope": "user", "owner": "aaa"},
				expectToGetDaemon:     true,
			},
			{
				name:                  "a user scoped filter tag set finds a user scoped provisioner daemon with an additional tag",
				tagsToFilterBy:        map[string]string{"scope": "user", "owner": "aaa"},
				provisionerDaemonTags: map[string]string{"scope": "user", "owner": "aaa", "environment": "on-prem"},
				expectToGetDaemon:     true,
			},
			{
				name:                  "user-scoped provisioner with tags and user-scoped filter with tags",
				tagsToFilterBy:        map[string]string{"scope": "user", "owner": "aaa", "environment": "on-prem"},
				provisionerDaemonTags: map[string]string{"scope": "user", "owner": "aaa", "environment": "on-prem"},
				expectToGetDaemon:     true,
			},
			{
				name:                  "user-scoped provisioner with multiple tags and user-scoped filter with a subset of tags",
				tagsToFilterBy:        map[string]string{"scope": "user", "owner": "aaa", "environment": "on-prem"},
				provisionerDaemonTags: map[string]string{"scope": "user", "owner": "aaa", "environment": "on-prem", "datacenter": "chicago"},
				expectToGetDaemon:     true,
			},
			{
				name:                  "user-scoped provisioner with multiple tags and user-scoped filter with multiple tags",
				tagsToFilterBy:        map[string]string{"scope": "user", "owner": "aaa", "environment": "on-prem", "datacenter": "chicago"},
				provisionerDaemonTags: map[string]string{"scope": "user", "owner": "aaa", "environment": "on-prem", "datacenter": "chicago"},
				expectToGetDaemon:     true,
			},
			{
				name:                  "untagged provisioner and tagged filter",
				tagsToFilterBy:        map[string]string{"scope": "organization", "owner": "", "environment": "on-prem"},
				provisionerDaemonTags: map[string]string{"scope": "organization", "owner": ""},
				expectToGetDaemon:     false,
			},
			{
				name:                  "tagged provisioner and untagged filter",
				tagsToFilterBy:        map[string]string{"scope": "organization", "owner": ""},
				provisionerDaemonTags: map[string]string{"scope": "organization", "owner": "", "environment": "on-prem"},
				expectToGetDaemon:     false,
			},
			{
				name:                  "tagged provisioner and double-tagged filter",
				tagsToFilterBy:        map[string]string{"scope": "organization", "owner": "", "environment": "on-prem", "datacenter": "chicago"},
				provisionerDaemonTags: map[string]string{"scope": "organization", "owner": "", "environment": "on-prem"},
				expectToGetDaemon:     false,
			},
			{
				name:                  "double-tagged provisioner and double-tagged filter with differing tags",
				tagsToFilterBy:        map[string]string{"scope": "organization", "owner": "", "environment": "on-prem", "datacenter": "chicago"},
				provisionerDaemonTags: map[string]string{"scope": "organization", "owner": "", "environment": "on-prem", "datacenter": "new_york"},
				expectToGetDaemon:     false,
			},
			{
				name:                  "user-scoped provisioner and untagged filter",
				tagsToFilterBy:        map[string]string{"scope": "organization", "owner": ""},
				provisionerDaemonTags: map[string]string{"scope": "user", "owner": "aaa"},
				expectToGetDaemon:     false,
			},
			{
				name:                  "user-scoped provisioner and different user-scoped filter",
				tagsToFilterBy:        map[string]string{"scope": "user", "owner": "bbb"},
				provisionerDaemonTags: map[string]string{"scope": "user", "owner": "aaa"},
				expectToGetDaemon:     false,
			},
			{
				name:                  "org-scoped provisioner and user-scoped filter",
				tagsToFilterBy:        map[string]string{"scope": "user", "owner": "aaa"},
				provisionerDaemonTags: map[string]string{"scope": "organization", "owner": ""},
				expectToGetDaemon:     false,
			},
			{
				name:                  "user-scoped provisioner and org-scoped filter with tags",
				tagsToFilterBy:        map[string]string{"scope": "user", "owner": "aaa", "environment": "on-prem"},
				provisionerDaemonTags: map[string]string{"scope": "organization", "owner": ""},
				expectToGetDaemon:     false,
			},
			{
				name:                  "user-scoped provisioner and user-scoped filter with tags",
				tagsToFilterBy:        map[string]string{"scope": "user", "owner": "aaa", "environment": "on-prem"},
				provisionerDaemonTags: map[string]string{"scope": "user", "owner": "aaa"},
				expectToGetDaemon:     false,
			},
			{
				name:                  "user-scoped provisioner with tags and user-scoped filter with multiple tags",
				tagsToFilterBy:        map[string]string{"scope": "user", "owner": "aaa", "environment": "on-prem", "datacenter": "chicago"},
				provisionerDaemonTags: map[string]string{"scope": "user", "owner": "aaa", "environment": "on-prem"},
				expectToGetDaemon:     false,
			},
			{
				name:                  "user-scoped provisioner with tags and user-scoped filter with differing tags",
				tagsToFilterBy:        map[string]string{"scope": "user", "owner": "aaa", "environment": "on-prem", "datacenter": "new_york"},
				provisionerDaemonTags: map[string]string{"scope": "user", "owner": "aaa", "environment": "on-prem", "datacenter": "chicago"},
				expectToGetDaemon:     false,
			},
		}
		for _, tt := range testCases {
			tt := tt
			t.Run(tt.name, func(t *testing.T) {
				t.Parallel()
				dv := coderdtest.DeploymentValues(t)
				client, db, _ := coderdenttest.NewWithDatabase(t, &coderdenttest.Options{
					Options: &coderdtest.Options{
						DeploymentValues: dv,
					},
					ProvisionerDaemonPSK: "provisionersftw",
					LicenseOptions: &coderdenttest.LicenseOptions{
						Features: license.Features{
							codersdk.FeatureExternalProvisionerDaemons: 1,
							codersdk.FeatureMultipleOrganizations:      1,
						},
					},
				})
				ctx := testutil.Context(t, testutil.WaitShort)

				org := coderdenttest.CreateOrganization(t, client, coderdenttest.CreateOrganizationOptions{
					IncludeProvisionerDaemon: false,
				})
				orgAdmin, _ := coderdtest.CreateAnotherUser(t, client, org.ID, rbac.ScopedRoleOrgMember(org.ID))

				daemonCreatedAt := time.Now()

				//nolint:gocritic // We're not testing auth on the following in this test
				provisionerKey, err := db.InsertProvisionerKey(dbauthz.AsSystemRestricted(ctx), database.InsertProvisionerKeyParams{
					Name:           "Test Provisioner Key",
					ID:             uuid.New(),
					CreatedAt:      daemonCreatedAt,
					OrganizationID: org.ID,
					HashedSecret:   []byte{},
					Tags:           tt.provisionerDaemonTags,
				})
				require.NoError(t, err, "should be able to create a provisioner key")

				//nolint:gocritic // We're not testing auth on the following in this test
				pd, err := db.UpsertProvisionerDaemon(dbauthz.AsSystemRestricted(ctx), database.UpsertProvisionerDaemonParams{
					CreatedAt:    daemonCreatedAt,
					Name:         "Test Provisioner Daemon",
					Provisioners: []database.ProvisionerType{},
					Tags:         tt.provisionerDaemonTags,
					LastSeenAt: sql.NullTime{
						Time:  daemonCreatedAt,
						Valid: true,
					},
					Version:        "",
					OrganizationID: org.ID,
					APIVersion:     "",
					KeyID:          provisionerKey.ID,
				})
				require.NoError(t, err, "should be able to create provisioner daemon")
				daemonAsCreated := db2sdk.ProvisionerDaemon(pd)

				allDaemons, err := orgAdmin.OrganizationProvisionerDaemons(ctx, org.ID, nil)
				require.NoError(t, err)
				require.Len(t, allDaemons, 1)

				daemonsAsFound, err := orgAdmin.OrganizationProvisionerDaemons(ctx, org.ID, &codersdk.OrganizationProvisionerDaemonsOptions{
					Tags: tt.tagsToFilterBy,
				})
				if tt.expectToGetDaemon {
					require.NoError(t, err)
					require.Len(t, daemonsAsFound, 1)
					require.Equal(t, daemonAsCreated.Tags, daemonsAsFound[0].Tags, "found daemon should have the same tags as created daemon")
					require.Equal(t, daemonsAsFound[0].KeyID, provisionerKey.ID)
				} else {
					require.NoError(t, err)
					assert.Empty(t, daemonsAsFound, "should not have found daemon")
				}
			})
		}
	})
}
