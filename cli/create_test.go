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

	"github.com/coder/coder/v2/cli"
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

	t.Run("CreateWithSpecificTemplateVersion", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, &coderdtest.Options{IncludeProvisionerDaemon: true})
		owner := coderdtest.CreateFirstUser(t, client)
		member, _ := coderdtest.CreateAnotherUser(t, client, owner.OrganizationID)
		version := coderdtest.CreateTemplateVersion(t, client, owner.OrganizationID, completeWithAgent())
		coderdtest.AwaitTemplateVersionJobCompleted(t, client, version.ID)
		template := coderdtest.CreateTemplate(t, client, owner.OrganizationID, version.ID)

		// Create a new version
		version2 := coderdtest.CreateTemplateVersion(t, client, owner.OrganizationID, completeWithAgent(), func(ctvr *codersdk.CreateTemplateVersionRequest) {
			ctvr.TemplateID = template.ID
		})
		coderdtest.AwaitTemplateVersionJobCompleted(t, client, version2.ID)

		args := []string{
			"create",
			"my-workspace",
			"--template", template.Name,
			"--template-version", version2.Name,
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
			// Check if the workspace is using the new template version
			assert.Equal(t, ws.LatestBuild.TemplateVersionID, version2.ID, "expected workspace to use the specified template version")
			if assert.NotNil(t, ws.AutostartSchedule) {
				assert.Equal(t, *ws.AutostartSchedule, "CRON_TZ=US/Central 30 9 * * Mon-Fri")
			}
			if assert.NotNil(t, ws.TTLMillis) {
				assert.Equal(t, *ws.TTLMillis, 8*time.Hour.Milliseconds())
			}
			assert.Equal(t, codersdk.AutomaticUpdatesAlways, ws.AutomaticUpdates)
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

	t.Run("NoWait", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, &coderdtest.Options{IncludeProvisionerDaemon: true})
		owner := coderdtest.CreateFirstUser(t, client)
		member, _ := coderdtest.CreateAnotherUser(t, client, owner.OrganizationID)
		version := coderdtest.CreateTemplateVersion(t, client, owner.OrganizationID, completeWithAgent())
		coderdtest.AwaitTemplateVersionJobCompleted(t, client, version.ID)
		template := coderdtest.CreateTemplate(t, client, owner.OrganizationID, version.ID)

		inv, root := clitest.New(t, "create", "my-workspace", "--template", template.Name, "-y", "--no-wait")
		clitest.SetupConfig(t, member, root)
		pty := ptytest.New(t).Attach(inv)

		err := inv.Run()
		require.NoError(t, err)

		pty.ExpectMatch("Building in the background")

		// Workspace should exist even though we didn't wait
		ws, err := member.WorkspaceByOwnerAndName(context.Background(), codersdk.Me, "my-workspace", codersdk.WorkspaceOptions{})
		require.NoError(t, err)
		require.Equal(t, template.Name, ws.TemplateName)
	})

	t.Run("NoPrompt/MissingWorkspaceName", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, &coderdtest.Options{IncludeProvisionerDaemon: true})
		owner := coderdtest.CreateFirstUser(t, client)
		member, _ := coderdtest.CreateAnotherUser(t, client, owner.OrganizationID)
		version := coderdtest.CreateTemplateVersion(t, client, owner.OrganizationID, nil)
		coderdtest.AwaitTemplateVersionJobCompleted(t, client, version.ID)
		_ = coderdtest.CreateTemplate(t, client, owner.OrganizationID, version.ID)

		// Don't provide workspace name and use --no-prompt
		inv, root := clitest.New(t, "create", "--no-prompt", "-y")
		clitest.SetupConfig(t, member, root)

		err := inv.Run()
		require.Error(t, err)
		require.Contains(t, err.Error(), "workspace name is required")
	})

	t.Run("NoPrompt/MissingTemplateName", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, &coderdtest.Options{IncludeProvisionerDaemon: true})
		owner := coderdtest.CreateFirstUser(t, client)
		member, _ := coderdtest.CreateAnotherUser(t, client, owner.OrganizationID)
		version := coderdtest.CreateTemplateVersion(t, client, owner.OrganizationID, nil)
		coderdtest.AwaitTemplateVersionJobCompleted(t, client, version.ID)
		_ = coderdtest.CreateTemplate(t, client, owner.OrganizationID, version.ID)

		// Don't provide template name and use --no-prompt
		inv, root := clitest.New(t, "create", "my-workspace", "--no-prompt", "-y")
		clitest.SetupConfig(t, member, root)

		err := inv.Run()
		require.Error(t, err)
		require.Contains(t, err.Error(), "template name is required")
	})

	t.Run("NoPrompt/Success", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, &coderdtest.Options{IncludeProvisionerDaemon: true})
		owner := coderdtest.CreateFirstUser(t, client)
		member, _ := coderdtest.CreateAnotherUser(t, client, owner.OrganizationID)
		version := coderdtest.CreateTemplateVersion(t, client, owner.OrganizationID, completeWithAgent())
		coderdtest.AwaitTemplateVersionJobCompleted(t, client, version.ID)
		template := coderdtest.CreateTemplate(t, client, owner.OrganizationID, version.ID)

		// Provide all required values so no prompts are needed
		inv, root := clitest.New(t, "create", "my-workspace", "--template", template.Name, "--no-prompt", "-y")
		clitest.SetupConfig(t, member, root)

		err := inv.Run()
		require.NoError(t, err)

		ws, err := member.WorkspaceByOwnerAndName(context.Background(), codersdk.Me, "my-workspace", codersdk.WorkspaceOptions{})
		require.NoError(t, err)
		require.Equal(t, template.Name, ws.TemplateName)
	})
}

func prepareEchoResponses(parameters []*proto.RichParameter, presets ...*proto.Preset) *echo.Responses {
	return &echo.Responses{
		Parse: echo.ParseComplete,
		ProvisionPlan: []*proto.Response{
			{
				Type: &proto.Response_Plan{
					Plan: &proto.PlanComplete{
						Parameters: parameters,
						Presets:    presets,
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

	echoResponses := func() *echo.Responses {
		return prepareEchoResponses([]*proto.RichParameter{
			{Name: firstParameterName, Description: firstParameterDescription, Mutable: true},
			{Name: secondParameterName, DisplayName: secondParameterDisplayName, Description: secondParameterDescription, Mutable: true},
			{Name: immutableParameterName, Description: immutableParameterDescription, Mutable: false},
		})
	}

	t.Run("InputParameters", func(t *testing.T) {
		t.Parallel()

		client := coderdtest.New(t, &coderdtest.Options{IncludeProvisionerDaemon: true})
		owner := coderdtest.CreateFirstUser(t, client)
		member, _ := coderdtest.CreateAnotherUser(t, client, owner.OrganizationID)
		version := coderdtest.CreateTemplateVersion(t, client, owner.OrganizationID, echoResponses())
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
		version := coderdtest.CreateTemplateVersion(t, client, owner.OrganizationID, echoResponses())
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
		version := coderdtest.CreateTemplateVersion(t, client, owner.OrganizationID, echoResponses())
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
		version := coderdtest.CreateTemplateVersion(t, client, owner.OrganizationID, echoResponses())
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
		version := coderdtest.CreateTemplateVersion(t, client, owner.OrganizationID, echoResponses())
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
		version := coderdtest.CreateTemplateVersion(t, client, owner.OrganizationID, echoResponses())
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
		version := coderdtest.CreateTemplateVersion(t, client, owner.OrganizationID, echoResponses())
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

func TestCreateWithPreset(t *testing.T) {
	t.Parallel()

	const (
		firstParameterName        = "first_parameter"
		firstParameterDisplayName = "First Parameter"
		firstParameterDescription = "This is the first parameter"
		firstParameterValue       = "1"

		firstOptionalParameterName         = "first_optional_parameter"
		firstOptionalParameterDescription  = "This is the first optional parameter"
		firstOptionalParameterValue        = "1"
		secondOptionalParameterName        = "second_optional_parameter"
		secondOptionalParameterDescription = "This is the second optional parameter"
		secondOptionalParameterValue       = "2"

		thirdParameterName        = "third_parameter"
		thirdParameterDescription = "This is the third parameter"
		thirdParameterValue       = "3"
	)

	echoResponses := func(presets ...*proto.Preset) *echo.Responses {
		return prepareEchoResponses([]*proto.RichParameter{
			{
				Name:         firstParameterName,
				DisplayName:  firstParameterDisplayName,
				Description:  firstParameterDescription,
				Mutable:      true,
				DefaultValue: firstParameterValue,
				Options: []*proto.RichParameterOption{
					{
						Name:        firstOptionalParameterName,
						Description: firstOptionalParameterDescription,
						Value:       firstOptionalParameterValue,
					},
					{
						Name:        secondOptionalParameterName,
						Description: secondOptionalParameterDescription,
						Value:       secondOptionalParameterValue,
					},
				},
			},
			{
				Name:         thirdParameterName,
				Description:  thirdParameterDescription,
				DefaultValue: thirdParameterValue,
				Mutable:      true,
			},
		}, presets...)
	}

	// This test verifies that when a template has presets,
	// including a default preset, and the user specifies a `--preset` flag,
	// the CLI uses the specified preset instead of the default
	t.Run("PresetFlag", func(t *testing.T) {
		t.Parallel()

		client := coderdtest.New(t, &coderdtest.Options{IncludeProvisionerDaemon: true})
		owner := coderdtest.CreateFirstUser(t, client)
		member, _ := coderdtest.CreateAnotherUser(t, client, owner.OrganizationID)

		// Given: a template and a template version with two presets, including a default
		defaultPreset := proto.Preset{
			Name:    "preset-default",
			Default: true,
			Parameters: []*proto.PresetParameter{
				{Name: thirdParameterName, Value: thirdParameterValue},
			},
		}
		preset := proto.Preset{
			Name: "preset-test",
			Parameters: []*proto.PresetParameter{
				{Name: firstParameterName, Value: secondOptionalParameterValue},
				{Name: thirdParameterName, Value: thirdParameterValue},
			},
		}
		version := coderdtest.CreateTemplateVersion(t, client, owner.OrganizationID, echoResponses(&defaultPreset, &preset))
		coderdtest.AwaitTemplateVersionJobCompleted(t, client, version.ID)
		template := coderdtest.CreateTemplate(t, client, owner.OrganizationID, version.ID)

		// When: running the create command with the specified preset
		workspaceName := "my-workspace"
		inv, root := clitest.New(t, "create", workspaceName, "--template", template.Name, "-y", "--preset", preset.Name)
		clitest.SetupConfig(t, member, root)
		pty := ptytest.New(t).Attach(inv)
		inv.Stdout = pty.Output()
		inv.Stderr = pty.Output()
		err := inv.Run()
		require.NoError(t, err)

		// Should: display the selected preset as well as its parameters
		presetName := fmt.Sprintf("Preset '%s' applied:", preset.Name)
		pty.ExpectMatch(presetName)
		pty.ExpectMatch(fmt.Sprintf("%s: '%s'", firstParameterName, secondOptionalParameterValue))
		pty.ExpectMatch(fmt.Sprintf("%s: '%s'", thirdParameterName, thirdParameterValue))

		// Verify if the new workspace uses expected parameters.
		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitShort)
		defer cancel()

		tvPresets, err := client.TemplateVersionPresets(ctx, version.ID)
		require.NoError(t, err)
		require.Len(t, tvPresets, 2)
		var selectedPreset *codersdk.Preset
		for _, tvPreset := range tvPresets {
			if tvPreset.Name == preset.Name {
				selectedPreset = &tvPreset
			}
		}
		require.NotNil(t, selectedPreset)

		workspaces, err := client.Workspaces(ctx, codersdk.WorkspaceFilter{
			Name: workspaceName,
		})
		require.NoError(t, err)
		require.Len(t, workspaces.Workspaces, 1)

		// Should: create a workspace using the expected template version and the preset-defined parameters
		workspaceLatestBuild := workspaces.Workspaces[0].LatestBuild
		require.Equal(t, version.ID, workspaceLatestBuild.TemplateVersionID)
		require.Equal(t, selectedPreset.ID, *workspaceLatestBuild.TemplateVersionPresetID)
		buildParameters, err := client.WorkspaceBuildParameters(ctx, workspaceLatestBuild.ID)
		require.NoError(t, err)
		require.Len(t, buildParameters, 2)
		require.Contains(t, buildParameters, codersdk.WorkspaceBuildParameter{Name: firstParameterName, Value: secondOptionalParameterValue})
		require.Contains(t, buildParameters, codersdk.WorkspaceBuildParameter{Name: thirdParameterName, Value: thirdParameterValue})
	})

	// This test verifies that when a template has presets,
	// including a default preset, and the user does not specify the `--preset` flag,
	// the CLI automatically uses the default preset to create the workspace
	t.Run("DefaultPreset", func(t *testing.T) {
		t.Parallel()

		client := coderdtest.New(t, &coderdtest.Options{IncludeProvisionerDaemon: true})
		owner := coderdtest.CreateFirstUser(t, client)
		member, _ := coderdtest.CreateAnotherUser(t, client, owner.OrganizationID)

		// Given: a template and a template version with two presets, including a default
		defaultPreset := proto.Preset{
			Name:    "preset-default",
			Default: true,
			Parameters: []*proto.PresetParameter{
				{Name: firstParameterName, Value: secondOptionalParameterValue},
				{Name: thirdParameterName, Value: thirdParameterValue},
			},
		}
		preset := proto.Preset{
			Name: "preset-test",
			Parameters: []*proto.PresetParameter{
				{Name: thirdParameterName, Value: thirdParameterValue},
			},
		}
		version := coderdtest.CreateTemplateVersion(t, client, owner.OrganizationID, echoResponses(&defaultPreset, &preset))
		coderdtest.AwaitTemplateVersionJobCompleted(t, client, version.ID)
		template := coderdtest.CreateTemplate(t, client, owner.OrganizationID, version.ID)

		// When: running the create command without a preset
		workspaceName := "my-workspace"
		inv, root := clitest.New(t, "create", workspaceName, "--template", template.Name, "-y")
		clitest.SetupConfig(t, member, root)
		pty := ptytest.New(t).Attach(inv)
		inv.Stdout = pty.Output()
		inv.Stderr = pty.Output()
		err := inv.Run()
		require.NoError(t, err)

		// Should: display the default preset as well as its parameters
		presetName := fmt.Sprintf("Preset '%s' (default) applied:", defaultPreset.Name)
		pty.ExpectMatch(presetName)
		pty.ExpectMatch(fmt.Sprintf("%s: '%s'", firstParameterName, secondOptionalParameterValue))
		pty.ExpectMatch(fmt.Sprintf("%s: '%s'", thirdParameterName, thirdParameterValue))

		// Verify if the new workspace uses expected parameters.
		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitShort)
		defer cancel()

		tvPresets, err := client.TemplateVersionPresets(ctx, version.ID)
		require.NoError(t, err)
		require.Len(t, tvPresets, 2)
		var selectedPreset *codersdk.Preset
		for _, tvPreset := range tvPresets {
			if tvPreset.Default {
				selectedPreset = &tvPreset
			}
		}
		require.NotNil(t, selectedPreset)

		workspaces, err := client.Workspaces(ctx, codersdk.WorkspaceFilter{
			Name: workspaceName,
		})
		require.NoError(t, err)
		require.Len(t, workspaces.Workspaces, 1)

		// Should: create a workspace using the expected template version and the default preset parameters
		workspaceLatestBuild := workspaces.Workspaces[0].LatestBuild
		require.Equal(t, version.ID, workspaceLatestBuild.TemplateVersionID)
		require.Equal(t, selectedPreset.ID, *workspaceLatestBuild.TemplateVersionPresetID)
		buildParameters, err := client.WorkspaceBuildParameters(ctx, workspaceLatestBuild.ID)
		require.NoError(t, err)
		require.Len(t, buildParameters, 2)
		require.Contains(t, buildParameters, codersdk.WorkspaceBuildParameter{Name: firstParameterName, Value: secondOptionalParameterValue})
		require.Contains(t, buildParameters, codersdk.WorkspaceBuildParameter{Name: thirdParameterName, Value: thirdParameterValue})
	})

	// This test verifies that when a template has presets but no default preset,
	// and the user does not provide the `--preset` flag,
	// the CLI prompts the user to select a preset.
	t.Run("NoDefaultPresetPromptUser", func(t *testing.T) {
		t.Parallel()

		client := coderdtest.New(t, &coderdtest.Options{IncludeProvisionerDaemon: true})
		owner := coderdtest.CreateFirstUser(t, client)
		member, _ := coderdtest.CreateAnotherUser(t, client, owner.OrganizationID)

		// Given: a template and a template version with two presets
		preset := proto.Preset{
			Name:        "preset-test",
			Description: "Preset Test.",
			Parameters: []*proto.PresetParameter{
				{Name: firstParameterName, Value: secondOptionalParameterValue},
				{Name: thirdParameterName, Value: thirdParameterValue},
			},
		}
		version := coderdtest.CreateTemplateVersion(t, client, owner.OrganizationID, echoResponses(&preset))
		coderdtest.AwaitTemplateVersionJobCompleted(t, client, version.ID)
		template := coderdtest.CreateTemplate(t, client, owner.OrganizationID, version.ID)

		// When: running the create command without specifying a preset
		workspaceName := "my-workspace"
		inv, root := clitest.New(t, "create", workspaceName, "--template", template.Name,
			"--parameter", fmt.Sprintf("%s=%s", firstParameterName, firstOptionalParameterValue),
			"--parameter", fmt.Sprintf("%s=%s", thirdParameterName, thirdParameterValue))
		clitest.SetupConfig(t, member, root)
		doneChan := make(chan struct{})
		pty := ptytest.New(t).Attach(inv)
		go func() {
			defer close(doneChan)
			err := inv.Run()
			assert.NoError(t, err)
		}()

		// Should: prompt the user for the preset
		pty.ExpectMatch("Select a preset below:")
		pty.WriteLine("\n")
		pty.ExpectMatch("Preset 'preset-test' applied")
		pty.ExpectMatch("Confirm create?")
		pty.WriteLine("yes")

		<-doneChan

		// Verify if the new workspace uses expected parameters.
		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitShort)
		defer cancel()

		tvPresets, err := client.TemplateVersionPresets(ctx, version.ID)
		require.NoError(t, err)
		require.Len(t, tvPresets, 1)

		workspaces, err := client.Workspaces(ctx, codersdk.WorkspaceFilter{
			Name: workspaceName,
		})
		require.NoError(t, err)
		require.Len(t, workspaces.Workspaces, 1)

		// Should: create a workspace using the expected template version and the preset-defined parameters
		workspaceLatestBuild := workspaces.Workspaces[0].LatestBuild
		require.Equal(t, version.ID, workspaceLatestBuild.TemplateVersionID)
		require.Equal(t, tvPresets[0].ID, *workspaceLatestBuild.TemplateVersionPresetID)
		buildParameters, err := client.WorkspaceBuildParameters(ctx, workspaceLatestBuild.ID)
		require.NoError(t, err)
		require.Len(t, buildParameters, 2)
		require.Contains(t, buildParameters, codersdk.WorkspaceBuildParameter{Name: firstParameterName, Value: secondOptionalParameterValue})
		require.Contains(t, buildParameters, codersdk.WorkspaceBuildParameter{Name: thirdParameterName, Value: thirdParameterValue})
	})

	// This test verifies that when a template version has no presets,
	// the CLI does not prompt the user to select a preset and proceeds
	// with workspace creation without applying any preset.
	t.Run("TemplateVersionWithoutPresets", func(t *testing.T) {
		t.Parallel()

		client := coderdtest.New(t, &coderdtest.Options{IncludeProvisionerDaemon: true})
		owner := coderdtest.CreateFirstUser(t, client)
		member, _ := coderdtest.CreateAnotherUser(t, client, owner.OrganizationID)

		// Given: a template and a template version without presets
		version := coderdtest.CreateTemplateVersion(t, client, owner.OrganizationID, echoResponses())
		coderdtest.AwaitTemplateVersionJobCompleted(t, client, version.ID)
		template := coderdtest.CreateTemplate(t, client, owner.OrganizationID, version.ID)

		// When: running the create command without a preset
		workspaceName := "my-workspace"
		inv, root := clitest.New(t, "create", workspaceName, "--template", template.Name, "-y",
			"--parameter", fmt.Sprintf("%s=%s", firstParameterName, firstOptionalParameterValue),
			"--parameter", fmt.Sprintf("%s=%s", thirdParameterName, thirdParameterValue))
		clitest.SetupConfig(t, member, root)
		pty := ptytest.New(t).Attach(inv)
		inv.Stdout = pty.Output()
		inv.Stderr = pty.Output()
		err := inv.Run()
		require.NoError(t, err)
		pty.ExpectMatch("No preset applied.")

		// Verify if the new workspace uses expected parameters.
		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitShort)
		defer cancel()

		workspaces, err := client.Workspaces(ctx, codersdk.WorkspaceFilter{
			Name: workspaceName,
		})
		require.NoError(t, err)
		require.Len(t, workspaces.Workspaces, 1)

		// Should: create a workspace using the expected template version and no preset
		workspaceLatestBuild := workspaces.Workspaces[0].LatestBuild
		require.Equal(t, version.ID, workspaceLatestBuild.TemplateVersionID)
		require.Nil(t, workspaceLatestBuild.TemplateVersionPresetID)
		buildParameters, err := client.WorkspaceBuildParameters(ctx, workspaceLatestBuild.ID)
		require.NoError(t, err)
		require.Len(t, buildParameters, 2)
		require.Contains(t, buildParameters, codersdk.WorkspaceBuildParameter{Name: firstParameterName, Value: firstOptionalParameterValue})
		require.Contains(t, buildParameters, codersdk.WorkspaceBuildParameter{Name: thirdParameterName, Value: thirdParameterValue})
	})

	// This test verifies that when the user provides `--preset none`,
	// the CLI skips applying any preset, even if the template version has a default preset.
	// The workspace should be created without using any preset-defined parameters.
	t.Run("PresetFlagNone", func(t *testing.T) {
		t.Parallel()

		client := coderdtest.New(t, &coderdtest.Options{IncludeProvisionerDaemon: true})
		owner := coderdtest.CreateFirstUser(t, client)
		member, _ := coderdtest.CreateAnotherUser(t, client, owner.OrganizationID)

		// Given: a template and a template version with a default preset
		preset := proto.Preset{
			Name:    "preset-test",
			Default: true,
			Parameters: []*proto.PresetParameter{
				{Name: firstParameterName, Value: secondOptionalParameterValue},
				{Name: thirdParameterName, Value: thirdParameterValue},
			},
		}
		version := coderdtest.CreateTemplateVersion(t, client, owner.OrganizationID, echoResponses(&preset))
		coderdtest.AwaitTemplateVersionJobCompleted(t, client, version.ID)
		template := coderdtest.CreateTemplate(t, client, owner.OrganizationID, version.ID)

		// When: running the create command with flag '--preset none'
		workspaceName := "my-workspace"
		inv, root := clitest.New(t, "create", workspaceName, "--template", template.Name, "-y", "--preset", cli.PresetNone,
			"--parameter", fmt.Sprintf("%s=%s", firstParameterName, firstOptionalParameterValue),
			"--parameter", fmt.Sprintf("%s=%s", thirdParameterName, thirdParameterValue))
		clitest.SetupConfig(t, member, root)
		pty := ptytest.New(t).Attach(inv)
		inv.Stdout = pty.Output()
		inv.Stderr = pty.Output()
		err := inv.Run()
		require.NoError(t, err)
		pty.ExpectMatch("No preset applied.")

		// Verify that the new workspace doesn't use the preset parameters.
		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitShort)
		defer cancel()

		tvPresets, err := client.TemplateVersionPresets(ctx, version.ID)
		require.NoError(t, err)
		require.Len(t, tvPresets, 1)

		workspaces, err := client.Workspaces(ctx, codersdk.WorkspaceFilter{
			Name: workspaceName,
		})
		require.NoError(t, err)
		require.Len(t, workspaces.Workspaces, 1)

		// Should: create a workspace using the expected template version and no preset
		workspaceLatestBuild := workspaces.Workspaces[0].LatestBuild
		require.Equal(t, version.ID, workspaceLatestBuild.TemplateVersionID)
		require.Nil(t, workspaceLatestBuild.TemplateVersionPresetID)
		buildParameters, err := client.WorkspaceBuildParameters(ctx, workspaceLatestBuild.ID)
		require.NoError(t, err)
		require.Len(t, buildParameters, 2)
		require.Contains(t, buildParameters, codersdk.WorkspaceBuildParameter{Name: firstParameterName, Value: firstOptionalParameterValue})
		require.Contains(t, buildParameters, codersdk.WorkspaceBuildParameter{Name: thirdParameterName, Value: thirdParameterValue})
	})

	// This test verifies that the CLI returns an appropriate error
	// when a user provides a `--preset` value that does not correspond
	// to any existing preset in the template version.
	t.Run("FailsWhenPresetDoesNotExist", func(t *testing.T) {
		t.Parallel()

		client := coderdtest.New(t, &coderdtest.Options{IncludeProvisionerDaemon: true})
		owner := coderdtest.CreateFirstUser(t, client)
		member, _ := coderdtest.CreateAnotherUser(t, client, owner.OrganizationID)

		// Given: a template and a template version where the preset defines values for all required parameters
		preset := proto.Preset{
			Name: "preset-test",
			Parameters: []*proto.PresetParameter{
				{Name: firstParameterName, Value: secondOptionalParameterValue},
				{Name: thirdParameterName, Value: thirdParameterValue},
			},
		}
		version := coderdtest.CreateTemplateVersion(t, client, owner.OrganizationID, echoResponses(&preset))
		coderdtest.AwaitTemplateVersionJobCompleted(t, client, version.ID)
		template := coderdtest.CreateTemplate(t, client, owner.OrganizationID, version.ID)

		// When: running the create command with a non-existent preset
		workspaceName := "my-workspace"
		inv, root := clitest.New(t, "create", workspaceName, "--template", template.Name, "-y", "--preset", "invalid-preset")
		clitest.SetupConfig(t, member, root)
		pty := ptytest.New(t).Attach(inv)
		inv.Stdout = pty.Output()
		inv.Stderr = pty.Output()
		err := inv.Run()

		// Should: fail with an error indicating the preset was not found
		require.Contains(t, err.Error(), "preset \"invalid-preset\" not found")
	})

	// This test verifies that when both a preset and a user-provided
	// `--parameter` flag define a value for the same parameter,
	// the preset's value takes precedence over the user's.
	//
	// The preset defines one parameter (A), and two `--parameter` flags provide A and B.
	// The workspace should be created using:
	// - the value of parameter A from the preset (overriding the parameter flag's value),
	// - and the value of parameter B from the parameter flag.
	t.Run("PresetOverridesParameterFlagValues", func(t *testing.T) {
		t.Parallel()

		client := coderdtest.New(t, &coderdtest.Options{IncludeProvisionerDaemon: true})
		owner := coderdtest.CreateFirstUser(t, client)
		member, _ := coderdtest.CreateAnotherUser(t, client, owner.OrganizationID)

		// Given: a template version with a preset that defines one parameter
		preset := proto.Preset{
			Name: "preset-test",
			Parameters: []*proto.PresetParameter{
				{Name: firstParameterName, Value: secondOptionalParameterValue},
			},
		}
		version := coderdtest.CreateTemplateVersion(t, client, owner.OrganizationID, echoResponses(&preset))
		coderdtest.AwaitTemplateVersionJobCompleted(t, client, version.ID)
		template := coderdtest.CreateTemplate(t, client, owner.OrganizationID, version.ID)

		// When: creating a workspace with a preset and passing overlapping and additional parameters via `--parameter`
		workspaceName := "my-workspace"
		inv, root := clitest.New(t, "create", workspaceName, "--template", template.Name, "-y",
			"--preset", preset.Name,
			"--parameter", fmt.Sprintf("%s=%s", firstParameterName, firstOptionalParameterValue),
			"--parameter", fmt.Sprintf("%s=%s", thirdParameterName, thirdParameterValue))
		clitest.SetupConfig(t, member, root)
		pty := ptytest.New(t).Attach(inv)
		inv.Stdout = pty.Output()
		inv.Stderr = pty.Output()
		err := inv.Run()
		require.NoError(t, err)

		// Should: display the selected preset as well as its parameter
		presetName := fmt.Sprintf("Preset '%s' applied:", preset.Name)
		pty.ExpectMatch(presetName)
		pty.ExpectMatch(fmt.Sprintf("%s: '%s'", firstParameterName, secondOptionalParameterValue))

		// Verify if the new workspace uses expected parameters.
		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitShort)
		defer cancel()

		tvPresets, err := client.TemplateVersionPresets(ctx, version.ID)
		require.NoError(t, err)
		require.Len(t, tvPresets, 1)

		workspaces, err := client.Workspaces(ctx, codersdk.WorkspaceFilter{
			Name: workspaceName,
		})
		require.NoError(t, err)
		require.Len(t, workspaces.Workspaces, 1)

		// Should: include both parameters, one from the preset and one from the `--parameter` flag
		workspaceLatestBuild := workspaces.Workspaces[0].LatestBuild
		require.Equal(t, version.ID, workspaceLatestBuild.TemplateVersionID)
		require.Equal(t, tvPresets[0].ID, *workspaceLatestBuild.TemplateVersionPresetID)
		buildParameters, err := client.WorkspaceBuildParameters(ctx, workspaceLatestBuild.ID)
		require.NoError(t, err)
		require.Len(t, buildParameters, 2)
		require.Contains(t, buildParameters, codersdk.WorkspaceBuildParameter{Name: firstParameterName, Value: secondOptionalParameterValue})
		require.Contains(t, buildParameters, codersdk.WorkspaceBuildParameter{Name: thirdParameterName, Value: thirdParameterValue})
	})

	// This test verifies that when both a preset and a user-provided
	// `--rich-parameter-file` define a value for the same parameter,
	// the preset's value takes precedence over the one in the file.
	//
	// The preset defines one parameter (A), and the parameter file provides two parameters (A and B).
	// The workspace should be created using:
	// - the value of parameter A from the preset (overriding the file's value),
	// - and the value of parameter B from the file.
	t.Run("PresetOverridesParameterFileValues", func(t *testing.T) {
		t.Parallel()

		client := coderdtest.New(t, &coderdtest.Options{IncludeProvisionerDaemon: true})
		owner := coderdtest.CreateFirstUser(t, client)
		member, _ := coderdtest.CreateAnotherUser(t, client, owner.OrganizationID)

		// Given: a template version with a preset that defines one parameter
		preset := proto.Preset{
			Name: "preset-test",
			Parameters: []*proto.PresetParameter{
				{Name: firstParameterName, Value: secondOptionalParameterValue},
			},
		}
		version := coderdtest.CreateTemplateVersion(t, client, owner.OrganizationID, echoResponses(&preset))
		coderdtest.AwaitTemplateVersionJobCompleted(t, client, version.ID)
		template := coderdtest.CreateTemplate(t, client, owner.OrganizationID, version.ID)

		// When: creating a workspace with the preset and passing the second required parameter via `--rich-parameter-file`
		workspaceName := "my-workspace"
		tempDir := t.TempDir()
		removeTmpDirUntilSuccessAfterTest(t, tempDir)
		parameterFile, _ := os.CreateTemp(tempDir, "testParameterFile*.yaml")
		_, _ = parameterFile.WriteString(
			firstParameterName + ": " + firstOptionalParameterValue + "\n" +
				thirdParameterName + ": " + thirdParameterValue)
		inv, root := clitest.New(t, "create", workspaceName, "--template", template.Name, "-y",
			"--preset", preset.Name,
			"--rich-parameter-file", parameterFile.Name())
		clitest.SetupConfig(t, member, root)
		pty := ptytest.New(t).Attach(inv)
		inv.Stdout = pty.Output()
		inv.Stderr = pty.Output()
		err := inv.Run()
		require.NoError(t, err)

		// Should: display the selected preset as well as its parameter
		presetName := fmt.Sprintf("Preset '%s' applied:", preset.Name)
		pty.ExpectMatch(presetName)
		pty.ExpectMatch(fmt.Sprintf("%s: '%s'", firstParameterName, secondOptionalParameterValue))

		// Verify if the new workspace uses expected parameters.
		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitShort)
		defer cancel()

		tvPresets, err := client.TemplateVersionPresets(ctx, version.ID)
		require.NoError(t, err)
		require.Len(t, tvPresets, 1)

		workspaces, err := client.Workspaces(ctx, codersdk.WorkspaceFilter{
			Name: workspaceName,
		})
		require.NoError(t, err)
		require.Len(t, workspaces.Workspaces, 1)

		// Should: include both parameters, one from the preset and one from the `--rich-parameter-file` flag
		workspaceLatestBuild := workspaces.Workspaces[0].LatestBuild
		require.Equal(t, version.ID, workspaceLatestBuild.TemplateVersionID)
		require.Equal(t, tvPresets[0].ID, *workspaceLatestBuild.TemplateVersionPresetID)
		buildParameters, err := client.WorkspaceBuildParameters(ctx, workspaceLatestBuild.ID)
		require.NoError(t, err)
		require.Len(t, buildParameters, 2)
		require.Contains(t, buildParameters, codersdk.WorkspaceBuildParameter{Name: firstParameterName, Value: secondOptionalParameterValue})
		require.Contains(t, buildParameters, codersdk.WorkspaceBuildParameter{Name: thirdParameterName, Value: thirdParameterValue})
	})

	// This test verifies that when a preset provides only some parameters,
	// and the remaining ones are not provided via flags,
	// the CLI prompts the user for input to fill in the missing parameters.
	t.Run("PromptsForMissingParametersWhenPresetIsIncomplete", func(t *testing.T) {
		t.Parallel()

		client := coderdtest.New(t, &coderdtest.Options{IncludeProvisionerDaemon: true})
		owner := coderdtest.CreateFirstUser(t, client)
		member, _ := coderdtest.CreateAnotherUser(t, client, owner.OrganizationID)

		// Given: a template version with a preset that defines one parameter
		preset := proto.Preset{
			Name: "preset-test",
			Parameters: []*proto.PresetParameter{
				{Name: firstParameterName, Value: secondOptionalParameterValue},
			},
		}
		version := coderdtest.CreateTemplateVersion(t, client, owner.OrganizationID, echoResponses(&preset))
		coderdtest.AwaitTemplateVersionJobCompleted(t, client, version.ID)
		template := coderdtest.CreateTemplate(t, client, owner.OrganizationID, version.ID)

		// When: running the create command with the specified preset
		workspaceName := "my-workspace"
		inv, root := clitest.New(t, "create", workspaceName, "--template", template.Name, "--preset", preset.Name)
		clitest.SetupConfig(t, member, root)
		doneChan := make(chan struct{})
		pty := ptytest.New(t).Attach(inv)
		go func() {
			defer close(doneChan)
			err := inv.Run()
			assert.NoError(t, err)
		}()

		// Should: display the selected preset as well as its parameters
		presetName := fmt.Sprintf("Preset '%s' applied:", preset.Name)
		pty.ExpectMatch(presetName)
		pty.ExpectMatch(fmt.Sprintf("%s: '%s'", firstParameterName, secondOptionalParameterValue))

		// Should: prompt for the missing parameter
		pty.ExpectMatch(thirdParameterDescription)
		pty.WriteLine(thirdParameterValue)
		pty.ExpectMatch("Confirm create?")
		pty.WriteLine("yes")

		<-doneChan

		// Verify if the new workspace uses expected parameters.
		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
		defer cancel()

		tvPresets, err := client.TemplateVersionPresets(ctx, version.ID)
		require.NoError(t, err)
		require.Len(t, tvPresets, 1)

		workspaces, err := client.Workspaces(ctx, codersdk.WorkspaceFilter{
			Name: workspaceName,
		})
		require.NoError(t, err)
		require.Len(t, workspaces.Workspaces, 1)

		// Should: create a workspace using the expected template version and the preset-defined parameters
		workspaceLatestBuild := workspaces.Workspaces[0].LatestBuild
		require.Equal(t, version.ID, workspaceLatestBuild.TemplateVersionID)
		require.Equal(t, tvPresets[0].ID, *workspaceLatestBuild.TemplateVersionPresetID)
		buildParameters, err := client.WorkspaceBuildParameters(ctx, workspaceLatestBuild.ID)
		require.NoError(t, err)
		require.Len(t, buildParameters, 2)
		require.Contains(t, buildParameters, codersdk.WorkspaceBuildParameter{Name: firstParameterName, Value: secondOptionalParameterValue})
		require.Contains(t, buildParameters, codersdk.WorkspaceBuildParameter{Name: thirdParameterName, Value: thirdParameterValue})
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

		t.Run("Prompt", func(t *testing.T) {
			inv, root := clitest.New(t, "create", "my-workspace-1", "--template", template.Name)
			clitest.SetupConfig(t, member, root)
			pty := ptytest.New(t).Attach(inv)
			clitest.Start(t, inv)

			pty.ExpectMatch(listOfStringsParameterName)
			pty.ExpectMatch("aaa, bbb, ccc")
			pty.ExpectMatch("Confirm create?")
			pty.WriteLine("yes")
		})

		t.Run("Default", func(t *testing.T) {
			t.Parallel()
			inv, root := clitest.New(t, "create", "my-workspace-2", "--template", template.Name, "--yes")
			clitest.SetupConfig(t, member, root)
			clitest.Run(t, inv)
		})

		t.Run("CLIOverride/DoubleQuote", func(t *testing.T) {
			t.Parallel()

			// Note: see https://go.dev/play/p/vhTUTZsVrEb for how to escape this properly
			parameterArg := fmt.Sprintf(`"%s=[""ddd=foo"",""eee=bar"",""fff=baz""]"`, listOfStringsParameterName)
			inv, root := clitest.New(t, "create", "my-workspace-3", "--template", template.Name, "--parameter", parameterArg, "--yes")
			clitest.SetupConfig(t, member, root)
			clitest.Run(t, inv)
		})
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
