package coderd_test

import (
	"bytes"
	"context"
	"net/http"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"cdr.dev/slog"
	"cdr.dev/slog/sloggers/slogtest"
	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/provisionerdserver"
	"github.com/coder/coder/v2/coderd/rbac"
	"github.com/coder/coder/v2/coderd/util/ptr"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/enterprise/coderd/coderdenttest"
	"github.com/coder/coder/v2/enterprise/coderd/license"
	"github.com/coder/coder/v2/provisioner/echo"
	"github.com/coder/coder/v2/provisionerd"
	provisionerdproto "github.com/coder/coder/v2/provisionerd/proto"
	"github.com/coder/coder/v2/provisionersdk"
	"github.com/coder/coder/v2/provisionersdk/proto"
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
		srv, err := templateAdminClient.ServeProvisionerDaemon(ctx, codersdk.ServeProvisionerDaemonRequest{
			Organization: user.OrganizationID,
			Provisioners: []codersdk.ProvisionerType{
				codersdk.ProvisionerTypeEcho,
			},
			Tags: map[string]string{},
		})
		require.NoError(t, err)
		srv.DRPCConn().Close()
	})

	t.Run("NoLicense", func(t *testing.T) {
		t.Parallel()
		client, user := coderdenttest.New(t, &coderdenttest.Options{DontAddLicense: true})
		templateAdminClient, _ := coderdtest.CreateAnotherUser(t, client, user.OrganizationID, rbac.RoleTemplateAdmin())
		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
		defer cancel()
		_, err := templateAdminClient.ServeProvisionerDaemon(ctx, codersdk.ServeProvisionerDaemonRequest{
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
			ProvisionPlan: []*proto.Response{{
				Type: &proto.Response_Plan{
					Plan: &proto.PlanComplete{
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
		//nolint:gocritic // Not testing file upload in this test.
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
		coderdtest.AwaitTemplateVersionJobCompleted(t, client, version.ID)
		template := coderdtest.CreateTemplate(t, client, user.OrganizationID, version.ID)
		another, _ := coderdtest.CreateAnotherUser(t, client, user.OrganizationID)
		_ = closer.Close()
		closer = coderdtest.NewExternalProvisionerDaemon(t, another, user.OrganizationID, map[string]string{
			provisionerdserver.TagScope: provisionerdserver.ScopeUser,
		})
		defer closer.Close()
		workspace := coderdtest.CreateWorkspace(t, another, user.OrganizationID, template.ID)
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
	})

	t.Run("PSK_daily_cost", func(t *testing.T) {
		t.Parallel()
		client, user := coderdenttest.New(t, &coderdenttest.Options{
			UserWorkspaceQuota: 10,
			LicenseOptions: &coderdenttest.LicenseOptions{
				Features: license.Features{
					codersdk.FeatureExternalProvisionerDaemons: 1,
					codersdk.FeatureTemplateRBAC:               1,
				},
			},
			ProvisionerDaemonPSK: "provisionersftw",
		})
		logger := slogtest.Make(t, nil).Leveled(slog.LevelDebug)
		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
		defer cancel()

		terraformClient, terraformServer := provisionersdk.MemTransportPipe()
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
			string(database.ProvisionerTypeEcho): proto.NewDRPCProvisionerClient(terraformClient),
		}
		another := codersdk.New(client.URL)
		pd := provisionerd.New(func(ctx context.Context) (provisionerdproto.DRPCProvisionerDaemonClient, error) {
			return another.ServeProvisionerDaemon(ctx, codersdk.ServeProvisionerDaemonRequest{
				Organization: user.OrganizationID,
				Provisioners: []codersdk.ProvisionerType{
					codersdk.ProvisionerTypeEcho,
				},
				Tags: map[string]string{
					provisionerdserver.TagScope: provisionerdserver.ScopeOrganization,
				},
				PreSharedKey: "provisionersftw",
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
			ProvisionApply: []*proto.Response{{
				Type: &proto.Response_Apply{
					Apply: &proto.ApplyComplete{
						Resources: []*proto.Resource{{
							Name:      "example",
							Type:      "aws_instance",
							DailyCost: 1,
							Agents: []*proto.Agent{{
								Id:   uuid.NewString(),
								Name: "example",
								Auth: &proto.Agent_Token{
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
		workspace := coderdtest.CreateWorkspace(t, client, user.OrganizationID, template.ID)
		build := coderdtest.AwaitWorkspaceBuildJobCompleted(t, client, workspace.LatestBuild.ID)
		require.Equal(t, codersdk.WorkspaceStatusRunning, build.Status)

		err = pd.Shutdown(ctx)
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
	})
}
