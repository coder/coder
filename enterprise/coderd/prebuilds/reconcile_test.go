package prebuilds_test

import (
	"context"
	"database/sql"
	"fmt"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/assert"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/coderd/database/dbtime"
	"github.com/coder/coder/v2/coderd/files"
	"github.com/coder/coder/v2/coderd/notifications"
	"github.com/coder/coder/v2/coderd/notifications/notificationstest"
	"github.com/coder/coder/v2/coderd/util/slice"
	sdkproto "github.com/coder/coder/v2/provisionersdk/proto"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"tailscale.com/types/ptr"

	"cdr.dev/slog"
	"cdr.dev/slog/sloggers/slogjson"
	"cdr.dev/slog/sloggers/slogtest"
	"github.com/coder/quartz"

	"github.com/coder/serpent"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbgen"
	"github.com/coder/coder/v2/coderd/database/dbtestutil"
	"github.com/coder/coder/v2/coderd/database/pubsub"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/enterprise/coderd/prebuilds"
	"github.com/coder/coder/v2/testutil"
)

func TestNoReconciliationActionsIfNoPresets(t *testing.T) {
	// Scenario: No reconciliation actions are taken if there are no presets
	t.Parallel()

	if !dbtestutil.WillUsePostgres() {
		t.Skip("dbmem times out on nesting transactions, postgres ignores the inner ones")
	}

	clock := quartz.NewMock(t)
	ctx := testutil.Context(t, testutil.WaitLong)
	db, ps := dbtestutil.NewDB(t)
	cfg := codersdk.PrebuildsConfig{
		ReconciliationInterval: serpent.Duration(testutil.WaitLong),
	}
	logger := testutil.Logger(t)
	cache := files.New(prometheus.NewRegistry(), &coderdtest.FakeAuthorizer{})
	controller := prebuilds.NewStoreReconciler(db, ps, cache, cfg, logger, quartz.NewMock(t), prometheus.NewRegistry(), newNoopEnqueuer())

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
	require.NoError(t, controller.ReconcileAll(ctx))

	// then no reconciliation actions are taken
	// because without presets, there are no prebuilds
	// and without prebuilds, there is nothing to reconcile
	jobs, err := db.GetProvisionerJobsCreatedAfter(ctx, clock.Now().Add(earlier))
	require.NoError(t, err)
	require.Empty(t, jobs)
}

func TestNoReconciliationActionsIfNoPrebuilds(t *testing.T) {
	// Scenario: No reconciliation actions are taken if there are no prebuilds
	t.Parallel()

	if !dbtestutil.WillUsePostgres() {
		t.Skip("dbmem times out on nesting transactions, postgres ignores the inner ones")
	}

	clock := quartz.NewMock(t)
	ctx := testutil.Context(t, testutil.WaitLong)
	db, ps := dbtestutil.NewDB(t)
	cfg := codersdk.PrebuildsConfig{
		ReconciliationInterval: serpent.Duration(testutil.WaitLong),
	}
	logger := testutil.Logger(t)
	cache := files.New(prometheus.NewRegistry(), &coderdtest.FakeAuthorizer{})
	controller := prebuilds.NewStoreReconciler(db, ps, cache, cfg, logger, quartz.NewMock(t), prometheus.NewRegistry(), newNoopEnqueuer())

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
	require.NoError(t, controller.ReconcileAll(ctx))

	// then no reconciliation actions are taken
	// because without prebuilds, there is nothing to reconcile
	// even if there are presets
	jobs, err := db.GetProvisionerJobsCreatedAfter(ctx, clock.Now().Add(earlier))
	require.NoError(t, err)
	require.Empty(t, jobs)
}

func TestPrebuildReconciliation(t *testing.T) {
	t.Parallel()

	if !dbtestutil.WillUsePostgres() {
		t.Skip("This test requires postgres")
	}

	type testCase struct {
		name                      string
		prebuildLatestTransitions []database.WorkspaceTransition
		prebuildJobStatuses       []database.ProvisionerJobStatus
		templateVersionActive     []bool
		templateDeleted           []bool
		shouldCreateNewPrebuild   *bool
		shouldDeleteOldPrebuild   *bool
	}

	testCases := []testCase{
		{
			name:                      "never create prebuilds for inactive template versions",
			prebuildLatestTransitions: allTransitions,
			prebuildJobStatuses:       allJobStatuses,
			templateVersionActive:     []bool{false},
			shouldCreateNewPrebuild:   ptr.To(false),
			templateDeleted:           []bool{false},
		},
		{
			name: "no need to create a new prebuild if one is already running",
			prebuildLatestTransitions: []database.WorkspaceTransition{
				database.WorkspaceTransitionStart,
			},
			prebuildJobStatuses: []database.ProvisionerJobStatus{
				database.ProvisionerJobStatusSucceeded,
			},
			templateVersionActive:   []bool{true},
			shouldCreateNewPrebuild: ptr.To(false),
			templateDeleted:         []bool{false},
		},
		{
			name: "don't create a new prebuild if one is queued to build or already building",
			prebuildLatestTransitions: []database.WorkspaceTransition{
				database.WorkspaceTransitionStart,
			},
			prebuildJobStatuses: []database.ProvisionerJobStatus{
				database.ProvisionerJobStatusPending,
				database.ProvisionerJobStatusRunning,
			},
			templateVersionActive:   []bool{true},
			shouldCreateNewPrebuild: ptr.To(false),
			templateDeleted:         []bool{false},
		},
		{
			name: "create a new prebuild if one is in a state that disqualifies it from ever being claimed",
			prebuildLatestTransitions: []database.WorkspaceTransition{
				database.WorkspaceTransitionStop,
				database.WorkspaceTransitionDelete,
			},
			prebuildJobStatuses: []database.ProvisionerJobStatus{
				database.ProvisionerJobStatusPending,
				database.ProvisionerJobStatusRunning,
				database.ProvisionerJobStatusCanceling,
				database.ProvisionerJobStatusSucceeded,
			},
			templateVersionActive:   []bool{true},
			shouldCreateNewPrebuild: ptr.To(true),
			templateDeleted:         []bool{false},
		},
		{
			// See TestFailedBuildBackoff for the start/failed case.
			name: "create a new prebuild if one is in any kind of exceptional state",
			prebuildLatestTransitions: []database.WorkspaceTransition{
				database.WorkspaceTransitionStop,
				database.WorkspaceTransitionDelete,
			},
			prebuildJobStatuses: []database.ProvisionerJobStatus{
				database.ProvisionerJobStatusCanceled,
			},
			templateVersionActive:   []bool{true},
			shouldCreateNewPrebuild: ptr.To(true),
			templateDeleted:         []bool{false},
		},
		{
			name: "never attempt to interfere with active builds",
			// The workspace builder does not allow scheduling a new build if there is already a build
			// pending, running, or canceling. As such, we should never attempt to start, stop or delete
			// such prebuilds. Rather, we should wait for the existing build to complete and reconcile
			// again in the next cycle.
			prebuildLatestTransitions: allTransitions,
			prebuildJobStatuses: []database.ProvisionerJobStatus{
				database.ProvisionerJobStatusPending,
				database.ProvisionerJobStatusRunning,
				database.ProvisionerJobStatusCanceling,
			},
			templateVersionActive:   []bool{true, false},
			shouldDeleteOldPrebuild: ptr.To(false),
			templateDeleted:         []bool{false},
		},
		{
			name: "never delete prebuilds in an exceptional state",
			// We don't want to destroy evidence that might be useful to operators
			// when troubleshooting issues. So we leave these prebuilds in place.
			// Operators are expected to manually delete these prebuilds.
			prebuildLatestTransitions: allTransitions,
			prebuildJobStatuses: []database.ProvisionerJobStatus{
				database.ProvisionerJobStatusCanceled,
				database.ProvisionerJobStatusFailed,
			},
			templateVersionActive:   []bool{true, false},
			shouldDeleteOldPrebuild: ptr.To(false),
			templateDeleted:         []bool{false},
		},
		{
			name: "delete running prebuilds for inactive template versions",
			// We only support prebuilds for active template versions.
			// If a template version is inactive, we should delete any prebuilds
			// that are running.
			prebuildLatestTransitions: []database.WorkspaceTransition{
				database.WorkspaceTransitionStart,
			},
			prebuildJobStatuses: []database.ProvisionerJobStatus{
				database.ProvisionerJobStatusSucceeded,
			},
			templateVersionActive:   []bool{false},
			shouldDeleteOldPrebuild: ptr.To(true),
			templateDeleted:         []bool{false},
		},
		{
			name: "don't delete running prebuilds for active template versions",
			prebuildLatestTransitions: []database.WorkspaceTransition{
				database.WorkspaceTransitionStart,
			},
			prebuildJobStatuses: []database.ProvisionerJobStatus{
				database.ProvisionerJobStatusSucceeded,
			},
			templateVersionActive:   []bool{true},
			shouldDeleteOldPrebuild: ptr.To(false),
			templateDeleted:         []bool{false},
		},
		{
			name: "don't delete stopped or already deleted prebuilds",
			// We don't ever stop prebuilds. A stopped prebuild is an exceptional state.
			// As such we keep it, to allow operators to investigate the cause.
			prebuildLatestTransitions: []database.WorkspaceTransition{
				database.WorkspaceTransitionStop,
				database.WorkspaceTransitionDelete,
			},
			prebuildJobStatuses: []database.ProvisionerJobStatus{
				database.ProvisionerJobStatusSucceeded,
			},
			templateVersionActive:   []bool{true, false},
			shouldDeleteOldPrebuild: ptr.To(false),
			templateDeleted:         []bool{false},
		},
		{
			// Templates can be soft-deleted (`deleted=true`) or hard-deleted (row is removed).
			// On the former there is *no* DB constraint to prevent soft deletion, so we have to ensure that if somehow
			// the template was soft-deleted any running prebuilds will be removed.
			// On the latter there is a DB constraint to prevent row deletion if any workspaces reference the deleting template.
			name:                      "soft-deleted templates MAY have prebuilds",
			prebuildLatestTransitions: []database.WorkspaceTransition{database.WorkspaceTransitionStart},
			prebuildJobStatuses:       []database.ProvisionerJobStatus{database.ProvisionerJobStatusSucceeded},
			templateVersionActive:     []bool{true, false},
			shouldCreateNewPrebuild:   ptr.To(false),
			shouldDeleteOldPrebuild:   ptr.To(true),
			templateDeleted:           []bool{true},
		},
	}
	for _, tc := range testCases {
		for _, templateVersionActive := range tc.templateVersionActive {
			for _, prebuildLatestTransition := range tc.prebuildLatestTransitions {
				for _, prebuildJobStatus := range tc.prebuildJobStatuses {
					for _, templateDeleted := range tc.templateDeleted {
						for _, useBrokenPubsub := range []bool{true, false} {
							t.Run(fmt.Sprintf("%s - %s - %s - pubsub_broken=%v", tc.name, prebuildLatestTransition, prebuildJobStatus, useBrokenPubsub), func(t *testing.T) {
								t.Parallel()
								t.Cleanup(func() {
									if t.Failed() {
										t.Logf("failed to run test: %s", tc.name)
										t.Logf("templateVersionActive: %t", templateVersionActive)
										t.Logf("prebuildLatestTransition: %s", prebuildLatestTransition)
										t.Logf("prebuildJobStatus: %s", prebuildJobStatus)
									}
								})
								clock := quartz.NewMock(t)
								ctx := testutil.Context(t, testutil.WaitShort)
								cfg := codersdk.PrebuildsConfig{}
								logger := slogtest.Make(
									t, &slogtest.Options{IgnoreErrors: true},
								).Leveled(slog.LevelDebug)
								db, pubSub := dbtestutil.NewDB(t)

								ownerID := uuid.New()
								dbgen.User(t, db, database.User{
									ID: ownerID,
								})
								org, template := setupTestDBTemplate(t, db, ownerID, templateDeleted)
								templateVersionID := setupTestDBTemplateVersion(
									ctx,
									t,
									clock,
									db,
									pubSub,
									org.ID,
									ownerID,
									template.ID,
								)
								preset := setupTestDBPreset(
									t,
									db,
									templateVersionID,
									1,
									uuid.New().String(),
								)
								prebuild, _ := setupTestDBPrebuild(
									t,
									clock,
									db,
									pubSub,
									prebuildLatestTransition,
									prebuildJobStatus,
									org.ID,
									preset,
									template.ID,
									templateVersionID,
								)

								setupTestDBPrebuildAntagonists(t, db, pubSub, org)

								if !templateVersionActive {
									// Create a new template version and mark it as active
									// This marks the template version that we care about as inactive
									setupTestDBTemplateVersion(ctx, t, clock, db, pubSub, org.ID, ownerID, template.ID)
								}

								if useBrokenPubsub {
									pubSub = &brokenPublisher{Pubsub: pubSub}
								}
								cache := files.New(prometheus.NewRegistry(), &coderdtest.FakeAuthorizer{})
								controller := prebuilds.NewStoreReconciler(db, pubSub, cache, cfg, logger, quartz.NewMock(t), prometheus.NewRegistry(), newNoopEnqueuer())

								// Run the reconciliation multiple times to ensure idempotency
								// 8 was arbitrary, but large enough to reasonably trust the result
								for i := 1; i <= 8; i++ {
									require.NoErrorf(t, controller.ReconcileAll(ctx), "failed on iteration %d", i)

									if tc.shouldCreateNewPrebuild != nil {
										newPrebuildCount := 0
										workspaces, err := db.GetWorkspacesByTemplateID(ctx, template.ID)
										require.NoError(t, err)
										for _, workspace := range workspaces {
											if workspace.ID != prebuild.ID {
												newPrebuildCount++
											}
										}
										// This test configures a preset that desires one prebuild.
										// In cases where new prebuilds should be created, there should be exactly one.
										require.Equal(t, *tc.shouldCreateNewPrebuild, newPrebuildCount == 1)
									}

									if tc.shouldDeleteOldPrebuild != nil {
										builds, err := db.GetWorkspaceBuildsByWorkspaceID(ctx, database.GetWorkspaceBuildsByWorkspaceIDParams{
											WorkspaceID: prebuild.ID,
										})
										require.NoError(t, err)
										if *tc.shouldDeleteOldPrebuild {
											require.Equal(t, 2, len(builds))
											require.Equal(t, database.WorkspaceTransitionDelete, builds[0].Transition)
										} else {
											require.Equal(t, 1, len(builds))
											require.Equal(t, prebuildLatestTransition, builds[0].Transition)
										}
									}
								}
							})
						}
					}
				}
			}
		}
	}
}

// brokenPublisher is used to validate that Publish() calls which always fail do not affect the reconciler's behavior,
// since the messages published are not essential but merely advisory.
type brokenPublisher struct {
	pubsub.Pubsub
}

// Publish deliberately fails.
// I'm explicitly _not_ checking for EventJobPosted (coderd/database/provisionerjobs/provisionerjobs.go) since that
// requires too much knowledge of the underlying implementation.
func (*brokenPublisher) Publish(event string, _ []byte) error {
	// Mimick some work being done.
	<-time.After(testutil.IntervalFast)
	return xerrors.Errorf("failed to publish %q", event)
}

func TestMultiplePresetsPerTemplateVersion(t *testing.T) {
	t.Parallel()

	if !dbtestutil.WillUsePostgres() {
		t.Skip("This test requires postgres")
	}

	prebuildLatestTransition := database.WorkspaceTransitionStart
	prebuildJobStatus := database.ProvisionerJobStatusRunning
	templateDeleted := false

	clock := quartz.NewMock(t)
	ctx := testutil.Context(t, testutil.WaitShort)
	cfg := codersdk.PrebuildsConfig{}
	logger := slogtest.Make(
		t, &slogtest.Options{IgnoreErrors: true},
	).Leveled(slog.LevelDebug)
	db, pubSub := dbtestutil.NewDB(t)
	cache := files.New(prometheus.NewRegistry(), &coderdtest.FakeAuthorizer{})
	controller := prebuilds.NewStoreReconciler(db, pubSub, cache, cfg, logger, quartz.NewMock(t), prometheus.NewRegistry(), newNoopEnqueuer())

	ownerID := uuid.New()
	dbgen.User(t, db, database.User{
		ID: ownerID,
	})
	org, template := setupTestDBTemplate(t, db, ownerID, templateDeleted)
	templateVersionID := setupTestDBTemplateVersion(
		ctx,
		t,
		clock,
		db,
		pubSub,
		org.ID,
		ownerID,
		template.ID,
	)
	preset := setupTestDBPreset(
		t,
		db,
		templateVersionID,
		4,
		uuid.New().String(),
	)
	preset2 := setupTestDBPreset(
		t,
		db,
		templateVersionID,
		10,
		uuid.New().String(),
	)
	prebuildIDs := make([]uuid.UUID, 0)
	for i := 0; i < int(preset.DesiredInstances.Int32); i++ {
		prebuild, _ := setupTestDBPrebuild(
			t,
			clock,
			db,
			pubSub,
			prebuildLatestTransition,
			prebuildJobStatus,
			org.ID,
			preset,
			template.ID,
			templateVersionID,
		)
		prebuildIDs = append(prebuildIDs, prebuild.ID)
	}

	// Run the reconciliation multiple times to ensure idempotency
	// 8 was arbitrary, but large enough to reasonably trust the result
	for i := 1; i <= 8; i++ {
		require.NoErrorf(t, controller.ReconcileAll(ctx), "failed on iteration %d", i)

		newPrebuildCount := 0
		workspaces, err := db.GetWorkspacesByTemplateID(ctx, template.ID)
		require.NoError(t, err)
		for _, workspace := range workspaces {
			if slice.Contains(prebuildIDs, workspace.ID) {
				continue
			}
			newPrebuildCount++
		}

		// NOTE: preset1 doesn't block creation of instances in preset2
		require.Equal(t, preset2.DesiredInstances.Int32, int32(newPrebuildCount)) // nolint:gosec
	}
}

func TestPrebuildScheduling(t *testing.T) {
	t.Parallel()

	if !dbtestutil.WillUsePostgres() {
		t.Skip("This test requires postgres")
	}

	templateDeleted := false

	// The test includes 2 presets, each with 2 schedules.
	// It checks that the number of created prebuilds match expectations for various provided times,
	// based on the corresponding schedules.
	testCases := []struct {
		name string
		// now specifies the current time.
		now time.Time
		// expected prebuild counts for preset1 and preset2, respectively.
		expectedPrebuildCounts []int
	}{
		{
			name:                   "Before the 1st schedule",
			now:                    mustParseTime(t, time.RFC1123, "Mon, 02 Jun 2025 01:00:00 UTC"),
			expectedPrebuildCounts: []int{1, 1},
		},
		{
			name:                   "1st schedule",
			now:                    mustParseTime(t, time.RFC1123, "Mon, 02 Jun 2025 03:00:00 UTC"),
			expectedPrebuildCounts: []int{2, 1},
		},
		{
			name:                   "2nd schedule",
			now:                    mustParseTime(t, time.RFC1123, "Mon, 02 Jun 2025 07:00:00 UTC"),
			expectedPrebuildCounts: []int{3, 1},
		},
		{
			name:                   "3rd schedule",
			now:                    mustParseTime(t, time.RFC1123, "Mon, 02 Jun 2025 11:00:00 UTC"),
			expectedPrebuildCounts: []int{1, 4},
		},
		{
			name:                   "4th schedule",
			now:                    mustParseTime(t, time.RFC1123, "Mon, 02 Jun 2025 15:00:00 UTC"),
			expectedPrebuildCounts: []int{1, 5},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			clock := quartz.NewMock(t)
			clock.Set(tc.now)
			ctx := testutil.Context(t, testutil.WaitShort)
			cfg := codersdk.PrebuildsConfig{}
			logger := slogtest.Make(
				t, &slogtest.Options{IgnoreErrors: true},
			).Leveled(slog.LevelDebug)
			db, pubSub := dbtestutil.NewDB(t)
			cache := files.New(prometheus.NewRegistry(), &coderdtest.FakeAuthorizer{})
			controller := prebuilds.NewStoreReconciler(db, pubSub, cache, cfg, logger, clock, prometheus.NewRegistry(), newNoopEnqueuer())

			ownerID := uuid.New()
			dbgen.User(t, db, database.User{
				ID: ownerID,
			})
			org, template := setupTestDBTemplate(t, db, ownerID, templateDeleted)
			templateVersionID := setupTestDBTemplateVersion(
				ctx,
				t,
				clock,
				db,
				pubSub,
				org.ID,
				ownerID,
				template.ID,
			)
			preset1 := setupTestDBPresetWithScheduling(
				t,
				db,
				templateVersionID,
				1,
				uuid.New().String(),
				"UTC",
			)
			preset2 := setupTestDBPresetWithScheduling(
				t,
				db,
				templateVersionID,
				1,
				uuid.New().String(),
				"UTC",
			)

			dbgen.PresetPrebuildSchedule(t, db, database.InsertPresetPrebuildScheduleParams{
				PresetID:         preset1.ID,
				CronExpression:   "* 2-4 * * 1-5",
				DesiredInstances: 2,
			})
			dbgen.PresetPrebuildSchedule(t, db, database.InsertPresetPrebuildScheduleParams{
				PresetID:         preset1.ID,
				CronExpression:   "* 6-8 * * 1-5",
				DesiredInstances: 3,
			})
			dbgen.PresetPrebuildSchedule(t, db, database.InsertPresetPrebuildScheduleParams{
				PresetID:         preset2.ID,
				CronExpression:   "* 10-12 * * 1-5",
				DesiredInstances: 4,
			})
			dbgen.PresetPrebuildSchedule(t, db, database.InsertPresetPrebuildScheduleParams{
				PresetID:         preset2.ID,
				CronExpression:   "* 14-16 * * 1-5",
				DesiredInstances: 5,
			})

			err := controller.ReconcileAll(ctx)
			require.NoError(t, err)

			// get workspace builds
			workspaces, err := db.GetWorkspacesByTemplateID(ctx, template.ID)
			require.NoError(t, err)
			workspaceIDs := make([]uuid.UUID, 0, len(workspaces))
			for _, workspace := range workspaces {
				workspaceIDs = append(workspaceIDs, workspace.ID)
			}
			workspaceBuilds, err := db.GetLatestWorkspaceBuildsByWorkspaceIDs(ctx, workspaceIDs)
			require.NoError(t, err)

			// calculate number of workspace builds per preset
			var (
				preset1PrebuildCount int
				preset2PrebuildCount int
			)
			for _, workspaceBuild := range workspaceBuilds {
				if preset1.ID == workspaceBuild.TemplateVersionPresetID.UUID {
					preset1PrebuildCount++
				}
				if preset2.ID == workspaceBuild.TemplateVersionPresetID.UUID {
					preset2PrebuildCount++
				}
			}

			require.Equal(t, tc.expectedPrebuildCounts[0], preset1PrebuildCount)
			require.Equal(t, tc.expectedPrebuildCounts[1], preset2PrebuildCount)
		})
	}
}

func TestInvalidPreset(t *testing.T) {
	t.Parallel()

	if !dbtestutil.WillUsePostgres() {
		t.Skip("This test requires postgres")
	}

	templateDeleted := false

	clock := quartz.NewMock(t)
	ctx := testutil.Context(t, testutil.WaitShort)
	cfg := codersdk.PrebuildsConfig{}
	logger := slogtest.Make(
		t, &slogtest.Options{IgnoreErrors: true},
	).Leveled(slog.LevelDebug)
	db, pubSub := dbtestutil.NewDB(t)
	cache := files.New(prometheus.NewRegistry(), &coderdtest.FakeAuthorizer{})
	controller := prebuilds.NewStoreReconciler(db, pubSub, cache, cfg, logger, quartz.NewMock(t), prometheus.NewRegistry(), newNoopEnqueuer())

	ownerID := uuid.New()
	dbgen.User(t, db, database.User{
		ID: ownerID,
	})
	org, template := setupTestDBTemplate(t, db, ownerID, templateDeleted)
	templateVersionID := setupTestDBTemplateVersion(
		ctx,
		t,
		clock,
		db,
		pubSub,
		org.ID,
		ownerID,
		template.ID,
	)
	// Add required param, which is not set in preset. It means that creating of prebuild will constantly fail.
	dbgen.TemplateVersionParameter(t, db, database.TemplateVersionParameter{
		TemplateVersionID: templateVersionID,
		Name:              "required-param",
		Description:       "required param to make sure creating prebuild will fail",
		Type:              "bool",
		DefaultValue:      "",
		Required:          true,
	})
	setupTestDBPreset(
		t,
		db,
		templateVersionID,
		1,
		uuid.New().String(),
	)

	// Run the reconciliation multiple times to ensure idempotency
	// 8 was arbitrary, but large enough to reasonably trust the result
	for i := 1; i <= 8; i++ {
		require.NoErrorf(t, controller.ReconcileAll(ctx), "failed on iteration %d", i)

		workspaces, err := db.GetWorkspacesByTemplateID(ctx, template.ID)
		require.NoError(t, err)
		newPrebuildCount := len(workspaces)

		// NOTE: we don't have any new prebuilds, because their creation constantly fails.
		require.Equal(t, int32(0), int32(newPrebuildCount)) // nolint:gosec
	}
}

func TestDeletionOfPrebuiltWorkspaceWithInvalidPreset(t *testing.T) {
	t.Parallel()

	if !dbtestutil.WillUsePostgres() {
		t.Skip("This test requires postgres")
	}

	templateDeleted := false

	clock := quartz.NewMock(t)
	ctx := testutil.Context(t, testutil.WaitShort)
	cfg := codersdk.PrebuildsConfig{}
	logger := slogtest.Make(
		t, &slogtest.Options{IgnoreErrors: true},
	).Leveled(slog.LevelDebug)
	db, pubSub := dbtestutil.NewDB(t)
	cache := files.New(prometheus.NewRegistry(), &coderdtest.FakeAuthorizer{})
	controller := prebuilds.NewStoreReconciler(db, pubSub, cache, cfg, logger, quartz.NewMock(t), prometheus.NewRegistry(), newNoopEnqueuer())

	ownerID := uuid.New()
	dbgen.User(t, db, database.User{
		ID: ownerID,
	})
	org, template := setupTestDBTemplate(t, db, ownerID, templateDeleted)
	templateVersionID := setupTestDBTemplateVersion(ctx, t, clock, db, pubSub, org.ID, ownerID, template.ID)
	preset := setupTestDBPreset(t, db, templateVersionID, 1, uuid.New().String())
	prebuiltWorkspace, _ := setupTestDBPrebuild(
		t,
		clock,
		db,
		pubSub,
		database.WorkspaceTransitionStart,
		database.ProvisionerJobStatusSucceeded,
		org.ID,
		preset,
		template.ID,
		templateVersionID,
	)

	workspaces, err := db.GetWorkspacesByTemplateID(ctx, template.ID)
	require.NoError(t, err)
	// make sure we have only one workspace
	require.Equal(t, 1, len(workspaces))

	// Create a new template version and mark it as active.
	// This marks the previous template version as inactive.
	templateVersionID = setupTestDBTemplateVersion(ctx, t, clock, db, pubSub, org.ID, ownerID, template.ID)
	// Add required param, which is not set in preset.
	// It means that creating of new prebuilt workspace will fail, but we should be able to clean up old prebuilt workspaces.
	dbgen.TemplateVersionParameter(t, db, database.TemplateVersionParameter{
		TemplateVersionID: templateVersionID,
		Name:              "required-param",
		Description:       "required param which isn't set in preset",
		Type:              "bool",
		DefaultValue:      "",
		Required:          true,
	})

	// Old prebuilt workspace should be deleted.
	require.NoError(t, controller.ReconcileAll(ctx))

	builds, err := db.GetWorkspaceBuildsByWorkspaceID(ctx, database.GetWorkspaceBuildsByWorkspaceIDParams{
		WorkspaceID: prebuiltWorkspace.ID,
	})
	require.NoError(t, err)
	// Make sure old prebuild workspace was deleted, despite it contains required parameter which isn't set in preset.
	require.Equal(t, 2, len(builds))
	require.Equal(t, database.WorkspaceTransitionDelete, builds[0].Transition)
}

func TestSkippingHardLimitedPresets(t *testing.T) {
	t.Parallel()

	if !dbtestutil.WillUsePostgres() {
		t.Skip("This test requires postgres")
	}

	// Test cases verify the behavior of prebuild creation depending on configured failure limits.
	testCases := []struct {
		name           string
		hardLimit      int64
		isHardLimitHit bool
	}{
		{
			name:           "hard limit is hit - skip creation of prebuilt workspace",
			hardLimit:      1,
			isHardLimitHit: true,
		},
		{
			name:           "hard limit is not hit - try to create prebuilt workspace again",
			hardLimit:      2,
			isHardLimitHit: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			templateDeleted := false

			clock := quartz.NewMock(t)
			ctx := testutil.Context(t, testutil.WaitShort)
			cfg := codersdk.PrebuildsConfig{
				FailureHardLimit:              serpent.Int64(tc.hardLimit),
				ReconciliationBackoffInterval: 0,
			}
			logger := slogtest.Make(
				t, &slogtest.Options{IgnoreErrors: true},
			).Leveled(slog.LevelDebug)
			db, pubSub := dbtestutil.NewDB(t)
			fakeEnqueuer := newFakeEnqueuer()
			registry := prometheus.NewRegistry()
			cache := files.New(prometheus.NewRegistry(), &coderdtest.FakeAuthorizer{})
			controller := prebuilds.NewStoreReconciler(db, pubSub, cache, cfg, logger, clock, registry, fakeEnqueuer)

			// Set up test environment with a template, version, and preset.
			ownerID := uuid.New()
			dbgen.User(t, db, database.User{
				ID: ownerID,
			})
			org, template := setupTestDBTemplate(t, db, ownerID, templateDeleted)
			templateVersionID := setupTestDBTemplateVersion(ctx, t, clock, db, pubSub, org.ID, ownerID, template.ID)
			preset := setupTestDBPreset(t, db, templateVersionID, 1, uuid.New().String())

			// Create a failed prebuild workspace that counts toward the hard failure limit.
			setupTestDBPrebuild(
				t,
				clock,
				db,
				pubSub,
				database.WorkspaceTransitionStart,
				database.ProvisionerJobStatusFailed,
				org.ID,
				preset,
				template.ID,
				templateVersionID,
			)

			// Verify initial state: one failed workspace exists.
			workspaces, err := db.GetWorkspacesByTemplateID(ctx, template.ID)
			require.NoError(t, err)
			workspaceCount := len(workspaces)
			require.Equal(t, 1, workspaceCount)

			// Verify initial state: metric is not set - meaning preset is not hard limited.
			require.NoError(t, controller.ForceMetricsUpdate(ctx))
			mf, err := registry.Gather()
			require.NoError(t, err)
			metric := findMetric(mf, prebuilds.MetricPresetHardLimitedGauge, map[string]string{
				"template_name": template.Name,
				"preset_name":   preset.Name,
				"org_name":      org.Name,
			})
			require.Nil(t, metric)

			// We simulate a failed prebuild in the test; Consequently, the backoff mechanism is triggered when ReconcileAll is called.
			// Even though ReconciliationBackoffInterval is set to zero, we still need to advance the clock by at least one nanosecond.
			clock.Advance(time.Nanosecond).MustWait(ctx)

			// Trigger reconciliation to attempt creating a new prebuild.
			// The outcome depends on whether the hard limit has been reached.
			require.NoError(t, controller.ReconcileAll(ctx))

			// These two additional calls to ReconcileAll should not trigger any notifications.
			// A notification is only sent once.
			require.NoError(t, controller.ReconcileAll(ctx))
			require.NoError(t, controller.ReconcileAll(ctx))

			// Verify the final state after reconciliation.
			workspaces, err = db.GetWorkspacesByTemplateID(ctx, template.ID)
			require.NoError(t, err)
			updatedPreset, err := db.GetPresetByID(ctx, preset.ID)
			require.NoError(t, err)

			if !tc.isHardLimitHit {
				// When hard limit is not reached, a new workspace should be created.
				require.Equal(t, 2, len(workspaces))
				require.Equal(t, database.PrebuildStatusHealthy, updatedPreset.PrebuildStatus)

				// When hard limit is not reached, metric is not set.
				mf, err = registry.Gather()
				require.NoError(t, err)
				metric = findMetric(mf, prebuilds.MetricPresetHardLimitedGauge, map[string]string{
					"template_name": template.Name,
					"preset_name":   preset.Name,
					"org_name":      org.Name,
				})
				require.Nil(t, metric)
				return
			}

			// When hard limit is reached, no new workspace should be created.
			require.Equal(t, 1, len(workspaces))
			require.Equal(t, database.PrebuildStatusHardLimited, updatedPreset.PrebuildStatus)

			// When hard limit is reached, metric is set to 1.
			mf, err = registry.Gather()
			require.NoError(t, err)
			metric = findMetric(mf, prebuilds.MetricPresetHardLimitedGauge, map[string]string{
				"template_name": template.Name,
				"preset_name":   preset.Name,
				"org_name":      org.Name,
			})
			require.NotNil(t, metric)
			require.NotNil(t, metric.GetGauge())
			require.EqualValues(t, 1, metric.GetGauge().GetValue())
		})
	}
}

func TestHardLimitedPresetShouldNotBlockDeletion(t *testing.T) {
	t.Parallel()

	if !dbtestutil.WillUsePostgres() {
		t.Skip("This test requires postgres")
	}

	// Test cases verify the behavior of prebuild creation depending on configured failure limits.
	testCases := []struct {
		name                     string
		hardLimit                int64
		createNewTemplateVersion bool
		deleteTemplate           bool
	}{
		{
			// hard limit is hit - but we allow deletion of prebuilt workspace because it's outdated (new template version was created)
			name:                     "new template version is created",
			hardLimit:                1,
			createNewTemplateVersion: true,
			deleteTemplate:           false,
		},
		{
			// hard limit is hit - but we allow deletion of prebuilt workspace because template is deleted
			name:                     "template is deleted",
			hardLimit:                1,
			createNewTemplateVersion: false,
			deleteTemplate:           true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			clock := quartz.NewMock(t)
			ctx := testutil.Context(t, testutil.WaitShort)
			cfg := codersdk.PrebuildsConfig{
				FailureHardLimit:              serpent.Int64(tc.hardLimit),
				ReconciliationBackoffInterval: 0,
			}
			logger := slogtest.Make(
				t, &slogtest.Options{IgnoreErrors: true},
			).Leveled(slog.LevelDebug)
			db, pubSub := dbtestutil.NewDB(t)
			fakeEnqueuer := newFakeEnqueuer()
			registry := prometheus.NewRegistry()
			cache := files.New(prometheus.NewRegistry(), &coderdtest.FakeAuthorizer{})
			controller := prebuilds.NewStoreReconciler(db, pubSub, cache, cfg, logger, clock, registry, fakeEnqueuer)

			// Set up test environment with a template, version, and preset.
			ownerID := uuid.New()
			dbgen.User(t, db, database.User{
				ID: ownerID,
			})
			org, template := setupTestDBTemplate(t, db, ownerID, false)
			templateVersionID := setupTestDBTemplateVersion(ctx, t, clock, db, pubSub, org.ID, ownerID, template.ID)
			preset := setupTestDBPreset(t, db, templateVersionID, 2, uuid.New().String())

			// Create a successful prebuilt workspace.
			successfulWorkspace, _ := setupTestDBPrebuild(
				t,
				clock,
				db,
				pubSub,
				database.WorkspaceTransitionStart,
				database.ProvisionerJobStatusSucceeded,
				org.ID,
				preset,
				template.ID,
				templateVersionID,
			)

			// Make sure that prebuilt workspaces created in such order: [successful, failed].
			clock.Advance(time.Second).MustWait(ctx)

			// Create a failed prebuilt workspace that counts toward the hard failure limit.
			setupTestDBPrebuild(
				t,
				clock,
				db,
				pubSub,
				database.WorkspaceTransitionStart,
				database.ProvisionerJobStatusFailed,
				org.ID,
				preset,
				template.ID,
				templateVersionID,
			)

			getJobStatusMap := func(workspaces []database.WorkspaceTable) map[database.ProvisionerJobStatus]int {
				jobStatusMap := make(map[database.ProvisionerJobStatus]int)
				for _, workspace := range workspaces {
					workspaceBuilds, err := db.GetWorkspaceBuildsByWorkspaceID(ctx, database.GetWorkspaceBuildsByWorkspaceIDParams{
						WorkspaceID: workspace.ID,
					})
					require.NoError(t, err)

					for _, workspaceBuild := range workspaceBuilds {
						job, err := db.GetProvisionerJobByID(ctx, workspaceBuild.JobID)
						require.NoError(t, err)
						jobStatusMap[job.JobStatus]++
					}
				}
				return jobStatusMap
			}

			// Verify initial state: two workspaces exist, one successful, one failed.
			workspaces, err := db.GetWorkspacesByTemplateID(ctx, template.ID)
			require.NoError(t, err)
			require.Equal(t, 2, len(workspaces))
			jobStatusMap := getJobStatusMap(workspaces)
			require.Len(t, jobStatusMap, 2)
			require.Equal(t, 1, jobStatusMap[database.ProvisionerJobStatusSucceeded])
			require.Equal(t, 1, jobStatusMap[database.ProvisionerJobStatusFailed])

			// Verify initial state: metric is not set - meaning preset is not hard limited.
			require.NoError(t, controller.ForceMetricsUpdate(ctx))
			mf, err := registry.Gather()
			require.NoError(t, err)
			metric := findMetric(mf, prebuilds.MetricPresetHardLimitedGauge, map[string]string{
				"template_name": template.Name,
				"preset_name":   preset.Name,
				"org_name":      org.Name,
			})
			require.Nil(t, metric)

			// We simulate a failed prebuild in the test; Consequently, the backoff mechanism is triggered when ReconcileAll is called.
			// Even though ReconciliationBackoffInterval is set to zero, we still need to advance the clock by at least one nanosecond.
			clock.Advance(time.Nanosecond).MustWait(ctx)

			// Trigger reconciliation to attempt creating a new prebuild.
			// The outcome depends on whether the hard limit has been reached.
			require.NoError(t, controller.ReconcileAll(ctx))

			// These two additional calls to ReconcileAll should not trigger any notifications.
			// A notification is only sent once.
			require.NoError(t, controller.ReconcileAll(ctx))
			require.NoError(t, controller.ReconcileAll(ctx))

			// Verify the final state after reconciliation.
			// When hard limit is reached, no new workspace should be created.
			workspaces, err = db.GetWorkspacesByTemplateID(ctx, template.ID)
			require.NoError(t, err)
			require.Equal(t, 2, len(workspaces))
			jobStatusMap = getJobStatusMap(workspaces)
			require.Len(t, jobStatusMap, 2)
			require.Equal(t, 1, jobStatusMap[database.ProvisionerJobStatusSucceeded])
			require.Equal(t, 1, jobStatusMap[database.ProvisionerJobStatusFailed])

			updatedPreset, err := db.GetPresetByID(ctx, preset.ID)
			require.NoError(t, err)
			require.Equal(t, database.PrebuildStatusHardLimited, updatedPreset.PrebuildStatus)

			// When hard limit is reached, metric is set to 1.
			mf, err = registry.Gather()
			require.NoError(t, err)
			metric = findMetric(mf, prebuilds.MetricPresetHardLimitedGauge, map[string]string{
				"template_name": template.Name,
				"preset_name":   preset.Name,
				"org_name":      org.Name,
			})
			require.NotNil(t, metric)
			require.NotNil(t, metric.GetGauge())
			require.EqualValues(t, 1, metric.GetGauge().GetValue())

			if tc.createNewTemplateVersion {
				// Create a new template version and mark it as active
				// This marks the template version that we care about as inactive
				setupTestDBTemplateVersion(ctx, t, clock, db, pubSub, org.ID, ownerID, template.ID)
			}

			if tc.deleteTemplate {
				require.NoError(t, db.UpdateTemplateDeletedByID(ctx, database.UpdateTemplateDeletedByIDParams{
					ID:        template.ID,
					Deleted:   true,
					UpdatedAt: dbtime.Now(),
				}))
			}

			// Trigger reconciliation to make sure that successful, but outdated prebuilt workspace will be deleted.
			require.NoError(t, controller.ReconcileAll(ctx))

			workspaces, err = db.GetWorkspacesByTemplateID(ctx, template.ID)
			require.NoError(t, err)
			require.Equal(t, 2, len(workspaces))

			jobStatusMap = getJobStatusMap(workspaces)
			require.Len(t, jobStatusMap, 3)
			require.Equal(t, 1, jobStatusMap[database.ProvisionerJobStatusSucceeded])
			require.Equal(t, 1, jobStatusMap[database.ProvisionerJobStatusFailed])
			// Pending job should be the job that deletes successful, but outdated prebuilt workspace.
			// Prebuilt workspace MUST be deleted, despite the fact that preset is marked as hard limited.
			require.Equal(t, 1, jobStatusMap[database.ProvisionerJobStatusPending])

			workspaceBuilds, err := db.GetWorkspaceBuildsByWorkspaceID(ctx, database.GetWorkspaceBuildsByWorkspaceIDParams{
				WorkspaceID: successfulWorkspace.ID,
			})
			require.NoError(t, err)
			require.Equal(t, 2, len(workspaceBuilds))
			// Make sure that successfully created, but outdated prebuilt workspace was scheduled for deletion.
			require.Equal(t, database.WorkspaceTransitionDelete, workspaceBuilds[0].Transition)
			require.Equal(t, database.WorkspaceTransitionStart, workspaceBuilds[1].Transition)

			// Metric is deleted after preset became outdated.
			mf, err = registry.Gather()
			require.NoError(t, err)
			metric = findMetric(mf, prebuilds.MetricPresetHardLimitedGauge, map[string]string{
				"template_name": template.Name,
				"preset_name":   preset.Name,
				"org_name":      org.Name,
			})
			require.Nil(t, metric)
		})
	}
}

func TestRunLoop(t *testing.T) {
	t.Parallel()

	if !dbtestutil.WillUsePostgres() {
		t.Skip("This test requires postgres")
	}

	prebuildLatestTransition := database.WorkspaceTransitionStart
	prebuildJobStatus := database.ProvisionerJobStatusRunning
	templateDeleted := false

	clock := quartz.NewMock(t)
	ctx := testutil.Context(t, testutil.WaitShort)
	backoffInterval := time.Minute
	cfg := codersdk.PrebuildsConfig{
		// Given: explicitly defined backoff configuration to validate timings.
		ReconciliationBackoffLookback: serpent.Duration(muchEarlier * -10), // Has to be positive.
		ReconciliationBackoffInterval: serpent.Duration(backoffInterval),
		ReconciliationInterval:        serpent.Duration(time.Second),
	}
	logger := slogtest.Make(
		t, &slogtest.Options{IgnoreErrors: true},
	).Leveled(slog.LevelDebug)
	db, pubSub := dbtestutil.NewDB(t)
	cache := files.New(prometheus.NewRegistry(), &coderdtest.FakeAuthorizer{})
	reconciler := prebuilds.NewStoreReconciler(db, pubSub, cache, cfg, logger, clock, prometheus.NewRegistry(), newNoopEnqueuer())

	ownerID := uuid.New()
	dbgen.User(t, db, database.User{
		ID: ownerID,
	})
	org, template := setupTestDBTemplate(t, db, ownerID, templateDeleted)
	templateVersionID := setupTestDBTemplateVersion(
		ctx,
		t,
		clock,
		db,
		pubSub,
		org.ID,
		ownerID,
		template.ID,
	)
	preset := setupTestDBPreset(
		t,
		db,
		templateVersionID,
		4,
		uuid.New().String(),
	)
	preset2 := setupTestDBPreset(
		t,
		db,
		templateVersionID,
		10,
		uuid.New().String(),
	)
	prebuildIDs := make([]uuid.UUID, 0)
	for i := 0; i < int(preset.DesiredInstances.Int32); i++ {
		prebuild, _ := setupTestDBPrebuild(
			t,
			clock,
			db,
			pubSub,
			prebuildLatestTransition,
			prebuildJobStatus,
			org.ID,
			preset,
			template.ID,
			templateVersionID,
		)
		prebuildIDs = append(prebuildIDs, prebuild.ID)
	}
	getNewPrebuildCount := func() int32 {
		newPrebuildCount := 0
		workspaces, err := db.GetWorkspacesByTemplateID(ctx, template.ID)
		require.NoError(t, err)
		for _, workspace := range workspaces {
			if slice.Contains(prebuildIDs, workspace.ID) {
				continue
			}
			newPrebuildCount++
		}

		return int32(newPrebuildCount) // nolint:gosec
	}

	// we need to wait until ticker is initialized, and only then use clock.Advance()
	// otherwise clock.Advance() will be ignored
	trap := clock.Trap().NewTicker()
	go reconciler.Run(ctx)
	// wait until ticker is initialized
	trap.MustWait(ctx).MustRelease(ctx)
	// start 1st iteration of ReconciliationLoop
	// NOTE: at this point MustWait waits that iteration is started (ReconcileAll is called), but it doesn't wait until it completes
	clock.Advance(cfg.ReconciliationInterval.Value()).MustWait(ctx)

	// wait until ReconcileAll is completed
	// TODO: is it possible to avoid Eventually and replace it with quartz?
	// Ideally to have all control on test-level, and be able to advance loop iterations from the test.
	require.Eventually(t, func() bool {
		newPrebuildCount := getNewPrebuildCount()

		// NOTE: preset1 doesn't block creation of instances in preset2
		return preset2.DesiredInstances.Int32 == newPrebuildCount
	}, testutil.WaitShort, testutil.IntervalFast)

	// setup one more preset with 5 prebuilds
	preset3 := setupTestDBPreset(
		t,
		db,
		templateVersionID,
		5,
		uuid.New().String(),
	)
	newPrebuildCount := getNewPrebuildCount()
	// nothing changed, because we didn't trigger a new iteration of a loop
	require.Equal(t, preset2.DesiredInstances.Int32, newPrebuildCount)

	// start 2nd iteration of ReconciliationLoop
	// NOTE: at this point MustWait waits that iteration is started (ReconcileAll is called), but it doesn't wait until it completes
	clock.Advance(cfg.ReconciliationInterval.Value()).MustWait(ctx)

	// wait until ReconcileAll is completed
	require.Eventually(t, func() bool {
		newPrebuildCount := getNewPrebuildCount()

		// both prebuilds for preset2 and preset3 were created
		return preset2.DesiredInstances.Int32+preset3.DesiredInstances.Int32 == newPrebuildCount
	}, testutil.WaitShort, testutil.IntervalFast)

	// gracefully stop the reconciliation loop
	reconciler.Stop(ctx, nil)
}

func TestFailedBuildBackoff(t *testing.T) {
	t.Parallel()

	if !dbtestutil.WillUsePostgres() {
		t.Skip("This test requires postgres")
	}
	ctx := testutil.Context(t, testutil.WaitSuperLong)

	// Setup.
	clock := quartz.NewMock(t)
	backoffInterval := time.Minute
	cfg := codersdk.PrebuildsConfig{
		// Given: explicitly defined backoff configuration to validate timings.
		ReconciliationBackoffLookback: serpent.Duration(muchEarlier * -10), // Has to be positive.
		ReconciliationBackoffInterval: serpent.Duration(backoffInterval),
		ReconciliationInterval:        serpent.Duration(time.Second),
	}
	logger := slogtest.Make(
		t, &slogtest.Options{IgnoreErrors: true},
	).Leveled(slog.LevelDebug)
	db, ps := dbtestutil.NewDB(t)
	cache := files.New(prometheus.NewRegistry(), &coderdtest.FakeAuthorizer{})
	reconciler := prebuilds.NewStoreReconciler(db, ps, cache, cfg, logger, clock, prometheus.NewRegistry(), newNoopEnqueuer())

	// Given: an active template version with presets and prebuilds configured.
	const desiredInstances = 2
	userID := uuid.New()
	dbgen.User(t, db, database.User{
		ID: userID,
	})
	org, template := setupTestDBTemplate(t, db, userID, false)
	templateVersionID := setupTestDBTemplateVersion(ctx, t, clock, db, ps, org.ID, userID, template.ID)

	preset := setupTestDBPreset(t, db, templateVersionID, desiredInstances, "test")
	for range desiredInstances {
		_, _ = setupTestDBPrebuild(t, clock, db, ps, database.WorkspaceTransitionStart, database.ProvisionerJobStatusFailed, org.ID, preset, template.ID, templateVersionID)
	}

	// When: determining what actions to take next, backoff is calculated because the prebuild is in a failed state.
	snapshot, err := reconciler.SnapshotState(ctx, db)
	require.NoError(t, err)
	require.Len(t, snapshot.Presets, 1)
	presetState, err := snapshot.FilterByPreset(preset.ID)
	require.NoError(t, err)
	state := presetState.CalculateState()
	actions, err := reconciler.CalculateActions(ctx, *presetState)
	require.NoError(t, err)
	require.Equal(t, 1, len(actions))

	// Then: the backoff time is in the future, no prebuilds are running, and we won't create any new prebuilds.
	require.EqualValues(t, 0, state.Actual)
	require.EqualValues(t, 0, actions[0].Create)
	require.EqualValues(t, desiredInstances, state.Desired)
	require.True(t, clock.Now().Before(actions[0].BackoffUntil))

	// Then: the backoff time is as expected based on the number of failed builds.
	require.NotNil(t, presetState.Backoff)
	require.EqualValues(t, desiredInstances, presetState.Backoff.NumFailed)
	require.EqualValues(t, backoffInterval*time.Duration(presetState.Backoff.NumFailed), clock.Until(actions[0].BackoffUntil).Truncate(backoffInterval))

	// When: advancing to the next tick which is still within the backoff time.
	clock.Advance(cfg.ReconciliationInterval.Value())

	// Then: the backoff interval will not have changed.
	snapshot, err = reconciler.SnapshotState(ctx, db)
	require.NoError(t, err)
	presetState, err = snapshot.FilterByPreset(preset.ID)
	require.NoError(t, err)
	newState := presetState.CalculateState()
	newActions, err := reconciler.CalculateActions(ctx, *presetState)
	require.NoError(t, err)
	require.Equal(t, 1, len(newActions))

	require.EqualValues(t, 0, newState.Actual)
	require.EqualValues(t, 0, newActions[0].Create)
	require.EqualValues(t, desiredInstances, newState.Desired)
	require.EqualValues(t, actions[0].BackoffUntil, newActions[0].BackoffUntil)

	// When: advancing beyond the backoff time.
	clock.Advance(clock.Until(actions[0].BackoffUntil.Add(time.Second)))

	// Then: we will attempt to create a new prebuild.
	snapshot, err = reconciler.SnapshotState(ctx, db)
	require.NoError(t, err)
	presetState, err = snapshot.FilterByPreset(preset.ID)
	require.NoError(t, err)
	state = presetState.CalculateState()
	actions, err = reconciler.CalculateActions(ctx, *presetState)
	require.NoError(t, err)
	require.Equal(t, 1, len(actions))

	require.EqualValues(t, 0, state.Actual)
	require.EqualValues(t, desiredInstances, state.Desired)
	require.EqualValues(t, desiredInstances, actions[0].Create)

	// When: the desired number of new prebuild are provisioned, but one fails again.
	for i := 0; i < desiredInstances; i++ {
		status := database.ProvisionerJobStatusFailed
		if i == 1 {
			status = database.ProvisionerJobStatusSucceeded
		}
		_, _ = setupTestDBPrebuild(t, clock, db, ps, database.WorkspaceTransitionStart, status, org.ID, preset, template.ID, templateVersionID)
	}

	// Then: the backoff time is roughly equal to two backoff intervals, since another build has failed.
	snapshot, err = reconciler.SnapshotState(ctx, db)
	require.NoError(t, err)
	presetState, err = snapshot.FilterByPreset(preset.ID)
	require.NoError(t, err)
	state = presetState.CalculateState()
	actions, err = reconciler.CalculateActions(ctx, *presetState)
	require.NoError(t, err)
	require.Equal(t, 1, len(actions))

	require.EqualValues(t, 1, state.Actual)
	require.EqualValues(t, desiredInstances, state.Desired)
	require.EqualValues(t, 0, actions[0].Create)
	require.EqualValues(t, 3, presetState.Backoff.NumFailed)
	require.EqualValues(t, backoffInterval*time.Duration(presetState.Backoff.NumFailed), clock.Until(actions[0].BackoffUntil).Truncate(backoffInterval))
}

func TestReconciliationLock(t *testing.T) {
	t.Parallel()

	if !dbtestutil.WillUsePostgres() {
		t.Skip("This test requires postgres")
	}

	ctx := testutil.Context(t, testutil.WaitSuperLong)
	logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true}).Leveled(slog.LevelDebug)
	db, ps := dbtestutil.NewDB(t)

	wg := sync.WaitGroup{}
	mutex := sync.Mutex{}
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			cache := files.New(prometheus.NewRegistry(), &coderdtest.FakeAuthorizer{})
			reconciler := prebuilds.NewStoreReconciler(
				db,
				ps,
				cache,
				codersdk.PrebuildsConfig{},
				slogtest.Make(t, &slogtest.Options{IgnoreErrors: true}).Leveled(slog.LevelDebug),
				quartz.NewMock(t),
				prometheus.NewRegistry(),
				newNoopEnqueuer())
			reconciler.WithReconciliationLock(ctx, logger, func(_ context.Context, _ database.Store) error {
				lockObtained := mutex.TryLock()
				// As long as the postgres lock is held, this mutex should always be unlocked when we get here.
				// If this mutex is ever locked at this point, then that means that the postgres lock is not being held while we're
				// inside WithReconciliationLock, which is meant to hold the lock.
				require.True(t, lockObtained)
				// Sleep a bit to give reconcilers more time to contend for the lock
				time.Sleep(time.Second)
				defer mutex.Unlock()
				return nil
			})
		}()
	}
	wg.Wait()
}

func TestTrackResourceReplacement(t *testing.T) {
	t.Parallel()

	if !dbtestutil.WillUsePostgres() {
		t.Skip("This test requires postgres")
	}

	ctx := testutil.Context(t, testutil.WaitSuperLong)

	// Setup.
	clock := quartz.NewMock(t)
	logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: false}).Leveled(slog.LevelDebug)
	db, ps := dbtestutil.NewDB(t)

	fakeEnqueuer := newFakeEnqueuer()
	registry := prometheus.NewRegistry()
	cache := files.New(registry, &coderdtest.FakeAuthorizer{})
	reconciler := prebuilds.NewStoreReconciler(db, ps, cache, codersdk.PrebuildsConfig{}, logger, clock, registry, fakeEnqueuer)

	// Given: a template admin to receive a notification.
	templateAdmin := dbgen.User(t, db, database.User{
		RBACRoles: []string{codersdk.RoleTemplateAdmin},
	})

	// Given: a prebuilt workspace.
	userID := uuid.New()
	dbgen.User(t, db, database.User{ID: userID})
	org, template := setupTestDBTemplate(t, db, userID, false)
	templateVersionID := setupTestDBTemplateVersion(ctx, t, clock, db, ps, org.ID, userID, template.ID)
	preset := setupTestDBPreset(t, db, templateVersionID, 1, "b0rked")
	prebuiltWorkspace, prebuild := setupTestDBPrebuild(t, clock, db, ps, database.WorkspaceTransitionStart, database.ProvisionerJobStatusSucceeded, org.ID, preset, template.ID, templateVersionID)

	// Given: no replacement has been tracked yet, we should not see a metric for it yet.
	require.NoError(t, reconciler.ForceMetricsUpdate(ctx))
	mf, err := registry.Gather()
	require.NoError(t, err)
	require.Nil(t, findMetric(mf, prebuilds.MetricResourceReplacementsCount, map[string]string{
		"template_name": template.Name,
		"preset_name":   preset.Name,
		"org_name":      org.Name,
	}))

	// When: a claim occurred and resource replacements are detected (_how_ is out of scope of this test).
	reconciler.TrackResourceReplacement(ctx, prebuiltWorkspace.ID, prebuild.ID, []*sdkproto.ResourceReplacement{
		{
			Resource: "docker_container[0]",
			Paths:    []string{"env", "image"},
		},
		{
			Resource: "docker_volume[0]",
			Paths:    []string{"name"},
		},
	})

	// Then: a notification will be sent detailing the replacement(s).
	matching := fakeEnqueuer.Sent(func(notification *notificationstest.FakeNotification) bool {
		// This is not an exhaustive check of the expected labels/data in the notification. This would tie the implementations
		// too tightly together.
		// All we need to validate is that a template of the right kind was sent, to the expected user, with some replacements.

		if !assert.Equal(t, notification.TemplateID, notifications.TemplateWorkspaceResourceReplaced, "unexpected template") {
			return false
		}

		if !assert.Equal(t, templateAdmin.ID, notification.UserID, "unexpected receiver") {
			return false
		}

		if !assert.Len(t, notification.Data["replacements"], 2, "unexpected replacements count") {
			return false
		}

		return true
	})
	require.Len(t, matching, 1)

	// Then: the metric will be incremented.
	mf, err = registry.Gather()
	require.NoError(t, err)
	metric := findMetric(mf, prebuilds.MetricResourceReplacementsCount, map[string]string{
		"template_name": template.Name,
		"preset_name":   preset.Name,
		"org_name":      org.Name,
	})
	require.NotNil(t, metric)
	require.NotNil(t, metric.GetCounter())
	require.EqualValues(t, 1, metric.GetCounter().GetValue())
}

func TestExpiredPrebuildsMultipleActions(t *testing.T) {
	t.Parallel()

	if !dbtestutil.WillUsePostgres() {
		t.Skip("This test requires postgres")
	}

	testCases := []struct {
		name       string
		running    int
		desired    int32
		expired    int
		extraneous int
		created    int
	}{
		// With 2 running prebuilds, none of which are expired, and the desired count is met,
		// no deletions or creations should occur.
		{
			name:       "no expired prebuilds - no actions taken",
			running:    2,
			desired:    2,
			expired:    0,
			extraneous: 0,
			created:    0,
		},
		// With 2 running prebuilds, 1 of which is expired, the expired prebuild should be deleted,
		// and one new prebuild should be created to maintain the desired count.
		{
			name:       "one expired prebuild – deleted and replaced",
			running:    2,
			desired:    2,
			expired:    1,
			extraneous: 0,
			created:    1,
		},
		// With 2 running prebuilds, both expired, both should be deleted,
		// and 2 new prebuilds created to match the desired count.
		{
			name:       "all prebuilds expired – all deleted and recreated",
			running:    2,
			desired:    2,
			expired:    2,
			extraneous: 0,
			created:    2,
		},
		// With 4 running prebuilds, 2 of which are expired, and the desired count is 2,
		// the expired prebuilds should be deleted. No new creations are needed
		// since removing the expired ones brings actual = desired.
		{
			name:       "expired prebuilds deleted to reach desired count",
			running:    4,
			desired:    2,
			expired:    2,
			extraneous: 0,
			created:    0,
		},
		// With 4 running prebuilds (1 expired), and the desired count is 2,
		// the first action should delete the expired one,
		// and the second action should delete one additional (non-expired) prebuild
		// to eliminate the remaining excess.
		{
			name:       "expired prebuild deleted first, then extraneous",
			running:    4,
			desired:    2,
			expired:    1,
			extraneous: 1,
			created:    0,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			clock := quartz.NewMock(t)
			ctx := testutil.Context(t, testutil.WaitLong)
			cfg := codersdk.PrebuildsConfig{}
			logger := slogtest.Make(
				t, &slogtest.Options{IgnoreErrors: true},
			).Leveled(slog.LevelDebug)
			db, pubSub := dbtestutil.NewDB(t)
			fakeEnqueuer := newFakeEnqueuer()
			registry := prometheus.NewRegistry()
			cache := files.New(registry, &coderdtest.FakeAuthorizer{})
			controller := prebuilds.NewStoreReconciler(db, pubSub, cache, cfg, logger, clock, registry, fakeEnqueuer)

			// Set up test environment with a template, version, and preset
			ownerID := uuid.New()
			dbgen.User(t, db, database.User{
				ID: ownerID,
			})
			org, template := setupTestDBTemplate(t, db, ownerID, false)
			templateVersionID := setupTestDBTemplateVersion(ctx, t, clock, db, pubSub, org.ID, ownerID, template.ID)

			ttlDuration := muchEarlier - time.Hour
			ttl := int32(-ttlDuration.Seconds())
			preset := setupTestDBPreset(t, db, templateVersionID, tc.desired, "b0rked", withTTL(ttl))

			// The implementation uses time.Since(prebuild.CreatedAt) > ttl to check a prebuild expiration.
			// Since our mock clock defaults to a fixed time, we must align it with the current time
			// to ensure time-based logic works correctly in tests.
			clock.Set(time.Now())

			runningWorkspaces := make(map[string]database.WorkspaceTable)
			nonExpiredWorkspaces := make([]database.WorkspaceTable, 0, tc.running-tc.expired)
			expiredWorkspaces := make([]database.WorkspaceTable, 0, tc.expired)
			expiredCount := 0
			for r := range tc.running {
				// Space out createdAt timestamps by 1 second to ensure deterministic ordering.
				// This lets the test verify that the correct (oldest) extraneous prebuilds are deleted.
				createdAt := muchEarlier + time.Duration(r)*time.Second
				isExpired := false
				if tc.expired > expiredCount {
					// Set createdAt far enough in the past so that time.Since(createdAt) > TTL,
					// ensuring the prebuild is treated as expired in the test.
					createdAt = ttlDuration - 1*time.Minute
					isExpired = true
					expiredCount++
				}

				workspace, _ := setupTestDBPrebuild(
					t,
					clock,
					db,
					pubSub,
					database.WorkspaceTransitionStart,
					database.ProvisionerJobStatusSucceeded,
					org.ID,
					preset,
					template.ID,
					templateVersionID,
					withCreatedAt(clock.Now().Add(createdAt)),
				)
				if isExpired {
					expiredWorkspaces = append(expiredWorkspaces, workspace)
				} else {
					nonExpiredWorkspaces = append(nonExpiredWorkspaces, workspace)
				}
				runningWorkspaces[workspace.ID.String()] = workspace
			}

			getJobStatusMap := func(workspaces []database.WorkspaceTable) map[database.ProvisionerJobStatus]int {
				jobStatusMap := make(map[database.ProvisionerJobStatus]int)
				for _, workspace := range workspaces {
					workspaceBuilds, err := db.GetWorkspaceBuildsByWorkspaceID(ctx, database.GetWorkspaceBuildsByWorkspaceIDParams{
						WorkspaceID: workspace.ID,
					})
					require.NoError(t, err)

					for _, workspaceBuild := range workspaceBuilds {
						job, err := db.GetProvisionerJobByID(ctx, workspaceBuild.JobID)
						require.NoError(t, err)
						jobStatusMap[job.JobStatus]++
					}
				}
				return jobStatusMap
			}

			// Assert that the build associated with the given workspace has a 'start' transition status.
			isWorkspaceStarted := func(workspace database.WorkspaceTable) {
				workspaceBuilds, err := db.GetWorkspaceBuildsByWorkspaceID(ctx, database.GetWorkspaceBuildsByWorkspaceIDParams{
					WorkspaceID: workspace.ID,
				})
				require.NoError(t, err)
				require.Equal(t, 1, len(workspaceBuilds))
				require.Equal(t, database.WorkspaceTransitionStart, workspaceBuilds[0].Transition)
			}

			// Assert that the workspace build history includes a 'start' followed by a 'delete' transition status.
			isWorkspaceDeleted := func(workspace database.WorkspaceTable) {
				workspaceBuilds, err := db.GetWorkspaceBuildsByWorkspaceID(ctx, database.GetWorkspaceBuildsByWorkspaceIDParams{
					WorkspaceID: workspace.ID,
				})
				require.NoError(t, err)
				require.Equal(t, 2, len(workspaceBuilds))
				require.Equal(t, database.WorkspaceTransitionDelete, workspaceBuilds[0].Transition)
				require.Equal(t, database.WorkspaceTransitionStart, workspaceBuilds[1].Transition)
			}

			// Verify that all running workspaces, whether expired or not, have successfully started.
			workspaces, err := db.GetWorkspacesByTemplateID(ctx, template.ID)
			require.NoError(t, err)
			require.Equal(t, tc.running, len(workspaces))
			jobStatusMap := getJobStatusMap(workspaces)
			require.Len(t, workspaces, tc.running)
			require.Len(t, jobStatusMap, 1)
			require.Equal(t, tc.running, jobStatusMap[database.ProvisionerJobStatusSucceeded])

			// Assert that all running workspaces (expired and non-expired) have a 'start' transition state.
			for _, workspace := range runningWorkspaces {
				isWorkspaceStarted(workspace)
			}

			// Trigger reconciliation to process expired prebuilds and enforce desired state.
			require.NoError(t, controller.ReconcileAll(ctx))

			// Sort non-expired workspaces by CreatedAt in ascending order (oldest first)
			sort.Slice(nonExpiredWorkspaces, func(i, j int) bool {
				return nonExpiredWorkspaces[i].CreatedAt.Before(nonExpiredWorkspaces[j].CreatedAt)
			})

			// Verify the status of each non-expired workspace:
			// - the oldest `tc.extraneous` should have been deleted (i.e., have a 'delete' transition),
			// - while the remaining newer ones should still be running (i.e., have a 'start' transition).
			extraneousCount := 0
			for _, running := range nonExpiredWorkspaces {
				if extraneousCount < tc.extraneous {
					isWorkspaceDeleted(running)
					extraneousCount++
				} else {
					isWorkspaceStarted(running)
				}
			}
			require.Equal(t, tc.extraneous, extraneousCount)

			// Verify that each expired workspace has a 'delete' transition recorded,
			// confirming it was properly marked for cleanup after reconciliation.
			for _, expired := range expiredWorkspaces {
				isWorkspaceDeleted(expired)
			}

			// After handling expired prebuilds, if running < desired, new prebuilds should be created.
			// Verify that the correct number of new prebuild workspaces were created and started.
			allWorkspaces, err := db.GetWorkspacesByTemplateID(ctx, template.ID)
			require.NoError(t, err)

			createdCount := 0
			for _, workspace := range allWorkspaces {
				if _, ok := runningWorkspaces[workspace.ID.String()]; !ok {
					// Count and verify only the newly created workspaces (i.e., not part of the original running set)
					isWorkspaceStarted(workspace)
					createdCount++
				}
			}
			require.Equal(t, tc.created, createdCount)
		})
	}
}

func newNoopEnqueuer() *notifications.NoopEnqueuer {
	return notifications.NewNoopEnqueuer()
}

func newFakeEnqueuer() *notificationstest.FakeEnqueuer {
	return notificationstest.NewFakeEnqueuer()
}

// nolint:revive // It's a control flag, but this is a test.
func setupTestDBTemplate(
	t *testing.T,
	db database.Store,
	userID uuid.UUID,
	templateDeleted bool,
) (
	database.Organization,
	database.Template,
) {
	t.Helper()
	org := dbgen.Organization(t, db, database.Organization{})

	template := dbgen.Template(t, db, database.Template{
		CreatedBy:      userID,
		OrganizationID: org.ID,
		CreatedAt:      time.Now().Add(muchEarlier),
	})
	if templateDeleted {
		ctx := testutil.Context(t, testutil.WaitShort)
		require.NoError(t, db.UpdateTemplateDeletedByID(ctx, database.UpdateTemplateDeletedByIDParams{
			ID:      template.ID,
			Deleted: true,
		}))
	}
	return org, template
}

// nolint:revive // It's a control flag, but this is a test.
func setupTestDBTemplateWithinOrg(
	t *testing.T,
	db database.Store,
	userID uuid.UUID,
	templateDeleted bool,
	templateName string,
	org database.Organization,
) database.Template {
	t.Helper()

	template := dbgen.Template(t, db, database.Template{
		Name:           templateName,
		CreatedBy:      userID,
		OrganizationID: org.ID,
		CreatedAt:      time.Now().Add(muchEarlier),
	})
	if templateDeleted {
		ctx := testutil.Context(t, testutil.WaitShort)
		require.NoError(t, db.UpdateTemplateDeletedByID(ctx, database.UpdateTemplateDeletedByIDParams{
			ID:      template.ID,
			Deleted: true,
		}))
	}
	return template
}

const (
	earlier     = -time.Hour
	muchEarlier = -time.Hour * 2
)

func setupTestDBTemplateVersion(
	ctx context.Context,
	t *testing.T,
	clock quartz.Clock,
	db database.Store,
	ps pubsub.Pubsub,
	orgID uuid.UUID,
	userID uuid.UUID,
	templateID uuid.UUID,
) uuid.UUID {
	t.Helper()
	templateVersionJob := dbgen.ProvisionerJob(t, db, ps, database.ProvisionerJob{
		CreatedAt:      clock.Now().Add(muchEarlier),
		CompletedAt:    sql.NullTime{Time: clock.Now().Add(earlier), Valid: true},
		OrganizationID: orgID,
		InitiatorID:    userID,
	})
	templateVersion := dbgen.TemplateVersion(t, db, database.TemplateVersion{
		TemplateID:     uuid.NullUUID{UUID: templateID, Valid: true},
		OrganizationID: orgID,
		CreatedBy:      userID,
		JobID:          templateVersionJob.ID,
		CreatedAt:      time.Now().Add(muchEarlier),
	})
	require.NoError(t, db.UpdateTemplateActiveVersionByID(ctx, database.UpdateTemplateActiveVersionByIDParams{
		ID:              templateID,
		ActiveVersionID: templateVersion.ID,
	}))
	// Make sure immutable params don't break prebuilt workspace deletion logic
	dbgen.TemplateVersionParameter(t, db, database.TemplateVersionParameter{
		TemplateVersionID: templateVersion.ID,
		Name:              "test",
		Description:       "required & immutable param",
		Type:              "string",
		DefaultValue:      "",
		Required:          true,
		Mutable:           false,
	})
	return templateVersion.ID
}

// Preset optional parameters.
// presetOptions defines a function type for modifying InsertPresetParams.
type presetOptions func(*database.InsertPresetParams)

// withTTL returns a presetOptions function that sets the invalidate_after_secs (TTL) field in InsertPresetParams.
func withTTL(ttl int32) presetOptions {
	return func(p *database.InsertPresetParams) {
		p.InvalidateAfterSecs = sql.NullInt32{Valid: true, Int32: ttl}
	}
}

func setupTestDBPreset(
	t *testing.T,
	db database.Store,
	templateVersionID uuid.UUID,
	desiredInstances int32,
	presetName string,
	opts ...presetOptions,
) database.TemplateVersionPreset {
	t.Helper()
	insertPresetParams := database.InsertPresetParams{
		TemplateVersionID: templateVersionID,
		Name:              presetName,
		DesiredInstances: sql.NullInt32{
			Valid: true,
			Int32: desiredInstances,
		},
	}

	// Apply optional parameters to insertPresetParams (e.g., TTL).
	for _, opt := range opts {
		opt(&insertPresetParams)
	}

	preset := dbgen.Preset(t, db, insertPresetParams)

	dbgen.PresetParameter(t, db, database.InsertPresetParametersParams{
		TemplateVersionPresetID: preset.ID,
		Names:                   []string{"test"},
		Values:                  []string{"test"},
	})
	return preset
}

func setupTestDBPresetWithScheduling(
	t *testing.T,
	db database.Store,
	templateVersionID uuid.UUID,
	desiredInstances int32,
	presetName string,
	schedulingTimezone string,
) database.TemplateVersionPreset {
	t.Helper()
	preset := dbgen.Preset(t, db, database.InsertPresetParams{
		TemplateVersionID: templateVersionID,
		Name:              presetName,
		DesiredInstances: sql.NullInt32{
			Valid: true,
			Int32: desiredInstances,
		},
		SchedulingTimezone: schedulingTimezone,
	})
	dbgen.PresetParameter(t, db, database.InsertPresetParametersParams{
		TemplateVersionPresetID: preset.ID,
		Names:                   []string{"test"},
		Values:                  []string{"test"},
	})
	return preset
}

// prebuildOptions holds optional parameters for creating a prebuild workspace.
type prebuildOptions struct {
	createdAt *time.Time
}

// prebuildOption defines a function type to apply optional settings to prebuildOptions.
type prebuildOption func(*prebuildOptions)

// withCreatedAt returns a prebuildOption that sets the CreatedAt timestamp.
func withCreatedAt(createdAt time.Time) prebuildOption {
	return func(opts *prebuildOptions) {
		opts.createdAt = &createdAt
	}
}

func setupTestDBPrebuild(
	t *testing.T,
	clock quartz.Clock,
	db database.Store,
	ps pubsub.Pubsub,
	transition database.WorkspaceTransition,
	prebuildStatus database.ProvisionerJobStatus,
	orgID uuid.UUID,
	preset database.TemplateVersionPreset,
	templateID uuid.UUID,
	templateVersionID uuid.UUID,
	opts ...prebuildOption,
) (database.WorkspaceTable, database.WorkspaceBuild) {
	t.Helper()
	return setupTestDBWorkspace(t, clock, db, ps, transition, prebuildStatus, orgID, preset, templateID, templateVersionID, database.PrebuildsSystemUserID, database.PrebuildsSystemUserID, opts...)
}

func setupTestDBWorkspace(
	t *testing.T,
	clock quartz.Clock,
	db database.Store,
	ps pubsub.Pubsub,
	transition database.WorkspaceTransition,
	prebuildStatus database.ProvisionerJobStatus,
	orgID uuid.UUID,
	preset database.TemplateVersionPreset,
	templateID uuid.UUID,
	templateVersionID uuid.UUID,
	initiatorID uuid.UUID,
	ownerID uuid.UUID,
	opts ...prebuildOption,
) (database.WorkspaceTable, database.WorkspaceBuild) {
	t.Helper()
	cancelledAt := sql.NullTime{}
	completedAt := sql.NullTime{}

	startedAt := sql.NullTime{}
	if prebuildStatus != database.ProvisionerJobStatusPending {
		startedAt = sql.NullTime{Time: clock.Now().Add(muchEarlier), Valid: true}
	}

	buildError := sql.NullString{}
	if prebuildStatus == database.ProvisionerJobStatusFailed {
		completedAt = sql.NullTime{Time: clock.Now().Add(earlier), Valid: true}
		buildError = sql.NullString{String: "build failed", Valid: true}
	}

	switch prebuildStatus {
	case database.ProvisionerJobStatusCanceling:
		cancelledAt = sql.NullTime{Time: clock.Now().Add(earlier), Valid: true}
	case database.ProvisionerJobStatusCanceled:
		completedAt = sql.NullTime{Time: clock.Now().Add(earlier), Valid: true}
		cancelledAt = sql.NullTime{Time: clock.Now().Add(earlier), Valid: true}
	case database.ProvisionerJobStatusSucceeded:
		completedAt = sql.NullTime{Time: clock.Now().Add(earlier), Valid: true}
	default:
	}

	// Apply all provided prebuild options.
	prebuiltOptions := &prebuildOptions{}
	for _, opt := range opts {
		opt(prebuiltOptions)
	}

	// Set createdAt to default value if not overridden by options.
	createdAt := clock.Now().Add(muchEarlier)
	if prebuiltOptions.createdAt != nil {
		createdAt = *prebuiltOptions.createdAt
		// Ensure startedAt matches createdAt for consistency.
		startedAt = sql.NullTime{Time: createdAt, Valid: true}
	}

	workspace := dbgen.Workspace(t, db, database.WorkspaceTable{
		TemplateID:     templateID,
		OrganizationID: orgID,
		OwnerID:        ownerID,
		Deleted:        false,
		CreatedAt:      createdAt,
	})
	job := dbgen.ProvisionerJob(t, db, ps, database.ProvisionerJob{
		InitiatorID:    initiatorID,
		CreatedAt:      createdAt,
		StartedAt:      startedAt,
		CompletedAt:    completedAt,
		CanceledAt:     cancelledAt,
		OrganizationID: orgID,
		Error:          buildError,
	})
	workspaceBuild := dbgen.WorkspaceBuild(t, db, database.WorkspaceBuild{
		WorkspaceID:             workspace.ID,
		InitiatorID:             initiatorID,
		TemplateVersionID:       templateVersionID,
		JobID:                   job.ID,
		TemplateVersionPresetID: uuid.NullUUID{UUID: preset.ID, Valid: true},
		Transition:              transition,
		CreatedAt:               clock.Now(),
	})
	dbgen.WorkspaceBuildParameters(t, db, []database.WorkspaceBuildParameter{
		{
			WorkspaceBuildID: workspaceBuild.ID,
			Name:             "test",
			Value:            "test",
		},
	})

	return workspace, workspaceBuild
}

// nolint:revive // It's a control flag, but this is a test.
func setupTestDBWorkspaceAgent(t *testing.T, db database.Store, workspaceID uuid.UUID, eligible bool) database.WorkspaceAgent {
	build, err := db.GetLatestWorkspaceBuildByWorkspaceID(t.Context(), workspaceID)
	require.NoError(t, err)

	res := dbgen.WorkspaceResource(t, db, database.WorkspaceResource{JobID: build.JobID})
	agent := dbgen.WorkspaceAgent(t, db, database.WorkspaceAgent{
		ResourceID: res.ID,
	})

	// A prebuilt workspace is considered eligible when its agent is in a "ready" lifecycle state.
	// i.e. connected to the control plane and all startup scripts have run.
	if eligible {
		require.NoError(t, db.UpdateWorkspaceAgentLifecycleStateByID(t.Context(), database.UpdateWorkspaceAgentLifecycleStateByIDParams{
			ID:             agent.ID,
			LifecycleState: database.WorkspaceAgentLifecycleStateReady,
			StartedAt:      sql.NullTime{Time: dbtime.Now().Add(-time.Minute), Valid: true},
			ReadyAt:        sql.NullTime{Time: dbtime.Now(), Valid: true},
		}))
	}

	return agent
}

// setupTestDBAntagonists creates test antagonists that should not influence running prebuild workspace tests.
//  1. A stopped prebuilt workspace (STOP then START transitions, owned by
//     prebuilds system user).
//  2. A running regular workspace (not owned by the prebuilds system user).
func setupTestDBPrebuildAntagonists(t *testing.T, db database.Store, ps pubsub.Pubsub, org database.Organization) {
	t.Helper()

	templateAdmin := dbgen.User(t, db, database.User{RBACRoles: []string{codersdk.RoleTemplateAdmin}})
	_ = dbgen.OrganizationMember(t, db, database.OrganizationMember{
		OrganizationID: org.ID,
		UserID:         templateAdmin.ID,
	})
	member := dbgen.User(t, db, database.User{})
	_ = dbgen.OrganizationMember(t, db, database.OrganizationMember{
		OrganizationID: org.ID,
		UserID:         member.ID,
	})
	tpl := dbgen.Template(t, db, database.Template{
		OrganizationID: org.ID,
		CreatedBy:      templateAdmin.ID,
	})
	tv := dbgen.TemplateVersion(t, db, database.TemplateVersion{
		TemplateID:     uuid.NullUUID{UUID: tpl.ID, Valid: true},
		OrganizationID: org.ID,
		CreatedBy:      templateAdmin.ID,
	})

	// 1) Stopped prebuilt workspace (owned by prebuilds system user)
	stoppedPrebuild := dbgen.Workspace(t, db, database.WorkspaceTable{
		OwnerID:    database.PrebuildsSystemUserID,
		TemplateID: tpl.ID,
		Name:       "prebuild-antagonist-stopped",
		Deleted:    false,
	})

	// STOP build (build number 2, most recent)
	stoppedJob2 := dbgen.ProvisionerJob(t, db, ps, database.ProvisionerJob{
		OrganizationID: org.ID,
		InitiatorID:    database.PrebuildsSystemUserID,
		Provisioner:    database.ProvisionerTypeEcho,
		Type:           database.ProvisionerJobTypeWorkspaceBuild,
		StartedAt:      sql.NullTime{Time: dbtime.Now().Add(-30 * time.Second), Valid: true},
		CompletedAt:    sql.NullTime{Time: dbtime.Now().Add(-20 * time.Second), Valid: true},
		Error:          sql.NullString{},
		ErrorCode:      sql.NullString{},
	})
	dbgen.WorkspaceBuild(t, db, database.WorkspaceBuild{
		WorkspaceID:       stoppedPrebuild.ID,
		TemplateVersionID: tv.ID,
		JobID:             stoppedJob2.ID,
		BuildNumber:       2,
		Transition:        database.WorkspaceTransitionStop,
		InitiatorID:       database.PrebuildsSystemUserID,
		Reason:            database.BuildReasonInitiator,
		// Explicitly not using a preset here. This shouldn't normally be possible,
		// but without this the reconciler will try to create a new prebuild for
		// this preset, which will affect the tests.
		TemplateVersionPresetID: uuid.NullUUID{},
	})

	// START build (build number 1, older)
	stoppedJob1 := dbgen.ProvisionerJob(t, db, ps, database.ProvisionerJob{
		OrganizationID: org.ID,
		InitiatorID:    database.PrebuildsSystemUserID,
		Provisioner:    database.ProvisionerTypeEcho,
		Type:           database.ProvisionerJobTypeWorkspaceBuild,
		StartedAt:      sql.NullTime{Time: dbtime.Now().Add(-60 * time.Second), Valid: true},
		CompletedAt:    sql.NullTime{Time: dbtime.Now().Add(-50 * time.Second), Valid: true},
		Error:          sql.NullString{},
		ErrorCode:      sql.NullString{},
	})
	dbgen.WorkspaceBuild(t, db, database.WorkspaceBuild{
		WorkspaceID:       stoppedPrebuild.ID,
		TemplateVersionID: tv.ID,
		JobID:             stoppedJob1.ID,
		BuildNumber:       1,
		Transition:        database.WorkspaceTransitionStart,
		InitiatorID:       database.PrebuildsSystemUserID,
		Reason:            database.BuildReasonInitiator,
	})

	// 2) Running regular workspace (not owned by prebuilds system user)
	regularWorkspace := dbgen.Workspace(t, db, database.WorkspaceTable{
		OwnerID:    member.ID,
		TemplateID: tpl.ID,
		Name:       "antagonist-regular-workspace",
		Deleted:    false,
	})
	regularJob := dbgen.ProvisionerJob(t, db, nil, database.ProvisionerJob{
		OrganizationID: org.ID,
		InitiatorID:    member.ID,
		Provisioner:    database.ProvisionerTypeEcho,
		Type:           database.ProvisionerJobTypeWorkspaceBuild,
		StartedAt:      sql.NullTime{Time: dbtime.Now().Add(-40 * time.Second), Valid: true},
		CompletedAt:    sql.NullTime{Time: dbtime.Now().Add(-30 * time.Second), Valid: true},
		Error:          sql.NullString{},
		ErrorCode:      sql.NullString{},
	})
	dbgen.WorkspaceBuild(t, db, database.WorkspaceBuild{
		WorkspaceID:       regularWorkspace.ID,
		TemplateVersionID: tv.ID,
		JobID:             regularJob.ID,
		BuildNumber:       1,
		Transition:        database.WorkspaceTransitionStart,
		InitiatorID:       member.ID,
		Reason:            database.BuildReasonInitiator,
	})
}

var allTransitions = []database.WorkspaceTransition{
	database.WorkspaceTransitionStart,
	database.WorkspaceTransitionStop,
	database.WorkspaceTransitionDelete,
}

var allJobStatuses = []database.ProvisionerJobStatus{
	database.ProvisionerJobStatusPending,
	database.ProvisionerJobStatusRunning,
	database.ProvisionerJobStatusSucceeded,
	database.ProvisionerJobStatusFailed,
	database.ProvisionerJobStatusCanceled,
	database.ProvisionerJobStatusCanceling,
}

func allJobStatusesExcept(except ...database.ProvisionerJobStatus) []database.ProvisionerJobStatus {
	return slice.Filter(except, func(status database.ProvisionerJobStatus) bool {
		return !slice.Contains(allJobStatuses, status)
	})
}

func mustParseTime(t *testing.T, layout, value string) time.Time {
	t.Helper()
	parsedTime, err := time.Parse(layout, value)
	require.NoError(t, err)
	return parsedTime
}

func TestReconciliationRespectsPauseSetting(t *testing.T) {
	t.Parallel()

	if !dbtestutil.WillUsePostgres() {
		t.Skip("This test requires postgres")
	}

	ctx := testutil.Context(t, testutil.WaitLong)
	clock := quartz.NewMock(t)
	db, ps := dbtestutil.NewDB(t)
	cfg := codersdk.PrebuildsConfig{
		ReconciliationInterval: serpent.Duration(testutil.WaitLong),
	}
	logger := testutil.Logger(t)
	cache := files.New(prometheus.NewRegistry(), &coderdtest.FakeAuthorizer{})
	reconciler := prebuilds.NewStoreReconciler(db, ps, cache, cfg, logger, clock, prometheus.NewRegistry(), newNoopEnqueuer())

	// Setup a template with a preset that should create prebuilds
	org := dbgen.Organization(t, db, database.Organization{})
	user := dbgen.User(t, db, database.User{})
	template := dbgen.Template(t, db, database.Template{
		CreatedBy:      user.ID,
		OrganizationID: org.ID,
	})
	templateVersionID := setupTestDBTemplateVersion(ctx, t, clock, db, ps, org.ID, user.ID, template.ID)
	_ = setupTestDBPreset(t, db, templateVersionID, 2, "test")

	// Initially, reconciliation should create prebuilds
	err := reconciler.ReconcileAll(ctx)
	require.NoError(t, err)

	// Verify that prebuilds were created
	workspaces, err := db.GetWorkspacesByTemplateID(ctx, template.ID)
	require.NoError(t, err)
	require.Len(t, workspaces, 2, "should have created 2 prebuilds")

	// Now pause prebuilds reconciliation
	err = prebuilds.SetPrebuildsReconciliationPaused(ctx, db, true)
	require.NoError(t, err)

	// Delete the existing prebuilds to simulate a scenario where reconciliation would normally recreate them
	for _, workspace := range workspaces {
		err = db.UpdateWorkspaceDeletedByID(ctx, database.UpdateWorkspaceDeletedByIDParams{
			ID:      workspace.ID,
			Deleted: true,
		})
		require.NoError(t, err)
	}

	// Verify prebuilds are deleted
	workspaces, err = db.GetWorkspacesByTemplateID(ctx, template.ID)
	require.NoError(t, err)
	require.Len(t, workspaces, 0, "prebuilds should be deleted")

	// Run reconciliation again - it should be paused and not recreate prebuilds
	err = reconciler.ReconcileAll(ctx)
	require.NoError(t, err)

	// Verify that no new prebuilds were created because reconciliation is paused
	workspaces, err = db.GetWorkspacesByTemplateID(ctx, template.ID)
	require.NoError(t, err)
	require.Len(t, workspaces, 0, "should not create prebuilds when reconciliation is paused")

	// Resume prebuilds reconciliation
	err = prebuilds.SetPrebuildsReconciliationPaused(ctx, db, false)
	require.NoError(t, err)

	// Run reconciliation again - it should now recreate the prebuilds
	err = reconciler.ReconcileAll(ctx)
	require.NoError(t, err)

	// Verify that prebuilds were recreated
	workspaces, err = db.GetWorkspacesByTemplateID(ctx, template.ID)
	require.NoError(t, err)
	require.Len(t, workspaces, 2, "should have recreated 2 prebuilds after resuming")
}

func TestCompareGetRunningPrebuiltWorkspacesResults(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	// Helper to create test data
	createWorkspaceRow := func(id string, name string, ready bool) database.GetRunningPrebuiltWorkspacesRow {
		uid := uuid.MustParse(id)
		return database.GetRunningPrebuiltWorkspacesRow{
			ID:                uid,
			Name:              name,
			TemplateID:        uuid.New(),
			TemplateVersionID: uuid.New(),
			CurrentPresetID:   uuid.NullUUID{UUID: uuid.New(), Valid: true},
			Ready:             ready,
			CreatedAt:         time.Now(),
		}
	}

	createOptimizedRow := func(row database.GetRunningPrebuiltWorkspacesRow) database.GetRunningPrebuiltWorkspacesOptimizedRow {
		return database.GetRunningPrebuiltWorkspacesOptimizedRow(row)
	}

	t.Run("identical results - no logging", func(t *testing.T) {
		t.Parallel()

		var sb strings.Builder
		logger := slog.Make(slogjson.Sink(&sb))

		original := []database.GetRunningPrebuiltWorkspacesRow{
			createWorkspaceRow("550e8400-e29b-41d4-a716-446655440000", "workspace1", true),
			createWorkspaceRow("550e8400-e29b-41d4-a716-446655440001", "workspace2", false),
		}

		optimized := []database.GetRunningPrebuiltWorkspacesOptimizedRow{
			createOptimizedRow(original[0]),
			createOptimizedRow(original[1]),
		}

		prebuilds.CompareGetRunningPrebuiltWorkspacesResults(ctx, logger, original, optimized)

		// Should not log any errors when results are identical
		require.Empty(t, strings.TrimSpace(sb.String()))
	})

	t.Run("count mismatch - logs error", func(t *testing.T) {
		t.Parallel()

		var sb strings.Builder
		logger := slog.Make(slogjson.Sink(&sb))

		original := []database.GetRunningPrebuiltWorkspacesRow{
			createWorkspaceRow("550e8400-e29b-41d4-a716-446655440000", "workspace1", true),
		}

		optimized := []database.GetRunningPrebuiltWorkspacesOptimizedRow{
			createOptimizedRow(original[0]),
			createOptimizedRow(createWorkspaceRow("550e8400-e29b-41d4-a716-446655440001", "workspace2", false)),
		}

		prebuilds.CompareGetRunningPrebuiltWorkspacesResults(ctx, logger, original, optimized)

		// Should log exactly one error.
		if lines := strings.Split(strings.TrimSpace(sb.String()), "\n"); assert.NotEmpty(t, lines) {
			require.Len(t, lines, 1)
			assert.Contains(t, lines[0], "ERROR")
			assert.Contains(t, lines[0], "workspace2")
			assert.Contains(t, lines[0], "CurrentPresetID")
		}
	})

	t.Run("count mismatch - other direction", func(t *testing.T) {
		t.Parallel()

		var sb strings.Builder
		logger := slog.Make(slogjson.Sink(&sb))

		original := []database.GetRunningPrebuiltWorkspacesRow{}

		optimized := []database.GetRunningPrebuiltWorkspacesOptimizedRow{
			createOptimizedRow(createWorkspaceRow("550e8400-e29b-41d4-a716-446655440001", "workspace2", false)),
		}

		prebuilds.CompareGetRunningPrebuiltWorkspacesResults(ctx, logger, original, optimized)

		if lines := strings.Split(strings.TrimSpace(sb.String()), "\n"); assert.NotEmpty(t, lines) {
			require.Len(t, lines, 1)
			assert.Contains(t, lines[0], "ERROR")
			assert.Contains(t, lines[0], "workspace2")
			assert.Contains(t, lines[0], "CurrentPresetID")
		}
	})

	t.Run("field differences - logs errors", func(t *testing.T) {
		t.Parallel()

		var sb strings.Builder
		logger := slog.Make(slogjson.Sink(&sb))

		workspace1 := createWorkspaceRow("550e8400-e29b-41d4-a716-446655440000", "workspace1", true)
		workspace2 := createWorkspaceRow("550e8400-e29b-41d4-a716-446655440001", "workspace2", false)

		original := []database.GetRunningPrebuiltWorkspacesRow{workspace1, workspace2}

		// Create optimized with different values
		optimized1 := createOptimizedRow(workspace1)
		optimized1.Name = "different-name" // Different name
		optimized1.Ready = false           // Different ready status

		optimized2 := createOptimizedRow(workspace2)
		optimized2.CurrentPresetID = uuid.NullUUID{Valid: false} // Different preset ID (NULL)

		optimized := []database.GetRunningPrebuiltWorkspacesOptimizedRow{optimized1, optimized2}

		prebuilds.CompareGetRunningPrebuiltWorkspacesResults(ctx, logger, original, optimized)

		// Should log exactly one error with a cmp.Diff output
		if lines := strings.Split(strings.TrimSpace(sb.String()), "\n"); assert.NotEmpty(t, lines) {
			require.Len(t, lines, 1)
			assert.Contains(t, lines[0], "ERROR")
			assert.Contains(t, lines[0], "different-name")
			assert.Contains(t, lines[0], "workspace1")
			assert.Contains(t, lines[0], "Ready")
			assert.Contains(t, lines[0], "CurrentPresetID")
		}
	})

	t.Run("empty results - no logging", func(t *testing.T) {
		t.Parallel()

		var sb strings.Builder
		logger := slog.Make(slogjson.Sink(&sb))

		original := []database.GetRunningPrebuiltWorkspacesRow{}
		optimized := []database.GetRunningPrebuiltWorkspacesOptimizedRow{}

		prebuilds.CompareGetRunningPrebuiltWorkspacesResults(ctx, logger, original, optimized)

		// Should not log any errors when both results are empty
		require.Empty(t, strings.TrimSpace(sb.String()))
	})

	t.Run("nil original", func(t *testing.T) {
		t.Parallel()
		var sb strings.Builder
		logger := slog.Make(slogjson.Sink(&sb))
		prebuilds.CompareGetRunningPrebuiltWorkspacesResults(ctx, logger, nil, []database.GetRunningPrebuiltWorkspacesOptimizedRow{})
		// Should not log any errors when original is nil
		require.Empty(t, strings.TrimSpace(sb.String()))
	})

	t.Run("nil optimized ", func(t *testing.T) {
		t.Parallel()
		var sb strings.Builder
		logger := slog.Make(slogjson.Sink(&sb))
		prebuilds.CompareGetRunningPrebuiltWorkspacesResults(ctx, logger, []database.GetRunningPrebuiltWorkspacesRow{}, nil)
		// Should not log any errors when optimized is nil
		require.Empty(t, strings.TrimSpace(sb.String()))
	})
}
