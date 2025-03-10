package prebuilds

import (
	"fmt"
	"testing"
	"time"

	"github.com/coder/quartz"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/database"
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
var templateID = uuid.New()

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
	current := opts[optionSet0]
	clock := quartz.NewMock(t)

	presets := []database.GetTemplatePresetsWithPrebuildsRow{
		preset(true, 0, current),
	}

	state := newReconciliationState(presets, nil, nil, nil)
	ps, err := state.filterByPreset(current.presetID)
	require.NoError(t, err)

	actions, err := ps.calculateActions(clock, backoffInterval)
	require.NoError(t, err)

	validateActions(t, reconciliationActions{ /*all zero values*/ }, *actions)
}

// A new template version with a preset with prebuilds configured should result in a new prebuild being created.
func TestNetNew(t *testing.T) {
	current := opts[optionSet0]
	clock := quartz.NewMock(t)

	presets := []database.GetTemplatePresetsWithPrebuildsRow{
		preset(true, 1, current),
	}

	state := newReconciliationState(presets, nil, nil, nil)
	ps, err := state.filterByPreset(current.presetID)
	require.NoError(t, err)

	actions, err := ps.calculateActions(clock, backoffInterval)
	require.NoError(t, err)

	validateActions(t, reconciliationActions{
		desired: 1,
		create:  1,
	}, *actions)
}

// A new template version is created with a preset with prebuilds configured; this outdates the older version and
// requires the old prebuilds to be destroyed and new prebuilds to be created.
func TestOutdatedPrebuilds(t *testing.T) {
	outdated := opts[optionSet0]
	current := opts[optionSet1]
	clock := quartz.NewMock(t)

	// GIVEN: 2 presets, one outdated and one new.
	presets := []database.GetTemplatePresetsWithPrebuildsRow{
		preset(false, 1, outdated),
		preset(true, 1, current),
	}

	// GIVEN: a running prebuild for the outdated preset.
	running := []database.GetRunningPrebuildsRow{
		prebuild(outdated, clock),
	}

	// GIVEN: no in-progress builds.
	var inProgress []database.GetPrebuildsInProgressRow

	// WHEN: calculating the outdated preset's state.
	state := newReconciliationState(presets, running, inProgress, nil)
	ps, err := state.filterByPreset(outdated.presetID)
	require.NoError(t, err)

	// THEN: we should identify that this prebuild is outdated and needs to be deleted.
	actions, err := ps.calculateActions(clock, backoffInterval)
	require.NoError(t, err)
	validateActions(t, reconciliationActions{outdated: 1, deleteIDs: []uuid.UUID{outdated.prebuildID}}, *actions)

	// WHEN: calculating the current preset's state.
	ps, err = state.filterByPreset(current.presetID)
	require.NoError(t, err)

	// THEN: we should not be blocked from creating a new prebuild while the outdate one deletes.
	actions, err = ps.calculateActions(clock, backoffInterval)
	require.NoError(t, err)
	validateActions(t, reconciliationActions{desired: 1, create: 1}, *actions)
}

// A new template version is created with a preset with prebuilds configured; while a prebuild is provisioning up or down,
// the calculated actions should indicate the state correctly.
func TestInProgressActions(t *testing.T) {
	current := opts[optionSet0]
	clock := quartz.NewMock(t)

	cases := []struct {
		name       string
		transition database.WorkspaceTransition
		desired    int32
		running    int32
		checkFn    func(actions reconciliationActions) bool
	}{
		// With no running prebuilds and one starting, no creations/deletions should take place.
		{
			name:       fmt.Sprintf("%s-short", database.WorkspaceTransitionStart),
			transition: database.WorkspaceTransitionStart,
			desired:    1,
			running:    0,
			checkFn: func(actions reconciliationActions) bool {
				return assert.True(t, validateActions(t, reconciliationActions{desired: 1, starting: 1}, actions))
			},
		},
		// With one running prebuild and one starting, no creations/deletions should occur since we're approaching the correct state.
		{
			name:       fmt.Sprintf("%s-balanced", database.WorkspaceTransitionStart),
			transition: database.WorkspaceTransitionStart,
			desired:    2,
			running:    1,
			checkFn: func(actions reconciliationActions) bool {
				return assert.True(t, validateActions(t, reconciliationActions{actual: 1, desired: 2, starting: 1}, actions))
			},
		},
		// With one running prebuild and one starting, no creations/deletions should occur
		// SIDE-NOTE: once the starting prebuild completes, the older of the two will be considered extraneous since we only desire 2.
		{
			name:       fmt.Sprintf("%s-extraneous", database.WorkspaceTransitionStart),
			transition: database.WorkspaceTransitionStart,
			desired:    2,
			running:    2,
			checkFn: func(actions reconciliationActions) bool {
				return assert.True(t, validateActions(t, reconciliationActions{actual: 2, desired: 2, starting: 1}, actions))
			},
		},
		// With one prebuild desired and one stopping, a new prebuild will be created.
		{
			name:       fmt.Sprintf("%s-short", database.WorkspaceTransitionStop),
			transition: database.WorkspaceTransitionStop,
			desired:    1,
			running:    0,
			checkFn: func(actions reconciliationActions) bool {
				return assert.True(t, validateActions(t, reconciliationActions{desired: 1, stopping: 1, create: 1}, actions))
			},
		},
		// With 3 prebuilds desired, 2 running, and 1 stopping, a new prebuild will be created.
		{
			name:       fmt.Sprintf("%s-balanced", database.WorkspaceTransitionStop),
			transition: database.WorkspaceTransitionStop,
			desired:    3,
			running:    2,
			checkFn: func(actions reconciliationActions) bool {
				return assert.True(t, validateActions(t, reconciliationActions{actual: 2, desired: 3, stopping: 1, create: 1}, actions))
			},
		},
		// With 3 prebuilds desired, 3 running, and 1 stopping, no creations/deletions should occur since the desired state is already achieved.
		{
			name:       fmt.Sprintf("%s-extraneous", database.WorkspaceTransitionStop),
			transition: database.WorkspaceTransitionStop,
			desired:    3,
			running:    3,
			checkFn: func(actions reconciliationActions) bool {
				return assert.True(t, validateActions(t, reconciliationActions{actual: 3, desired: 3, stopping: 1}, actions))
			},
		},
		// With one prebuild desired and one deleting, a new prebuild will be created.
		{
			name:       fmt.Sprintf("%s-short", database.WorkspaceTransitionDelete),
			transition: database.WorkspaceTransitionDelete,
			desired:    1,
			running:    0,
			checkFn: func(actions reconciliationActions) bool {
				return assert.True(t, validateActions(t, reconciliationActions{desired: 1, deleting: 1, create: 1}, actions))
			},
		},
		// With 2 prebuilds desired, 1 running, and 1 deleting, a new prebuild will be created.
		{
			name:       fmt.Sprintf("%s-balanced", database.WorkspaceTransitionDelete),
			transition: database.WorkspaceTransitionDelete,
			desired:    2,
			running:    1,
			checkFn: func(actions reconciliationActions) bool {
				return assert.True(t, validateActions(t, reconciliationActions{actual: 1, desired: 2, deleting: 1, create: 1}, actions))
			},
		},
		// With 2 prebuilds desired, 2 running, and 1 deleting, no creations/deletions should occur since the desired state is already achieved.
		{
			name:       fmt.Sprintf("%s-extraneous", database.WorkspaceTransitionDelete),
			transition: database.WorkspaceTransitionDelete,
			desired:    2,
			running:    2,
			checkFn: func(actions reconciliationActions) bool {
				return assert.True(t, validateActions(t, reconciliationActions{actual: 2, desired: 2, deleting: 1}, actions))
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			// GIVEN: a presets.
			presets := []database.GetTemplatePresetsWithPrebuildsRow{
				preset(true, tc.desired, current),
			}

			// GIVEN: a running prebuild for the preset.
			running := make([]database.GetRunningPrebuildsRow, 0, tc.running)
			for range tc.running {
				name, err := generateName()
				require.NoError(t, err)
				running = append(running, database.GetRunningPrebuildsRow{
					WorkspaceID:       uuid.New(),
					WorkspaceName:     name,
					TemplateID:        current.templateID,
					TemplateVersionID: current.templateVersionID,
					CurrentPresetID:   uuid.NullUUID{UUID: current.presetID, Valid: true},
					Ready:             false,
					CreatedAt:         clock.Now(),
				})
			}

			// GIVEN: one prebuild for the old preset which is currently transitioning.
			inProgress := []database.GetPrebuildsInProgressRow{
				{
					TemplateID:        current.templateID,
					TemplateVersionID: current.templateVersionID,
					Transition:        tc.transition,
					Count:             1,
				},
			}

			// WHEN: calculating the current preset's state.
			state := newReconciliationState(presets, running, inProgress, nil)
			ps, err := state.filterByPreset(current.presetID)
			require.NoError(t, err)

			// THEN: we should identify that this prebuild is in progress.
			actions, err := ps.calculateActions(clock, backoffInterval)
			require.NoError(t, err)
			require.True(t, tc.checkFn(*actions))
		})
	}
}

// Additional prebuilds exist for a given preset configuration; these must be deleted.
func TestExtraneous(t *testing.T) {
	current := opts[optionSet0]
	clock := quartz.NewMock(t)

	// GIVEN: a preset with 1 desired prebuild.
	presets := []database.GetTemplatePresetsWithPrebuildsRow{
		preset(true, 1, current),
	}

	var older uuid.UUID
	// GIVEN: 2 running prebuilds for the preset.
	running := []database.GetRunningPrebuildsRow{
		prebuild(current, clock, func(row database.GetRunningPrebuildsRow) database.GetRunningPrebuildsRow {
			// The older of the running prebuilds will be deleted in order to maintain freshness.
			row.CreatedAt = clock.Now().Add(-time.Hour)
			older = row.WorkspaceID
			return row
		}),
		prebuild(current, clock, func(row database.GetRunningPrebuildsRow) database.GetRunningPrebuildsRow {
			row.CreatedAt = clock.Now()
			return row
		}),
	}

	// GIVEN: NO prebuilds in progress.
	var inProgress []database.GetPrebuildsInProgressRow

	// WHEN: calculating the current preset's state.
	state := newReconciliationState(presets, running, inProgress, nil)
	ps, err := state.filterByPreset(current.presetID)
	require.NoError(t, err)

	// THEN: an extraneous prebuild is detected and marked for deletion.
	actions, err := ps.calculateActions(clock, backoffInterval)
	require.NoError(t, err)
	validateActions(t, reconciliationActions{
		actual: 2, desired: 1, extraneous: 1, deleteIDs: []uuid.UUID{older}, eligible: 2,
	}, *actions)
}

// As above, but no actions will be performed because
func TestExtraneousInProgress(t *testing.T) {
	current := opts[optionSet0]
	clock := quartz.NewMock(t)

	// GIVEN: a preset with 1 desired prebuild.
	presets := []database.GetTemplatePresetsWithPrebuildsRow{
		preset(true, 1, current),
	}

	var older uuid.UUID
	// GIVEN: 2 running prebuilds for the preset.
	running := []database.GetRunningPrebuildsRow{
		prebuild(current, clock, func(row database.GetRunningPrebuildsRow) database.GetRunningPrebuildsRow {
			// The older of the running prebuilds will be deleted in order to maintain freshness.
			row.CreatedAt = clock.Now().Add(-time.Hour)
			older = row.WorkspaceID
			return row
		}),
		prebuild(current, clock, func(row database.GetRunningPrebuildsRow) database.GetRunningPrebuildsRow {
			row.CreatedAt = clock.Now()
			return row
		}),
	}

	// GIVEN: NO prebuilds in progress.
	var inProgress []database.GetPrebuildsInProgressRow

	// WHEN: calculating the current preset's state.
	state := newReconciliationState(presets, running, inProgress, nil)
	ps, err := state.filterByPreset(current.presetID)
	require.NoError(t, err)

	// THEN: an extraneous prebuild is detected and marked for deletion.
	actions, err := ps.calculateActions(clock, backoffInterval)
	require.NoError(t, err)
	validateActions(t, reconciliationActions{
		actual: 2, desired: 1, extraneous: 1, deleteIDs: []uuid.UUID{older}, eligible: 2,
	}, *actions)
}

// A template marked as deprecated will not have prebuilds running.
func TestDeprecated(t *testing.T) {
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
	running := []database.GetRunningPrebuildsRow{
		prebuild(current, clock),
	}

	// GIVEN: NO prebuilds in progress.
	var inProgress []database.GetPrebuildsInProgressRow

	// WHEN: calculating the current preset's state.
	state := newReconciliationState(presets, running, inProgress, nil)
	ps, err := state.filterByPreset(current.presetID)
	require.NoError(t, err)

	// THEN: all running prebuilds should be deleted because the template is deprecated.
	actions, err := ps.calculateActions(clock, backoffInterval)
	require.NoError(t, err)
	validateActions(t, reconciliationActions{
		actual: 1, deleteIDs: []uuid.UUID{current.prebuildID}, eligible: 1,
	}, *actions)
}

// If the latest build failed, backoff exponentially with the given interval.
func TestLatestBuildFailed(t *testing.T) {
	current := opts[optionSet0]
	other := opts[optionSet1]
	clock := quartz.NewMock(t)

	// GIVEN: two presets.
	presets := []database.GetTemplatePresetsWithPrebuildsRow{
		preset(true, 1, current),
		preset(true, 1, other),
	}

	// GIVEN: running prebuilds only for one preset (the other will be failing, as evidenced by the backoffs below).
	running := []database.GetRunningPrebuildsRow{
		prebuild(other, clock),
	}

	// GIVEN: NO prebuilds in progress.
	var inProgress []database.GetPrebuildsInProgressRow

	// GIVEN: a backoff entry.
	lastBuildTime := clock.Now()
	numFailed := 1
	backoffs := []database.GetPresetsBackoffRow{
		{
			TemplateVersionID: current.templateVersionID,
			PresetID:          current.presetID,
			LatestBuildStatus: database.ProvisionerJobStatusFailed,
			NumFailed:         int32(numFailed),
			LastBuildAt:       lastBuildTime,
		},
	}

	// WHEN: calculating the current preset's state.
	state := newReconciliationState(presets, running, inProgress, backoffs)
	psCurrent, err := state.filterByPreset(current.presetID)
	require.NoError(t, err)

	// THEN: reconciliation should backoff.
	actions, err := psCurrent.calculateActions(clock, backoffInterval)
	require.NoError(t, err)
	validateActions(t, reconciliationActions{
		actual: 0, desired: 1, backoffUntil: lastBuildTime.Add(time.Duration(numFailed) * backoffInterval),
	}, *actions)

	// WHEN: calculating the other preset's state.
	psOther, err := state.filterByPreset(other.presetID)
	require.NoError(t, err)

	// THEN: it should NOT be in backoff because all is OK.
	actions, err = psOther.calculateActions(clock, backoffInterval)
	require.NoError(t, err)
	validateActions(t, reconciliationActions{
		actual: 1, desired: 1, eligible: 1, backoffUntil: time.Time{},
	}, *actions)

	// WHEN: the clock is advanced a backoff interval.
	clock.Advance(backoffInterval + time.Microsecond)

	// THEN: a new prebuild should be created.
	psCurrent, err = state.filterByPreset(current.presetID)
	require.NoError(t, err)
	actions, err = psCurrent.calculateActions(clock, backoffInterval)
	require.NoError(t, err)
	validateActions(t, reconciliationActions{
		create: 1, // <--- NOTE: we're now able to create a new prebuild because the interval has elapsed.
		actual: 0, desired: 1, backoffUntil: lastBuildTime.Add(time.Duration(numFailed) * backoffInterval),
	}, *actions)
}

func preset(active bool, instances int32, opts options, muts ...func(row database.GetTemplatePresetsWithPrebuildsRow) database.GetTemplatePresetsWithPrebuildsRow) database.GetTemplatePresetsWithPrebuildsRow {
	entry := database.GetTemplatePresetsWithPrebuildsRow{
		TemplateID:         opts.templateID,
		TemplateVersionID:  opts.templateVersionID,
		PresetID:           opts.presetID,
		UsingActiveVersion: active,
		Name:               opts.presetName,
		DesiredInstances:   instances,
		Deleted:            false,
		Deprecated:         false,
	}

	for _, mut := range muts {
		entry = mut(entry)
	}
	return entry
}

func prebuild(opts options, clock quartz.Clock, muts ...func(row database.GetRunningPrebuildsRow) database.GetRunningPrebuildsRow) database.GetRunningPrebuildsRow {
	entry := database.GetRunningPrebuildsRow{
		WorkspaceID:       opts.prebuildID,
		WorkspaceName:     opts.workspaceName,
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

// validateActions is a convenience func to make tests more readable; it exploits the fact that the default states for
// prebuilds align with zero values.
func validateActions(t *testing.T, expected, actual reconciliationActions) bool {
	return assert.EqualValuesf(t, expected.deleteIDs, actual.deleteIDs, "'deleteIDs' did not match expectation") &&
		assert.EqualValuesf(t, expected.create, actual.create, "'create' did not match expectation") &&
		assert.EqualValuesf(t, expected.desired, actual.desired, "'desired' did not match expectation") &&
		assert.EqualValuesf(t, expected.actual, actual.actual, "'actual' did not match expectation") &&
		assert.EqualValuesf(t, expected.eligible, actual.eligible, "'eligible' did not match expectation") &&
		assert.EqualValuesf(t, expected.extraneous, actual.extraneous, "'extraneous' did not match expectation") &&
		assert.EqualValuesf(t, expected.outdated, actual.outdated, "'outdated' did not match expectation") &&
		assert.EqualValuesf(t, expected.starting, actual.starting, "'starting' did not match expectation") &&
		assert.EqualValuesf(t, expected.stopping, actual.stopping, "'stopping' did not match expectation") &&
		assert.EqualValuesf(t, expected.deleting, actual.deleting, "'deleting' did not match expectation")
}
