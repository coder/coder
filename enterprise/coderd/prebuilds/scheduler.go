package prebuilds

import (
	"context"

	"github.com/google/uuid"
	"github.com/hashicorp/go-multierror"
	"golang.org/x/xerrors"

	"cdr.dev/slog"

	"github.com/coder/coder/v2/coderd/prebuilds"
)

// TODO: configuration value or calculated based on the total number of daemons?
//   Should the prebuild scheduler be aware of the number of daemons?
//   If configurable we can define a value (e.g., 0) to not limit the number of actions
const maxPendingPrebuildActions = 2

// PresetActions is a batch of actions computed for a single preset.
type PresetActions struct {
	Preset  prebuilds.PresetSnapshot
	Actions []*prebuilds.ReconciliationActions
}

// JobsScheduler executes reconciliation actions by scheduling the required provisioner jobs
type JobsScheduler struct {
	logger slog.Logger
	// Remaining provisioner jobs this reconciliation loop may execute; decremented per executed action.
	remainingActions int
	exec             ActionExecutor
}

type ActionExecutor func(context.Context, slog.Logger, prebuilds.PresetSnapshot, *prebuilds.ReconciliationActions) error

func NewJobsScheduler(logger slog.Logger, pending int, exec ActionExecutor) (*JobsScheduler, error) {
	// TODO: if pending == 0 should we consider this unlimited (same as before)

	if pending >= maxPendingPrebuildActions {
		return nil, xerrors.Errorf("pending jobs exceed max pending prebuild actions")
	}
	return &JobsScheduler{
		logger:           logger,
		remainingActions: maxPendingPrebuildActions - pending,
		exec:             exec,
	}, nil
}

// Run executes all actions for all presets by calling exec for each action.
func (s *JobsScheduler) Run(
	ctx context.Context,
	presetActions []PresetActions,
) error {
	var multiErr multierror.Error

	// TODO: This could probably be done concurrently
	for _, item := range presetActions {
		for _, action := range item.Actions {

			// TODO: a ReconciliationActions can correspond to multiple actions (multiple create or multiple delete)
			//   Goroutines should return multiple unit actions, instead of a set of actions where each one can correspond to a set
			//   By doing this change, there is also a higher change for a distribution of actions of different presets
			switch {
			case action.ActionType == prebuilds.ActionTypeDelete:
				for _, deleteID := range action.DeleteIDs {
					if s.remainingActions <= 0 {
						s.logger.Info(ctx, "no more allowed actions, stopping job scheduling")
						return multiErr.ErrorOrNil()
					}
					// Create unit action
					singleDeleteAction := &prebuilds.ReconciliationActions{
						ActionType: prebuilds.ActionTypeDelete,
						DeleteIDs:  []uuid.UUID{deleteID},
					}
					if err := s.exec(ctx, s.logger, item.Preset, singleDeleteAction); err != nil {
						s.logger.Error(ctx, "failed to execute scheduled action", slog.Error(err))
						multiErr.Errors = append(multiErr.Errors, err)
					}
					// Count this action against the budget
					s.remainingActions--
				}
			case action.ActionType == prebuilds.ActionTypeCreate:
				for range action.Create {
					if s.remainingActions <= 0 {
						s.logger.Info(ctx, "no more allowed actions, stopping job scheduling")
						return multiErr.ErrorOrNil()
					}
					// Create unit action
					singleCreateAction := &prebuilds.ReconciliationActions{
						ActionType: prebuilds.ActionTypeCreate,
						Create:     1,
					}
					if err := s.exec(ctx, s.logger, item.Preset, singleCreateAction); err != nil {
						s.logger.Error(ctx, "failed to execute scheduled action", slog.Error(err))
						multiErr.Errors = append(multiErr.Errors, err)
					}
					// Count this action against the budget
					s.remainingActions--
				}
			default:
				continue
			}
		}
	}

	s.logger.Info(ctx, "all actions successfully scheduled")

	return multiErr.ErrorOrNil()
}
