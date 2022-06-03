package cli_test

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/cli/clitest"
	"github.com/coder/coder/coderd/coderdtest"
	"github.com/coder/coder/coderd/database"
	"github.com/coder/coder/coderd/util/ptr"
	"github.com/coder/coder/codersdk"
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
		args := []string{
			"create",
			"my-workspace",
			"--template", template.Name,
			"--tz", "US/Central",
			"--autostart-minute", "0",
			"--autostart-hour", "*/2",
			"--autostart-day-of-week", "MON-FRI",
			"--ttl", "8h",
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

	t.Run("AboveTemplateMaxTTL", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, &coderdtest.Options{IncludeProvisionerD: true})
		user := coderdtest.CreateFirstUser(t, client)
		version := coderdtest.CreateTemplateVersion(t, client, user.OrganizationID, nil)
		coderdtest.AwaitTemplateVersionJob(t, client, version.ID)
		template := coderdtest.CreateTemplate(t, client, user.OrganizationID, version.ID, func(ctr *codersdk.CreateTemplateRequest) {
			ctr.MaxTTLMillis = ptr.Ref((12 * time.Hour).Milliseconds())
		})
		args := []string{
			"create",
			"my-workspace",
			"--template", template.Name,
			"--ttl", "12h1m",
			"-y", // don't bother with waiting
		}
		cmd, root := clitest.New(t, args...)
		clitest.SetupConfig(t, client, root)
		pty := ptytest.New(t)
		cmd.SetIn(pty.Input())
		cmd.SetOut(pty.Output())
		err := cmd.Execute()
		assert.ErrorContains(t, err, "TTL must be below template maximum 12h0m0s")
	})

	t.Run("BelowTemplateMinAutostartInterval", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, &coderdtest.Options{IncludeProvisionerD: true})
		user := coderdtest.CreateFirstUser(t, client)
		version := coderdtest.CreateTemplateVersion(t, client, user.OrganizationID, nil)
		coderdtest.AwaitTemplateVersionJob(t, client, version.ID)
		template := coderdtest.CreateTemplate(t, client, user.OrganizationID, version.ID, func(ctr *codersdk.CreateTemplateRequest) {
			ctr.MinAutostartIntervalMillis = ptr.Ref(time.Hour.Milliseconds())
		})
		args := []string{
			"create",
			"my-workspace",
			"--template", template.Name,
			"--autostart-minute", "*", // Every minute
			"--autostart-hour", "*", // Every hour
			"-y", // don't bother with waiting
		}
		cmd, root := clitest.New(t, args...)
		clitest.SetupConfig(t, client, root)
		pty := ptytest.New(t)
		cmd.SetIn(pty.Input())
		cmd.SetOut(pty.Output())
		err := cmd.Execute()
		assert.ErrorContains(t, err, "minimum autostart interval 1m0s is above template constraint 1h0m0s")
	})

	t.Run("CreateErrInvalidTz", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, &coderdtest.Options{IncludeProvisionerD: true})
		user := coderdtest.CreateFirstUser(t, client)
		version := coderdtest.CreateTemplateVersion(t, client, user.OrganizationID, nil)
		coderdtest.AwaitTemplateVersionJob(t, client, version.ID)
		template := coderdtest.CreateTemplate(t, client, user.OrganizationID, version.ID)
		args := []string{
			"create",
			"my-workspace",
			"--template", template.Name,
			"--tz", "invalid",
			"-y",
		}
		cmd, root := clitest.New(t, args...)
		clitest.SetupConfig(t, client, root)
		pty := ptytest.New(t)
		cmd.SetIn(pty.Input())
		cmd.SetOut(pty.Output())
		err := cmd.Execute()
		assert.ErrorContains(t, err, "schedule: parse schedule: provided bad location invalid: unknown time zone invalid")
	})

	t.Run("CreateErrInvalidTTL", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, &coderdtest.Options{IncludeProvisionerD: true})
		user := coderdtest.CreateFirstUser(t, client)
		version := coderdtest.CreateTemplateVersion(t, client, user.OrganizationID, nil)
		coderdtest.AwaitTemplateVersionJob(t, client, version.ID)
		template := coderdtest.CreateTemplate(t, client, user.OrganizationID, version.ID)
		args := []string{
			"create",
			"my-workspace",
			"--template", template.Name,
			"--ttl", "0s",
			"-y",
		}
		cmd, root := clitest.New(t, args...)
		clitest.SetupConfig(t, client, root)
		pty := ptytest.New(t)
		cmd.SetIn(pty.Input())
		cmd.SetOut(pty.Output())
		err := cmd.Execute()
		assert.EqualError(t, err, "TTL must be at least 1 minute")
	})

	t.Run("CreateFromListWithSkip", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, &coderdtest.Options{IncludeProvisionerD: true})
		user := coderdtest.CreateFirstUser(t, client)
		version := coderdtest.CreateTemplateVersion(t, client, user.OrganizationID, nil)
		coderdtest.AwaitTemplateVersionJob(t, client, version.ID)
		_ = coderdtest.CreateTemplate(t, client, user.OrganizationID, version.ID)
		cmd, root := clitest.New(t, "create", "my-workspace", "-y")

		member := coderdtest.CreateAnotherUser(t, client, user.OrganizationID)
		clitest.SetupConfig(t, member, root)
		cmdCtx, done := context.WithTimeout(context.Background(), time.Second*3)
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
			assert.EqualError(t, err, "Parameter value absent in parameter file for \"region\"!")
		}()
		<-doneChan
		removeTmpDirUntilSuccess(t, tempDir)
	})

	t.Run("FailedDryRun", func(t *testing.T) {
		t.Parallel()
		client, api := coderdtest.NewWithAPI(t, &coderdtest.Options{IncludeProvisionerD: true})
		user := coderdtest.CreateFirstUser(t, client)
		version := coderdtest.CreateTemplateVersion(t, client, user.OrganizationID, &echo.Responses{
			Parse: echo.ParseComplete,
			ProvisionDryRun: []*proto.Provision_Response{
				{
					Type: &proto.Provision_Response_Complete{
						Complete: &proto.Provision_Complete{
							Error: "test error",
						},
					},
				},
			},
		})

		// The template import job should end up failed, but we need it to be
		// succeeded so the dry-run can begin.
		version = coderdtest.AwaitTemplateVersionJob(t, client, version.ID)
		require.Equal(t, codersdk.ProvisionerJobFailed, version.Job.Status, "job is not failed")
		err := api.Database.UpdateProvisionerJobWithCompleteByID(context.Background(), database.UpdateProvisionerJobWithCompleteByIDParams{
			ID: version.Job.ID,
			CompletedAt: sql.NullTime{
				Time:  time.Now(),
				Valid: true,
			},
			UpdatedAt: time.Now(),
			Error:     sql.NullString{},
		})
		require.NoError(t, err, "update provisioner job")

		_ = coderdtest.CreateTemplate(t, client, user.OrganizationID, version.ID)
		cmd, root := clitest.New(t, "create", "test")
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
