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
	"github.com/coder/coder/codersdk"
	"github.com/coder/coder/enterprise/coderd/coderdenttest"
	"github.com/coder/coder/enterprise/coderd/license"
	"github.com/coder/coder/provisioner/echo"
	"github.com/coder/coder/provisionersdk/proto"
)

func TestProvisionerDaemonServe(t *testing.T) {
	t.Parallel()
	t.Run("NoLicense", func(t *testing.T) {
		t.Parallel()
		client := coderdenttest.New(t, nil)
		user := coderdtest.CreateFirstUser(t, client)
		_, err := client.ServeProvisionerDaemon(context.Background(), user.OrganizationID, []codersdk.ProvisionerType{
			codersdk.ProvisionerTypeEcho,
		}, map[string]string{})
		require.Error(t, err)
		var apiError *codersdk.Error
		require.ErrorAs(t, err, &apiError)
		require.Equal(t, http.StatusForbidden, apiError.StatusCode())
	})

	t.Run("Organization", func(t *testing.T) {
		t.Parallel()
		client := coderdenttest.New(t, nil)
		user := coderdtest.CreateFirstUser(t, client)
		coderdenttest.AddLicense(t, client, coderdenttest.LicenseOptions{
			Features: license.Features{
				codersdk.FeatureExternalProvisionerDaemons: 1,
			},
		})
		srv, err := client.ServeProvisionerDaemon(context.Background(), user.OrganizationID, []codersdk.ProvisionerType{
			codersdk.ProvisionerTypeEcho,
		}, map[string]string{})
		require.NoError(t, err)
		srv.DRPCConn().Close()
	})

	t.Run("OrganizationNoPerms", func(t *testing.T) {
		t.Parallel()
		client := coderdenttest.New(t, nil)
		user := coderdtest.CreateFirstUser(t, client)
		coderdenttest.AddLicense(t, client, coderdenttest.LicenseOptions{
			Features: license.Features{
				codersdk.FeatureExternalProvisionerDaemons: 1,
			},
		})
		another, _ := coderdtest.CreateAnotherUser(t, client, user.OrganizationID)
		_, err := another.ServeProvisionerDaemon(context.Background(), user.OrganizationID, []codersdk.ProvisionerType{
			codersdk.ProvisionerTypeEcho,
		}, map[string]string{
			provisionerdserver.TagScope: provisionerdserver.ScopeOrganization,
		})
		require.Error(t, err)
		var apiError *codersdk.Error
		require.ErrorAs(t, err, &apiError)
		require.Equal(t, http.StatusForbidden, apiError.StatusCode())
	})

	t.Run("UserLocal", func(t *testing.T) {
		t.Parallel()
		client := coderdenttest.New(t, nil)
		user := coderdtest.CreateFirstUser(t, client)
		coderdenttest.AddLicense(t, client, coderdenttest.LicenseOptions{
			Features: license.Features{
				codersdk.FeatureExternalProvisionerDaemons: 1,
			},
		})
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
			ProvisionApply: []*proto.Provision_Response{{
				Type: &proto.Provision_Response_Complete{
					Complete: &proto.Provision_Complete{
						Resources: []*proto.Resource{{
							Name: "example",
							Type: "aws_instance",
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
}
