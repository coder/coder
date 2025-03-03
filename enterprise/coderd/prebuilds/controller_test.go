package prebuilds_test

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/coder/serpent"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbgen"
	"github.com/coder/coder/v2/coderd/database/dbtestutil"
	"github.com/coder/coder/v2/coderd/database/pubsub"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/enterprise/coderd/prebuilds"
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
	controller := prebuilds.NewController(db, pubsub, cfg, logger)

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
	controller.Reconcile(ctx, nil)

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
	controller := prebuilds.NewController(db, pubsub, cfg, logger)

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
	controller.Reconcile(ctx, nil)

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
		OwnerID:        prebuilds.OwnerID,
		Deleted:        deleted,
	})
	job := dbgen.ProvisionerJob(t, db, pubsub, database.ProvisionerJob{
		InitiatorID:    prebuilds.OwnerID,
		CreatedAt:      time.Now().Add(-2 * time.Hour),
		CompletedAt:    completedAt,
		CanceledAt:     cancelledAt,
		OrganizationID: orgID,
		Error:          buildError,
	})
	dbgen.WorkspaceBuild(t, db, database.WorkspaceBuild{
		WorkspaceID:             workspace.ID,
		InitiatorID:             prebuilds.OwnerID,
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
			controller := prebuilds.NewController(db, pubsub, cfg, logger)

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

			controller.Reconcile(ctx, nil)

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

	ctx := testutil.Context(t, testutil.WaitShort)
	db, pubsub := dbtestutil.NewDB(t)
	cfg := codersdk.PrebuildsConfig{}
	logger := testutil.Logger(t)
	controller := prebuilds.NewController(db, pubsub, cfg, logger)

	// when does a prebuild get deleted?
	// * when it is in some way permanently ineligible to be claimed
	//   * this could be because the build failed or was canceled
	//   * or it belongs to a template version that is no longer active
	//   * or it belongs to a template version that is deprecated
	// * when there are more prebuilds than the preset desires
	//   * someone could have manually created a workspace for the prebuild user
	// * any workspaces that were created for the prebuilds user and don't match a preset should be deleted - deferred

	// given a preset that desires 2 prebuilds
	// and there are 3 running prebuilds for the preset
	// and there are 4 non-running prebuilds for the preset
	// * one is not running because its latest build was a stop transition
	// * another is not running because its latest build was a delete transition
	// * a third is not running because its latest build was a start transition but the build failed
	// * a fourth is not running because its latest build was a start transition but the build was canceled
	// when we trigger the reconciliation loop for all templates
	controller.Reconcile(ctx, nil)
	// then the four non running prebuilds are deleted
	// and 1 of the running prebuilds is deleted
	// because stopped, deleted and failed builds are not considered running in terms of the definition of "running" above.
}

// TODO (sasswart): test idempotency of reconciliation
// TODO (sasswart): test mutual exclusion
