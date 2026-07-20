package aibridgedserver

import (
	"context"
	"fmt"
	"strconv"

	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd/aibridge/budget"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/notifications"
)

// warningThresholdPercent is the percentage of the spend limit that triggers a
// warning notification.
const warningThresholdPercent = 90

// budgetNotificationsCreatedBy records what enqueued AI budget notifications.
const budgetNotificationsCreatedBy = "aigateway"

// budgetThresholdCrossing describes a user crossing a budget threshold on a
// single interception. It carries only stable values so that if the same
// crossing is ever enqueued more than once, the payloads match and the
// duplicate is dropped.
type budgetThresholdCrossing struct {
	userID           uuid.UUID
	groupID          uuid.UUID
	spendLimitMicros int64
	thresholdPercent int
}

// detectBudgetThresholdCrossing reports whether recording this interception's
// cost pushed the user's period spend across the warning threshold.
func (s *Server) detectBudgetThresholdCrossing(ctx context.Context, tx database.Store, intc database.AIBridgeInterception, cost tokenUsageCost) (budgetThresholdCrossing, bool, error) {
	if !cost.effectiveGroupID.Valid || !cost.spendLimitMicros.Valid || cost.spendLimitMicros.Int64 <= 0 {
		return budgetThresholdCrossing{}, false, nil
	}

	period, err := budget.CurrentPeriod(s.clock.Now(), s.budgetPeriod)
	if err != nil {
		return budgetThresholdCrossing{}, false, xerrors.Errorf("compute AI budget period: %w", err)
	}

	spend, err := tx.GetUserAISpendSince(ctx, database.GetUserAISpendSinceParams{
		UserID:           intc.InitiatorID,
		EffectiveGroupID: cost.effectiveGroupID.UUID,
		PeriodStart:      period.Start,
	})
	if err != nil {
		return budgetThresholdCrossing{}, false, xerrors.Errorf("get user AI spend for user %q in group %q: %w", intc.InitiatorID, cost.effectiveGroupID.UUID, err)
	}

	limit := cost.spendLimitMicros.Int64
	newSpend := spend.SpendMicros
	// Pre-interception total is the current total minus this interception's cost.
	oldSpend := newSpend - cost.costMicros.Int64

	warnAt := limit * warningThresholdPercent / 100
	if oldSpend < warnAt && newSpend >= warnAt {
		return budgetThresholdCrossing{
			userID:           intc.InitiatorID,
			groupID:          cost.effectiveGroupID.UUID,
			spendLimitMicros: limit,
			thresholdPercent: warningThresholdPercent,
		}, true, nil
	}
	return budgetThresholdCrossing{}, false, nil
}

// notifyBudgetThresholdCrossing enqueues the warning notification for the user
// who crossed the threshold.
func (s *Server) notifyBudgetThresholdCrossing(ctx context.Context, crossing budgetThresholdCrossing) error {
	//nolint:gocritic // The interception context is scoped to AI Bridge; reading the group and enqueuing need system access.
	sysCtx := dbauthz.AsSystemRestricted(ctx)

	group, err := s.store.GetGroupByID(sysCtx, crossing.groupID)
	if err != nil {
		return xerrors.Errorf("look up group %q: %w", crossing.groupID, err)
	}

	labels := map[string]string{
		"threshold":  strconv.Itoa(crossing.thresholdPercent),
		"limit":      formatSpendLimit(crossing.spendLimitMicros),
		"group_name": group.Name,
	}

	if _, err := s.notifEnqueuer.EnqueueWithData(sysCtx, crossing.userID, notifications.TemplateAIBudgetWarningUser,
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
