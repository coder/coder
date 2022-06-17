package cli_test

import (
	"io"
	"os"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/cli/clitest"
	"github.com/coder/coder/coderd/coderdtest"
	"github.com/coder/coder/coderd/database"
	"github.com/coder/coder/provisioner/echo"
	"github.com/coder/coder/provisionersdk/proto"
	"github.com/coder/coder/pty/ptytest"
)

var provisionCompleteWithAgent = []*proto.Provision_Response{
	{
		Type: &proto.Provision_Response_Complete{
			Complete: &proto.Provision_Complete{
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
}

func TestTemplateCreate(t *testing.T) {
	t.Parallel()
	t.Run("Create", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, &coderdtest.Options{IncludeProvisionerD: true})
		coderdtest.CreateFirstUser(t, client)
		source := clitest.CreateTemplateVersionSource(t, &echo.Responses{
			Parse:     echo.ParseComplete,
			Provision: provisionCompleteWithAgent,
		})
		args := []string{
			"templates",
			"create",
			"my-template",
			"--directory", source,
			"--test.provisioner", string(database.ProvisionerTypeEcho),
			"--max-ttl", "24h",
			"--min-autostart-interval", "2h",
		}
		cmd, root := clitest.New(t, args...)
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
			{match: "Create and upload", write: "yes"},
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

		require.NoError(t, <-execDone)
	})

	t.Run("WithParameter", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, &coderdtest.Options{IncludeProvisionerD: true})
		coderdtest.CreateFirstUser(t, client)
		source := clitest.CreateTemplateVersionSource(t, &echo.Responses{
			Parse:           createTestParseResponse(),
			Provision:       echo.ProvisionComplete,
			ProvisionDryRun: echo.ProvisionComplete,
		})
		cmd, root := clitest.New(t, "templates", "create", "my-template", "--directory", source, "--test.provisioner", string(database.ProvisionerTypeEcho))
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
			{match: "Create and upload", write: "yes"},
			{match: "Enter a value:", write: "bananas"},
			{match: "Confirm create?", write: "yes"},
		}
		for _, m := range matches {
			pty.ExpectMatch(m.match)
			pty.WriteLine(m.write)
		}

		require.NoError(t, <-execDone)
	})

	t.Run("WithParameterFileContainingTheValue", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, &coderdtest.Options{IncludeProvisionerD: true})
		coderdtest.CreateFirstUser(t, client)
		source := clitest.CreateTemplateVersionSource(t, &echo.Responses{
			Parse:           createTestParseResponse(),
			Provision:       echo.ProvisionComplete,
			ProvisionDryRun: echo.ProvisionComplete,
		})
		tempDir := t.TempDir()
		parameterFile, _ := os.CreateTemp(tempDir, "testParameterFile*.yaml")
		_, _ = parameterFile.WriteString("region: \"bananas\"")
		cmd, root := clitest.New(t, "templates", "create", "my-template", "--directory", source, "--test.provisioner", string(database.ProvisionerTypeEcho), "--parameter-file", parameterFile.Name())
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
			{match: "Create and upload", write: "yes"},
			{match: "Confirm create?", write: "yes"},
		}
		for _, m := range matches {
			pty.ExpectMatch(m.match)
			pty.WriteLine(m.write)
		}

		require.NoError(t, <-execDone)
		removeTmpDirUntilSuccess(t, tempDir)
	})

	t.Run("WithParameterFileNotContainingTheValue", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, &coderdtest.Options{IncludeProvisionerD: true})
		coderdtest.CreateFirstUser(t, client)
		source := clitest.CreateTemplateVersionSource(t, &echo.Responses{
			Parse:           createTestParseResponse(),
			Provision:       echo.ProvisionComplete,
			ProvisionDryRun: echo.ProvisionComplete,
		})
		tempDir := t.TempDir()
		parameterFile, _ := os.CreateTemp(tempDir, "testParameterFile*.yaml")
		_, _ = parameterFile.WriteString("zone: \"bananas\"")
		cmd, root := clitest.New(t, "templates", "create", "my-template", "--directory", source, "--test.provisioner", string(database.ProvisionerTypeEcho), "--parameter-file", parameterFile.Name())
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
			{match: "Create and upload", write: "yes"},
		}
		for _, m := range matches {
			pty.ExpectMatch(m.match)
			pty.WriteLine(m.write)
		}

		require.EqualError(t, <-execDone, "Parameter value absent in parameter file for \"region\"!")
		removeTmpDirUntilSuccess(t, tempDir)
	})

	t.Run("Recreate template with same name (create, delete, create)", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, &coderdtest.Options{IncludeProvisionerD: true})
		coderdtest.CreateFirstUser(t, client)

		create := func() error {
			source := clitest.CreateTemplateVersionSource(t, &echo.Responses{
				Parse:     echo.ParseComplete,
				Provision: provisionCompleteWithAgent,
			})
			args := []string{
				"templates",
				"create",
				"my-template",
				"--yes",
				"--directory", source,
				"--test.provisioner", string(database.ProvisionerTypeEcho),
			}
			cmd, root := clitest.New(t, args...)
			clitest.SetupConfig(t, client, root)
			cmd.SetOut(io.Discard)
			cmd.SetErr(io.Discard)

			return cmd.Execute()
		}
		del := func() error {
			args := []string{
				"templates",
				"delete",
				"my-template",
			}
			cmd, root := clitest.New(t, args...)
			clitest.SetupConfig(t, client, root)
			cmd.SetOut(io.Discard)
			cmd.SetErr(io.Discard)

			return cmd.Execute()
		}

		err := create()
		require.NoError(t, err, "Template must be created without error")
		err = del()
		require.NoError(t, err, "Template must be deleted without error")
		err = create()
		require.NoError(t, err, "Template must be recreated without error")
	})
}

func createTestParseResponse() []*proto.Parse_Response {
	return []*proto.Parse_Response{{
		Type: &proto.Parse_Response_Complete{
			Complete: &proto.Parse_Complete{
				ParameterSchemas: []*proto.ParameterSchema{{
					AllowOverrideSource: true,
					Name:                "region",
					Description:         "description",
					DefaultDestination: &proto.ParameterDestination{
						Scheme: proto.ParameterDestination_PROVISIONER_VARIABLE,
					},
				}},
			},
		},
	}}
}

// Need this for Windows because of a known issue with Go:
// https://github.com/golang/go/issues/52986
func removeTmpDirUntilSuccess(t *testing.T, tempDir string) {
	t.Helper()
	t.Cleanup(func() {
		err := os.RemoveAll(tempDir)
		for err != nil {
			err = os.RemoveAll(tempDir)
		}
	})
}
