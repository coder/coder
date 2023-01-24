package cli_test

import (
	"context"
	"fmt"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/cli/clitest"
	"github.com/coder/coder/coderd/coderdtest"
	"github.com/coder/coder/codersdk"
	"github.com/coder/coder/provisioner/echo"
	"github.com/coder/coder/provisionersdk/proto"
	"github.com/coder/coder/pty/ptytest"
)

func TestUpdate(t *testing.T) {
	t.Parallel()

	// Test that the function does not panic on missing arg.
	t.Run("NoArgs", func(t *testing.T) {
		t.Parallel()

		cmd, _ := clitest.New(t, "update")
		err := cmd.Execute()
		require.Error(t, err)
	})

	t.Run("OK", func(t *testing.T) {
		t.Parallel()

		client := coderdtest.New(t, &coderdtest.Options{IncludeProvisionerDaemon: true})
		user := coderdtest.CreateFirstUser(t, client)
		version1 := coderdtest.CreateTemplateVersion(t, client, user.OrganizationID, nil)

		coderdtest.AwaitTemplateVersionJob(t, client, version1.ID)
		template := coderdtest.CreateTemplate(t, client, user.OrganizationID, version1.ID)

		cmd, root := clitest.New(t, "create",
			"my-workspace",
			"--template", template.Name,
			"-y",
		)
		clitest.SetupConfig(t, client, root)

		err := cmd.Execute()
		require.NoError(t, err)

		ws, err := client.WorkspaceByOwnerAndName(context.Background(), "testuser", "my-workspace", codersdk.WorkspaceOptions{})
		require.NoError(t, err)
		require.Equal(t, version1.ID.String(), ws.LatestBuild.TemplateVersionID.String())

		version2 := coderdtest.UpdateTemplateVersion(t, client, user.OrganizationID, &echo.Responses{
			Parse:          echo.ParseComplete,
			ProvisionApply: echo.ProvisionComplete,
			ProvisionPlan:  echo.ProvisionComplete,
		}, template.ID)
		_ = coderdtest.AwaitTemplateVersionJob(t, client, version2.ID)

		err = client.UpdateActiveTemplateVersion(context.Background(), template.ID, codersdk.UpdateActiveTemplateVersion{
			ID: version2.ID,
		})
		require.NoError(t, err)

		cmd, root = clitest.New(t, "update", ws.Name)
		clitest.SetupConfig(t, client, root)

		err = cmd.Execute()
		require.NoError(t, err)

		ws, err = client.WorkspaceByOwnerAndName(context.Background(), "testuser", "my-workspace", codersdk.WorkspaceOptions{})
		require.NoError(t, err)
		require.Equal(t, version2.ID.String(), ws.LatestBuild.TemplateVersionID.String())
	})

	t.Run("WithParameter", func(t *testing.T) {
		t.Parallel()

		client := coderdtest.New(t, &coderdtest.Options{IncludeProvisionerDaemon: true})
		user := coderdtest.CreateFirstUser(t, client)
		version1 := coderdtest.CreateTemplateVersion(t, client, user.OrganizationID, nil)

		coderdtest.AwaitTemplateVersionJob(t, client, version1.ID)
		template := coderdtest.CreateTemplate(t, client, user.OrganizationID, version1.ID)

		cmd, root := clitest.New(t, "create",
			"my-workspace",
			"--template", template.Name,
			"-y",
		)
		clitest.SetupConfig(t, client, root)

		err := cmd.Execute()
		require.NoError(t, err)

		ws, err := client.WorkspaceByOwnerAndName(context.Background(), "testuser", "my-workspace", codersdk.WorkspaceOptions{})
		require.NoError(t, err)
		require.Equal(t, version1.ID.String(), ws.LatestBuild.TemplateVersionID.String())

		defaultValue := "something"
		version2 := coderdtest.UpdateTemplateVersion(t, client, user.OrganizationID, &echo.Responses{
			Parse:          createTestParseResponseWithDefault(defaultValue),
			ProvisionApply: echo.ProvisionComplete,
			ProvisionPlan:  echo.ProvisionComplete,
		}, template.ID)
		coderdtest.AwaitTemplateVersionJob(t, client, version2.ID)

		err = client.UpdateActiveTemplateVersion(context.Background(), template.ID, codersdk.UpdateActiveTemplateVersion{
			ID: version2.ID,
		})
		require.NoError(t, err)

		cmd, root = clitest.New(t, "update", ws.Name)
		clitest.SetupConfig(t, client, root)

		pty := ptytest.New(t)
		cmd.SetIn(pty.Input())
		cmd.SetOut(pty.Output())

		doneChan := make(chan struct{})
		go func() {
			defer close(doneChan)
			err := cmd.Execute()
			assert.NoError(t, err)
		}()

		matches := []string{
			fmt.Sprintf("Enter a value (default: %q):", defaultValue), "bingo",
			"Enter a value:", "boingo",
		}
		for i := 0; i < len(matches); i += 2 {
			match := matches[i]
			value := matches[i+1]
			pty.ExpectMatch(match)
			pty.WriteLine(value)
		}

		<-doneChan
	})
}

func TestUpdateWithRichParameters(t *testing.T) {
	t.Parallel()

	const (
		firstParameterName        = "first_parameter"
		firstParameterDescription = "This is first parameter"
		firstParameterValue       = "1"

		secondParameterName        = "second_parameter"
		secondParameterDescription = "This is second parameter"
		secondParameterValue       = "2"

		immutableParameterName        = "immutable_parameter"
		immutableParameterDescription = "This is not mutable parameter"
		immutableParameterValue       = "3"
	)

	echoResponses := &echo.Responses{
		Parse: echo.ParseComplete,
		ProvisionPlan: []*proto.Provision_Response{
			{
				Type: &proto.Provision_Response_Complete{
					Complete: &proto.Provision_Complete{
						Parameters: []*proto.RichParameter{
							{Name: firstParameterName, Description: firstParameterDescription, Mutable: true},
							{Name: immutableParameterName, Description: immutableParameterDescription, Mutable: false},
							{Name: secondParameterName, Description: secondParameterDescription, Mutable: true},
						},
					},
				},
			}},
		ProvisionApply: []*proto.Provision_Response{{
			Type: &proto.Provision_Response_Complete{
				Complete: &proto.Provision_Complete{},
			},
		}},
	}

	t.Run("ImmutableCannotBeCustomized", func(t *testing.T) {
		t.Parallel()

		client := coderdtest.New(t, &coderdtest.Options{IncludeProvisionerDaemon: true})
		user := coderdtest.CreateFirstUser(t, client)
		version := coderdtest.CreateTemplateVersion(t, client, user.OrganizationID, echoResponses)
		coderdtest.AwaitTemplateVersionJob(t, client, version.ID)

		template := coderdtest.CreateTemplate(t, client, user.OrganizationID, version.ID)

		tempDir := t.TempDir()
		removeTmpDirUntilSuccessAfterTest(t, tempDir)
		parameterFile, _ := os.CreateTemp(tempDir, "testParameterFile*.yaml")
		_, _ = parameterFile.WriteString(
			firstParameterName + ": " + firstParameterValue + "\n" +
				immutableParameterName + ": " + immutableParameterValue + "\n" +
				secondParameterName + ": " + secondParameterValue)

		cmd, root := clitest.New(t, "create", "my-workspace", "--template", template.Name, "--rich-parameter-file", parameterFile.Name(), "-y")
		clitest.SetupConfig(t, client, root)
		err := cmd.Execute()
		assert.NoError(t, err)

		cmd, root = clitest.New(t, "update", "my-workspace", "--always-prompt")
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
			firstParameterDescription, firstParameterValue,
			fmt.Sprintf("Parameter %q is not mutable, so can't be customized after workspace creation.", immutableParameterName), "",
			secondParameterDescription, secondParameterValue,
		}
		for i := 0; i < len(matches); i += 2 {
			match := matches[i]
			value := matches[i+1]
			pty.ExpectMatch(match)
			if value != "" {
				pty.WriteLine(value)
			}
		}
		<-doneChan
	})
}

func TestUpdateValidateRichParameters(t *testing.T) {
	t.Parallel()

	const (
		stringParameterName  = "string_parameter"
		stringParameterValue = "abc"

		numberParameterName  = "number_parameter"
		numberParameterValue = "7"

		boolParameterName  = "bool_parameter"
		boolParameterValue = "true"
	)

	numberRichParameters := []*proto.RichParameter{
		{Name: numberParameterName, Type: "number", Mutable: true, ValidationMin: 3, ValidationMax: 10},
	}

	stringRichParameters := []*proto.RichParameter{
		{Name: stringParameterName, Type: "string", Mutable: true, ValidationRegex: "^[a-z]+$", ValidationError: "this is error"},
	}

	boolRichParameters := []*proto.RichParameter{
		{Name: boolParameterName, Type: "bool", Mutable: true},
	}

	prepareEchoResponses := func(richParameters []*proto.RichParameter) *echo.Responses {
		return &echo.Responses{
			Parse: echo.ParseComplete,
			ProvisionPlan: []*proto.Provision_Response{
				{
					Type: &proto.Provision_Response_Complete{
						Complete: &proto.Provision_Complete{
							Parameters: richParameters,
						},
					},
				}},
			ProvisionApply: []*proto.Provision_Response{
				{
					Type: &proto.Provision_Response_Complete{
						Complete: &proto.Provision_Complete{},
					},
				},
			},
		}
	}

	t.Run("ValidateString", func(t *testing.T) {
		t.Parallel()

		client := coderdtest.New(t, &coderdtest.Options{IncludeProvisionerDaemon: true})
		user := coderdtest.CreateFirstUser(t, client)
		version := coderdtest.CreateTemplateVersion(t, client, user.OrganizationID, prepareEchoResponses(stringRichParameters))
		coderdtest.AwaitTemplateVersionJob(t, client, version.ID)
		template := coderdtest.CreateTemplate(t, client, user.OrganizationID, version.ID)

		tempDir := t.TempDir()
		removeTmpDirUntilSuccessAfterTest(t, tempDir)
		parameterFile, _ := os.CreateTemp(tempDir, "testParameterFile*.yaml")
		_, _ = parameterFile.WriteString(
			stringParameterName + ": " + stringParameterValue)

		cmd, root := clitest.New(t, "create", "my-workspace", "--template", template.Name, "--rich-parameter-file", parameterFile.Name(), "-y")
		clitest.SetupConfig(t, client, root)
		err := cmd.Execute()
		require.NoError(t, err)

		cmd, root = clitest.New(t, "update", "my-workspace", "--always-prompt")
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
			stringParameterName, "$$",
			"does not match", "",
			"Enter a value", "abc",
		}
		for i := 0; i < len(matches); i += 2 {
			match := matches[i]
			value := matches[i+1]
			pty.ExpectMatch(match)
			pty.WriteLine(value)
		}
		<-doneChan
	})

	t.Run("ValidateNumber", func(t *testing.T) {
		t.Parallel()

		client := coderdtest.New(t, &coderdtest.Options{IncludeProvisionerDaemon: true})
		user := coderdtest.CreateFirstUser(t, client)
		version := coderdtest.CreateTemplateVersion(t, client, user.OrganizationID, prepareEchoResponses(numberRichParameters))
		coderdtest.AwaitTemplateVersionJob(t, client, version.ID)

		template := coderdtest.CreateTemplate(t, client, user.OrganizationID, version.ID)

		tempDir := t.TempDir()
		removeTmpDirUntilSuccessAfterTest(t, tempDir)
		parameterFile, _ := os.CreateTemp(tempDir, "testParameterFile*.yaml")
		_, _ = parameterFile.WriteString(
			numberParameterName + ": " + numberParameterValue)

		cmd, root := clitest.New(t, "create", "my-workspace", "--template", template.Name, "--rich-parameter-file", parameterFile.Name(), "-y")
		clitest.SetupConfig(t, client, root)
		err := cmd.Execute()
		require.NoError(t, err)

		cmd, root = clitest.New(t, "update", "my-workspace", "--always-prompt")
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
			numberParameterName, "12",
			"is more than the maximum", "",
			"Enter a value", "8",
		}
		for i := 0; i < len(matches); i += 2 {
			match := matches[i]
			value := matches[i+1]
			pty.ExpectMatch(match)

			if value != "" {
				pty.WriteLine(value)
			}
		}
		<-doneChan
	})

	t.Run("ValidateBool", func(t *testing.T) {
		t.Parallel()

		client := coderdtest.New(t, &coderdtest.Options{IncludeProvisionerDaemon: true})
		user := coderdtest.CreateFirstUser(t, client)
		version := coderdtest.CreateTemplateVersion(t, client, user.OrganizationID, prepareEchoResponses(boolRichParameters))
		coderdtest.AwaitTemplateVersionJob(t, client, version.ID)

		template := coderdtest.CreateTemplate(t, client, user.OrganizationID, version.ID)

		tempDir := t.TempDir()
		removeTmpDirUntilSuccessAfterTest(t, tempDir)
		parameterFile, _ := os.CreateTemp(tempDir, "testParameterFile*.yaml")
		_, _ = parameterFile.WriteString(
			boolParameterName + ": " + boolParameterValue)

		cmd, root := clitest.New(t, "create", "my-workspace", "--template", template.Name, "--rich-parameter-file", parameterFile.Name(), "-y")
		clitest.SetupConfig(t, client, root)
		err := cmd.Execute()
		require.NoError(t, err)

		cmd, root = clitest.New(t, "update", "my-workspace", "--always-prompt")
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
			boolParameterName, "cat",
			"boolean value can be either", "",
			"Enter a value", "false",
		}
		for i := 0; i < len(matches); i += 2 {
			match := matches[i]
			value := matches[i+1]
			pty.ExpectMatch(match)
			pty.WriteLine(value)
		}
		<-doneChan
	})
}
