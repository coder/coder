package prebuilds_test

import (
	"database/sql"
	"fmt"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"cdr.dev/slog/sloggers/slogtest"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/prebuilds"
	"github.com/coder/coder/v2/testutil"
	"github.com/coder/quartz"
)

type options struct {
	templateID          uuid.UUID
	templateVersionID   uuid.UUID
	presetID            uuid.UUID
	presetName          string
	prebuiltWorkspaceID uuid.UUID
	workspaceName       string
	ttl                 int32
}

// templateID is common across all option sets.
var templateID = uuid.UUID{1}

const (
	backoffInterval = time.Second * 5

	optionSet0 = iota
	optionSet1
	optionSet2
	optionSet3
)

var opts = map[uint]options{
	optionSet0: {
		templateID:          templateID,
		templateVersionID:   uuid.UUID{11},
		presetID:            uuid.UUID{12},
		presetName:          "my-preset",
		prebuiltWorkspaceID: uuid.UUID{13},
		workspaceName:       "prebuilds0",
	},
	optionSet1: {
		templateID:          templateID,
		templateVersionID:   uuid.UUID{21},
		presetID:            uuid.UUID{22},
		presetName:          "my-preset",
		prebuiltWorkspaceID: uuid.UUID{23},
		workspaceName:       "prebuilds1",
	},
	optionSet2: {
		templateID:          templateID,
		templateVersionID:   uuid.UUID{31},
		presetID:            uuid.UUID{32},
		presetName:          "my-preset",
		prebuiltWorkspaceID: uuid.UUID{33},
		workspaceName:       "prebuilds2",
	},
	optionSet3: {
		templateID:          templateID,
		templateVersionID:   uuid.UUID{41},
		presetID:            uuid.UUID{42},
		presetName:          "my-preset",
		prebuiltWorkspaceID: uuid.UUID{43},
		workspaceName:       "prebuilds3",
		ttl:                 5, // seconds
	},
}

// A new template version with a preset without prebuilds configured should result in no prebuilds being created.
func TestNoPrebuilds(t *testing.T) {
	t.Parallel()
	current := opts[optionSet0]
	clock := quartz.NewMock(t)

	presets := []database.GetTemplatePresetsWithPrebuildsRow{
		preset(true, 0, current),
	}

	snapshot := prebuilds.NewGlobalSnapshot(presets, nil, nil, nil, nil, nil, nil, clock, testutil.Logger(t))
	ps, err := snapshot.FilterByPreset(current.presetID)
	require.NoError(t, err)

	state := ps.CalculateState()
	actions, err := ps.CalculateActions(backoffInterval)
	require.NoError(t, err)

	validateState(t, prebuilds.ReconciliationState{ /*all zero values*/ }, *state)
	validateActions(t, nil, actions)
}

// A new template version with a preset with prebuilds configured should result in a new prebuild being created.
func TestNetNew(t *testing.T) {
	t.Parallel()
	current := opts[optionSet0]
	clock := quartz.NewMock(t)

	presets := []database.GetTemplatePresetsWithPrebuildsRow{
		preset(true, 1, current),
	}

	snapshot := prebuilds.NewGlobalSnapshot(presets, nil, nil, nil, nil, nil, nil, clock, testutil.Logger(t))
	ps, err := snapshot.FilterByPreset(current.presetID)
	require.NoError(t, err)

	state := ps.CalculateState()
	actions, err := ps.CalculateActions(backoffInterval)
	require.NoError(t, err)

	validateState(t, prebuilds.ReconciliationState{
		Desired: 1,
	}, *state)
	validateActions(t, []*prebuilds.ReconciliationActions{
		{
			ActionType: prebuilds.ActionTypeCreate,
			Create:     1,
		},
	}, actions)
}

// A new template version is created with a preset with prebuilds configured; this outdates the older version and
// requires the old prebuilds to be destroyed and new prebuilds to be created.
func TestOutdatedPrebuilds(t *testing.T) {
	t.Parallel()
	outdated := opts[optionSet0]
	current := opts[optionSet1]
	clock := quartz.NewMock(t)

	// GIVEN: 2 presets, one outdated and one new.
	presets := []database.GetTemplatePresetsWithPrebuildsRow{
		preset(false, 1, outdated),
		preset(true, 1, current),
	}

	// GIVEN: a running prebuild for the outdated preset.
	running := []database.GetRunningPrebuiltWorkspacesRow{
		prebuiltWorkspace(outdated, clock),
	}

	// GIVEN: no in-progress builds.
	var inProgress []database.CountInProgressPrebuildsRow

	// WHEN: calculating the outdated preset's state.
	snapshot := prebuilds.NewGlobalSnapshot(presets, nil, running, inProgress, nil, nil, nil, quartz.NewMock(t), testutil.Logger(t))
	ps, err := snapshot.FilterByPreset(outdated.presetID)
	require.NoError(t, err)

	// THEN: we should identify that this prebuild is outdated and needs to be deleted.
	state := ps.CalculateState()
	actions, err := ps.CalculateActions(backoffInterval)
	require.NoError(t, err)
	validateState(t, prebuilds.ReconciliationState{
		Actual: 1,
	}, *state)
	validateActions(t, []*prebuilds.ReconciliationActions{
		{
			ActionType: prebuilds.ActionTypeDelete,
			DeleteIDs:  []uuid.UUID{outdated.prebuiltWorkspaceID},
		},
	}, actions)

	// WHEN: calculating the current preset's state.
	ps, err = snapshot.FilterByPreset(current.presetID)
	require.NoError(t, err)

	// THEN: we should not be blocked from creating a new prebuild while the outdate one deletes.
	state = ps.CalculateState()
	actions, err = ps.CalculateActions(backoffInterval)
	require.NoError(t, err)
	validateState(t, prebuilds.ReconciliationState{Desired: 1}, *state)
	validateActions(t, []*prebuilds.ReconciliationActions{
		{
			ActionType: prebuilds.ActionTypeCreate,
			Create:     1,
		},
	}, actions)
}

// Make sure that outdated prebuild will be deleted, even if deletion of another outdated prebuild is already in progress.
func TestDeleteOutdatedPrebuilds(t *testing.T) {
	t.Parallel()
	outdated := opts[optionSet0]
	clock := quartz.NewMock(t)

	// GIVEN: 1 outdated preset.
	presets := []database.GetTemplatePresetsWithPrebuildsRow{
		preset(false, 1, outdated),
	}

	// GIVEN: one running prebuild for the outdated preset.
	running := []database.GetRunningPrebuiltWorkspacesRow{
		prebuiltWorkspace(outdated, clock),
	}

	// GIVEN: one deleting prebuild for the outdated preset.
	inProgress := []database.CountInProgressPrebuildsRow{
		{
			TemplateID:        outdated.templateID,
			TemplateVersionID: outdated.templateVersionID,
			Transition:        database.WorkspaceTransitionDelete,
			Count:             1,
			PresetID: uuid.NullUUID{
				UUID:  outdated.presetID,
				Valid: true,
			},
		},
	}

	// WHEN: calculating the outdated preset's state.
	snapshot := prebuilds.NewGlobalSnapshot(presets, nil, running, inProgress, nil, nil, nil, quartz.NewMock(t), testutil.Logger(t))
	ps, err := snapshot.FilterByPreset(outdated.presetID)
	require.NoError(t, err)

	// THEN: we should identify that this prebuild is outdated and needs to be deleted.
	// Despite the fact that deletion of another outdated prebuild is already in progress.
	state := ps.CalculateState()
	actions, err := ps.CalculateActions(backoffInterval)
	require.NoError(t, err)
	validateState(t, prebuilds.ReconciliationState{
		Actual:   1,
		Deleting: 1,
	}, *state)

	validateActions(t, []*prebuilds.ReconciliationActions{
		{
			ActionType: prebuilds.ActionTypeDelete,
			DeleteIDs:  []uuid.UUID{outdated.prebuiltWorkspaceID},
		},
	}, actions)
}

func TestCancelPendingPrebuilds(t *testing.T) {
	t.Parallel()

	// Setup
	current := opts[optionSet3]
	clock := quartz.NewMock(t)

	t.Run("CancelPendingPrebuildsNonActiveVersion", func(t *testing.T) {
		t.Parallel()

		// Given: a preset from a non-active version
		defaultPreset := preset(false, 0, current)
		presets := []database.GetTemplatePresetsWithPrebuildsRow{
			defaultPreset,
		}

		// Given: 2 pending prebuilt workspaces for the preset
		pending := []database.CountPendingNonActivePrebuildsRow{{
			PresetID: uuid.NullUUID{
				UUID:  defaultPreset.ID,
				Valid: true,
			},
			Count: 2,
		}}

		// When: calculating the current preset's state
		snapshot := prebuilds.NewGlobalSnapshot(presets, nil, nil, nil, pending, nil, nil, clock, testutil.Logger(t))
		ps, err := snapshot.FilterByPreset(current.presetID)
		require.NoError(t, err)

		// Then: it should create a cancel reconciliation action
		actions, err := ps.CalculateActions(backoffInterval)
		require.NoError(t, err)
		expectedAction := []*prebuilds.ReconciliationActions{{ActionType: prebuilds.ActionTypeCancelPending}}
		require.Equal(t, expectedAction, actions)
	})

	t.Run("NotCancelPendingPrebuildsActiveVersion", func(t *testing.T) {
		t.Parallel()

		// Given: a preset from an active version
		defaultPreset := preset(true, 0, current)
		presets := []database.GetTemplatePresetsWithPrebuildsRow{
			defaultPreset,
		}

		// Given: 2 pending prebuilt workspaces for the preset
		pending := []database.CountPendingNonActivePrebuildsRow{{
			PresetID: uuid.NullUUID{
				UUID:  defaultPreset.ID,
				Valid: true,
			},
			Count: 2,
		}}

		// When: calculating the current preset's state
		snapshot := prebuilds.NewGlobalSnapshot(presets, nil, nil, nil, pending, nil, nil, clock, testutil.Logger(t))
		ps, err := snapshot.FilterByPreset(current.presetID)
		require.NoError(t, err)

		// Then: it should not create a cancel reconciliation action
		actions, err := ps.CalculateActions(backoffInterval)
		require.NoError(t, err)
		var expectedAction []*prebuilds.ReconciliationActions
		require.Equal(t, expectedAction, actions)
	})
}

// A new template version is created with a preset with prebuilds configured; while a prebuild is provisioning up or down,
// the calculated actions should indicate the state correctly.
func TestInProgressActions(t *testing.T) {
	t.Parallel()
	current := opts[optionSet0]
	clock := quartz.NewMock(t)

	cases := []struct {
		name       string
		transition database.WorkspaceTransition
		desired    int32
		running    int32
		inProgress int32
		checkFn    func(state prebuilds.ReconciliationState, actions []*prebuilds.ReconciliationActions)
	}{
		// With no running prebuilds and one starting, no creations/deletions should take place.
		{
			name:       fmt.Sprintf("%s-short", database.WorkspaceTransitionStart),
			transition: database.WorkspaceTransitionStart,
			desired:    1,
			running:    0,
			inProgress: 1,
			checkFn: func(state prebuilds.ReconciliationState, actions []*prebuilds.ReconciliationActions) {
				validateState(t, prebuilds.ReconciliationState{Desired: 1, Starting: 1}, state)
				validateActions(t, nil, actions)
			},
		},
		// With one running prebuild and one starting, no creations/deletions should occur since we're approaching the correct state.
		{
			name:       fmt.Sprintf("%s-balanced", database.WorkspaceTransitionStart),
			transition: database.WorkspaceTransitionStart,
			desired:    2,
			running:    1,
			inProgress: 1,
			checkFn: func(state prebuilds.ReconciliationState, actions []*prebuilds.ReconciliationActions) {
				validateState(t, prebuilds.ReconciliationState{Actual: 1, Desired: 2, Starting: 1}, state)
				validateActions(t, nil, actions)
			},
		},
		// With one running prebuild and one starting, no creations/deletions should occur
		// SIDE-NOTE: once the starting prebuild completes, the older of the two will be considered extraneous since we only desire 2.
		{
			name:       fmt.Sprintf("%s-extraneous", database.WorkspaceTransitionStart),
			transition: database.WorkspaceTransitionStart,
			desired:    2,
			running:    2,
			inProgress: 1,
			checkFn: func(state prebuilds.ReconciliationState, actions []*prebuilds.ReconciliationActions) {
				validateState(t, prebuilds.ReconciliationState{Actual: 2, Desired: 2, Starting: 1}, state)
				validateActions(t, nil, actions)
			},
		},
		// With one prebuild desired and one stopping, a new prebuild will be created.
		{
			name:       fmt.Sprintf("%s-short", database.WorkspaceTransitionStop),
			transition: database.WorkspaceTransitionStop,
			desired:    1,
			running:    0,
			inProgress: 1,
			checkFn: func(state prebuilds.ReconciliationState, actions []*prebuilds.ReconciliationActions) {
				validateState(t, prebuilds.ReconciliationState{Desired: 1, Stopping: 1}, state)
				validateActions(t, []*prebuilds.ReconciliationActions{
					{
						ActionType: prebuilds.ActionTypeCreate,
						Create:     1,
					},
				}, actions)
			},
		},
		// With 3 prebuilds desired, 2 running, and 1 stopping, a new prebuild will be created.
		{
			name:       fmt.Sprintf("%s-balanced", database.WorkspaceTransitionStop),
			transition: database.WorkspaceTransitionStop,
			desired:    3,
			running:    2,
			inProgress: 1,
			checkFn: func(state prebuilds.ReconciliationState, actions []*prebuilds.ReconciliationActions) {
				validateState(t, prebuilds.ReconciliationState{Actual: 2, Desired: 3, Stopping: 1}, state)
				validateActions(t, []*prebuilds.ReconciliationActions{
					{
						ActionType: prebuilds.ActionTypeCreate,
						Create:     1,
					},
				}, actions)
			},
		},
		// With 3 prebuilds desired, 3 running, and 1 stopping, no creations/deletions should occur since the desired state is already achieved.
		{
			name:       fmt.Sprintf("%s-extraneous", database.WorkspaceTransitionStop),
			transition: database.WorkspaceTransitionStop,
			desired:    3,
			running:    3,
			inProgress: 1,
			checkFn: func(state prebuilds.ReconciliationState, actions []*prebuilds.ReconciliationActions) {
				validateState(t, prebuilds.ReconciliationState{Actual: 3, Desired: 3, Stopping: 1}, state)
				validateActions(t, nil, actions)
			},
		},
		// With one prebuild desired and one deleting, a new prebuild will be created.
		{
			name:       fmt.Sprintf("%s-short", database.WorkspaceTransitionDelete),
			transition: database.WorkspaceTransitionDelete,
			desired:    1,
			running:    0,
			inProgress: 1,
			checkFn: func(state prebuilds.ReconciliationState, actions []*prebuilds.ReconciliationActions) {
				validateState(t, prebuilds.ReconciliationState{Desired: 1, Deleting: 1}, state)
				validateActions(t, []*prebuilds.ReconciliationActions{
					{
						ActionType: prebuilds.ActionTypeCreate,
						Create:     1,
					},
				}, actions)
			},
		},
		// With 2 prebuilds desired, 1 running, and 1 deleting, a new prebuild will be created.
		{
			name:       fmt.Sprintf("%s-balanced", database.WorkspaceTransitionDelete),
			transition: database.WorkspaceTransitionDelete,
			desired:    2,
			running:    1,
			inProgress: 1,
			checkFn: func(state prebuilds.ReconciliationState, actions []*prebuilds.ReconciliationActions) {
				validateState(t, prebuilds.ReconciliationState{Actual: 1, Desired: 2, Deleting: 1}, state)
				validateActions(t, []*prebuilds.ReconciliationActions{
					{
						ActionType: prebuilds.ActionTypeCreate,
						Create:     1,
					},
				}, actions)
			},
		},
		// With 2 prebuilds desired, 2 running, and 1 deleting, no creations/deletions should occur since the desired state is already achieved.
		{
			name:       fmt.Sprintf("%s-extraneous", database.WorkspaceTransitionDelete),
			transition: database.WorkspaceTransitionDelete,
			desired:    2,
			running:    2,
			inProgress: 1,
			checkFn: func(state prebuilds.ReconciliationState, actions []*prebuilds.ReconciliationActions) {
				validateState(t, prebuilds.ReconciliationState{Actual: 2, Desired: 2, Deleting: 1}, state)
				validateActions(t, nil, actions)
			},
		},
		// With 3 prebuilds desired, 1 running, and 2 starting, no creations should occur since the builds are in progress.
		{
			name:       fmt.Sprintf("%s-inhibit", database.WorkspaceTransitionStart),
			transition: database.WorkspaceTransitionStart,
			desired:    3,
			running:    1,
			inProgress: 2,
			checkFn: func(state prebuilds.ReconciliationState, actions []*prebuilds.ReconciliationActions) {
				validateState(t, prebuilds.ReconciliationState{Actual: 1, Desired: 3, Starting: 2}, state)
				validateActions(t, nil, actions)
			},
		},
		// With 3 prebuilds desired, 5 running, and 2 deleting, no deletions should occur since the builds are in progress.
		{
			name:       fmt.Sprintf("%s-inhibit", database.WorkspaceTransitionDelete),
			transition: database.WorkspaceTransitionDelete,
			desired:    3,
			running:    5,
			inProgress: 2,
			checkFn: func(state prebuilds.ReconciliationState, actions []*prebuilds.ReconciliationActions) {
				expectedState := prebuilds.ReconciliationState{Actual: 5, Desired: 3, Deleting: 2, Extraneous: 2}
				expectedActions := []*prebuilds.ReconciliationActions{
					{
						ActionType: prebuilds.ActionTypeDelete,
					},
				}

				validateState(t, expectedState, state)
				require.Equal(t, len(expectedActions), len(actions))
				assert.EqualValuesf(t, expectedActions[0].ActionType, actions[0].ActionType, "'ActionType' did not match expectation")
				assert.Len(t, actions[0].DeleteIDs, 2, "'deleteIDs' did not match expectation")
				assert.EqualValuesf(t, expectedActions[0].Create, actions[0].Create, "'create' did not match expectation")
				assert.EqualValuesf(t, expectedActions[0].BackoffUntil, actions[0].BackoffUntil, "'BackoffUntil' did not match expectation")
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			// GIVEN: a preset.
			defaultPreset := preset(true, tc.desired, current)
			presets := []database.GetTemplatePresetsWithPrebuildsRow{
				defaultPreset,
			}

			// GIVEN: running prebuilt workspaces for the preset.
			running := make([]database.GetRunningPrebuiltWorkspacesRow, 0, tc.running)
			for range tc.running {
				name, err := prebuilds.GenerateName()
				require.NoError(t, err)
				running = append(running, database.GetRunningPrebuiltWorkspacesRow{
					ID:                uuid.New(),
					Name:              name,
					TemplateID:        current.templateID,
					TemplateVersionID: current.templateVersionID,
					CurrentPresetID:   uuid.NullUUID{UUID: current.presetID, Valid: true},
					Ready:             false,
					CreatedAt:         clock.Now(),
				})
			}

			// GIVEN: some prebuilds for the preset which are currently transitioning.
			inProgress := []database.CountInProgressPrebuildsRow{
				{
					TemplateID:        current.templateID,
					TemplateVersionID: current.templateVersionID,
					Transition:        tc.transition,
					Count:             tc.inProgress,
					PresetID: uuid.NullUUID{
						UUID:  defaultPreset.ID,
						Valid: true,
					},
				},
			}

			// WHEN: calculating the current preset's state.
			snapshot := prebuilds.NewGlobalSnapshot(presets, nil, running, inProgress, nil, nil, nil, quartz.NewMock(t), testutil.Logger(t))
			ps, err := snapshot.FilterByPreset(current.presetID)
			require.NoError(t, err)

			// THEN: we should identify that this prebuild is in progress.
			state := ps.CalculateState()
			actions, err := ps.CalculateActions(backoffInterval)
			require.NoError(t, err)
			tc.checkFn(*state, actions)
		})
	}
}

// Additional prebuilds exist for a given preset configuration; these must be deleted.
func TestExtraneous(t *testing.T) {
	t.Parallel()
	current := opts[optionSet0]
	clock := quartz.NewMock(t)

	// GIVEN: a preset with 1 desired prebuild.
	presets := []database.GetTemplatePresetsWithPrebuildsRow{
		preset(true, 1, current),
	}

	var older uuid.UUID
	// GIVEN: 2 running prebuilds for the preset.
	running := []database.GetRunningPrebuiltWorkspacesRow{
		prebuiltWorkspace(current, clock, func(row database.GetRunningPrebuiltWorkspacesRow) database.GetRunningPrebuiltWorkspacesRow {
			// The older of the running prebuilds will be deleted in order to maintain freshness.
			row.CreatedAt = clock.Now().Add(-time.Hour)
			older = row.ID
			return row
		}),
		prebuiltWorkspace(current, clock, func(row database.GetRunningPrebuiltWorkspacesRow) database.GetRunningPrebuiltWorkspacesRow {
			row.CreatedAt = clock.Now()
			return row
		}),
	}

	// GIVEN: NO prebuilds in progress.
	var inProgress []database.CountInProgressPrebuildsRow

	// WHEN: calculating the current preset's state.
	snapshot := prebuilds.NewGlobalSnapshot(presets, nil, running, inProgress, nil, nil, nil, quartz.NewMock(t), testutil.Logger(t))
	ps, err := snapshot.FilterByPreset(current.presetID)
	require.NoError(t, err)

	// THEN: an extraneous prebuild is detected and marked for deletion.
	state := ps.CalculateState()
	actions, err := ps.CalculateActions(backoffInterval)
	require.NoError(t, err)
	validateState(t, prebuilds.ReconciliationState{
		Actual: 2, Desired: 1, Extraneous: 1, Eligible: 2,
	}, *state)
	validateActions(t, []*prebuilds.ReconciliationActions{
		{
			ActionType: prebuilds.ActionTypeDelete,
			DeleteIDs:  []uuid.UUID{older},
		},
	}, actions)
}

// A prebuild is considered Expired when it has exceeded their time-to-live (TTL)
// specified in the preset's cache invalidation invalidate_after_secs parameter.
func TestExpiredPrebuilds(t *testing.T) {
	t.Parallel()
	current := opts[optionSet3]
	clock := quartz.NewMock(t)

	cases := []struct {
		name    string
		running int32
		desired int32
		expired int32

		invalidated int32

		checkFn func(runningPrebuilds []database.GetRunningPrebuiltWorkspacesRow, state prebuilds.ReconciliationState, actions []*prebuilds.ReconciliationActions)
	}{
		// With 2 running prebuilds, none of which are expired, and the desired count is met,
		// no deletions or creations should occur.
		{
			name:    "no expired prebuilds - no actions taken",
			running: 2,
			desired: 2,
			expired: 0,
			checkFn: func(runningPrebuilds []database.GetRunningPrebuiltWorkspacesRow, state prebuilds.ReconciliationState, actions []*prebuilds.ReconciliationActions) {
				validateState(t, prebuilds.ReconciliationState{Actual: 2, Desired: 2, Expired: 0}, state)
				validateActions(t, nil, actions)
			},
		},
		// With 2 running prebuilds, 1 of which is expired, the expired prebuild should be deleted,
		// and one new prebuild should be created to maintain the desired count.
		{
			name:    "one expired prebuild – deleted and replaced",
			running: 2,
			desired: 2,
			expired: 1,
			checkFn: func(runningPrebuilds []database.GetRunningPrebuiltWorkspacesRow, state prebuilds.ReconciliationState, actions []*prebuilds.ReconciliationActions) {
				expectedState := prebuilds.ReconciliationState{Actual: 2, Desired: 2, Expired: 1}
				expectedActions := []*prebuilds.ReconciliationActions{
					{
						ActionType: prebuilds.ActionTypeDelete,
						DeleteIDs:  []uuid.UUID{runningPrebuilds[0].ID},
					},
					{
						ActionType: prebuilds.ActionTypeCreate,
						Create:     1,
					},
				}

				validateState(t, expectedState, state)
				validateActions(t, expectedActions, actions)
			},
		},
		// With 2 running prebuilds, both expired, both should be deleted,
		// and 2 new prebuilds created to match the desired count.
		{
			name:    "all prebuilds expired – all deleted and recreated",
			running: 2,
			desired: 2,
			expired: 2,
			checkFn: func(runningPrebuilds []database.GetRunningPrebuiltWorkspacesRow, state prebuilds.ReconciliationState, actions []*prebuilds.ReconciliationActions) {
				expectedState := prebuilds.ReconciliationState{Actual: 2, Desired: 2, Expired: 2}
				expectedActions := []*prebuilds.ReconciliationActions{
					{
						ActionType: prebuilds.ActionTypeDelete,
						DeleteIDs:  []uuid.UUID{runningPrebuilds[0].ID, runningPrebuilds[1].ID},
					},
					{
						ActionType: prebuilds.ActionTypeCreate,
						Create:     2,
					},
				}

				validateState(t, expectedState, state)
				validateActions(t, expectedActions, actions)
			},
		},
		// With 4 running prebuilds, 2 of which are expired, and the desired count is 2,
		// the expired prebuilds should be deleted. No new creations are needed
		// since removing the expired ones brings actual = desired.
		{
			name:    "expired prebuilds deleted to reach desired count",
			running: 4,
			desired: 2,
			expired: 2,
			checkFn: func(runningPrebuilds []database.GetRunningPrebuiltWorkspacesRow, state prebuilds.ReconciliationState, actions []*prebuilds.ReconciliationActions) {
				expectedState := prebuilds.ReconciliationState{Actual: 4, Desired: 2, Expired: 2, Extraneous: 0}
				expectedActions := []*prebuilds.ReconciliationActions{
					{
						ActionType: prebuilds.ActionTypeDelete,
						DeleteIDs:  []uuid.UUID{runningPrebuilds[0].ID, runningPrebuilds[1].ID},
					},
				}

				validateState(t, expectedState, state)
				validateActions(t, expectedActions, actions)
			},
		},
		// With 4 running prebuilds (1 expired), and the desired count is 2,
		// the first action should delete the expired one,
		// and the second action should delete one additional (non-expired) prebuild
		// to eliminate the remaining excess.
		{
			name:    "expired prebuild deleted first, then extraneous",
			running: 4,
			desired: 2,
			expired: 1,
			checkFn: func(runningPrebuilds []database.GetRunningPrebuiltWorkspacesRow, state prebuilds.ReconciliationState, actions []*prebuilds.ReconciliationActions) {
				expectedState := prebuilds.ReconciliationState{Actual: 4, Desired: 2, Expired: 1, Extraneous: 1}
				expectedActions := []*prebuilds.ReconciliationActions{
					// First action correspond to deleting the expired prebuild,
					// and the second action corresponds to deleting the extraneous prebuild
					// corresponding to the oldest one after the expired prebuild
					{
						ActionType: prebuilds.ActionTypeDelete,
						DeleteIDs:  []uuid.UUID{runningPrebuilds[0].ID},
					},
					{
						ActionType: prebuilds.ActionTypeDelete,
						DeleteIDs:  []uuid.UUID{runningPrebuilds[1].ID},
					},
				}

				validateState(t, expectedState, state)
				validateActions(t, expectedActions, actions)
			},
		},
		{
			name:        "preset has been invalidated - both instances expired",
			running:     2,
			desired:     2,
			expired:     0,
			invalidated: 2,
			checkFn: func(runningPrebuilds []database.GetRunningPrebuiltWorkspacesRow, state prebuilds.ReconciliationState, actions []*prebuilds.ReconciliationActions) {
				expectedState := prebuilds.ReconciliationState{Actual: 2, Desired: 2, Expired: 2}
				expectedActions := []*prebuilds.ReconciliationActions{
					{
						ActionType: prebuilds.ActionTypeDelete,
						DeleteIDs:  []uuid.UUID{runningPrebuilds[0].ID, runningPrebuilds[1].ID},
					},
					{
						ActionType: prebuilds.ActionTypeCreate,
						Create:     2,
					},
				}

				validateState(t, expectedState, state)
				validateActions(t, expectedActions, actions)
			},
		},
		{
			name:        "preset has been invalidated, but one prebuild instance is newer",
			running:     2,
			desired:     2,
			expired:     0,
			invalidated: 1,
			checkFn: func(runningPrebuilds []database.GetRunningPrebuiltWorkspacesRow, state prebuilds.ReconciliationState, actions []*prebuilds.ReconciliationActions) {
				expectedState := prebuilds.ReconciliationState{Actual: 2, Desired: 2, Expired: 1}
				expectedActions := []*prebuilds.ReconciliationActions{
					{
						ActionType: prebuilds.ActionTypeDelete,
						DeleteIDs:  []uuid.UUID{runningPrebuilds[0].ID},
					},
					{
						ActionType: prebuilds.ActionTypeCreate,
						Create:     1,
					},
				}

				validateState(t, expectedState, state)
				validateActions(t, expectedActions, actions)
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			// GIVEN: a preset.
			now := time.Now()
			invalidatedAt := now.Add(1 * time.Minute)

			var muts []func(row database.GetTemplatePresetsWithPrebuildsRow) database.GetTemplatePresetsWithPrebuildsRow
			if tc.invalidated > 0 {
				muts = append(muts, func(row database.GetTemplatePresetsWithPrebuildsRow) database.GetTemplatePresetsWithPrebuildsRow {
					row.LastInvalidatedAt = sql.NullTime{Valid: true, Time: invalidatedAt}
					return row
				})
			}
			defaultPreset := preset(true, tc.desired, current, muts...)
			presets := []database.GetTemplatePresetsWithPrebuildsRow{
				defaultPreset,
			}

			// GIVEN: running prebuilt workspaces for the preset.
			running := make([]database.GetRunningPrebuiltWorkspacesRow, 0, tc.running)
			expiredCount := 0
			invalidatedCount := 0
			ttlDuration := time.Duration(defaultPreset.Ttl.Int32)
			for range tc.running {
				name, err := prebuilds.GenerateName()
				require.NoError(t, err)

				prebuildCreateAt := time.Now()
				if int(tc.invalidated) > invalidatedCount {
					prebuildCreateAt = prebuildCreateAt.Add(-ttlDuration - 10*time.Second)
					invalidatedCount++
				} else if invalidatedCount > 0 {
					// Only `tc.invalidated` instances have been invalidated,
					// so the next instance is assumed to be created after `invalidatedAt`.
					prebuildCreateAt = invalidatedAt.Add(1 * time.Minute)
				}

				if int(tc.expired) > expiredCount {
					// Update the prebuild workspace createdAt to exceed its TTL (5 seconds)
					prebuildCreateAt = prebuildCreateAt.Add(-ttlDuration - 10*time.Second)
					expiredCount++
				}
				running = append(running, database.GetRunningPrebuiltWorkspacesRow{
					ID:                uuid.New(),
					Name:              name,
					TemplateID:        current.templateID,
					TemplateVersionID: current.templateVersionID,
					CurrentPresetID:   uuid.NullUUID{UUID: current.presetID, Valid: true},
					Ready:             false,
					CreatedAt:         prebuildCreateAt,
				})
			}

			// WHEN: calculating the current preset's state.
			snapshot := prebuilds.NewGlobalSnapshot(presets, nil, running, nil, nil, nil, nil, clock, testutil.Logger(t))
			ps, err := snapshot.FilterByPreset(current.presetID)
			require.NoError(t, err)

			// THEN: we should identify that this prebuild is expired.
			state := ps.CalculateState()
			actions, err := ps.CalculateActions(backoffInterval)
			require.NoError(t, err)
			tc.checkFn(running, *state, actions)
		})
	}
}

// A template marked as deprecated will not have prebuilds running.
func TestDeprecated(t *testing.T) {
	t.Parallel()
	current := opts[optionSet0]
	clock := quartz.NewMock(t)

	// GIVEN: a preset with 1 desired prebuild.
	presets := []database.GetTemplatePresetsWithPrebuildsRow{
		preset(true, 1, current, func(row database.GetTemplatePresetsWithPrebuildsRow) database.GetTemplatePresetsWithPrebuildsRow {
			row.Deprecated = true
			return row
		}),
	}

	// GIVEN: 1 running prebuilds for the preset.
	running := []database.GetRunningPrebuiltWorkspacesRow{
		prebuiltWorkspace(current, clock),
	}

	// GIVEN: NO prebuilds in progress.
	var inProgress []database.CountInProgressPrebuildsRow

	// WHEN: calculating the current preset's state.
	snapshot := prebuilds.NewGlobalSnapshot(presets, nil, running, inProgress, nil, nil, nil, quartz.NewMock(t), testutil.Logger(t))
	ps, err := snapshot.FilterByPreset(current.presetID)
	require.NoError(t, err)

	// THEN: all running prebuilds should be deleted because the template is deprecated.
	state := ps.CalculateState()
	actions, err := ps.CalculateActions(backoffInterval)
	require.NoError(t, err)
	validateState(t, prebuilds.ReconciliationState{
		Actual: 1,
	}, *state)
	validateActions(t, []*prebuilds.ReconciliationActions{
		{
			ActionType: prebuilds.ActionTypeDelete,
			DeleteIDs:  []uuid.UUID{current.prebuiltWorkspaceID},
		},
	}, actions)
}

// If the latest build failed, backoff exponentially with the given interval.
func TestLatestBuildFailed(t *testing.T) {
	t.Parallel()
	current := opts[optionSet0]
	other := opts[optionSet1]
	clock := quartz.NewMock(t)

	// GIVEN: two presets.
	presets := []database.GetTemplatePresetsWithPrebuildsRow{
		preset(true, 1, current),
		preset(true, 1, other),
	}

	// GIVEN: running prebuilds only for one preset (the other will be failing, as evidenced by the backoffs below).
	running := []database.GetRunningPrebuiltWorkspacesRow{
		prebuiltWorkspace(other, clock),
	}

	// GIVEN: NO prebuilds in progress.
	var inProgress []database.CountInProgressPrebuildsRow

	// GIVEN: a backoff entry.
	lastBuildTime := clock.Now()
	numFailed := 1
	backoffs := []database.GetPresetsBackoffRow{
		{
			TemplateVersionID: current.templateVersionID,
			PresetID:          current.presetID,
			NumFailed:         int32(numFailed),
			LastBuildAt:       lastBuildTime,
		},
	}

	// WHEN: calculating the current preset's state.
	snapshot := prebuilds.NewGlobalSnapshot(presets, nil, running, inProgress, nil, backoffs, nil, clock, testutil.Logger(t))
	psCurrent, err := snapshot.FilterByPreset(current.presetID)
	require.NoError(t, err)

	// THEN: reconciliation should backoff.
	state := psCurrent.CalculateState()
	actions, err := psCurrent.CalculateActions(backoffInterval)
	require.NoError(t, err)
	validateState(t, prebuilds.ReconciliationState{
		Actual: 0, Desired: 1,
	}, *state)
	validateActions(t, []*prebuilds.ReconciliationActions{
		{
			ActionType:   prebuilds.ActionTypeBackoff,
			BackoffUntil: lastBuildTime.Add(time.Duration(numFailed) * backoffInterval),
		},
	}, actions)

	// WHEN: calculating the other preset's state.
	psOther, err := snapshot.FilterByPreset(other.presetID)
	require.NoError(t, err)

	// THEN: it should NOT be in backoff because all is OK.
	state = psOther.CalculateState()
	actions, err = psOther.CalculateActions(backoffInterval)
	require.NoError(t, err)
	validateState(t, prebuilds.ReconciliationState{
		Actual: 1, Desired: 1, Eligible: 1,
	}, *state)
	validateActions(t, nil, actions)

	// WHEN: the clock is advanced a backoff interval.
	clock.Advance(backoffInterval + time.Microsecond)

	// THEN: a new prebuild should be created.
	psCurrent, err = snapshot.FilterByPreset(current.presetID)
	require.NoError(t, err)
	state = psCurrent.CalculateState()
	actions, err = psCurrent.CalculateActions(backoffInterval)
	require.NoError(t, err)
	validateState(t, prebuilds.ReconciliationState{
		Actual: 0, Desired: 1,
	}, *state)
	validateActions(t, []*prebuilds.ReconciliationActions{
		{
			ActionType: prebuilds.ActionTypeCreate,
			Create:     1, // <--- NOTE: we're now able to create a new prebuild because the interval has elapsed.
		},
	}, actions)
}

func TestMultiplePresetsPerTemplateVersion(t *testing.T) {
	t.Parallel()

	templateID := uuid.New()
	templateVersionID := uuid.New()
	presetOpts1 := options{
		templateID:          templateID,
		templateVersionID:   templateVersionID,
		presetID:            uuid.New(),
		presetName:          "my-preset-1",
		prebuiltWorkspaceID: uuid.New(),
		workspaceName:       "prebuilds1",
	}
	presetOpts2 := options{
		templateID:          templateID,
		templateVersionID:   templateVersionID,
		presetID:            uuid.New(),
		presetName:          "my-preset-2",
		prebuiltWorkspaceID: uuid.New(),
		workspaceName:       "prebuilds2",
	}

	clock := quartz.NewMock(t)

	presets := []database.GetTemplatePresetsWithPrebuildsRow{
		preset(true, 1, presetOpts1),
		preset(true, 1, presetOpts2),
	}

	inProgress := []database.CountInProgressPrebuildsRow{
		{
			TemplateID:        templateID,
			TemplateVersionID: templateVersionID,
			Transition:        database.WorkspaceTransitionStart,
			Count:             1,
			PresetID: uuid.NullUUID{
				UUID:  presetOpts1.presetID,
				Valid: true,
			},
		},
	}

	snapshot := prebuilds.NewGlobalSnapshot(presets, nil, nil, inProgress, nil, nil, nil, clock, testutil.Logger(t))

	// Nothing has to be created for preset 1.
	{
		ps, err := snapshot.FilterByPreset(presetOpts1.presetID)
		require.NoError(t, err)

		state := ps.CalculateState()
		actions, err := ps.CalculateActions(backoffInterval)
		require.NoError(t, err)

		validateState(t, prebuilds.ReconciliationState{
			Starting: 1,
			Desired:  1,
		}, *state)
		validateActions(t, nil, actions)
	}

	// One prebuild has to be created for preset 2. Make sure preset 1 doesn't block preset 2.
	{
		ps, err := snapshot.FilterByPreset(presetOpts2.presetID)
		require.NoError(t, err)

		state := ps.CalculateState()
		actions, err := ps.CalculateActions(backoffInterval)
		require.NoError(t, err)

		validateState(t, prebuilds.ReconciliationState{
			Starting: 0,
			Desired:  1,
		}, *state)
		validateActions(t, []*prebuilds.ReconciliationActions{
			{
				ActionType: prebuilds.ActionTypeCreate,
				Create:     1,
			},
		}, actions)
	}
}

func TestPrebuildScheduling(t *testing.T) {
	t.Parallel()

	// The test includes 2 presets, each with 2 schedules.
	// It checks that the calculated actions match expectations for various provided times,
	// based on the corresponding schedules.
	testCases := []struct {
		name string
		// now specifies the current time.
		now time.Time
		// expected instances for preset1 and preset2, respectively.
		expectedInstances []int32
	}{
		{
			name:              "Before the 1st schedule",
			now:               mustParseTime(t, time.RFC1123, "Mon, 02 Jun 2025 01:00:00 UTC"),
			expectedInstances: []int32{1, 1},
		},
		{
			name:              "1st schedule",
			now:               mustParseTime(t, time.RFC1123, "Mon, 02 Jun 2025 03:00:00 UTC"),
			expectedInstances: []int32{2, 1},
		},
		{
			name:              "2nd schedule",
			now:               mustParseTime(t, time.RFC1123, "Mon, 02 Jun 2025 07:00:00 UTC"),
			expectedInstances: []int32{3, 1},
		},
		{
			name:              "3rd schedule",
			now:               mustParseTime(t, time.RFC1123, "Mon, 02 Jun 2025 11:00:00 UTC"),
			expectedInstances: []int32{1, 4},
		},
		{
			name:              "4th schedule",
			now:               mustParseTime(t, time.RFC1123, "Mon, 02 Jun 2025 15:00:00 UTC"),
			expectedInstances: []int32{1, 5},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			templateID := uuid.New()
			templateVersionID := uuid.New()
			presetOpts1 := options{
				templateID:          templateID,
				templateVersionID:   templateVersionID,
				presetID:            uuid.New(),
				presetName:          "my-preset-1",
				prebuiltWorkspaceID: uuid.New(),
				workspaceName:       "prebuilds1",
			}
			presetOpts2 := options{
				templateID:          templateID,
				templateVersionID:   templateVersionID,
				presetID:            uuid.New(),
				presetName:          "my-preset-2",
				prebuiltWorkspaceID: uuid.New(),
				workspaceName:       "prebuilds2",
			}

			clock := quartz.NewMock(t)
			clock.Set(tc.now)
			enableScheduling := func(preset database.GetTemplatePresetsWithPrebuildsRow) database.GetTemplatePresetsWithPrebuildsRow {
				preset.SchedulingTimezone = "UTC"
				return preset
			}
			presets := []database.GetTemplatePresetsWithPrebuildsRow{
				preset(true, 1, presetOpts1, enableScheduling),
				preset(true, 1, presetOpts2, enableScheduling),
			}
			schedules := []database.TemplateVersionPresetPrebuildSchedule{
				schedule(presets[0].ID, "* 2-4 * * 1-5", 2),
				schedule(presets[0].ID, "* 6-8 * * 1-5", 3),
				schedule(presets[1].ID, "* 10-12 * * 1-5", 4),
				schedule(presets[1].ID, "* 14-16 * * 1-5", 5),
			}

			snapshot := prebuilds.NewGlobalSnapshot(presets, schedules, nil, nil, nil, nil, nil, clock, testutil.Logger(t))

			// Check 1st preset.
			{
				ps, err := snapshot.FilterByPreset(presetOpts1.presetID)
				require.NoError(t, err)

				state := ps.CalculateState()
				actions, err := ps.CalculateActions(backoffInterval)
				require.NoError(t, err)

				validateState(t, prebuilds.ReconciliationState{
					Starting: 0,
					Desired:  tc.expectedInstances[0],
				}, *state)
				validateActions(t, []*prebuilds.ReconciliationActions{
					{
						ActionType: prebuilds.ActionTypeCreate,
						Create:     tc.expectedInstances[0],
					},
				}, actions)
			}

			// Check 2nd preset.
			{
				ps, err := snapshot.FilterByPreset(presetOpts2.presetID)
				require.NoError(t, err)

				state := ps.CalculateState()
				actions, err := ps.CalculateActions(backoffInterval)
				require.NoError(t, err)

				validateState(t, prebuilds.ReconciliationState{
					Starting: 0,
					Desired:  tc.expectedInstances[1],
				}, *state)
				validateActions(t, []*prebuilds.ReconciliationActions{
					{
						ActionType: prebuilds.ActionTypeCreate,
						Create:     tc.expectedInstances[1],
					},
				}, actions)
			}
		})
	}
}

func TestMatchesCron(t *testing.T) {
	t.Parallel()
	testCases := []struct {
		name            string
		spec            string
		at              time.Time
		expectedMatches bool
	}{
		// A comprehensive test suite for time range evaluation is implemented in TestIsWithinRange.
		// This test provides only basic coverage.
		{
			name:            "Right before the start of the time range",
			spec:            "* 9-18 * * 1-5",
			at:              mustParseTime(t, time.RFC1123, "Mon, 02 Jun 2025 8:59:59 UTC"),
			expectedMatches: false,
		},
		{
			name:            "Start of the time range",
			spec:            "* 9-18 * * 1-5",
			at:              mustParseTime(t, time.RFC1123, "Mon, 02 Jun 2025 9:00:00 UTC"),
			expectedMatches: true,
		},
	}

	for _, testCase := range testCases {
		testCase := testCase
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			matches, err := prebuilds.MatchesCron(testCase.spec, testCase.at)
			require.NoError(t, err)
			require.Equal(t, testCase.expectedMatches, matches)
		})
	}
}

func TestCalculateDesiredInstances(t *testing.T) {
	t.Parallel()

	mkPreset := func(instances int32, timezone string) database.GetTemplatePresetsWithPrebuildsRow {
		return database.GetTemplatePresetsWithPrebuildsRow{
			DesiredInstances: sql.NullInt32{
				Int32: instances,
				Valid: true,
			},
			SchedulingTimezone: timezone,
		}
	}
	mkSchedule := func(cronExpr string, instances int32) database.TemplateVersionPresetPrebuildSchedule {
		return database.TemplateVersionPresetPrebuildSchedule{
			CronExpression:   cronExpr,
			DesiredInstances: instances,
		}
	}
	mkSnapshot := func(preset database.GetTemplatePresetsWithPrebuildsRow, schedules ...database.TemplateVersionPresetPrebuildSchedule) prebuilds.PresetSnapshot {
		return prebuilds.NewPresetSnapshot(
			preset,
			schedules,
			nil,
			nil,
			nil,
			0,
			nil,
			false,
			quartz.NewMock(t),
			testutil.Logger(t),
		)
	}

	testCases := []struct {
		name                        string
		snapshot                    prebuilds.PresetSnapshot
		at                          time.Time
		expectedCalculatedInstances int32
	}{
		// "* 9-18 * * 1-5" should be interpreted as a continuous time range from 09:00:00 to 18:59:59, Monday through Friday
		{
			name: "Right before the start of the time range",
			snapshot: mkSnapshot(
				mkPreset(1, "UTC"),
				mkSchedule("* 9-18 * * 1-5", 3),
			),
			at:                          mustParseTime(t, time.RFC1123, "Mon, 02 Jun 2025 8:59:59 UTC"),
			expectedCalculatedInstances: 1,
		},
		{
			name: "Start of the time range",
			snapshot: mkSnapshot(
				mkPreset(1, "UTC"),
				mkSchedule("* 9-18 * * 1-5", 3),
			),
			at:                          mustParseTime(t, time.RFC1123, "Mon, 02 Jun 2025 9:00:00 UTC"),
			expectedCalculatedInstances: 3,
		},
		{
			name: "9:01AM - One minute after the start of the time range",
			snapshot: mkSnapshot(
				mkPreset(1, "UTC"),
				mkSchedule("* 9-18 * * 1-5", 3),
			),
			at:                          mustParseTime(t, time.RFC1123, "Mon, 02 Jun 2025 9:01:00 UTC"),
			expectedCalculatedInstances: 3,
		},
		{
			name: "2PM - The middle of the time range",
			snapshot: mkSnapshot(
				mkPreset(1, "UTC"),
				mkSchedule("* 9-18 * * 1-5", 3),
			),
			at:                          mustParseTime(t, time.RFC1123, "Mon, 02 Jun 2025 14:00:00 UTC"),
			expectedCalculatedInstances: 3,
		},
		{
			name: "6PM - One hour before the end of the time range",
			snapshot: mkSnapshot(
				mkPreset(1, "UTC"),
				mkSchedule("* 9-18 * * 1-5", 3),
			),
			at:                          mustParseTime(t, time.RFC1123, "Mon, 02 Jun 2025 18:00:00 UTC"),
			expectedCalculatedInstances: 3,
		},
		{
			name: "End of the time range",
			snapshot: mkSnapshot(
				mkPreset(1, "UTC"),
				mkSchedule("* 9-18 * * 1-5", 3),
			),
			at:                          mustParseTime(t, time.RFC1123, "Mon, 02 Jun 2025 18:59:59 UTC"),
			expectedCalculatedInstances: 3,
		},
		{
			name: "Right after the end of the time range",
			snapshot: mkSnapshot(
				mkPreset(1, "UTC"),
				mkSchedule("* 9-18 * * 1-5", 3),
			),
			at:                          mustParseTime(t, time.RFC1123, "Mon, 02 Jun 2025 19:00:00 UTC"),
			expectedCalculatedInstances: 1,
		},
		{
			name: "7:01PM - Around one minute after the end of the time range",
			snapshot: mkSnapshot(
				mkPreset(1, "UTC"),
				mkSchedule("* 9-18 * * 1-5", 3),
			),
			at:                          mustParseTime(t, time.RFC1123, "Mon, 02 Jun 2025 19:01:00 UTC"),
			expectedCalculatedInstances: 1,
		},
		{
			name: "2AM - Significantly outside the time range",
			snapshot: mkSnapshot(
				mkPreset(1, "UTC"),
				mkSchedule("* 9-18 * * 1-5", 3),
			),
			at:                          mustParseTime(t, time.RFC1123, "Mon, 02 Jun 2025 02:00:00 UTC"),
			expectedCalculatedInstances: 1,
		},
		{
			name: "Outside the day range #1",
			snapshot: mkSnapshot(
				mkPreset(1, "UTC"),
				mkSchedule("* 9-18 * * 1-5", 3),
			),
			at:                          mustParseTime(t, time.RFC1123, "Sat, 07 Jun 2025 14:00:00 UTC"),
			expectedCalculatedInstances: 1,
		},
		{
			name: "Outside the day range #2",
			snapshot: mkSnapshot(
				mkPreset(1, "UTC"),
				mkSchedule("* 9-18 * * 1-5", 3),
			),
			at:                          mustParseTime(t, time.RFC1123, "Sun, 08 Jun 2025 14:00:00 UTC"),
			expectedCalculatedInstances: 1,
		},

		// Test multiple schedules during the day
		// - "* 6-10 * * 1-5"
		// - "* 12-16 * * 1-5"
		// - "* 18-22 * * 1-5"
		{
			name: "Before the first schedule",
			snapshot: mkSnapshot(
				mkPreset(1, "UTC"),
				mkSchedule("* 6-10 * * 1-5", 2),
				mkSchedule("* 12-16 * * 1-5", 3),
				mkSchedule("* 18-22 * * 1-5", 4),
			),
			at:                          mustParseTime(t, time.RFC1123, "Mon, 02 Jun 2025 5:00:00 UTC"),
			expectedCalculatedInstances: 1,
		},
		{
			name: "The middle of the first schedule",
			snapshot: mkSnapshot(
				mkPreset(1, "UTC"),
				mkSchedule("* 6-10 * * 1-5", 2),
				mkSchedule("* 12-16 * * 1-5", 3),
				mkSchedule("* 18-22 * * 1-5", 4),
			),
			at:                          mustParseTime(t, time.RFC1123, "Mon, 02 Jun 2025 8:00:00 UTC"),
			expectedCalculatedInstances: 2,
		},
		{
			name: "Between the first and second schedule",
			snapshot: mkSnapshot(
				mkPreset(1, "UTC"),
				mkSchedule("* 6-10 * * 1-5", 2),
				mkSchedule("* 12-16 * * 1-5", 3),
				mkSchedule("* 18-22 * * 1-5", 4),
			),
			at:                          mustParseTime(t, time.RFC1123, "Mon, 02 Jun 2025 11:00:00 UTC"),
			expectedCalculatedInstances: 1,
		},
		{
			name: "The middle of the second schedule",
			snapshot: mkSnapshot(
				mkPreset(1, "UTC"),
				mkSchedule("* 6-10 * * 1-5", 2),
				mkSchedule("* 12-16 * * 1-5", 3),
				mkSchedule("* 18-22 * * 1-5", 4),
			),
			at:                          mustParseTime(t, time.RFC1123, "Mon, 02 Jun 2025 14:00:00 UTC"),
			expectedCalculatedInstances: 3,
		},
		{
			name: "The middle of the third schedule",
			snapshot: mkSnapshot(
				mkPreset(1, "UTC"),
				mkSchedule("* 6-10 * * 1-5", 2),
				mkSchedule("* 12-16 * * 1-5", 3),
				mkSchedule("* 18-22 * * 1-5", 4),
			),
			at:                          mustParseTime(t, time.RFC1123, "Mon, 02 Jun 2025 20:00:00 UTC"),
			expectedCalculatedInstances: 4,
		},
		{
			name: "After the last schedule",
			snapshot: mkSnapshot(
				mkPreset(1, "UTC"),
				mkSchedule("* 6-10 * * 1-5", 2),
				mkSchedule("* 12-16 * * 1-5", 3),
				mkSchedule("* 18-22 * * 1-5", 4),
			),
			at:                          mustParseTime(t, time.RFC1123, "Mon, 02 Jun 2025 23:00:00 UTC"),
			expectedCalculatedInstances: 1,
		},

		// Test multiple schedules during the week
		// - "* 9-18 * * 1-5"
		// - "* 9-13 * * 6-7"
		{
			name: "First schedule",
			snapshot: mkSnapshot(
				mkPreset(1, "UTC"),
				mkSchedule("* 9-18 * * 1-5", 2),
				mkSchedule("* 9-13 * * 6,0", 3),
			),
			at:                          mustParseTime(t, time.RFC1123, "Mon, 02 Jun 2025 14:00:00 UTC"),
			expectedCalculatedInstances: 2,
		},
		{
			name: "Second schedule",
			snapshot: mkSnapshot(
				mkPreset(1, "UTC"),
				mkSchedule("* 9-18 * * 1-5", 2),
				mkSchedule("* 9-13 * * 6,0", 3),
			),
			at:                          mustParseTime(t, time.RFC1123, "Sat, 07 Jun 2025 10:00:00 UTC"),
			expectedCalculatedInstances: 3,
		},
		{
			name: "Outside schedule",
			snapshot: mkSnapshot(
				mkPreset(1, "UTC"),
				mkSchedule("* 9-18 * * 1-5", 2),
				mkSchedule("* 9-13 * * 6,0", 3),
			),
			at:                          mustParseTime(t, time.RFC1123, "Sat, 07 Jun 2025 14:00:00 UTC"),
			expectedCalculatedInstances: 1,
		},

		// Test different timezones
		{
			name: "3PM UTC - 8AM America/Los_Angeles; An hour before the start of the time range",
			snapshot: mkSnapshot(
				mkPreset(1, "America/Los_Angeles"),
				mkSchedule("* 9-13 * * 1-5", 3),
			),
			at:                          mustParseTime(t, time.RFC1123, "Mon, 02 Jun 2025 15:00:00 UTC"),
			expectedCalculatedInstances: 1,
		},
		{
			name: "4PM UTC - 9AM America/Los_Angeles; Start of the time range",
			snapshot: mkSnapshot(
				mkPreset(1, "America/Los_Angeles"),
				mkSchedule("* 9-13 * * 1-5", 3),
			),
			at:                          mustParseTime(t, time.RFC1123, "Mon, 02 Jun 2025 16:00:00 UTC"),
			expectedCalculatedInstances: 3,
		},
		{
			name: "8:59PM UTC - 1:58PM America/Los_Angeles; Right before the end of the time range",
			snapshot: mkSnapshot(
				mkPreset(1, "America/Los_Angeles"),
				mkSchedule("* 9-13 * * 1-5", 3),
			),
			at:                          mustParseTime(t, time.RFC1123, "Mon, 02 Jun 2025 20:59:00 UTC"),
			expectedCalculatedInstances: 3,
		},
		{
			name: "9PM UTC - 2PM America/Los_Angeles; Right after the end of the time range",
			snapshot: mkSnapshot(
				mkPreset(1, "America/Los_Angeles"),
				mkSchedule("* 9-13 * * 1-5", 3),
			),
			at:                          mustParseTime(t, time.RFC1123, "Mon, 02 Jun 2025 21:00:00 UTC"),
			expectedCalculatedInstances: 1,
		},
		{
			name: "11PM UTC - 4PM America/Los_Angeles; Outside the time range",
			snapshot: mkSnapshot(
				mkPreset(1, "America/Los_Angeles"),
				mkSchedule("* 9-13 * * 1-5", 3),
			),
			at:                          mustParseTime(t, time.RFC1123, "Mon, 02 Jun 2025 23:00:00 UTC"),
			expectedCalculatedInstances: 1,
		},

		// Verify support for time values specified in non-UTC time zones.
		{
			name: "8AM - before the start of the time range",
			snapshot: mkSnapshot(
				mkPreset(1, "UTC"),
				mkSchedule("* 9-18 * * 1-5", 3),
			),
			at:                          mustParseTime(t, time.RFC1123Z, "Mon, 02 Jun 2025 04:00:00 -0400"),
			expectedCalculatedInstances: 1,
		},
		{
			name: "9AM - after the start of the time range",
			snapshot: mkSnapshot(
				mkPreset(1, "UTC"),
				mkSchedule("* 9-18 * * 1-5", 3),
			),
			at:                          mustParseTime(t, time.RFC1123Z, "Mon, 02 Jun 2025 05:00:00 -0400"),
			expectedCalculatedInstances: 3,
		},
	}

	for _, tc := range testCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			desiredInstances := tc.snapshot.CalculateDesiredInstances(tc.at)
			require.Equal(t, tc.expectedCalculatedInstances, desiredInstances)
		})
	}
}

// TestCanSkipReconciliation ensures that CanSkipReconciliation only returns true
// when CalculateActions would return no actions.
func TestCanSkipReconciliation(t *testing.T) {
	t.Parallel()

	clock := quartz.NewMock(t)
	logger := slogtest.Make(t, nil)
	backoffInterval := 5 * time.Minute

	tests := []struct {
		name               string
		preset             database.GetTemplatePresetsWithPrebuildsRow
		running            []database.GetRunningPrebuiltWorkspacesRow
		expired            []database.GetRunningPrebuiltWorkspacesRow
		inProgress         []database.CountInProgressPrebuildsRow
		pendingCount       int
		backoff            *database.GetPresetsBackoffRow
		isHardLimited      bool
		expectedCanSkip    bool
		expectedActionNoOp bool
	}{
		{
			name: "inactive_with_nothing_to_cleanup",
			preset: database.GetTemplatePresetsWithPrebuildsRow{
				UsingActiveVersion: false,
				Deleted:            false,
				Deprecated:         false,
				DesiredInstances:   sql.NullInt32{Int32: 5, Valid: true},
			},
			running:            []database.GetRunningPrebuiltWorkspacesRow{},
			expired:            []database.GetRunningPrebuiltWorkspacesRow{},
			inProgress:         []database.CountInProgressPrebuildsRow{},
			pendingCount:       0,
			backoff:            nil,
			isHardLimited:      false,
			expectedCanSkip:    true,
			expectedActionNoOp: true,
		},
		{
			name: "inactive_with_running_workspaces",
			preset: database.GetTemplatePresetsWithPrebuildsRow{
				UsingActiveVersion: false,
				Deleted:            false,
				Deprecated:         false,
			},
			running: []database.GetRunningPrebuiltWorkspacesRow{
				{ID: uuid.New()},
			},
			expired:            []database.GetRunningPrebuiltWorkspacesRow{},
			inProgress:         []database.CountInProgressPrebuildsRow{},
			pendingCount:       0,
			backoff:            nil,
			isHardLimited:      false,
			expectedCanSkip:    false,
			expectedActionNoOp: false,
		},
		{
			name: "inactive_with_pending_jobs",
			preset: database.GetTemplatePresetsWithPrebuildsRow{
				UsingActiveVersion: false,
				Deleted:            false,
				Deprecated:         false,
			},
			running:            []database.GetRunningPrebuiltWorkspacesRow{},
			expired:            []database.GetRunningPrebuiltWorkspacesRow{},
			inProgress:         []database.CountInProgressPrebuildsRow{},
			pendingCount:       3,
			backoff:            nil,
			isHardLimited:      false,
			expectedCanSkip:    false,
			expectedActionNoOp: false,
		},
		{
			name: "inactive_deleted_template_with_nothing_to_cleanup",
			preset: database.GetTemplatePresetsWithPrebuildsRow{
				UsingActiveVersion: false,
				Deleted:            true,
				Deprecated:         false,
			},
			running:            []database.GetRunningPrebuiltWorkspacesRow{},
			expired:            []database.GetRunningPrebuiltWorkspacesRow{},
			inProgress:         []database.CountInProgressPrebuildsRow{},
			pendingCount:       0,
			backoff:            nil,
			isHardLimited:      false,
			expectedCanSkip:    true,
			expectedActionNoOp: true,
		},
		{
			name: "inactive_deprecated_template_with_nothing_to_cleanup",
			preset: database.GetTemplatePresetsWithPrebuildsRow{
				UsingActiveVersion: false,
				Deleted:            false,
				Deprecated:         true,
			},
			running:            []database.GetRunningPrebuiltWorkspacesRow{},
			expired:            []database.GetRunningPrebuiltWorkspacesRow{},
			inProgress:         []database.CountInProgressPrebuildsRow{},
			pendingCount:       0,
			backoff:            nil,
			isHardLimited:      false,
			expectedCanSkip:    true,
			expectedActionNoOp: true,
		},
		{
			name: "inactive_hard_limited",
			preset: database.GetTemplatePresetsWithPrebuildsRow{
				UsingActiveVersion: false,
				Deleted:            false,
				Deprecated:         false,
			},
			running:            []database.GetRunningPrebuiltWorkspacesRow{},
			expired:            []database.GetRunningPrebuiltWorkspacesRow{},
			inProgress:         []database.CountInProgressPrebuildsRow{},
			pendingCount:       0,
			backoff:            nil,
			isHardLimited:      true,
			expectedCanSkip:    true,
			expectedActionNoOp: true,
		},
		{
			name: "active_with_desired_instances",
			preset: database.GetTemplatePresetsWithPrebuildsRow{
				UsingActiveVersion: true,
				Deleted:            false,
				Deprecated:         false,
				DesiredInstances:   sql.NullInt32{Int32: 2, Valid: true},
			},
			running: []database.GetRunningPrebuiltWorkspacesRow{
				{ID: uuid.New()},
				{ID: uuid.New()},
			},
			expired:            []database.GetRunningPrebuiltWorkspacesRow{},
			inProgress:         []database.CountInProgressPrebuildsRow{},
			pendingCount:       0,
			backoff:            nil,
			isHardLimited:      false,
			expectedCanSkip:    false, // Active presets are never skipped
			expectedActionNoOp: true,  // But no action needed
		},
		{
			name: "active_with_no_workspaces",
			preset: database.GetTemplatePresetsWithPrebuildsRow{
				UsingActiveVersion: true,
				Deleted:            false,
				Deprecated:         false,
				DesiredInstances:   sql.NullInt32{Int32: 5, Valid: true},
			},
			running:            []database.GetRunningPrebuiltWorkspacesRow{},
			expired:            []database.GetRunningPrebuiltWorkspacesRow{},
			inProgress:         []database.CountInProgressPrebuildsRow{},
			pendingCount:       0,
			backoff:            nil,
			isHardLimited:      false,
			expectedCanSkip:    false,
			expectedActionNoOp: false, // Should create 5 workspaces
		},
		{
			name: "active_with_backoff",
			preset: database.GetTemplatePresetsWithPrebuildsRow{
				UsingActiveVersion: true,
				Deleted:            false,
				Deprecated:         false,
				DesiredInstances:   sql.NullInt32{Int32: 5, Valid: true},
			},
			running:      []database.GetRunningPrebuiltWorkspacesRow{},
			expired:      []database.GetRunningPrebuiltWorkspacesRow{},
			inProgress:   []database.CountInProgressPrebuildsRow{},
			pendingCount: 0,
			backoff: &database.GetPresetsBackoffRow{
				NumFailed:   3,
				LastBuildAt: clock.Now().Add(-1 * time.Minute),
			},
			isHardLimited:      false,
			expectedCanSkip:    false,
			expectedActionNoOp: false, // Should backoff
		},
		{
			name: "active_hard_limited",
			preset: database.GetTemplatePresetsWithPrebuildsRow{
				UsingActiveVersion: true,
				Deleted:            false,
				Deprecated:         false,
				DesiredInstances:   sql.NullInt32{Int32: 5, Valid: true},
			},
			running:            []database.GetRunningPrebuiltWorkspacesRow{},
			expired:            []database.GetRunningPrebuiltWorkspacesRow{},
			inProgress:         []database.CountInProgressPrebuildsRow{},
			pendingCount:       0,
			backoff:            nil,
			isHardLimited:      true,
			expectedCanSkip:    false, // Active presets are never skipped
			expectedActionNoOp: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			ps := prebuilds.NewPresetSnapshot(
				tt.preset,
				[]database.TemplateVersionPresetPrebuildSchedule{},
				tt.running,
				tt.expired,
				tt.inProgress,
				tt.pendingCount,
				tt.backoff,
				tt.isHardLimited,
				clock,
				logger,
			)

			canSkip := ps.CanSkipReconciliation()
			require.Equal(t, tt.expectedCanSkip, canSkip)

			actions, err := ps.CalculateActions(backoffInterval)
			require.NoError(t, err)

			actionNoOp := true
			for _, action := range actions {
				if !action.IsNoop() {
					actionNoOp = false
					break
				}
			}
			require.Equal(t, tt.expectedActionNoOp, actionNoOp,
				"CalculateActions() isNoOp mismatch")

			// IMPORTANT: If CanSkipReconciliation is true, CalculateActions must return no actions
			if canSkip {
				require.True(t, actionNoOp)
			}
		})
	}
}

func mustParseTime(t *testing.T, layout, value string) time.Time {
	t.Helper()
	parsedTime, err := time.Parse(layout, value)
	require.NoError(t, err)
	return parsedTime
}

func preset(active bool, instances int32, opts options, muts ...func(row database.GetTemplatePresetsWithPrebuildsRow) database.GetTemplatePresetsWithPrebuildsRow) database.GetTemplatePresetsWithPrebuildsRow {
	ttl := sql.NullInt32{}
	if opts.ttl > 0 {
		ttl = sql.NullInt32{
			Valid: true,
			Int32: opts.ttl,
		}
	}
	entry := database.GetTemplatePresetsWithPrebuildsRow{
		TemplateID:         opts.templateID,
		TemplateVersionID:  opts.templateVersionID,
		ID:                 opts.presetID,
		UsingActiveVersion: active,
		Name:               opts.presetName,
		DesiredInstances: sql.NullInt32{
			Valid: true,
			Int32: instances,
		},
		Deleted:    false,
		Deprecated: false,
		Ttl:        ttl,
	}

	for _, mut := range muts {
		entry = mut(entry)
	}
	return entry
}

func schedule(presetID uuid.UUID, cronExpr string, instances int32) database.TemplateVersionPresetPrebuildSchedule {
	return database.TemplateVersionPresetPrebuildSchedule{
		ID:               uuid.New(),
		PresetID:         presetID,
		CronExpression:   cronExpr,
		DesiredInstances: instances,
	}
}

func prebuiltWorkspace(
	opts options,
	clock quartz.Clock,
	muts ...func(row database.GetRunningPrebuiltWorkspacesRow) database.GetRunningPrebuiltWorkspacesRow,
) database.GetRunningPrebuiltWorkspacesRow {
	entry := database.GetRunningPrebuiltWorkspacesRow{
		ID:                opts.prebuiltWorkspaceID,
		Name:              opts.workspaceName,
		TemplateID:        opts.templateID,
		TemplateVersionID: opts.templateVersionID,
		CurrentPresetID:   uuid.NullUUID{UUID: opts.presetID, Valid: true},
		Ready:             true,
		CreatedAt:         clock.Now(),
	}

	for _, mut := range muts {
		entry = mut(entry)
	}
	return entry
}

func validateState(t *testing.T, expected, actual prebuilds.ReconciliationState) {
	require.Equal(t, expected, actual)
}

// validateActions is a convenience func to make tests more readable; it exploits the fact that the default states for
// prebuilds align with zero values.
func validateActions(t *testing.T, expected, actual []*prebuilds.ReconciliationActions) {
	require.Equal(t, expected, actual)
}
