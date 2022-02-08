package coderd_test

import (
	"context"
	"testing"
	"time"

	"github.com/coder/coder/coderd"
	"github.com/coder/coder/coderd/coderdtest"
	"github.com/coder/coder/codersdk"
	"github.com/coder/coder/database"
	"github.com/coder/coder/provisioner/echo"
	"github.com/coder/coder/provisionersdk/proto"
	"github.com/stretchr/testify/require"
)

func TestPostProvisionerJobsByOrganization(t *testing.T) {
	t.Parallel()
	t.Run("Create", func(t *testing.T) {
		client := coderdtest.New(t)
		user := coderdtest.CreateInitialUser(t, client)
		_ = coderdtest.NewProvisionerDaemon(t, client)
		data, err := echo.Tar(&echo.Responses{
			Parse: []*proto.Parse_Response{{
				Type: &proto.Parse_Response_Complete{
					Complete: &proto.Parse_Complete{
						ParameterSchemas: []*proto.ParameterSchema{},
					},
				},
			}},
			Provision: []*proto.Provision_Response{{
				Type: &proto.Provision_Response_Complete{
					Complete: &proto.Provision_Complete{
						Resources: []*proto.Resource{{
							Name: "dev",
							Type: "ec2_instance",
						}},
					},
				},
			}},
		})
		require.NoError(t, err)
		file, err := client.UploadFile(context.Background(), codersdk.ContentTypeTar, data)
		require.NoError(t, err)
		job, err := client.CreateProjectImportJob(context.Background(), user.Organization, coderd.CreateProjectImportJobRequest{
			FileHash:      file.Hash,
			Provisioner:   database.ProvisionerTypeEcho,
			SkipResources: false,
		})
		require.NoError(t, err)
		t.Log(job.ID)

		time.Sleep(250 * time.Millisecond)
	})
}
