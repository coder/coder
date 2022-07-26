package cli_test

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/cli/clitest"
	"github.com/coder/coder/coderd/coderdtest"
	"github.com/coder/coder/provisioner/echo"
	"github.com/coder/coder/provisionersdk/proto"
)

func TestStatePull(t *testing.T) {
	t.Parallel()
	t.Run("File", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, &coderdtest.Options{IncludeProvisionerD: true})
		user := coderdtest.CreateFirstUser(t, client)
		wantState := []byte("some state")
		version := coderdtest.CreateTemplateVersion(t, client, user.OrganizationID, &echo.Responses{
			Parse: echo.ParseComplete,
			Provision: []*proto.Provision_Response{{
				Type: &proto.Provision_Response_Complete{
					Complete: &proto.Provision_Complete{
						State: wantState,
					},
				},
			}},
		})
		coderdtest.AwaitTemplateVersionJob(t, client, version.ID)
		template := coderdtest.CreateTemplate(t, client, user.OrganizationID, version.ID)
		workspace := coderdtest.CreateWorkspace(t, client, user.OrganizationID, template.ID)
		coderdtest.AwaitWorkspaceBuildJob(t, client, workspace.LatestBuild.ID)
		statefilePath := filepath.Join(t.TempDir(), "state")
		cmd, root := clitest.New(t, "state", "pull", workspace.Name, statefilePath)
		clitest.SetupConfig(t, client, root)
		err := cmd.Execute()
		require.NoError(t, err)
		gotState, err := os.ReadFile(statefilePath)
		require.NoError(t, err)
		require.Equal(t, wantState, gotState)
	})
	t.Run("Stdout", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, &coderdtest.Options{IncludeProvisionerD: true})
		user := coderdtest.CreateFirstUser(t, client)
		wantState := []byte("some state")
		version := coderdtest.CreateTemplateVersion(t, client, user.OrganizationID, &echo.Responses{
			Parse: echo.ParseComplete,
			Provision: []*proto.Provision_Response{{
				Type: &proto.Provision_Response_Complete{
					Complete: &proto.Provision_Complete{
						State: wantState,
					},
				},
			}},
		})
		coderdtest.AwaitTemplateVersionJob(t, client, version.ID)
		template := coderdtest.CreateTemplate(t, client, user.OrganizationID, version.ID)
		workspace := coderdtest.CreateWorkspace(t, client, user.OrganizationID, template.ID)
		coderdtest.AwaitWorkspaceBuildJob(t, client, workspace.LatestBuild.ID)
		cmd, root := clitest.New(t, "state", "pull", workspace.Name)
		var gotState bytes.Buffer
		cmd.SetOut(&gotState)
		clitest.SetupConfig(t, client, root)
		err := cmd.Execute()
		require.NoError(t, err)
		require.Equal(t, wantState, bytes.TrimSpace(gotState.Bytes()))
	})
}

func TestStatePush(t *testing.T) {
	t.Parallel()
	t.Run("File", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, &coderdtest.Options{IncludeProvisionerD: true})
		user := coderdtest.CreateFirstUser(t, client)
		version := coderdtest.CreateTemplateVersion(t, client, user.OrganizationID, &echo.Responses{
			Parse:     echo.ParseComplete,
			Provision: echo.ProvisionComplete,
		})
		coderdtest.AwaitTemplateVersionJob(t, client, version.ID)
		template := coderdtest.CreateTemplate(t, client, user.OrganizationID, version.ID)
		workspace := coderdtest.CreateWorkspace(t, client, user.OrganizationID, template.ID)
		coderdtest.AwaitWorkspaceBuildJob(t, client, workspace.LatestBuild.ID)
		stateFile, err := os.CreateTemp(t.TempDir(), "")
		require.NoError(t, err)
		wantState := []byte("some magic state")
		_, err = stateFile.Write(wantState)
		require.NoError(t, err)
		err = stateFile.Close()
		require.NoError(t, err)
		cmd, root := clitest.New(t, "state", "push", workspace.Name, stateFile.Name())
		clitest.SetupConfig(t, client, root)
		err = cmd.Execute()
		require.NoError(t, err)
	})

	t.Run("Stdin", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, &coderdtest.Options{IncludeProvisionerD: true})
		user := coderdtest.CreateFirstUser(t, client)
		version := coderdtest.CreateTemplateVersion(t, client, user.OrganizationID, &echo.Responses{
			Parse:     echo.ParseComplete,
			Provision: echo.ProvisionComplete,
		})
		coderdtest.AwaitTemplateVersionJob(t, client, version.ID)
		template := coderdtest.CreateTemplate(t, client, user.OrganizationID, version.ID)
		workspace := coderdtest.CreateWorkspace(t, client, user.OrganizationID, template.ID)
		coderdtest.AwaitWorkspaceBuildJob(t, client, workspace.LatestBuild.ID)
		cmd, root := clitest.New(t, "state", "push", "--build", workspace.LatestBuild.Name, workspace.Name, "-")
		clitest.SetupConfig(t, client, root)
		cmd.SetIn(strings.NewReader("some magic state"))
		err := cmd.Execute()
		require.NoError(t, err)
	})
}
