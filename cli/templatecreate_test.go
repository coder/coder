package cli_test

import (
	"bytes"
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
		client := coderdtest.New(t, &coderdtest.Options{IncludeProvisionerDaemon: true})
		coderdtest.CreateFirstUser(t, client)
		source := clitest.CreateTemplateVersionSource(t, &echo.Responses{
			Parse:          echo.ParseComplete,
			ProvisionApply: provisionCompleteWithAgent,
		})
		args := []string{
			"templates",
			"create",
			"my-template",
			"--directory", source,
			"--test.provisioner", string(database.ProvisionerTypeEcho),
			"--default-ttl", "24h",
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

		require.NoError(t, <-execDone)
	})

	t.Run("CreateStdin", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, &coderdtest.Options{IncludeProvisionerDaemon: true})
		coderdtest.CreateFirstUser(t, client)
		source, err := echo.Tar(&echo.Responses{
			Parse:          echo.ParseComplete,
			ProvisionApply: provisionCompleteWithAgent,
		})
		require.NoError(t, err)

		args := []string{
			"templates",
			"create",
			"my-template",
			"--directory", "-",
			"--test.provisioner", string(database.ProvisionerTypeEcho),
			"--default-ttl", "24h",
		}
		cmd, root := clitest.New(t, args...)
		clitest.SetupConfig(t, client, root)
		pty := ptytest.New(t)
		cmd.SetIn(bytes.NewReader(source))
		cmd.SetOut(pty.Output())

		execDone := make(chan error)
		go func() {
			execDone <- cmd.Execute()
		}()

		require.NoError(t, <-execDone)
	})

	t.Run("WithParameter", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, &coderdtest.Options{IncludeProvisionerDaemon: true})
		coderdtest.CreateFirstUser(t, client)
		source := clitest.CreateTemplateVersionSource(t, &echo.Responses{
			Parse:          createTestParseResponse(),
			ProvisionApply: echo.ProvisionComplete,
			ProvisionPlan:  echo.ProvisionComplete,
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
			{match: "Upload", write: "yes"},
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
		client := coderdtest.New(t, &coderdtest.Options{IncludeProvisionerDaemon: true})
		coderdtest.CreateFirstUser(t, client)
		source := clitest.CreateTemplateVersionSource(t, &echo.Responses{
			Parse:          createTestParseResponse(),
			ProvisionApply: echo.ProvisionComplete,
			ProvisionPlan:  echo.ProvisionComplete,
		})
		tempDir := t.TempDir()
		removeTmpDirUntilSuccessAfterTest(t, tempDir)
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
			{match: "Upload", write: "yes"},
			{match: "Confirm create?", write: "yes"},
		}
		for _, m := range matches {
			pty.ExpectMatch(m.match)
			pty.WriteLine(m.write)
		}

		require.NoError(t, <-execDone)
	})

	t.Run("WithParameterFileNotContainingTheValue", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, &coderdtest.Options{IncludeProvisionerDaemon: true})
		coderdtest.CreateFirstUser(t, client)
		source := clitest.CreateTemplateVersionSource(t, &echo.Responses{
			Parse:          createTestParseResponse(),
			ProvisionApply: echo.ProvisionComplete,
			ProvisionPlan:  echo.ProvisionComplete,
		})
		tempDir := t.TempDir()
		removeTmpDirUntilSuccessAfterTest(t, tempDir)
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
			{
				match: "Upload",
				write: "yes",
			},
			{
				match: "Enter a value:",
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

		require.NoError(t, <-execDone)
	})

	t.Run("Recreate template with same name (create, delete, create)", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, &coderdtest.Options{IncludeProvisionerDaemon: true})
		coderdtest.CreateFirstUser(t, client)

		create := func() error {
			source := clitest.CreateTemplateVersionSource(t, &echo.Responses{
				Parse:          echo.ParseComplete,
				ProvisionApply: provisionCompleteWithAgent,
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

			return cmd.Execute()
		}
		del := func() error {
			args := []string{
				"templates",
				"delete",
				"my-template",
				"--yes",
			}
			cmd, root := clitest.New(t, args...)
			clitest.SetupConfig(t, client, root)

			return cmd.Execute()
		}

		err := create()
		require.NoError(t, err, "Template must be created without error")
		err = del()
		require.NoError(t, err, "Template must be deleted without error")
		err = create()
		require.NoError(t, err, "Template must be recreated without error")
	})

	t.Run("WithParameterExceedingCharLimit", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, &coderdtest.Options{IncludeProvisionerDaemon: true})
		coderdtest.CreateFirstUser(t, client)
		cmd, root := clitest.New(t, "templates", "create", "1234567890123456789012345678901234567891", "--test.provisioner", string(database.ProvisionerTypeEcho))
		clitest.SetupConfig(t, client, root)

		execDone := make(chan error)
		go func() {
			execDone <- cmd.Execute()
		}()

		require.EqualError(t, <-execDone, "Template name must be less than 32 characters")
	})

	t.Run("WithVariablesFileWithoutRequiredValue", func(t *testing.T) {
		t.Parallel()

		client := coderdtest.New(t, &coderdtest.Options{IncludeProvisionerDaemon: true})
		coderdtest.CreateFirstUser(t, client)

		templateVariables := []*proto.TemplateVariable{
			{
				Name:        "first_variable",
				Description: "This is the first variable",
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
		cmd, root := clitest.New(t, "templates", "create", "my-template", "--directory", source, "--test.provisioner", string(database.ProvisionerTypeEcho), "--variables-file", variablesFile.Name())
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
			if len(m.write) > 0 {
				pty.WriteLine(m.write)
			}
		}

		require.Error(t, <-execDone)
	})

	t.Run("WithVariablesFileWithTheRequiredValue", func(t *testing.T) {
		t.Parallel()

		client := coderdtest.New(t, &coderdtest.Options{IncludeProvisionerDaemon: true})
		coderdtest.CreateFirstUser(t, client)

		templateVariables := []*proto.TemplateVariable{
			{
				Name:        "first_variable",
				Description: "This is the first variable",
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
		cmd, root := clitest.New(t, "templates", "create", "my-template", "--directory", source, "--test.provisioner", string(database.ProvisionerTypeEcho), "--variables-file", variablesFile.Name())
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
	t.Run("WithVariableOption", func(t *testing.T) {
		t.Parallel()

		client := coderdtest.New(t, &coderdtest.Options{IncludeProvisionerDaemon: true})
		coderdtest.CreateFirstUser(t, client)

		templateVariables := []*proto.TemplateVariable{
			{
				Name:        "first_variable",
				Description: "This is the first variable",
				Type:        "string",
				Required:    true,
				Sensitive:   true,
			},
		}
		source := clitest.CreateTemplateVersionSource(t,
			createEchoResponsesWithTemplateVariables(templateVariables))
		cmd, root := clitest.New(t, "templates", "create", "my-template", "--directory", source, "--test.provisioner", string(database.ProvisionerTypeEcho), "--variable", "first_variable=foobar")
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
			{match: "Confirm create?", write: "yes"},
		}
		for _, m := range matches {
			pty.ExpectMatch(m.match)
			pty.WriteLine(m.write)
		}

		require.NoError(t, <-execDone)
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
func removeTmpDirUntilSuccessAfterTest(t *testing.T, tempDir string) {
	t.Helper()
	t.Cleanup(func() {
		err := os.RemoveAll(tempDir)
		for err != nil {
			err = os.RemoveAll(tempDir)
		}
	})
}
