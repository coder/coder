package coderd_test

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/coderd/coderdtest"
	"github.com/coder/coder/coderd/provisionerdserver"
	"github.com/coder/coder/codersdk"
	"github.com/coder/coder/enterprise/coderd/coderdenttest"
	"github.com/coder/coder/provisioner/echo"
	"github.com/coder/coder/provisionerd"
	provisionerdproto "github.com/coder/coder/provisionerd/proto"
	"github.com/coder/coder/provisionersdk/proto"
)

func TestProvisionerDaemonServe(t *testing.T) {
	t.Parallel()
	t.Run("Organization", func(t *testing.T) {
		t.Parallel()
		client := coderdenttest.New(t, nil)
		user := coderdtest.CreateFirstUser(t, client)
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
		another := coderdtest.CreateAnotherUser(t, client, user.OrganizationID)
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
		srv := provisionerd.New(func(ctx context.Context) (provisionerdproto.DRPCProvisionerDaemonClient, error) {
			return client.ServeProvisionerDaemon(context.Background(), user.OrganizationID, []codersdk.ProvisionerType{
				codersdk.ProvisionerTypeEcho,
			}, map[string]string{
				provisionerdserver.TagScope: provisionerdserver.ScopeUser,
			})
		}, nil)
		defer srv.Close()

		authToken := uuid.NewString()
		data, err := echo.Tar(&echo.Responses{
			Parse: echo.ParseComplete,
			ProvisionDryRun: []*proto.Provision_Response{{
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
			Provision: []*proto.Provision_Response{{
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
		file, err := client.Upload(context.Background(), codersdk.ContentTypeTar, data)
		require.NoError(t, err)

		_, err = client.CreateTemplateVersion(context.Background(), user.OrganizationID, codersdk.CreateTemplateVersionRequest{
			Name:          "example",
			StorageMethod: codersdk.ProvisionerStorageMethodFile,
			FileID:        file.ID,
			Provisioner:   codersdk.ProvisionerTypeEcho,
			ProvisionerTags: map[string]string{
				provisionerdserver.TagScope: provisionerdserver.ScopeUser,
			},
		})
		require.NoError(t, err)
		// coderdtest.AwaitTemplateVersionJob(t, client, version.ID)

		time.Sleep(time.Second)
	})
}

func TestPostProvisionerDaemon(t *testing.T) {
	t.Parallel()
}
