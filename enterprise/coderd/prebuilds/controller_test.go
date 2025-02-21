package prebuilds

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/database"
)

var (
	templateID        = uuid.New()
	templateVersionID = uuid.New()
	presetID          = uuid.New()
	preset2ID         = uuid.New()
	prebuildID        = uuid.New()
)

func TestReconciliationActions(t *testing.T) {
	cases := map[string]struct {
		preset     database.GetTemplatePresetsWithPrebuildsRow // TODO: make own structs; reusing these types is lame
		running    []database.GetRunningPrebuildsRow
		inProgress []database.GetPrebuildsInProgressRow
		expected   reconciliationActions
	}{
		// New template version created which adds a new preset with prebuilds configured.
		"CreateNetNew": {
			preset: preset(true, 1),
			expected: reconciliationActions{
				desired: 1,
				create:  1,
			},
		},
		// New template version created, making an existing preset and its prebuilds outdated.
		"DeleteOutdated": {
			preset: preset(false, 1),
			running: []database.GetRunningPrebuildsRow{
				{
					WorkspaceID:       prebuildID,
					TemplateID:        templateID,
					TemplateVersionID: templateVersionID,
					CurrentPresetID:   uuid.NullUUID{UUID: presetID, Valid: true},
					DesiredPresetID:   uuid.NullUUID{UUID: uuid.New(), Valid: true},
					Ready:             true,
				},
			},
			expected: reconciliationActions{
				outdated:  1,
				deleteIDs: []uuid.UUID{prebuildID},
			},
		},
		// Somehow an additional prebuild is running, delete it.
		// This can happen if an operator messes with a prebuild's state (stop, start).
		"DeleteOldestExtraneous": {
			preset: preset(true, 1),
			running: []database.GetRunningPrebuildsRow{
				{
					WorkspaceID:       prebuildID,
					TemplateID:        templateID,
					TemplateVersionID: templateVersionID,
					CurrentPresetID:   uuid.NullUUID{UUID: presetID, Valid: true},
					DesiredPresetID:   uuid.NullUUID{UUID: uuid.New(), Valid: true},
					CreatedAt:         time.Now().Add(-time.Hour),
				},
				{
					WorkspaceID:       uuid.New(),
					TemplateID:        templateID,
					TemplateVersionID: templateVersionID,
					CurrentPresetID:   uuid.NullUUID{UUID: presetID, Valid: true},
					DesiredPresetID:   uuid.NullUUID{UUID: uuid.New(), Valid: true},
					CreatedAt:         time.Now(),
				},
			},
			expected: reconciliationActions{
				desired:    1,
				extraneous: 1,
				actual:     2,
				deleteIDs:  []uuid.UUID{prebuildID},
			},
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			ps := presetState{
				preset:     tc.preset,
				running:    tc.running,
				inProgress: tc.inProgress,
			}

			actions, err := ps.calculateActions()
			require.NoError(t, err, "could not calculate reconciliation actions")

			validateActions(t, tc.expected, *actions)
		})
	}
}

func preset(active bool, instances int32) database.GetTemplatePresetsWithPrebuildsRow {
	return database.GetTemplatePresetsWithPrebuildsRow{
		TemplateID:         templateID,
		TemplateVersionID:  templateVersionID,
		UsingActiveVersion: active,
		PresetID:           presetID,
		Name:               "bob",
		DesiredInstances:   instances,
		Deleted:            false,
		Deprecated:         false,
	}
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
