package cli_test

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/cli/clitest"
	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/rbac"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/provisioner/echo"
	"github.com/coder/coder/v2/provisionersdk/proto"
	"github.com/coder/coder/v2/pty/ptytest"
	"github.com/coder/coder/v2/testutil"
)

func TestTemplatePush(t *testing.T) {
	t.Parallel()

	t.Run("OK", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, &coderdtest.Options{IncludeProvisionerDaemon: true})
		owner := coderdtest.CreateFirstUser(t, client)
		templateAdmin, _ := coderdtest.CreateAnotherUser(t, client, owner.OrganizationID, rbac.RoleTemplateAdmin())
		version := coderdtest.CreateTemplateVersion(t, client, owner.OrganizationID, nil)
		_ = coderdtest.AwaitTemplateVersionJobCompleted(t, client, version.ID)

		template := coderdtest.CreateTemplate(t, client, owner.OrganizationID, version.ID)

		// Test the cli command.
		source := clitest.CreateTemplateVersionSource(t, &echo.Responses{
			Parse:          echo.ParseComplete,
			ProvisionApply: echo.ApplyComplete,
		})
		inv, root := clitest.New(t, "templates", "push", template.Name, "--directory", source, "--test.provisioner", string(database.ProvisionerTypeEcho), "--name", "example")
		clitest.SetupConfig(t, templateAdmin, root)
		pty := ptytest.New(t).Attach(inv)

		execDone := make(chan error)
		go func() {
			execDone <- inv.Run()
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

	t.Run("Message less than or equal to 72 chars", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, &coderdtest.Options{IncludeProvisionerDaemon: true})
		owner := coderdtest.CreateFirstUser(t, client)
		templateAdmin, _ := coderdtest.CreateAnotherUser(t, client, owner.OrganizationID, rbac.RoleTemplateAdmin())
		version := coderdtest.CreateTemplateVersion(t, client, owner.OrganizationID, nil)
		_ = coderdtest.AwaitTemplateVersionJobCompleted(t, client, version.ID)

		template := coderdtest.CreateTemplate(t, client, owner.OrganizationID, version.ID)
		source := clitest.CreateTemplateVersionSource(t, &echo.Responses{
			Parse:          echo.ParseComplete,
			ProvisionApply: echo.ApplyComplete,
		})

		wantMessage := strings.Repeat("a", 72)

		inv, root := clitest.New(t, "templates", "push", template.Name, "--directory", source, "--test.provisioner", string(database.ProvisionerTypeEcho), "--name", "example", "--message", wantMessage, "--yes")
		clitest.SetupConfig(t, templateAdmin, root)
		pty := ptytest.New(t).Attach(inv)

		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitMedium)
		defer cancel()

		inv = inv.WithContext(ctx)
		w := clitest.StartWithWaiter(t, inv)

		pty.ExpectNoMatchBefore(ctx, "Template message is longer than 72 characters", "Updated version at")

		w.RequireSuccess()

		// Assert that the template version changed.
		templateVersions, err := client.TemplateVersionsByTemplate(ctx, codersdk.TemplateVersionsByTemplateRequest{
			TemplateID: template.ID,
		})
		require.NoError(t, err)
		assert.Len(t, templateVersions, 2)
		assert.NotEqual(t, template.ActiveVersionID, templateVersions[1].ID)
		require.Equal(t, wantMessage, templateVersions[1].Message)
	})

	t.Run("Message too long, warn but continue", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, &coderdtest.Options{IncludeProvisionerDaemon: true})
		owner := coderdtest.CreateFirstUser(t, client)
		templateAdmin, _ := coderdtest.CreateAnotherUser(t, client, owner.OrganizationID, rbac.RoleTemplateAdmin())
		version := coderdtest.CreateTemplateVersion(t, client, owner.OrganizationID, nil)
		_ = coderdtest.AwaitTemplateVersionJobCompleted(t, client, version.ID)

		template := coderdtest.CreateTemplate(t, client, owner.OrganizationID, version.ID)
		source := clitest.CreateTemplateVersionSource(t, &echo.Responses{
			Parse:          echo.ParseComplete,
			ProvisionApply: echo.ApplyComplete,
		})

		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
		defer cancel()

		for i, tt := range []struct {
			wantMessage string
			wantMatch   string
		}{
			{wantMessage: strings.Repeat("a", 73), wantMatch: "Template message is longer than 72 characters"},
			{wantMessage: "This is my title\n\nAnd this is my body.", wantMatch: "Template message contains newlines"},
		} {
			inv, root := clitest.New(t, "templates", "push", template.Name,
				"--directory", source,
				"--test.provisioner", string(database.ProvisionerTypeEcho),
				"--message", tt.wantMessage,
				"--yes",
			)
			clitest.SetupConfig(t, templateAdmin, root)
			pty := ptytest.New(t).Attach(inv)

			inv = inv.WithContext(ctx)
			w := clitest.StartWithWaiter(t, inv)

			pty.ExpectMatchContext(ctx, tt.wantMatch)

			w.RequireSuccess()

			// Assert that the template version changed.
			templateVersions, err := client.TemplateVersionsByTemplate(ctx, codersdk.TemplateVersionsByTemplateRequest{
				TemplateID: template.ID,
			})
			require.NoError(t, err)
			assert.Len(t, templateVersions, 2+i)
			assert.NotEqual(t, template.ActiveVersionID, templateVersions[1+i].ID)
			require.Equal(t, tt.wantMessage, templateVersions[1+i].Message)
		}
	})

	t.Run("NoLockfile", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, &coderdtest.Options{IncludeProvisionerDaemon: true})
		owner := coderdtest.CreateFirstUser(t, client)
		templateAdmin, _ := coderdtest.CreateAnotherUser(t, client, owner.OrganizationID, rbac.RoleTemplateAdmin())
		version := coderdtest.CreateTemplateVersion(t, client, owner.OrganizationID, nil)
		_ = coderdtest.AwaitTemplateVersionJobCompleted(t, client, version.ID)

		template := coderdtest.CreateTemplate(t, client, owner.OrganizationID, version.ID)

		// Test the cli command.
		source := clitest.CreateTemplateVersionSource(t, &echo.Responses{
			Parse:          echo.ParseComplete,
			ProvisionApply: echo.ApplyComplete,
		})
		require.NoError(t, os.Remove(filepath.Join(source, ".terraform.lock.hcl")))

		inv, root := clitest.New(t, "templates", "push", template.Name,
			"--directory", source,
			"--test.provisioner", string(database.ProvisionerTypeEcho),
			"--name", "example",
		)
		clitest.SetupConfig(t, templateAdmin, root)
		pty := ptytest.New(t).Attach(inv)

		execDone := make(chan error)
		go func() {
			execDone <- inv.Run()
		}()

		matches := []struct {
			match string
			write string
		}{
			{match: "No .terraform.lock.hcl file found"},
			{match: "Upload", write: "no"},
		}
		for _, m := range matches {
			pty.ExpectMatch(m.match)
			if m.write != "" {
				pty.WriteLine(m.write)
			}
		}

		// cmd should error once we say no.
		require.Error(t, <-execDone)
	})

	t.Run("NoLockfileIgnored", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, &coderdtest.Options{IncludeProvisionerDaemon: true})
		owner := coderdtest.CreateFirstUser(t, client)
		templateAdmin, _ := coderdtest.CreateAnotherUser(t, client, owner.OrganizationID, rbac.RoleTemplateAdmin())
		version := coderdtest.CreateTemplateVersion(t, client, owner.OrganizationID, nil)
		_ = coderdtest.AwaitTemplateVersionJobCompleted(t, client, version.ID)

		template := coderdtest.CreateTemplate(t, client, owner.OrganizationID, version.ID)

		// Test the cli command.
		source := clitest.CreateTemplateVersionSource(t, &echo.Responses{
			Parse:          echo.ParseComplete,
			ProvisionApply: echo.ApplyComplete,
		})
		require.NoError(t, os.Remove(filepath.Join(source, ".terraform.lock.hcl")))

		inv, root := clitest.New(t, "templates", "push", template.Name,
			"--directory", source,
			"--test.provisioner", string(database.ProvisionerTypeEcho),
			"--name", "example",
			"--ignore-lockfile",
		)
		clitest.SetupConfig(t, templateAdmin, root)
		pty := ptytest.New(t).Attach(inv)

		execDone := make(chan error)
		go func() {
			execDone <- inv.Run()
		}()

		{
			ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitMedium)
			defer cancel()

			pty.ExpectNoMatchBefore(ctx, "No .terraform.lock.hcl file found", "Upload")
			pty.WriteLine("no")
		}

		// cmd should error once we say no.
		require.Error(t, <-execDone)
	})

	t.Run("PushInactiveTemplateVersion", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, &coderdtest.Options{IncludeProvisionerDaemon: true})
		owner := coderdtest.CreateFirstUser(t, client)
		templateAdmin, _ := coderdtest.CreateAnotherUser(t, client, owner.OrganizationID, rbac.RoleTemplateAdmin())
		version := coderdtest.CreateTemplateVersion(t, client, owner.OrganizationID, nil)
		_ = coderdtest.AwaitTemplateVersionJobCompleted(t, client, version.ID)

		template := coderdtest.CreateTemplate(t, client, owner.OrganizationID, version.ID)

		// Test the cli command.
		source := clitest.CreateTemplateVersionSource(t, &echo.Responses{
			Parse:          echo.ParseComplete,
			ProvisionApply: echo.ApplyComplete,
		})
		inv, root := clitest.New(t, "templates", "push", template.Name,
			"--activate=false",
			"--directory", source,
			"--test.provisioner", string(database.ProvisionerTypeEcho),
			"--name", "example",
		)
		clitest.SetupConfig(t, templateAdmin, root)
		pty := ptytest.New(t).Attach(inv)
		w := clitest.StartWithWaiter(t, inv)

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

		w.RequireSuccess()

		// Assert that the template version didn't change.
		templateVersions, err := client.TemplateVersionsByTemplate(context.Background(), codersdk.TemplateVersionsByTemplateRequest{
			TemplateID: template.ID,
		})
		require.NoError(t, err)
		assert.Len(t, templateVersions, 2)
		assert.Equal(t, template.ActiveVersionID, templateVersions[0].ID)
		require.NotEqual(t, "example", templateVersions[0].Name)
	})

	t.Run("UseWorkingDir", func(t *testing.T) {
		t.Parallel()

		if runtime.GOOS == "windows" {
			t.Skip(`On Windows this test flakes with: "The process cannot access the file because it is being used by another process"`)
		}

		client := coderdtest.New(t, &coderdtest.Options{IncludeProvisionerDaemon: true})
		owner := coderdtest.CreateFirstUser(t, client)
		templateAdmin, _ := coderdtest.CreateAnotherUser(t, client, owner.OrganizationID, rbac.RoleTemplateAdmin())
		version := coderdtest.CreateTemplateVersion(t, client, owner.OrganizationID, nil)
		_ = coderdtest.AwaitTemplateVersionJobCompleted(t, client, version.ID)

		// Test the cli command.
		source := clitest.CreateTemplateVersionSource(t, &echo.Responses{
			Parse:          echo.ParseComplete,
			ProvisionApply: echo.ApplyComplete,
		})

		template := coderdtest.CreateTemplate(t, client, owner.OrganizationID, version.ID,
			func(r *codersdk.CreateTemplateRequest) {
				r.Name = filepath.Base(source)
			})

		// Don't pass the name of the template, it should use the
		// directory of the source.
		inv, root := clitest.New(t, "templates", "push",
			"--test.provisioner", string(database.ProvisionerTypeEcho),
			"--test.workdir", source,
		)
		clitest.SetupConfig(t, templateAdmin, root)
		pty := ptytest.New(t).Attach(inv)

		waiter := clitest.StartWithWaiter(t, inv)

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

		waiter.RequireSuccess()

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
		owner := coderdtest.CreateFirstUser(t, client)
		templateAdmin, _ := coderdtest.CreateAnotherUser(t, client, owner.OrganizationID, rbac.RoleTemplateAdmin())
		version := coderdtest.CreateTemplateVersion(t, client, owner.OrganizationID, nil)
		_ = coderdtest.AwaitTemplateVersionJobCompleted(t, client, version.ID)

		source, err := echo.Tar(&echo.Responses{
			Parse:          echo.ParseComplete,
			ProvisionApply: echo.ApplyComplete,
		})
		require.NoError(t, err)

		template := coderdtest.CreateTemplate(t, client, owner.OrganizationID, version.ID)

		inv, root := clitest.New(
			t, "templates", "push", "--directory", "-",
			"--test.provisioner", string(database.ProvisionerTypeEcho),
			template.Name,
		)
		clitest.SetupConfig(t, templateAdmin, root)
		pty := ptytest.New(t)
		inv.Stdin = bytes.NewReader(source)
		inv.Stdout = pty.Output()

		execDone := make(chan error)
		go func() {
			execDone <- inv.Run()
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
			owner := coderdtest.CreateFirstUser(t, client)
			templateAdmin, _ := coderdtest.CreateAnotherUser(t, client, owner.OrganizationID, rbac.RoleTemplateAdmin())

			templateVersion := coderdtest.CreateTemplateVersion(t, client, owner.OrganizationID, createEchoResponsesWithTemplateVariables(initialTemplateVariables))
			_ = coderdtest.AwaitTemplateVersionJobCompleted(t, client, templateVersion.ID)
			template := coderdtest.CreateTemplate(t, client, owner.OrganizationID, templateVersion.ID)

			// Test the cli command.
			modifiedTemplateVariables := append(initialTemplateVariables,
				&proto.TemplateVariable{
					Name:        "second_variable",
					Description: "This is the second variable.",
					Type:        "string",
					Required:    true,
				},
			)
			source := clitest.CreateTemplateVersionSource(t, createEchoResponsesWithTemplateVariables(modifiedTemplateVariables))
			tempDir := t.TempDir()
			removeTmpDirUntilSuccessAfterTest(t, tempDir)
			variablesFile, _ := os.CreateTemp(tempDir, "variables*.yaml")
			_, _ = variablesFile.WriteString(`second_variable: foobar`)
			inv, root := clitest.New(t, "templates", "push", template.Name,
				"--directory", source,
				"--test.provisioner", string(database.ProvisionerTypeEcho),
				"--name", "example",
				"--variables-file", variablesFile.Name(),
			)
			clitest.SetupConfig(t, templateAdmin, root)
			pty := ptytest.New(t)
			inv.Stdin = pty.Input()
			inv.Stdout = pty.Output()

			execDone := make(chan error)
			go func() {
				execDone <- inv.Run()
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
			owner := coderdtest.CreateFirstUser(t, client)
			templateAdmin, _ := coderdtest.CreateAnotherUser(t, client, owner.OrganizationID, rbac.RoleTemplateAdmin())

			templateVersion := coderdtest.CreateTemplateVersion(t, client, owner.OrganizationID, createEchoResponsesWithTemplateVariables(initialTemplateVariables))
			_ = coderdtest.AwaitTemplateVersionJobCompleted(t, client, templateVersion.ID)
			template := coderdtest.CreateTemplate(t, client, owner.OrganizationID, templateVersion.ID)

			// Test the cli command.
			modifiedTemplateVariables := append(initialTemplateVariables,
				&proto.TemplateVariable{
					Name:        "second_variable",
					Description: "This is the second variable.",
					Type:        "string",
					Required:    true,
				},
			)
			source := clitest.CreateTemplateVersionSource(t, createEchoResponsesWithTemplateVariables(modifiedTemplateVariables))
			inv, root := clitest.New(t, "templates", "push", template.Name, "--directory", source, "--test.provisioner", string(database.ProvisionerTypeEcho), "--name", "example")
			clitest.SetupConfig(t, templateAdmin, root)
			pty := ptytest.New(t)
			inv.Stdin = pty.Input()
			inv.Stdout = pty.Output()

			execDone := make(chan error)
			go func() {
				execDone <- inv.Run()
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
			owner := coderdtest.CreateFirstUser(t, client)
			templateAdmin, _ := coderdtest.CreateAnotherUser(t, client, owner.OrganizationID, rbac.RoleTemplateAdmin())

			templateVersion := coderdtest.CreateTemplateVersion(t, client, owner.OrganizationID, createEchoResponsesWithTemplateVariables(initialTemplateVariables))
			_ = coderdtest.AwaitTemplateVersionJobCompleted(t, client, templateVersion.ID)
			template := coderdtest.CreateTemplate(t, client, owner.OrganizationID, templateVersion.ID)

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
			inv, root := clitest.New(t, "templates", "push", template.Name,
				"--directory", source,
				"--test.provisioner", string(database.ProvisionerTypeEcho),
				"--name", "example",
			)
			clitest.SetupConfig(t, templateAdmin, root)
			pty := ptytest.New(t)
			inv.Stdin = pty.Input()
			inv.Stdout = pty.Output()

			execDone := make(chan error)
			go func() {
				execDone <- inv.Run()
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
			owner := coderdtest.CreateFirstUser(t, client)
			templateAdmin, _ := coderdtest.CreateAnotherUser(t, client, owner.OrganizationID, rbac.RoleTemplateAdmin())

			templateVersion := coderdtest.CreateTemplateVersion(t, client, owner.OrganizationID, createEchoResponsesWithTemplateVariables(initialTemplateVariables))
			_ = coderdtest.AwaitTemplateVersionJobCompleted(t, client, templateVersion.ID)
			template := coderdtest.CreateTemplate(t, client, owner.OrganizationID, templateVersion.ID)

			// Test the cli command.
			modifiedTemplateVariables := append(initialTemplateVariables,
				&proto.TemplateVariable{
					Name:        "second_variable",
					Description: "This is the second variable.",
					Type:        "string",
					Required:    true,
				},
			)
			source := clitest.CreateTemplateVersionSource(t, createEchoResponsesWithTemplateVariables(modifiedTemplateVariables))
			inv, root := clitest.New(t,
				"templates", "push", template.Name,
				"--directory", source,
				"--test.provisioner", string(database.ProvisionerTypeEcho),
				"--name", "example",
				"--variable", "second_variable=foobar",
			)
			clitest.SetupConfig(t, templateAdmin, root)
			pty := ptytest.New(t)
			inv.Stdin = pty.Input()
			inv.Stdout = pty.Output()

			execDone := make(chan error)
			go func() {
				execDone <- inv.Run()
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

		t.Run("CreateTemplate", func(t *testing.T) {
			t.Parallel()
			client := coderdtest.New(t, &coderdtest.Options{IncludeProvisionerDaemon: true})
			owner := coderdtest.CreateFirstUser(t, client)
			templateAdmin, _ := coderdtest.CreateAnotherUser(t, client, owner.OrganizationID, rbac.RoleTemplateAdmin())
			source := clitest.CreateTemplateVersionSource(t, completeWithAgent())

			const templateName = "my-template"
			args := []string{
				"templates",
				"push",
				templateName,
				"--directory", source,
				"--test.provisioner", string(database.ProvisionerTypeEcho),
				"--create",
			}
			inv, root := clitest.New(t, args...)
			clitest.SetupConfig(t, templateAdmin, root)
			pty := ptytest.New(t).Attach(inv)

			waiter := clitest.StartWithWaiter(t, inv)

			matches := []struct {
				match string
				write string
			}{
				{match: "Upload", write: "yes"},
				{match: "template has been created"},
			}
			for _, m := range matches {
				pty.ExpectMatch(m.match)
				if m.write != "" {
					pty.WriteLine(m.write)
				}
			}

			waiter.RequireSuccess()

			template, err := client.TemplateByName(context.Background(), owner.OrganizationID, templateName)
			require.NoError(t, err)
			require.Equal(t, templateName, template.Name)
			require.NotEqual(t, uuid.Nil, template.ActiveVersionID)
		})
	})
}

func createEchoResponsesWithTemplateVariables(templateVariables []*proto.TemplateVariable) *echo.Responses {
	return &echo.Responses{
		Parse: []*proto.Response{
			{
				Type: &proto.Response_Parse{
					Parse: &proto.ParseComplete{
						TemplateVariables: templateVariables,
					},
				},
			},
		},
		ProvisionPlan:  echo.PlanComplete,
		ProvisionApply: echo.ApplyComplete,
	}
}
