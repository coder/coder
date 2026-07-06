package coderd_test

import (
	"encoding/json"
	"net/http"
	"strconv"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbgen"
	"github.com/coder/coder/v2/coderd/database/dbtestutil"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/provisioner/echo"
	"github.com/coder/coder/v2/provisionersdk/proto"
	"github.com/coder/coder/v2/testutil"
)

func TestPostWorkspaceBuildsOnSuccessRestart(t *testing.T) {
	t.Parallel()

	const paramName = "foo"

	// GIVEN: a running workspace with an existing rich parameter value.
	deploymentValues := coderdtest.DeploymentValues(t)
	deploymentValues.EnableTerraformDebugMode = true
	db, ps, sqlDB := dbtestutil.NewDBWithSQLDB(t)
	client, _, _ := coderdtest.NewWithAPI(t, &coderdtest.Options{
		Database:                 db,
		Pubsub:                   ps,
		IncludeProvisionerDaemon: true,
		DeploymentValues:         deploymentValues,
	})
	first := coderdtest.CreateFirstUser(t, client)
	version := coderdtest.CreateTemplateVersion(t, client, first.OrganizationID,
		echoResponsesWithRichParameter(paramName, echoResponseOptions{
			blockStopApply: false,
		}),
	)
	coderdtest.AwaitTemplateVersionJobCompleted(t, client, version.ID)
	template := coderdtest.CreateTemplate(t, client, first.OrganizationID, version.ID)
	workspace := coderdtest.CreateWorkspace(t, client, template.ID, func(request *codersdk.CreateWorkspaceRequest) {
		request.RichParameterValues = []codersdk.WorkspaceBuildParameter{
			{Name: paramName, Value: "bar"},
		}
	})
	initialBuild := coderdtest.AwaitWorkspaceBuildJobCompleted(t, client, workspace.LatestBuild.ID)
	require.Equal(t, codersdk.WorkspaceStatusRunning, initialBuild.Status)

	// WHEN: a stop build is created with an on_success start build.
	ctx := testutil.Context(t, testutil.WaitLong)
	user, err := client.User(ctx, codersdk.Me)
	require.NoError(t, err)

	stopBuild, err := client.CreateWorkspaceBuild(ctx, workspace.ID, codersdk.CreateWorkspaceBuildRequest{
		Transition: codersdk.WorkspaceTransitionStop,
		Reason:     codersdk.CreateWorkspaceBuildReasonCLI,
		LogLevel:   codersdk.ProvisionerLogLevelDebug,
		OnSuccess: &codersdk.CreateWorkspaceBuildOnSuccessRequest{
			Transition:        codersdk.WorkspaceTransitionStart,
			TemplateVersionID: template.ActiveVersionID,
			RichParameterValues: []codersdk.WorkspaceBuildParameter{
				{Name: paramName, Value: "baz"},
			},
		},
	})
	require.NoError(t, err)
	require.Equal(t, codersdk.WorkspaceTransitionStop, stopBuild.Transition)
	require.Equal(t, codersdk.BuildReasonCLI, stopBuild.Reason)

	// THEN: the server persists the child start build intent.
	orchestration, err := dbtestutil.GetWorkspaceBuildOrchestrationByParentBuildID(ctx, sqlDB, stopBuild.ID)
	require.NoError(t, err)
	require.Equal(t, "pending", orchestration.Status)
	require.Equal(t, codersdk.WorkspaceTransitionStart, codersdk.WorkspaceTransition(orchestration.ChildTransition))
	require.True(t, orchestration.ChildTemplateVersionID.Valid)
	require.Equal(t, template.ActiveVersionID, orchestration.ChildTemplateVersionID.UUID)
	require.False(t, orchestration.ChildTemplateVersionPresetID.Valid)
	require.Equal(t, string(codersdk.ProvisionerLogLevelDebug), orchestration.ChildLogLevel)
	require.True(t, orchestration.ChildReason.Valid)
	require.Equal(t, codersdk.BuildReasonCLI, codersdk.BuildReason(orchestration.ChildReason.BuildReason))

	var childRichParameterValues []codersdk.WorkspaceBuildParameter
	require.NoError(t, json.Unmarshal(orchestration.ChildRichParameterValues, &childRichParameterValues))
	require.ElementsMatch(t, []codersdk.WorkspaceBuildParameter{
		{Name: paramName, Value: "baz"},
	}, childRichParameterValues)

	// THEN: the returned parent stop build completes successfully.
	stopBuild = coderdtest.AwaitWorkspaceBuildJobCompleted(t, client, stopBuild.ID)
	require.Equal(t, codersdk.ProvisionerJobSucceeded, stopBuild.Job.Status)
	require.Equal(t, codersdk.WorkspaceStatusStopped, stopBuild.Status)

	// THEN: the server creates and completes the child start build.
	var childBuild codersdk.WorkspaceBuild
	require.Eventually(t, func() bool {
		childBuild, err = client.WorkspaceBuildByUsernameAndWorkspaceNameAndBuildNumber(
			ctx,
			user.Username,
			workspace.Name,
			strconv.FormatInt(int64(stopBuild.BuildNumber+1), 10),
		)
		return err == nil &&
			childBuild.Transition == codersdk.WorkspaceTransitionStart
	}, testutil.WaitMedium, testutil.IntervalFast)

	childBuild = coderdtest.AwaitWorkspaceBuildJobCompleted(t, client, childBuild.ID)
	require.Equal(t, codersdk.ProvisionerJobSucceeded, childBuild.Job.Status)
	require.Equal(t, codersdk.WorkspaceStatusRunning, childBuild.Status)
	require.Equal(t, codersdk.BuildReasonCLI, childBuild.Reason)
	require.Equal(t, template.ActiveVersionID, childBuild.TemplateVersionID)

	// THEN: the child build uses the on_success parameter values.
	params, err := client.WorkspaceBuildParameters(ctx, childBuild.ID)
	require.NoError(t, err)
	require.ElementsMatch(t, []codersdk.WorkspaceBuildParameter{
		{Name: paramName, Value: "baz"},
	}, params)
}

func TestPostWorkspaceBuildsOnSuccessTemplateVersionPreset(t *testing.T) {
	t.Parallel()

	// GIVEN: a running workspace and a preset on its active template
	// version.
	db, ps, sqlDB := dbtestutil.NewDBWithSQLDB(t)
	client, _, _ := coderdtest.NewWithAPI(t, &coderdtest.Options{
		Database:                 db,
		Pubsub:                   ps,
		IncludeProvisionerDaemon: true,
	})
	first := coderdtest.CreateFirstUser(t, client)
	version := coderdtest.CreateTemplateVersion(t, client, first.OrganizationID, nil)
	coderdtest.AwaitTemplateVersionJobCompleted(t, client, version.ID)
	template := coderdtest.CreateTemplate(t, client, first.OrganizationID, version.ID)
	preset := dbgen.Preset(t, db, database.InsertPresetParams{
		Name:              "on-success-preset",
		TemplateVersionID: version.ID,
	})
	workspace := coderdtest.CreateWorkspace(t, client, template.ID)
	initialBuild := coderdtest.AwaitWorkspaceBuildJobCompleted(t, client, workspace.LatestBuild.ID)
	require.Equal(t, codersdk.WorkspaceStatusRunning, initialBuild.Status)

	// WHEN: a stop build is created with an on_success start build
	// that requests the preset.
	ctx := testutil.Context(t, testutil.WaitLong)
	user, err := client.User(ctx, codersdk.Me)
	require.NoError(t, err)

	stopBuild, err := client.CreateWorkspaceBuild(ctx, workspace.ID, codersdk.CreateWorkspaceBuildRequest{
		Transition: codersdk.WorkspaceTransitionStop,
		OnSuccess: &codersdk.CreateWorkspaceBuildOnSuccessRequest{
			Transition:              codersdk.WorkspaceTransitionStart,
			TemplateVersionID:       template.ActiveVersionID,
			TemplateVersionPresetID: preset.ID,
		},
	})
	require.NoError(t, err)

	// THEN: the server persists the child preset intent.
	orchestration, err := dbtestutil.GetWorkspaceBuildOrchestrationByParentBuildID(ctx, sqlDB, stopBuild.ID)
	require.NoError(t, err)
	require.True(t, orchestration.ChildTemplateVersionPresetID.Valid)
	require.Equal(t, preset.ID, orchestration.ChildTemplateVersionPresetID.UUID)

	stopBuild = coderdtest.AwaitWorkspaceBuildJobCompleted(t, client, stopBuild.ID)
	require.Equal(t, codersdk.ProvisionerJobSucceeded, stopBuild.Job.Status)
	require.Equal(t, codersdk.WorkspaceStatusStopped, stopBuild.Status)

	// THEN: the child start build uses the preset.
	var childBuild codersdk.WorkspaceBuild
	require.Eventually(t, func() bool {
		childBuild, err = client.WorkspaceBuildByUsernameAndWorkspaceNameAndBuildNumber(
			ctx,
			user.Username,
			workspace.Name,
			strconv.FormatInt(int64(stopBuild.BuildNumber+1), 10),
		)
		return err == nil &&
			childBuild.Transition == codersdk.WorkspaceTransitionStart
	}, testutil.WaitShort, testutil.IntervalFast)

	childBuild = coderdtest.AwaitWorkspaceBuildJobCompleted(t, client, childBuild.ID)
	require.Equal(t, codersdk.ProvisionerJobSucceeded, childBuild.Job.Status)
	require.Equal(t, codersdk.WorkspaceStatusRunning, childBuild.Status)
	require.NotNil(t, childBuild.TemplateVersionPresetID)
	require.Equal(t, preset.ID, *childBuild.TemplateVersionPresetID)
}

func TestPostWorkspaceBuildsOnSuccessUnpinnedChildUsesActiveTemplateVersion(t *testing.T) {
	t.Parallel()

	// GIVEN: a running workspace and a second completed template
	// version that is not active yet.
	db, ps, sqlDB := dbtestutil.NewDBWithSQLDB(t)
	client, provisionerCloser, api := coderdtest.NewWithAPI(t, &coderdtest.Options{
		Database:                 db,
		Pubsub:                   ps,
		IncludeProvisionerDaemon: true,
	})
	first := coderdtest.CreateFirstUser(t, client)
	userClient, user := coderdtest.CreateAnotherUser(t, client, first.OrganizationID)

	version := coderdtest.CreateTemplateVersion(t, client, first.OrganizationID, nil)
	coderdtest.AwaitTemplateVersionJobCompleted(t, client, version.ID)
	template := coderdtest.CreateTemplate(t, client, first.OrganizationID, version.ID)
	workspace := coderdtest.CreateWorkspace(t, userClient, template.ID)
	initialBuild := coderdtest.AwaitWorkspaceBuildJobCompleted(t, userClient, workspace.LatestBuild.ID)
	require.Equal(t, codersdk.WorkspaceStatusRunning, initialBuild.Status)

	newVersion := coderdtest.UpdateTemplateVersion(t, client, first.OrganizationID, nil, template.ID)
	coderdtest.AwaitTemplateVersionJobCompleted(t, client, newVersion.ID)

	// WHEN: a non-template-admin queues an unpinned on_success child
	// build.

	// Stop the provisioner so the parent build cannot complete before
	// the test updates the active template version.
	require.NoError(t, provisionerCloser.Close())
	ctx := testutil.Context(t, testutil.WaitLong)
	stopBuild, err := userClient.CreateWorkspaceBuild(ctx, workspace.ID, codersdk.CreateWorkspaceBuildRequest{
		Transition: codersdk.WorkspaceTransitionStop,
		OnSuccess: &codersdk.CreateWorkspaceBuildOnSuccessRequest{
			Transition: codersdk.WorkspaceTransitionStart,
		},
	})
	require.NoError(t, err)

	// THEN: the child build remains unpinned in the orchestration row.
	orchestration, err := dbtestutil.GetWorkspaceBuildOrchestrationByParentBuildID(ctx, sqlDB, stopBuild.ID)
	require.NoError(t, err)
	require.False(t, orchestration.ChildTemplateVersionID.Valid, "child build should remain unpinned")

	// WHEN: the active version changes before the parent succeeds.
	coderdtest.UpdateActiveTemplateVersion(t, client, template.ID, newVersion.ID)
	coderdtest.NewProvisionerDaemon(t, api)

	stopBuild = coderdtest.AwaitWorkspaceBuildJobCompleted(t, userClient, stopBuild.ID)
	require.Equal(t, codersdk.ProvisionerJobSucceeded, stopBuild.Job.Status)
	require.Equal(t, codersdk.WorkspaceStatusStopped, stopBuild.Status)

	// THEN: the child build uses the active version when the
	// orchestrator creates it.
	var childBuild codersdk.WorkspaceBuild
	require.Eventually(t, func() bool {
		childBuild, err = userClient.WorkspaceBuildByUsernameAndWorkspaceNameAndBuildNumber(
			ctx,
			user.Username,
			workspace.Name,
			strconv.FormatInt(int64(stopBuild.BuildNumber+1), 10),
		)
		return err == nil &&
			childBuild.Transition == codersdk.WorkspaceTransitionStart
	}, testutil.WaitMedium, testutil.IntervalFast)

	childBuild = coderdtest.AwaitWorkspaceBuildJobCompleted(t, userClient, childBuild.ID)
	require.Equal(t, codersdk.ProvisionerJobSucceeded, childBuild.Job.Status)
	require.Equal(t, codersdk.WorkspaceStatusRunning, childBuild.Status)
	require.Equal(t, newVersion.ID, childBuild.TemplateVersionID)
}

func TestPostWorkspaceBuildsOnSuccessUnpinnedChildNoParams(t *testing.T) {
	t.Parallel()

	// GIVEN: a running workspace owned by a non-template-admin.
	db, ps, sqlDB := dbtestutil.NewDBWithSQLDB(t)
	client, _, _ := coderdtest.NewWithAPI(t, &coderdtest.Options{
		Database:                 db,
		Pubsub:                   ps,
		IncludeProvisionerDaemon: true,
	})
	first := coderdtest.CreateFirstUser(t, client)
	userClient, _ := coderdtest.CreateAnotherUser(t, client, first.OrganizationID)

	version := coderdtest.CreateTemplateVersion(t, client, first.OrganizationID, nil)
	coderdtest.AwaitTemplateVersionJobCompleted(t, client, version.ID)
	template := coderdtest.CreateTemplate(t, client, first.OrganizationID, version.ID)
	workspace := coderdtest.CreateWorkspace(t, userClient, template.ID)
	initialBuild := coderdtest.AwaitWorkspaceBuildJobCompleted(t, userClient, workspace.LatestBuild.ID)
	require.Equal(t, codersdk.WorkspaceStatusRunning, initialBuild.Status)

	// WHEN: the non-template-admin queues a stop build with an unpinned
	// on_success start build that supplies no parameters, reason, or log level.
	ctx := testutil.Context(t, testutil.WaitLong)
	stopBuild, err := userClient.CreateWorkspaceBuild(ctx, workspace.ID, codersdk.CreateWorkspaceBuildRequest{
		Transition: codersdk.WorkspaceTransitionStop,
		OnSuccess: &codersdk.CreateWorkspaceBuildOnSuccessRequest{
			Transition: codersdk.WorkspaceTransitionStart,
		},
	})
	// THEN: the request is permitted without template-update privileges,
	// because no durable template version pin is requested.
	require.NoError(t, err)
	require.Equal(t, codersdk.WorkspaceTransitionStop, stopBuild.Transition)

	// THEN: the persisted child build intent leaves the optional fields unset.
	orchestration, err := dbtestutil.GetWorkspaceBuildOrchestrationByParentBuildID(ctx, sqlDB, stopBuild.ID)
	require.NoError(t, err)
	require.Equal(t, "pending", orchestration.Status)
	require.Equal(t, workspace.ID, orchestration.WorkspaceID)
	require.Equal(t, codersdk.WorkspaceTransitionStart, codersdk.WorkspaceTransition(orchestration.ChildTransition))
	require.False(t, orchestration.ChildTemplateVersionID.Valid)
	require.False(t, orchestration.ChildTemplateVersionPresetID.Valid)
	require.False(t, orchestration.ChildReason.Valid)
	require.Empty(t, orchestration.ChildLogLevel)

	// THEN: nil parameters are coerced to an empty JSON array, not null, to
	// satisfy the database CHECK constraint.
	require.JSONEq(t, "[]", string(orchestration.ChildRichParameterValues))
	var childRichParameterValues []codersdk.WorkspaceBuildParameter
	require.NoError(t, json.Unmarshal(orchestration.ChildRichParameterValues, &childRichParameterValues))
	require.Empty(t, childRichParameterValues)
}

func TestPostWorkspaceBuildsOnSuccessPinnedChildVersionRequiresTemplateUpdate(t *testing.T) {
	t.Parallel()

	// GIVEN: a running workspace owned by a non-template-admin.
	client, _ := coderdtest.NewWithDatabase(t, &coderdtest.Options{IncludeProvisionerDaemon: true})
	first := coderdtest.CreateFirstUser(t, client)
	userClient, _ := coderdtest.CreateAnotherUser(t, client, first.OrganizationID)

	version := coderdtest.CreateTemplateVersion(t, client, first.OrganizationID, nil)
	coderdtest.AwaitTemplateVersionJobCompleted(t, client, version.ID)
	template := coderdtest.CreateTemplate(t, client, first.OrganizationID, version.ID)
	workspace := coderdtest.CreateWorkspace(t, userClient, template.ID)
	initialBuild := coderdtest.AwaitWorkspaceBuildJobCompleted(t, userClient, workspace.LatestBuild.ID)
	require.Equal(t, codersdk.WorkspaceStatusRunning, initialBuild.Status)

	// WHEN: the non-template-admin tries to queue a stop build with a
	// pinned on_success child version.
	ctx := testutil.Context(t, testutil.WaitLong)
	_, err := userClient.CreateWorkspaceBuild(ctx, workspace.ID, codersdk.CreateWorkspaceBuildRequest{
		Transition: codersdk.WorkspaceTransitionStop,
		OnSuccess: &codersdk.CreateWorkspaceBuildOnSuccessRequest{
			Transition:        codersdk.WorkspaceTransitionStart,
			TemplateVersionID: version.ID,
		},
	})
	require.Error(t, err)

	// THEN: the API rejects the durable child version pin and explains the
	// missing template update permission.
	var apiErr *codersdk.Error
	require.ErrorAs(t, err, &apiErr)
	require.Equal(t, http.StatusForbidden, apiErr.StatusCode())
	require.Contains(t, apiErr.Response.Detail, "template update permission")

	// THEN: no new workspace build is created.
	builds, err := userClient.WorkspaceBuilds(ctx, codersdk.WorkspaceBuildsRequest{WorkspaceID: workspace.ID})
	require.NoError(t, err)
	require.Len(t, builds, 1)
	require.Equal(t, initialBuild.ID, builds[0].ID)
}

func TestPostWorkspaceBuildsOnSuccessValidation(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		request     codersdk.CreateWorkspaceBuildRequest
		wantMessage string
	}{
		{
			name: "ParentMustBeStop",
			request: codersdk.CreateWorkspaceBuildRequest{
				Transition: codersdk.WorkspaceTransitionStart,
				OnSuccess: &codersdk.CreateWorkspaceBuildOnSuccessRequest{
					Transition: codersdk.WorkspaceTransitionStart,
				},
			},
			wantMessage: "OnSuccess is only permitted when stopping a workspace.",
		},
		{
			// The oneof=start struct tag on OnSuccess.Transition rejects this
			// during httpapi.Read, before the explicit check is reached.
			name: "ChildMustBeStart",
			request: codersdk.CreateWorkspaceBuildRequest{
				Transition: codersdk.WorkspaceTransitionStop,
				OnSuccess: &codersdk.CreateWorkspaceBuildOnSuccessRequest{
					Transition: codersdk.WorkspaceTransitionStop,
				},
			},
			wantMessage: "Validation failed.",
		},
		{
			name: "ParentDryRunRejected",
			request: codersdk.CreateWorkspaceBuildRequest{
				Transition: codersdk.WorkspaceTransitionStop,
				DryRun:     true,
				OnSuccess: &codersdk.CreateWorkspaceBuildOnSuccessRequest{
					Transition: codersdk.WorkspaceTransitionStart,
				},
			},
			wantMessage: "OnSuccess cannot be set alongside DryRun.",
		},
		{
			name: "ParentOrphanRejected",
			request: codersdk.CreateWorkspaceBuildRequest{
				Transition: codersdk.WorkspaceTransitionStop,
				Orphan:     true,
				OnSuccess: &codersdk.CreateWorkspaceBuildOnSuccessRequest{
					Transition: codersdk.WorkspaceTransitionStart,
				},
			},
			wantMessage: "OnSuccess cannot be set alongside Orphan.",
		},
		{
			name: "ParentProvisionerStateRejected",
			request: codersdk.CreateWorkspaceBuildRequest{
				Transition:       codersdk.WorkspaceTransitionStop,
				ProvisionerState: []byte("state"),
				OnSuccess: &codersdk.CreateWorkspaceBuildOnSuccessRequest{
					Transition: codersdk.WorkspaceTransitionStart,
				},
			},
			wantMessage: "OnSuccess cannot be set alongside ProvisionerState.",
		},
		{
			name: "ChildPresetWithoutVersionRejected",
			request: codersdk.CreateWorkspaceBuildRequest{
				Transition: codersdk.WorkspaceTransitionStop,
				OnSuccess: &codersdk.CreateWorkspaceBuildOnSuccessRequest{
					Transition:              codersdk.WorkspaceTransitionStart,
					TemplateVersionPresetID: uuid.New(),
				},
			},
			wantMessage: "OnSuccess TemplateVersionPresetID requires TemplateVersionID.",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			// GIVEN: a running workspace.
			client := coderdtest.New(t, &coderdtest.Options{IncludeProvisionerDaemon: true})
			first := coderdtest.CreateFirstUser(t, client)
			version := coderdtest.CreateTemplateVersion(t, client, first.OrganizationID, nil)
			coderdtest.AwaitTemplateVersionJobCompleted(t, client, version.ID)
			template := coderdtest.CreateTemplate(t, client, first.OrganizationID, version.ID)
			workspace := coderdtest.CreateWorkspace(t, client, template.ID)
			coderdtest.AwaitWorkspaceBuildJobCompleted(t, client, workspace.LatestBuild.ID)

			// WHEN: an invalid on_success request is posted.
			_, err := client.CreateWorkspaceBuild(testutil.Context(t, testutil.WaitLong), workspace.ID, tt.request)
			require.Error(t, err)

			// THEN: the API rejects the request before creating a build.
			var apiErr *codersdk.Error
			require.ErrorAs(t, err, &apiErr)
			require.Equal(t, http.StatusBadRequest, apiErr.StatusCode())
			require.Contains(t, apiErr.Message, tt.wantMessage)
		})
	}
}

// Canceling an already-running job resolves the orchestration as
// "failed", not "canceled". This hits the same orchestrator branch as
// TestPostWorkspaceBuildsOnSuccessParentFailed below; despite that
// overlap, the test pins this non-obvious end-to-end behavior.
func TestPostWorkspaceBuildsOnSuccessParentCanceledMidFlight(t *testing.T) {
	t.Parallel()

	// GIVEN: a running workspace whose stop apply will block.
	db, ps, sqlDB := dbtestutil.NewDBWithSQLDB(t)
	client, _, _ := coderdtest.NewWithAPI(t, &coderdtest.Options{
		Database:                 db,
		Pubsub:                   ps,
		IncludeProvisionerDaemon: true,
	})
	first := coderdtest.CreateFirstUser(t, client)
	version := coderdtest.CreateTemplateVersion(t, client, first.OrganizationID,
		echoResponsesWithRichParameter("foo", echoResponseOptions{
			blockStopApply: true,
		}),
	)
	coderdtest.AwaitTemplateVersionJobCompleted(t, client, version.ID)
	template := coderdtest.CreateTemplate(t, client, first.OrganizationID, version.ID)
	workspace := coderdtest.CreateWorkspace(t, client, template.ID)
	initialBuild := coderdtest.AwaitWorkspaceBuildJobCompleted(t, client, workspace.LatestBuild.ID)
	require.Equal(t, codersdk.WorkspaceStatusRunning, initialBuild.Status)

	// WHEN: a stop build is created with an on_success start build.
	ctx := testutil.Context(t, testutil.WaitLong)
	user, err := client.User(ctx, codersdk.Me)
	require.NoError(t, err)

	stopBuild, err := client.CreateWorkspaceBuild(ctx, workspace.ID, codersdk.CreateWorkspaceBuildRequest{
		Transition: codersdk.WorkspaceTransitionStop,
		OnSuccess: &codersdk.CreateWorkspaceBuildOnSuccessRequest{
			Transition:        codersdk.WorkspaceTransitionStart,
			TemplateVersionID: template.ActiveVersionID,
		},
	})
	require.NoError(t, err)
	require.Equal(t, codersdk.WorkspaceTransitionStop, stopBuild.Transition)
	require.Equal(t, codersdk.BuildReasonInitiator, stopBuild.Reason)

	// WHEN: the parent stop build starts running and is canceled.
	require.Eventually(t, func() bool {
		var err error
		stopBuild, err = client.WorkspaceBuild(ctx, stopBuild.ID)
		return err == nil &&
			stopBuild.Job.Status == codersdk.ProvisionerJobRunning
	}, testutil.WaitShort, testutil.IntervalFast)

	require.NoError(t, client.CancelWorkspaceBuild(ctx, stopBuild.ID, codersdk.CancelWorkspaceBuildParams{}))
	require.Eventually(t, func() bool {
		var err error
		stopBuild, err = client.WorkspaceBuild(ctx, stopBuild.ID)
		if err != nil {
			return false
		}
		return stopBuild.Job.Status == codersdk.ProvisionerJobFailed &&
			stopBuild.Job.Error == "canceled"
	}, testutil.WaitShort, testutil.IntervalFast)

	// THEN: the server resolves the orchestration without creating the
	// child start build.
	require.Eventually(t, func() bool {
		orchestration, err := dbtestutil.GetWorkspaceBuildOrchestrationByParentBuildID(ctx, sqlDB, stopBuild.ID)
		return err == nil &&
			orchestration.Status == "failed" &&
			!orchestration.ChildBuildID.Valid &&
			orchestration.Error.Valid &&
			orchestration.Error.String == "parent workspace build failed: canceled"
	}, testutil.WaitShort, testutil.IntervalFast)

	_, err = client.WorkspaceBuildByUsernameAndWorkspaceNameAndBuildNumber(
		ctx,
		user.Username,
		workspace.Name,
		strconv.FormatInt(int64(stopBuild.BuildNumber+1), 10),
	)
	var apiErr *codersdk.Error
	require.ErrorAs(t, err, &apiErr)
	require.Equal(t, http.StatusNotFound, apiErr.StatusCode())
}

func TestPostWorkspaceBuildsOnSuccessParentFailed(t *testing.T) {
	t.Parallel()

	// GIVEN: a running workspace whose stop apply will fail.
	db, ps, sqlDB := dbtestutil.NewDBWithSQLDB(t)
	client, _, _ := coderdtest.NewWithAPI(t, &coderdtest.Options{
		Database:                 db,
		Pubsub:                   ps,
		IncludeProvisionerDaemon: true,
	})
	first := coderdtest.CreateFirstUser(t, client)
	version := coderdtest.CreateTemplateVersion(t, client, first.OrganizationID,
		echoResponsesWithRichParameter("foo", echoResponseOptions{
			failStopApply: true,
		}),
	)
	coderdtest.AwaitTemplateVersionJobCompleted(t, client, version.ID)
	template := coderdtest.CreateTemplate(t, client, first.OrganizationID, version.ID)
	workspace := coderdtest.CreateWorkspace(t, client, template.ID)
	initialBuild := coderdtest.AwaitWorkspaceBuildJobCompleted(t, client, workspace.LatestBuild.ID)
	require.Equal(t, codersdk.WorkspaceStatusRunning, initialBuild.Status)

	// WHEN: a stop build is created with an on_success start build.
	ctx := testutil.Context(t, testutil.WaitLong)
	user, err := client.User(ctx, codersdk.Me)
	require.NoError(t, err)

	stopBuild, err := client.CreateWorkspaceBuild(ctx, workspace.ID, codersdk.CreateWorkspaceBuildRequest{
		Transition: codersdk.WorkspaceTransitionStop,
		OnSuccess: &codersdk.CreateWorkspaceBuildOnSuccessRequest{
			Transition:        codersdk.WorkspaceTransitionStart,
			TemplateVersionID: template.ActiveVersionID,
		},
	})
	require.NoError(t, err)

	// WHEN: the parent stop build fails.
	stopBuild = coderdtest.AwaitWorkspaceBuildJobCompleted(t, client, stopBuild.ID)
	require.Equal(t, codersdk.ProvisionerJobFailed, stopBuild.Job.Status)

	// THEN: the server resolves the orchestration without creating the
	// child start build.
	require.Eventually(t, func() bool {
		orchestration, err := dbtestutil.GetWorkspaceBuildOrchestrationByParentBuildID(ctx, sqlDB, stopBuild.ID)
		return err == nil &&
			orchestration.Status == "failed" &&
			!orchestration.ChildBuildID.Valid &&
			orchestration.Error.Valid &&
			orchestration.Error.String == "parent workspace build failed: failed!"
	}, testutil.WaitShort, testutil.IntervalFast)

	_, err = client.WorkspaceBuildByUsernameAndWorkspaceNameAndBuildNumber(
		ctx,
		user.Username,
		workspace.Name,
		strconv.FormatInt(int64(stopBuild.BuildNumber+1), 10),
	)
	var apiErr *codersdk.Error
	require.ErrorAs(t, err, &apiErr)
	require.Equal(t, http.StatusNotFound, apiErr.StatusCode())
}

func TestPostWorkspaceBuildsOnSuccessNonRetryableChildBuildFailure(t *testing.T) {
	t.Parallel()

	// GIVEN: a running workspace with a rich parameter value that
	// satisfies the template regex validation.
	db, ps, sqlDB := dbtestutil.NewDBWithSQLDB(t)
	client, _, _ := coderdtest.NewWithAPI(t, &coderdtest.Options{
		Database:                 db,
		Pubsub:                   ps,
		IncludeProvisionerDaemon: true,
	})
	first := coderdtest.CreateFirstUser(t, client)
	version := coderdtest.CreateTemplateVersion(t, client, first.OrganizationID,
		echoResponsesWithRichParameter("foo", echoResponseOptions{
			validationRegex: "^good$",
		}),
	)
	coderdtest.AwaitTemplateVersionJobCompleted(t, client, version.ID)
	template := coderdtest.CreateTemplate(t, client, first.OrganizationID, version.ID)
	workspace := coderdtest.CreateWorkspace(t, client, template.ID, func(request *codersdk.CreateWorkspaceRequest) {
		request.RichParameterValues = []codersdk.WorkspaceBuildParameter{
			{Name: "foo", Value: "good"},
		}
	})
	initialBuild := coderdtest.AwaitWorkspaceBuildJobCompleted(t, client, workspace.LatestBuild.ID)
	require.Equal(t, codersdk.WorkspaceStatusRunning, initialBuild.Status)

	// WHEN: a stop build is created with an on_success child build
	// that has an invalid rich parameter value.
	ctx := testutil.Context(t, testutil.WaitLong)
	user, err := client.User(ctx, codersdk.Me)
	require.NoError(t, err)

	stopBuild, err := client.CreateWorkspaceBuild(ctx, workspace.ID, codersdk.CreateWorkspaceBuildRequest{
		Transition: codersdk.WorkspaceTransitionStop,
		OnSuccess: &codersdk.CreateWorkspaceBuildOnSuccessRequest{
			Transition:        codersdk.WorkspaceTransitionStart,
			TemplateVersionID: template.ActiveVersionID,
			// The invalid value triggers a non-retryable child build
			// creation error when the orchestrator processes the row.
			RichParameterValues: []codersdk.WorkspaceBuildParameter{
				{Name: "foo", Value: "bad"},
			},
		},
	})
	require.NoError(t, err)

	stopBuild = coderdtest.AwaitWorkspaceBuildJobCompleted(t, client, stopBuild.ID)
	require.Equal(t, codersdk.ProvisionerJobSucceeded, stopBuild.Job.Status)
	require.Equal(t, codersdk.WorkspaceStatusStopped, stopBuild.Status)

	// THEN: the server marks the orchestration as failed without
	// retrying or creating the child start build.
	var orchestration database.WorkspaceBuildOrchestration
	require.Eventually(t, func() bool {
		orchestration, err = dbtestutil.GetWorkspaceBuildOrchestrationByParentBuildID(ctx, sqlDB, stopBuild.ID)
		return err == nil &&
			orchestration.Status == "failed" &&
			!orchestration.ChildBuildID.Valid &&
			orchestration.AttemptCount == 0 &&
			!orchestration.NextRetryAfter.Valid &&
			orchestration.Error.Valid
	}, testutil.WaitShort, testutil.IntervalFast)
	require.Contains(t, orchestration.Error.String, "Unable to validate parameters")

	_, err = client.WorkspaceBuildByUsernameAndWorkspaceNameAndBuildNumber(
		ctx,
		user.Username,
		workspace.Name,
		strconv.FormatInt(int64(stopBuild.BuildNumber+1), 10),
	)
	var apiErr *codersdk.Error
	require.ErrorAs(t, err, &apiErr)
	require.Equal(t, http.StatusNotFound, apiErr.StatusCode())
}

func TestPostWorkspaceBuildsOnSuccessRetryableChildBuildFailureDoesNotBlockLaterRestart(t *testing.T) {
	t.Parallel()

	// GIVEN: two provisioners, one holding a template import job
	// open to make the child build fail retryably while the other
	// processes workspace builds.
	db, ps, sqlDB := dbtestutil.NewDBWithSQLDB(t)
	client, _, api := coderdtest.NewWithAPI(t, &coderdtest.Options{
		Database:                 db,
		Pubsub:                   ps,
		IncludeProvisionerDaemon: true,
	})
	coderdtest.NewProvisionerDaemon(t, api)

	first := coderdtest.CreateFirstUser(t, client)
	version := coderdtest.CreateTemplateVersion(t, client, first.OrganizationID, nil)
	coderdtest.AwaitTemplateVersionJobCompleted(t, client, version.ID)
	template := coderdtest.CreateTemplate(t, client, first.OrganizationID, version.ID)

	workspace := coderdtest.CreateWorkspace(t, client, template.ID)
	startBuild := coderdtest.AwaitWorkspaceBuildJobCompleted(t, client, workspace.LatestBuild.ID)
	require.Equal(t, codersdk.WorkspaceStatusRunning, startBuild.Status)

	blockedVersion := coderdtest.UpdateTemplateVersion(t, client, first.OrganizationID,
		// Without a PlanComplete response, the echo provisioner will
		// keep the template version import job running.
		&echo.Responses{
			Parse: echo.ParseComplete,
			ProvisionPlan: []*proto.Response{{
				Type: &proto.Response_Log{
					Log: &proto.Log{},
				},
			}},
		}, template.ID,
	)
	coderdtest.AwaitTemplateVersionJobRunning(t, client, blockedVersion.ID)

	// WHEN: a stop build is created with an on_success child build
	// request that references a template version whose import job is
	// still running, causing child build creation to be retried later.
	ctx := testutil.Context(t, testutil.WaitLong)
	badStopBuild, err := client.CreateWorkspaceBuild(ctx, workspace.ID, codersdk.CreateWorkspaceBuildRequest{
		Transition: codersdk.WorkspaceTransitionStop,
		OnSuccess: &codersdk.CreateWorkspaceBuildOnSuccessRequest{
			Transition:        codersdk.WorkspaceTransitionStart,
			TemplateVersionID: blockedVersion.ID,
		},
	})
	require.NoError(t, err)

	badStopBuild = coderdtest.AwaitWorkspaceBuildJobCompleted(t, client, badStopBuild.ID)
	require.Equal(t, codersdk.ProvisionerJobSucceeded, badStopBuild.Job.Status)
	require.Equal(t, codersdk.WorkspaceStatusStopped, badStopBuild.Status)

	// THEN: the orchestrator records a delayed retry after child
	// build creation fails because the requested template version
	// is still importing.
	var badOrchestration database.WorkspaceBuildOrchestration
	require.Eventually(t, func() bool {
		badOrchestration, err = dbtestutil.GetWorkspaceBuildOrchestrationByParentBuildID(ctx, sqlDB, badStopBuild.ID)
		return err == nil &&
			badOrchestration.Status == "pending" &&
			badOrchestration.AttemptCount == 1 &&
			badOrchestration.NextRetryAfter.Valid
	}, testutil.WaitShort, testutil.IntervalFast)
	require.False(t, badOrchestration.ChildBuildID.Valid)
	require.True(t, badOrchestration.Error.Valid)
	require.Contains(t, badOrchestration.Error.String, "template version is running")

	// WHEN: a later restart uses a valid child build request.
	goodWorkspace := coderdtest.CreateWorkspace(t, client, template.ID)
	goodStartBuild := coderdtest.AwaitWorkspaceBuildJobCompleted(t, client, goodWorkspace.LatestBuild.ID)
	require.Equal(t, codersdk.WorkspaceStatusRunning, goodStartBuild.Status)

	user, err := client.User(ctx, codersdk.Me)
	require.NoError(t, err)

	goodStopBuild, err := client.CreateWorkspaceBuild(ctx, goodWorkspace.ID, codersdk.CreateWorkspaceBuildRequest{
		Transition: codersdk.WorkspaceTransitionStop,
		OnSuccess: &codersdk.CreateWorkspaceBuildOnSuccessRequest{
			Transition:        codersdk.WorkspaceTransitionStart,
			TemplateVersionID: template.ActiveVersionID,
		},
	})
	require.NoError(t, err)

	goodStopBuild = coderdtest.AwaitWorkspaceBuildJobCompleted(t, client, goodStopBuild.ID)
	require.Equal(t, codersdk.ProvisionerJobSucceeded, goodStopBuild.Job.Status)
	require.Equal(t, codersdk.WorkspaceStatusStopped, goodStopBuild.Status)

	// THEN: the delayed retry row does not block the later orchestration.
	var goodChildBuild codersdk.WorkspaceBuild
	require.Eventually(t, func() bool {
		goodChildBuild, err = client.WorkspaceBuildByUsernameAndWorkspaceNameAndBuildNumber(
			ctx,
			user.Username,
			goodWorkspace.Name,
			strconv.FormatInt(int64(goodStopBuild.BuildNumber+1), 10),
		)
		return err == nil &&
			goodChildBuild.Transition == codersdk.WorkspaceTransitionStart
	}, testutil.WaitMedium, testutil.IntervalFast)

	goodChildBuild = coderdtest.AwaitWorkspaceBuildJobCompleted(t, client, goodChildBuild.ID)
	require.Equal(t, codersdk.ProvisionerJobSucceeded, goodChildBuild.Job.Status)
	require.Equal(t, codersdk.WorkspaceStatusRunning, goodChildBuild.Status)
}

type echoResponseOptions struct {
	blockStopApply  bool
	failStopApply   bool
	validationRegex string
}

func echoResponsesWithRichParameter(paramName string, options echoResponseOptions) *echo.Responses {
	validationError := ""
	if options.validationRegex != "" {
		validationError = "invalid parameter value"
	}

	responses := &echo.Responses{
		Parse:         echo.ParseComplete,
		ProvisionInit: echo.InitComplete,
		ProvisionGraph: []*proto.Response{{
			Type: &proto.Response_Graph{
				Graph: &proto.GraphComplete{
					Parameters: []*proto.RichParameter{{
						Name:            paramName,
						Type:            "string",
						DefaultValue:    "bar",
						Mutable:         true,
						FormType:        proto.ParameterFormType_INPUT,
						ValidationRegex: options.validationRegex,
						ValidationError: validationError,
					}},
				},
			},
		}},
		ProvisionPlan:  echo.PlanComplete,
		ProvisionApply: echo.ApplyComplete,
	}
	if options.blockStopApply {
		responses.ProvisionApplyMap = map[proto.WorkspaceTransition][]*proto.Response{
			proto.WorkspaceTransition_START: echo.ApplyComplete,
			proto.WorkspaceTransition_STOP: {{
				Type: &proto.Response_Log{
					Log: &proto.Log{},
				},
			}},
		}
	}
	if options.failStopApply {
		responses.ProvisionApplyMap = map[proto.WorkspaceTransition][]*proto.Response{
			proto.WorkspaceTransition_START: echo.ApplyComplete,
			proto.WorkspaceTransition_STOP:  echo.ApplyFailed,
		}
	}
	return responses
}
