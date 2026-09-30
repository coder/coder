package cli_test

import (
	"fmt"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/cli/clitest"
	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbfake"
	"github.com/coder/coder/v2/coderd/database/dbtestutil"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/provisioner/echo"
	"github.com/coder/coder/v2/provisionersdk/proto"
	"github.com/coder/coder/v2/testutil"
)

func TestWorkspaceBuildTemplateVersion(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name        string
		command     string
		startPin    bool
		stopPin     bool
		stopped     bool
		sameVersion bool
		missing     bool
	}{
		{name: "Stop", command: "stop", startPin: true},
		{name: "StartRunning", command: "start", startPin: true},
		{name: "StartStopped", command: "start", startPin: true, stopped: true},
		{name: "StartSameVersion", command: "start", startPin: true, sameVersion: true},
		{name: "UpdateStart", command: "update", startPin: true},
		{name: "UpdateStop", command: "update", stopPin: true},
		{name: "UpdateBoth", command: "update", startPin: true, stopPin: true},
		{name: "UpdateStopped", command: "update", startPin: true, stopPin: true, stopped: true},
		{name: "UpdateMissingStart", command: "update", startPin: true, stopPin: true, missing: true},
		{name: "UpdateMissingStop", command: "update", stopPin: true, missing: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			client := coderdtest.New(t, &coderdtest.Options{IncludeProvisionerDaemon: true})
			owner := coderdtest.CreateFirstUser(t, client)
			member, _ := coderdtest.CreateAnotherUser(t, client, owner.OrganizationID)
			version := coderdtest.CreateTemplateVersion(t, client, owner.OrganizationID, nil)
			coderdtest.AwaitTemplateVersionJobCompleted(t, client, version.ID)
			template := coderdtest.CreateTemplate(t, client, owner.OrganizationID, version.ID)
			workspace := coderdtest.CreateWorkspace(t, member, template.ID, func(req *codersdk.CreateWorkspaceRequest) {
				req.AutomaticUpdates = codersdk.AutomaticUpdatesAlways
			})
			coderdtest.AwaitWorkspaceBuildJobCompleted(t, client, workspace.LatestBuild.ID)
			if tc.stopped {
				coderdtest.MustTransitionWorkspace(t, member, workspace.ID,
					codersdk.WorkspaceTransitionStart, codersdk.WorkspaceTransitionStop)
			}

			target := coderdtest.CreateTemplateVersion(t, client, owner.OrganizationID,
				prepareEchoResponses([]*proto.RichParameter{
					{Name: "target_parameter", Type: "string", Mutable: true, DefaultValue: "target"},
				}), func(req *codersdk.CreateTemplateVersionRequest) {
					req.TemplateID = template.ID
				})
			coderdtest.AwaitTemplateVersionJobCompleted(t, client, target.ID)
			stopTarget := coderdtest.CreateTemplateVersion(t, client, owner.OrganizationID, nil,
				func(req *codersdk.CreateTemplateVersionRequest) {
					req.TemplateID = template.ID
				})
			coderdtest.AwaitTemplateVersionJobCompleted(t, client, stopTarget.ID)

			// Keep the workspace current so explicit update flags must bypass
			// the up-to-date shortcut. Targets remain non-active.
			ctx := testutil.Context(t, testutil.WaitLong)
			workspace, err := client.Workspace(ctx, workspace.ID)
			require.NoError(t, err)
			require.False(t, workspace.Outdated)
			before := workspace.LatestBuild.BuildNumber

			args := []string{tc.command, workspace.Name}
			if tc.command != "update" {
				args = append(args, "-y")
			}
			startID := version.ID
			if tc.startPin {
				name := target.Name
				startID = target.ID
				if tc.sameVersion {
					name = version.Name
					startID = version.ID
				}
				if tc.missing {
					name = uuid.NewString()
				}
				args = append(args, "--template-version", name)
			}
			if tc.stopPin {
				name := stopTarget.Name
				if tc.missing && !tc.startPin {
					name = uuid.NewString()
				}
				args = append(args, "--stop-template-version", name)
			}
			if tc.command != "stop" {
				args = append(args, "--use-parameter-defaults")
				if tc.startPin && !tc.sameVersion && !tc.missing {
					args = append(args, "--parameter", "target_parameter=chosen")
				}
			}
			inv, root := clitest.New(t, args...)
			clitest.SetupConfig(t, member, root)
			err = inv.Run()
			if tc.missing {
				require.Error(t, err)
				require.Contains(t, err.Error(), "get template version by name")
				after, err := client.Workspace(ctx, workspace.ID)
				require.NoError(t, err)
				require.Equal(t, before, after.LatestBuild.BuildNumber)
				return
			}
			require.NoError(t, err)
			after, err := client.Workspace(ctx, workspace.ID)
			require.NoError(t, err)

			if tc.command == "stop" {
				require.Equal(t, before+1, after.LatestBuild.BuildNumber)
				require.Equal(t, codersdk.WorkspaceTransitionStop, after.LatestBuild.Transition)
				require.Equal(t, target.ID, after.LatestBuild.TemplateVersionID)
				return
			}
			expectedBuilds := before + 1
			if tc.command == "update" && !tc.stopped {
				expectedBuilds++
				stopBuild, err := client.WorkspaceBuildByUsernameAndWorkspaceNameAndBuildNumber(
					ctx, workspace.OwnerName, workspace.Name, fmt.Sprint(before+1))
				require.NoError(t, err)
				require.Equal(t, codersdk.WorkspaceTransitionStop, stopBuild.Transition)
				stopID := version.ID
				if tc.stopPin {
					stopID = stopTarget.ID
				}
				require.Equal(t, stopID, stopBuild.TemplateVersionID)
			}
			require.Equal(t, expectedBuilds, after.LatestBuild.BuildNumber)
			require.Equal(t, codersdk.WorkspaceTransitionStart, after.LatestBuild.Transition)
			require.Equal(t, startID, after.LatestBuild.TemplateVersionID)
			if startID == target.ID {
				parameters, err := client.WorkspaceBuildParameters(ctx, after.LatestBuild.ID)
				require.NoError(t, err)
				require.Contains(t, parameters, codersdk.WorkspaceBuildParameter{
					Name: "target_parameter", Value: "chosen",
				})
			}
		})
	}
}

func TestTemplateVersionClassicParameterContinuity(t *testing.T) {
	t.Parallel()
	for _, required := range []bool{false, true} {
		for _, mode := range []string{"Update", "SeparateCommands", "InterruptedUpdate"} {
			t.Run(fmt.Sprintf("%s/Required=%t", mode, required), func(t *testing.T) {
				t.Parallel()
				client := coderdtest.New(t, &coderdtest.Options{IncludeProvisionerDaemon: true})
				owner := coderdtest.CreateFirstUser(t, client)
				member, _ := coderdtest.CreateAnotherUser(t, client, owner.OrganizationID)
				prior := coderdtest.CreateTemplateVersion(t, client, owner.OrganizationID,
					prepareEchoResponses([]*proto.RichParameter{{
						Name: "disk_size", Type: "string", Required: required, DefaultValue: "64",
					}}))
				coderdtest.AwaitTemplateVersionJobCompleted(t, client, prior.ID)
				template := coderdtest.CreateTemplate(t, client, owner.OrganizationID, prior.ID,
					func(req *codersdk.CreateTemplateRequest) {
						classic := true
						req.UseClassicParameterFlow = &classic
					})
				workspace := coderdtest.CreateWorkspace(t, member, template.ID,
					func(req *codersdk.CreateWorkspaceRequest) {
						req.RichParameterValues = []codersdk.WorkspaceBuildParameter{
							{Name: "disk_size", Value: "128"},
						}
					})
				coderdtest.AwaitWorkspaceBuildJobCompleted(t, client, workspace.LatestBuild.ID)
				stopTarget := coderdtest.CreateTemplateVersion(t, client, owner.OrganizationID, nil,
					func(req *codersdk.CreateTemplateVersionRequest) { req.TemplateID = template.ID })
				coderdtest.AwaitTemplateVersionJobCompleted(t, client, stopTarget.ID)
				run := func(args ...string) error {
					inv, root := clitest.New(t, args...)
					clitest.SetupConfig(t, member, root)
					return inv.Run()
				}
				if mode == "SeparateCommands" {
					require.NoError(t, run("stop", workspace.Name, "--template-version", stopTarget.Name, "-y"))
				} else {
					args := []string{
						"update", workspace.Name, "--stop-template-version", stopTarget.Name,
						"--use-parameter-defaults",
					}
					if mode == "InterruptedUpdate" {
						args = append(args, "--parameter", "unknown=value")
						err := run(args...)
						require.Error(t, err)
						require.Contains(t, err.Error(), "workspace is stopped")
					} else {
						require.NoError(t, run(args...))
					}
				}
				ctx := testutil.Context(t, testutil.WaitLong)
				stop, err := client.WorkspaceBuildByUsernameAndWorkspaceNameAndBuildNumber(
					ctx, workspace.OwnerName, workspace.Name, "2")
				require.NoError(t, err)
				require.Equal(t, stopTarget.ID, stop.TemplateVersionID)
				require.Equal(t, codersdk.ProvisionerJobSucceeded, stop.Job.Status)
				params, err := client.WorkspaceBuildParameters(ctx, stop.ID)
				require.NoError(t, err)
				require.Contains(t, params, codersdk.WorkspaceBuildParameter{Name: "disk_size", Value: "128"})
				if mode != "Update" {
					require.NoError(t, run("start", workspace.Name, "--template-version", prior.Name,
						"--use-parameter-defaults", "-y"))
				}
				after := coderdtest.MustWorkspace(t, client, workspace.ID)
				require.Equal(t, int32(3), after.LatestBuild.BuildNumber)
				require.Equal(t, prior.ID, after.LatestBuild.TemplateVersionID)
				require.Equal(t, codersdk.WorkspaceStatusRunning, after.LatestBuild.Status)
				params, err = client.WorkspaceBuildParameters(ctx, after.LatestBuild.ID)
				require.NoError(t, err)
				require.Contains(t, params, codersdk.WorkspaceBuildParameter{Name: "disk_size", Value: "128"})
			})
		}
	}
}

func TestUpdateTemplateVersionApplyFailure(t *testing.T) {
	t.Parallel()
	for _, pinned := range []bool{false, true} {
		t.Run(fmt.Sprint(pinned), func(t *testing.T) {
			t.Parallel()
			client := coderdtest.New(t, &coderdtest.Options{IncludeProvisionerDaemon: true})
			owner := coderdtest.CreateFirstUser(t, client)
			member, _ := coderdtest.CreateAnotherUser(t, client, owner.OrganizationID)
			prior := coderdtest.CreateTemplateVersion(t, client, owner.OrganizationID, nil)
			coderdtest.AwaitTemplateVersionJobCompleted(t, client, prior.ID)
			template := coderdtest.CreateTemplate(t, client, owner.OrganizationID, prior.ID)
			workspace := coderdtest.CreateWorkspace(t, member, template.ID)
			coderdtest.AwaitWorkspaceBuildJobCompleted(t, client, workspace.LatestBuild.ID)

			const failure = "real start apply failed"
			const partialState = "partial start state"
			target := coderdtest.CreateTemplateVersion(t, client, owner.OrganizationID, &echo.Responses{
				Parse: echo.ParseComplete, ProvisionInit: echo.InitComplete,
				ProvisionPlan: echo.PlanComplete, ProvisionGraph: echo.GraphComplete,
				ProvisionApply: echo.ApplyComplete,
				ProvisionApplyMap: map[proto.WorkspaceTransition][]*proto.Response{
					proto.WorkspaceTransition_START: {
						{Type: &proto.Response_Apply{Apply: &proto.ApplyComplete{
							Error: failure, State: []byte(partialState),
						}}},
					},
				},
			}, func(req *codersdk.CreateTemplateVersionRequest) {
				req.TemplateID = template.ID
			})
			coderdtest.AwaitTemplateVersionJobCompleted(t, client, target.ID)
			args := []string{"update", workspace.Name, "--use-parameter-defaults"}
			if pinned {
				args = append(args, "--template-version", target.Name)
			} else {
				coderdtest.UpdateActiveTemplateVersion(t, client, template.ID, target.ID)
			}
			inv, root := clitest.New(t, args...)
			clitest.SetupConfig(t, member, root)
			err := inv.Run()
			require.Error(t, err)
			require.Contains(t, err.Error(), failure)
			require.Contains(t, err.Error(), "workspace did not restart successfully")
			require.Contains(t, err.Error(), "run coder start to retry")

			ctx := testutil.Context(t, testutil.WaitLong)
			after := coderdtest.MustWorkspace(t, client, workspace.ID)
			require.Equal(t, int32(3), after.LatestBuild.BuildNumber)
			require.Equal(t, target.ID, after.LatestBuild.TemplateVersionID)
			require.Equal(t, codersdk.WorkspaceStatusFailed, after.LatestBuild.Status)
			require.Equal(t, failure, after.LatestBuild.Job.Error)
			state, err := client.WorkspaceBuildState(ctx, after.LatestBuild.ID)
			require.NoError(t, err)
			require.Equal(t, partialState, string(state))
			stop, err := client.WorkspaceBuildByUsernameAndWorkspaceNameAndBuildNumber(
				ctx, workspace.OwnerName, workspace.Name, "2")
			require.NoError(t, err)
			require.Equal(t, prior.ID, stop.TemplateVersionID)
			require.Equal(t, codersdk.WorkspaceTransitionStop, stop.Transition)
			require.Equal(t, codersdk.ProvisionerJobSucceeded, stop.Job.Status)
		})
	}
}

func TestStartTemplateVersionBuildInProgress(t *testing.T) {
	t.Parallel()
	for _, starting := range []bool{false, true} {
		t.Run(fmt.Sprint(starting), func(t *testing.T) {
			t.Parallel()
			store, ps := dbtestutil.NewDB(t)
			client := coderdtest.New(t, &coderdtest.Options{Database: store, Pubsub: ps})
			owner := coderdtest.CreateFirstUser(t, client)
			member, memberUser := coderdtest.CreateAnotherUser(t, client, owner.OrganizationID)
			builder := dbfake.WorkspaceBuild(t, store, database.WorkspaceTable{
				OwnerID: memberUser.ID, OrganizationID: owner.OrganizationID,
			})
			if starting {
				builder = builder.Starting()
			} else {
				builder = builder.Pending()
			}
			ws := builder.Do()
			ctx := testutil.Context(t, testutil.WaitShort)
			version, err := client.TemplateVersion(ctx, ws.Build.TemplateVersionID)
			require.NoError(t, err)
			inv, root := clitest.New(t, "start", ws.Workspace.Name,
				"--template-version", version.Name, "-y")
			clitest.SetupConfig(t, member, root)
			err = inv.Run()
			require.Error(t, err)
			require.Contains(t, err.Error(), "wait for the current build to finish")
			after := coderdtest.MustWorkspace(t, client, ws.Workspace.ID)
			require.Equal(t, ws.Build.ID, after.LatestBuild.ID)
		})
	}
}
