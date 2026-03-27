package prebuilds

import (
	"context"
	"fmt"
	"math"
	"slices"
	"strings"
	"time"

	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/prebuilds/targetexpr"
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
	EventCounts       PrebuildEventCounts
	IsHardLimited     bool
	evaluator         targetexpr.Evaluator
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
	eventCounts PrebuildEventCounts,
	isHardLimited bool,
	evaluator targetexpr.Evaluator,
	clock quartz.Clock,
	logger slog.Logger,
) PresetSnapshot {
	if evaluator == nil {
		evaluator = targetexpr.NewEvaluator()
	}

	return PresetSnapshot{
		Preset:            preset,
		PrebuildSchedules: prebuildSchedules,
		Running:           running,
		Expired:           expired,
		InProgress:        inProgress,
		PendingCount:      pendingCount,
		Backoff:           backoff,
		EventCounts:       eventCounts,
		IsHardLimited:     isHardLimited,
		evaluator:         evaluator,
		clock:             clock,
		logger:            logger,
	}
}

// CanSkipReconciliation returns true if this preset can safely be skipped during
// the reconciliation loop.
//
// This is a performance optimization to avoid spawning goroutines for presets
// that have no work to do. It only returns true for presets from inactive
// template versions that have no running workspaces, no pending jobs, and no
// in-progress builds.
func (p PresetSnapshot) CanSkipReconciliation() bool {
	// Active presets are never skipped. Presets from active template versions always
	// go through the reconciliation loop to ensure desired_instances is maintained correctly.
	if p.isActive() {
		return false
	}

	// Inactive presets with running prebuilds means there are prebuilds to delete.
	if len(p.Running) > 0 {
		return false
	}

	// Inactive presets with expired prebuilds means there are expired prebuilds to delete.
	if len(p.Expired) > 0 {
		return false
	}

	// Inactive presets with pending jobs means there are pending jobs to cancel.
	if p.PendingCount > 0 {
		return false
	}

	// Backoff is only populated for active presets, but check defensively.
	if p.Backoff != nil {
		return false
	}

	// Fields not checked (only relevant for active presets):
	// - PrebuildSchedules: Only affects desired instance calculation.
	// - InProgress: Only populated for active template versions.
	// - IsHardLimited: Only populated for active template versions.

	// Inactive preset with nothing to clean up: safe to skip.
	return true
}

// ReconciliationState represents the processed state of a preset's prebuilds,
// calculated from a PresetSnapshot. While PresetSnapshot contains raw data,
// ReconciliationState contains derived metrics that are directly used to
// determine what actions are needed (create, delete, or backoff).
// For example, it calculates how many prebuilds are expired, eligible,
// how many are extraneous, and how many are in various transition states.
type ReconciliationState struct {
	Actual               int32  // Number of currently running prebuilds, i.e., non-expired, expired and extraneous prebuilds
	Expired              int32  // Number of currently running prebuilds that exceeded their allowed time-to-live (TTL)
	Desired              int32  // Number of prebuilds desired as defined in the preset
	ScheduledTarget      int32  // Number of prebuilds desired before expression overrides are applied
	TargetSource         string // Source used to resolve Desired: scheduled, expression, or expression_fallback
	ExpressionConfigured bool   // Whether the preset has a desired instances expression configured
	ExpressionError      string // Expression validation, environment, or evaluation error when falling back
	Eligible             int32  // Number of prebuilds that are ready to be claimed
	Extraneous           int32  // Number of extra running prebuilds beyond the desired count

	// Counts of prebuilds in various transition states.
	Starting int32
	Stopping int32
	Deleting int32
}

type baseCounts struct {
	running  int32
	expired  int32
	eligible int32
	starting int32
	stopping int32
	deleting int32
}

type resolvedTarget struct {
	desired              int32
	scheduledTarget      int32
	targetSource         string
	expressionConfigured bool
	expressionError      string
}

// ReconciliationActions represents actions needed to reconcile the current state with the desired state.
// Based on ActionType, exactly one of Create, DeleteIDs, or BackoffUntil will be set.
type ReconciliationActions struct {
	// ActionType determines which field is set and what action should be taken.
	ActionType ActionType

	// Create is set when ActionType is ActionTypeCreate and indicates the number of prebuilds to create.
	Create int32

	// DeleteIDs is set when ActionType is ActionTypeDelete and contains the IDs of prebuilds to delete.
	DeleteIDs []uuid.UUID

	// BackoffUntil is set when ActionType is ActionTypeBackoff and indicates when to retry creating prebuilds.
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
func (p PresetSnapshot) CalculateDesiredInstances(at time.Time) int32 {
	return p.resolveDesiredTarget(at).desired
}

func (p PresetSnapshot) calculateScheduledTarget(at time.Time) int32 {
	if len(p.PrebuildSchedules) == 0 {
		// If no schedules are defined, fall back to the default desired instance count.
		return p.Preset.DesiredInstances.Int32
	}

	if p.Preset.SchedulingTimezone == "" {
		p.logger.Error(context.Background(), "timezone is not set in prebuild scheduling configuration",
			slog.F("preset_id", p.Preset.ID),
			slog.F("timezone", p.Preset.SchedulingTimezone))

		// If timezone is not set, fall back to the default desired instance count.
		return p.Preset.DesiredInstances.Int32
	}

	// Validate that the provided timezone is valid.
	_, err := time.LoadLocation(p.Preset.SchedulingTimezone)
	if err != nil {
		p.logger.Error(context.Background(), "invalid timezone in prebuild scheduling configuration",
			slog.F("preset_id", p.Preset.ID),
			slog.F("timezone", p.Preset.SchedulingTimezone),
			slog.Error(err))

		// If timezone is invalid, fall back to the default desired instance count.
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

		// If schedules are invalid, fall back to the default desired instance count.
		return p.Preset.DesiredInstances.Int32
	}

	// Look for a schedule whose cron expression matches the provided time.
	for _, schedule := range p.PrebuildSchedules {
		// Prefix the cron expression with timezone information.
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

	// If no schedule matches, fall back to the default desired instance count.
	return p.Preset.DesiredInstances.Int32
}

func (p PresetSnapshot) calculateBaseCounts() baseCounts {
	starting, stopping, deleting := p.countInProgress()

	return baseCounts{
		running:  safeNarrowInt64(int64(len(p.Running))),
		expired:  safeNarrowInt64(int64(len(p.Expired))),
		eligible: p.countEligible(),
		starting: starting,
		stopping: stopping,
		deleting: deleting,
	}
}

func safeNarrowInt64(v int64) int32 {
	if v < 0 {
		return 0
	}
	if v > math.MaxInt32 {
		return math.MaxInt32
	}
	return int32(v)
}

func (p PresetSnapshot) buildTargetEnv(at time.Time, scheduledTarget int32, counts baseCounts) (targetexpr.TargetEnv, error) {
	timezone := strings.TrimSpace(p.Preset.SchedulingTimezone)
	if timezone == "" {
		return targetexpr.TargetEnv{}, xerrors.New("missing scheduling timezone for desired instances expression")
	}

	location, err := time.LoadLocation(timezone)
	if err != nil {
		return targetexpr.TargetEnv{}, xerrors.Errorf("invalid scheduling timezone for desired instances expression: %w", err)
	}

	localizedTime := at.In(location)
	claims := p.EventCounts.ClaimSucceeded
	misses := p.EventCounts.ClaimMissed

	claims5m := safeNarrowInt64(claims.Count5m)
	claims10m := safeNarrowInt64(claims.Count10m)
	claims30m := safeNarrowInt64(claims.Count30m)
	claims60m := safeNarrowInt64(claims.Count60m)
	claims120m := safeNarrowInt64(claims.Count120m)
	misses5m := safeNarrowInt64(misses.Count5m)
	misses10m := safeNarrowInt64(misses.Count10m)
	misses30m := safeNarrowInt64(misses.Count30m)
	misses60m := safeNarrowInt64(misses.Count60m)
	misses120m := safeNarrowInt64(misses.Count120m)

	return targetexpr.TargetEnv{
		ScheduledTarget: scheduledTarget,
		Running:         counts.running,
		Eligible:        counts.eligible,
		Starting:        counts.starting,
		Stopping:        counts.stopping,
		Deleting:        counts.deleting,
		Expired:         counts.expired,
		Claims5m:        claims5m,
		Claims10m:       claims10m,
		Claims30m:       claims30m,
		Claims60m:       claims60m,
		Claims120m:      claims120m,
		Misses5m:        misses5m,
		Misses10m:       misses10m,
		Misses30m:       misses30m,
		Misses60m:       misses60m,
		Misses120m:      misses120m,
		ClaimRate5m:     float64(claims5m) / 5,
		ClaimRate10m:    float64(claims10m) / 10,
		ClaimRate30m:    float64(claims30m) / 30,
		ClaimRate60m:    float64(claims60m) / 60,
		ClaimRate120m:   float64(claims120m) / 120,
		Hour:            localizedTime.Hour(),
		Weekday:         int(localizedTime.Weekday()),
	}, nil
}

func (p PresetSnapshot) resolveDesiredTarget(at time.Time) resolvedTarget {
	scheduledTarget := p.calculateScheduledTarget(at)
	expression := strings.TrimSpace(p.Preset.DesiredInstancesExpression.String)
	result := resolvedTarget{
		desired:              scheduledTarget,
		scheduledTarget:      scheduledTarget,
		targetSource:         "scheduled",
		expressionConfigured: p.Preset.DesiredInstancesExpression.Valid && expression != "",
	}

	if !p.isActive() {
		result.desired = 0
		return result
	}

	if !result.expressionConfigured {
		return result
	}

	if p.evaluator == nil {
		result.targetSource = "expression_fallback"
		result.expressionError = "no evaluator configured"
		p.logger.Error(context.Background(), "desired instances expression evaluator is not configured",
			slog.F("preset_id", p.Preset.ID),
			slog.F("scheduled_target", scheduledTarget),
			slog.F("desired_instances_expression", expression))
		return result
	}

	counts := p.calculateBaseCounts()
	env, err := p.buildTargetEnv(at, scheduledTarget, counts)
	if err != nil {
		result.targetSource = "expression_fallback"
		result.expressionError = err.Error()
		p.logger.Error(context.Background(), "failed to build desired instances expression environment",
			slog.F("preset_id", p.Preset.ID),
			slog.F("scheduled_target", scheduledTarget),
			slog.F("desired_instances_expression", expression),
			slog.Error(err))
		return result
	}

	if err := p.evaluator.Validate(expression); err != nil {
		result.targetSource = "expression_fallback"
		result.expressionError = err.Error()
		p.logger.Error(context.Background(), "invalid desired instances expression",
			slog.F("preset_id", p.Preset.ID),
			slog.F("scheduled_target", scheduledTarget),
			slog.F("desired_instances_expression", expression),
			slog.Error(err))
		return result
	}

	desiredTarget, err := p.evaluator.Evaluate(expression, env)
	if err != nil {
		result.targetSource = "expression_fallback"
		result.expressionError = err.Error()
		p.logger.Error(context.Background(), "failed to evaluate desired instances expression",
			slog.F("preset_id", p.Preset.ID),
			slog.F("scheduled_target", scheduledTarget),
			slog.F("desired_instances_expression", expression),
			slog.Error(err))
		return result
	}

	result.desired = desiredTarget
	result.targetSource = "expression"
	return result
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
	counts := p.calculateBaseCounts()
	actual := safeNarrowInt64(int64(len(p.Running)) + int64(len(p.Expired)))
	resolvedTarget := p.resolveDesiredTarget(p.clock.Now())
	eligible := int32(0)
	extraneous := int32(0)
	if p.isActive() {
		eligible = counts.eligible
		extraneous = max(actual-counts.expired-resolvedTarget.desired, 0)
	}

	return &ReconciliationState{
		Actual:               actual,
		Expired:              counts.expired,
		Desired:              resolvedTarget.desired,
		ScheduledTarget:      resolvedTarget.scheduledTarget,
		TargetSource:         resolvedTarget.targetSource,
		ExpressionConfigured: resolvedTarget.expressionConfigured,
		ExpressionError:      resolvedTarget.expressionError,
		Eligible:             eligible,
		Extraneous:           extraneous,

		Starting: counts.starting,
		Stopping: counts.stopping,
		Deleting: counts.deleting,
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

	// If we have expired prebuilds, delete them.
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

	// If we still have more prebuilds than desired, delete the oldest ones.
	if state.Extraneous > 0 {
		actions = append(actions,
			&ReconciliationActions{
				ActionType: ActionTypeDelete,
				DeleteIDs:  p.getOldestPrebuildIDs(int(state.Extraneous)),
			})
	}

	// Number of running prebuilds excluding the recently deleted Expired.
	runningValid := state.Actual - state.Expired

	// Calculate how many new prebuilds we need to create.
	// We subtract starting prebuilds since they're already being created.
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
	// Cancel pending initial prebuild jobs from inactive version.
	if p.PendingCount > 0 {
		actions = append(actions,
			&ReconciliationActions{
				ActionType: ActionTypeCancelPending,
			})
	}

	// Delete prebuilds running in inactive version.
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
	// Sort by creation time, oldest first.
	slices.SortFunc(p.Running, func(a, b database.GetRunningPrebuiltWorkspacesRow) int {
		return a.CreatedAt.Compare(b.CreatedAt)
	})

	// Take the first N IDs.
	n = min(n, len(p.Running))
	ids := make([]uuid.UUID, n)
	for i := 0; i < n; i++ {
		ids[i] = p.Running[i].ID
	}

	return ids
}
