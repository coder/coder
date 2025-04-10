package prebuilds

import (
	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/util/slice"
)

// GlobalSnapshot represents a full point-in-time snapshot of state relating to prebuilds across all templates.
type GlobalSnapshot struct {
	Presets             []database.GetTemplatePresetsWithPrebuildsRow
	RunningPrebuilds    []database.GetRunningPrebuiltWorkspacesRow
	PrebuildsInProgress []database.CountInProgressPrebuildsRow
	Backoffs            []database.GetPresetsBackoffRow
}

func NewGlobalSnapshot(
	presets []database.GetTemplatePresetsWithPrebuildsRow,
	runningPrebuilds []database.GetRunningPrebuiltWorkspacesRow,
	prebuildsInProgress []database.CountInProgressPrebuildsRow,
	backoffs []database.GetPresetsBackoffRow,
) GlobalSnapshot {
	return GlobalSnapshot{
		Presets:             presets,
		RunningPrebuilds:    runningPrebuilds,
		PrebuildsInProgress: prebuildsInProgress,
		Backoffs:            backoffs,
	}
}

func (s GlobalSnapshot) FilterByPreset(presetID uuid.UUID) (*PresetSnapshot, error) {
	preset, found := slice.Find(s.Presets, func(preset database.GetTemplatePresetsWithPrebuildsRow) bool {
		return preset.ID == presetID
	})
	if !found {
		return nil, xerrors.Errorf("no preset found with ID %q", presetID)
	}

	running := slice.Filter(s.RunningPrebuilds, func(prebuild database.GetRunningPrebuiltWorkspacesRow) bool {
		if !prebuild.CurrentPresetID.Valid {
			return false
		}
		return prebuild.CurrentPresetID.UUID == preset.ID
	})

	// These aren't preset-specific, but they need to inhibit all presets of this template from operating since they could
	// be in-progress builds which might impact another preset. For example, if a template goes from no defined prebuilds to defined prebuilds
	// and back, or a template is updated from one version to another.
	// We group by the template so that all prebuilds being provisioned for a prebuild are inhibited if any prebuild for
	// any preset in that template are in progress, to prevent clobbering.
	inProgress := slice.Filter(s.PrebuildsInProgress, func(prebuild database.CountInProgressPrebuildsRow) bool {
		return prebuild.TemplateID == preset.TemplateID
	})

	var backoff *database.GetPresetsBackoffRow
	backoffs := slice.Filter(s.Backoffs, func(row database.GetPresetsBackoffRow) bool {
		return row.PresetID == preset.ID
	})
	if len(backoffs) == 1 {
		backoff = &backoffs[0]
	}

	return &PresetSnapshot{
		Preset:     preset,
		Running:    running,
		InProgress: inProgress,
		Backoff:    backoff,
	}, nil
}
