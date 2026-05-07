package coderd_test

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/coderdtest"
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

	// THEN: the API rejects the durable child version pin.
	var apiErr *codersdk.Error
	require.ErrorAs(t, err, &apiErr)
	require.Equal(t, http.StatusForbidden, apiErr.StatusCode())

	// THEN: no new workspace build is created.
	builds, err := userClient.WorkspaceBuilds(ctx, codersdk.WorkspaceBuildsRequest{WorkspaceID: workspace.ID})
	require.NoError(t, err)
	require.Len(t, builds, 1)
	require.Equal(t, initialBuild.ID, builds[0].ID)
}

func TestPostWorkspaceBuildsOnSuccessValidation(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		request codersdk.CreateWorkspaceBuildRequest
	}{
		{
			name: "ParentMustBeStop",
			request: codersdk.CreateWorkspaceBuildRequest{
				Transition: codersdk.WorkspaceTransitionStart,
				OnSuccess: &codersdk.CreateWorkspaceBuildOnSuccessRequest{
					Transition: codersdk.WorkspaceTransitionStart,
				},
			},
		},
		{
			name: "ChildMustBeStart",
			request: codersdk.CreateWorkspaceBuildRequest{
				Transition: codersdk.WorkspaceTransitionStop,
				OnSuccess: &codersdk.CreateWorkspaceBuildOnSuccessRequest{
					Transition: codersdk.WorkspaceTransitionStop,
				},
			},
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
		})
	}
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
