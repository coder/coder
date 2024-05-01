package cli_test

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"regexp"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/cli/clitest"
	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/coderd/externalauth"
	"github.com/coder/coder/v2/coderd/util/ptr"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/provisioner/echo"
	"github.com/coder/coder/v2/provisionersdk/proto"
	"github.com/coder/coder/v2/pty/ptytest"
	"github.com/coder/coder/v2/testutil"
)

func TestCreate(t *testing.T) {
	t.Parallel()
	t.Run("Create", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, &coderdtest.Options{IncludeProvisionerDaemon: true})
		owner := coderdtest.CreateFirstUser(t, client)
		member, _ := coderdtest.CreateAnotherUser(t, client, owner.OrganizationID)
		version := coderdtest.CreateTemplateVersion(t, client, owner.OrganizationID, completeWithAgent())
		coderdtest.AwaitTemplateVersionJobCompleted(t, client, version.ID)
		template := coderdtest.CreateTemplate(t, client, owner.OrganizationID, version.ID)
		args := []string{
			"create",
			"my-workspace",
			"--template", template.Name,
			"--start-at", "9:30AM Mon-Fri US/Central",
			"--stop-after", "8h",
			"--automatic-updates", "always",
		}
		inv, root := clitest.New(t, args...)
		clitest.SetupConfig(t, member, root)
		doneChan := make(chan struct{})
		pty := ptytest.New(t).Attach(inv)
		go func() {
			defer close(doneChan)
			err := inv.Run()
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

		ws, err := member.WorkspaceByOwnerAndName(context.Background(), codersdk.Me, "my-workspace", codersdk.WorkspaceOptions{})
		if assert.NoError(t, err, "expected workspace to be created") {
			assert.Equal(t, ws.TemplateName, template.Name)
			if assert.NotNil(t, ws.AutostartSchedule) {
				assert.Equal(t, *ws.AutostartSchedule, "CRON_TZ=US/Central 30 9 * * Mon-Fri")
			}
			if assert.NotNil(t, ws.TTLMillis) {
				assert.Equal(t, *ws.TTLMillis, 8*time.Hour.Milliseconds())
			}
			assert.Equal(t, codersdk.AutomaticUpdatesAlways, ws.AutomaticUpdates)
		}
	})

	t.Run("CreateForOtherUser", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, &coderdtest.Options{IncludeProvisionerDaemon: true})
		owner := coderdtest.CreateFirstUser(t, client)
		version := coderdtest.CreateTemplateVersion(t, client, owner.OrganizationID, completeWithAgent())
		coderdtest.AwaitTemplateVersionJobCompleted(t, client, version.ID)
		template := coderdtest.CreateTemplate(t, client, owner.OrganizationID, version.ID)
		_, user := coderdtest.CreateAnotherUser(t, client, owner.OrganizationID)
		args := []string{
			"create",
			user.Username + "/their-workspace",
			"--template", template.Name,
			"--start-at", "9:30AM Mon-Fri US/Central",
			"--stop-after", "8h",
		}

		inv, root := clitest.New(t, args...)
		//nolint:gocritic // Creating a workspace for another user requires owner permissions.
		clitest.SetupConfig(t, client, root)
		doneChan := make(chan struct{})
		pty := ptytest.New(t).Attach(inv)
		go func() {
			defer close(doneChan)
			err := inv.Run()
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

		ws, err := client.WorkspaceByOwnerAndName(context.Background(), user.Username, "their-workspace", codersdk.WorkspaceOptions{})
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

	t.Run("InheritStopAfterFromTemplate", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, &coderdtest.Options{IncludeProvisionerDaemon: true})
		owner := coderdtest.CreateFirstUser(t, client)
		member, _ := coderdtest.CreateAnotherUser(t, client, owner.OrganizationID)
		version := coderdtest.CreateTemplateVersion(t, client, owner.OrganizationID, completeWithAgent())
		coderdtest.AwaitTemplateVersionJobCompleted(t, client, version.ID)
		template := coderdtest.CreateTemplate(t, client, owner.OrganizationID, version.ID, func(ctr *codersdk.CreateTemplateRequest) {
			var defaultTTLMillis int64 = 2 * 60 * 60 * 1000 // 2 hours
			ctr.DefaultTTLMillis = &defaultTTLMillis
		})
		args := []string{
			"create",
			"my-workspace",
			"--template", template.Name,
		}
		inv, root := clitest.New(t, args...)
		clitest.SetupConfig(t, member, root)
		pty := ptytest.New(t).Attach(inv)
		waiter := clitest.StartWithWaiter(t, inv)
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
		waiter.RequireSuccess()

		ws, err := member.WorkspaceByOwnerAndName(context.Background(), codersdk.Me, "my-workspace", codersdk.WorkspaceOptions{})
		require.NoError(t, err, "expected workspace to be created")
		assert.Equal(t, ws.TemplateName, template.Name)
		assert.Equal(t, *ws.TTLMillis, template.DefaultTTLMillis)
	})

	t.Run("CreateFromListWithSkip", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, &coderdtest.Options{IncludeProvisionerDaemon: true})
		user := coderdtest.CreateFirstUser(t, client)
		version := coderdtest.CreateTemplateVersion(t, client, user.OrganizationID, nil)
		coderdtest.AwaitTemplateVersionJobCompleted(t, client, version.ID)
		_ = coderdtest.CreateTemplate(t, client, user.OrganizationID, version.ID)
		inv, root := clitest.New(t, "create", "my-workspace", "-y")

		member, _ := coderdtest.CreateAnotherUser(t, client, user.OrganizationID)
		clitest.SetupConfig(t, member, root)
		cmdCtx, done := context.WithTimeout(context.Background(), testutil.WaitLong)
		go func() {
			defer done()
			err := inv.WithContext(cmdCtx).Run()
			assert.NoError(t, err)
		}()
		// No pty interaction needed since we use the -y skip prompt flag
		<-cmdCtx.Done()
		require.ErrorIs(t, cmdCtx.Err(), context.Canceled)
	})

	t.Run("FromNothing", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, &coderdtest.Options{IncludeProvisionerDaemon: true})
		owner := coderdtest.CreateFirstUser(t, client)
		member, _ := coderdtest.CreateAnotherUser(t, client, owner.OrganizationID)
		version := coderdtest.CreateTemplateVersion(t, client, owner.OrganizationID, nil)
		coderdtest.AwaitTemplateVersionJobCompleted(t, client, version.ID)
		template := coderdtest.CreateTemplate(t, client, owner.OrganizationID, version.ID)
		inv, root := clitest.New(t, "create", "")
		clitest.SetupConfig(t, member, root)
		doneChan := make(chan struct{})
		pty := ptytest.New(t).Attach(inv)
		go func() {
			defer close(doneChan)
			err := inv.Run()
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

		ws, err := member.WorkspaceByOwnerAndName(inv.Context(), codersdk.Me, "my-workspace", codersdk.WorkspaceOptions{})
		if assert.NoError(t, err, "expected workspace to be created") {
			assert.Equal(t, ws.TemplateName, template.Name)
			assert.Nil(t, ws.AutostartSchedule, "expected workspace autostart schedule to be nil")
		}
	})
}

func prepareEchoResponses(parameters []*proto.RichParameter) *echo.Responses {
	return &echo.Responses{
		Parse: echo.ParseComplete,
		ProvisionPlan: []*proto.Response{
			{
				Type: &proto.Response_Plan{
					Plan: &proto.PlanComplete{
						Parameters: parameters,
					},
				},
			},
		},
		ProvisionApply: echo.ApplyComplete,
	}
}

func TestCreateWithRichParameters(t *testing.T) {
	t.Parallel()

	const (
		firstParameterName        = "first_parameter"
		firstParameterDescription = "This is first parameter"
		firstParameterValue       = "1"

		secondParameterName        = "second_parameter"
		secondParameterDisplayName = "Second Parameter"
		secondParameterDescription = "This is second parameter"
		secondParameterValue       = "2"

		immutableParameterName        = "third_parameter"
		immutableParameterDescription = "This is not mutable parameter"
		immutableParameterValue       = "4"
	)

	echoResponses := prepareEchoResponses([]*proto.RichParameter{
		{Name: firstParameterName, Description: firstParameterDescription, Mutable: true},
		{Name: secondParameterName, DisplayName: secondParameterDisplayName, Description: secondParameterDescription, Mutable: true},
		{Name: immutableParameterName, Description: immutableParameterDescription, Mutable: false},
	},
	)

	t.Run("InputParameters", func(t *testing.T) {
		t.Parallel()

		client := coderdtest.New(t, &coderdtest.Options{IncludeProvisionerDaemon: true})
		owner := coderdtest.CreateFirstUser(t, client)
		member, _ := coderdtest.CreateAnotherUser(t, client, owner.OrganizationID)
		version := coderdtest.CreateTemplateVersion(t, client, owner.OrganizationID, echoResponses)
		coderdtest.AwaitTemplateVersionJobCompleted(t, client, version.ID)

		template := coderdtest.CreateTemplate(t, client, owner.OrganizationID, version.ID)

		inv, root := clitest.New(t, "create", "my-workspace", "--template", template.Name)
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
			secondParameterDisplayName, "",
			secondParameterDescription, secondParameterValue,
			immutableParameterDescription, immutableParameterValue,
			"Confirm create?", "yes",
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

	t.Run("ParametersDefaults", func(t *testing.T) {
		t.Parallel()

		client := coderdtest.New(t, &coderdtest.Options{IncludeProvisionerDaemon: true})
		owner := coderdtest.CreateFirstUser(t, client)
		member, _ := coderdtest.CreateAnotherUser(t, client, owner.OrganizationID)
		version := coderdtest.CreateTemplateVersion(t, client, owner.OrganizationID, echoResponses)
		coderdtest.AwaitTemplateVersionJobCompleted(t, client, version.ID)

		template := coderdtest.CreateTemplate(t, client, owner.OrganizationID, version.ID)

		inv, root := clitest.New(t, "create", "my-workspace", "--template", template.Name,
			"--parameter-default", fmt.Sprintf("%s=%s", firstParameterName, firstParameterValue),
			"--parameter-default", fmt.Sprintf("%s=%s", secondParameterName, secondParameterValue),
			"--parameter-default", fmt.Sprintf("%s=%s", immutableParameterName, immutableParameterValue))
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
			secondParameterDescription, secondParameterValue,
			immutableParameterDescription, immutableParameterValue,
		}
		for i := 0; i < len(matches); i += 2 {
			match := matches[i]
			defaultValue := matches[i+1]

			pty.ExpectMatch(match)
			pty.ExpectMatch(`Enter a value (default: "` + defaultValue + `")`)
			pty.WriteLine("")
		}
		pty.ExpectMatch("Confirm create?")
		pty.WriteLine("yes")
		<-doneChan

		// Verify that the expected default values were used.
		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitShort)
		defer cancel()

		workspaces, err := client.Workspaces(ctx, codersdk.WorkspaceFilter{
			Name: "my-workspace",
		})
		require.NoError(t, err, "can't list available workspaces")
		require.Len(t, workspaces.Workspaces, 1)

		workspaceLatestBuild := workspaces.Workspaces[0].LatestBuild
		require.Equal(t, version.ID, workspaceLatestBuild.TemplateVersionID)

		buildParameters, err := client.WorkspaceBuildParameters(ctx, workspaceLatestBuild.ID)
		require.NoError(t, err)
		require.Len(t, buildParameters, 3)
		require.Contains(t, buildParameters, codersdk.WorkspaceBuildParameter{Name: firstParameterName, Value: firstParameterValue})
		require.Contains(t, buildParameters, codersdk.WorkspaceBuildParameter{Name: secondParameterName, Value: secondParameterValue})
		require.Contains(t, buildParameters, codersdk.WorkspaceBuildParameter{Name: immutableParameterName, Value: immutableParameterValue})
	})

	t.Run("RichParametersFile", func(t *testing.T) {
		t.Parallel()

		client := coderdtest.New(t, &coderdtest.Options{IncludeProvisionerDaemon: true})
		owner := coderdtest.CreateFirstUser(t, client)
		member, _ := coderdtest.CreateAnotherUser(t, client, owner.OrganizationID)
		version := coderdtest.CreateTemplateVersion(t, client, owner.OrganizationID, echoResponses)
		coderdtest.AwaitTemplateVersionJobCompleted(t, client, version.ID)

		template := coderdtest.CreateTemplate(t, client, owner.OrganizationID, version.ID)

		tempDir := t.TempDir()
		removeTmpDirUntilSuccessAfterTest(t, tempDir)
		parameterFile, _ := os.CreateTemp(tempDir, "testParameterFile*.yaml")
		_, _ = parameterFile.WriteString(
			firstParameterName + ": " + firstParameterValue + "\n" +
				secondParameterName + ": " + secondParameterValue + "\n" +
				immutableParameterName + ": " + immutableParameterValue)
		inv, root := clitest.New(t, "create", "my-workspace", "--template", template.Name, "--rich-parameter-file", parameterFile.Name())
		clitest.SetupConfig(t, member, root)

		doneChan := make(chan struct{})
		pty := ptytest.New(t).Attach(inv)
		go func() {
			defer close(doneChan)
			err := inv.Run()
			assert.NoError(t, err)
		}()

		matches := []string{
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

	t.Run("ParameterFlags", func(t *testing.T) {
		t.Parallel()

		client := coderdtest.New(t, &coderdtest.Options{IncludeProvisionerDaemon: true})
		owner := coderdtest.CreateFirstUser(t, client)
		member, _ := coderdtest.CreateAnotherUser(t, client, owner.OrganizationID)
		version := coderdtest.CreateTemplateVersion(t, client, owner.OrganizationID, echoResponses)
		coderdtest.AwaitTemplateVersionJobCompleted(t, client, version.ID)

		template := coderdtest.CreateTemplate(t, client, owner.OrganizationID, version.ID)

		inv, root := clitest.New(t, "create", "my-workspace", "--template", template.Name,
			"--parameter", fmt.Sprintf("%s=%s", firstParameterName, firstParameterValue),
			"--parameter", fmt.Sprintf("%s=%s", secondParameterName, secondParameterValue),
			"--parameter", fmt.Sprintf("%s=%s", immutableParameterName, immutableParameterValue))
		clitest.SetupConfig(t, member, root)
		doneChan := make(chan struct{})
		pty := ptytest.New(t).Attach(inv)
		go func() {
			defer close(doneChan)
			err := inv.Run()
			assert.NoError(t, err)
		}()

		matches := []string{
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

	t.Run("WrongParameterName/DidYouMean", func(t *testing.T) {
		t.Parallel()

		client := coderdtest.New(t, &coderdtest.Options{IncludeProvisionerDaemon: true})
		owner := coderdtest.CreateFirstUser(t, client)
		member, _ := coderdtest.CreateAnotherUser(t, client, owner.OrganizationID)
		version := coderdtest.CreateTemplateVersion(t, client, owner.OrganizationID, echoResponses)
		coderdtest.AwaitTemplateVersionJobCompleted(t, client, version.ID)

		template := coderdtest.CreateTemplate(t, client, owner.OrganizationID, version.ID)

		wrongFirstParameterName := "frst-prameter"
		inv, root := clitest.New(t, "create", "my-workspace", "--template", template.Name,
			"--parameter", fmt.Sprintf("%s=%s", wrongFirstParameterName, firstParameterValue),
			"--parameter", fmt.Sprintf("%s=%s", secondParameterName, secondParameterValue),
			"--parameter", fmt.Sprintf("%s=%s", immutableParameterName, immutableParameterValue))
		clitest.SetupConfig(t, member, root)
		pty := ptytest.New(t).Attach(inv)
		inv.Stdout = pty.Output()
		inv.Stderr = pty.Output()
		err := inv.Run()
		assert.ErrorContains(t, err, "parameter \""+wrongFirstParameterName+"\" is not present in the template")
		assert.ErrorContains(t, err, "Did you mean: "+firstParameterName)
	})

	t.Run("CopyParameters", func(t *testing.T) {
		t.Parallel()

		client := coderdtest.New(t, &coderdtest.Options{IncludeProvisionerDaemon: true})
		owner := coderdtest.CreateFirstUser(t, client)
		member, _ := coderdtest.CreateAnotherUser(t, client, owner.OrganizationID)
		version := coderdtest.CreateTemplateVersion(t, client, owner.OrganizationID, echoResponses)
		coderdtest.AwaitTemplateVersionJobCompleted(t, client, version.ID)

		template := coderdtest.CreateTemplate(t, client, owner.OrganizationID, version.ID)

		// Firstly, create a regular workspace using template with parameters.
		inv, root := clitest.New(t, "create", "my-workspace", "--template", template.Name, "-y",
			"--parameter", fmt.Sprintf("%s=%s", firstParameterName, firstParameterValue),
			"--parameter", fmt.Sprintf("%s=%s", secondParameterName, secondParameterValue),
			"--parameter", fmt.Sprintf("%s=%s", immutableParameterName, immutableParameterValue))
		clitest.SetupConfig(t, member, root)
		pty := ptytest.New(t).Attach(inv)
		inv.Stdout = pty.Output()
		inv.Stderr = pty.Output()
		err := inv.Run()
		require.NoError(t, err, "can't create first workspace")

		// Secondly, create a new workspace using parameters from the previous workspace.
		const otherWorkspace = "other-workspace"

		inv, root = clitest.New(t, "create", "--copy-parameters-from", "my-workspace", otherWorkspace, "-y")
		clitest.SetupConfig(t, member, root)
		pty = ptytest.New(t).Attach(inv)
		inv.Stdout = pty.Output()
		inv.Stderr = pty.Output()
		err = inv.Run()
		require.NoError(t, err, "can't create a workspace based on the source workspace")

		// Verify if the new workspace uses expected parameters.
		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitShort)
		defer cancel()

		workspaces, err := client.Workspaces(ctx, codersdk.WorkspaceFilter{
			Name: otherWorkspace,
		})
		require.NoError(t, err, "can't list available workspaces")
		require.Len(t, workspaces.Workspaces, 1)

		otherWorkspaceLatestBuild := workspaces.Workspaces[0].LatestBuild

		buildParameters, err := client.WorkspaceBuildParameters(ctx, otherWorkspaceLatestBuild.ID)
		require.NoError(t, err)
		require.Len(t, buildParameters, 3)
		require.Contains(t, buildParameters, codersdk.WorkspaceBuildParameter{Name: firstParameterName, Value: firstParameterValue})
		require.Contains(t, buildParameters, codersdk.WorkspaceBuildParameter{Name: secondParameterName, Value: secondParameterValue})
		require.Contains(t, buildParameters, codersdk.WorkspaceBuildParameter{Name: immutableParameterName, Value: immutableParameterValue})
	})

	t.Run("CopyParametersFromNotUpdatedWorkspace", func(t *testing.T) {
		t.Parallel()

		client := coderdtest.New(t, &coderdtest.Options{IncludeProvisionerDaemon: true})
		owner := coderdtest.CreateFirstUser(t, client)
		member, _ := coderdtest.CreateAnotherUser(t, client, owner.OrganizationID)
		version := coderdtest.CreateTemplateVersion(t, client, owner.OrganizationID, echoResponses)
		coderdtest.AwaitTemplateVersionJobCompleted(t, client, version.ID)

		template := coderdtest.CreateTemplate(t, client, owner.OrganizationID, version.ID)

		// Firstly, create a regular workspace using template with parameters.
		inv, root := clitest.New(t, "create", "my-workspace", "--template", template.Name, "-y",
			"--parameter", fmt.Sprintf("%s=%s", firstParameterName, firstParameterValue),
			"--parameter", fmt.Sprintf("%s=%s", secondParameterName, secondParameterValue),
			"--parameter", fmt.Sprintf("%s=%s", immutableParameterName, immutableParameterValue))
		clitest.SetupConfig(t, member, root)
		pty := ptytest.New(t).Attach(inv)
		inv.Stdout = pty.Output()
		inv.Stderr = pty.Output()
		err := inv.Run()
		require.NoError(t, err, "can't create first workspace")

		// Secondly, update the template to the newer version.
		version2 := coderdtest.CreateTemplateVersion(t, client, owner.OrganizationID, prepareEchoResponses([]*proto.RichParameter{
			{Name: "third_parameter", Type: "string", DefaultValue: "not-relevant"},
		}), func(ctvr *codersdk.CreateTemplateVersionRequest) {
			ctvr.TemplateID = template.ID
		})
		coderdtest.AwaitTemplateVersionJobCompleted(t, client, version2.ID)
		coderdtest.UpdateActiveTemplateVersion(t, client, template.ID, version2.ID)

		// Thirdly, create a new workspace using parameters from the previous workspace.
		const otherWorkspace = "other-workspace"

		inv, root = clitest.New(t, "create", "--copy-parameters-from", "my-workspace", otherWorkspace, "-y")
		clitest.SetupConfig(t, member, root)
		pty = ptytest.New(t).Attach(inv)
		inv.Stdout = pty.Output()
		inv.Stderr = pty.Output()
		err = inv.Run()
		require.NoError(t, err, "can't create a workspace based on the source workspace")

		// Verify if the new workspace uses expected parameters.
		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitShort)
		defer cancel()

		workspaces, err := client.Workspaces(ctx, codersdk.WorkspaceFilter{
			Name: otherWorkspace,
		})
		require.NoError(t, err, "can't list available workspaces")
		require.Len(t, workspaces.Workspaces, 1)

		otherWorkspaceLatestBuild := workspaces.Workspaces[0].LatestBuild
		require.Equal(t, version.ID, otherWorkspaceLatestBuild.TemplateVersionID)

		buildParameters, err := client.WorkspaceBuildParameters(ctx, otherWorkspaceLatestBuild.ID)
		require.NoError(t, err)
		require.Len(t, buildParameters, 3)
		require.Contains(t, buildParameters, codersdk.WorkspaceBuildParameter{Name: firstParameterName, Value: firstParameterValue})
		require.Contains(t, buildParameters, codersdk.WorkspaceBuildParameter{Name: secondParameterName, Value: secondParameterValue})
		require.Contains(t, buildParameters, codersdk.WorkspaceBuildParameter{Name: immutableParameterName, Value: immutableParameterValue})
	})
}

func TestCreateValidateRichParameters(t *testing.T) {
	t.Parallel()

	const (
		stringParameterName  = "string_parameter"
		stringParameterValue = "abc"

		listOfStringsParameterName = "list_of_strings_parameter"

		numberParameterName  = "number_parameter"
		numberParameterValue = "7"

		boolParameterName  = "bool_parameter"
		boolParameterValue = "true"
	)

	numberRichParameters := []*proto.RichParameter{
		{Name: numberParameterName, Type: "number", Mutable: true, ValidationMin: ptr.Ref(int32(3)), ValidationMax: ptr.Ref(int32(10))},
	}

	numberCustomErrorRichParameters := []*proto.RichParameter{
		{
			Name: numberParameterName, Type: "number", Mutable: true,
			ValidationMin: ptr.Ref(int32(3)), ValidationMax: ptr.Ref(int32(10)),
			ValidationError: "These are values: {min}, {max}, and {value}.",
		},
	}

	stringRichParameters := []*proto.RichParameter{
		{Name: stringParameterName, Type: "string", Mutable: true, ValidationRegex: "^[a-z]+$", ValidationError: "this is error"},
	}

	listOfStringsRichParameters := []*proto.RichParameter{
		{Name: listOfStringsParameterName, Type: "list(string)", Mutable: true, DefaultValue: `["aaa","bbb","ccc"]`},
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

		inv, root := clitest.New(t, "create", "my-workspace", "--template", template.Name)
		clitest.SetupConfig(t, member, root)
		doneChan := make(chan struct{})
		pty := ptytest.New(t).Attach(inv)
		go func() {
			defer close(doneChan)
			err := inv.Run()
			assert.NoError(t, err)
		}()

		matches := []string{
			stringParameterName, "$$",
			"does not match", "",
			"Enter a value", "abc",
			"Confirm create?", "yes",
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

	t.Run("ValidateNumber", func(t *testing.T) {
		t.Parallel()

		client := coderdtest.New(t, &coderdtest.Options{IncludeProvisionerDaemon: true})
		owner := coderdtest.CreateFirstUser(t, client)
		member, _ := coderdtest.CreateAnotherUser(t, client, owner.OrganizationID)
		version := coderdtest.CreateTemplateVersion(t, client, owner.OrganizationID, prepareEchoResponses(numberRichParameters))
		coderdtest.AwaitTemplateVersionJobCompleted(t, client, version.ID)

		template := coderdtest.CreateTemplate(t, client, owner.OrganizationID, version.ID)

		inv, root := clitest.New(t, "create", "my-workspace", "--template", template.Name)
		clitest.SetupConfig(t, member, root)
		doneChan := make(chan struct{})
		pty := ptytest.New(t).Attach(inv)
		go func() {
			defer close(doneChan)
			err := inv.Run()
			assert.NoError(t, err)
		}()

		matches := []string{
			numberParameterName, "12",
			"is more than the maximum", "",
			"Enter a value", "8",
			"Confirm create?", "yes",
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

	t.Run("ValidateNumber_CustomError", func(t *testing.T) {
		t.Parallel()

		client := coderdtest.New(t, &coderdtest.Options{IncludeProvisionerDaemon: true})
		owner := coderdtest.CreateFirstUser(t, client)
		member, _ := coderdtest.CreateAnotherUser(t, client, owner.OrganizationID)
		version := coderdtest.CreateTemplateVersion(t, client, owner.OrganizationID, prepareEchoResponses(numberCustomErrorRichParameters))
		coderdtest.AwaitTemplateVersionJobCompleted(t, client, version.ID)

		template := coderdtest.CreateTemplate(t, client, owner.OrganizationID, version.ID)

		inv, root := clitest.New(t, "create", "my-workspace", "--template", template.Name)
		clitest.SetupConfig(t, member, root)
		doneChan := make(chan struct{})
		pty := ptytest.New(t).Attach(inv)
		go func() {
			defer close(doneChan)
			err := inv.Run()
			assert.NoError(t, err)
		}()

		matches := []string{
			numberParameterName, "12",
			"These are values: 3, 10, and 12.", "",
			"Enter a value", "8",
			"Confirm create?", "yes",
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
		owner := coderdtest.CreateFirstUser(t, client)
		member, _ := coderdtest.CreateAnotherUser(t, client, owner.OrganizationID)
		version := coderdtest.CreateTemplateVersion(t, client, owner.OrganizationID, prepareEchoResponses(boolRichParameters))
		coderdtest.AwaitTemplateVersionJobCompleted(t, client, version.ID)

		template := coderdtest.CreateTemplate(t, client, owner.OrganizationID, version.ID)

		inv, root := clitest.New(t, "create", "my-workspace", "--template", template.Name)
		clitest.SetupConfig(t, member, root)
		doneChan := make(chan struct{})
		pty := ptytest.New(t).Attach(inv)
		go func() {
			defer close(doneChan)
			err := inv.Run()
			assert.NoError(t, err)
		}()

		matches := []string{
			boolParameterName, "cat",
			"boolean value can be either", "",
			"Enter a value", "true",
			"Confirm create?", "yes",
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

	t.Run("ValidateListOfStrings", func(t *testing.T) {
		t.Parallel()

		client := coderdtest.New(t, &coderdtest.Options{IncludeProvisionerDaemon: true})
		owner := coderdtest.CreateFirstUser(t, client)
		member, _ := coderdtest.CreateAnotherUser(t, client, owner.OrganizationID)
		version := coderdtest.CreateTemplateVersion(t, client, owner.OrganizationID, prepareEchoResponses(listOfStringsRichParameters))
		coderdtest.AwaitTemplateVersionJobCompleted(t, client, version.ID)
		template := coderdtest.CreateTemplate(t, client, owner.OrganizationID, version.ID)

		inv, root := clitest.New(t, "create", "my-workspace", "--template", template.Name)
		clitest.SetupConfig(t, member, root)
		pty := ptytest.New(t).Attach(inv)
		clitest.Start(t, inv)

		matches := []string{
			listOfStringsParameterName, "",
			"aaa, bbb, ccc", "",
			"Confirm create?", "yes",
		}
		for i := 0; i < len(matches); i += 2 {
			match := matches[i]
			value := matches[i+1]
			pty.ExpectMatch(match)
			if value != "" {
				pty.WriteLine(value)
			}
		}
	})

	t.Run("ValidateListOfStrings_YAMLFile", func(t *testing.T) {
		t.Parallel()

		client := coderdtest.New(t, &coderdtest.Options{IncludeProvisionerDaemon: true})
		owner := coderdtest.CreateFirstUser(t, client)
		member, _ := coderdtest.CreateAnotherUser(t, client, owner.OrganizationID)
		version := coderdtest.CreateTemplateVersion(t, client, owner.OrganizationID, prepareEchoResponses(listOfStringsRichParameters))
		coderdtest.AwaitTemplateVersionJobCompleted(t, client, version.ID)
		template := coderdtest.CreateTemplate(t, client, owner.OrganizationID, version.ID)

		tempDir := t.TempDir()
		removeTmpDirUntilSuccessAfterTest(t, tempDir)
		parameterFile, _ := os.CreateTemp(tempDir, "testParameterFile*.yaml")
		_, _ = parameterFile.WriteString(listOfStringsParameterName + `:
  - ddd
  - eee
  - fff`)
		inv, root := clitest.New(t, "create", "my-workspace", "--template", template.Name, "--rich-parameter-file", parameterFile.Name())
		clitest.SetupConfig(t, member, root)
		pty := ptytest.New(t).Attach(inv)

		clitest.Start(t, inv)

		matches := []string{
			"Confirm create?", "yes",
		}
		for i := 0; i < len(matches); i += 2 {
			match := matches[i]
			value := matches[i+1]
			pty.ExpectMatch(match)
			if value != "" {
				pty.WriteLine(value)
			}
		}
	})
}

func TestCreateWithGitAuth(t *testing.T) {
	t.Parallel()
	echoResponses := &echo.Responses{
		Parse: echo.ParseComplete,
		ProvisionPlan: []*proto.Response{
			{
				Type: &proto.Response_Plan{
					Plan: &proto.PlanComplete{
						ExternalAuthProviders: []*proto.ExternalAuthProviderResource{{Id: "github"}},
					},
				},
			},
		},
		ProvisionApply: echo.ApplyComplete,
	}

	client := coderdtest.New(t, &coderdtest.Options{
		ExternalAuthConfigs: []*externalauth.Config{{
			InstrumentedOAuth2Config: &testutil.OAuth2Config{},
			ID:                       "github",
			Regex:                    regexp.MustCompile(`github\.com`),
			Type:                     codersdk.EnhancedExternalAuthProviderGitHub.String(),
			DisplayName:              "GitHub",
		}},
		IncludeProvisionerDaemon: true,
	})
	owner := coderdtest.CreateFirstUser(t, client)
	member, _ := coderdtest.CreateAnotherUser(t, client, owner.OrganizationID)
	version := coderdtest.CreateTemplateVersion(t, client, owner.OrganizationID, echoResponses)
	coderdtest.AwaitTemplateVersionJobCompleted(t, client, version.ID)
	template := coderdtest.CreateTemplate(t, client, owner.OrganizationID, version.ID)

	inv, root := clitest.New(t, "create", "my-workspace", "--template", template.Name)
	clitest.SetupConfig(t, member, root)
	pty := ptytest.New(t).Attach(inv)
	clitest.Start(t, inv)

	pty.ExpectMatch("You must authenticate with GitHub to create a workspace")
	resp := coderdtest.RequestExternalAuthCallback(t, "github", member)
	_ = resp.Body.Close()
	require.Equal(t, http.StatusTemporaryRedirect, resp.StatusCode)
	pty.ExpectMatch("Confirm create?")
	pty.WriteLine("yes")
}
