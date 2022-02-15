package cli_test

import (
	"testing"

	"github.com/coder/coder/cli/clitest"
	"github.com/coder/coder/coderd/coderdtest"
	"github.com/coder/coder/expect"
	"github.com/coder/coder/provisioner/echo"
	"github.com/coder/coder/provisionersdk/proto"
	"github.com/stretchr/testify/require"
)

func TestWorkspaceCreate(t *testing.T) {
	t.Parallel()
	t.Run("Create", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t)
		user := coderdtest.CreateInitialUser(t, client)
		_ = coderdtest.NewProvisionerDaemon(t, client)
		job := coderdtest.CreateProjectImportJob(t, client, user.Organization, &echo.Responses{
			Parse: echo.ParseComplete,
			Provision: []*proto.Provision_Response{{
				Type: &proto.Provision_Response_Complete{
					Complete: &proto.Provision_Complete{
						Resources: []*proto.Resource{{
							Name: "example",
							Type: "aws_instance",
						}},
					},
				},
			}},
		})
		coderdtest.AwaitProjectImportJob(t, client, user.Organization, job.ID)
		project := coderdtest.CreateProject(t, client, user.Organization, job.ID)
		cmd, root := clitest.New(t, "workspaces", "create", project.Name)
		clitest.SetupConfig(t, client, root)

		console := expect.NewTestConsole(t, cmd)
		closeChan := make(chan struct{})
		go func() {
			err := cmd.Execute()
			require.NoError(t, err)
			close(closeChan)
		}()

		matches := []string{
			"name?", "workspace-name",
			"Create workspace", "y",
		}
		for i := 0; i < len(matches); i += 2 {
			match := matches[i]
			value := matches[i+1]
			_, err := console.ExpectString(match)
			require.NoError(t, err)
			_, err = console.SendLine(value)
			require.NoError(t, err)
		}
		_, err := console.ExpectString("Create")
		require.NoError(t, err)
		<-closeChan
	})
}
