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
}

func prepareEchoResponses(parameters []*proto.RichParameter, presets ...*proto.Preset) *echo.Responses {
	return &echo.Responses{
		Parse:         echo.ParseComplete,
		ProvisionInit: echo.InitComplete,
		ProvisionPlan: echo.PlanComplete,
		ProvisionGraph: []*proto.Response{
			{
				Type: &proto.Response_Graph{
					Graph: &proto.GraphComplete{
						Parameters: parameters,
						Presets:    presets,
					},
				},
			},
		},
		ProvisionApply: echo.ApplyComplete,
	}
}

type param struct {
	name    string
	ptype   string
	value   string
	mutable bool
}

func TestCreateWithRichParameters(t *testing.T) {
	t.Parallel()

	// Default parameters and their expected values.
	params := []param{
		{
			name:    "number_param",
			ptype:   "number",
			value:   "777",
			mutable: true,
		},
		{
			name:    "string_param",
			ptype:   "string",
			value:   "qux",
			mutable: true,
		},
		{
			name: "bool_param",
			// TODO: Setting the type breaks booleans.  It claims the default is false
			// but when you then accept this default it errors saying that the value
			// must be true or false.  For now, use a string.
			ptype:   "string",
			value:   "false",
			mutable: true,
		},
		{
			name:    "immutable_string_param",
			ptype:   "string",
			value:   "i am eternal",
			mutable: false,
		},
	}

	type testContext struct {
		client        *codersdk.Client
		member        *codersdk.Client
		owner         codersdk.CreateFirstUserResponse
		template      codersdk.Template
		workspaceName string
	}

	tests := []struct {
		name string
		// setup runs before the command is started and return arguments that will
		// be appended to the create command.
		setup func() []string
		// handlePty optionally runs after the command is started.  It should handle
		// all expected prompts from the pty.
		handlePty func(pty *ptytest.PTY)
		// postRun runs after the command has finished but before the workspace is
		// verified.  It must return the workspace name to check (used for the copy
		// workspace tests).
		postRun func(t *testing.T, args testContext) string
		// errors contains expected errors.  The workspace will not be verified if
		// errors are expected.
		errors []string
		// inputParameters overrides the default parameters.
		inputParameters []param
		// expectedParameters defaults to inputParameters.
		expectedParameters []param
		// withDefaults sets DefaultValue to each parameter's value.
		withDefaults bool
	}{
		{
			name: "ValuesFromPrompt",
			handlePty: func(pty *ptytest.PTY) {
				// Enter the value for each parameter as prompted.
				for _, param := range params {
					pty.ExpectMatch(param.name)
					pty.WriteLine(param.value)
				}
				// Confirm the creation.
				pty.ExpectMatch("Confirm create?")
				pty.WriteLine("yes")
			},
		},
		{
			name: "ValuesFromDefaultFlags",
			setup: func() []string {
				// Provide the defaults on the command line.
				args := []string{}
				for _, param := range params {
					args = append(args, "--parameter-default", fmt.Sprintf("%s=%s", param.name, param.value))
				}
				return args
			},
			handlePty: func(pty *ptytest.PTY) {
				// Simply accept the defaults.
				for _, param := range params {
					pty.ExpectMatch(param.name)
					pty.ExpectMatch(`Enter a value (default: "` + param.value + `")`)
					pty.WriteLine("")
				}
				// Confirm the creation.
				pty.ExpectMatch("Confirm create?")
				pty.WriteLine("yes")
			},
		},
		{
			name: "ValuesFromFile",
			setup: func() []string {
				// Create a file with the values.
				tempDir := t.TempDir()
				removeTmpDirUntilSuccessAfterTest(t, tempDir)
				parameterFile, _ := os.CreateTemp(tempDir, "testParameterFile*.yaml")
				for _, param := range params {
					_, err := parameterFile.WriteString(fmt.Sprintf("%s: %s\n", param.name, param.value))
					require.NoError(t, err)
				}

				return []string{"--rich-parameter-file", parameterFile.Name()}
			},
			handlePty: func(pty *ptytest.PTY) {
				// No prompts, we only need to confirm.
				pty.ExpectMatch("Confirm create?")
				pty.WriteLine("yes")
			},
		},
		{
			name: "ValuesFromFlags",
			setup: func() []string {
				// Provide the values on the command line.
				var args []string
				for _, param := range params {
					args = append(args, "--parameter", fmt.Sprintf("%s=%s", param.name, param.value))
				}
				return args
			},
			handlePty: func(pty *ptytest.PTY) {
				// No prompts, we only need to confirm.
				pty.ExpectMatch("Confirm create?")
				pty.WriteLine("yes")
			},
		},
		{
			name: "MisspelledParameter",
			setup: func() []string {
				// Provide the values on the command line.
				args := []string{}
				for i, param := range params {
					if i == 0 {
						// Slightly misspell the first parameter with an extra character.
						args = append(args, "--parameter", fmt.Sprintf("n%s=%s", param.name, param.value))
					} else {
						args = append(args, "--parameter", fmt.Sprintf("%s=%s", param.name, param.value))
					}
				}
				return args
			},
			errors: []string{
				"parameter \"n" + params[0].name + "\" is not present in the template",
				"Did you mean: " + params[0].name,
			},
		},
		{
			name: "ValuesFromWorkspace",
			setup: func() []string {
				// Provide the values on the command line.
				args := []string{"-y"}
				for _, param := range params {
					args = append(args, "--parameter", fmt.Sprintf("%s=%s", param.name, param.value))
				}
				return args
			},
			postRun: func(t *testing.T, tctx testContext) string {
				inv, root := clitest.New(t, "create", "--copy-parameters-from", tctx.workspaceName, "other-workspace", "-y")
				clitest.SetupConfig(t, tctx.member, root)
				pty := ptytest.New(t).Attach(inv)
				inv.Stdout = pty.Output()
				inv.Stderr = pty.Output()
				err := inv.Run()
				require.NoError(t, err, "failed to create a workspace based on the source workspace")
				return "other-workspace"
			},
		},
		{
			name: "ValuesFromOutdatedWorkspace",
			setup: func() []string {
				// Provide the values on the command line.
				args := []string{"-y"}
				for _, param := range params {
					args = append(args, "--parameter", fmt.Sprintf("%s=%s", param.name, param.value))
				}
				return args
			},
			postRun: func(t *testing.T, tctx testContext) string {
				// Update the template to a new version.
				version2 := coderdtest.CreateTemplateVersion(t, tctx.client, tctx.owner.OrganizationID, prepareEchoResponses([]*proto.RichParameter{
					{Name: "another_parameter", Type: "string", DefaultValue: "not-relevant"},
				}), func(ctvr *codersdk.CreateTemplateVersionRequest) {
					ctvr.TemplateID = tctx.template.ID
				})
				coderdtest.AwaitTemplateVersionJobCompleted(t, tctx.client, version2.ID)
				coderdtest.UpdateActiveTemplateVersion(t, tctx.client, tctx.template.ID, version2.ID)

				// Then create the copy.  It should use the old template version.
				inv, root := clitest.New(t, "create", "--copy-parameters-from", tctx.workspaceName, "other-workspace", "-y")
				clitest.SetupConfig(t, tctx.member, root)
				pty := ptytest.New(t).Attach(inv)
				inv.Stdout = pty.Output()
				inv.Stderr = pty.Output()
				err := inv.Run()
				require.NoError(t, err, "failed to create a workspace based on the source workspace")
				return "other-workspace"
			},
		},
		{
			name: "ValuesFromTemplateDefaults",
			handlePty: func(pty *ptytest.PTY) {
				// Simply accept the defaults.
				for _, param := range params {
					pty.ExpectMatch(param.name)
					pty.ExpectMatch(`Enter a value (default: "` + param.value + `")`)
					pty.WriteLine("")
				}
				// Confirm the creation.
				pty.ExpectMatch("Confirm create?")
				pty.WriteLine("yes")
			},
			withDefaults: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			parameters := params
			if len(tt.inputParameters) > 0 {
				parameters = tt.inputParameters
			}

			// Convert parameters for the echo provisioner response.
			var rparams []*proto.RichParameter
			for i, param := range parameters {
				defaultValue := ""
				if tt.withDefaults {
					defaultValue = param.value
				}
				rparams = append(rparams, &proto.RichParameter{
					Name:         param.name,
					Type:         param.ptype,
					Mutable:      param.mutable,
					DefaultValue: defaultValue,
					Order:        int32(i), //nolint:gosec
				})
			}

			// Set up the template.
			client := coderdtest.New(t, &coderdtest.Options{IncludeProvisionerDaemon: true})
			owner := coderdtest.CreateFirstUser(t, client)
			member, _ := coderdtest.CreateAnotherUser(t, client, owner.OrganizationID)
			version := coderdtest.CreateTemplateVersion(t, client, owner.OrganizationID, prepareEchoResponses(rparams))

			coderdtest.AwaitTemplateVersionJobCompleted(t, client, version.ID)
			template := coderdtest.CreateTemplate(t, client, owner.OrganizationID, version.ID)

			// Run the command, possibly setting up values.
			workspaceName := "my-workspace"
			args := []string{"create", workspaceName, "--template", template.Name}
			if tt.setup != nil {
				args = append(args, tt.setup()...)
			}
			inv, root := clitest.New(t, args...)
			clitest.SetupConfig(t, member, root)
			doneChan := make(chan error)
			pty := ptytest.New(t).Attach(inv)
			go func() {
				doneChan <- inv.Run()
			}()

			// The test may do something with the pty.
			if tt.handlePty != nil {
				tt.handlePty(pty)
			}

			// Wait for the command to exit.
			err := <-doneChan

			// The test may want to run additional setup like copying the workspace.
			if tt.postRun != nil {
				workspaceName = tt.postRun(t, testContext{
					client:        client,
					member:        member,
					owner:         owner,
					template:      template,
					workspaceName: workspaceName,
				})
			}

			if len(tt.errors) > 0 {
				require.Error(t, err)
				for _, errstr := range tt.errors {
					assert.ErrorContains(t, err, errstr)
				}
			} else {
				require.NoError(t, err)

				// Verify the workspace was created and has the right template and values.
				ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitShort)
				defer cancel()

				workspaces, err := client.Workspaces(ctx, codersdk.WorkspaceFilter{Name: workspaceName})
				require.NoError(t, err, "expected to find created workspace")
				require.Len(t, workspaces.Workspaces, 1)

				workspaceLatestBuild := workspaces.Workspaces[0].LatestBuild
				require.Equal(t, version.ID, workspaceLatestBuild.TemplateVersionID)

				buildParameters, err := client.WorkspaceBuildParameters(ctx, workspaceLatestBuild.ID)
				require.NoError(t, err)
				if len(tt.expectedParameters) > 0 {
					parameters = tt.expectedParameters
				}
				require.Len(t, buildParameters, len(parameters))
				for _, param := range parameters {
					require.Contains(t, buildParameters, codersdk.WorkspaceBuildParameter{Name: param.name, Value: param.value})
				}
			}
		})
	}
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
		Parse:         echo.ParseComplete,
		ProvisionInit: echo.InitComplete,
		ProvisionPlan: echo.PlanComplete,
		ProvisionGraph: []*proto.Response{
			{
				Type: &proto.Response_Graph{
					Graph: &proto.GraphComplete{
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
