package cli_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/cli/clitest"
	"github.com/coder/coder/coderd/coderdtest"
	"github.com/coder/coder/database"
	"github.com/coder/coder/provisioner/echo"
	"github.com/coder/coder/provisionersdk/proto"
	"github.com/coder/coder/pty/ptytest"
)

func TestProjectCreate(t *testing.T) {
	t.Parallel()
	t.Run("NoParameters", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t)
		coderdtest.CreateInitialUser(t, client)
		source := clitest.CreateProjectVersionSource(t, &echo.Responses{
			Parse:     echo.ParseComplete,
			Provision: echo.ProvisionComplete,
		})
		cmd, root := clitest.New(t, "projects", "create", "--directory", source, "--provisioner", string(database.ProvisionerTypeEcho))
		clitest.SetupConfig(t, client, root)
		_ = coderdtest.NewProvisionerDaemon(t, client)
		pty := ptytest.New(t)
		cmd.SetIn(pty.Input())
		cmd.SetOut(pty.Output())
		closeChan := make(chan struct{})
		go func() {
			err := cmd.Execute()
			require.NoError(t, err)
			close(closeChan)
		}()

		matches := []string{
			"organization?", "y",
			"name?", "test-project",
			"project?", "y",
			"created!", "n",
		}
		for i := 0; i < len(matches); i += 2 {
			match := matches[i]
			value := matches[i+1]
			pty.ExpectMatch(match)
			pty.WriteLine(value)
		}
		<-closeChan
	})

	t.Run("Parameter", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t)
		coderdtest.CreateInitialUser(t, client)
		source := clitest.CreateProjectVersionSource(t, &echo.Responses{
			Parse: []*proto.Parse_Response{{
				Type: &proto.Parse_Response_Complete{
					Complete: &proto.Parse_Complete{
						ParameterSchemas: []*proto.ParameterSchema{{
							Name: "somevar",
							DefaultDestination: &proto.ParameterDestination{
								Scheme: proto.ParameterDestination_PROVISIONER_VARIABLE,
							},
						}},
					},
				},
			}},
			Provision: echo.ProvisionComplete,
		})
		cmd, root := clitest.New(t, "projects", "create", "--directory", source, "--provisioner", string(database.ProvisionerTypeEcho))
		clitest.SetupConfig(t, client, root)
		coderdtest.NewProvisionerDaemon(t, client)
		pty := ptytest.New(t)
		cmd.SetIn(pty.Input())
		cmd.SetOut(pty.Output())
		closeChan := make(chan struct{})
		go func() {
			err := cmd.Execute()
			require.NoError(t, err)
			close(closeChan)
		}()

		matches := []string{
			"organization?", "y",
			"name?", "test-project",
			"somevar:", "value",
			"project?", "y",
			"created!", "n",
		}
		for i := 0; i < len(matches); i += 2 {
			match := matches[i]
			value := matches[i+1]
			pty.ExpectMatch(match)
			pty.WriteLine(value)
		}
		<-closeChan
	})
}
