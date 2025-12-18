package prebuilds_test

import (
	"context"
	"database/sql"
	"fmt"
	"sort"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/xerrors"
	"tailscale.com/types/ptr"

	"cdr.dev/slog"
	"cdr.dev/slog/sloggers/slogtest"

	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/database/dbfake"
	"github.com/coder/coder/v2/coderd/database/dbgen"
	"github.com/coder/coder/v2/coderd/database/dbtestutil"
	"github.com/coder/coder/v2/coderd/database/dbtime"
	"github.com/coder/coder/v2/coderd/database/pubsub"
	"github.com/coder/coder/v2/coderd/files"
	"github.com/coder/coder/v2/coderd/notifications"
	"github.com/coder/coder/v2/coderd/notifications/notificationstest"
	"github.com/coder/coder/v2/coderd/rbac"
	"github.com/coder/coder/v2/coderd/util/slice"
	"github.com/coder/coder/v2/coderd/wsbuilder"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/enterprise/coderd/prebuilds"
	sdkproto "github.com/coder/coder/v2/provisionersdk/proto"
	"github.com/coder/coder/v2/testutil"
	"github.com/coder/quartz"
	"github.com/coder/serpent"
)

func TestNoReconciliationActionsIfNoPresets(t *testing.T) {
	// Scenario: No reconciliation actions are taken if there are no presets
	t.Parallel()

	clock := quartz.NewMock(t)
	ctx := testutil.Context(t, testutil.WaitLong)
	db, ps := dbtestutil.NewDB(t)
	cfg := codersdk.PrebuildsConfig{
		ReconciliationInterval: serpent.Duration(testutil.WaitLong),
	}
	logger := testutil.Logger(t)
	cache := files.New(prometheus.NewRegistry(), &coderdtest.FakeAuthorizer{})
	controller := prebuilds.NewStoreReconciler(db, ps, cache, cfg, logger, quartz.NewMock(t), prometheus.NewRegistry(), newNoopEnqueuer(), newNoopUsageCheckerPtr())

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
	_, err = controller.ReconcileAll(ctx)
	require.NoError(t, err)

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

	clock := quartz.NewMock(t)
	ctx := testutil.Context(t, testutil.WaitLong)
	db, ps := dbtestutil.NewDB(t)
	cfg := codersdk.PrebuildsConfig{
		ReconciliationInterval: serpent.Duration(testutil.WaitLong),
	}
	logger := testutil.Logger(t)
	cache := files.New(prometheus.NewRegistry(), &coderdtest.FakeAuthorizer{})
	controller := prebuilds.NewStoreReconciler(db, ps, cache, cfg, logger, quartz.NewMock(t), prometheus.NewRegistry(), newNoopEnqueuer(), newNoopUsageCheckerPtr())

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
	_, err = controller.ReconcileAll(ctx)
	require.NoError(t, err)

	// then no reconciliation actions are taken
	// because without prebuilds, there is nothing to reconcile
	// even if there are presets
	jobs, err := db.GetProvisionerJobsCreatedAfter(ctx, clock.Now().Add(earlier))
	require.NoError(t, err)
	require.Empty(t, jobs)
}

func TestPrebuildReconciliation(t *testing.T) {
	t.Parallel()

	testScenarios := []testScenario{
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
			templateDeleted:         []bool{false},
			shouldCreateNewPrebuild: ptr.To(false),
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
			// TODO(ssncferreira): Investigate why the GetRunningPrebuiltWorkspaces query is returning 0 rows.
			//   When a template version is inactive (templateVersionActive = false), any prebuilds in the
			//   database.ProvisionerJobStatusRunning state should be deleted.
			name: "never attempt to interfere with prebuilds from an active template version",
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
			templateVersionActive:   []bool{true},
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
	for _, tc := range testScenarios {
		testCases := tc.testCases()
		for _, tc := range testCases {
			tc.run(t)
		}
	}
}

// testScenario is a collection of test cases that illustrate the same business rule.
// A testScenario describes a set of test properties for which the same test expecations
// hold. A testScenario may be decomposed into multiple testCase structs, which can then be run.
type testScenario struct {
	name                      string
	prebuildLatestTransitions []database.WorkspaceTransition
	prebuildJobStatuses       []database.ProvisionerJobStatus
	templateVersionActive     []bool
	templateDeleted           []bool
	shouldCreateNewPrebuild   *bool
	shouldDeleteOldPrebuild   *bool
	expectOrgMembership       *bool
	expectGroupMembership     *bool
}

func (ts testScenario) testCases() []testCase {
	testCases := []testCase{}
	for _, templateVersionActive := range ts.templateVersionActive {
		for _, prebuildLatestTransition := range ts.prebuildLatestTransitions {
			for _, prebuildJobStatus := range ts.prebuildJobStatuses {
				for _, templateDeleted := range ts.templateDeleted {
					for _, useBrokenPubsub := range []bool{true, false} {
						testCase := testCase{
							name:                     ts.name,
							templateVersionActive:    templateVersionActive,
							prebuildLatestTransition: prebuildLatestTransition,
							prebuildJobStatus:        prebuildJobStatus,
							templateDeleted:          templateDeleted,
							useBrokenPubsub:          useBrokenPubsub,
							shouldCreateNewPrebuild:  ts.shouldCreateNewPrebuild,
							shouldDeleteOldPrebuild:  ts.shouldDeleteOldPrebuild,
							expectOrgMembership:      ts.expectOrgMembership,
							expectGroupMembership:    ts.expectGroupMembership,
						}
						testCases = append(testCases, testCase)
					}
				}
			}
		}
	}

	return testCases
}

type testCase struct {
	name                     string
	prebuildLatestTransition database.WorkspaceTransition
	prebuildJobStatus        database.ProvisionerJobStatus
	templateVersionActive    bool
	templateDeleted          bool
	useBrokenPubsub          bool
	shouldCreateNewPrebuild  *bool
	shouldDeleteOldPrebuild  *bool
	expectOrgMembership      *bool
	expectGroupMembership    *bool
}

func (tc testCase) run(t *testing.T) {
	t.Run(tc.name, func(t *testing.T) {
		t.Parallel()
		t.Cleanup(func() {
			if t.Failed() {
				t.Logf("failed to run test: %s", tc.name)
				t.Logf("templateVersionActive: %t", tc.templateVersionActive)
				t.Logf("prebuildLatestTransition: %s", tc.prebuildLatestTransition)
				t.Logf("prebuildJobStatus: %s", tc.prebuildJobStatus)
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
		org, template := setupTestDBTemplate(t, db, ownerID, tc.templateDeleted)
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
			tc.prebuildLatestTransition,
			tc.prebuildJobStatus,
			org.ID,
			preset,
			template.ID,
			templateVersionID,
		)

		setupTestDBPrebuildAntagonists(t, db, pubSub, org)

		if !tc.templateVersionActive {
			// Create a new template version and mark it as active
			// This marks the template version that we care about as inactive
			setupTestDBTemplateVersion(ctx, t, clock, db, pubSub, org.ID, ownerID, template.ID)
		}

		if tc.useBrokenPubsub {
			pubSub = &brokenPublisher{Pubsub: pubSub}
		}
		cache := files.New(prometheus.NewRegistry(), &coderdtest.FakeAuthorizer{})
		controller := prebuilds.NewStoreReconciler(db, pubSub, cache, cfg, logger, quartz.NewMock(t), prometheus.NewRegistry(), newNoopEnqueuer(), newNoopUsageCheckerPtr())

		// Run the reconciliation multiple times to ensure idempotency
		// 8 was arbitrary, but large enough to reasonably trust the result
		for i := 1; i <= 8; i++ {
			_, err := controller.ReconcileAll(ctx)
			require.NoErrorf(t, err, "failed on iteration %d", i)

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
					require.Equal(t, tc.prebuildLatestTransition, builds[0].Transition)
				}
			}
		}
	})
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
	controller := prebuilds.NewStoreReconciler(db, pubSub, cache, cfg, logger, quartz.NewMock(t), prometheus.NewRegistry(), newNoopEnqueuer(), newNoopUsageCheckerPtr())

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
		_, err := controller.ReconcileAll(ctx)
		require.NoErrorf(t, err, "failed on iteration %d", i)

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
			controller := prebuilds.NewStoreReconciler(db, pubSub, cache, cfg, logger, clock, prometheus.NewRegistry(), newNoopEnqueuer(), newNoopUsageCheckerPtr())

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

			_, err := controller.ReconcileAll(ctx)
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

	templateDeleted := false

	clock := quartz.NewMock(t)
	ctx := testutil.Context(t, testutil.WaitShort)
	cfg := codersdk.PrebuildsConfig{}
	logger := slogtest.Make(
		t, &slogtest.Options{IgnoreErrors: true},
	).Leveled(slog.LevelDebug)
	db, pubSub := dbtestutil.NewDB(t)
	cache := files.New(prometheus.NewRegistry(), &coderdtest.FakeAuthorizer{})
	controller := prebuilds.NewStoreReconciler(db, pubSub, cache, cfg, logger, quartz.NewMock(t), prometheus.NewRegistry(), newNoopEnqueuer(), newNoopUsageCheckerPtr())

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
		_, err := controller.ReconcileAll(ctx)
		require.NoErrorf(t, err, "failed on iteration %d", i)

		workspaces, err := db.GetWorkspacesByTemplateID(ctx, template.ID)
		require.NoError(t, err)
		newPrebuildCount := len(workspaces)

		// NOTE: we don't have any new prebuilds, because their creation constantly fails.
		require.Equal(t, int32(0), int32(newPrebuildCount)) // nolint:gosec
	}
}

func TestDeletionOfPrebuiltWorkspaceWithInvalidPreset(t *testing.T) {
	t.Parallel()

	templateDeleted := false

	clock := quartz.NewMock(t)
	ctx := testutil.Context(t, testutil.WaitShort)
	cfg := codersdk.PrebuildsConfig{}
	logger := slogtest.Make(
		t, &slogtest.Options{IgnoreErrors: true},
	).Leveled(slog.LevelDebug)
	db, pubSub := dbtestutil.NewDB(t)
	cache := files.New(prometheus.NewRegistry(), &coderdtest.FakeAuthorizer{})
	controller := prebuilds.NewStoreReconciler(db, pubSub, cache, cfg, logger, quartz.NewMock(t), prometheus.NewRegistry(), newNoopEnqueuer(), newNoopUsageCheckerPtr())

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
	_, err = controller.ReconcileAll(ctx)
	require.NoError(t, err)

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
			controller := prebuilds.NewStoreReconciler(db, pubSub, cache, cfg, logger, clock, registry, fakeEnqueuer, newNoopUsageCheckerPtr())

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
			_, err = controller.ReconcileAll(ctx)
			require.NoError(t, err)

			// These two additional calls to ReconcileAll should not trigger any notifications.
			// A notification is only sent once.
			_, err = controller.ReconcileAll(ctx)
			require.NoError(t, err)
			_, err = controller.ReconcileAll(ctx)
			require.NoError(t, err)

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
			controller := prebuilds.NewStoreReconciler(db, pubSub, cache, cfg, logger, clock, registry, fakeEnqueuer, newNoopUsageCheckerPtr())

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
			_, err = controller.ReconcileAll(ctx)
			require.NoError(t, err)

			// These two additional calls to ReconcileAll should not trigger any notifications.
			// A notification is only sent once.
			_, err = controller.ReconcileAll(ctx)
			require.NoError(t, err)
			_, err = controller.ReconcileAll(ctx)
			require.NoError(t, err)

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
			_, err = controller.ReconcileAll(ctx)
			require.NoError(t, err)

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
	reconciler := prebuilds.NewStoreReconciler(db, pubSub, cache, cfg, logger, clock, prometheus.NewRegistry(), newNoopEnqueuer(), newNoopUsageCheckerPtr())

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
	reconciler := prebuilds.NewStoreReconciler(db, ps, cache, cfg, logger, clock, prometheus.NewRegistry(), newNoopEnqueuer(), newNoopUsageCheckerPtr())

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
				newNoopEnqueuer(),
				newNoopUsageCheckerPtr())
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

	ctx := testutil.Context(t, testutil.WaitSuperLong)

	// Setup.
	clock := quartz.NewMock(t)
	logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: false}).Leveled(slog.LevelDebug)
	db, ps := dbtestutil.NewDB(t)

	fakeEnqueuer := newFakeEnqueuer()
	registry := prometheus.NewRegistry()
	cache := files.New(registry, &coderdtest.FakeAuthorizer{})
	reconciler := prebuilds.NewStoreReconciler(db, ps, cache, codersdk.PrebuildsConfig{}, logger, clock, registry, fakeEnqueuer, newNoopUsageCheckerPtr())

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
			controller := prebuilds.NewStoreReconciler(db, pubSub, cache, cfg, logger, clock, registry, fakeEnqueuer, newNoopUsageCheckerPtr())

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
			_, err = controller.ReconcileAll(ctx)
			require.NoError(t, err)

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

func TestCancelPendingPrebuilds(t *testing.T) {
	t.Parallel()

	t.Run("CancelPendingPrebuilds", func(t *testing.T) {
		t.Parallel()

		for _, tt := range []struct {
			name       string
			setupBuild func(
				t *testing.T,
				db database.Store,
				client *codersdk.Client,
				orgID uuid.UUID,
				templateID uuid.UUID,
				templateVersionID uuid.UUID,
				presetID uuid.NullUUID,
			) dbfake.WorkspaceResponse
			activeTemplateVersion bool
			previouslyCanceled    bool
			previouslyCompleted   bool
			shouldCancel          bool
		}{
			// Should cancel pending prebuild-related jobs from a non-active template version
			{
				name: "CancelsPendingPrebuildJobNonActiveVersion",
				// Given: a pending prebuild job
				setupBuild: func(t *testing.T,
					db database.Store,
					client *codersdk.Client,
					orgID uuid.UUID,
					templateID uuid.UUID,
					templateVersionID uuid.UUID,
					presetID uuid.NullUUID,
				) dbfake.WorkspaceResponse {
					return dbfake.WorkspaceBuild(t, db, database.WorkspaceTable{
						OwnerID:        database.PrebuildsSystemUserID,
						OrganizationID: orgID,
						TemplateID:     templateID,
					}).Pending().Seed(database.WorkspaceBuild{
						InitiatorID:             database.PrebuildsSystemUserID,
						TemplateVersionID:       templateVersionID,
						TemplateVersionPresetID: presetID,
					}).Do()
				},
				activeTemplateVersion: false,
				previouslyCanceled:    false,
				previouslyCompleted:   false,
				shouldCancel:          true,
			},
			// Should not cancel pending prebuild-related jobs from an active template version
			{
				name: "DoesNotCancelPendingPrebuildJobActiveVersion",
				// Given: a pending prebuild job
				setupBuild: func(t *testing.T,
					db database.Store,
					client *codersdk.Client,
					orgID uuid.UUID,
					templateID uuid.UUID,
					templateVersionID uuid.UUID,
					presetID uuid.NullUUID,
				) dbfake.WorkspaceResponse {
					return dbfake.WorkspaceBuild(t, db, database.WorkspaceTable{
						OwnerID:        database.PrebuildsSystemUserID,
						OrganizationID: orgID,
						TemplateID:     templateID,
					}).Pending().Seed(database.WorkspaceBuild{
						InitiatorID:             database.PrebuildsSystemUserID,
						TemplateVersionID:       templateVersionID,
						TemplateVersionPresetID: presetID,
					}).Do()
				},
				activeTemplateVersion: true,
				previouslyCanceled:    false,
				previouslyCompleted:   false,
				shouldCancel:          false,
			},
			// Should not cancel pending prebuild-related jobs associated to a second workspace build
			{
				name: "DoesNotCancelPendingPrebuildJobSecondBuild",
				// Given: a pending prebuild job associated to a second workspace build
				setupBuild: func(t *testing.T,
					db database.Store,
					client *codersdk.Client,
					orgID uuid.UUID,
					templateID uuid.UUID,
					templateVersionID uuid.UUID,
					presetID uuid.NullUUID,
				) dbfake.WorkspaceResponse {
					return dbfake.WorkspaceBuild(t, db, database.WorkspaceTable{
						OwnerID:        database.PrebuildsSystemUserID,
						OrganizationID: orgID,
						TemplateID:     templateID,
					}).Pending().Seed(database.WorkspaceBuild{
						InitiatorID:             database.PrebuildsSystemUserID,
						BuildNumber:             int32(2),
						TemplateVersionID:       templateVersionID,
						TemplateVersionPresetID: presetID,
					}).Do()
				},
				activeTemplateVersion: false,
				previouslyCanceled:    false,
				previouslyCompleted:   false,
				shouldCancel:          false,
			},
			// Should not cancel pending prebuild-related jobs of a different template
			{
				name: "DoesNotCancelPrebuildJobDifferentTemplate",
				// Given: a pending prebuild job belonging to a different template
				setupBuild: func(
					t *testing.T,
					db database.Store,
					client *codersdk.Client,
					orgID uuid.UUID,
					templateID uuid.UUID,
					templateVersionID uuid.UUID,
					presetID uuid.NullUUID,
				) dbfake.WorkspaceResponse {
					return dbfake.WorkspaceBuild(t, db, database.WorkspaceTable{
						OwnerID:        database.PrebuildsSystemUserID,
						OrganizationID: orgID,
						TemplateID:     uuid.Nil,
					}).Pending().Seed(database.WorkspaceBuild{
						InitiatorID:             database.PrebuildsSystemUserID,
						TemplateVersionID:       templateVersionID,
						TemplateVersionPresetID: presetID,
					}).Do()
				},
				activeTemplateVersion: false,
				previouslyCanceled:    false,
				previouslyCompleted:   false,
				shouldCancel:          false,
			},
			// Should not cancel pending user workspace build jobs
			{
				name: "DoesNotCancelUserWorkspaceJob",
				// Given: a pending user workspace build job
				setupBuild: func(
					t *testing.T,
					db database.Store,
					client *codersdk.Client,
					orgID uuid.UUID,
					templateID uuid.UUID,
					templateVersionID uuid.UUID,
					presetID uuid.NullUUID,
				) dbfake.WorkspaceResponse {
					_, member := coderdtest.CreateAnotherUser(t, client, orgID, rbac.RoleMember())
					return dbfake.WorkspaceBuild(t, db, database.WorkspaceTable{
						OwnerID:        member.ID,
						OrganizationID: orgID,
						TemplateID:     uuid.Nil,
					}).Pending().Seed(database.WorkspaceBuild{
						InitiatorID:             member.ID,
						TemplateVersionID:       templateVersionID,
						TemplateVersionPresetID: presetID,
					}).Do()
				},
				activeTemplateVersion: false,
				previouslyCanceled:    false,
				previouslyCompleted:   false,
				shouldCancel:          false,
			},
			// Should not cancel pending prebuild-related jobs with a delete transition
			{
				name: "DoesNotCancelPrebuildJobDeleteTransition",
				// Given: a pending prebuild job with a delete transition
				setupBuild: func(
					t *testing.T,
					db database.Store,
					client *codersdk.Client,
					orgID uuid.UUID,
					templateID uuid.UUID,
					templateVersionID uuid.UUID,
					presetID uuid.NullUUID,
				) dbfake.WorkspaceResponse {
					return dbfake.WorkspaceBuild(t, db, database.WorkspaceTable{
						OwnerID:        database.PrebuildsSystemUserID,
						OrganizationID: orgID,
						TemplateID:     templateID,
					}).Pending().Seed(database.WorkspaceBuild{
						InitiatorID:             database.PrebuildsSystemUserID,
						Transition:              database.WorkspaceTransitionDelete,
						TemplateVersionID:       templateVersionID,
						TemplateVersionPresetID: presetID,
					}).Do()
				},
				activeTemplateVersion: false,
				previouslyCanceled:    false,
				previouslyCompleted:   false,
				shouldCancel:          false,
			},
			// Should not cancel prebuild-related jobs already being processed by a provisioner
			{
				name: "DoesNotCancelRunningPrebuildJob",
				// Given: a running prebuild job
				setupBuild: func(
					t *testing.T,
					db database.Store,
					client *codersdk.Client,
					orgID uuid.UUID,
					templateID uuid.UUID,
					templateVersionID uuid.UUID,
					presetID uuid.NullUUID,
				) dbfake.WorkspaceResponse {
					return dbfake.WorkspaceBuild(t, db, database.WorkspaceTable{
						OwnerID:        database.PrebuildsSystemUserID,
						OrganizationID: orgID,
						TemplateID:     templateID,
					}).Starting().Seed(database.WorkspaceBuild{
						InitiatorID:             database.PrebuildsSystemUserID,
						TemplateVersionID:       templateVersionID,
						TemplateVersionPresetID: presetID,
					}).Do()
				},
				activeTemplateVersion: false,
				previouslyCanceled:    false,
				previouslyCompleted:   false,
				shouldCancel:          false,
			},
			// Should not cancel already canceled prebuild-related jobs
			{
				name: "DoesNotCancelCanceledPrebuildJob",
				// Given: a canceled prebuild job
				setupBuild: func(
					t *testing.T,
					db database.Store,
					client *codersdk.Client,
					orgID uuid.UUID,
					templateID uuid.UUID,
					templateVersionID uuid.UUID,
					presetID uuid.NullUUID,
				) dbfake.WorkspaceResponse {
					return dbfake.WorkspaceBuild(t, db, database.WorkspaceTable{
						OwnerID:        database.PrebuildsSystemUserID,
						OrganizationID: orgID,
						TemplateID:     templateID,
					}).Canceled().Seed(database.WorkspaceBuild{
						InitiatorID:             database.PrebuildsSystemUserID,
						TemplateVersionID:       templateVersionID,
						TemplateVersionPresetID: presetID,
					}).Do()
				},
				activeTemplateVersion: false,
				shouldCancel:          false,
				previouslyCanceled:    true,
				previouslyCompleted:   true,
			},
			// Should not cancel completed prebuild-related jobs
			{
				name: "DoesNotCancelCompletedPrebuildJob",
				// Given: a completed prebuild job
				setupBuild: func(
					t *testing.T,
					db database.Store,
					client *codersdk.Client,
					orgID uuid.UUID,
					templateID uuid.UUID,
					templateVersionID uuid.UUID,
					presetID uuid.NullUUID,
				) dbfake.WorkspaceResponse {
					return dbfake.WorkspaceBuild(t, db, database.WorkspaceTable{
						OwnerID:        database.PrebuildsSystemUserID,
						OrganizationID: orgID,
						TemplateID:     templateID,
					}).Seed(database.WorkspaceBuild{
						InitiatorID:             database.PrebuildsSystemUserID,
						TemplateVersionID:       templateVersionID,
						TemplateVersionPresetID: presetID,
					}).Do()
				},
				activeTemplateVersion: false,
				shouldCancel:          false,
				previouslyCanceled:    false,
				previouslyCompleted:   true,
			},
		} {
			t.Run(tt.name, func(t *testing.T) {
				t.Parallel()

				// Set the clock to Monday, January 1st, 2024 at 8:00 AM UTC to keep the test deterministic
				clock := quartz.NewMock(t)
				clock.Set(time.Date(2024, 1, 1, 8, 0, 0, 0, time.UTC))

				ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
				defer cancel()

				// Setup
				db, ps := dbtestutil.NewDB(t)
				client, _, _ := coderdtest.NewWithAPI(t, &coderdtest.Options{
					// Explicitly not including provisioner daemons, as we don't want the jobs to be processed
					// Jobs operations will be simulated via the database model
					IncludeProvisionerDaemon: false,
					Database:                 db,
					Pubsub:                   ps,
					Clock:                    clock,
				})
				fakeEnqueuer := newFakeEnqueuer()
				registry := prometheus.NewRegistry()
				cache := files.New(registry, &coderdtest.FakeAuthorizer{})
				logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: false}).Leveled(slog.LevelDebug)
				reconciler := prebuilds.NewStoreReconciler(db, ps, cache, codersdk.PrebuildsConfig{}, logger, clock, registry, fakeEnqueuer, newNoopUsageCheckerPtr())
				owner := coderdtest.CreateFirstUser(t, client)

				// Given: a template with a version containing a preset with 1 prebuild instance
				nonActivePresetID := uuid.NullUUID{
					UUID:  uuid.New(),
					Valid: true,
				}
				nonActiveTemplateVersion := dbfake.TemplateVersion(t, db).Seed(database.TemplateVersion{
					OrganizationID: owner.OrganizationID,
					CreatedBy:      owner.UserID,
				}).Preset(database.TemplateVersionPreset{
					ID: nonActivePresetID.UUID,
					DesiredInstances: sql.NullInt32{
						Int32: 1,
						Valid: true,
					},
				}).Do()
				templateID := nonActiveTemplateVersion.Template.ID

				// Given: a new active template version
				activePresetID := uuid.NullUUID{
					UUID:  uuid.New(),
					Valid: true,
				}
				activeTemplateVersion := dbfake.TemplateVersion(t, db).Seed(database.TemplateVersion{
					OrganizationID: owner.OrganizationID,
					CreatedBy:      owner.UserID,
					TemplateID: uuid.NullUUID{
						UUID:  templateID,
						Valid: true,
					},
				}).Preset(database.TemplateVersionPreset{
					ID: activePresetID.UUID,
					DesiredInstances: sql.NullInt32{
						Int32: 1,
						Valid: true,
					},
				}).SkipCreateTemplate().Do()

				var pendingWorkspace dbfake.WorkspaceResponse
				if tt.activeTemplateVersion {
					// Given: a prebuilt workspace, workspace build and respective provisioner job from an
					// active template version
					pendingWorkspace = tt.setupBuild(t, db, client,
						owner.OrganizationID, templateID, activeTemplateVersion.TemplateVersion.ID, activePresetID)
				} else {
					// Given: a prebuilt workspace, workspace build and respective provisioner job from a
					// non-active template version
					pendingWorkspace = tt.setupBuild(t, db, client,
						owner.OrganizationID, templateID, nonActiveTemplateVersion.TemplateVersion.ID, nonActivePresetID)
				}

				// Given: the new template version is promoted to active
				err := db.UpdateTemplateActiveVersionByID(ctx, database.UpdateTemplateActiveVersionByIDParams{
					ID:              templateID,
					ActiveVersionID: activeTemplateVersion.TemplateVersion.ID,
				})
				require.NoError(t, err)

				// When: the reconciliation loop is triggered
				_, err = reconciler.ReconcileAll(ctx)
				require.NoError(t, err)

				if tt.shouldCancel {
					// Then: the pending prebuild job from non-active version should be canceled
					cancelledJob, err := db.GetProvisionerJobByID(ctx, pendingWorkspace.Build.JobID)
					require.NoError(t, err)
					require.Equal(t, clock.Now().UTC(), cancelledJob.CanceledAt.Time.UTC())
					require.Equal(t, clock.Now().UTC(), cancelledJob.CompletedAt.Time.UTC())
					require.Equal(t, database.ProvisionerJobStatusCanceled, cancelledJob.JobStatus)

					// Then: the workspace should be deleted
					deletedWorkspace, err := db.GetWorkspaceByID(ctx, pendingWorkspace.Workspace.ID)
					require.NoError(t, err)
					require.True(t, deletedWorkspace.Deleted)
					latestBuild, err := db.GetLatestWorkspaceBuildByWorkspaceID(ctx, deletedWorkspace.ID)
					require.NoError(t, err)
					require.Equal(t, database.WorkspaceTransitionDelete, latestBuild.Transition)
					deleteJob, err := db.GetProvisionerJobByID(ctx, latestBuild.JobID)
					require.NoError(t, err)
					require.True(t, deleteJob.CompletedAt.Valid)
					require.False(t, deleteJob.WorkerID.Valid)
					require.Equal(t, database.ProvisionerJobStatusSucceeded, deleteJob.JobStatus)
				} else {
					// Then: the pending prebuild job should not be canceled
					job, err := db.GetProvisionerJobByID(ctx, pendingWorkspace.Build.JobID)
					require.NoError(t, err)
					if !tt.previouslyCanceled {
						require.Zero(t, job.CanceledAt.Time.UTC())
						require.NotEqual(t, database.ProvisionerJobStatusCanceled, job.JobStatus)
					}
					if !tt.previouslyCompleted {
						require.Zero(t, job.CompletedAt.Time.UTC())
					}

					// Then: the workspace should not be deleted
					workspace, err := db.GetWorkspaceByID(ctx, pendingWorkspace.Workspace.ID)
					require.NoError(t, err)
					require.False(t, workspace.Deleted)
				}
			})
		}
	})

	t.Run("CancelPendingPrebuildsMultipleTemplates", func(t *testing.T) {
		t.Parallel()

		createTemplateVersionWithPreset := func(
			t *testing.T,
			db database.Store,
			orgID uuid.UUID,
			userID uuid.UUID,
			templateID uuid.UUID,
			prebuiltInstances int32,
		) (uuid.UUID, uuid.UUID, uuid.UUID) {
			templatePreset := uuid.NullUUID{
				UUID:  uuid.New(),
				Valid: true,
			}
			templateVersion := dbfake.TemplateVersion(t, db).Seed(database.TemplateVersion{
				OrganizationID: orgID,
				CreatedBy:      userID,
				TemplateID: uuid.NullUUID{
					UUID:  templateID,
					Valid: true,
				},
			}).Preset(database.TemplateVersionPreset{
				ID: templatePreset.UUID,
				DesiredInstances: sql.NullInt32{
					Int32: prebuiltInstances,
					Valid: true,
				},
			}).Do()

			return templateVersion.Template.ID, templateVersion.TemplateVersion.ID, templatePreset.UUID
		}

		setupPrebuilds := func(
			t *testing.T,
			db database.Store,
			orgID uuid.UUID,
			templateID uuid.UUID,
			versionID uuid.UUID,
			presetID uuid.UUID,
			count int,
			pending bool,
		) []dbfake.WorkspaceResponse {
			prebuilds := make([]dbfake.WorkspaceResponse, count)
			for i := range count {
				builder := dbfake.WorkspaceBuild(t, db, database.WorkspaceTable{
					OwnerID:        database.PrebuildsSystemUserID,
					OrganizationID: orgID,
					TemplateID:     templateID,
				})

				if pending {
					builder = builder.Pending()
				}

				prebuilds[i] = builder.Seed(database.WorkspaceBuild{
					InitiatorID:       database.PrebuildsSystemUserID,
					TemplateVersionID: versionID,
					TemplateVersionPresetID: uuid.NullUUID{
						UUID:  presetID,
						Valid: true,
					},
				}).Do()
			}

			return prebuilds
		}

		checkIfJobCanceledAndDeleted := func(
			t *testing.T,
			clock *quartz.Mock,
			ctx context.Context,
			db database.Store,
			shouldBeCanceledAndDeleted bool,
			prebuilds []dbfake.WorkspaceResponse,
		) {
			for _, prebuild := range prebuilds {
				pendingJob, err := db.GetProvisionerJobByID(ctx, prebuild.Build.JobID)
				require.NoError(t, err)

				if shouldBeCanceledAndDeleted {
					// Pending job should be canceled
					require.Equal(t, database.ProvisionerJobStatusCanceled, pendingJob.JobStatus)
					require.Equal(t, clock.Now().UTC(), pendingJob.CanceledAt.Time.UTC())
					require.Equal(t, clock.Now().UTC(), pendingJob.CompletedAt.Time.UTC())

					// Workspace should be deleted
					deletedWorkspace, err := db.GetWorkspaceByID(ctx, prebuild.Workspace.ID)
					require.NoError(t, err)
					require.True(t, deletedWorkspace.Deleted)
					latestBuild, err := db.GetLatestWorkspaceBuildByWorkspaceID(ctx, deletedWorkspace.ID)
					require.NoError(t, err)
					require.Equal(t, database.WorkspaceTransitionDelete, latestBuild.Transition)
					deleteJob, err := db.GetProvisionerJobByID(ctx, latestBuild.JobID)
					require.NoError(t, err)
					require.True(t, deleteJob.CompletedAt.Valid)
					require.False(t, deleteJob.WorkerID.Valid)
					require.Equal(t, database.ProvisionerJobStatusSucceeded, deleteJob.JobStatus)
				} else {
					// Pending job should not be canceled
					require.NotEqual(t, database.ProvisionerJobStatusCanceled, pendingJob.JobStatus)
					require.Zero(t, pendingJob.CanceledAt.Time.UTC())

					// Workspace should not be deleted
					workspace, err := db.GetWorkspaceByID(ctx, prebuild.Workspace.ID)
					require.NoError(t, err)
					require.False(t, workspace.Deleted)
				}
			}
		}

		// Set the clock to Monday, January 1st, 2024 at 8:00 AM UTC to keep the test deterministic
		clock := quartz.NewMock(t)
		clock.Set(time.Date(2024, 1, 1, 8, 0, 0, 0, time.UTC))

		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
		defer cancel()

		// Setup
		db, ps := dbtestutil.NewDB(t)
		client, _, _ := coderdtest.NewWithAPI(t, &coderdtest.Options{
			// Explicitly not including provisioner daemons, as we don't want the jobs to be processed
			// Jobs operations will be simulated via the database model
			IncludeProvisionerDaemon: false,
			Database:                 db,
			Pubsub:                   ps,
			Clock:                    clock,
		})
		fakeEnqueuer := newFakeEnqueuer()
		registry := prometheus.NewRegistry()
		cache := files.New(registry, &coderdtest.FakeAuthorizer{})
		logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: false}).Leveled(slog.LevelDebug)
		reconciler := prebuilds.NewStoreReconciler(db, ps, cache, codersdk.PrebuildsConfig{}, logger, clock, registry, fakeEnqueuer, newNoopUsageCheckerPtr())
		owner := coderdtest.CreateFirstUser(t, client)

		// Given: template A with 2 versions
		// Given: template A version v1: with a preset with 5 instances (2 running, 3 pending)
		templateAID, templateAVersion1ID, templateAVersion1PresetID := createTemplateVersionWithPreset(t, db, owner.OrganizationID, owner.UserID, uuid.Nil, 5)
		templateAVersion1Running := setupPrebuilds(t, db, owner.OrganizationID, templateAID, templateAVersion1ID, templateAVersion1PresetID, 2, false)
		templateAVersion1Pending := setupPrebuilds(t, db, owner.OrganizationID, templateAID, templateAVersion1ID, templateAVersion1PresetID, 3, true)
		// Given: template A version v2 (active version): with a preset with 2 instances (1 running, 1 pending)
		_, templateAVersion2ID, templateAVersion2PresetID := createTemplateVersionWithPreset(t, db, owner.OrganizationID, owner.UserID, templateAID, 2)
		templateAVersion2Running := setupPrebuilds(t, db, owner.OrganizationID, templateAID, templateAVersion2ID, templateAVersion2PresetID, 1, false)
		templateAVersion2Pending := setupPrebuilds(t, db, owner.OrganizationID, templateAID, templateAVersion2ID, templateAVersion2PresetID, 1, true)

		// Given: template B with 3 versions
		// Given: template B version v1: with a preset with 3 instances (1 running, 2 pending)
		templateBID, templateBVersion1ID, templateBVersion1PresetID := createTemplateVersionWithPreset(t, db, owner.OrganizationID, owner.UserID, uuid.Nil, 3)
		templateBVersion1Running := setupPrebuilds(t, db, owner.OrganizationID, templateBID, templateBVersion1ID, templateBVersion1PresetID, 1, false)
		templateBVersion1Pending := setupPrebuilds(t, db, owner.OrganizationID, templateBID, templateBVersion1ID, templateBVersion1PresetID, 2, true)
		// Given: template B version v2: with a preset with 2 instances (2 pending)
		_, templateBVersion2ID, templateBVersion2PresetID := createTemplateVersionWithPreset(t, db, owner.OrganizationID, owner.UserID, templateBID, 2)
		templateBVersion2Pending := setupPrebuilds(t, db, owner.OrganizationID, templateBID, templateBVersion2ID, templateBVersion2PresetID, 2, true)
		// Given: template B version v3 (active version): with a preset with 2 instances (1 running, 1 pending)
		_, templateBVersion3ID, templateBVersion3PresetID := createTemplateVersionWithPreset(t, db, owner.OrganizationID, owner.UserID, templateBID, 2)
		templateBVersion3Running := setupPrebuilds(t, db, owner.OrganizationID, templateBID, templateBVersion3ID, templateBVersion3PresetID, 1, false)
		templateBVersion3Pending := setupPrebuilds(t, db, owner.OrganizationID, templateBID, templateBVersion3ID, templateBVersion3PresetID, 1, true)

		// When: the reconciliation loop is executed
		_, err := reconciler.ReconcileAll(ctx)
		require.NoError(t, err)

		// Then: template A version 1 running workspaces should not be canceled
		checkIfJobCanceledAndDeleted(t, clock, ctx, db, false, templateAVersion1Running)
		// Then: template A version 1 pending workspaces should be canceled
		checkIfJobCanceledAndDeleted(t, clock, ctx, db, true, templateAVersion1Pending)
		// Then: template A version 2 running and pending workspaces should not be canceled
		checkIfJobCanceledAndDeleted(t, clock, ctx, db, false, templateAVersion2Running)
		checkIfJobCanceledAndDeleted(t, clock, ctx, db, false, templateAVersion2Pending)

		// Then: template B version 1 running workspaces should not be canceled
		checkIfJobCanceledAndDeleted(t, clock, ctx, db, false, templateBVersion1Running)
		// Then: template B version 1 pending workspaces should be canceled
		checkIfJobCanceledAndDeleted(t, clock, ctx, db, true, templateBVersion1Pending)
		// Then: template B version 2 pending workspaces should be canceled
		checkIfJobCanceledAndDeleted(t, clock, ctx, db, true, templateBVersion2Pending)
		// Then: template B version 3 running and pending workspaces should not be canceled
		checkIfJobCanceledAndDeleted(t, clock, ctx, db, false, templateBVersion3Running)
		checkIfJobCanceledAndDeleted(t, clock, ctx, db, false, templateBVersion3Pending)
	})
}

func TestReconciliationStats(t *testing.T) {
	t.Parallel()

	// Setup
	clock := quartz.NewReal()
	db, ps := dbtestutil.NewDB(t)
	client, _, _ := coderdtest.NewWithAPI(t, &coderdtest.Options{
		Database: db,
		Pubsub:   ps,
		Clock:    clock,
	})
	fakeEnqueuer := newFakeEnqueuer()
	registry := prometheus.NewRegistry()
	cache := files.New(registry, &coderdtest.FakeAuthorizer{})
	logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: false}).Leveled(slog.LevelDebug)
	reconciler := prebuilds.NewStoreReconciler(db, ps, cache, codersdk.PrebuildsConfig{}, logger, clock, registry, fakeEnqueuer, newNoopUsageCheckerPtr())
	owner := coderdtest.CreateFirstUser(t, client)

	ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitShort)
	defer cancel()

	// Create a template version with a preset
	dbfake.TemplateVersion(t, db).Seed(database.TemplateVersion{
		OrganizationID: owner.OrganizationID,
		CreatedBy:      owner.UserID,
	}).Preset(database.TemplateVersionPreset{
		DesiredInstances: sql.NullInt32{
			Int32: 1,
			Valid: true,
		},
	}).Do()

	// Verify that ReconcileAll tracks and returns elapsed time
	start := time.Now()
	stats, err := reconciler.ReconcileAll(ctx)
	actualElapsed := time.Since(start)
	require.NoError(t, err)
	require.Greater(t, stats.Elapsed, time.Duration(0))

	// Verify stats.Elapsed matches actual execution time
	require.InDelta(t, actualElapsed.Milliseconds(), stats.Elapsed.Milliseconds(), 100)
	// Verify reconciliation loop is not unexpectedly slow
	require.Less(t, stats.Elapsed, 5*time.Second)
}

func newNoopEnqueuer() *notifications.NoopEnqueuer {
	return notifications.NewNoopEnqueuer()
}

func newFakeEnqueuer() *notificationstest.FakeEnqueuer {
	return notificationstest.NewFakeEnqueuer()
}

func newNoopUsageCheckerPtr() *atomic.Pointer[wsbuilder.UsageChecker] {
	var noopUsageChecker wsbuilder.UsageChecker = wsbuilder.NoopUsageChecker{}
	buildUsageChecker := atomic.Pointer[wsbuilder.UsageChecker]{}
	buildUsageChecker.Store(&noopUsageChecker)
	return &buildUsageChecker
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

	ctx := testutil.Context(t, testutil.WaitLong)
	clock := quartz.NewMock(t)
	db, ps := dbtestutil.NewDB(t)
	cfg := codersdk.PrebuildsConfig{
		ReconciliationInterval: serpent.Duration(testutil.WaitLong),
	}
	logger := testutil.Logger(t)
	cache := files.New(prometheus.NewRegistry(), &coderdtest.FakeAuthorizer{})
	reconciler := prebuilds.NewStoreReconciler(db, ps, cache, cfg, logger, clock, prometheus.NewRegistry(), newNoopEnqueuer(), newNoopUsageCheckerPtr())

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
	_, err := reconciler.ReconcileAll(ctx)
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
	_, err = reconciler.ReconcileAll(ctx)
	require.NoError(t, err)

	// Verify that no new prebuilds were created because reconciliation is paused
	workspaces, err = db.GetWorkspacesByTemplateID(ctx, template.ID)
	require.NoError(t, err)
	require.Len(t, workspaces, 0, "should not create prebuilds when reconciliation is paused")

	// Resume prebuilds reconciliation
	err = prebuilds.SetPrebuildsReconciliationPaused(ctx, db, false)
	require.NoError(t, err)

	// Run reconciliation again - it should now recreate the prebuilds
	_, err = reconciler.ReconcileAll(ctx)
	require.NoError(t, err)

	// Verify that prebuilds were recreated
	workspaces, err = db.GetWorkspacesByTemplateID(ctx, template.ID)
	require.NoError(t, err)
	require.Len(t, workspaces, 2, "should have recreated 2 prebuilds after resuming")
}

func TestIncrementalBackoffOnCreationFailure(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitLong)
	clock := quartz.NewMock(t)
	db, ps := dbtestutil.NewDB(t)
	backoffInterval := 1 * time.Minute
	cfg := codersdk.PrebuildsConfig{
		ReconciliationInterval:        serpent.Duration(testutil.WaitLong),
		ReconciliationBackoffInterval: serpent.Duration(backoffInterval),
	}
	logger := slogtest.Make(t, nil)
	cache := files.New(prometheus.NewRegistry(), &coderdtest.FakeAuthorizer{})
	reconciler := prebuilds.NewStoreReconciler(db, ps, cache, cfg, logger, clock, prometheus.NewRegistry(), newNoopEnqueuer(), newNoopUsageCheckerPtr())

	// Setup a template with a preset
	org := dbgen.Organization(t, db, database.Organization{})
	user := dbgen.User(t, db, database.User{})
	template := dbgen.Template(t, db, database.Template{
		CreatedBy:      user.ID,
		OrganizationID: org.ID,
	})
	templateVersionID := setupTestDBTemplateVersion(ctx, t, clock, db, ps, org.ID, user.ID, template.ID)
	presetID := setupTestDBPreset(t, db, templateVersionID, 1, "test").ID

	// Test the backoff mechanism directly by simulating failures
	// First failure
	reconciler.RecordCreationFailure(presetID)

	// Check that backoff is active
	shouldBackoff, backoffUntil := reconciler.ShouldBackoffCreation(presetID)
	require.True(t, shouldBackoff, "should be in backoff after first failure")
	expectedBackoff := clock.Now().Add(backoffInterval)
	require.Equal(t, expectedBackoff, backoffUntil, "backoff should be 1x interval after first failure")

	// Advance clock past first backoff
	clock.Advance(backoffInterval + time.Second)

	// Should no longer be in backoff
	shouldBackoff, _ = reconciler.ShouldBackoffCreation(presetID)
	require.False(t, shouldBackoff, "should not be in backoff after period expires")

	// Second consecutive failure
	reconciler.RecordCreationFailure(presetID)

	// Check that backoff is longer now (2 * interval)
	shouldBackoff, backoffUntil = reconciler.ShouldBackoffCreation(presetID)
	require.True(t, shouldBackoff, "should be in backoff after second failure")
	expectedBackoff = clock.Now().Add(2 * backoffInterval)
	require.Equal(t, expectedBackoff, backoffUntil, "backoff should be 2x interval after second failure")

	// Advance clock by only 1 interval - should still be in backoff
	clock.Advance(backoffInterval)
	shouldBackoff, _ = reconciler.ShouldBackoffCreation(presetID)
	require.True(t, shouldBackoff, "should still be in backoff after 1 interval with 2 failures")

	// Advance clock by another interval - backoff should expire
	clock.Advance(backoffInterval + time.Second)
	shouldBackoff, _ = reconciler.ShouldBackoffCreation(presetID)
	require.False(t, shouldBackoff, "should not be in backoff after 2 intervals expire")

	// Third consecutive failure
	reconciler.RecordCreationFailure(presetID)

	// Check that backoff is even longer now (3 * interval)
	shouldBackoff, backoffUntil = reconciler.ShouldBackoffCreation(presetID)
	require.True(t, shouldBackoff, "should be in backoff after third failure")
	expectedBackoff = clock.Now().Add(3 * backoffInterval)
	require.Equal(t, expectedBackoff, backoffUntil, "backoff should be 3x interval after third failure")

	// Successful creation should clear the backoff
	reconciler.RecordCreationSuccess(presetID)
	shouldBackoff, _ = reconciler.ShouldBackoffCreation(presetID)
	require.False(t, shouldBackoff, "should not be in backoff after successful creation")

	// New failure after success should start backoff from 1x interval again
	reconciler.RecordCreationFailure(presetID)
	shouldBackoff, backoffUntil = reconciler.ShouldBackoffCreation(presetID)
	require.True(t, shouldBackoff, "should be in backoff after failure following success")
	expectedBackoff = clock.Now().Add(backoffInterval)
	require.Equal(t, expectedBackoff, backoffUntil, "backoff should reset to 1x interval after success")
}

func TestHardFailureLimitTracking(t *testing.T) {
	// This test verifies that failed prebuild attempts are correctly tracked
	// in the database and counted by GetPresetsAtFailureLimit.
	// Similar to TestIncrementalBackoffOnCreationFailure, this test manually
	// creates the database state rather than running the full reconciliation.
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitLong)
	ctx = dbauthz.AsSystemRestricted(ctx)
	clock := quartz.NewMock(t)
	db, ps := dbtestutil.NewDB(t)

	// Setup template with preset
	org := dbgen.Organization(t, db, database.Organization{})
	user := dbgen.User(t, db, database.User{})
	template := dbgen.Template(t, db, database.Template{
		CreatedBy:      user.ID,
		OrganizationID: org.ID,
	})
	templateVersionID := setupTestDBTemplateVersion(ctx, t, clock, db, ps, org.ID, user.ID, template.ID)
	preset := setupTestDBPreset(t, db, templateVersionID, 3, "test-preset")

	// Get the template version for provisioner job setup
	templateVersion, err := db.GetTemplateVersionByID(ctx, templateVersionID)
	require.NoError(t, err)
	templateVersionJob, err := db.GetProvisionerJobByID(ctx, templateVersion.JobID)
	require.NoError(t, err)

	// Helper to create a failed prebuild workspace build
	createFailedPrebuild := func(buildNum int) {
		// Create workspace for this prebuild
		workspace := dbgen.Workspace(t, db, database.Workspace{
			TemplateID:     template.ID,
			OrganizationID: org.ID,
			OwnerID:        database.PrebuildsSystemUserID,
			Name:           fmt.Sprintf("prebuild-%d-%d", preset.ID, buildNum),
		})

		// Create failed provisioner job
		now := clock.Now()
		job, err := db.InsertProvisionerJob(ctx, database.InsertProvisionerJobParams{
			ID:             uuid.New(),
			CreatedAt:      now,
			UpdatedAt:      now,
			InitiatorID:    database.PrebuildsSystemUserID,
			OrganizationID: org.ID,
			Provisioner:    template.Provisioner,
			Type:           database.ProvisionerJobTypeWorkspaceBuild,
			StorageMethod:  templateVersionJob.StorageMethod,
			FileID:         templateVersionJob.FileID,
			Input:          []byte("{}"),
			Tags:           database.StringMap{},
		})
		require.NoError(t, err)

		// Mark job as failed - this sets job_status to 'failed' via generated column
		err = db.UpdateProvisionerJobWithCompleteByID(ctx, database.UpdateProvisionerJobWithCompleteByIDParams{
			ID:          job.ID,
			UpdatedAt:   now,
			CompletedAt: sql.NullTime{Valid: true, Time: now},
			Error:       sql.NullString{Valid: true, String: fmt.Sprintf("config error: missing required param (build %d)", buildNum)},
		})
		require.NoError(t, err)

		// Create workspace build linking to failed job
		workspaceBuildID := uuid.New()
		err = db.InsertWorkspaceBuild(ctx, database.InsertWorkspaceBuildParams{
			ID:                workspaceBuildID,
			CreatedAt:         now,
			UpdatedAt:         now,
			WorkspaceID:       workspace.ID,
			TemplateVersionID: templateVersionID,
			BuildNumber:       int32(buildNum),
			ProvisionerState:  []byte("[]"),
			InitiatorID:       database.PrebuildsSystemUserID,
			Transition:        database.WorkspaceTransitionStart,
			JobID:             job.ID,
			Reason:            database.BuildReasonInitiator,
			TemplateVersionPresetID: uuid.NullUUID{
				UUID:  preset.ID,
				Valid: true,
			},
		})
		require.NoError(t, err)

		// Verify the job has failed status
		verifyJob, err := db.GetProvisionerJobByID(ctx, job.ID)
		require.NoError(t, err)
		require.Equal(t, database.ProvisionerJobStatusFailed, verifyJob.JobStatus, "job_status should be failed")
	}

	// Test 1: Create one failed build, should NOT hit hard limit
	createFailedPrebuild(1)

	presetsAtLimit, err := db.GetPresetsAtFailureLimit(ctx, 3)
	require.NoError(t, err)
	require.Empty(t, presetsAtLimit, "preset should not hit hard limit after 1 failure (limit is 3)")

	// Test 2: Create second failed build, still should NOT hit limit
	createFailedPrebuild(2)

	presetsAtLimit, err = db.GetPresetsAtFailureLimit(ctx, 3)
	require.NoError(t, err)
	require.Empty(t, presetsAtLimit, "preset should not hit hard limit after 2 failures (limit is 3)")

	// Test 3: Create third failed build, should NOW hit hard limit
	createFailedPrebuild(3)

	presetsAtLimit, err = db.GetPresetsAtFailureLimit(ctx, 3)
	require.NoError(t, err)
	require.Len(t, presetsAtLimit, 1, "preset should hit hard limit after 3 consecutive failures")
	require.Equal(t, preset.ID, presetsAtLimit[0].PresetID, "correct preset should be at failure limit")

	// Test 4: Verify lower limit also catches it
	presetsAtLimit, err = db.GetPresetsAtFailureLimit(ctx, 2)
	require.NoError(t, err)
	require.Len(t, presetsAtLimit, 1, "preset should also hit limit=2 with 3 failures")

	// This test validates that our database schema correctly tracks failed
	// builds and the GetPresetsAtFailureLimit query accurately identifies
	// presets that have hit the failure threshold.
}

func TestConfigErrorCreatesFailedBuildRecord(t *testing.T) {
	// This test verifies that when createPrebuiltWorkspace encounters a config error
	// (HTTP 400-level error from wsbuilder.Build), it creates a failed build record
	// in the database so the error counts toward the hard failure limit.
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitLong)
	ctx = dbauthz.AsPrebuildsOrchestrator(ctx)
	clock := quartz.NewMock(t)
	db, ps := dbtestutil.NewDB(t)
	cfg := codersdk.PrebuildsConfig{
		ReconciliationInterval:        serpent.Duration(testutil.WaitLong),
		ReconciliationBackoffInterval: serpent.Duration(1 * time.Minute),
	}
	logger := slogtest.Make(t, nil)
	cache := files.New(prometheus.NewRegistry(), &coderdtest.FakeAuthorizer{})
	reconciler := prebuilds.NewStoreReconciler(db, ps, cache, cfg, logger, clock, prometheus.NewRegistry(), newNoopEnqueuer(), newNoopUsageCheckerPtr())

	// Setup template with a preset that has required mutable parameters.
	// This will cause wsbuilder.Build to fail with a BadRequest error when
	// the preset doesn't provide values for required mutable parameters.
	org := dbgen.Organization(t, db, database.Organization{})
	user := dbgen.User(t, db, database.User{})
	template := dbgen.Template(t, db, database.Template{
		CreatedBy:      user.ID,
		OrganizationID: org.ID,
	})

	// Create a template version with a required mutable parameter
	templateVersionJob := dbgen.ProvisionerJob(t, db, ps, database.ProvisionerJob{
		CreatedAt:      clock.Now().Add(muchEarlier),
		CompletedAt:    sql.NullTime{Time: clock.Now().Add(earlier), Valid: true},
		OrganizationID: org.ID,
		InitiatorID:    user.ID,
	})
	templateVersion := dbgen.TemplateVersion(t, db, database.TemplateVersion{
		TemplateID:     uuid.NullUUID{UUID: template.ID, Valid: true},
		OrganizationID: org.ID,
		CreatedBy:      user.ID,
		JobID:          templateVersionJob.ID,
		CreatedAt:      clock.Now().Add(muchEarlier),
	})
	require.NoError(t, db.UpdateTemplateActiveVersionByID(ctx, database.UpdateTemplateActiveVersionByIDParams{
		ID:              template.ID,
		ActiveVersionID: templateVersion.ID,
	}))

	// Add a required mutable parameter - this will cause validation to fail
	// when the preset doesn't provide a value
	dbgen.TemplateVersionParameter(t, db, database.TemplateVersionParameter{
		TemplateVersionID: templateVersion.ID,
		Name:              "required_param",
		Type:              "string",
		Required:          true,
		Mutable:           true,
		DefaultValue:      "",
	})

	// Create preset without providing the required parameter
	preset := setupTestDBPreset(t, db, templateVersion.ID, 1, "test-preset")

	// Get initial workspace count
	workspacesBefore, err := db.GetWorkspacesByTemplateID(ctx, template.ID)
	require.NoError(t, err)
	initialWorkspaceCount := len(workspacesBefore)

	// Run reconciliation - this should attempt to create a prebuild, fail with config error,
	// and create a failed build record
	_, err = reconciler.ReconcileAll(ctx)
	require.NoError(t, err, "reconciliation should complete even if prebuild creation fails")

	// Verify a workspace was created (even though build failed)
	workspacesAfter, err := db.GetWorkspacesByTemplateID(ctx, template.ID)
	require.NoError(t, err)
	require.Equal(t, initialWorkspaceCount+1, len(workspacesAfter), "should have created one workspace")

	// Find the new workspace
	var newWorkspaceID uuid.UUID
	for _, ws := range workspacesAfter {
		found := false
		for _, oldWs := range workspacesBefore {
			if ws.ID == oldWs.ID {
				found = true
				break
			}
		}
		if !found {
			newWorkspaceID = ws.ID
			break
		}
	}
	require.NotEqual(t, uuid.Nil, newWorkspaceID, "should have found new workspace")

	// Verify a failed build record was created
	build, err := db.GetLatestWorkspaceBuildByWorkspaceID(ctx, newWorkspaceID)
	require.NoError(t, err)
	require.Equal(t, database.WorkspaceTransitionStart, build.Transition, "build should be start transition")
	require.Equal(t, preset.ID, build.TemplateVersionPresetID.UUID, "build should reference preset")

	// Verify the provisioner job exists and is marked as failed
	job, err := db.GetProvisionerJobByID(ctx, build.JobID)
	require.NoError(t, err)
	require.True(t, job.CompletedAt.Valid, "job should be completed")
	require.True(t, job.Error.Valid, "job should have error set")
	require.NotEmpty(t, job.Error.String, "job error message should not be empty")
	require.Contains(t, job.Error.String, "required_param", "error should mention the missing parameter")

	// Most importantly: verify job_status is 'failed' (this is what counts toward hard limit)
	// job_status is a generated column that becomes 'failed' when completed_at is set and error is non-empty
	require.Equal(t, database.ProvisionerJobStatusFailed, job.JobStatus, "job status should be failed")

	// Verify this failure would be counted by GetPresetsAtFailureLimit query
	// The query looks at workspace_latest_builds view which includes prebuilds with failed job_status
	presetsAtLimit, err := db.GetPresetsAtFailureLimit(ctx, 1)
	require.NoError(t, err)

	// Check if our preset appears in the list (it should after 1 failure)
	foundPreset := false
	for _, p := range presetsAtLimit {
		if p.PresetID == preset.ID {
			foundPreset = true
			break
		}
	}
	require.True(t, foundPreset, "preset should appear in failure limit list after config error")
}
