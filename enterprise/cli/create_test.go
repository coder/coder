package cli_test

import (
	"context"
	"database/sql"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/coder/coder/v2/cli"

	"github.com/coder/coder/v2/coderd/wsbuilder"

	"github.com/google/uuid"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/alerts"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbtestutil"
	"github.com/coder/coder/v2/coderd/files"
	agplprebuilds "github.com/coder/coder/v2/coderd/prebuilds"
	"github.com/coder/coder/v2/enterprise/coderd/prebuilds"
	"github.com/coder/coder/v2/provisioner/echo"
	"github.com/coder/coder/v2/provisionersdk/proto"
	"github.com/coder/coder/v2/testutil"
	"github.com/coder/quartz"

	"github.com/coder/coder/v2/cli/clitest"
	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/coderd/rbac"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/enterprise/coderd/coderdenttest"
	"github.com/coder/coder/v2/enterprise/coderd/license"
	"github.com/coder/coder/v2/pty/ptytest"
)

func TestEnterpriseCreate(t *testing.T) {
	t.Parallel()

	type setupData struct {
		firstResponse codersdk.CreateFirstUserResponse
		second        codersdk.Organization
		owner         *codersdk.Client
		member        *codersdk.Client
	}

	type setupArgs struct {
		firstTemplates  []string
		secondTemplates []string
	}

	// setupMultipleOrganizations creates an extra organization, assigns a member
	// both organizations, and optionally creates templates in each organization.
	setupMultipleOrganizations := func(t *testing.T, args setupArgs) setupData {
		ownerClient, first := coderdenttest.New(t, &coderdenttest.Options{
			Options: &coderdtest.Options{
				// This only affects the first org.
				IncludeProvisionerDaemon: true,
			},
			LicenseOptions: &coderdenttest.LicenseOptions{
				Features: license.Features{
					codersdk.FeatureExternalProvisionerDaemons: 1,
					codersdk.FeatureMultipleOrganizations:      1,
				},
			},
		})

		second := coderdenttest.CreateOrganization(t, ownerClient, coderdenttest.CreateOrganizationOptions{
			IncludeProvisionerDaemon: true,
		})
		member, _ := coderdtest.CreateAnotherUser(t, ownerClient, first.OrganizationID, rbac.ScopedRoleOrgMember(second.ID))

		var wg sync.WaitGroup

		createTemplate := func(tplName string, orgID uuid.UUID) {
			version := coderdtest.CreateTemplateVersion(t, ownerClient, orgID, nil)
			wg.Add(1)
			go func() {
				coderdtest.AwaitTemplateVersionJobCompleted(t, ownerClient, version.ID)
				wg.Done()
			}()

			coderdtest.CreateTemplate(t, ownerClient, orgID, version.ID, func(request *codersdk.CreateTemplateRequest) {
				request.Name = tplName
			})
		}

		for _, tplName := range args.firstTemplates {
			createTemplate(tplName, first.OrganizationID)
		}

		for _, tplName := range args.secondTemplates {
			createTemplate(tplName, second.ID)
		}

		wg.Wait()

		return setupData{
			firstResponse: first,
			owner:         ownerClient,
			second:        second,
			member:        member,
		}
	}

	// Test creating a workspace in the second organization with a template
	// name.
	t.Run("CreateMultipleOrganization", func(t *testing.T) {
		t.Parallel()

		const templateName = "secondtemplate"
		setup := setupMultipleOrganizations(t, setupArgs{
			secondTemplates: []string{templateName},
		})
		member := setup.member

		args := []string{
			"create",
			"my-workspace",
			"-y",
			"--template", templateName,
		}
		inv, root := clitest.New(t, args...)
		clitest.SetupConfig(t, member, root)
		_ = ptytest.New(t).Attach(inv)
		err := inv.Run()
		require.NoError(t, err)

		ws, err := member.WorkspaceByOwnerAndName(context.Background(), codersdk.Me, "my-workspace", codersdk.WorkspaceOptions{})
		if assert.NoError(t, err, "expected workspace to be created") {
			assert.Equal(t, ws.TemplateName, templateName)
			assert.Equal(t, ws.OrganizationName, setup.second.Name, "workspace in second organization")
		}
	})

	// If a template name exists in two organizations, the workspace create will
	// fail.
	t.Run("AmbiguousTemplateName", func(t *testing.T) {
		t.Parallel()

		const templateName = "ambiguous"
		setup := setupMultipleOrganizations(t, setupArgs{
			firstTemplates:  []string{templateName},
			secondTemplates: []string{templateName},
		})
		member := setup.member

		args := []string{
			"create",
			"my-workspace",
			"-y",
			"--template", templateName,
		}
		inv, root := clitest.New(t, args...)
		clitest.SetupConfig(t, member, root)
		_ = ptytest.New(t).Attach(inv)
		err := inv.Run()
		require.Error(t, err, "expected error due to ambiguous template name")
		require.ErrorContains(t, err, "multiple templates found")
	})

	// Ambiguous template names are allowed if the organization is specified.
	t.Run("WorkingAmbiguousTemplateName", func(t *testing.T) {
		t.Parallel()

		const templateName = "ambiguous"
		setup := setupMultipleOrganizations(t, setupArgs{
			firstTemplates:  []string{templateName},
			secondTemplates: []string{templateName},
		})
		member := setup.member

		args := []string{
			"create",
			"my-workspace",
			"-y",
			"--template", templateName,
			"--org", setup.second.Name,
		}
		inv, root := clitest.New(t, args...)
		clitest.SetupConfig(t, member, root)
		_ = ptytest.New(t).Attach(inv)
		err := inv.Run()
		require.NoError(t, err)

		ws, err := member.WorkspaceByOwnerAndName(context.Background(), codersdk.Me, "my-workspace", codersdk.WorkspaceOptions{})
		if assert.NoError(t, err, "expected workspace to be created") {
			assert.Equal(t, ws.TemplateName, templateName)
			assert.Equal(t, ws.OrganizationName, setup.second.Name, "workspace in second organization")
		}
	})

	// If an organization is specified, but the template is not in that
	// organization, an error is thrown.
	t.Run("CreateIncorrectOrg", func(t *testing.T) {
		t.Parallel()

		const templateName = "secondtemplate"
		setup := setupMultipleOrganizations(t, setupArgs{
			firstTemplates: []string{templateName},
		})
		member := setup.member

		args := []string{
			"create",
			"my-workspace",
			"-y",
			"--org", setup.second.Name,
			"--template", templateName,
		}
		inv, root := clitest.New(t, args...)
		clitest.SetupConfig(t, member, root)
		_ = ptytest.New(t).Attach(inv)
		err := inv.Run()
		require.Error(t, err)
		// The error message should indicate the flag to fix the issue.
		require.ErrorContains(t, err, fmt.Sprintf("--org=%q", "coder"))
	})
}

func TestEnterpriseCreateWithPreset(t *testing.T) {
	t.Parallel()

	const (
		firstParameterName        = "first_parameter"
		firstParameterDisplayName = "First Parameter"
		firstParameterDescription = "This is the first parameter"
		firstParameterValue       = "1"

		firstOptionalParameterName         = "first_optional_parameter"
		firstOptionParameterDescription    = "This is the first optional parameter"
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
						Description: firstOptionParameterDescription,
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

	runReconciliationLoop := func(
		t *testing.T,
		ctx context.Context,
		db database.Store,
		reconciler *prebuilds.StoreReconciler,
		presets []codersdk.Preset,
	) {
		t.Helper()

		state, err := reconciler.SnapshotState(ctx, db)
		require.NoError(t, err)
		require.Len(t, presets, 1)
		ps, err := state.FilterByPreset(presets[0].ID)
		require.NoError(t, err)
		require.NotNil(t, ps)
		actions, err := reconciler.CalculateActions(ctx, *ps)
		require.NoError(t, err)
		require.NotNil(t, actions)
		require.NoError(t, reconciler.ReconcilePreset(ctx, *ps))
	}

	getRunningPrebuilds := func(
		t *testing.T,
		ctx context.Context,
		db database.Store,
		prebuildInstances int,
	) []database.GetRunningPrebuiltWorkspacesRow {
		t.Helper()

		var runningPrebuilds []database.GetRunningPrebuiltWorkspacesRow
		testutil.Eventually(ctx, t, func(context.Context) bool {
			runningPrebuilds = nil
			rows, err := db.GetRunningPrebuiltWorkspaces(ctx)
			if err != nil {
				return false
			}

			for _, row := range rows {
				runningPrebuilds = append(runningPrebuilds, row)

				agents, err := db.GetWorkspaceAgentsInLatestBuildByWorkspaceID(ctx, row.ID)
				if err != nil || len(agents) == 0 {
					return false
				}

				for _, agent := range agents {
					err = db.UpdateWorkspaceAgentLifecycleStateByID(ctx, database.UpdateWorkspaceAgentLifecycleStateByIDParams{
						ID:             agent.ID,
						LifecycleState: database.WorkspaceAgentLifecycleStateReady,
						StartedAt:      sql.NullTime{Time: time.Now().Add(time.Hour), Valid: true},
						ReadyAt:        sql.NullTime{Time: time.Now().Add(-1 * time.Hour), Valid: true},
					})
					if err != nil {
						return false
					}
				}
			}

			t.Logf("found %d running prebuilds so far, want %d", len(runningPrebuilds), prebuildInstances)
			return len(runningPrebuilds) == prebuildInstances
		}, testutil.IntervalSlow, "prebuilds not running")

		return runningPrebuilds
	}

	// This test verifies that when the selected preset has running prebuilds,
	// one of those prebuilds is claimed for the user upon workspace creation.
	t.Run("PresetFlagClaimsPrebuiltWorkspace", func(t *testing.T) {
		t.Parallel()

		// Setup
		ctx := testutil.Context(t, testutil.WaitSuperLong)
		db, pb := dbtestutil.NewDB(t, dbtestutil.WithDumpOnFailure())
		client, _, api, owner := coderdenttest.NewWithAPI(t, &coderdenttest.Options{
			Options: &coderdtest.Options{
				Database:                 db,
				Pubsub:                   pb,
				IncludeProvisionerDaemon: true,
			},
		})

		// Setup Prebuild reconciler
		cache := files.New(prometheus.NewRegistry(), &coderdtest.FakeAuthorizer{})
		newNoopUsageCheckerPtr := func() *atomic.Pointer[wsbuilder.UsageChecker] {
			var noopUsageChecker wsbuilder.UsageChecker = wsbuilder.NoopUsageChecker{}
			buildUsageChecker := atomic.Pointer[wsbuilder.UsageChecker]{}
			buildUsageChecker.Store(&noopUsageChecker)
			return &buildUsageChecker
		}
		reconciler := prebuilds.NewStoreReconciler(
			db, pb, cache,
			codersdk.PrebuildsConfig{},
			testutil.Logger(t),
			quartz.NewMock(t),
			prometheus.NewRegistry(),
			alerts.NewNoopEnqueuer(),
			newNoopUsageCheckerPtr(),
		)
		var claimer agplprebuilds.Claimer = prebuilds.NewEnterpriseClaimer(db)
		api.AGPL.PrebuildsClaimer.Store(&claimer)

		// Given: a template and a template version where the preset defines values for all required parameters,
		// and is configured to have 1 prebuild instance
		prebuildInstances := int32(1)
		preset := proto.Preset{
			Name: "preset-test",
			Parameters: []*proto.PresetParameter{
				{Name: firstParameterName, Value: secondOptionalParameterValue},
				{Name: thirdParameterName, Value: thirdParameterValue},
			},
			Prebuild: &proto.Prebuild{
				Instances: prebuildInstances,
			},
		}
		member, _ := coderdtest.CreateAnotherUser(t, client, owner.OrganizationID)
		version := coderdtest.CreateTemplateVersion(t, client, owner.OrganizationID, echoResponses(&preset))
		coderdtest.AwaitTemplateVersionJobCompleted(t, client, version.ID)
		template := coderdtest.CreateTemplate(t, client, owner.OrganizationID, version.ID)
		presets, err := client.TemplateVersionPresets(ctx, version.ID)
		require.NoError(t, err)
		require.Len(t, presets, 1)
		require.Equal(t, preset.Name, presets[0].Name)

		// Given: Reconciliation loop runs and starts prebuilt workspaces
		runReconciliationLoop(t, ctx, db, reconciler, presets)
		runningPrebuilds := getRunningPrebuilds(t, ctx, db, int(prebuildInstances))
		require.Len(t, runningPrebuilds, int(prebuildInstances))
		require.Equal(t, presets[0].ID, runningPrebuilds[0].CurrentPresetID.UUID)

		// Given: a running prebuilt workspace, ready to be claimed
		prebuild := coderdtest.MustWorkspace(t, client, runningPrebuilds[0].ID)
		require.Equal(t, codersdk.WorkspaceTransitionStart, prebuild.LatestBuild.Transition)
		require.Equal(t, template.ID, prebuild.TemplateID)
		require.Equal(t, version.ID, prebuild.TemplateActiveVersionID)
		require.Equal(t, presets[0].ID, *prebuild.LatestBuild.TemplateVersionPresetID)

		// When: running the create command with the specified preset
		workspaceName := "my-workspace"
		inv, root := clitest.New(t, "create", workspaceName, "--template", template.Name, "-y", "--preset", preset.Name)
		clitest.SetupConfig(t, member, root)
		pty := ptytest.New(t).Attach(inv)
		inv.Stdout = pty.Output()
		inv.Stderr = pty.Output()
		err = inv.Run()
		require.NoError(t, err)

		// Should: display the selected preset as well as its parameters
		presetName := fmt.Sprintf("Preset '%s' applied:", preset.Name)
		pty.ExpectMatch(presetName)
		pty.ExpectMatch(fmt.Sprintf("%s: '%s'", firstParameterName, secondOptionalParameterValue))
		pty.ExpectMatch(fmt.Sprintf("%s: '%s'", thirdParameterName, thirdParameterValue))

		// Verify if the new workspace uses expected parameters.
		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitShort)
		defer cancel()

		// Should: create the user's workspace by claiming the existing prebuilt workspace
		workspaces, err := client.Workspaces(ctx, codersdk.WorkspaceFilter{
			Name: workspaceName,
		})
		require.NoError(t, err)
		require.Len(t, workspaces.Workspaces, 1)
		require.Equal(t, prebuild.ID, workspaces.Workspaces[0].ID)

		// Should: create a workspace using the expected template version and the preset-defined parameters
		workspaceLatestBuild := workspaces.Workspaces[0].LatestBuild
		require.Equal(t, version.ID, workspaceLatestBuild.TemplateVersionID)
		require.Equal(t, presets[0].ID, *workspaceLatestBuild.TemplateVersionPresetID)
		buildParameters, err := client.WorkspaceBuildParameters(ctx, workspaceLatestBuild.ID)
		require.NoError(t, err)
		require.Len(t, buildParameters, 2)
		require.Contains(t, buildParameters, codersdk.WorkspaceBuildParameter{Name: firstParameterName, Value: secondOptionalParameterValue})
		require.Contains(t, buildParameters, codersdk.WorkspaceBuildParameter{Name: thirdParameterName, Value: thirdParameterValue})
	})

	// This test verifies that when the user provides `--preset None`,
	// no preset is applied, no prebuilt workspace is claimed, and
	// a new regular workspace is created instead.
	t.Run("PresetNoneDoesNotClaimPrebuiltWorkspace", func(t *testing.T) {
		t.Parallel()

		// Setup
		ctx := testutil.Context(t, testutil.WaitSuperLong)
		db, pb := dbtestutil.NewDB(t, dbtestutil.WithDumpOnFailure())
		client, _, api, owner := coderdenttest.NewWithAPI(t, &coderdenttest.Options{
			Options: &coderdtest.Options{
				Database:                 db,
				Pubsub:                   pb,
				IncludeProvisionerDaemon: true,
			},
		})

		// Setup Prebuild reconciler
		cache := files.New(prometheus.NewRegistry(), &coderdtest.FakeAuthorizer{})
		newNoopUsageCheckerPtr := func() *atomic.Pointer[wsbuilder.UsageChecker] {
			var noopUsageChecker wsbuilder.UsageChecker = wsbuilder.NoopUsageChecker{}
			buildUsageChecker := atomic.Pointer[wsbuilder.UsageChecker]{}
			buildUsageChecker.Store(&noopUsageChecker)
			return &buildUsageChecker
		}
		reconciler := prebuilds.NewStoreReconciler(
			db, pb, cache,
			codersdk.PrebuildsConfig{},
			testutil.Logger(t),
			quartz.NewMock(t),
			prometheus.NewRegistry(),
			alerts.NewNoopEnqueuer(),
			newNoopUsageCheckerPtr(),
		)
		var claimer agplprebuilds.Claimer = prebuilds.NewEnterpriseClaimer(db)
		api.AGPL.PrebuildsClaimer.Store(&claimer)

		// Given: a template and a template version where the preset defines values for all required parameters,
		// and is configured to have 1 prebuild instance
		prebuildInstances := int32(1)
		presetWithPrebuild := proto.Preset{
			Name: "preset-test",
			Parameters: []*proto.PresetParameter{
				{Name: firstParameterName, Value: secondOptionalParameterValue},
				{Name: thirdParameterName, Value: thirdParameterValue},
			},
			Prebuild: &proto.Prebuild{
				Instances: prebuildInstances,
			},
		}
		member, _ := coderdtest.CreateAnotherUser(t, client, owner.OrganizationID)
		version := coderdtest.CreateTemplateVersion(t, client, owner.OrganizationID, echoResponses(&presetWithPrebuild))
		coderdtest.AwaitTemplateVersionJobCompleted(t, client, version.ID)
		template := coderdtest.CreateTemplate(t, client, owner.OrganizationID, version.ID)
		presets, err := client.TemplateVersionPresets(ctx, version.ID)
		require.NoError(t, err)
		require.Len(t, presets, 1)

		// Given: Reconciliation loop runs and starts prebuilt workspaces
		runReconciliationLoop(t, ctx, db, reconciler, presets)
		runningPrebuilds := getRunningPrebuilds(t, ctx, db, int(prebuildInstances))
		require.Len(t, runningPrebuilds, int(prebuildInstances))
		require.Equal(t, presets[0].ID, runningPrebuilds[0].CurrentPresetID.UUID)

		// Given: a running prebuilt workspace, ready to be claimed
		prebuild := coderdtest.MustWorkspace(t, client, runningPrebuilds[0].ID)
		require.Equal(t, codersdk.WorkspaceTransitionStart, prebuild.LatestBuild.Transition)
		require.Equal(t, template.ID, prebuild.TemplateID)
		require.Equal(t, version.ID, prebuild.TemplateActiveVersionID)
		require.Equal(t, presets[0].ID, *prebuild.LatestBuild.TemplateVersionPresetID)

		// When: running the create command without a preset flag
		workspaceName := "my-workspace"
		inv, root := clitest.New(t, "create", workspaceName, "--template", template.Name, "-y",
			"--preset", cli.PresetNone,
			"--parameter", fmt.Sprintf("%s=%s", firstParameterName, firstParameterValue),
			"--parameter", fmt.Sprintf("%s=%s", thirdParameterName, thirdParameterValue))
		clitest.SetupConfig(t, member, root)
		pty := ptytest.New(t).Attach(inv)
		inv.Stdout = pty.Output()
		inv.Stderr = pty.Output()
		err = inv.Run()
		require.NoError(t, err)
		pty.ExpectMatch("No preset applied.")

		// Verify if the new workspace uses expected parameters.
		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitShort)
		defer cancel()

		// Should: create a new user's workspace without claiming the existing prebuilt workspace
		workspaces, err := client.Workspaces(ctx, codersdk.WorkspaceFilter{
			Name: workspaceName,
		})
		require.NoError(t, err)
		require.Len(t, workspaces.Workspaces, 1)
		require.NotEqual(t, prebuild.ID, workspaces.Workspaces[0].ID)

		// Should: create a workspace using the expected template version and the specified parameters
		workspaceLatestBuild := workspaces.Workspaces[0].LatestBuild
		require.Equal(t, version.ID, workspaceLatestBuild.TemplateVersionID)
		require.Nil(t, workspaceLatestBuild.TemplateVersionPresetID)
		buildParameters, err := client.WorkspaceBuildParameters(ctx, workspaceLatestBuild.ID)
		require.NoError(t, err)
		require.Len(t, buildParameters, 2)
		require.Contains(t, buildParameters, codersdk.WorkspaceBuildParameter{Name: firstParameterName, Value: firstParameterValue})
		require.Contains(t, buildParameters, codersdk.WorkspaceBuildParameter{Name: thirdParameterName, Value: thirdParameterValue})
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
		ProvisionApply: []*proto.Response{
			{
				Type: &proto.Response_Apply{
					Apply: &proto.ApplyComplete{
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
		},
	}
}
