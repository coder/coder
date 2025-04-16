package prebuilds_test

import (
	"database/sql"
	"fmt"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/coder/quartz"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/prebuilds"
)

type options struct {
	templateID        uuid.UUID
	templateVersionID uuid.UUID
	presetID          uuid.UUID
	presetName        string
	prebuildID        uuid.UUID
	workspaceName     string
}

// templateID is common across all option sets.
var templateID = uuid.UUID{5}

const (
	backoffInterval = time.Second * 5

	optionSet0 = iota
	optionSet1
	optionSet2
)

var opts = map[uint]options{
	optionSet0: {
		templateID:        templateID,
		templateVersionID: uuid.New(),
		presetID:          uuid.New(),
		presetName:        "my-preset",
		prebuildID:        uuid.New(),
		workspaceName:     "prebuilds0",
	},
	optionSet1: {
		templateID:        templateID,
		templateVersionID: uuid.New(),
		presetID:          uuid.New(),
		presetName:        "my-preset",
		prebuildID:        uuid.New(),
		workspaceName:     "prebuilds1",
	},
	optionSet2: {
		templateID:        templateID,
		templateVersionID: uuid.New(),
		presetID:          uuid.New(),
		presetName:        "my-preset",
		prebuildID:        uuid.New(),
		workspaceName:     "prebuilds2",
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

	snapshot := prebuilds.NewGlobalSnapshot(presets, nil, nil, nil)
	ps, err := snapshot.FilterByPreset(current.presetID)
	require.NoError(t, err)

	state := ps.CalculateState()
	actions, err := ps.CalculateActions(clock, backoffInterval)
	require.NoError(t, err)

	validateState(t, prebuilds.ReconciliationState{ /*all zero values*/ }, *state)
	validateActions(t, prebuilds.ReconciliationActions{
		ActionType: prebuilds.ActionTypeCreate,
		Create:     0,
	}, *actions)
}

// A new template version with a preset with prebuilds configured should result in a new prebuild being created.
func TestNetNew(t *testing.T) {
	t.Parallel()
	current := opts[optionSet0]
	clock := quartz.NewMock(t)

	presets := []database.GetTemplatePresetsWithPrebuildsRow{
		preset(true, 1, current),
	}

	snapshot := prebuilds.NewGlobalSnapshot(presets, nil, nil, nil)
	ps, err := snapshot.FilterByPreset(current.presetID)
	require.NoError(t, err)

	state := ps.CalculateState()
	actions, err := ps.CalculateActions(clock, backoffInterval)
	require.NoError(t, err)

	validateState(t, prebuilds.ReconciliationState{
		Desired: 1,
	}, *state)
	validateActions(t, prebuilds.ReconciliationActions{
		ActionType: prebuilds.ActionTypeCreate,
		Create:     1,
	}, *actions)
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
		prebuild(outdated, clock),
	}

	// GIVEN: no in-progress builds.
	var inProgress []database.CountInProgressPrebuildsRow

	// WHEN: calculating the outdated preset's state.
	snapshot := prebuilds.NewGlobalSnapshot(presets, running, inProgress, nil)
	ps, err := snapshot.FilterByPreset(outdated.presetID)
	require.NoError(t, err)

	// THEN: we should identify that this prebuild is outdated and needs to be deleted.
	state := ps.CalculateState()
	actions, err := ps.CalculateActions(clock, backoffInterval)
	require.NoError(t, err)
	validateState(t, prebuilds.ReconciliationState{}, *state)
	validateActions(t, prebuilds.ReconciliationActions{
		ActionType: prebuilds.ActionTypeDelete,
		DeleteIDs:  []uuid.UUID{outdated.prebuildID},
	}, *actions)

	// WHEN: calculating the current preset's state.
	ps, err = snapshot.FilterByPreset(current.presetID)
	require.NoError(t, err)

	// THEN: we should not be blocked from creating a new prebuild while the outdate one deletes.
	state = ps.CalculateState()
	actions, err = ps.CalculateActions(clock, backoffInterval)
	require.NoError(t, err)
	validateState(t, prebuilds.ReconciliationState{Desired: 1}, *state)
	validateActions(t, prebuilds.ReconciliationActions{
		ActionType: prebuilds.ActionTypeCreate,
		Create:     1,
	}, *actions)
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
		prebuild(outdated, clock),
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
	snapshot := prebuilds.NewGlobalSnapshot(presets, running, inProgress, nil)
	ps, err := snapshot.FilterByPreset(outdated.presetID)
	require.NoError(t, err)

	// THEN: we should identify that this prebuild is outdated and needs to be deleted.
	// Despite the fact that deletion of another outdated prebuild is already in progress.
	state := ps.CalculateState()
	actions, err := ps.CalculateActions(clock, backoffInterval)
	require.NoError(t, err)
	validateState(t, prebuilds.ReconciliationState{
		Deleting: 1,
	}, *state)

	validateActions(t, prebuilds.ReconciliationActions{
		ActionType: prebuilds.ActionTypeDelete,
		DeleteIDs:  []uuid.UUID{outdated.prebuildID},
	}, *actions)
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
		checkFn    func(state prebuilds.ReconciliationState, actions prebuilds.ReconciliationActions) bool
	}{
		// With no running prebuilds and one starting, no creations/deletions should take place.
		{
			name:       fmt.Sprintf("%s-short", database.WorkspaceTransitionStart),
			transition: database.WorkspaceTransitionStart,
			desired:    1,
			running:    0,
			inProgress: 1,
			checkFn: func(state prebuilds.ReconciliationState, actions prebuilds.ReconciliationActions) bool {
				return validateState(t, prebuilds.ReconciliationState{Desired: 1, Starting: 1}, state) &&
					validateActions(t, prebuilds.ReconciliationActions{
						ActionType: prebuilds.ActionTypeCreate,
					}, actions)
			},
		},
		// With one running prebuild and one starting, no creations/deletions should occur since we're approaching the correct state.
		{
			name:       fmt.Sprintf("%s-balanced", database.WorkspaceTransitionStart),
			transition: database.WorkspaceTransitionStart,
			desired:    2,
			running:    1,
			inProgress: 1,
			checkFn: func(state prebuilds.ReconciliationState, actions prebuilds.ReconciliationActions) bool {
				return validateState(t, prebuilds.ReconciliationState{Actual: 1, Desired: 2, Starting: 1}, state) &&
					validateActions(t, prebuilds.ReconciliationActions{
						ActionType: prebuilds.ActionTypeCreate,
					}, actions)
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
			checkFn: func(state prebuilds.ReconciliationState, actions prebuilds.ReconciliationActions) bool {
				return validateState(t, prebuilds.ReconciliationState{Actual: 2, Desired: 2, Starting: 1}, state) &&
					validateActions(t, prebuilds.ReconciliationActions{
						ActionType: prebuilds.ActionTypeCreate,
					}, actions)
			},
		},
		// With one prebuild desired and one stopping, a new prebuild will be created.
		{
			name:       fmt.Sprintf("%s-short", database.WorkspaceTransitionStop),
			transition: database.WorkspaceTransitionStop,
			desired:    1,
			running:    0,
			inProgress: 1,
			checkFn: func(state prebuilds.ReconciliationState, actions prebuilds.ReconciliationActions) bool {
				return validateState(t, prebuilds.ReconciliationState{Desired: 1, Stopping: 1}, state) &&
					validateActions(t, prebuilds.ReconciliationActions{
						ActionType: prebuilds.ActionTypeCreate,
						Create:     1,
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
			checkFn: func(state prebuilds.ReconciliationState, actions prebuilds.ReconciliationActions) bool {
				return validateState(t, prebuilds.ReconciliationState{Actual: 2, Desired: 3, Stopping: 1}, state) &&
					validateActions(t, prebuilds.ReconciliationActions{
						ActionType: prebuilds.ActionTypeCreate,
						Create:     1,
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
			checkFn: func(state prebuilds.ReconciliationState, actions prebuilds.ReconciliationActions) bool {
				return validateState(t, prebuilds.ReconciliationState{Actual: 3, Desired: 3, Stopping: 1}, state) &&
					validateActions(t, prebuilds.ReconciliationActions{
						ActionType: prebuilds.ActionTypeCreate,
					}, actions)
			},
		},
		// With one prebuild desired and one deleting, a new prebuild will be created.
		{
			name:       fmt.Sprintf("%s-short", database.WorkspaceTransitionDelete),
			transition: database.WorkspaceTransitionDelete,
			desired:    1,
			running:    0,
			inProgress: 1,
			checkFn: func(state prebuilds.ReconciliationState, actions prebuilds.ReconciliationActions) bool {
				return validateState(t, prebuilds.ReconciliationState{Desired: 1, Deleting: 1}, state) &&
					validateActions(t, prebuilds.ReconciliationActions{
						ActionType: prebuilds.ActionTypeCreate,
						Create:     1,
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
			checkFn: func(state prebuilds.ReconciliationState, actions prebuilds.ReconciliationActions) bool {
				return validateState(t, prebuilds.ReconciliationState{Actual: 1, Desired: 2, Deleting: 1}, state) &&
					validateActions(t, prebuilds.ReconciliationActions{
						ActionType: prebuilds.ActionTypeCreate,
						Create:     1,
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
			checkFn: func(state prebuilds.ReconciliationState, actions prebuilds.ReconciliationActions) bool {
				return validateState(t, prebuilds.ReconciliationState{Actual: 2, Desired: 2, Deleting: 1}, state) &&
					validateActions(t, prebuilds.ReconciliationActions{
						ActionType: prebuilds.ActionTypeCreate,
					}, actions)
			},
		},
		// With 3 prebuilds desired, 1 running, and 2 starting, no creations should occur since the builds are in progress.
		{
			name:       fmt.Sprintf("%s-inhibit", database.WorkspaceTransitionStart),
			transition: database.WorkspaceTransitionStart,
			desired:    3,
			running:    1,
			inProgress: 2,
			checkFn: func(state prebuilds.ReconciliationState, actions prebuilds.ReconciliationActions) bool {
				return validateState(t, prebuilds.ReconciliationState{Actual: 1, Desired: 3, Starting: 2}, state) &&
					validateActions(t, prebuilds.ReconciliationActions{ActionType: prebuilds.ActionTypeCreate, Create: 0}, actions)
			},
		},
		// With 3 prebuilds desired, 5 running, and 2 deleting, no deletions should occur since the builds are in progress.
		{
			name:       fmt.Sprintf("%s-inhibit", database.WorkspaceTransitionDelete),
			transition: database.WorkspaceTransitionDelete,
			desired:    3,
			running:    5,
			inProgress: 2,
			checkFn: func(state prebuilds.ReconciliationState, actions prebuilds.ReconciliationActions) bool {
				expectedState := prebuilds.ReconciliationState{Actual: 5, Desired: 3, Deleting: 2, Extraneous: 2}
				expectedActions := prebuilds.ReconciliationActions{
					ActionType: prebuilds.ActionTypeDelete,
				}

				return validateState(t, expectedState, state) &&
					assert.EqualValuesf(t, expectedActions.ActionType, actions.ActionType, "'ActionType' did not match expectation") &&
					assert.Len(t, actions.DeleteIDs, 2, "'deleteIDs' did not match expectation") &&
					assert.EqualValuesf(t, expectedActions.Create, actions.Create, "'create' did not match expectation") &&
					assert.EqualValuesf(t, expectedActions.BackoffUntil, actions.BackoffUntil, "'BackoffUntil' did not match expectation")
			},
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			// GIVEN: a preset.
			defaultPreset := preset(true, tc.desired, current)
			presets := []database.GetTemplatePresetsWithPrebuildsRow{
				defaultPreset,
			}

			// GIVEN: a running prebuild for the preset.
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

			// GIVEN: one prebuild for the old preset which is currently transitioning.
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
			snapshot := prebuilds.NewGlobalSnapshot(presets, running, inProgress, nil)
			ps, err := snapshot.FilterByPreset(current.presetID)
			require.NoError(t, err)

			// THEN: we should identify that this prebuild is in progress.
			state := ps.CalculateState()
			actions, err := ps.CalculateActions(clock, backoffInterval)
			require.NoError(t, err)
			require.True(t, tc.checkFn(*state, *actions))
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
		prebuild(current, clock, func(row database.GetRunningPrebuiltWorkspacesRow) database.GetRunningPrebuiltWorkspacesRow {
			// The older of the running prebuilds will be deleted in order to maintain freshness.
			row.CreatedAt = clock.Now().Add(-time.Hour)
			older = row.ID
			return row
		}),
		prebuild(current, clock, func(row database.GetRunningPrebuiltWorkspacesRow) database.GetRunningPrebuiltWorkspacesRow {
			row.CreatedAt = clock.Now()
			return row
		}),
	}

	// GIVEN: NO prebuilds in progress.
	var inProgress []database.CountInProgressPrebuildsRow

	// WHEN: calculating the current preset's state.
	snapshot := prebuilds.NewGlobalSnapshot(presets, running, inProgress, nil)
	ps, err := snapshot.FilterByPreset(current.presetID)
	require.NoError(t, err)

	// THEN: an extraneous prebuild is detected and marked for deletion.
	state := ps.CalculateState()
	actions, err := ps.CalculateActions(clock, backoffInterval)
	require.NoError(t, err)
	validateState(t, prebuilds.ReconciliationState{
		Actual: 2, Desired: 1, Extraneous: 1, Eligible: 2,
	}, *state)
	validateActions(t, prebuilds.ReconciliationActions{
		ActionType: prebuilds.ActionTypeDelete,
		DeleteIDs:  []uuid.UUID{older},
	}, *actions)
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
		prebuild(current, clock),
	}

	// GIVEN: NO prebuilds in progress.
	var inProgress []database.CountInProgressPrebuildsRow

	// WHEN: calculating the current preset's state.
	snapshot := prebuilds.NewGlobalSnapshot(presets, running, inProgress, nil)
	ps, err := snapshot.FilterByPreset(current.presetID)
	require.NoError(t, err)

	// THEN: all running prebuilds should be deleted because the template is deprecated.
	state := ps.CalculateState()
	actions, err := ps.CalculateActions(clock, backoffInterval)
	require.NoError(t, err)
	validateState(t, prebuilds.ReconciliationState{}, *state)
	validateActions(t, prebuilds.ReconciliationActions{
		ActionType: prebuilds.ActionTypeDelete,
		DeleteIDs:  []uuid.UUID{current.prebuildID},
	}, *actions)
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
		prebuild(other, clock),
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
	snapshot := prebuilds.NewGlobalSnapshot(presets, running, inProgress, backoffs)
	psCurrent, err := snapshot.FilterByPreset(current.presetID)
	require.NoError(t, err)

	// THEN: reconciliation should backoff.
	state := psCurrent.CalculateState()
	actions, err := psCurrent.CalculateActions(clock, backoffInterval)
	require.NoError(t, err)
	validateState(t, prebuilds.ReconciliationState{
		Actual: 0, Desired: 1,
	}, *state)
	validateActions(t, prebuilds.ReconciliationActions{
		ActionType:   prebuilds.ActionTypeBackoff,
		BackoffUntil: lastBuildTime.Add(time.Duration(numFailed) * backoffInterval),
	}, *actions)

	// WHEN: calculating the other preset's state.
	psOther, err := snapshot.FilterByPreset(other.presetID)
	require.NoError(t, err)

	// THEN: it should NOT be in backoff because all is OK.
	state = psOther.CalculateState()
	actions, err = psOther.CalculateActions(clock, backoffInterval)
	require.NoError(t, err)
	validateState(t, prebuilds.ReconciliationState{
		Actual: 1, Desired: 1, Eligible: 1,
	}, *state)
	validateActions(t, prebuilds.ReconciliationActions{
		ActionType:   prebuilds.ActionTypeCreate,
		BackoffUntil: time.Time{},
	}, *actions)

	// WHEN: the clock is advanced a backoff interval.
	clock.Advance(backoffInterval + time.Microsecond)

	// THEN: a new prebuild should be created.
	psCurrent, err = snapshot.FilterByPreset(current.presetID)
	require.NoError(t, err)
	state = psCurrent.CalculateState()
	actions, err = psCurrent.CalculateActions(clock, backoffInterval)
	require.NoError(t, err)
	validateState(t, prebuilds.ReconciliationState{
		Actual: 0, Desired: 1,
	}, *state)
	validateActions(t, prebuilds.ReconciliationActions{
		ActionType: prebuilds.ActionTypeCreate,
		Create:     1, // <--- NOTE: we're now able to create a new prebuild because the interval has elapsed.

	}, *actions)
}

func TestMultiplePresetsPerTemplateVersion(t *testing.T) {
	t.Parallel()

	templateID := uuid.New()
	templateVersionID := uuid.New()
	presetOpts1 := options{
		templateID:        templateID,
		templateVersionID: templateVersionID,
		presetID:          uuid.New(),
		presetName:        "my-preset-1",
		prebuildID:        uuid.New(),
		workspaceName:     "prebuilds1",
	}
	presetOpts2 := options{
		templateID:        templateID,
		templateVersionID: templateVersionID,
		presetID:          uuid.New(),
		presetName:        "my-preset-2",
		prebuildID:        uuid.New(),
		workspaceName:     "prebuilds2",
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

	snapshot := prebuilds.NewGlobalSnapshot(presets, nil, inProgress, nil)

	// Nothing has to be created for preset 1.
	{
		ps, err := snapshot.FilterByPreset(presetOpts1.presetID)
		require.NoError(t, err)

		state := ps.CalculateState()
		actions, err := ps.CalculateActions(clock, backoffInterval)
		require.NoError(t, err)

		validateState(t, prebuilds.ReconciliationState{
			Starting: 1,
			Desired:  1,
		}, *state)
		validateActions(t, prebuilds.ReconciliationActions{
			ActionType: prebuilds.ActionTypeCreate,
			Create:     0,
		}, *actions)
	}

	// One prebuild has to be created for preset 2. Make sure preset 1 doesn't block preset 2.
	{
		ps, err := snapshot.FilterByPreset(presetOpts2.presetID)
		require.NoError(t, err)

		state := ps.CalculateState()
		actions, err := ps.CalculateActions(clock, backoffInterval)
		require.NoError(t, err)

		validateState(t, prebuilds.ReconciliationState{
			Starting: 0,
			Desired:  1,
		}, *state)
		validateActions(t, prebuilds.ReconciliationActions{
			ActionType: prebuilds.ActionTypeCreate,
			Create:     1,
		}, *actions)
	}
}

func preset(active bool, instances int32, opts options, muts ...func(row database.GetTemplatePresetsWithPrebuildsRow) database.GetTemplatePresetsWithPrebuildsRow) database.GetTemplatePresetsWithPrebuildsRow {
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
	}

	for _, mut := range muts {
		entry = mut(entry)
	}
	return entry
}

func prebuild(
	opts options,
	clock quartz.Clock,
	muts ...func(row database.GetRunningPrebuiltWorkspacesRow) database.GetRunningPrebuiltWorkspacesRow,
) database.GetRunningPrebuiltWorkspacesRow {
	entry := database.GetRunningPrebuiltWorkspacesRow{
		ID:                opts.prebuildID,
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

func validateState(t *testing.T, expected, actual prebuilds.ReconciliationState) bool {
	return assert.EqualValuesf(t, expected.Desired, actual.Desired, "'desired' did not match expectation") &&
		assert.EqualValuesf(t, expected.Actual, actual.Actual, "'actual' did not match expectation") &&
		assert.EqualValuesf(t, expected.Eligible, actual.Eligible, "'eligible' did not match expectation") &&
		assert.EqualValuesf(t, expected.Extraneous, actual.Extraneous, "'extraneous' did not match expectation") &&
		assert.EqualValuesf(t, expected.Starting, actual.Starting, "'starting' did not match expectation") &&
		assert.EqualValuesf(t, expected.Stopping, actual.Stopping, "'stopping' did not match expectation") &&
		assert.EqualValuesf(t, expected.Deleting, actual.Deleting, "'deleting' did not match expectation")
}

// validateActions is a convenience func to make tests more readable; it exploits the fact that the default states for
// prebuilds align with zero values.
func validateActions(t *testing.T, expected, actual prebuilds.ReconciliationActions) bool {
	return assert.EqualValuesf(t, expected.ActionType, actual.ActionType, "'ActionType' did not match expectation") &&
		assert.EqualValuesf(t, expected.DeleteIDs, actual.DeleteIDs, "'deleteIDs' did not match expectation") &&
		assert.EqualValuesf(t, expected.Create, actual.Create, "'create' did not match expectation") &&
		assert.EqualValuesf(t, expected.BackoffUntil, actual.BackoffUntil, "'BackoffUntil' did not match expectation")
}
