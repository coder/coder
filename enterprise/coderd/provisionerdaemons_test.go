package coderd_test

import (
	"bytes"
	"context"
	"net/http"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/coderd/coderdtest"
	"github.com/coder/coder/coderd/provisionerdserver"
	"github.com/coder/coder/coderd/rbac"
	"github.com/coder/coder/codersdk"
	"github.com/coder/coder/enterprise/coderd/coderdenttest"
	"github.com/coder/coder/enterprise/coderd/license"
	"github.com/coder/coder/provisioner/echo"
	"github.com/coder/coder/provisionersdk/proto"
	"github.com/coder/coder/testutil"
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
		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
		defer cancel()
		srv, err := client.ServeProvisionerDaemon(ctx, codersdk.ServeProvisionerDaemonRequest{
			Organization: user.OrganizationID,
			Provisioners: []codersdk.ProvisionerType{
				codersdk.ProvisionerTypeEcho,
			},
			Tags: map[string]string{},
		})
		require.NoError(t, err)
		srv.DRPCConn().Close()
		daemons, err := client.ProvisionerDaemons(ctx)
		require.NoError(t, err)
		require.Len(t, daemons, 1)
	})

	t.Run("NoLicense", func(t *testing.T) {
		t.Parallel()
		client, user := coderdenttest.New(t, &coderdenttest.Options{DontAddLicense: true})
		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
		defer cancel()
		_, err := client.ServeProvisionerDaemon(ctx, codersdk.ServeProvisionerDaemonRequest{
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

		// querying provisioner daemons is forbidden without license
		_, err = client.ProvisionerDaemons(ctx)
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
		another, _ := coderdtest.CreateAnotherUser(t, client, user.OrganizationID, rbac.RoleOrgAdmin(user.OrganizationID))
		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
		defer cancel()
		_, err := another.ServeProvisionerDaemon(ctx, codersdk.ServeProvisionerDaemonRequest{
			Organization: user.OrganizationID,
			Provisioners: []codersdk.ProvisionerType{
				codersdk.ProvisionerTypeEcho,
			},
			Tags: map[string]string{
				provisionerdserver.TagScope: provisionerdserver.ScopeOrganization,
			},
		})
		require.Error(t, err)
		var apiError *codersdk.Error
		require.ErrorAs(t, err, &apiError)
		require.Equal(t, http.StatusForbidden, apiError.StatusCode())
		daemons, err := client.ProvisionerDaemons(ctx)
		require.NoError(t, err)
		require.Len(t, daemons, 0)
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
			Organization: user.OrganizationID,
			Provisioners: []codersdk.ProvisionerType{
				codersdk.ProvisionerTypeEcho,
			},
			Tags: map[string]string{
				provisionerdserver.TagScope: provisionerdserver.ScopeOrganization,
			},
		})
		require.Error(t, err)
		var apiError *codersdk.Error
		require.ErrorAs(t, err, &apiError)
		require.Equal(t, http.StatusForbidden, apiError.StatusCode())
		daemons, err := client.ProvisionerDaemons(ctx)
		require.NoError(t, err)
		require.Len(t, daemons, 0)
	})

	t.Run("UserLocal", func(t *testing.T) {
		t.Parallel()
		client, user := coderdenttest.New(t, &coderdenttest.Options{LicenseOptions: &coderdenttest.LicenseOptions{
			Features: license.Features{
				codersdk.FeatureExternalProvisionerDaemons: 1,
			},
		}})
		closer := coderdtest.NewExternalProvisionerDaemon(t, client, user.OrganizationID, map[string]string{
			provisionerdserver.TagScope: provisionerdserver.ScopeUser,
		})
		defer closer.Close()

		authToken := uuid.NewString()
		data, err := echo.Tar(&echo.Responses{
			Parse: echo.ParseComplete,
			ProvisionPlan: []*proto.Provision_Response{{
				Type: &proto.Provision_Response_Complete{
					Complete: &proto.Provision_Complete{
						Resources: []*proto.Resource{{
							Name: "example",
							Type: "aws_instance",
							Agents: []*proto.Agent{{
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
		file, err := client.Upload(context.Background(), codersdk.ContentTypeTar, bytes.NewReader(data))
		require.NoError(t, err)

		version, err := client.CreateTemplateVersion(context.Background(), user.OrganizationID, codersdk.CreateTemplateVersionRequest{
			Name:          "example",
			StorageMethod: codersdk.ProvisionerStorageMethodFile,
			FileID:        file.ID,
			Provisioner:   codersdk.ProvisionerTypeEcho,
			ProvisionerTags: map[string]string{
				provisionerdserver.TagScope: provisionerdserver.ScopeUser,
			},
		})
		require.NoError(t, err)
		coderdtest.AwaitTemplateVersionJob(t, client, version.ID)
		template := coderdtest.CreateTemplate(t, client, user.OrganizationID, version.ID)
		another, _ := coderdtest.CreateAnotherUser(t, client, user.OrganizationID)
		_ = closer.Close()
		closer = coderdtest.NewExternalProvisionerDaemon(t, another, user.OrganizationID, map[string]string{
			provisionerdserver.TagScope: provisionerdserver.ScopeUser,
		})
		defer closer.Close()
		workspace := coderdtest.CreateWorkspace(t, another, user.OrganizationID, template.ID)
		coderdtest.AwaitWorkspaceBuildJob(t, client, workspace.LatestBuild.ID)
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
		srv, err := another.ServeProvisionerDaemon(ctx, codersdk.ServeProvisionerDaemonRequest{
			Organization: user.OrganizationID,
			Provisioners: []codersdk.ProvisionerType{
				codersdk.ProvisionerTypeEcho,
			},
			Tags: map[string]string{
				provisionerdserver.TagScope: provisionerdserver.ScopeOrganization,
			},
			PreSharedKey: "provisionersftw",
		})
		require.NoError(t, err)
		err = srv.DRPCConn().Close()
		require.NoError(t, err)
		daemons, err := client.ProvisionerDaemons(ctx)
		require.NoError(t, err)
		require.Len(t, daemons, 1)
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
			Organization: user.OrganizationID,
			Provisioners: []codersdk.ProvisionerType{
				codersdk.ProvisionerTypeEcho,
			},
			Tags: map[string]string{
				provisionerdserver.TagScope: provisionerdserver.ScopeOrganization,
			},
			PreSharedKey: "the wrong key",
		})
		require.Error(t, err)
		var apiError *codersdk.Error
		require.ErrorAs(t, err, &apiError)
		require.Equal(t, http.StatusForbidden, apiError.StatusCode())
		daemons, err := client.ProvisionerDaemons(ctx)
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
			Organization: user.OrganizationID,
			Provisioners: []codersdk.ProvisionerType{
				codersdk.ProvisionerTypeEcho,
			},
			Tags: map[string]string{
				provisionerdserver.TagScope: provisionerdserver.ScopeOrganization,
			},
		})
		require.Error(t, err)
		var apiError *codersdk.Error
		require.ErrorAs(t, err, &apiError)
		require.Equal(t, http.StatusForbidden, apiError.StatusCode())
		daemons, err := client.ProvisionerDaemons(ctx)
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
			Organization: user.OrganizationID,
			Provisioners: []codersdk.ProvisionerType{
				codersdk.ProvisionerTypeEcho,
			},
			Tags: map[string]string{
				provisionerdserver.TagScope: provisionerdserver.ScopeOrganization,
			},
			PreSharedKey: "provisionersftw",
		})
		require.Error(t, err)
		var apiError *codersdk.Error
		require.ErrorAs(t, err, &apiError)
		require.Equal(t, http.StatusForbidden, apiError.StatusCode())
		daemons, err := client.ProvisionerDaemons(ctx)
		require.NoError(t, err)
		require.Len(t, daemons, 0)
	})
}
