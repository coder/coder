package prebuilds

import (
	"time"

	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/util/slice"
)

// ActionType represents the type of action needed to reconcile prebuilds.
type ActionType int

const (
	// ActionTypeCreate indicates that new prebuilds should be created.
	ActionTypeCreate ActionType = iota

	// ActionTypeDelete indicates that existing prebuilds should be deleted.
	ActionTypeDelete

	// ActionTypeBackoff indicates that prebuild creation should be delayed.
	ActionTypeBackoff
)

// GlobalSnapshot represents a full point-in-time snapshot of state relating to prebuilds across all templates.
type GlobalSnapshot struct {
	Presets             []database.GetTemplatePresetsWithPrebuildsRow
	RunningPrebuilds    []database.GetRunningPrebuiltWorkspacesRow
	PrebuildsInProgress []database.CountInProgressPrebuildsRow
	Backoffs            []database.GetPresetsBackoffRow
}

// PresetSnapshot is a filtered view of GlobalSnapshot focused on a single preset.
// It contains the raw data needed to calculate the current state of a preset's prebuilds,
// including running prebuilds, in-progress builds, and backoff information.
type PresetSnapshot struct {
	Preset     database.GetTemplatePresetsWithPrebuildsRow
	Running    []database.GetRunningPrebuiltWorkspacesRow
	InProgress []database.CountInProgressPrebuildsRow
	Backoff    *database.GetPresetsBackoffRow
}

// ReconciliationState represents the processed state of a preset's prebuilds,
// calculated from a PresetSnapshot. While PresetSnapshot contains raw data,
// ReconciliationState contains derived metrics that are directly used to
// determine what actions are needed (create, delete, or backoff).
// For example, it calculates how many prebuilds are eligible, how many are
// extraneous, and how many are in various transition states.
type ReconciliationState struct {
	Actual     int32 // Number of currently running prebuilds
	Desired    int32 // Number of prebuilds desired as defined in the preset
	Eligible   int32 // Number of prebuilds that are ready to be claimed
	Extraneous int32 // Number of extra running prebuilds beyond the desired count

	// Counts of prebuilds in various transition states
	Starting int32
	Stopping int32
	Deleting int32
}

// ReconciliationActions represents a single action needed to reconcile the current state with the desired state.
// Exactly one field will be set based on the ActionType.
type ReconciliationActions struct {
	// ActionType determines which field is set and what action should be taken
	ActionType ActionType

	// Create is set when ActionType is ActionTypeCreate and indicates the number of prebuilds to create
	Create int32

	// DeleteIDs is set when ActionType is ActionTypeDelete and contains the IDs of prebuilds to delete
	DeleteIDs []uuid.UUID

	// BackoffUntil is set when ActionType is ActionTypeBackoff and indicates when to retry creating prebuilds
	BackoffUntil time.Time
}

func NewGlobalSnapshot(presets []database.GetTemplatePresetsWithPrebuildsRow, runningPrebuilds []database.GetRunningPrebuiltWorkspacesRow,
	prebuildsInProgress []database.CountInProgressPrebuildsRow, backoffs []database.GetPresetsBackoffRow,
) GlobalSnapshot {
	return GlobalSnapshot{Presets: presets, RunningPrebuilds: runningPrebuilds, PrebuildsInProgress: prebuildsInProgress, Backoffs: backoffs}
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
		return prebuild.CurrentPresetID.UUID == preset.ID &&
			prebuild.TemplateVersionID == preset.TemplateVersionID // Not strictly necessary since presets are 1:1 with template versions, but no harm in being extra safe.
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
