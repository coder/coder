package prebuilds

import (
	"time"

	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"cdr.dev/slog"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/util/slice"
	"github.com/coder/quartz"
)

// GlobalSnapshot represents a full point-in-time snapshot of state relating to prebuilds across all templates.
type GlobalSnapshot struct {
	Presets               []database.GetTemplatePresetsWithPrebuildsRow
	PrebuildSchedules     []database.TemplateVersionPresetPrebuildSchedule
	RunningPrebuilds      []database.GetRunningPrebuiltWorkspacesRow
	PrebuildsInProgress   []database.CountInProgressPrebuildsRow
	PendingPrebuilds      []database.CountPendingNonActivePrebuildsRow
	Backoffs              []database.GetPresetsBackoffRow
	HardLimitedPresetsMap map[uuid.UUID]database.GetPresetsAtFailureLimitRow
	clock                 quartz.Clock
	logger                slog.Logger
}

func NewGlobalSnapshot(
	presets []database.GetTemplatePresetsWithPrebuildsRow,
	prebuildSchedules []database.TemplateVersionPresetPrebuildSchedule,
	runningPrebuilds []database.GetRunningPrebuiltWorkspacesRow,
	prebuildsInProgress []database.CountInProgressPrebuildsRow,
	pendingPrebuilds []database.CountPendingNonActivePrebuildsRow,
	backoffs []database.GetPresetsBackoffRow,
	hardLimitedPresets []database.GetPresetsAtFailureLimitRow,
	clock quartz.Clock,
	logger slog.Logger,
) GlobalSnapshot {
	hardLimitedPresetsMap := make(map[uuid.UUID]database.GetPresetsAtFailureLimitRow, len(hardLimitedPresets))
	for _, preset := range hardLimitedPresets {
		hardLimitedPresetsMap[preset.PresetID] = preset
	}

	return GlobalSnapshot{
		Presets:               presets,
		PrebuildSchedules:     prebuildSchedules,
		RunningPrebuilds:      runningPrebuilds,
		PrebuildsInProgress:   prebuildsInProgress,
		PendingPrebuilds:      pendingPrebuilds,
		Backoffs:              backoffs,
		HardLimitedPresetsMap: hardLimitedPresetsMap,
		clock:                 clock,
		logger:                logger,
	}
}

func (s GlobalSnapshot) FilterByPreset(presetID uuid.UUID) (*PresetSnapshot, error) {
	preset, found := slice.Find(s.Presets, func(preset database.GetTemplatePresetsWithPrebuildsRow) bool {
		return preset.ID == presetID
	})
	if !found {
		return nil, xerrors.Errorf("no preset found with ID %q", presetID)
	}

	prebuildSchedules := slice.Filter(s.PrebuildSchedules, func(schedule database.TemplateVersionPresetPrebuildSchedule) bool {
		return schedule.PresetID == presetID
	})

	// Only include workspaces that have successfully started
	running := slice.Filter(s.RunningPrebuilds, func(prebuild database.GetRunningPrebuiltWorkspacesRow) bool {
		if !prebuild.CurrentPresetID.Valid {
			return false
		}
		return prebuild.CurrentPresetID.UUID == preset.ID
	})

	// Separate running workspaces into non-expired and expired based on the preset's TTL
	nonExpired, expired := filterExpiredWorkspaces(preset, running)

	// Includes in-progress prebuilds only for active template versions.
	// In-progress prebuilds correspond to workspace statuses: 'pending', 'starting', 'stopping', and 'deleting'
	inProgress := slice.Filter(s.PrebuildsInProgress, func(prebuild database.CountInProgressPrebuildsRow) bool {
		return prebuild.PresetID.UUID == preset.ID
	})

	// Includes count of pending prebuilds only for non-active template versions
	pendingCount := 0
	if found, ok := slice.Find(s.PendingPrebuilds, func(prebuild database.CountPendingNonActivePrebuildsRow) bool {
		return prebuild.PresetID.UUID == preset.ID
	}); ok {
		pendingCount = int(found.Count)
	}

	var backoffPtr *database.GetPresetsBackoffRow
	backoff, found := slice.Find(s.Backoffs, func(row database.GetPresetsBackoffRow) bool {
		return row.PresetID == preset.ID
	})
	if found {
		backoffPtr = &backoff
	}

	_, isHardLimited := s.HardLimitedPresetsMap[preset.ID]

	presetSnapshot := NewPresetSnapshot(
		preset,
		prebuildSchedules,
		nonExpired,
		expired,
		inProgress,
		pendingCount,
		backoffPtr,
		isHardLimited,
		s.clock,
		s.logger,
	)

	return &presetSnapshot, nil
}

func (s GlobalSnapshot) IsHardLimited(presetID uuid.UUID) bool {
	_, isHardLimited := s.HardLimitedPresetsMap[presetID]

	return isHardLimited
}

// filterExpiredWorkspaces splits running workspaces into expired and non-expired
// based on the preset's TTL and last_invalidated_at timestamp.
// A prebuild is considered expired if:
// 1. The preset has been invalidated (last_invalidated_at is set), OR
// 2. It exceeds the preset's TTL (if TTL is set)
// If TTL is missing or zero, only last_invalidated_at is checked.
func filterExpiredWorkspaces(preset database.GetTemplatePresetsWithPrebuildsRow, runningWorkspaces []database.GetRunningPrebuiltWorkspacesRow) (nonExpired []database.GetRunningPrebuiltWorkspacesRow, expired []database.GetRunningPrebuiltWorkspacesRow) {
	for _, prebuild := range runningWorkspaces {
		isExpired := false

		// Check if prebuild was created before last invalidation
		if preset.LastInvalidatedAt.Valid && prebuild.CreatedAt.Before(preset.LastInvalidatedAt.Time) {
			isExpired = true
		}

		// Check TTL expiration if set
		if !isExpired && preset.Ttl.Valid {
			ttl := time.Duration(preset.Ttl.Int32) * time.Second
			if ttl > 0 && time.Since(prebuild.CreatedAt) > ttl {
				isExpired = true
			}
		}

		if isExpired {
			expired = append(expired, prebuild)
		} else {
			nonExpired = append(nonExpired, prebuild)
		}
	}
	return nonExpired, expired
}
