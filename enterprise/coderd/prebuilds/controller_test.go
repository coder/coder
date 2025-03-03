package prebuilds

import (
	"context"
	"database/sql"
	"strings"
	"testing"
	"time"

	"github.com/coder/serpent"
	"github.com/google/uuid"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/database/dbgen"
	"github.com/coder/coder/v2/coderd/database/dbtestutil"
	"github.com/coder/coder/v2/coderd/database/pubsub"
	"github.com/coder/coder/v2/coderd/rbac"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/provisioner/echo"
	"github.com/coder/coder/v2/provisionersdk/proto"
	"github.com/coder/coder/v2/testutil"
)

func TestNoReconciliationActionsIfNoPresets(t *testing.T) {
	if !dbtestutil.WillUsePostgres() {
		t.Skip("This test requires postgres")
	}

	// Scenario: No reconciliation actions are taken if there are no presets
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitLong)
	db, pubsub := dbtestutil.NewDB(t)
	cfg := codersdk.PrebuildsConfig{
		ReconciliationInterval: serpent.Duration(testutil.WaitLong),
	}
	logger := testutil.Logger(t)
	controller := NewController(db, pubsub, cfg, logger)

	// given a template version with no presets
	org := dbgen.Organization(t, db, database.Organization{})
	user := dbgen.User(t, db, database.User{})
	template := dbgen.Template(t, db, database.Template{
		CreatedBy:      user.ID,
		OrganizationID: org.ID,
	})
	templateVersion := dbgen.TemplateVersion(t, db, database.TemplateVersion{
		TemplateID:     uuid.NullUUID{UUID: template.ID, Valid: true},
		OrganizationID: org.ID,
		CreatedBy:      user.ID,
	})
	// verify that the db state is correct
	gotTemplateVersion, err := db.GetTemplateVersionByID(ctx, templateVersion.ID)
	require.NoError(t, err)
	require.Equal(t, templateVersion, gotTemplateVersion)

	// when we trigger the reconciliation loop for all templates
	controller.reconcile(ctx, nil)

	// then no reconciliation actions are taken
	// because without presets, there are no prebuilds
	// and without prebuilds, there is nothing to reconcile
	jobs, err := db.GetProvisionerJobsCreatedAfter(ctx, time.Now().Add(-time.Hour))
	require.NoError(t, err)
	require.Empty(t, jobs)
}

func TestNoReconciliationActionsIfNoPrebuilds(t *testing.T) {
	if !dbtestutil.WillUsePostgres() {
		t.Skip("This test requires postgres")
	}

	// Scenario: No reconciliation actions are taken if there are no prebuilds
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitLong)
	db, pubsub := dbtestutil.NewDB(t)
	cfg := codersdk.PrebuildsConfig{
		ReconciliationInterval: serpent.Duration(testutil.WaitLong),
	}
	logger := testutil.Logger(t)
	controller := NewController(db, pubsub, cfg, logger)

	// given there are presets, but no prebuilds
	org := dbgen.Organization(t, db, database.Organization{})
	user := dbgen.User(t, db, database.User{})
	template := dbgen.Template(t, db, database.Template{
		CreatedBy:      user.ID,
		OrganizationID: org.ID,
	})
	templateVersion := dbgen.TemplateVersion(t, db, database.TemplateVersion{
		TemplateID:     uuid.NullUUID{UUID: template.ID, Valid: true},
		OrganizationID: org.ID,
		CreatedBy:      user.ID,
	})
	preset, err := db.InsertPreset(ctx, database.InsertPresetParams{
		TemplateVersionID: templateVersion.ID,
		Name:              "test",
	})
	require.NoError(t, err)
	_, err = db.InsertPresetParameters(ctx, database.InsertPresetParametersParams{
		TemplateVersionPresetID: preset.ID,
		Names:                   []string{"test"},
		Values:                  []string{"test"},
	})
	require.NoError(t, err)

	// verify that the db state is correct
	presetParameters, err := db.GetPresetParametersByTemplateVersionID(ctx, templateVersion.ID)
	require.NoError(t, err)
	require.NotEmpty(t, presetParameters)

	// when we trigger the reconciliation loop for all templates
	controller.reconcile(ctx, nil)

	// then no reconciliation actions are taken
	// because without prebuilds, there is nothing to reconcile
	// even if there are presets
	jobs, err := db.GetProvisionerJobsCreatedAfter(ctx, time.Now().Add(-time.Hour))
	require.NoError(t, err)
	require.Empty(t, jobs)
}

func setupTestDBTemplate(
	t *testing.T,
	db database.Store,
) (
	orgID uuid.UUID,
	userID uuid.UUID,
	templateID uuid.UUID,
) {
	t.Helper()
	org := dbgen.Organization(t, db, database.Organization{})
	user := dbgen.User(t, db, database.User{})

	template := dbgen.Template(t, db, database.Template{
		CreatedBy:      user.ID,
		OrganizationID: org.ID,
	})

	return org.ID, user.ID, template.ID
}
func setupTestDBPrebuild(
	t *testing.T,
	ctx context.Context,
	db database.Store,
	pubsub pubsub.Pubsub,
	prebuildStatus database.WorkspaceStatus,
	orgID uuid.UUID,
	userID uuid.UUID,
	templateID uuid.UUID,
) (
	templateVersionID uuid.UUID,
	presetID uuid.UUID,
	prebuildID uuid.UUID,
) {
	templateVersionJob := dbgen.ProvisionerJob(t, db, pubsub, database.ProvisionerJob{
		ID:             uuid.New(),
		CreatedAt:      time.Now().Add(-2 * time.Hour),
		CompletedAt:    sql.NullTime{Time: time.Now().Add(-time.Hour), Valid: true},
		OrganizationID: orgID,
		InitiatorID:    userID,
	})
	templateVersion := dbgen.TemplateVersion(t, db, database.TemplateVersion{
		TemplateID:     uuid.NullUUID{UUID: templateID, Valid: true},
		OrganizationID: orgID,
		CreatedBy:      userID,
		JobID:          templateVersionJob.ID,
	})
	db.UpdateTemplateActiveVersionByID(ctx, database.UpdateTemplateActiveVersionByIDParams{
		ID:              templateID,
		ActiveVersionID: templateVersion.ID,
	})
	preset, err := db.InsertPreset(ctx, database.InsertPresetParams{
		TemplateVersionID: templateVersion.ID,
		Name:              "test",
	})
	require.NoError(t, err)
	_, err = db.InsertPresetParameters(ctx, database.InsertPresetParametersParams{
		TemplateVersionPresetID: preset.ID,
		Names:                   []string{"test"},
		Values:                  []string{"test"},
	})
	require.NoError(t, err)
	_, err = db.InsertPresetPrebuild(ctx, database.InsertPresetPrebuildParams{
		ID:               uuid.New(),
		PresetID:         preset.ID,
		DesiredInstances: 1,
	})
	require.NoError(t, err)

	completedAt := sql.NullTime{}
	cancelledAt := sql.NullTime{}
	transition := database.WorkspaceTransitionStart
	deleted := false
	buildError := sql.NullString{}
	switch prebuildStatus {
	case database.WorkspaceStatusRunning:
		completedAt = sql.NullTime{Time: time.Now().Add(-time.Hour), Valid: true}
	case database.WorkspaceStatusStopped:
		completedAt = sql.NullTime{Time: time.Now().Add(-time.Hour), Valid: true}
		transition = database.WorkspaceTransitionStop
	case database.WorkspaceStatusFailed:
		completedAt = sql.NullTime{Time: time.Now().Add(-time.Hour), Valid: true}
		buildError = sql.NullString{String: "build failed", Valid: true}
	case database.WorkspaceStatusCanceled:
		completedAt = sql.NullTime{Time: time.Now().Add(-time.Hour), Valid: true}
		cancelledAt = sql.NullTime{Time: time.Now().Add(-time.Hour), Valid: true}
	case database.WorkspaceStatusDeleted:
		completedAt = sql.NullTime{Time: time.Now().Add(-time.Hour), Valid: true}
		transition = database.WorkspaceTransitionDelete
		deleted = true
	case database.WorkspaceStatusPending:
		completedAt = sql.NullTime{}
		transition = database.WorkspaceTransitionStart
	default:
	}

	workspace := dbgen.Workspace(t, db, database.WorkspaceTable{
		TemplateID:     templateID,
		OrganizationID: orgID,
		OwnerID:        OwnerID,
		Deleted:        deleted,
	})
	job := dbgen.ProvisionerJob(t, db, pubsub, database.ProvisionerJob{
		InitiatorID:    OwnerID,
		CreatedAt:      time.Now().Add(-2 * time.Hour),
		CompletedAt:    completedAt,
		CanceledAt:     cancelledAt,
		OrganizationID: orgID,
		Error:          buildError,
	})
	dbgen.WorkspaceBuild(t, db, database.WorkspaceBuild{
		WorkspaceID:             workspace.ID,
		InitiatorID:             OwnerID,
		TemplateVersionID:       templateVersion.ID,
		JobID:                   job.ID,
		TemplateVersionPresetID: uuid.NullUUID{UUID: preset.ID, Valid: true},
		Transition:              transition,
	})

	return templateVersion.ID, preset.ID, workspace.ID
}

func TestActiveTemplateVersionPrebuilds(t *testing.T) {
	if !dbtestutil.WillUsePostgres() {
		t.Skip("This test requires postgres")
	}

	t.Parallel()

	type testCase struct {
		name                    string
		prebuildStatus          database.WorkspaceStatus
		shouldCreateNewPrebuild bool
		shouldDeleteOldPrebuild bool
	}

	testCases := []testCase{
		{
			name:                    "running prebuild",
			prebuildStatus:          database.WorkspaceStatusRunning,
			shouldCreateNewPrebuild: false,
			shouldDeleteOldPrebuild: false,
		},
		{
			name:                    "stopped prebuild",
			prebuildStatus:          database.WorkspaceStatusStopped,
			shouldCreateNewPrebuild: true,
			shouldDeleteOldPrebuild: false,
		},
		{
			name:                    "failed prebuild",
			prebuildStatus:          database.WorkspaceStatusFailed,
			shouldCreateNewPrebuild: true,
			shouldDeleteOldPrebuild: false,
		},
		{
			name:                    "canceled prebuild",
			prebuildStatus:          database.WorkspaceStatusCanceled,
			shouldCreateNewPrebuild: true,
			shouldDeleteOldPrebuild: false,
		},
		// {
		// 	name:                    "deleted prebuild",
		// 	prebuildStatus:          database.WorkspaceStatusDeleted,
		// 	shouldConsiderPrebuildRunning: false,
		// 	shouldConsiderPrebuildInProgress: false,
		// 	shouldCreateNewPrebuild: true,
		// 	shouldDeleteOldPrebuild: false,
		// },
		{
			name:                    "pending prebuild",
			prebuildStatus:          database.WorkspaceStatusPending,
			shouldCreateNewPrebuild: false,
			shouldDeleteOldPrebuild: false,
		},
	}
	for _, tc := range testCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			ctx := testutil.Context(t, testutil.WaitShort)
			db, pubsub := dbtestutil.NewDB(t)
			cfg := codersdk.PrebuildsConfig{}
			logger := testutil.Logger(t)
			controller := NewController(db, pubsub, cfg, logger)

			orgID, userID, templateID := setupTestDBTemplate(t, db)
			_, _, prebuildID := setupTestDBPrebuild(
				t,
				ctx,
				db,
				pubsub,
				tc.prebuildStatus,
				orgID,
				userID,
				templateID,
			)

			controller.reconcile(ctx, nil)

			createdNewPrebuild := false
			deletedOldPrebuild := true
			workspaces, err := db.GetWorkspacesByTemplateID(ctx, templateID)
			require.NoError(t, err)
			for _, workspace := range workspaces {
				if workspace.ID == prebuildID {
					deletedOldPrebuild = false
				}

				if workspace.ID != prebuildID {
					createdNewPrebuild = true
				}
			}
			require.Equal(t, tc.shouldCreateNewPrebuild, createdNewPrebuild)
			require.Equal(t, tc.shouldDeleteOldPrebuild, deletedOldPrebuild)
		})
	}
}

func TestInactiveTemplateVersionPrebuilds(t *testing.T) {
	// Scenario: Prebuilds are never created and always deleted if the template version is inactive
	t.Parallel()
	t.Skip("todo")
}

type partiallyMockedDB struct {
	mock.Mock
	database.Store
}

func (m *partiallyMockedDB) ClaimPrebuild(ctx context.Context, arg database.ClaimPrebuildParams) (database.ClaimPrebuildRow, error) {
	args := m.Mock.Called(ctx, arg)
	return args.Get(0).(database.ClaimPrebuildRow), args.Error(1)
}

func TestClaimPrebuild(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitSuperLong)

	db, pubsub := dbtestutil.NewDB(t)
	mockedDB := &partiallyMockedDB{
		Store: db,
	}

	client, _, api := coderdtest.NewWithAPI(t, &coderdtest.Options{
		IncludeProvisionerDaemon: true,
		Database:                 mockedDB,
		Pubsub:                   pubsub,
	})

	cfg := codersdk.PrebuildsConfig{}
	logger := testutil.Logger(t)
	controller := NewController(mockedDB, pubsub, cfg, logger)

	const (
		desiredInstances = 1
		presetCount      = 2
	)

	// Setup. // TODO: abstract?
	owner := coderdtest.CreateFirstUser(t, client)
	version := coderdtest.CreateTemplateVersion(t, client, owner.OrganizationID, templateWithAgentAndPresetsWithPrebuilds(desiredInstances))
	_ = coderdtest.AwaitTemplateVersionJobCompleted(t, client, version.ID)
	coderdtest.CreateTemplate(t, client, owner.OrganizationID, version.ID)
	presets, err := client.TemplateVersionPresets(ctx, version.ID)
	require.NoError(t, err)
	require.Len(t, presets, presetCount)


	//
	//
	//
	//
	//
	// TODO: for Monday: need to get this feature entitled so the EnterpriseClaimer is used, otherwise it's a noop.
	//
	//
	//
	//
	//
	//

	api.Entitlements.Modify(func(entitlements *codersdk.Entitlements) {
		entitlements.Features[codersdk.FeatureWorkspacePrebuilds] = codersdk.Feature{
			Enabled:     true,
			Entitlement: codersdk.EntitlementEntitled,
		}
	})
	// TODO: can't use coderd.PubsubEventLicenses const because of an import cycle.
	require.NoError(t, api.Pubsub.Publish("licenses", []byte("add")))

	userClient, user := coderdtest.CreateAnotherUser(t, client, owner.OrganizationID, rbac.RoleMember())

	ctx = dbauthz.AsSystemRestricted(ctx)

	//claimer := EnterpriseClaimer{}
	//prebuildsUser, err := db.GetUserByID(ctx, claimer.Initiator())
	//require.NoError(t, err)

	controller.reconcile(ctx, nil)

	runningPrebuilds := make(map[uuid.UUID]database.GetRunningPrebuildsRow, desiredInstances*presetCount)
	require.Eventually(t, func() bool {
		rows, err := mockedDB.GetRunningPrebuilds(ctx)
		require.NoError(t, err)
		t.Logf("found %d running prebuilds so far", len(rows))

		for _, row := range rows {
			runningPrebuilds[row.CurrentPresetID.UUID] = row
		}

		return len(runningPrebuilds) == (desiredInstances * presetCount)
	}, testutil.WaitSuperLong, testutil.IntervalSlow)

	workspaceName := strings.ReplaceAll(testutil.GetRandomName(t), "_", "-")

	params := database.ClaimPrebuildParams{
		NewUserID: user.ID,
		NewName:   workspaceName,
		PresetID:  presets[0].ID,
	}
	mockedDB.On("ClaimPrebuild", mock.Anything, params).Return(db.ClaimPrebuild(ctx, params)).Once()

	// When: a user creates a new workspace with a preset for which prebuilds are configured.
	userWorkspace, err := userClient.CreateUserWorkspace(ctx, user.Username, codersdk.CreateWorkspaceRequest{
		TemplateVersionID:        version.ID,
		Name:                     workspaceName,
		TemplateVersionPresetID:  presets[0].ID,
		ClaimPrebuildIfAvailable: true, // TODO: doesn't do anything yet; it probably should though.
	})
	require.NoError(t, err)
	coderdtest.AwaitWorkspaceBuildJobCompleted(t, client, userWorkspace.LatestBuild.ID)

	require.True(t, mockedDB.AssertCalled(t, "ClaimPrebuild", ctx, params))

	for _, rp := range runningPrebuilds {
		t.Logf("prev >>%s", rp.WorkspaceName)
	}

	pb, err := mockedDB.GetRunningPrebuilds(ctx)
	require.NoError(t, err)
	for _, rp := range pb {
		t.Logf("new >>%s", rp.WorkspaceName)
	}
	require.Len(t, pb, 4)

	//var prebuildIDs []uuid.UUID
	//// Given: two running prebuilds.
	//for i := 0; i < 2; i++ {
	//	prebuiltWorkspace := dbgen.Workspace(t, db, database.WorkspaceTable{
	//		TemplateID:     template.ID,
	//		OrganizationID: owner.OrganizationID,
	//		OwnerID:        prebuildsUser.ID,
	//	})
	//	prebuildIDs = append(prebuildIDs, prebuiltWorkspace.ID)
	//
	//	job := dbgen.ProvisionerJob(t, db, pubsub, database.ProvisionerJob{
	//		InitiatorID:    OwnerID,
	//		CreatedAt:      time.Now().Add(-2 * time.Hour),
	//		CompletedAt:    sql.NullTime{Time: time.Now().Add(-time.Hour), Valid: true},
	//		OrganizationID: owner.OrganizationID,
	//		Provisioner:    database.ProvisionerTypeEcho,
	//		Type:           database.ProvisionerJobTypeWorkspaceBuild,
	//	})
	//	dbgen.WorkspaceBuild(t, db, database.WorkspaceBuild{
	//		WorkspaceID:             prebuiltWorkspace.ID,
	//		InitiatorID:             OwnerID,
	//		TemplateVersionID:       version.ID,
	//		JobID:                   job.ID,
	//		TemplateVersionPresetID: uuid.NullUUID{UUID: presets[0].ID, Valid: true},
	//		Transition:              database.WorkspaceTransitionStart,
	//	})
	//
	//	// Setup workspace agent which is in a given state. // TODO: table test with unclaimable when !ready
	//	resource := dbgen.WorkspaceResource(t, db, database.WorkspaceResource{
	//		ID:         uuid.New(),
	//		CreatedAt:  time.Now().Add(-1 * time.Hour),
	//		JobID:      job.ID,
	//		Transition: database.WorkspaceTransitionStart,
	//		Type:       "some_compute_resource",
	//		Name:       "beep_boop",
	//	})
	//	agent := dbgen.WorkspaceAgent(t, db, database.WorkspaceAgent{
	//		ID:               uuid.New(),
	//		CreatedAt:        time.Now().Add(-1 * time.Hour),
	//		Name:             "main",
	//		FirstConnectedAt: sql.NullTime{Time: time.Now().Add(-1 * time.Hour), Valid: true},
	//		LastConnectedAt:  sql.NullTime{Time: time.Now().Add(-1 * time.Hour), Valid: true},
	//		ResourceID:       resource.ID,
	//	})
	//	require.NoError(t, db.UpdateWorkspaceAgentLifecycleStateByID(ctx, database.UpdateWorkspaceAgentLifecycleStateByIDParams{
	//		ID:             agent.ID,
	//		LifecycleState: database.WorkspaceAgentLifecycleStateReady,
	//		StartedAt:      sql.NullTime{Time: time.Now().Add(-1 * time.Hour), Valid: true},
	//		ReadyAt:        sql.NullTime{Time: time.Now().Add(-1 * time.Hour), Valid: true},
	//	}))
	//}
	//
	////Then: validate that these prebuilds are indeed considered running.
	//running, err := db.GetRunningPrebuilds(ctx)
	//require.NoError(t, err)
	//var (
	//	found []uuid.UUID
	//	ready int
	//)
	//for _, w := range running {
	//	found = append(found, w.WorkspaceID)
	//	if w.Ready {
	//		ready++
	//	}
	//}
	//require.ElementsMatch(t, prebuildIDs, found)
	//require.EqualValues(t, len(prebuildIDs), ready)

	//// When: a user creates a new workspace with a preset for which prebuilds are configured.
	//userWorkspace, err := userClient.CreateUserWorkspace(ctx, user.Username, codersdk.CreateWorkspaceRequest{
	//	TemplateVersionID:        templateVersion.ID,
	//	Name:                     strings.ReplaceAll(testutil.GetRandomName(t), "_", "-"),
	//	TemplateVersionPresetID:  preset.ID,
	//	ClaimPrebuildIfAvailable: true, // TODO: doesn't do anything yet; it probably should though.
	//})
	//require.NoError(t, err)
	//require.NoError(t, cliui.WorkspaceBuild(ctx, os.Stderr, userClient, userWorkspace.LatestBuild.ID))
}

//func addPremiumLicense(t *testing.T) (*codersdk.Entitlements, error) {
//	premiumLicense := (&coderdenttest.LicenseOptions{
//		AccountType:   "salesforce",
//		AccountID:     "Charlie",
//		DeploymentIDs: nil,
//		Trial:         false,
//		FeatureSet:    codersdk.FeatureSetPremium,
//		AllFeatures:   true,
//	}).Valid(time.Now())
//	licenses := []*coderdenttest.LicenseOptions{premiumLicense}
//
//	allEnablements := make(map[codersdk.FeatureName]bool, len(codersdk.FeatureNames))
//	for _, e := range codersdk.FeatureNames {
//		allEnablements[e] = true
//	}
//
//	generatedLicenses := make([]database.License, 0, len(licenses))
//	for i, lo := range licenses {
//		generatedLicenses = append(generatedLicenses, database.License{
//			ID:         int32(i),
//			UploadedAt: time.Now().Add(time.Hour * -1),
//			JWT:        lo.Generate(t),
//			Exp:        lo.GraceAt,
//			UUID:       uuid.New(),
//		})
//	}
//
//	ents, err := license.LicensesEntitlements(time.Now(), generatedLicenses, allEnablements, coderdenttest.Keys, license.FeatureArguments{})
//	return &ents, err
//}

func templateWithAgentAndPresetsWithPrebuilds(desiredInstances int32) *echo.Responses {
	return &echo.Responses{
		Parse: echo.ParseComplete,
		ProvisionPlan: []*proto.Response{
			{
				Type: &proto.Response_Plan{
					Plan: &proto.PlanComplete{
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
						Presets: []*proto.Preset{
							{
								Name: "preset-a",
								Parameters: []*proto.PresetParameter{
									{
										Name:  "k1",
										Value: "v1",
									},
								},
								Prebuild: &proto.Prebuild{
									Instances: desiredInstances,
								},
							},
							{
								Name: "preset-b",
								Parameters: []*proto.PresetParameter{
									{
										Name:  "k1",
										Value: "v2",
									},
								},
								Prebuild: &proto.Prebuild{
									Instances: desiredInstances,
								},
							},
						},
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

// TODO(dannyk): test that prebuilds are only attempted to be claimed for net-new workspace builds
// TODO (sasswart): test idempotency of reconciliation
// TODO (sasswart): test mutual exclusion
