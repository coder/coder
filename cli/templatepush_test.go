package cli_test

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/cli/clitest"
	"github.com/coder/coder/coderd/coderdtest"
	"github.com/coder/coder/coderd/database"
	"github.com/coder/coder/codersdk"
	"github.com/coder/coder/provisioner/echo"
	"github.com/coder/coder/provisionersdk/proto"
	"github.com/coder/coder/pty/ptytest"
)

func TestTemplatePush(t *testing.T) {
	t.Parallel()
	// NewParameter will:
	//	1. Create a template version with 0 params
	//	2. Create a new version with 1 param
	//		2a. Expects 1 param prompt, fills in value
	//	3. Assert 1 param value in new version
	//	4. Creates a new version with same param
	//		4a. Expects 0 prompts as the param value is carried over
	//	5. Assert 1 param value in new version
	//	6. Creates a new version with 0 params
	//	7. Asset 0 params in new version
	t.Run("NewParameter", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, &coderdtest.Options{IncludeProvisionerDaemon: true})
		user := coderdtest.CreateFirstUser(t, client)
		// Create initial template version to update
		lastActiveVersion := coderdtest.CreateTemplateVersion(t, client, user.OrganizationID, nil)
		_ = coderdtest.AwaitTemplateVersionJob(t, client, lastActiveVersion.ID)
		template := coderdtest.CreateTemplate(t, client, user.OrganizationID, lastActiveVersion.ID)

		// Create new template version with a new parameter
		source := clitest.CreateTemplateVersionSource(t, &echo.Responses{
			Parse:          createTestParseResponse(),
			ProvisionApply: echo.ProvisionComplete,
		})
		cmd, root := clitest.New(t, "templates", "push", template.Name, "-y", "--directory", source, "--test.provisioner", string(database.ProvisionerTypeEcho))
		clitest.SetupConfig(t, client, root)
		pty := ptytest.New(t)
		cmd.SetIn(pty.Input())
		cmd.SetOut(pty.Output())

		execDone := make(chan error)
		go func() {
			execDone <- cmd.Execute()
		}()

		matches := []struct {
			match string
			write string
		}{
			// Expect to be prompted for the new param
			{match: "Enter a value:", write: "peter-pan"},
		}
		for _, m := range matches {
			pty.ExpectMatch(m.match)
			pty.WriteLine(m.write)
		}

		require.NoError(t, <-execDone)

		// Assert template version changed and we have the new param
		latestTV, latestParams := latestTemplateVersion(t, client, template.ID)
		assert.NotEqual(t, lastActiveVersion.ID, latestTV.ID)
		require.Len(t, latestParams, 1, "expect 1 param")
		lastActiveVersion = latestTV

		// Second update of the same source requires no prompt since the params
		// are carried over.
		cmd, root = clitest.New(t, "templates", "push", template.Name, "-y", "--directory", source, "--test.provisioner", string(database.ProvisionerTypeEcho))
		clitest.SetupConfig(t, client, root)
		go func() {
			execDone <- cmd.Execute()
		}()
		require.NoError(t, <-execDone)

		// Assert template version changed and we have the carried over param
		latestTV, latestParams = latestTemplateVersion(t, client, template.ID)
		assert.NotEqual(t, lastActiveVersion.ID, latestTV.ID)
		require.Len(t, latestParams, 1, "expect 1 param")
		lastActiveVersion = latestTV

		// Remove the param
		source = clitest.CreateTemplateVersionSource(t, &echo.Responses{
			Parse:          echo.ParseComplete,
			ProvisionApply: echo.ProvisionComplete,
		})

		cmd, root = clitest.New(t, "templates", "push", template.Name, "-y", "--directory", source, "--test.provisioner", string(database.ProvisionerTypeEcho))
		clitest.SetupConfig(t, client, root)
		go func() {
			execDone <- cmd.Execute()
		}()
		require.NoError(t, <-execDone)
		// Assert template version changed and the param was removed
		latestTV, latestParams = latestTemplateVersion(t, client, template.ID)
		assert.NotEqual(t, lastActiveVersion.ID, latestTV.ID)
		require.Len(t, latestParams, 0, "expect 0 param")
		lastActiveVersion = latestTV
	})

	t.Run("OK", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, &coderdtest.Options{IncludeProvisionerDaemon: true})
		user := coderdtest.CreateFirstUser(t, client)
		version := coderdtest.CreateTemplateVersion(t, client, user.OrganizationID, nil)
		_ = coderdtest.AwaitTemplateVersionJob(t, client, version.ID)

		template := coderdtest.CreateTemplate(t, client, user.OrganizationID, version.ID)

		// Test the cli command.
		source := clitest.CreateTemplateVersionSource(t, &echo.Responses{
			Parse:          echo.ParseComplete,
			ProvisionApply: echo.ProvisionComplete,
		})
		cmd, root := clitest.New(t, "templates", "push", template.Name, "--directory", source, "--test.provisioner", string(database.ProvisionerTypeEcho), "--name", "example")
		clitest.SetupConfig(t, client, root)
		pty := ptytest.New(t)
		cmd.SetIn(pty.Input())
		cmd.SetOut(pty.Output())

		execDone := make(chan error)
		go func() {
			execDone <- cmd.Execute()
		}()

		matches := []struct {
			match string
			write string
		}{
			{match: "Upload", write: "yes"},
		}
		for _, m := range matches {
			pty.ExpectMatch(m.match)
			pty.WriteLine(m.write)
		}

		require.NoError(t, <-execDone)

		// Assert that the template version changed.
		templateVersions, err := client.TemplateVersionsByTemplate(context.Background(), codersdk.TemplateVersionsByTemplateRequest{
			TemplateID: template.ID,
		})
		require.NoError(t, err)
		assert.Len(t, templateVersions, 2)
		assert.NotEqual(t, template.ActiveVersionID, templateVersions[1].ID)
		require.Equal(t, "example", templateVersions[1].Name)
	})

	t.Run("UseWorkingDir", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, &coderdtest.Options{IncludeProvisionerDaemon: true})
		user := coderdtest.CreateFirstUser(t, client)
		version := coderdtest.CreateTemplateVersion(t, client, user.OrganizationID, nil)
		_ = coderdtest.AwaitTemplateVersionJob(t, client, version.ID)

		// Test the cli command.
		source := clitest.CreateTemplateVersionSource(t, &echo.Responses{
			Parse:          echo.ParseComplete,
			ProvisionApply: echo.ProvisionComplete,
		})

		template := coderdtest.CreateTemplate(t, client, user.OrganizationID, version.ID,
			func(r *codersdk.CreateTemplateRequest) {
				r.Name = filepath.Base(source)
			})

		// Don't pass the name of the template, it should use the
		// directory of the source.
		cmd, root := clitest.New(t, "templates", "push", "--directory", source, "--test.provisioner", string(database.ProvisionerTypeEcho))
		clitest.SetupConfig(t, client, root)
		pty := ptytest.New(t)
		cmd.SetIn(pty.Input())
		cmd.SetOut(pty.Output())

		execDone := make(chan error)
		go func() {
			execDone <- cmd.Execute()
		}()

		matches := []struct {
			match string
			write string
		}{
			{match: "Upload", write: "yes"},
		}
		for _, m := range matches {
			pty.ExpectMatch(m.match)
			pty.WriteLine(m.write)
		}

		require.NoError(t, <-execDone)

		// Assert that the template version changed.
		templateVersions, err := client.TemplateVersionsByTemplate(context.Background(), codersdk.TemplateVersionsByTemplateRequest{
			TemplateID: template.ID,
		})
		require.NoError(t, err)
		assert.Len(t, templateVersions, 2)
		assert.NotEqual(t, template.ActiveVersionID, templateVersions[1].ID)
	})

	t.Run("Stdin", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, &coderdtest.Options{IncludeProvisionerDaemon: true})
		user := coderdtest.CreateFirstUser(t, client)
		version := coderdtest.CreateTemplateVersion(t, client, user.OrganizationID, nil)
		_ = coderdtest.AwaitTemplateVersionJob(t, client, version.ID)

		source, err := echo.Tar(&echo.Responses{
			Parse:          echo.ParseComplete,
			ProvisionApply: echo.ProvisionComplete,
		})
		require.NoError(t, err)

		template := coderdtest.CreateTemplate(t, client, user.OrganizationID, version.ID)

		cmd, root := clitest.New(
			t, "templates", "push", "--directory", "-",
			"--test.provisioner", string(database.ProvisionerTypeEcho),
			template.Name,
		)
		clitest.SetupConfig(t, client, root)
		pty := ptytest.New(t)
		cmd.SetIn(bytes.NewReader(source))
		cmd.SetOut(pty.Output())

		execDone := make(chan error)
		go func() {
			execDone <- cmd.Execute()
		}()

		require.NoError(t, <-execDone)

		// Assert that the template version changed.
		templateVersions, err := client.TemplateVersionsByTemplate(context.Background(), codersdk.TemplateVersionsByTemplateRequest{
			TemplateID: template.ID,
		})
		require.NoError(t, err)
		assert.Len(t, templateVersions, 2)
		assert.NotEqual(t, template.ActiveVersionID, templateVersions[1].ID)
	})

	t.Run("Variables", func(t *testing.T) {
		t.Parallel()

		initialTemplateVariables := []*proto.TemplateVariable{
			{
				Name:         "first_variable",
				Description:  "This is the first variable",
				Type:         "string",
				DefaultValue: "abc",
				Required:     false,
				Sensitive:    true,
			},
		}

		t.Run("VariableIsRequired", func(t *testing.T) {
			t.Parallel()
			client := coderdtest.New(t, &coderdtest.Options{IncludeProvisionerDaemon: true})
			user := coderdtest.CreateFirstUser(t, client)

			templateVersion := coderdtest.CreateTemplateVersion(t, client, user.OrganizationID, createEchoResponsesWithTemplateVariables(initialTemplateVariables))
			_ = coderdtest.AwaitTemplateVersionJob(t, client, templateVersion.ID)
			template := coderdtest.CreateTemplate(t, client, user.OrganizationID, templateVersion.ID)

			// Test the cli command.
			modifiedTemplateVariables := append(initialTemplateVariables,
				&proto.TemplateVariable{
					Name:        "second_variable",
					Description: "This is the second variable",
					Type:        "string",
					Required:    true,
				},
			)
			source := clitest.CreateTemplateVersionSource(t, createEchoResponsesWithTemplateVariables(modifiedTemplateVariables))
			tempDir := t.TempDir()
			removeTmpDirUntilSuccessAfterTest(t, tempDir)
			variablesFile, _ := os.CreateTemp(tempDir, "variables*.yaml")
			_, _ = variablesFile.WriteString(`second_variable: foobar`)
			cmd, root := clitest.New(t, "templates", "push", template.Name, "--directory", source, "--test.provisioner", string(database.ProvisionerTypeEcho), "--name", "example", "--variables-file", variablesFile.Name())
			clitest.SetupConfig(t, client, root)
			pty := ptytest.New(t)
			cmd.SetIn(pty.Input())
			cmd.SetOut(pty.Output())

			execDone := make(chan error)
			go func() {
				execDone <- cmd.Execute()
			}()

			matches := []struct {
				match string
				write string
			}{
				{match: "Upload", write: "yes"},
			}
			for _, m := range matches {
				pty.ExpectMatch(m.match)
				pty.WriteLine(m.write)
			}

			require.NoError(t, <-execDone)

			// Assert that the template version changed.
			templateVersions, err := client.TemplateVersionsByTemplate(context.Background(), codersdk.TemplateVersionsByTemplateRequest{
				TemplateID: template.ID,
			})
			require.NoError(t, err)
			assert.Len(t, templateVersions, 2)
			assert.NotEqual(t, template.ActiveVersionID, templateVersions[1].ID)
			require.Equal(t, "example", templateVersions[1].Name)

			templateVariables, err := client.TemplateVersionVariables(context.Background(), templateVersions[1].ID)
			require.NoError(t, err)
			assert.Len(t, templateVariables, 2)
			require.Equal(t, "second_variable", templateVariables[1].Name)
			require.Equal(t, "foobar", templateVariables[1].Value)
		})

		t.Run("VariableIsRequiredButNotProvided", func(t *testing.T) {
			t.Parallel()
			client := coderdtest.New(t, &coderdtest.Options{IncludeProvisionerDaemon: true})
			user := coderdtest.CreateFirstUser(t, client)

			templateVersion := coderdtest.CreateTemplateVersion(t, client, user.OrganizationID, createEchoResponsesWithTemplateVariables(initialTemplateVariables))
			_ = coderdtest.AwaitTemplateVersionJob(t, client, templateVersion.ID)
			template := coderdtest.CreateTemplate(t, client, user.OrganizationID, templateVersion.ID)

			// Test the cli command.
			modifiedTemplateVariables := append(initialTemplateVariables,
				&proto.TemplateVariable{
					Name:        "second_variable",
					Description: "This is the second variable",
					Type:        "string",
					Required:    true,
				},
			)
			source := clitest.CreateTemplateVersionSource(t, createEchoResponsesWithTemplateVariables(modifiedTemplateVariables))
			cmd, root := clitest.New(t, "templates", "push", template.Name, "--directory", source, "--test.provisioner", string(database.ProvisionerTypeEcho), "--name", "example")
			clitest.SetupConfig(t, client, root)
			pty := ptytest.New(t)
			cmd.SetIn(pty.Input())
			cmd.SetOut(pty.Output())

			execDone := make(chan error)
			go func() {
				execDone <- cmd.Execute()
			}()

			matches := []struct {
				match string
				write string
			}{
				{match: "Upload", write: "yes"},
			}
			for _, m := range matches {
				pty.ExpectMatch(m.match)
				pty.WriteLine(m.write)
			}

			wantErr := <-execDone
			require.Error(t, wantErr)
			require.Contains(t, wantErr.Error(), "required template variables need values")
		})

		t.Run("VariableIsOptionalButNotProvided", func(t *testing.T) {
			t.Parallel()
			client := coderdtest.New(t, &coderdtest.Options{IncludeProvisionerDaemon: true})
			user := coderdtest.CreateFirstUser(t, client)

			templateVersion := coderdtest.CreateTemplateVersion(t, client, user.OrganizationID, createEchoResponsesWithTemplateVariables(initialTemplateVariables))
			_ = coderdtest.AwaitTemplateVersionJob(t, client, templateVersion.ID)
			template := coderdtest.CreateTemplate(t, client, user.OrganizationID, templateVersion.ID)

			// Test the cli command.
			modifiedTemplateVariables := append(initialTemplateVariables,
				&proto.TemplateVariable{
					Name:         "second_variable",
					Description:  "This is the second variable",
					Type:         "string",
					DefaultValue: "abc",
					Required:     true,
				},
			)
			source := clitest.CreateTemplateVersionSource(t, createEchoResponsesWithTemplateVariables(modifiedTemplateVariables))
			cmd, root := clitest.New(t, "templates", "push", template.Name, "--directory", source, "--test.provisioner", string(database.ProvisionerTypeEcho), "--name", "example")
			clitest.SetupConfig(t, client, root)
			pty := ptytest.New(t)
			cmd.SetIn(pty.Input())
			cmd.SetOut(pty.Output())

			execDone := make(chan error)
			go func() {
				execDone <- cmd.Execute()
			}()

			matches := []struct {
				match string
				write string
			}{
				{match: "Upload", write: "yes"},
			}
			for _, m := range matches {
				pty.ExpectMatch(m.match)
				pty.WriteLine(m.write)
			}

			require.NoError(t, <-execDone)

			// Assert that the template version changed.
			templateVersions, err := client.TemplateVersionsByTemplate(context.Background(), codersdk.TemplateVersionsByTemplateRequest{
				TemplateID: template.ID,
			})
			require.NoError(t, err)
			assert.Len(t, templateVersions, 2)
			assert.NotEqual(t, template.ActiveVersionID, templateVersions[1].ID)
			require.Equal(t, "example", templateVersions[1].Name)

			templateVariables, err := client.TemplateVersionVariables(context.Background(), templateVersions[1].ID)
			require.NoError(t, err)
			assert.Len(t, templateVariables, 2)
			require.Equal(t, "second_variable", templateVariables[1].Name)
			require.Equal(t, "abc", templateVariables[1].Value)
			require.Equal(t, templateVariables[1].DefaultValue, templateVariables[1].Value)
		})

		t.Run("WithVariableOption", func(t *testing.T) {
			t.Parallel()
			client := coderdtest.New(t, &coderdtest.Options{IncludeProvisionerDaemon: true})
			user := coderdtest.CreateFirstUser(t, client)

			templateVersion := coderdtest.CreateTemplateVersion(t, client, user.OrganizationID, createEchoResponsesWithTemplateVariables(initialTemplateVariables))
			_ = coderdtest.AwaitTemplateVersionJob(t, client, templateVersion.ID)
			template := coderdtest.CreateTemplate(t, client, user.OrganizationID, templateVersion.ID)

			// Test the cli command.
			modifiedTemplateVariables := append(initialTemplateVariables,
				&proto.TemplateVariable{
					Name:        "second_variable",
					Description: "This is the second variable",
					Type:        "string",
					Required:    true,
				},
			)
			source := clitest.CreateTemplateVersionSource(t, createEchoResponsesWithTemplateVariables(modifiedTemplateVariables))
			cmd, root := clitest.New(t, "templates", "push", template.Name, "--directory", source, "--test.provisioner", string(database.ProvisionerTypeEcho), "--name", "example", "--variable", "second_variable=foobar")
			clitest.SetupConfig(t, client, root)
			pty := ptytest.New(t)
			cmd.SetIn(pty.Input())
			cmd.SetOut(pty.Output())

			execDone := make(chan error)
			go func() {
				execDone <- cmd.Execute()
			}()

			matches := []struct {
				match string
				write string
			}{
				{match: "Upload", write: "yes"},
			}
			for _, m := range matches {
				pty.ExpectMatch(m.match)
				pty.WriteLine(m.write)
			}

			require.NoError(t, <-execDone)

			// Assert that the template version changed.
			templateVersions, err := client.TemplateVersionsByTemplate(context.Background(), codersdk.TemplateVersionsByTemplateRequest{
				TemplateID: template.ID,
			})
			require.NoError(t, err)
			assert.Len(t, templateVersions, 2)
			assert.NotEqual(t, template.ActiveVersionID, templateVersions[1].ID)
			require.Equal(t, "example", templateVersions[1].Name)

			templateVariables, err := client.TemplateVersionVariables(context.Background(), templateVersions[1].ID)
			require.NoError(t, err)
			assert.Len(t, templateVariables, 2)
			require.Equal(t, "second_variable", templateVariables[1].Name)
			require.Equal(t, "foobar", templateVariables[1].Value)
		})
	})
}

func latestTemplateVersion(t *testing.T, client *codersdk.Client, templateID uuid.UUID) (codersdk.TemplateVersion, []codersdk.Parameter) {
	t.Helper()

	ctx := context.Background()
	newTemplate, err := client.Template(ctx, templateID)
	require.NoError(t, err)
	tv, err := client.TemplateVersion(ctx, newTemplate.ActiveVersionID)
	require.NoError(t, err)
	params, err := client.Parameters(ctx, codersdk.ParameterImportJob, tv.Job.ID)
	require.NoError(t, err)

	return tv, params
}

func createEchoResponsesWithTemplateVariables(templateVariables []*proto.TemplateVariable) *echo.Responses {
	return &echo.Responses{
		Parse: []*proto.Parse_Response{
			{
				Type: &proto.Parse_Response_Complete{
					Complete: &proto.Parse_Complete{
						TemplateVariables: templateVariables,
					},
				},
			},
		},
		ProvisionPlan:  echo.ProvisionComplete,
		ProvisionApply: echo.ProvisionComplete,
	}
}
