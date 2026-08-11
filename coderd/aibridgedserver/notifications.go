package aibridgedserver

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd/aibridge/budget"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/notifications"
	"github.com/coder/coder/v2/codersdk"
)

// warningThresholdPercent triggers a warning notification; limitThresholdPercent
// triggers the limit-reached notification (after which requests are blocked).
const (
	warningThresholdPercent = 85
	limitThresholdPercent   = 100
)

// budgetNotificationsCreatedBy records what enqueued AI budget notifications.
const budgetNotificationsCreatedBy = "aigateway"

// budgetThreshold pairs a percentage of the spend limit with the notifications
// sent when a user's spend crosses it.
type budgetThreshold struct {
	percent                   int
	userNotificationTemplate  uuid.UUID
	adminNotificationTemplate uuid.UUID
}

// budgetThresholds are the thresholds evaluated on every priced interception,
// ordered ascending. A single interception can cross more than one.
var budgetThresholds = []budgetThreshold{
	{
		percent:                   warningThresholdPercent,
		userNotificationTemplate:  notifications.TemplateAIBudgetWarningUser,
		adminNotificationTemplate: notifications.TemplateAIBudgetWarningAdmin,
	},
	{
		percent:                   limitThresholdPercent,
		userNotificationTemplate:  notifications.TemplateAIBudgetLimitReachedUser,
		adminNotificationTemplate: notifications.TemplateAIBudgetLimitReachedAdmin,
	},
}

// budgetThresholdCrossing describes a user crossing a budget threshold on a
// single interception. It carries only stable values so that if the same
// crossing is ever enqueued more than once, the payloads match and the
// duplicate is dropped.
type budgetThresholdCrossing struct {
	userID                    uuid.UUID
	effectiveGroupID          uuid.UUID
	spendLimitMicros          int64
	thresholdPercent          int
	userNotificationTemplate  uuid.UUID
	adminNotificationTemplate uuid.UUID
	limitSource               codersdk.AIBudgetLimitSource
	// periodStart and periodEnd bound the budget period [start, end) the
	// crossing occurred in.
	periodStart time.Time
	periodEnd   time.Time
}

// detectBudgetThresholdCrossings checks whether this interception's cost pushed
// the user's period spend across any budget thresholds, returning each one
// crossed. A single interception can cross several at once (e.g. straight past
// both the warning and limit thresholds).
//
// The period is derived from the interception's recorded time (the same
// timestamp the spend row is bucketed by) rather than the current wall clock,
// so an interception recorded near a period boundary but processed after it is
// evaluated against the period it belongs to.
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
				userID:                    intc.InitiatorID,
				effectiveGroupID:          cost.effectiveGroupID.UUID,
				spendLimitMicros:          limit,
				thresholdPercent:          t.percent,
				userNotificationTemplate:  t.userNotificationTemplate,
				adminNotificationTemplate: t.adminNotificationTemplate,
				limitSource:               cost.limitSource,
				periodStart:               period.Start,
				periodEnd:                 period.End,
			})
		}
	}
	return crossings, nil
}

// notifyBudgetThresholdCrossings enqueues user and admin notifications for the
// thresholds crossed by a single interception. All crossings share the same
// user and effective group, so the group, username, and admin recipients are
// resolved once.
func (s *Server) notifyBudgetThresholdCrossings(ctx context.Context, crossings []budgetThresholdCrossing) error {
	if len(crossings) == 0 {
		return nil
	}

	userID := crossings[0].userID
	effectiveGroupID := crossings[0].effectiveGroupID

	group, err := s.store.GetGroupByID(ctx, effectiveGroupID)
	if err != nil {
		return xerrors.Errorf("look up group %q: %w", effectiveGroupID, err)
	}
	user, err := s.store.GetUserByID(ctx, userID)
	if err != nil {
		return xerrors.Errorf("look up user %q: %w", userID, err)
	}
	admins, err := s.budgetNotificationAdmins(ctx, userID)
	if err != nil {
		return xerrors.Errorf("look up budget notification admins: %w", err)
	}

	//nolint:gocritic // Enqueuing notifications requires the notifier actor.
	notifCtx := dbauthz.AsNotifier(ctx)

	var errs []error
	for _, c := range crossings {
		labels := map[string]string{
			"threshold":            strconv.Itoa(c.thresholdPercent),
			"limit":                formatSpendLimit(c.spendLimitMicros),
			"period":               s.budgetPeriod.Adjective(),
			"limit_source":         string(c.limitSource),
			"username":             user.Username,
			"effective_group_name": group.Name,
			// Both bounds carry the year so a period straddling a year boundary
			// (e.g. December 1, 2026 - January 1, 2027) is unambiguous.
			"period_start": c.periodStart.UTC().Format("January 2, 2006"),
			"period_end":   c.periodEnd.UTC().Format("January 2, 2006"),
		}

		// Notify the user who crossed the threshold.
		if _, err := s.notifEnqueuer.EnqueueWithData(notifCtx, userID, c.userNotificationTemplate,
			labels, nil, budgetNotificationsCreatedBy, effectiveGroupID); err != nil {
			errs = append(errs, xerrors.Errorf("enqueue user notification (threshold %d%%): %w", c.thresholdPercent, err))
		}

		// Notify admins, naming the affected user.
		for _, admin := range admins {
			if _, err := s.notifEnqueuer.EnqueueWithData(notifCtx, admin.ID, c.adminNotificationTemplate,
				labels, nil, budgetNotificationsCreatedBy, userID, effectiveGroupID); err != nil {
				errs = append(errs, xerrors.Errorf("enqueue admin notification (threshold %d%%, admin %q): %w", c.thresholdPercent, admin.ID, err))
			}
		}
	}
	return errors.Join(errs...)
}

// budgetNotificationAdmins returns the users who should receive admin budget
// notifications: deployment owners and user admins, excluding the affected
// user, who receives the user-facing notification instead.
func (s *Server) budgetNotificationAdmins(ctx context.Context, excludeUserID uuid.UUID) ([]database.GetUsersRow, error) {
	admins, err := s.store.GetUsers(ctx, database.GetUsersParams{
		RbacRole: []string{codersdk.RoleOwner, codersdk.RoleUserAdmin},
	})
	if err != nil {
		return nil, err
	}

	recipients := make([]database.GetUsersRow, 0, len(admins))
	for _, admin := range admins {
		if admin.ID == excludeUserID {
			continue
		}
		recipients = append(recipients, admin)
	}
	return recipients, nil
}

// formatSpendLimit renders a spend limit as a USD string.
func formatSpendLimit(micros int64) string {
	return fmt.Sprintf("$%.2f", float64(micros)/1_000_000)
}
