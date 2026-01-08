package prebuilds

import (
	"context"
	"fmt"
	"slices"
	"time"

	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/schedule/cron"
	"github.com/coder/quartz"
	tf_provider_helpers "github.com/coder/terraform-provider-coder/v2/provider/helpers"
)

// ActionType represents the type of action needed to reconcile prebuilds.
type ActionType int

const (
	// ActionTypeUndefined represents an uninitialized or invalid action type.
	ActionTypeUndefined ActionType = iota

	// ActionTypeCreate indicates that new prebuilds should be created.
	ActionTypeCreate

	// ActionTypeDelete indicates that existing prebuilds should be deleted.
	ActionTypeDelete

	// ActionTypeBackoff indicates that prebuild creation should be delayed.
	ActionTypeBackoff

	// ActionTypeCancelPending indicates that pending prebuilds should be canceled.
	ActionTypeCancelPending
)

// PresetSnapshot is a filtered view of GlobalSnapshot focused on a single preset.
// It contains the raw data needed to calculate the current state of a preset's prebuilds,
// including running prebuilds, in-progress builds, and backoff information.
// - Running: prebuilds running and non-expired
// - Expired: prebuilds running and expired due to the preset's TTL
// - InProgress: prebuilds currently in progress
// - Backoff: holds failure info to decide if prebuild creation should be backed off
type PresetSnapshot struct {
	Preset            database.GetTemplatePresetsWithPrebuildsRow
	PrebuildSchedules []database.TemplateVersionPresetPrebuildSchedule
	Running           []database.GetRunningPrebuiltWorkspacesRow
	Expired           []database.GetRunningPrebuiltWorkspacesRow
	InProgress        []database.CountInProgressPrebuildsRow
	PendingCount      int
	Backoff           *database.GetPresetsBackoffRow
	IsHardLimited     bool
	clock             quartz.Clock
	logger            slog.Logger
}

func NewPresetSnapshot(
	preset database.GetTemplatePresetsWithPrebuildsRow,
	prebuildSchedules []database.TemplateVersionPresetPrebuildSchedule,
	running []database.GetRunningPrebuiltWorkspacesRow,
	expired []database.GetRunningPrebuiltWorkspacesRow,
	inProgress []database.CountInProgressPrebuildsRow,
	pendingCount int,
	backoff *database.GetPresetsBackoffRow,
	isHardLimited bool,
	clock quartz.Clock,
	logger slog.Logger,
) PresetSnapshot {
	return PresetSnapshot{
		Preset:            preset,
		PrebuildSchedules: prebuildSchedules,
		Running:           running,
		Expired:           expired,
		InProgress:        inProgress,
		PendingCount:      pendingCount,
		Backoff:           backoff,
		IsHardLimited:     isHardLimited,
		clock:             clock,
		logger:            logger,
	}
}

// ReconciliationState represents the processed state of a preset's prebuilds,
// calculated from a PresetSnapshot. While PresetSnapshot contains raw data,
// ReconciliationState contains derived metrics that are directly used to
// determine what actions are needed (create, delete, or backoff).
// For example, it calculates how many prebuilds are expired, eligible,
// how many are extraneous, and how many are in various transition states.
type ReconciliationState struct {
	Actual     int32 // Number of currently running prebuilds, i.e., non-expired, expired and extraneous prebuilds
	Expired    int32 // Number of currently running prebuilds that exceeded their allowed time-to-live (TTL)
	Desired    int32 // Number of prebuilds desired as defined in the preset
	Eligible   int32 // Number of prebuilds that are ready to be claimed
	Extraneous int32 // Number of extra running prebuilds beyond the desired count

	// Counts of prebuilds in various transition states
	Starting int32
	Stopping int32
	Deleting int32
}

// ReconciliationActions represents actions needed to reconcile the current state with the desired state.
// Based on ActionType, exactly one of Create, DeleteIDs, or BackoffUntil will be set.
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

func (ra *ReconciliationActions) IsNoop() bool {
	return ra.ActionType != ActionTypeCancelPending && ra.Create == 0 && len(ra.DeleteIDs) == 0 && ra.BackoffUntil.IsZero()
}

// MatchesCron interprets a cron spec as a continuous time range,
// and returns whether the provided time value falls within that range.
func MatchesCron(cronExpression string, at time.Time) (bool, error) {
	sched, err := cron.TimeRange(cronExpression)
	if err != nil {
		return false, xerrors.Errorf("failed to parse cron expression: %w", err)
	}

	return sched.IsWithinRange(at), nil
}

// CalculateDesiredInstances returns the number of desired instances based on the provided time.
// If the time matches any defined prebuild schedule, the corresponding number of instances is returned.
// Otherwise, it falls back to the default number of instances specified in the prebuild configuration.
func (p PresetSnapshot) CalculateDesiredInstances(at time.Time) int32 {
	if len(p.PrebuildSchedules) == 0 {
		// If no schedules are defined, fall back to the default desired instance count
		return p.Preset.DesiredInstances.Int32
	}

	if p.Preset.SchedulingTimezone == "" {
		p.logger.Error(context.Background(), "timezone is not set in prebuild scheduling configuration",
			slog.F("preset_id", p.Preset.ID),
			slog.F("timezone", p.Preset.SchedulingTimezone))

		// If timezone is not set, fall back to the default desired instance count
		return p.Preset.DesiredInstances.Int32
	}

	// Validate that the provided timezone is valid
	_, err := time.LoadLocation(p.Preset.SchedulingTimezone)
	if err != nil {
		p.logger.Error(context.Background(), "invalid timezone in prebuild scheduling configuration",
			slog.F("preset_id", p.Preset.ID),
			slog.F("timezone", p.Preset.SchedulingTimezone),
			slog.Error(err))

		// If timezone is invalid, fall back to the default desired instance count
		return p.Preset.DesiredInstances.Int32
	}

	// Validate that all prebuild schedules are valid and don't overlap with each other.
	// If any schedule is invalid or schedules overlap, fall back to the default desired instance count.
	cronSpecs := make([]string, len(p.PrebuildSchedules))
	for i, schedule := range p.PrebuildSchedules {
		cronSpecs[i] = schedule.CronExpression
	}
	err = tf_provider_helpers.ValidateSchedules(cronSpecs)
	if err != nil {
		p.logger.Error(context.Background(), "schedules are invalid or overlap with each other",
			slog.F("preset_id", p.Preset.ID),
			slog.F("cron_specs", cronSpecs),
			slog.Error(err))

		// If schedules are invalid, fall back to the default desired instance count
		return p.Preset.DesiredInstances.Int32
	}

	// Look for a schedule whose cron expression matches the provided time
	for _, schedule := range p.PrebuildSchedules {
		// Prefix the cron expression with timezone information
		cronExprWithTimezone := fmt.Sprintf("CRON_TZ=%s %s", p.Preset.SchedulingTimezone, schedule.CronExpression)
		matches, err := MatchesCron(cronExprWithTimezone, at)
		if err != nil {
			p.logger.Error(context.Background(), "cron expression is invalid",
				slog.F("preset_id", p.Preset.ID),
				slog.F("cron_expression", cronExprWithTimezone),
				slog.Error(err))
			continue
		}
		if matches {
			p.logger.Debug(context.Background(), "current time matched cron expression",
				slog.F("preset_id", p.Preset.ID),
				slog.F("current_time", at.String()),
				slog.F("cron_expression", cronExprWithTimezone),
				slog.F("desired_instances", schedule.DesiredInstances),
			)

			return schedule.DesiredInstances
		}
	}

	// If no schedule matches, fall back to the default desired instance count
	return p.Preset.DesiredInstances.Int32
}

// CalculateState computes the current state of prebuilds for a preset, including:
// - Actual: Number of currently running prebuilds, i.e., non-expired and expired prebuilds
// - Expired: Number of currently running expired prebuilds
// - Desired: Number of prebuilds desired as defined in the preset
// - Eligible: Number of prebuilds that are ready to be claimed
// - Extraneous: Number of extra running prebuilds beyond the desired count
// - Starting/Stopping/Deleting: Counts of prebuilds in various transition states
//
// The function takes into account whether the preset is active (using the active template version)
// and calculates appropriate counts based on the current state of running prebuilds and
// in-progress transitions. This state information is used to determine what reconciliation
// actions are needed to reach the desired state.
func (p PresetSnapshot) CalculateState() *ReconciliationState {
	var (
		actual     int32
		desired    int32
		expired    int32
		eligible   int32
		extraneous int32
	)

	// #nosec G115 - Safe conversion as p.Running and p.Expired slice length is expected to be within int32 range
	actual = int32(len(p.Running) + len(p.Expired))

	// #nosec G115 - Safe conversion as p.Expired slice length is expected to be within int32 range
	expired = int32(len(p.Expired))

	if p.isActive() {
		desired = p.CalculateDesiredInstances(p.clock.Now())
		eligible = p.countEligible()
		extraneous = max(actual-expired-desired, 0)
	}

	starting, stopping, deleting := p.countInProgress()

	return &ReconciliationState{
		Actual:     actual,
		Expired:    expired,
		Desired:    desired,
		Eligible:   eligible,
		Extraneous: extraneous,

		Starting: starting,
		Stopping: stopping,
		Deleting: deleting,
	}
}

// CalculateActions determines what actions are needed to reconcile the current state with the desired state.
// The function:
// 1. First checks if a backoff period is needed (if previous builds failed)
// 2. If the preset is inactive (template version is not active), it will delete all running prebuilds
// 3. For active presets, it calculates the number of prebuilds to create or delete based on:
//   - The desired number of instances
//   - Currently running prebuilds
//   - Currently running expired prebuilds
//   - Prebuilds in transition states (starting/stopping/deleting)
//   - Any extraneous prebuilds that need to be removed
//
// The function returns a ReconciliationActions struct that will have exactly one action type set:
// - ActionTypeBackoff: Only BackoffUntil is set, indicating when to retry
// - ActionTypeCreate: Only Create is set, indicating how many prebuilds to create
// - ActionTypeDelete: Only DeleteIDs is set, containing IDs of prebuilds to delete
func (p PresetSnapshot) CalculateActions(backoffInterval time.Duration) ([]*ReconciliationActions, error) {
	// TODO: align workspace states with how we represent them on the FE and the CLI
	//	     right now there's some slight differences which can lead to additional prebuilds being created

	// TODO: add mechanism to prevent prebuilds being reconciled from being claimable by users; i.e. if a prebuild is
	// 		 about to be deleted, it should not be deleted if it has been claimed - beware of TOCTOU races!

	actions, needsBackoff := p.needsBackoffPeriod(p.clock, backoffInterval)
	if needsBackoff {
		return actions, nil
	}

	if !p.isActive() {
		return p.handleInactiveTemplateVersion()
	}

	return p.handleActiveTemplateVersion()
}

// isActive returns true if the preset's template version is the active version, and it is neither deleted nor deprecated.
// This determines whether we should maintain prebuilds for this preset or delete them.
func (p PresetSnapshot) isActive() bool {
	return p.Preset.UsingActiveVersion && !p.Preset.Deleted && !p.Preset.Deprecated
}

// handleActiveTemplateVersion determines the reconciliation actions for a preset with an active template version.
// It ensures the system moves towards the desired number of healthy prebuilds.
//
// The reconciliation follows this order:
//  1. Delete expired prebuilds: These are no longer valid and must be removed first.
//  2. Delete extraneous prebuilds: After expired ones are removed, if the number of running non-expired prebuilds
//     still exceeds the desired count, the oldest prebuilds are deleted to reduce excess.
//  3. Create missing prebuilds: If the number of non-expired, non-starting prebuilds is still below the desired count,
//     create the necessary number of prebuilds to reach the target.
//
// The function returns a list of actions to be executed to achieve the desired state.
func (p PresetSnapshot) handleActiveTemplateVersion() (actions []*ReconciliationActions, err error) {
	state := p.CalculateState()

	// If we have expired prebuilds, delete them
	if state.Expired > 0 {
		var deleteIDs []uuid.UUID
		for _, expired := range p.Expired {
			deleteIDs = append(deleteIDs, expired.ID)
		}
		actions = append(actions,
			&ReconciliationActions{
				ActionType: ActionTypeDelete,
				DeleteIDs:  deleteIDs,
			})
	}

	// If we still have more prebuilds than desired, delete the oldest ones
	if state.Extraneous > 0 {
		actions = append(actions,
			&ReconciliationActions{
				ActionType: ActionTypeDelete,
				DeleteIDs:  p.getOldestPrebuildIDs(int(state.Extraneous)),
			})
	}

	// Number of running prebuilds excluding the recently deleted Expired
	runningValid := state.Actual - state.Expired

	// Calculate how many new prebuilds we need to create
	// We subtract starting prebuilds since they're already being created
	prebuildsToCreate := max(state.Desired-runningValid-state.Starting, 0)
	if prebuildsToCreate > 0 {
		actions = append(actions,
			&ReconciliationActions{
				ActionType: ActionTypeCreate,
				Create:     prebuildsToCreate,
			})
	}

	return actions, nil
}

// handleInactiveTemplateVersion handles prebuilds from inactive template versions:
//  1. If the preset has pending prebuild jobs from an inactive template version, create a cancel reconciliation action.
//     This cancels all pending prebuild jobs for this preset's template version.
//  2. If the preset has prebuilt workspaces currently running from an inactive template version,
//     create a delete reconciliation action to remove all running prebuilt workspaces.
func (p PresetSnapshot) handleInactiveTemplateVersion() (actions []*ReconciliationActions, err error) {
	// Cancel pending initial prebuild jobs from inactive version
	if p.PendingCount > 0 {
		actions = append(actions,
			&ReconciliationActions{
				ActionType: ActionTypeCancelPending,
			})
	}

	// Delete prebuilds running in inactive version
	deleteIDs := p.getOldestPrebuildIDs(len(p.Running))
	if len(deleteIDs) > 0 {
		actions = append(actions,
			&ReconciliationActions{
				ActionType: ActionTypeDelete,
				DeleteIDs:  deleteIDs,
			})
	}
	return actions, nil
}

// needsBackoffPeriod checks if we should delay prebuild creation due to recent failures.
// If there were failures, it calculates a backoff period based on the number of failures
// and returns true if we're still within that period.
func (p PresetSnapshot) needsBackoffPeriod(clock quartz.Clock, backoffInterval time.Duration) ([]*ReconciliationActions, bool) {
	if p.Backoff == nil || p.Backoff.NumFailed == 0 {
		return nil, false
	}
	backoffUntil := p.Backoff.LastBuildAt.Add(time.Duration(p.Backoff.NumFailed) * backoffInterval)
	if clock.Now().After(backoffUntil) {
		return nil, false
	}

	return []*ReconciliationActions{
		{
			ActionType:   ActionTypeBackoff,
			BackoffUntil: backoffUntil,
		},
	}, true
}

// countEligible returns the number of prebuilds that are ready to be claimed.
// A prebuild is eligible if it's running and its agents are in ready state.
func (p PresetSnapshot) countEligible() int32 {
	var count int32
	for _, prebuild := range p.Running {
		if prebuild.Ready {
			count++
		}
	}
	return count
}

// countInProgress returns counts of prebuilds in transition states (starting, stopping, deleting).
// These counts are tracked at the template level, so all presets sharing the same template see the same values.
func (p PresetSnapshot) countInProgress() (starting int32, stopping int32, deleting int32) {
	for _, progress := range p.InProgress {
		num := progress.Count
		switch progress.Transition {
		case database.WorkspaceTransitionStart:
			starting += num
		case database.WorkspaceTransitionStop:
			stopping += num
		case database.WorkspaceTransitionDelete:
			deleting += num
		}
	}

	return starting, stopping, deleting
}

// getOldestPrebuildIDs returns the IDs of the N oldest prebuilds, sorted by creation time.
// This is used when we need to delete prebuilds, ensuring we remove the oldest ones first.
func (p PresetSnapshot) getOldestPrebuildIDs(n int) []uuid.UUID {
	// Sort by creation time, oldest first
	slices.SortFunc(p.Running, func(a, b database.GetRunningPrebuiltWorkspacesRow) int {
		return a.CreatedAt.Compare(b.CreatedAt)
	})

	// Take the first N IDs
	n = min(n, len(p.Running))
	ids := make([]uuid.UUID, n)
	for i := 0; i < n; i++ {
		ids[i] = p.Running[i].ID
	}

	return ids
}
