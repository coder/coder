package cli_test

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/cli/clitest"
	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/provisioner/echo"
	"github.com/coder/coder/v2/provisionersdk/proto"
	"github.com/coder/coder/v2/pty/ptytest"
	"github.com/coder/coder/v2/testutil"
)

func completeWithAgent() *echo.Responses {
	return &echo.Responses{
		Parse: echo.ParseComplete,
		ProvisionPlan: []*proto.Response{
			{
				Type: &proto.Response_Plan{
					Plan: &proto.PlanComplete{
						Resources: []*proto.Resource{
							{
								Type: "compute",
								Name: "main",
								Agents: []*proto.Agent{
									{
										Name:            "smith",
										OperatingSystem: "linux",
										Architecture:    "i386",
									},
								},
							},
						},
					},
				},
			},
		},
		ProvisionApply: []*proto.Response{
			{
				Type: &proto.Response_Apply{
					Apply: &proto.ApplyComplete{
						Resources: []*proto.Resource{
							{
								Type: "compute",
								Name: "main",
								Agents: []*proto.Agent{
									{
										Name:            "smith",
										OperatingSystem: "linux",
										Architecture:    "i386",
									},
								},
							},
						},
					},
				},
			},
		},
	}
}

func TestTemplateCreate(t *testing.T) {
	t.Parallel()
	t.Run("Create", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, &coderdtest.Options{IncludeProvisionerDaemon: true})
		coderdtest.CreateFirstUser(t, client)
		source := clitest.CreateTemplateVersionSource(t, completeWithAgent())
		args := []string{
			"templates",
			"create",
			"my-template",
			"--directory", source,
			"--test.provisioner", string(database.ProvisionerTypeEcho),
			"--default-ttl", "24h",
		}
		inv, root := clitest.New(t, args...)
		clitest.SetupConfig(t, client, root)
		pty := ptytest.New(t).Attach(inv)

		clitest.Start(t, inv)

		matches := []struct {
			match string
			write string
		}{
			{match: "Upload", write: "yes"},
			{match: "compute.main"},
			{match: "smith (linux, i386)"},
			{match: "Confirm create?", write: "yes"},
		}
		for _, m := range matches {
			pty.ExpectMatch(m.match)
			if len(m.write) > 0 {
				pty.WriteLine(m.write)
			}
		}
	})
	t.Run("CreateNoLockfile", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, &coderdtest.Options{IncludeProvisionerDaemon: true})
		coderdtest.CreateFirstUser(t, client)
		source := clitest.CreateTemplateVersionSource(t, completeWithAgent())
		require.NoError(t, os.Remove(filepath.Join(source, ".terraform.lock.hcl")))
		args := []string{
			"templates",
			"create",
			"my-template",
			"--directory", source,
			"--test.provisioner", string(database.ProvisionerTypeEcho),
			"--default-ttl", "24h",
		}
		inv, root := clitest.New(t, args...)
		clitest.SetupConfig(t, client, root)
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
			if len(m.write) > 0 {
				pty.WriteLine(m.write)
			}
		}

		// cmd should error once we say no.
		require.Error(t, <-execDone)
	})
	t.Run("CreateNoLockfileIgnored", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, &coderdtest.Options{IncludeProvisionerDaemon: true})
		coderdtest.CreateFirstUser(t, client)
		source := clitest.CreateTemplateVersionSource(t, completeWithAgent())
		require.NoError(t, os.Remove(filepath.Join(source, ".terraform.lock.hcl")))
		args := []string{
			"templates",
			"create",
			"my-template",
			"--directory", source,
			"--test.provisioner", string(database.ProvisionerTypeEcho),
			"--default-ttl", "24h",
			"--ignore-lockfile",
		}
		inv, root := clitest.New(t, args...)
		clitest.SetupConfig(t, client, root)
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

	t.Run("CreateStdin", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, &coderdtest.Options{IncludeProvisionerDaemon: true})
		coderdtest.CreateFirstUser(t, client)
		source, err := echo.Tar(completeWithAgent())
		require.NoError(t, err)

		args := []string{
			"templates",
			"create",
			"my-template",
			"--directory", "-",
			"--test.provisioner", string(database.ProvisionerTypeEcho),
			"--default-ttl", "24h",
		}
		inv, root := clitest.New(t, args...)
		clitest.SetupConfig(t, client, root)
		pty := ptytest.New(t)
		inv.Stdin = bytes.NewReader(source)
		inv.Stdout = pty.Output()

		require.NoError(t, inv.Run())
	})

	t.Run("Recreate template with same name (create, delete, create)", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, &coderdtest.Options{IncludeProvisionerDaemon: true})
		coderdtest.CreateFirstUser(t, client)

		create := func() error {
			source := clitest.CreateTemplateVersionSource(t, completeWithAgent())
			args := []string{
				"templates",
				"create",
				"my-template",
				"--yes",
				"--directory", source,
				"--test.provisioner", string(database.ProvisionerTypeEcho),
			}
			inv, root := clitest.New(t, args...)
			clitest.SetupConfig(t, client, root)

			return inv.Run()
		}
		del := func() error {
			args := []string{
				"templates",
				"delete",
				"my-template",
				"--yes",
			}
			inv, root := clitest.New(t, args...)
			clitest.SetupConfig(t, client, root)

			return inv.Run()
		}

		err := create()
		require.NoError(t, err, "Template must be created without error")
		err = del()
		require.NoError(t, err, "Template must be deleted without error")
		err = create()
		require.NoError(t, err, "Template must be recreated without error")
	})

	t.Run("WithVariablesFileWithoutRequiredValue", func(t *testing.T) {
		t.Parallel()

		client := coderdtest.New(t, &coderdtest.Options{IncludeProvisionerDaemon: true})
		coderdtest.CreateFirstUser(t, client)

		templateVariables := []*proto.TemplateVariable{
			{
				Name:        "first_variable",
				Description: "This is the first variable.",
				Type:        "string",
				Required:    true,
				Sensitive:   true,
			},
			{
				Name:         "second_variable",
				Description:  "This is the first variable",
				Type:         "string",
				DefaultValue: "abc",
				Required:     false,
				Sensitive:    true,
			},
		}
		source := clitest.CreateTemplateVersionSource(t,
			createEchoResponsesWithTemplateVariables(templateVariables))
		tempDir := t.TempDir()
		removeTmpDirUntilSuccessAfterTest(t, tempDir)
		variablesFile, _ := os.CreateTemp(tempDir, "variables*.yaml")
		_, _ = variablesFile.WriteString(`second_variable: foobar`)

		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitShort)
		defer cancel()

		inv, root := clitest.New(t, "templates", "create", "my-template", "--directory", source, "--test.provisioner", string(database.ProvisionerTypeEcho), "--variables-file", variablesFile.Name())
		clitest.SetupConfig(t, client, root)
		inv = inv.WithContext(ctx)
		pty := ptytest.New(t).Attach(inv)

		// We expect the cli to return an error, so we have to handle it
		// ourselves.
		go func() {
			cancel()
			err := inv.Run()
			assert.Error(t, err)
		}()

		matches := []struct {
			match string
			write string
		}{
			{match: "Upload", write: "yes"},
		}
		for _, m := range matches {
			pty.ExpectMatch(m.match)
			if len(m.write) > 0 {
				pty.WriteLine(m.write)
			}
		}

		<-ctx.Done()
	})

	t.Run("WithVariablesFileWithTheRequiredValue", func(t *testing.T) {
		t.Parallel()

		client := coderdtest.New(t, &coderdtest.Options{IncludeProvisionerDaemon: true})
		coderdtest.CreateFirstUser(t, client)

		templateVariables := []*proto.TemplateVariable{
			{
				Name:        "first_variable",
				Description: "This is the first variable.",
				Type:        "string",
				Required:    true,
				Sensitive:   true,
			},
			{
				Name:         "second_variable",
				Description:  "This is the second variable",
				Type:         "string",
				DefaultValue: "abc",
				Required:     false,
				Sensitive:    true,
			},
		}
		source := clitest.CreateTemplateVersionSource(t,
			createEchoResponsesWithTemplateVariables(templateVariables))
		tempDir := t.TempDir()
		removeTmpDirUntilSuccessAfterTest(t, tempDir)
		variablesFile, _ := os.CreateTemp(tempDir, "variables*.yaml")
		_, _ = variablesFile.WriteString(`first_variable: foobar`)
		inv, root := clitest.New(t, "templates", "create", "my-template", "--directory", source, "--test.provisioner", string(database.ProvisionerTypeEcho), "--variables-file", variablesFile.Name())
		clitest.SetupConfig(t, client, root)
		pty := ptytest.New(t).Attach(inv)

		clitest.Start(t, inv)

		matches := []struct {
			match string
			write string
		}{
			{match: "Upload", write: "yes"},
			{match: "Confirm create?", write: "yes"},
		}
		for _, m := range matches {
			pty.ExpectMatch(m.match)
			if len(m.write) > 0 {
				pty.WriteLine(m.write)
			}
		}
	})
	t.Run("WithVariableOption", func(t *testing.T) {
		t.Parallel()

		client := coderdtest.New(t, &coderdtest.Options{IncludeProvisionerDaemon: true})
		coderdtest.CreateFirstUser(t, client)

		templateVariables := []*proto.TemplateVariable{
			{
				Name:        "first_variable",
				Description: "This is the first variable.",
				Type:        "string",
				Required:    true,
				Sensitive:   true,
			},
		}
		source := clitest.CreateTemplateVersionSource(t,
			createEchoResponsesWithTemplateVariables(templateVariables))
		inv, root := clitest.New(t, "templates", "create", "my-template", "--directory", source, "--test.provisioner", string(database.ProvisionerTypeEcho), "--variable", "first_variable=foobar")
		clitest.SetupConfig(t, client, root)
		pty := ptytest.New(t).Attach(inv)

		clitest.Start(t, inv)

		matches := []struct {
			match string
			write string
		}{
			{match: "Upload", write: "yes"},
			{match: "Confirm create?", write: "yes"},
		}
		for _, m := range matches {
			pty.ExpectMatch(m.match)
			if len(m.write) > 0 {
				pty.WriteLine(m.write)
			}
		}
	})

	t.Run("RequireActiveVersionInvalid", func(t *testing.T) {
		t.Parallel()

		client := coderdtest.New(t, &coderdtest.Options{
			IncludeProvisionerDaemon: true,
		})
		coderdtest.CreateFirstUser(t, client)
		source := clitest.CreateTemplateVersionSource(t, completeWithAgent())
		args := []string{
			"templates",
			"create",
			"my-template",
			"--directory", source,
			"--test.provisioner", string(database.ProvisionerTypeEcho),
			"--require-active-version",
		}
		inv, root := clitest.New(t, args...)
		clitest.SetupConfig(t, client, root)

		err := inv.Run()
		require.Error(t, err)
		require.Contains(t, err.Error(), "your deployment appears to be an AGPL deployment, so you cannot set enterprise-only flags")
	})
}

// Need this for Windows because of a known issue with Go:
// https://github.com/golang/go/issues/52986
func removeTmpDirUntilSuccessAfterTest(t *testing.T, tempDir string) {
	t.Helper()
	t.Cleanup(func() {
		err := os.RemoveAll(tempDir)
		for err != nil {
			err = os.RemoveAll(tempDir)
		}
	})
}
