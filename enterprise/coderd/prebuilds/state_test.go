package prebuilds

import (
	"testing"
	"time"

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

	presets := []database.GetTemplatePresetsWithPrebuildsRow{
		preset(true, 0, current),
	}

	state := newReconciliationState(presets, nil, nil)
	ps, err := state.filterByPreset(current.presetID)
	require.NoError(t, err)

	actions, err := ps.calculateActions()
	require.NoError(t, err)

	validateActions(t, reconciliationActions{ /*all zero values*/ }, *actions)
}

// A new template version with a preset with prebuilds configured should result in a new prebuild being created.
func TestNetNew(t *testing.T) {
	current := opts[optionSet0]

	presets := []database.GetTemplatePresetsWithPrebuildsRow{
		preset(true, 1, current),
	}

	state := newReconciliationState(presets, nil, nil)
	ps, err := state.filterByPreset(current.presetID)
	require.NoError(t, err)

	actions, err := ps.calculateActions()
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

	// GIVEN: 2 presets, one outdated and one new.
	presets := []database.GetTemplatePresetsWithPrebuildsRow{
		preset(false, 1, outdated),
		preset(true, 1, current),
	}

	// GIVEN: a running prebuild for the outdated preset.
	running := []database.GetRunningPrebuildsRow{
		prebuild(outdated),
	}

	// GIVEN: no in-progress builds.
	var inProgress []database.GetPrebuildsInProgressRow

	// WHEN: calculating the outdated preset's state.
	state := newReconciliationState(presets, running, inProgress)
	ps, err := state.filterByPreset(outdated.presetID)
	require.NoError(t, err)

	// THEN: we should identify that this prebuild is outdated and needs to be deleted.
	actions, err := ps.calculateActions()
	require.NoError(t, err)
	validateActions(t, reconciliationActions{outdated: 1, deleteIDs: []uuid.UUID{outdated.prebuildID}}, *actions)

	// WHEN: calculating the current preset's state.
	ps, err = state.filterByPreset(current.presetID)
	require.NoError(t, err)

	// THEN: we should not be blocked from creating a new prebuild while the outdate one deletes.
	actions, err = ps.calculateActions()
	require.NoError(t, err)
	validateActions(t, reconciliationActions{desired: 1, create: 1}, *actions)
}

// A new template version is created with a preset with prebuilds configured; while the outdated prebuild is deleting,
// the new preset's prebuild cannot be provisioned concurrently, to prevent clobbering.
func TestBlockedOnDeleteActions(t *testing.T) {
	outdated := opts[optionSet0]
	current := opts[optionSet1]

	// GIVEN: 2 presets, one outdated and one new.
	presets := []database.GetTemplatePresetsWithPrebuildsRow{
		preset(false, 1, outdated),
		preset(true, 1, current),
	}

	// GIVEN: a running prebuild for the outdated preset.
	running := []database.GetRunningPrebuildsRow{
		prebuild(outdated),
	}

	// GIVEN: one prebuild for the old preset which is currently deleting.
	inProgress := []database.GetPrebuildsInProgressRow{
		{
			TemplateID:        outdated.templateID,
			TemplateVersionID: outdated.templateVersionID,
			Transition:        database.WorkspaceTransitionDelete,
			Count:             1,
		},
	}

	// WHEN: calculating the outdated preset's state.
	state := newReconciliationState(presets, running, inProgress)
	ps, err := state.filterByPreset(outdated.presetID)
	require.NoError(t, err)

	// THEN: we should identify that this prebuild is in progress, and not attempt to delete this prebuild again.
	actions, err := ps.calculateActions()
	require.NoError(t, err)
	validateActions(t, reconciliationActions{outdated: 1, deleting: 1}, *actions)

	// WHEN: calculating the current preset's state.
	ps, err = state.filterByPreset(current.presetID)
	require.NoError(t, err)

	// THEN: we are blocked from creating a new prebuild while another one is busy provisioning.
	actions, err = ps.calculateActions()
	require.NoError(t, err)
	validateActions(t, reconciliationActions{desired: 1, create: 0, deleting: 1}, *actions)
}

// A new template version is created with a preset with prebuilds configured. An operator comes along and stops one of the
// running prebuilds (this shouldn't be done, but it's possible). While this prebuild is stopping, all other prebuild
// actions are blocked.
func TestBlockedOnStopActions(t *testing.T) {
	outdated := opts[optionSet0]
	current := opts[optionSet1]

	// GIVEN: 2 presets, one outdated and one new (which now expects 2 prebuilds!).
	presets := []database.GetTemplatePresetsWithPrebuildsRow{
		preset(false, 1, outdated),
		preset(true, 2, current),
	}

	// GIVEN: NO running prebuilds for either preset.
	var running []database.GetRunningPrebuildsRow

	// GIVEN: one prebuild for the old preset which is currently stopping.
	inProgress := []database.GetPrebuildsInProgressRow{
		{
			TemplateID:        outdated.templateID,
			TemplateVersionID: outdated.templateVersionID,
			Transition:        database.WorkspaceTransitionStop,
			Count:             1,
		},
	}

	// WHEN: calculating the outdated preset's state.
	state := newReconciliationState(presets, running, inProgress)
	ps, err := state.filterByPreset(outdated.presetID)
	require.NoError(t, err)

	// THEN: there is nothing to do.
	actions, err := ps.calculateActions()
	require.NoError(t, err)
	validateActions(t, reconciliationActions{stopping: 1}, *actions)

	// WHEN: calculating the current preset's state.
	ps, err = state.filterByPreset(current.presetID)
	require.NoError(t, err)

	// THEN: we are blocked from creating a new prebuild while another one is busy provisioning.
	actions, err = ps.calculateActions()
	require.NoError(t, err)
	validateActions(t, reconciliationActions{desired: 2, stopping: 1, create: 0}, *actions)
}

// A new template version is created with a preset with prebuilds configured; the outdated prebuilds are deleted,
// and one of the new prebuilds is already being provisioned, but we bail out early if operations are already in progress
// for this prebuild - to prevent clobbering.
func TestBlockedOnStartActions(t *testing.T) {
	outdated := opts[optionSet0]
	current := opts[optionSet1]

	// GIVEN: 2 presets, one outdated and one new (which now expects 2 prebuilds!).
	presets := []database.GetTemplatePresetsWithPrebuildsRow{
		preset(false, 1, outdated),
		preset(true, 2, current),
	}

	// GIVEN: NO running prebuilds for either preset.
	var running []database.GetRunningPrebuildsRow

	// GIVEN: one prebuild for the old preset which is currently provisioning.
	inProgress := []database.GetPrebuildsInProgressRow{
		{
			TemplateID:        current.templateID,
			TemplateVersionID: current.templateVersionID,
			Transition:        database.WorkspaceTransitionStart,
			Count:             1,
		},
	}

	// WHEN: calculating the outdated preset's state.
	state := newReconciliationState(presets, running, inProgress)
	ps, err := state.filterByPreset(outdated.presetID)
	require.NoError(t, err)

	// THEN: there is nothing to do.
	actions, err := ps.calculateActions()
	require.NoError(t, err)
	validateActions(t, reconciliationActions{starting: 1}, *actions)

	// WHEN: calculating the current preset's state.
	ps, err = state.filterByPreset(current.presetID)
	require.NoError(t, err)

	// THEN: we are blocked from creating a new prebuild while another one is busy provisioning.
	actions, err = ps.calculateActions()
	require.NoError(t, err)
	validateActions(t, reconciliationActions{desired: 2, starting: 1, create: 0}, *actions)
}

// Additional prebuilds exist for a given preset configuration; these must be deleted.
func TestExtraneous(t *testing.T) {
	current := opts[optionSet0]

	// GIVEN: a preset with 1 desired prebuild.
	presets := []database.GetTemplatePresetsWithPrebuildsRow{
		preset(true, 1, current),
	}

	var older uuid.UUID
	// GIVEN: 2 running prebuilds for the preset.
	running := []database.GetRunningPrebuildsRow{
		prebuild(current, func(row database.GetRunningPrebuildsRow) database.GetRunningPrebuildsRow {
			// The older of the running prebuilds will be deleted in order to maintain freshness.
			row.CreatedAt = time.Now().Add(-time.Hour)
			older = row.WorkspaceID
			return row
		}),
		prebuild(current, func(row database.GetRunningPrebuildsRow) database.GetRunningPrebuildsRow {
			row.CreatedAt = time.Now()
			return row
		}),
	}

	// GIVEN: NO prebuilds in progress.
	var inProgress []database.GetPrebuildsInProgressRow

	// WHEN: calculating the current preset's state.
	state := newReconciliationState(presets, running, inProgress)
	ps, err := state.filterByPreset(current.presetID)
	require.NoError(t, err)

	// THEN: an extraneous prebuild is detected and marked for deletion.
	actions, err := ps.calculateActions()
	require.NoError(t, err)
	validateActions(t, reconciliationActions{
		actual: 2, desired: 1, extraneous: 1, deleteIDs: []uuid.UUID{older}, eligible: 2,
	}, *actions)
}

// As above, but no actions will be performed because
func TestExtraneousInProgress(t *testing.T) {
	current := opts[optionSet0]

	// GIVEN: a preset with 1 desired prebuild.
	presets := []database.GetTemplatePresetsWithPrebuildsRow{
		preset(true, 1, current),
	}

	var older uuid.UUID
	// GIVEN: 2 running prebuilds for the preset.
	running := []database.GetRunningPrebuildsRow{
		prebuild(current, func(row database.GetRunningPrebuildsRow) database.GetRunningPrebuildsRow {
			// The older of the running prebuilds will be deleted in order to maintain freshness.
			row.CreatedAt = time.Now().Add(-time.Hour)
			older = row.WorkspaceID
			return row
		}),
		prebuild(current, func(row database.GetRunningPrebuildsRow) database.GetRunningPrebuildsRow {
			row.CreatedAt = time.Now()
			return row
		}),
	}

	// GIVEN: NO prebuilds in progress.
	var inProgress []database.GetPrebuildsInProgressRow

	// WHEN: calculating the current preset's state.
	state := newReconciliationState(presets, running, inProgress)
	ps, err := state.filterByPreset(current.presetID)
	require.NoError(t, err)

	// THEN: an extraneous prebuild is detected and marked for deletion.
	actions, err := ps.calculateActions()
	require.NoError(t, err)
	validateActions(t, reconciliationActions{
		actual: 2, desired: 1, extraneous: 1, deleteIDs: []uuid.UUID{older}, eligible: 2,
	}, *actions)
}

// A template marked as deprecated will not have prebuilds running.
func TestDeprecated(t *testing.T) {
	current := opts[optionSet0]

	// GIVEN: a preset with 1 desired prebuild.
	presets := []database.GetTemplatePresetsWithPrebuildsRow{
		preset(true, 1, current, func(row database.GetTemplatePresetsWithPrebuildsRow) database.GetTemplatePresetsWithPrebuildsRow {
			row.Deprecated = true
			return row
		}),
	}

	// GIVEN: 1 running prebuilds for the preset.
	running := []database.GetRunningPrebuildsRow{
		prebuild(current),
	}

	// GIVEN: NO prebuilds in progress.
	var inProgress []database.GetPrebuildsInProgressRow

	// WHEN: calculating the current preset's state.
	state := newReconciliationState(presets, running, inProgress)
	ps, err := state.filterByPreset(current.presetID)
	require.NoError(t, err)

	// THEN: all running prebuilds should be deleted because the template is deprecated.
	actions, err := ps.calculateActions()
	require.NoError(t, err)
	validateActions(t, reconciliationActions{
		actual: 1, deleteIDs: []uuid.UUID{current.prebuildID}, eligible: 1,
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

func prebuild(opts options, muts ...func(row database.GetRunningPrebuildsRow) database.GetRunningPrebuildsRow) database.GetRunningPrebuildsRow {
	entry := database.GetRunningPrebuildsRow{
		WorkspaceID:       opts.prebuildID,
		WorkspaceName:     opts.workspaceName,
		TemplateID:        opts.templateID,
		TemplateVersionID: opts.templateVersionID,
		CurrentPresetID:   uuid.NullUUID{UUID: opts.presetID, Valid: true},
		Ready:             true,
		CreatedAt:         time.Now(),
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
