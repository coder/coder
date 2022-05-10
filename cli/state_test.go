package cli_test

import (
	"bytes"
	"io"
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
		api := coderdtest.New(t, nil)
		user := coderdtest.CreateFirstUser(t, api.Client)
		coderdtest.NewProvisionerDaemon(t, api.Client)
		wantState := []byte("some state")
		version := coderdtest.CreateTemplateVersion(t, api.Client, user.OrganizationID, &echo.Responses{
			Parse: echo.ParseComplete,
			Provision: []*proto.Provision_Response{{
				Type: &proto.Provision_Response_Complete{
					Complete: &proto.Provision_Complete{
						State: wantState,
					},
				},
			}},
		})
		coderdtest.AwaitTemplateVersionJob(t, api.Client, version.ID)
		template := coderdtest.CreateTemplate(t, api.Client, user.OrganizationID, version.ID)
		workspace := coderdtest.CreateWorkspace(t, api.Client, user.OrganizationID, template.ID)
		coderdtest.AwaitWorkspaceBuildJob(t, api.Client, workspace.LatestBuild.ID)
		statefilePath := filepath.Join(t.TempDir(), "state")
		cmd, root := clitest.New(t, "state", "pull", workspace.Name, statefilePath)
		clitest.SetupConfig(t, api.Client, root)
		err := cmd.Execute()
		require.NoError(t, err)
		gotState, err := os.ReadFile(statefilePath)
		require.NoError(t, err)
		require.Equal(t, wantState, gotState)
	})
	t.Run("Stdout", func(t *testing.T) {
		t.Parallel()
		api := coderdtest.New(t, nil)
		user := coderdtest.CreateFirstUser(t, api.Client)
		coderdtest.NewProvisionerDaemon(t, api.Client)
		wantState := []byte("some state")
		version := coderdtest.CreateTemplateVersion(t, api.Client, user.OrganizationID, &echo.Responses{
			Parse: echo.ParseComplete,
			Provision: []*proto.Provision_Response{{
				Type: &proto.Provision_Response_Complete{
					Complete: &proto.Provision_Complete{
						State: wantState,
					},
				},
			}},
		})
		coderdtest.AwaitTemplateVersionJob(t, api.Client, version.ID)
		template := coderdtest.CreateTemplate(t, api.Client, user.OrganizationID, version.ID)
		workspace := coderdtest.CreateWorkspace(t, api.Client, user.OrganizationID, template.ID)
		coderdtest.AwaitWorkspaceBuildJob(t, api.Client, workspace.LatestBuild.ID)
		cmd, root := clitest.New(t, "state", "pull", workspace.Name)
		var gotState bytes.Buffer
		cmd.SetOut(&gotState)
		clitest.SetupConfig(t, api.Client, root)
		err := cmd.Execute()
		require.NoError(t, err)
		require.Equal(t, wantState, bytes.TrimSpace(gotState.Bytes()))
	})
}

func TestStatePush(t *testing.T) {
	t.Parallel()
	t.Run("File", func(t *testing.T) {
		t.Parallel()
		api := coderdtest.New(t, nil)
		user := coderdtest.CreateFirstUser(t, api.Client)
		coderdtest.NewProvisionerDaemon(t, api.Client)
		version := coderdtest.CreateTemplateVersion(t, api.Client, user.OrganizationID, &echo.Responses{
			Parse:     echo.ParseComplete,
			Provision: echo.ProvisionComplete,
		})
		coderdtest.AwaitTemplateVersionJob(t, api.Client, version.ID)
		template := coderdtest.CreateTemplate(t, api.Client, user.OrganizationID, version.ID)
		workspace := coderdtest.CreateWorkspace(t, api.Client, user.OrganizationID, template.ID)
		coderdtest.AwaitWorkspaceBuildJob(t, api.Client, workspace.LatestBuild.ID)
		stateFile, err := os.CreateTemp(t.TempDir(), "")
		require.NoError(t, err)
		wantState := []byte("some magic state")
		_, err = stateFile.Write(wantState)
		require.NoError(t, err)
		err = stateFile.Close()
		require.NoError(t, err)
		cmd, root := clitest.New(t, "state", "push", workspace.Name, stateFile.Name())
		cmd.SetErr(io.Discard)
		cmd.SetOut(io.Discard)
		clitest.SetupConfig(t, api.Client, root)
		err = cmd.Execute()
		require.NoError(t, err)
	})

	t.Run("Stdin", func(t *testing.T) {
		t.Parallel()
		api := coderdtest.New(t, nil)
		user := coderdtest.CreateFirstUser(t, api.Client)
		coderdtest.NewProvisionerDaemon(t, api.Client)
		version := coderdtest.CreateTemplateVersion(t, api.Client, user.OrganizationID, &echo.Responses{
			Parse:     echo.ParseComplete,
			Provision: echo.ProvisionComplete,
		})
		coderdtest.AwaitTemplateVersionJob(t, api.Client, version.ID)
		template := coderdtest.CreateTemplate(t, api.Client, user.OrganizationID, version.ID)
		workspace := coderdtest.CreateWorkspace(t, api.Client, user.OrganizationID, template.ID)
		coderdtest.AwaitWorkspaceBuildJob(t, api.Client, workspace.LatestBuild.ID)
		cmd, root := clitest.New(t, "state", "push", "--build", workspace.LatestBuild.Name, workspace.Name, "-")
		clitest.SetupConfig(t, api.Client, root)
		cmd.SetIn(strings.NewReader("some magic state"))
		err := cmd.Execute()
		require.NoError(t, err)
	})
}
