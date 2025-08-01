package cli_test

import (
	"context"
	"fmt"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/cli/clitest"
	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/coderd/util/ptr"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/provisioner/echo"
	"github.com/coder/coder/v2/provisionersdk/proto"
	"github.com/coder/coder/v2/pty/ptytest"
	"github.com/coder/coder/v2/testutil"
)

func TestUpdate(t *testing.T) {
	t.Parallel()

	// Test that the function does not panic on missing arg.
	t.Run("NoArgs", func(t *testing.T) {
		t.Parallel()

		inv, _ := clitest.New(t, "update")
		err := inv.Run()
		require.Error(t, err)
	})

	t.Run("OK", func(t *testing.T) {
		t.Parallel()

		// Given: a workspace exists on the latest template version.
		client := coderdtest.New(t, &coderdtest.Options{IncludeProvisionerDaemon: true})
		owner := coderdtest.CreateFirstUser(t, client)
		member, _ := coderdtest.CreateAnotherUser(t, client, owner.OrganizationID)
		version1 := coderdtest.CreateTemplateVersion(t, client, owner.OrganizationID, nil)

		coderdtest.AwaitTemplateVersionJobCompleted(t, client, version1.ID)
		template := coderdtest.CreateTemplate(t, client, owner.OrganizationID, version1.ID)

		ws := coderdtest.CreateWorkspace(t, member, template.ID, func(cwr *codersdk.CreateWorkspaceRequest) {
			cwr.Name = "my-workspace"
		})
		require.False(t, ws.Outdated, "newly created workspace with active template version must not be outdated")

		// Given: the template version is updated
		version2 := coderdtest.UpdateTemplateVersion(t, client, owner.OrganizationID, &echo.Responses{
			Parse:          echo.ParseComplete,
			ProvisionApply: echo.ApplyComplete,
			ProvisionPlan:  echo.PlanComplete,
		}, template.ID)
		_ = coderdtest.AwaitTemplateVersionJobCompleted(t, client, version2.ID)

		ctx := testutil.Context(t, testutil.WaitShort)
		err := client.UpdateActiveTemplateVersion(ctx, template.ID, codersdk.UpdateActiveTemplateVersion{
			ID: version2.ID,
		})
		require.NoError(t, err, "failed to update active template version")

		// Then: the workspace is marked as 'outdated'
		ws, err = member.WorkspaceByOwnerAndName(ctx, codersdk.Me, "my-workspace", codersdk.WorkspaceOptions{})
		require.NoError(t, err, "member failed to get workspace they themselves own")
		require.True(t, ws.Outdated, "workspace must be outdated after template version update")

		// When: the workspace is updated
		inv, root := clitest.New(t, "update", ws.Name)
		clitest.SetupConfig(t, member, root)

		err = inv.Run()
		require.NoError(t, err, "update command failed")

		// Then: the workspace is no longer 'outdated'
		ws, err = member.WorkspaceByOwnerAndName(ctx, codersdk.Me, "my-workspace", codersdk.WorkspaceOptions{})
		require.NoError(t, err, "member failed to get workspace they themselves own after update")
		require.Equal(t, version2.ID.String(), ws.LatestBuild.TemplateVersionID.String(), "workspace must have latest template version after update")
		require.False(t, ws.Outdated, "workspace must not be outdated after update")

		// Then: the workspace must have been started with the new template version
		require.Equal(t, int32(3), ws.LatestBuild.BuildNumber, "workspace must have 3 builds after update")
		require.Equal(t, codersdk.WorkspaceTransitionStart, ws.LatestBuild.Transition, "latest build must be a start transition")

		// Then: the previous workspace build must be a stop transition with the old
		// template version.
		// This is important to ensure that the workspace resources are recreated
		// correctly. Simply running a start transition with the new template
		// version may not recreate resources that were changed in the new
		// template version. This can happen, for example, if a user specifies
		// ignore_changes in the template.
		prevBuild, err := member.WorkspaceBuildByUsernameAndWorkspaceNameAndBuildNumber(ctx, codersdk.Me, ws.Name, "2")
		require.NoError(t, err, "failed to get previous workspace build")
		require.Equal(t, codersdk.WorkspaceTransitionStop, prevBuild.Transition, "previous build must be a stop transition")
		require.Equal(t, version1.ID.String(), prevBuild.TemplateVersionID.String(), "previous build must have the old template version")
	})

	t.Run("Stopped", func(t *testing.T) {
		t.Parallel()

		// Given: a workspace exists on the latest template version.
		client := coderdtest.New(t, &coderdtest.Options{IncludeProvisionerDaemon: true})
		owner := coderdtest.CreateFirstUser(t, client)
		member, _ := coderdtest.CreateAnotherUser(t, client, owner.OrganizationID)
		version1 := coderdtest.CreateTemplateVersion(t, client, owner.OrganizationID, nil)

		coderdtest.AwaitTemplateVersionJobCompleted(t, client, version1.ID)
		template := coderdtest.CreateTemplate(t, client, owner.OrganizationID, version1.ID)

		ws := coderdtest.CreateWorkspace(t, member, template.ID, func(cwr *codersdk.CreateWorkspaceRequest) {
			cwr.Name = "my-workspace"
		})
		require.False(t, ws.Outdated, "newly created workspace with active template version must not be outdated")

		// Given: the template version is updated
		version2 := coderdtest.UpdateTemplateVersion(t, client, owner.OrganizationID, &echo.Responses{
			Parse:          echo.ParseComplete,
			ProvisionApply: echo.ApplyComplete,
			ProvisionPlan:  echo.PlanComplete,
		}, template.ID)
		_ = coderdtest.AwaitTemplateVersionJobCompleted(t, client, version2.ID)

		ctx := testutil.Context(t, testutil.WaitShort)
		err := client.UpdateActiveTemplateVersion(ctx, template.ID, codersdk.UpdateActiveTemplateVersion{
			ID: version2.ID,
		})
		require.NoError(t, err, "failed to update active template version")

		// Given: the workspace is in a stopped state.
		coderdtest.MustTransitionWorkspace(t, member, ws.ID, codersdk.WorkspaceTransitionStart, codersdk.WorkspaceTransitionStop)

		// Then: the workspace is marked as 'outdated'
		ws, err = member.WorkspaceByOwnerAndName(ctx, codersdk.Me, "my-workspace", codersdk.WorkspaceOptions{})
		require.NoError(t, err, "member failed to get workspace they themselves own")
		require.True(t, ws.Outdated, "workspace must be outdated after template version update")

		// When: the workspace is updated
		inv, root := clitest.New(t, "update", ws.Name)
		clitest.SetupConfig(t, member, root)

		err = inv.Run()
		require.NoError(t, err, "update command failed")

		// Then: the workspace is no longer 'outdated'
		ws, err = member.WorkspaceByOwnerAndName(ctx, codersdk.Me, "my-workspace", codersdk.WorkspaceOptions{})
		require.NoError(t, err, "member failed to get workspace they themselves own after update")
		require.Equal(t, version2.ID.String(), ws.LatestBuild.TemplateVersionID.String(), "workspace must have latest template version after update")
		require.False(t, ws.Outdated, "workspace must not be outdated after update")

		// Then: the workspace must have been started with the new template version
		require.Equal(t, codersdk.WorkspaceTransitionStart, ws.LatestBuild.Transition, "latest build must be a start transition")
		// Then: we expect 3 builds, as we manually stopped the workspace.
		require.Equal(t, int32(3), ws.LatestBuild.BuildNumber, "workspace must have 3 builds after update")
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

		ephemeralParameterName        = "ephemeral_parameter"
		ephemeralParameterDescription = "This is ephemeral parameter"
		ephemeralParameterValue       = "3"

		immutableParameterName        = "immutable_parameter"
		immutableParameterDescription = "This is not mutable parameter"
		immutableParameterValue       = "4"
	)

	echoResponses := func() *echo.Responses {
		return prepareEchoResponses([]*proto.RichParameter{
			{Name: firstParameterName, Description: firstParameterDescription, Mutable: true},
			{Name: immutableParameterName, Description: immutableParameterDescription, Mutable: false},
			{Name: secondParameterName, Description: secondParameterDescription, Mutable: true},
			{Name: ephemeralParameterName, Description: ephemeralParameterDescription, Mutable: true, Ephemeral: true, DefaultValue: "unset"},
		})
	}

	t.Run("ImmutableCannotBeCustomized", func(t *testing.T) {
		t.Parallel()

		client := coderdtest.New(t, &coderdtest.Options{IncludeProvisionerDaemon: true})
		owner := coderdtest.CreateFirstUser(t, client)
		member, _ := coderdtest.CreateAnotherUser(t, client, owner.OrganizationID)
		version := coderdtest.CreateTemplateVersion(t, client, owner.OrganizationID, echoResponses())
		coderdtest.AwaitTemplateVersionJobCompleted(t, client, version.ID)

		template := coderdtest.CreateTemplate(t, client, owner.OrganizationID, version.ID)

		tempDir := t.TempDir()
		removeTmpDirUntilSuccessAfterTest(t, tempDir)
		parameterFile, _ := os.CreateTemp(tempDir, "testParameterFile*.yaml")
		_, _ = parameterFile.WriteString(
			firstParameterName + ": " + firstParameterValue + "\n" +
				immutableParameterName + ": " + immutableParameterValue + "\n" +
				secondParameterName + ": " + secondParameterValue)

		inv, root := clitest.New(t, "create", "my-workspace", "--template", template.Name, "--rich-parameter-file", parameterFile.Name(), "-y")
		clitest.SetupConfig(t, member, root)
		err := inv.Run()
		assert.NoError(t, err)

		inv, root = clitest.New(t, "update", "my-workspace", "--always-prompt")
		clitest.SetupConfig(t, member, root)

		doneChan := make(chan struct{})
		pty := ptytest.New(t).Attach(inv)
		go func() {
			defer close(doneChan)
			err := inv.Run()
			assert.NoError(t, err)
		}()

		matches := []string{
			firstParameterDescription, firstParameterValue,
			fmt.Sprintf("Parameter %q is not mutable, and cannot be customized after workspace creation.", immutableParameterName), "",
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

	t.Run("PromptEphemeralParameters", func(t *testing.T) {
		t.Parallel()

		client := coderdtest.New(t, &coderdtest.Options{IncludeProvisionerDaemon: true})
		owner := coderdtest.CreateFirstUser(t, client)
		member, memberUser := coderdtest.CreateAnotherUser(t, client, owner.OrganizationID)
		version := coderdtest.CreateTemplateVersion(t, client, owner.OrganizationID, echoResponses())
		coderdtest.AwaitTemplateVersionJobCompleted(t, client, version.ID)

		template := coderdtest.CreateTemplate(t, client, owner.OrganizationID, version.ID)

		tempDir := t.TempDir()
		removeTmpDirUntilSuccessAfterTest(t, tempDir)
		parameterFile, _ := os.CreateTemp(tempDir, "testParameterFile*.yaml")
		_, _ = parameterFile.WriteString(
			firstParameterName + ": " + firstParameterValue + "\n" +
				immutableParameterName + ": " + immutableParameterValue + "\n" +
				secondParameterName + ": " + secondParameterValue)

		const workspaceName = "my-workspace"

		inv, root := clitest.New(t, "create", workspaceName, "--template", template.Name, "--rich-parameter-file", parameterFile.Name(), "-y")
		clitest.SetupConfig(t, member, root)
		err := inv.Run()
		assert.NoError(t, err)

		inv, root = clitest.New(t, "update", workspaceName, "--prompt-ephemeral-parameters")
		clitest.SetupConfig(t, member, root)

		doneChan := make(chan struct{})
		pty := ptytest.New(t).Attach(inv)
		go func() {
			defer close(doneChan)
			err := inv.Run()
			assert.NoError(t, err)
		}()

		matches := []string{
			ephemeralParameterDescription, ephemeralParameterValue,
			"Planning workspace", "",
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

		// Verify if ephemeral parameter is set
		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitShort)
		defer cancel()

		workspace, err := client.WorkspaceByOwnerAndName(ctx, memberUser.ID.String(), workspaceName, codersdk.WorkspaceOptions{})
		require.NoError(t, err)
		actualParameters, err := client.WorkspaceBuildParameters(ctx, workspace.LatestBuild.ID)
		require.NoError(t, err)
		require.Contains(t, actualParameters, codersdk.WorkspaceBuildParameter{
			Name:  ephemeralParameterName,
			Value: ephemeralParameterValue,
		})
	})

	t.Run("EphemeralParameterFlags", func(t *testing.T) {
		t.Parallel()

		client := coderdtest.New(t, &coderdtest.Options{IncludeProvisionerDaemon: true})
		owner := coderdtest.CreateFirstUser(t, client)
		member, memberUser := coderdtest.CreateAnotherUser(t, client, owner.OrganizationID)
		version := coderdtest.CreateTemplateVersion(t, client, owner.OrganizationID, echoResponses())
		coderdtest.AwaitTemplateVersionJobCompleted(t, client, version.ID)

		template := coderdtest.CreateTemplate(t, client, owner.OrganizationID, version.ID)

		const workspaceName = "my-workspace"

		inv, root := clitest.New(t, "create", workspaceName, "--template", template.Name, "-y",
			"--parameter", fmt.Sprintf("%s=%s", firstParameterName, firstParameterValue),
			"--parameter", fmt.Sprintf("%s=%s", immutableParameterName, immutableParameterValue),
			"--parameter", fmt.Sprintf("%s=%s", secondParameterName, secondParameterValue))
		clitest.SetupConfig(t, member, root)
		err := inv.Run()
		assert.NoError(t, err)

		inv, root = clitest.New(t, "update", workspaceName,
			"--ephemeral-parameter", fmt.Sprintf("%s=%s", ephemeralParameterName, ephemeralParameterValue))
		clitest.SetupConfig(t, member, root)

		doneChan := make(chan struct{})
		pty := ptytest.New(t).Attach(inv)
		go func() {
			defer close(doneChan)
			err := inv.Run()
			assert.NoError(t, err)
		}()

		pty.ExpectMatch("Planning workspace")
		<-doneChan

		// Verify if ephemeral parameter is set
		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitShort)
		defer cancel()

		workspace, err := client.WorkspaceByOwnerAndName(ctx, memberUser.ID.String(), workspaceName, codersdk.WorkspaceOptions{})
		require.NoError(t, err)
		actualParameters, err := client.WorkspaceBuildParameters(ctx, workspace.LatestBuild.ID)
		require.NoError(t, err)
		require.Contains(t, actualParameters, codersdk.WorkspaceBuildParameter{
			Name:  ephemeralParameterName,
			Value: ephemeralParameterValue,
		})
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
		{Name: numberParameterName, Type: "number", Mutable: true, ValidationMin: ptr.Ref(int32(3)), ValidationMax: ptr.Ref(int32(10))},
	}

	stringRichParameters := []*proto.RichParameter{
		{Name: stringParameterName, Type: "string", Mutable: true, ValidationRegex: "^[a-z]+$", ValidationError: "this is error"},
	}

	boolRichParameters := []*proto.RichParameter{
		{Name: boolParameterName, Type: "bool", Mutable: true},
	}

	t.Run("ValidateString", func(t *testing.T) {
		t.Parallel()

		client := coderdtest.New(t, &coderdtest.Options{IncludeProvisionerDaemon: true})
		owner := coderdtest.CreateFirstUser(t, client)
		member, _ := coderdtest.CreateAnotherUser(t, client, owner.OrganizationID)
		version := coderdtest.CreateTemplateVersion(t, client, owner.OrganizationID, prepareEchoResponses(stringRichParameters))
		coderdtest.AwaitTemplateVersionJobCompleted(t, client, version.ID)
		template := coderdtest.CreateTemplate(t, client, owner.OrganizationID, version.ID)

		tempDir := t.TempDir()
		removeTmpDirUntilSuccessAfterTest(t, tempDir)
		parameterFile, _ := os.CreateTemp(tempDir, "testParameterFile*.yaml")
		_, _ = parameterFile.WriteString(
			stringParameterName + ": " + stringParameterValue)

		inv, root := clitest.New(t, "create", "my-workspace", "--template", template.Name, "--rich-parameter-file", parameterFile.Name(), "-y")
		clitest.SetupConfig(t, member, root)
		err := inv.Run()
		require.NoError(t, err)

		ctx := testutil.Context(t, testutil.WaitLong)
		inv, root = clitest.New(t, "update", "my-workspace", "--always-prompt")
		inv = inv.WithContext(ctx)
		clitest.SetupConfig(t, member, root)
		doneChan := make(chan struct{})
		pty := ptytest.New(t).Attach(inv)
		go func() {
			defer close(doneChan)
			err := inv.Run()
			assert.NoError(t, err)
		}()

		pty.ExpectMatch(stringParameterName)
		pty.ExpectMatch("> Enter a value (default: \"\"): ")
		pty.WriteLine("$$")
		pty.ExpectMatch("does not match")
		pty.ExpectMatch("> Enter a value (default: \"\"): ")
		pty.WriteLine("")
		pty.ExpectMatch("does not match")
		pty.ExpectMatch("> Enter a value (default: \"\"): ")
		pty.WriteLine("abc")
		_ = testutil.TryReceive(ctx, t, doneChan)
	})

	t.Run("ValidateNumber", func(t *testing.T) {
		t.Parallel()

		client := coderdtest.New(t, &coderdtest.Options{IncludeProvisionerDaemon: true})
		owner := coderdtest.CreateFirstUser(t, client)
		member, _ := coderdtest.CreateAnotherUser(t, client, owner.OrganizationID)
		version := coderdtest.CreateTemplateVersion(t, client, owner.OrganizationID, prepareEchoResponses(numberRichParameters))
		coderdtest.AwaitTemplateVersionJobCompleted(t, client, version.ID)

		template := coderdtest.CreateTemplate(t, client, owner.OrganizationID, version.ID)

		tempDir := t.TempDir()
		removeTmpDirUntilSuccessAfterTest(t, tempDir)
		parameterFile, _ := os.CreateTemp(tempDir, "testParameterFile*.yaml")
		_, _ = parameterFile.WriteString(
			numberParameterName + ": " + numberParameterValue)

		inv, root := clitest.New(t, "create", "my-workspace", "--template", template.Name, "--rich-parameter-file", parameterFile.Name(), "-y")
		clitest.SetupConfig(t, member, root)
		err := inv.Run()
		require.NoError(t, err)

		ctx := testutil.Context(t, testutil.WaitLong)
		inv, root = clitest.New(t, "update", "my-workspace", "--always-prompt")
		inv.WithContext(ctx)
		clitest.SetupConfig(t, member, root)
		doneChan := make(chan struct{})
		pty := ptytest.New(t).Attach(inv)
		go func() {
			defer close(doneChan)
			err := inv.Run()
			assert.NoError(t, err)
		}()

		pty.ExpectMatch(numberParameterName)
		pty.ExpectMatch("> Enter a value (default: \"\"): ")
		pty.WriteLine("12")
		pty.ExpectMatch("is more than the maximum")
		pty.ExpectMatch("> Enter a value (default: \"\"): ")
		pty.WriteLine("")
		pty.ExpectMatch("is not a number")
		pty.ExpectMatch("> Enter a value (default: \"\"): ")
		pty.WriteLine("8")
		_ = testutil.TryReceive(ctx, t, doneChan)
	})

	t.Run("ValidateBool", func(t *testing.T) {
		t.Parallel()

		client := coderdtest.New(t, &coderdtest.Options{IncludeProvisionerDaemon: true})
		owner := coderdtest.CreateFirstUser(t, client)
		member, _ := coderdtest.CreateAnotherUser(t, client, owner.OrganizationID)
		version := coderdtest.CreateTemplateVersion(t, client, owner.OrganizationID, prepareEchoResponses(boolRichParameters))
		coderdtest.AwaitTemplateVersionJobCompleted(t, client, version.ID)

		template := coderdtest.CreateTemplate(t, client, owner.OrganizationID, version.ID)

		tempDir := t.TempDir()
		removeTmpDirUntilSuccessAfterTest(t, tempDir)
		parameterFile, _ := os.CreateTemp(tempDir, "testParameterFile*.yaml")
		_, _ = parameterFile.WriteString(
			boolParameterName + ": " + boolParameterValue)

		inv, root := clitest.New(t, "create", "my-workspace", "--template", template.Name, "--rich-parameter-file", parameterFile.Name(), "-y")
		clitest.SetupConfig(t, member, root)
		err := inv.Run()
		require.NoError(t, err)

		ctx := testutil.Context(t, testutil.WaitLong)
		inv, root = clitest.New(t, "update", "my-workspace", "--always-prompt")
		inv = inv.WithContext(ctx)
		clitest.SetupConfig(t, member, root)
		doneChan := make(chan struct{})
		pty := ptytest.New(t).Attach(inv)
		go func() {
			defer close(doneChan)
			err := inv.Run()
			assert.NoError(t, err)
		}()

		pty.ExpectMatch(boolParameterName)
		pty.ExpectMatch("> Enter a value (default: \"\"): ")
		pty.WriteLine("cat")
		pty.ExpectMatch("boolean value can be either \"true\" or \"false\"")
		pty.ExpectMatch("> Enter a value (default: \"\"): ")
		pty.WriteLine("")
		pty.ExpectMatch("boolean value can be either \"true\" or \"false\"")
		pty.ExpectMatch("> Enter a value (default: \"\"): ")
		pty.WriteLine("false")
		_ = testutil.TryReceive(ctx, t, doneChan)
	})

	t.Run("RequiredParameterAdded", func(t *testing.T) {
		t.Parallel()

		client := coderdtest.New(t, &coderdtest.Options{IncludeProvisionerDaemon: true})
		owner := coderdtest.CreateFirstUser(t, client)
		member, _ := coderdtest.CreateAnotherUser(t, client, owner.OrganizationID)

		// Upload the initial template
		version := coderdtest.CreateTemplateVersion(t, client, owner.OrganizationID, prepareEchoResponses(stringRichParameters))
		coderdtest.AwaitTemplateVersionJobCompleted(t, client, version.ID)
		template := coderdtest.CreateTemplate(t, client, owner.OrganizationID, version.ID)

		tempDir := t.TempDir()
		removeTmpDirUntilSuccessAfterTest(t, tempDir)
		parameterFile, _ := os.CreateTemp(tempDir, "testParameterFile*.yaml")
		_, _ = parameterFile.WriteString(
			stringParameterName + ": " + stringParameterValue)

		// Create workspace
		inv, root := clitest.New(t, "create", "my-workspace", "--template", template.Name, "--rich-parameter-file", parameterFile.Name(), "-y")
		clitest.SetupConfig(t, member, root)
		err := inv.Run()
		require.NoError(t, err)

		// Modify template
		const addedParameterName = "added_parameter"

		var modifiedParameters []*proto.RichParameter
		modifiedParameters = append(modifiedParameters, stringRichParameters...)
		modifiedParameters = append(modifiedParameters, &proto.RichParameter{
			Name:     addedParameterName,
			Type:     "string",
			Mutable:  true,
			Required: true,
		})
		version = coderdtest.UpdateTemplateVersion(t, client, owner.OrganizationID, prepareEchoResponses(modifiedParameters), template.ID)
		coderdtest.AwaitTemplateVersionJobCompleted(t, client, version.ID)
		err = client.UpdateActiveTemplateVersion(context.Background(), template.ID, codersdk.UpdateActiveTemplateVersion{
			ID: version.ID,
		})
		require.NoError(t, err)

		// Update the workspace
		ctx := testutil.Context(t, testutil.WaitLong)
		inv, root = clitest.New(t, "update", "my-workspace")
		inv.WithContext(ctx)
		clitest.SetupConfig(t, member, root)
		doneChan := make(chan struct{})
		pty := ptytest.New(t).Attach(inv)
		go func() {
			defer close(doneChan)
			err := inv.Run()
			assert.NoError(t, err)
		}()

		matches := []string{
			"added_parameter", "",
			"Enter a value:", "abc",
		}
		for i := 0; i < len(matches); i += 2 {
			match := matches[i]
			value := matches[i+1]
			pty.ExpectMatch(match)

			if value != "" {
				pty.WriteLine(value)
			}
		}
		_ = testutil.TryReceive(ctx, t, doneChan)
	})

	t.Run("OptionalParameterAdded", func(t *testing.T) {
		t.Parallel()

		client := coderdtest.New(t, &coderdtest.Options{IncludeProvisionerDaemon: true})
		owner := coderdtest.CreateFirstUser(t, client)
		member, _ := coderdtest.CreateAnotherUser(t, client, owner.OrganizationID)

		// Upload the initial template
		version := coderdtest.CreateTemplateVersion(t, client, owner.OrganizationID, prepareEchoResponses(stringRichParameters))
		coderdtest.AwaitTemplateVersionJobCompleted(t, client, version.ID)
		template := coderdtest.CreateTemplate(t, client, owner.OrganizationID, version.ID)

		tempDir := t.TempDir()
		removeTmpDirUntilSuccessAfterTest(t, tempDir)
		parameterFile, _ := os.CreateTemp(tempDir, "testParameterFile*.yaml")
		_, _ = parameterFile.WriteString(
			stringParameterName + ": " + stringParameterValue)

		// Create workspace
		inv, root := clitest.New(t, "create", "my-workspace", "--template", template.Name, "--rich-parameter-file", parameterFile.Name(), "-y")
		clitest.SetupConfig(t, member, root)
		err := inv.Run()
		require.NoError(t, err)

		// Modify template
		const addedParameterName = "added_parameter"

		var modifiedParameters []*proto.RichParameter
		modifiedParameters = append(modifiedParameters, stringRichParameters...)
		modifiedParameters = append(modifiedParameters, &proto.RichParameter{
			Name:         addedParameterName,
			Type:         "string",
			Mutable:      true,
			DefaultValue: "foobar",
			Required:     false,
		})
		version = coderdtest.UpdateTemplateVersion(t, client, owner.OrganizationID, prepareEchoResponses(modifiedParameters), template.ID)
		coderdtest.AwaitTemplateVersionJobCompleted(t, client, version.ID)
		err = client.UpdateActiveTemplateVersion(context.Background(), template.ID, codersdk.UpdateActiveTemplateVersion{
			ID: version.ID,
		})
		require.NoError(t, err)

		// Update the workspace
		ctx := testutil.Context(t, testutil.WaitLong)
		inv, root = clitest.New(t, "update", "my-workspace")
		inv.WithContext(ctx)
		clitest.SetupConfig(t, member, root)
		doneChan := make(chan struct{})
		pty := ptytest.New(t).Attach(inv)
		go func() {
			defer close(doneChan)
			err := inv.Run()
			assert.NoError(t, err)
		}()

		pty.ExpectMatch("Planning workspace...")
		_ = testutil.TryReceive(ctx, t, doneChan)
	})

	t.Run("ParameterOptionChanged", func(t *testing.T) {
		t.Parallel()

		// Create template and workspace
		client := coderdtest.New(t, &coderdtest.Options{IncludeProvisionerDaemon: true})
		user := coderdtest.CreateFirstUser(t, client)
		member, _ := coderdtest.CreateAnotherUser(t, client, user.OrganizationID)

		templateParameters := []*proto.RichParameter{
			{Name: stringParameterName, Type: "string", Mutable: true, Required: true, Options: []*proto.RichParameterOption{
				{Name: "First option", Description: "This is first option", Value: "1st"},
				{Name: "Second option", Description: "This is second option", Value: "2nd"},
				{Name: "Third option", Description: "This is third option", Value: "3rd"},
			}},
		}
		version := coderdtest.CreateTemplateVersion(t, client, user.OrganizationID, prepareEchoResponses(templateParameters))
		coderdtest.AwaitTemplateVersionJobCompleted(t, client, version.ID)
		template := coderdtest.CreateTemplate(t, client, user.OrganizationID, version.ID)

		// Create new workspace
		inv, root := clitest.New(t, "create", "my-workspace", "--yes", "--template", template.Name, "--parameter", fmt.Sprintf("%s=%s", stringParameterName, "2nd"))
		clitest.SetupConfig(t, member, root)
		err := inv.Run()
		require.NoError(t, err)

		// Update template
		updatedTemplateParameters := []*proto.RichParameter{
			// The order of rich parameter options must be maintained because `cliui.Select` automatically selects the first option during tests.
			{Name: stringParameterName, Type: "string", Mutable: true, Required: true, Options: []*proto.RichParameterOption{
				{Name: "first_option", Description: "This is first option", Value: "1"},
				{Name: "second_option", Description: "This is second option", Value: "2"},
				{Name: "third_option", Description: "This is third option", Value: "3"},
			}},
		}

		updatedVersion := coderdtest.UpdateTemplateVersion(t, client, user.OrganizationID, prepareEchoResponses(updatedTemplateParameters), template.ID)
		coderdtest.AwaitTemplateVersionJobCompleted(t, client, updatedVersion.ID)
		err = client.UpdateActiveTemplateVersion(context.Background(), template.ID, codersdk.UpdateActiveTemplateVersion{
			ID: updatedVersion.ID,
		})
		require.NoError(t, err)

		// Update the workspace
		ctx := testutil.Context(t, testutil.WaitLong)
		inv, root = clitest.New(t, "update", "my-workspace")
		inv.WithContext(ctx)
		clitest.SetupConfig(t, member, root)
		doneChan := make(chan struct{})
		pty := ptytest.New(t).Attach(inv)
		go func() {
			defer close(doneChan)
			err := inv.Run()
			assert.NoError(t, err)
		}()

		matches := []string{
			// `cliui.Select` will automatically pick the first option
			"Planning workspace...", "",
		}
		for i := 0; i < len(matches); i += 2 {
			match := matches[i]
			value := matches[i+1]
			pty.ExpectMatch(match)

			if value != "" {
				pty.WriteLine(value)
			}
		}

		_ = testutil.TryReceive(ctx, t, doneChan)
	})

	t.Run("ParameterOptionDisappeared", func(t *testing.T) {
		t.Parallel()

		// Create template and workspace
		client := coderdtest.New(t, &coderdtest.Options{IncludeProvisionerDaemon: true})
		owner := coderdtest.CreateFirstUser(t, client)
		member, _ := coderdtest.CreateAnotherUser(t, client, owner.OrganizationID)

		templateParameters := []*proto.RichParameter{
			{Name: stringParameterName, Type: "string", Mutable: true, Required: true, Options: []*proto.RichParameterOption{
				{Name: "First option", Description: "This is first option", Value: "1st"},
				{Name: "Second option", Description: "This is second option", Value: "2nd"},
				{Name: "Third option", Description: "This is third option", Value: "3rd"},
			}},
		}
		version := coderdtest.CreateTemplateVersion(t, client, owner.OrganizationID, prepareEchoResponses(templateParameters))
		coderdtest.AwaitTemplateVersionJobCompleted(t, client, version.ID)
		template := coderdtest.CreateTemplate(t, client, owner.OrganizationID, version.ID)

		// Create new workspace
		inv, root := clitest.New(t, "create", "my-workspace", "--yes", "--template", template.Name, "--parameter", fmt.Sprintf("%s=%s", stringParameterName, "2nd"))
		clitest.SetupConfig(t, member, root)
		ptytest.New(t).Attach(inv)
		err := inv.Run()
		require.NoError(t, err)

		// Update template - 2nd option disappeared, 4th option added
		updatedTemplateParameters := []*proto.RichParameter{
			// The order of rich parameter options must be maintained because `cliui.Select` automatically selects the first option during tests.
			{Name: stringParameterName, Type: "string", Mutable: true, Required: true, Options: []*proto.RichParameterOption{
				{Name: "Third option", Description: "This is third option", Value: "3rd"},
				{Name: "First option", Description: "This is first option", Value: "1st"},
				{Name: "Fourth option", Description: "This is fourth option", Value: "4th"},
			}},
		}

		updatedVersion := coderdtest.UpdateTemplateVersion(t, client, owner.OrganizationID, prepareEchoResponses(updatedTemplateParameters), template.ID)
		coderdtest.AwaitTemplateVersionJobCompleted(t, client, updatedVersion.ID)
		err = client.UpdateActiveTemplateVersion(context.Background(), template.ID, codersdk.UpdateActiveTemplateVersion{
			ID: updatedVersion.ID,
		})
		require.NoError(t, err)

		// Update the workspace
		ctx := testutil.Context(t, testutil.WaitLong)
		inv, root = clitest.New(t, "update", "my-workspace")
		inv.WithContext(ctx)
		clitest.SetupConfig(t, member, root)
		doneChan := make(chan struct{})
		pty := ptytest.New(t).Attach(inv)
		go func() {
			defer close(doneChan)
			err := inv.Run()
			assert.NoError(t, err)
		}()

		matches := []string{
			// `cliui.Select` will automatically pick the first option
			"Planning workspace...", "",
		}
		for i := 0; i < len(matches); i += 2 {
			match := matches[i]
			value := matches[i+1]
			pty.ExpectMatch(match)

			if value != "" {
				pty.WriteLine(value)
			}
		}

		_ = testutil.TryReceive(ctx, t, doneChan)
	})

	t.Run("ParameterOptionFailsMonotonicValidation", func(t *testing.T) {
		t.Parallel()

		// Create template and workspace
		client := coderdtest.New(t, &coderdtest.Options{IncludeProvisionerDaemon: true})
		owner := coderdtest.CreateFirstUser(t, client)
		member, _ := coderdtest.CreateAnotherUser(t, client, owner.OrganizationID)

		const tempVal = "2"

		templateParameters := []*proto.RichParameter{
			{Name: numberParameterName, Type: "number", Mutable: true, Required: true, Options: []*proto.RichParameterOption{
				{Name: "First option", Description: "This is first option", Value: "1"},
				{Name: "Second option", Description: "This is second option", Value: tempVal},
				{Name: "Third option", Description: "This is third option", Value: "3"},
			}, ValidationMonotonic: string(codersdk.MonotonicOrderIncreasing)},
		}
		version := coderdtest.CreateTemplateVersion(t, client, owner.OrganizationID, prepareEchoResponses(templateParameters))
		coderdtest.AwaitTemplateVersionJobCompleted(t, client, version.ID)
		template := coderdtest.CreateTemplate(t, client, owner.OrganizationID, version.ID, func(request *codersdk.CreateTemplateRequest) {
			request.UseClassicParameterFlow = ptr.Ref(true) // TODO: Remove when dynamic parameters can pass this test
		})

		// Create new workspace
		inv, root := clitest.New(t, "create", "my-workspace", "--yes", "--template", template.Name, "--parameter", fmt.Sprintf("%s=%s", numberParameterName, tempVal))
		clitest.SetupConfig(t, member, root)
		ptytest.New(t).Attach(inv)
		err := inv.Run()
		require.NoError(t, err)

		// Update the workspace
		ctx := testutil.Context(t, testutil.WaitLong)
		inv, root = clitest.New(t, "update", "my-workspace", "--always-prompt=true")
		inv.WithContext(ctx)
		clitest.SetupConfig(t, member, root)

		doneChan := make(chan struct{})
		pty := ptytest.New(t).Attach(inv)
		go func() {
			defer close(doneChan)
			err := inv.Run()
			// TODO: improve validation so we catch this problem before it reaches the server
			// 		 but for now just validate that the server actually catches invalid monotonicity
			assert.ErrorContains(t, err, "parameter value '1' must be equal or greater than previous value: 2")
		}()

		matches := []string{
			// `cliui.Select` will automatically pick the first option, which will cause the validation to fail because
			// "1" is less than "2" which was selected initially.
			numberParameterName,
		}
		for i := 0; i < len(matches); i += 2 {
			match := matches[i]
			pty.ExpectMatch(match)
		}

		_ = testutil.TryReceive(ctx, t, doneChan)
	})

	t.Run("ImmutableRequiredParameterExists_MutableRequiredParameterAdded", func(t *testing.T) {
		t.Parallel()

		// Create template and workspace
		client := coderdtest.New(t, &coderdtest.Options{IncludeProvisionerDaemon: true})
		owner := coderdtest.CreateFirstUser(t, client)
		member, _ := coderdtest.CreateAnotherUser(t, client, owner.OrganizationID)

		templateParameters := []*proto.RichParameter{
			{Name: stringParameterName, Type: "string", Mutable: false, Required: true, Options: []*proto.RichParameterOption{
				{Name: "First option", Description: "This is first option", Value: "1st"},
				{Name: "Second option", Description: "This is second option", Value: "2nd"},
				{Name: "Third option", Description: "This is third option", Value: "3rd"},
			}},
		}
		version := coderdtest.CreateTemplateVersion(t, client, owner.OrganizationID, prepareEchoResponses(templateParameters))
		coderdtest.AwaitTemplateVersionJobCompleted(t, client, version.ID)
		template := coderdtest.CreateTemplate(t, client, owner.OrganizationID, version.ID)

		inv, root := clitest.New(t, "create", "my-workspace", "--yes", "--template", template.Name, "--parameter", fmt.Sprintf("%s=%s", stringParameterName, "2nd"))
		clitest.SetupConfig(t, member, root)
		err := inv.Run()
		require.NoError(t, err)

		// Update template: add required, mutable parameter
		const mutableParameterName = "foobar"
		updatedTemplateParameters := []*proto.RichParameter{
			templateParameters[0],
			{Name: mutableParameterName, Type: "string", Mutable: true, Required: true},
		}

		updatedVersion := coderdtest.UpdateTemplateVersion(t, client, owner.OrganizationID, prepareEchoResponses(updatedTemplateParameters), template.ID)
		coderdtest.AwaitTemplateVersionJobCompleted(t, client, updatedVersion.ID)
		err = client.UpdateActiveTemplateVersion(context.Background(), template.ID, codersdk.UpdateActiveTemplateVersion{
			ID: updatedVersion.ID,
		})
		require.NoError(t, err)

		// Update the workspace
		ctx := testutil.Context(t, testutil.WaitLong)
		inv, root = clitest.New(t, "update", "my-workspace")
		inv.WithContext(ctx)
		clitest.SetupConfig(t, member, root)
		doneChan := make(chan struct{})
		pty := ptytest.New(t).Attach(inv)
		go func() {
			defer close(doneChan)
			err := inv.Run()
			assert.NoError(t, err)
		}()

		matches := []string{
			mutableParameterName, "hello",
			"Planning workspace...", "",
		}
		for i := 0; i < len(matches); i += 2 {
			match := matches[i]
			value := matches[i+1]
			pty.ExpectMatch(match)

			if value != "" {
				pty.WriteLine(value)
			}
		}

		_ = testutil.TryReceive(ctx, t, doneChan)
	})

	t.Run("MutableRequiredParameterExists_ImmutableRequiredParameterAdded", func(t *testing.T) {
		t.Parallel()

		// Create template and workspace
		client := coderdtest.New(t, &coderdtest.Options{IncludeProvisionerDaemon: true})
		owner := coderdtest.CreateFirstUser(t, client)
		member, _ := coderdtest.CreateAnotherUser(t, client, owner.OrganizationID)

		templateParameters := []*proto.RichParameter{
			{Name: stringParameterName, Type: "string", Mutable: true, Required: true, Options: []*proto.RichParameterOption{
				{Name: "First option", Description: "This is first option", Value: "1st"},
				{Name: "Second option", Description: "This is second option", Value: "2nd"},
				{Name: "Third option", Description: "This is third option", Value: "3rd"},
			}},
		}
		version := coderdtest.CreateTemplateVersion(t, client, owner.OrganizationID, prepareEchoResponses(templateParameters))
		coderdtest.AwaitTemplateVersionJobCompleted(t, client, version.ID)
		template := coderdtest.CreateTemplate(t, client, owner.OrganizationID, version.ID)

		inv, root := clitest.New(t, "create", "my-workspace", "--yes", "--template", template.Name, "--parameter", fmt.Sprintf("%s=%s", stringParameterName, "2nd"))
		clitest.SetupConfig(t, member, root)
		err := inv.Run()
		require.NoError(t, err)

		// Update template: add required, immutable parameter
		updatedTemplateParameters := []*proto.RichParameter{
			templateParameters[0],
			// The order of rich parameter options must be maintained because `cliui.Select` automatically selects the first option during tests.
			{Name: immutableParameterName, Type: "string", Mutable: false, Required: true, Options: []*proto.RichParameterOption{
				{Name: "thi", Description: "This is third option for immutable parameter", Value: "III"},
				{Name: "fir", Description: "This is first option for immutable parameter", Value: "I"},
				{Name: "sec", Description: "This is second option for immutable parameter", Value: "II"},
			}},
		}

		updatedVersion := coderdtest.UpdateTemplateVersion(t, client, owner.OrganizationID, prepareEchoResponses(updatedTemplateParameters), template.ID)
		coderdtest.AwaitTemplateVersionJobCompleted(t, client, updatedVersion.ID)
		err = client.UpdateActiveTemplateVersion(context.Background(), template.ID, codersdk.UpdateActiveTemplateVersion{
			ID: updatedVersion.ID,
		})
		require.NoError(t, err)

		// Update the workspace
		ctx := testutil.Context(t, testutil.WaitLong)
		inv, root = clitest.New(t, "update", "my-workspace")
		inv.WithContext(ctx)
		clitest.SetupConfig(t, member, root)
		doneChan := make(chan struct{})
		pty := ptytest.New(t).Attach(inv)
		go func() {
			defer close(doneChan)
			err := inv.Run()
			assert.NoError(t, err)
		}()

		matches := []string{
			// `cliui.Select` will automatically pick the first option
			"Planning workspace...", "",
		}
		for i := 0; i < len(matches); i += 2 {
			match := matches[i]
			value := matches[i+1]
			pty.ExpectMatch(match)

			if value != "" {
				pty.WriteLine(value)
			}
		}

		_ = testutil.TryReceive(ctx, t, doneChan)
	})
}
