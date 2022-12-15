package cli_test

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/cli/clitest"
	"github.com/coder/coder/coderd/coderdtest"
	"github.com/coder/coder/codersdk"
	"github.com/coder/coder/provisioner/echo"
	"github.com/coder/coder/provisionersdk/proto"
	"github.com/coder/coder/pty/ptytest"
	"github.com/coder/coder/testutil"
)

func TestCreate(t *testing.T) {
	t.Parallel()
	t.Run("Create", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, &coderdtest.Options{IncludeProvisionerDaemon: true})
		user := coderdtest.CreateFirstUser(t, client)
		version := coderdtest.CreateTemplateVersion(t, client, user.OrganizationID, &echo.Responses{
			Parse:          echo.ParseComplete,
			ProvisionApply: provisionCompleteWithAgent,
			ProvisionPlan:  provisionCompleteWithAgent,
		})
		coderdtest.AwaitTemplateVersionJob(t, client, version.ID)
		template := coderdtest.CreateTemplate(t, client, user.OrganizationID, version.ID)
		args := []string{
			"create",
			"my-workspace",
			"--template", template.Name,
			"--start-at", "9:30AM Mon-Fri US/Central",
			"--stop-after", "8h",
		}
		cmd, root := clitest.New(t, args...)
		clitest.SetupConfig(t, client, root)
		doneChan := make(chan struct{})
		pty := ptytest.New(t)
		cmd.SetIn(pty.Input())
		cmd.SetOut(pty.Output())
		go func() {
			defer close(doneChan)
			err := cmd.Execute()
			assert.NoError(t, err)
		}()
		matches := []struct {
			match string
			write string
		}{
			{match: "compute.main"},
			{match: "smith (linux, i386)"},
			{match: "Confirm create", write: "yes"},
		}
		for _, m := range matches {
			pty.ExpectMatch(m.match)
			if len(m.write) > 0 {
				pty.WriteLine(m.write)
			}
		}
		<-doneChan

		ws, err := client.WorkspaceByOwnerAndName(context.Background(), "testuser", "my-workspace", codersdk.WorkspaceOptions{})
		if assert.NoError(t, err, "expected workspace to be created") {
			assert.Equal(t, ws.TemplateName, template.Name)
			if assert.NotNil(t, ws.AutostartSchedule) {
				assert.Equal(t, *ws.AutostartSchedule, "CRON_TZ=US/Central 30 9 * * Mon-Fri")
			}
			if assert.NotNil(t, ws.TTLMillis) {
				assert.Equal(t, *ws.TTLMillis, 8*time.Hour.Milliseconds())
			}
		}
	})

	t.Run("CreateFromListWithSkip", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, &coderdtest.Options{IncludeProvisionerDaemon: true})
		user := coderdtest.CreateFirstUser(t, client)
		version := coderdtest.CreateTemplateVersion(t, client, user.OrganizationID, nil)
		coderdtest.AwaitTemplateVersionJob(t, client, version.ID)
		_ = coderdtest.CreateTemplate(t, client, user.OrganizationID, version.ID)
		cmd, root := clitest.New(t, "create", "my-workspace", "-y")

		member := coderdtest.CreateAnotherUser(t, client, user.OrganizationID)
		clitest.SetupConfig(t, member, root)
		cmdCtx, done := context.WithTimeout(context.Background(), testutil.WaitLong)
		go func() {
			defer done()
			err := cmd.ExecuteContext(cmdCtx)
			assert.NoError(t, err)
		}()
		// No pty interaction needed since we use the -y skip prompt flag
		<-cmdCtx.Done()
		require.ErrorIs(t, cmdCtx.Err(), context.Canceled)
	})

	t.Run("FromNothing", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, &coderdtest.Options{IncludeProvisionerDaemon: true})
		user := coderdtest.CreateFirstUser(t, client)
		version := coderdtest.CreateTemplateVersion(t, client, user.OrganizationID, nil)
		coderdtest.AwaitTemplateVersionJob(t, client, version.ID)
		template := coderdtest.CreateTemplate(t, client, user.OrganizationID, version.ID)
		cmd, root := clitest.New(t, "create", "")
		clitest.SetupConfig(t, client, root)
		doneChan := make(chan struct{})
		pty := ptytest.New(t)
		cmd.SetIn(pty.Input())
		cmd.SetOut(pty.Output())
		go func() {
			defer close(doneChan)
			err := cmd.Execute()
			assert.NoError(t, err)
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

		ws, err := client.WorkspaceByOwnerAndName(cmd.Context(), "testuser", "my-workspace", codersdk.WorkspaceOptions{})
		if assert.NoError(t, err, "expected workspace to be created") {
			assert.Equal(t, ws.TemplateName, template.Name)
			assert.Nil(t, ws.AutostartSchedule, "expected workspace autostart schedule to be nil")
		}
	})

	t.Run("WithParameter", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, &coderdtest.Options{IncludeProvisionerDaemon: true})
		user := coderdtest.CreateFirstUser(t, client)

		defaultValue := "something"
		version := coderdtest.CreateTemplateVersion(t, client, user.OrganizationID, &echo.Responses{
			Parse:          createTestParseResponseWithDefault(defaultValue),
			ProvisionApply: echo.ProvisionComplete,
			ProvisionPlan:  echo.ProvisionComplete,
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
			assert.NoError(t, err)
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
		client := coderdtest.New(t, &coderdtest.Options{IncludeProvisionerDaemon: true})
		user := coderdtest.CreateFirstUser(t, client)

		defaultValue := "something"
		version := coderdtest.CreateTemplateVersion(t, client, user.OrganizationID, &echo.Responses{
			Parse:          createTestParseResponseWithDefault(defaultValue),
			ProvisionApply: echo.ProvisionComplete,
			ProvisionPlan:  echo.ProvisionComplete,
		})

		coderdtest.AwaitTemplateVersionJob(t, client, version.ID)
		_ = coderdtest.CreateTemplate(t, client, user.OrganizationID, version.ID)
		tempDir := t.TempDir()
		removeTmpDirUntilSuccessAfterTest(t, tempDir)
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
			assert.NoError(t, err)
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

	t.Run("WithParameterFileNotContainingTheValue", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, &coderdtest.Options{IncludeProvisionerDaemon: true})
		user := coderdtest.CreateFirstUser(t, client)

		defaultValue := "something"
		version := coderdtest.CreateTemplateVersion(t, client, user.OrganizationID, &echo.Responses{
			Parse:          createTestParseResponseWithDefault(defaultValue),
			ProvisionApply: echo.ProvisionComplete,
			ProvisionPlan:  echo.ProvisionComplete,
		})

		coderdtest.AwaitTemplateVersionJob(t, client, version.ID)
		_ = coderdtest.CreateTemplate(t, client, user.OrganizationID, version.ID)

		tempDir := t.TempDir()
		removeTmpDirUntilSuccessAfterTest(t, tempDir)
		parameterFile, _ := os.CreateTemp(tempDir, "testParameterFile*.yaml")
		_, _ = parameterFile.WriteString("username: \"boingo\"")

		cmd, root := clitest.New(t, "create", "", "--parameter-file", parameterFile.Name())
		clitest.SetupConfig(t, client, root)
		doneChan := make(chan struct{})
		pty := ptytest.New(t)
		cmd.SetIn(pty.Input())
		cmd.SetOut(pty.Output())
		go func() {
			defer close(doneChan)
			err := cmd.Execute()
			assert.NoError(t, err)
		}()
		matches := []struct {
			match string
			write string
		}{
			{
				match: "Specify a name",
				write: "my-workspace",
			},
			{
				match: fmt.Sprintf("Enter a value (default: %q):", defaultValue),
				write: "bingo",
			},
			{
				match: "Confirm create?",
				write: "yes",
			},
		}

		for _, m := range matches {
			pty.ExpectMatch(m.match)
			pty.WriteLine(m.write)
		}
		<-doneChan
	})
	t.Run("FailedDryRun", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, &coderdtest.Options{IncludeProvisionerDaemon: true})
		user := coderdtest.CreateFirstUser(t, client)
		version := coderdtest.CreateTemplateVersion(t, client, user.OrganizationID, &echo.Responses{
			Parse: []*proto.Parse_Response{{
				Type: &proto.Parse_Response_Complete{
					Complete: &proto.Parse_Complete{
						ParameterSchemas: echo.ParameterSuccess,
					},
				},
			}},
			ProvisionPlan: []*proto.Provision_Response{
				{
					Type: &proto.Provision_Response_Complete{
						Complete: &proto.Provision_Complete{},
					},
				},
			},
		})

		tempDir := t.TempDir()
		parameterFile, err := os.CreateTemp(tempDir, "testParameterFile*.yaml")
		require.NoError(t, err)
		defer parameterFile.Close()
		_, _ = parameterFile.WriteString(fmt.Sprintf("%s: %q", echo.ParameterExecKey, echo.ParameterError("fail")))

		// The template import job should end up failed, but we need it to be
		// succeeded so the dry-run can begin.
		version = coderdtest.AwaitTemplateVersionJob(t, client, version.ID)
		require.Equal(t, codersdk.ProvisionerJobSucceeded, version.Job.Status, "job is not failed")

		_ = coderdtest.CreateTemplate(t, client, user.OrganizationID, version.ID)
		cmd, root := clitest.New(t, "create", "test", "--parameter-file", parameterFile.Name())
		clitest.SetupConfig(t, client, root)
		pty := ptytest.New(t)
		cmd.SetIn(pty.Input())
		cmd.SetOut(pty.Output())

		err = cmd.Execute()
		require.Error(t, err)
		require.ErrorContains(t, err, "dry-run workspace")
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
