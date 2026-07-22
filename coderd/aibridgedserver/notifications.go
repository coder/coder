package aibridgedserver

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd/aibridge/budget"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/notifications"
)

// warningThresholdPercent triggers a warning notification; limitThresholdPercent
// triggers the limit-reached notification (after which requests are blocked).
const (
	warningThresholdPercent = 85
	limitThresholdPercent   = 100
)

// budgetNotificationsCreatedBy records what enqueued AI budget notifications.
const budgetNotificationsCreatedBy = "aigateway"

// budgetThreshold pairs a percentage of the spend limit with the notification
// template sent when a user's spend crosses it.
type budgetThreshold struct {
	percent  int
	template uuid.UUID
}

// budgetThresholds are the thresholds evaluated on every priced interception,
// ordered ascending. A single interception can cross more than one.
var budgetThresholds = []budgetThreshold{
	{percent: warningThresholdPercent, template: notifications.TemplateAIBudgetWarningUser},
	{percent: limitThresholdPercent, template: notifications.TemplateAIBudgetLimitReachedUser},
}

// budgetThresholdCrossing describes a user crossing a budget threshold on a
// single interception. It carries only stable values so that if the same
// crossing is ever enqueued more than once, the payloads match and the
// duplicate is dropped.
type budgetThresholdCrossing struct {
	userID           uuid.UUID
	groupID          uuid.UUID
	spendLimitMicros int64
	thresholdPercent int
	template         uuid.UUID
}

// detectBudgetThresholdCrossings checks whether this interception's cost pushed
// the user's period spend across any budget thresholds, returning each one
// crossed. A single interception can cross several at once (e.g. straight past
// both the warning and limit thresholds).
//
// The period is derived from createdAt, the same timestamp the spend row is
// bucketed by. Using it (rather than the current wall clock) keeps the summed
// window and the incremented row in the same period, so an interception whose
// createdAt and processing time span across a period boundary is evaluated
// against the period it was recorded in.
func (s *Server) detectBudgetThresholdCrossings(ctx context.Context, tx database.Store, intc database.AIBridgeInterception, cost tokenUsageCost, createdAt time.Time) ([]budgetThresholdCrossing, error) {
	if !cost.effectiveGroupID.Valid || !cost.spendLimitMicros.Valid || cost.spendLimitMicros.Int64 <= 0 {
		return nil, nil
	}

	period, err := budget.CurrentPeriod(createdAt, s.budgetPeriod)
	if err != nil {
		return nil, xerrors.Errorf("compute AI budget period: %w", err)
	}

	spend, err := tx.GetUserAISpendSince(ctx, database.GetUserAISpendSinceParams{
		UserID:           intc.InitiatorID,
		EffectiveGroupID: cost.effectiveGroupID.UUID,
		PeriodStart:      period.Start,
	})
	if err != nil {
		return nil, xerrors.Errorf("get user AI spend for user %q in group %q: %w", intc.InitiatorID, cost.effectiveGroupID.UUID, err)
	}

	limit := cost.spendLimitMicros.Int64
	newSpend := spend.SpendMicros
	// Pre-interception total is the current total minus this interception's cost.
	oldSpend := newSpend - cost.costMicros.Int64

	var crossings []budgetThresholdCrossing
	for _, t := range budgetThresholds {
		at := limit * int64(t.percent) / 100
		if oldSpend < at && newSpend >= at {
			crossings = append(crossings, budgetThresholdCrossing{
				userID:           intc.InitiatorID,
				groupID:          cost.effectiveGroupID.UUID,
				spendLimitMicros: limit,
				thresholdPercent: t.percent,
				template:         t.template,
			})
		}
	}
	return crossings, nil
}

// notifyBudgetThresholdCrossing enqueues the notification for the user who
// crossed the threshold.
func (s *Server) notifyBudgetThresholdCrossing(ctx context.Context, crossing budgetThresholdCrossing) error {
	group, err := s.store.GetGroupByID(ctx, crossing.groupID)
	if err != nil {
		return xerrors.Errorf("look up group %q: %w", crossing.groupID, err)
	}

	labels := map[string]string{
		"threshold":  strconv.Itoa(crossing.thresholdPercent),
		"limit":      formatSpendLimit(crossing.spendLimitMicros),
		"group_name": group.Name,
	}

	//nolint:gocritic // Enqueuing notifications requires the notifier actor.
	if _, err := s.notifEnqueuer.EnqueueWithData(dbauthz.AsNotifier(ctx), crossing.userID, crossing.template,
		labels, nil, budgetNotificationsCreatedBy,
		crossing.groupID,
	); err != nil {
		return xerrors.Errorf("enqueue notification: %w", err)
	}
	return nil
}

// formatSpendLimit renders a spend limit as a USD string.
func formatSpendLimit(micros int64) string {
	return fmt.Sprintf("$%.2f", float64(micros)/1_000_000)
}
