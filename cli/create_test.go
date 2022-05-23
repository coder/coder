package cli_test

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/cli/clitest"
	"github.com/coder/coder/coderd/coderdtest"
	"github.com/coder/coder/provisioner/echo"
	"github.com/coder/coder/provisionersdk/proto"
	"github.com/coder/coder/pty/ptytest"
)

func TestCreate(t *testing.T) {
	t.Parallel()
	t.Run("Create", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, &coderdtest.Options{IncludeProvisionerD: true})
		user := coderdtest.CreateFirstUser(t, client)
		version := coderdtest.CreateTemplateVersion(t, client, user.OrganizationID, nil)
		coderdtest.AwaitTemplateVersionJob(t, client, version.ID)
		template := coderdtest.CreateTemplate(t, client, user.OrganizationID, version.ID)
		cmd, root := clitest.New(t, "create", "my-workspace", "--template", template.Name)
		clitest.SetupConfig(t, client, root)
		doneChan := make(chan struct{})
		pty := ptytest.New(t)
		cmd.SetIn(pty.Input())
		cmd.SetOut(pty.Output())
		go func() {
			defer close(doneChan)
			err := cmd.Execute()
			require.NoError(t, err)
		}()
		matches := []string{
			"Confirm create", "yes",
		}
		for i := 0; i < len(matches); i += 2 {
			match := matches[i]
			value := matches[i+1]
			pty.ExpectMatch(match)
			pty.WriteLine(value)
		}
		<-doneChan
	})

	t.Run("CreateFromListWithSkip", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, &coderdtest.Options{IncludeProvisionerD: true})
		user := coderdtest.CreateFirstUser(t, client)
		version := coderdtest.CreateTemplateVersion(t, client, user.OrganizationID, nil)
		coderdtest.AwaitTemplateVersionJob(t, client, version.ID)
		_ = coderdtest.CreateTemplate(t, client, user.OrganizationID, version.ID)
		cmd, root := clitest.New(t, "create", "my-workspace", "-y")
		clitest.SetupConfig(t, client, root)
		cmdCtx, done := context.WithTimeout(context.Background(), time.Second*3)
		go func() {
			defer done()
			err := cmd.ExecuteContext(cmdCtx)
			require.NoError(t, err)
		}()
		// No pty interaction needed since we use the -y skip prompt flag
		<-cmdCtx.Done()
		require.ErrorIs(t, cmdCtx.Err(), context.Canceled)
	})

	t.Run("FromNothing", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, &coderdtest.Options{IncludeProvisionerD: true})
		user := coderdtest.CreateFirstUser(t, client)
		version := coderdtest.CreateTemplateVersion(t, client, user.OrganizationID, nil)
		coderdtest.AwaitTemplateVersionJob(t, client, version.ID)
		_ = coderdtest.CreateTemplate(t, client, user.OrganizationID, version.ID)
		cmd, root := clitest.New(t, "create", "")
		clitest.SetupConfig(t, client, root)
		doneChan := make(chan struct{})
		pty := ptytest.New(t)
		cmd.SetIn(pty.Input())
		cmd.SetOut(pty.Output())
		go func() {
			defer close(doneChan)
			err := cmd.Execute()
			require.NoError(t, err)
		}()
		matches := []string{
			"Specify a name", "my-workspace",
			"Confirm create?", "yes",
		}
		for i := 0; i < len(matches); i += 2 {
			match := matches[i]
			value := matches[i+1]
			pty.ExpectMatch(match)
			pty.WriteLine(value)
		}
		<-doneChan
	})

	t.Run("WithParameter", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, &coderdtest.Options{IncludeProvisionerD: true})
		user := coderdtest.CreateFirstUser(t, client)

		defaultValue := "something"
		version := coderdtest.CreateTemplateVersion(t, client, user.OrganizationID, &echo.Responses{
			Parse:           createTestParseResponseWithDefault(defaultValue),
			Provision:       echo.ProvisionComplete,
			ProvisionDryRun: echo.ProvisionComplete,
		})

		coderdtest.AwaitTemplateVersionJob(t, client, version.ID)
		_ = coderdtest.CreateTemplate(t, client, user.OrganizationID, version.ID)
		cmd, root := clitest.New(t, "create", "")
		clitest.SetupConfig(t, client, root)
		doneChan := make(chan struct{})
		pty := ptytest.New(t)
		cmd.SetIn(pty.Input())
		cmd.SetOut(pty.Output())
		go func() {
			defer close(doneChan)
			err := cmd.Execute()
			require.NoError(t, err)
		}()

		matches := []string{
			"Specify a name", "my-workspace",
			fmt.Sprintf("Enter a value (default: %q):", defaultValue), "bingo",
			"Enter a value:", "boingo",
			"Confirm create?", "yes",
		}
		for i := 0; i < len(matches); i += 2 {
			match := matches[i]
			value := matches[i+1]
			pty.ExpectMatch(match)
			pty.WriteLine(value)
		}
		<-doneChan
	})

	t.Run("WithParameterFileContainingTheValue", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, &coderdtest.Options{IncludeProvisionerD: true})
		user := coderdtest.CreateFirstUser(t, client)

		defaultValue := "something"
		version := coderdtest.CreateTemplateVersion(t, client, user.OrganizationID, &echo.Responses{
			Parse:           createTestParseResponseWithDefault(defaultValue),
			Provision:       echo.ProvisionComplete,
			ProvisionDryRun: echo.ProvisionComplete,
		})

		coderdtest.AwaitTemplateVersionJob(t, client, version.ID)
		_ = coderdtest.CreateTemplate(t, client, user.OrganizationID, version.ID)
		tempDir := t.TempDir()
		parameterFile, _ := os.CreateTemp(tempDir, "testParameterFile*.yaml")
		_, _ = parameterFile.WriteString("region: \"bingo\"\nusername: \"boingo\"")
		cmd, root := clitest.New(t, "create", "", "--parameter-file", parameterFile.Name())
		clitest.SetupConfig(t, client, root)
		doneChan := make(chan struct{})
		pty := ptytest.New(t)
		cmd.SetIn(pty.Input())
		cmd.SetOut(pty.Output())
		go func() {
			defer close(doneChan)
			err := cmd.Execute()
			require.NoError(t, err)
		}()

		matches := []string{
			"Specify a name", "my-workspace",
			"Confirm create?", "yes",
		}
		for i := 0; i < len(matches); i += 2 {
			match := matches[i]
			value := matches[i+1]
			pty.ExpectMatch(match)
			pty.WriteLine(value)
		}
		<-doneChan
		removeTmpDirUntilSuccess(t, tempDir)
	})
	t.Run("WithParameterFileNotContainingTheValue", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, &coderdtest.Options{IncludeProvisionerD: true})
		user := coderdtest.CreateFirstUser(t, client)

		defaultValue := "something"
		version := coderdtest.CreateTemplateVersion(t, client, user.OrganizationID, &echo.Responses{
			Parse:           createTestParseResponseWithDefault(defaultValue),
			Provision:       echo.ProvisionComplete,
			ProvisionDryRun: echo.ProvisionComplete,
		})
		coderdtest.AwaitTemplateVersionJob(t, client, version.ID)
		template := coderdtest.CreateTemplate(t, client, user.OrganizationID, version.ID)
		tempDir := t.TempDir()
		parameterFile, _ := os.CreateTemp(tempDir, "testParameterFile*.yaml")
		_, _ = parameterFile.WriteString("zone: \"bananas\"")
		cmd, root := clitest.New(t, "create", "my-workspace", "--template", template.Name, "--parameter-file", parameterFile.Name())
		clitest.SetupConfig(t, client, root)
		doneChan := make(chan struct{})
		pty := ptytest.New(t)
		cmd.SetIn(pty.Input())
		cmd.SetOut(pty.Output())
		go func() {
			defer close(doneChan)
			err := cmd.Execute()
			require.EqualError(t, err, "Parameter value absent in parameter file for \"region\"!")
		}()
		<-doneChan
		removeTmpDirUntilSuccess(t, tempDir)
	})
}

func createTestParseResponseWithDefault(defaultValue string) []*proto.Parse_Response {
	return []*proto.Parse_Response{{
		Type: &proto.Parse_Response_Complete{
			Complete: &proto.Parse_Complete{
				ParameterSchemas: []*proto.ParameterSchema{
					{
						AllowOverrideSource: true,
						Name:                "region",
						Description:         "description 1",
						DefaultSource: &proto.ParameterSource{
							Scheme: proto.ParameterSource_DATA,
							Value:  defaultValue,
						},
						DefaultDestination: &proto.ParameterDestination{
							Scheme: proto.ParameterDestination_PROVISIONER_VARIABLE,
						},
					},
					{
						AllowOverrideSource: true,
						Name:                "username",
						Description:         "description 2",
						DefaultSource: &proto.ParameterSource{
							Scheme: proto.ParameterSource_DATA,
							// No default value
							Value: "",
						},
						DefaultDestination: &proto.ParameterDestination{
							Scheme: proto.ParameterDestination_PROVISIONER_VARIABLE,
						},
					},
				},
			},
		},
	}}
}
