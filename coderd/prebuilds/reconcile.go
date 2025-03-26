package prebuilds

import (
	"time"

	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/util/slice"
)

// ReconciliationState represents a full point-in-time snapshot of state relating to prebuilds across all templates.
type ReconciliationState struct {
	Presets             []database.GetTemplatePresetsWithPrebuildsRow
	RunningPrebuilds    []database.GetRunningPrebuildsRow
	PrebuildsInProgress []database.GetPrebuildsInProgressRow
	Backoffs            []database.GetPresetsBackoffRow
}

// PresetState is a subset of ReconciliationState but specifically for a single preset.
type PresetState struct {
	Preset     database.GetTemplatePresetsWithPrebuildsRow
	Running    []database.GetRunningPrebuildsRow
	InProgress []database.GetPrebuildsInProgressRow
	Backoff    *database.GetPresetsBackoffRow
}

// ReconciliationActions represents the set of actions which must be taken to achieve the desired state for prebuilds.
type ReconciliationActions struct {
	Actual                       int32       // Running prebuilds for active version.
	Desired                      int32       // Active template version's desired instances as defined in preset.
	Eligible                     int32       // Prebuilds which can be claimed.
	Outdated                     int32       // Prebuilds which no longer match the active template version.
	Extraneous                   int32       // Extra running prebuilds for active version (somehow).
	Starting, Stopping, Deleting int32       // Prebuilds currently being provisioned up or down.
	Failed                       int32       // Number of prebuilds which have failed in the past CODER_WORKSPACE_PREBUILDS_RECONCILIATION_BACKOFF_LOOKBACK_PERIOD.
	Create                       int32       // The number of prebuilds required to be created to reconcile required state.
	DeleteIDs                    []uuid.UUID // IDs of running prebuilds required to be deleted to reconcile required state.
	BackoffUntil                 time.Time   // The time to wait until before trying to provision a new prebuild.
}

func NewReconciliationState(presets []database.GetTemplatePresetsWithPrebuildsRow, runningPrebuilds []database.GetRunningPrebuildsRow,
	prebuildsInProgress []database.GetPrebuildsInProgressRow, backoffs []database.GetPresetsBackoffRow,
) ReconciliationState {
	return ReconciliationState{Presets: presets, RunningPrebuilds: runningPrebuilds, PrebuildsInProgress: prebuildsInProgress, Backoffs: backoffs}
}

func (s ReconciliationState) FilterByPreset(presetID uuid.UUID) (*PresetState, error) {
	preset, found := slice.Find(s.Presets, func(preset database.GetTemplatePresetsWithPrebuildsRow) bool {
		return preset.ID == presetID
	})
	if !found {
		return nil, xerrors.Errorf("no preset found with ID %q", presetID)
	}

	running := slice.Filter(s.RunningPrebuilds, func(prebuild database.GetRunningPrebuildsRow) bool {
		if !prebuild.CurrentPresetID.Valid {
			return false
		}
		return prebuild.CurrentPresetID.UUID == preset.ID &&
			prebuild.TemplateVersionID == preset.TemplateVersionID // Not strictly necessary since presets are 1:1 with template versions, but no harm in being extra safe.
	})

	// These aren't preset-specific, but they need to inhibit all presets of this template from operating since they could
	// be in-progress builds which might impact another preset. For example, if a template goes from no defined prebuilds to defined prebuilds
	// and back, or a template is updated from one version to another.
	// We group by the template so that all prebuilds being provisioned for a prebuild are inhibited if any prebuild for
	// any preset in that template are in progress, to prevent clobbering.
	inProgress := slice.Filter(s.PrebuildsInProgress, func(prebuild database.GetPrebuildsInProgressRow) bool {
		return prebuild.TemplateID == preset.TemplateID
	})

	var backoff *database.GetPresetsBackoffRow
	backoffs := slice.Filter(s.Backoffs, func(row database.GetPresetsBackoffRow) bool {
		return row.PresetID == preset.ID
	})
	if len(backoffs) == 1 {
		backoff = &backoffs[0]
	}

	return &PresetState{
		Preset:     preset,
		Running:    running,
		InProgress: inProgress,
		Backoff:    backoff,
	}, nil
}
