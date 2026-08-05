package aibridgedserver

import (
	"context"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"golang.org/x/xerrors"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/coderd/aibridge/budget"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/quartz"
)

// blockedUsersRefreshInterval is the default cadence for recomputing the
// blocked_users gauge.
const blockedUsersRefreshInterval = 5 * time.Minute

// Metrics holds the AI budget cost-control metrics emitted by the aibridged
// server.
type Metrics struct {
	// Requests blocked because the initiator's AI budget was exceeded.
	BlockedRequests *prometheus.CounterVec
	// Users currently over their AI budget. Updated periodically.
	BlockedUsers *prometheus.GaugeVec
	// Recorded token-usage records for which no model price was found.
	UnpricedTokenUsageRecords *prometheus.CounterVec
	// Duration of budget enforcement checks.
	EnforcementDuration *prometheus.HistogramVec
}

// NewMetrics creates and registers metrics. It will panic if a collector has
// already been registered. The provided registerer may specify a namespace
// prefix using [prometheus.WrapRegistererWithPrefix].
func NewMetrics(reg prometheus.Registerer) *Metrics {
	return &Metrics{
		// Pessimistic cardinality: one series per group, bounded per deployment.
		BlockedRequests: promauto.With(reg).NewCounterVec(prometheus.CounterOpts{
			Subsystem: "cost_control",
			Name:      "blocked_requests_total",
			Help:      "The number of AI requests blocked because the initiator's budget was exceeded.",
		}, []string{"group_id"}),
		// Pessimistic cardinality: one series per group with an over-budget user.
		BlockedUsers: promauto.With(reg).NewGaugeVec(prometheus.GaugeOpts{
			Subsystem: "cost_control",
			Name:      "blocked_users",
			Help:      "The number of users currently over their AI budget.",
		}, []string{"group_id"}),
		// Pessimistic cardinality: one series per configured provider and model.
		UnpricedTokenUsageRecords: promauto.With(reg).NewCounterVec(prometheus.CounterOpts{
			Subsystem: "cost_control",
			Name:      "unpriced_token_usage_records_total",
			Help:      "The number of recorded AI token-usage records for which no (provider, model) price was found.",
		}, []string{"provider", "model"}),
		// Pessimistic cardinality: 3 outcomes, 8 buckets + 3 extra series
		// (count, sum, +Inf) = up to 33.
		EnforcementDuration: promauto.With(reg).NewHistogramVec(prometheus.HistogramOpts{
			Subsystem: "cost_control",
			Name:      "enforcement_duration_seconds",
			Help: "The duration of AI budget enforcement checks, in seconds " +
				"(outcome: allowed, blocked, error).",
			Buckets:                         []float64{0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1},
			NativeHistogramBucketFactor:     1.1,
			NativeHistogramMaxBucketNumber:  100,
			NativeHistogramMinResetDuration: time.Hour,
			NativeHistogramZeroThreshold:    0,
			NativeHistogramMaxZeroThreshold: 0,
		}, []string{"outcome"}),
	}
}

// StartBlockedUsersCollector periodically updates the blocked_users gauge from
// the database until ctx is canceled. A non-positive interval uses the 5m
// default. The returned function stops the collector and waits for it to exit.
// It is a no-op returning a no-op closer when m is nil, so callers need not
// nil-check.
func (m *Metrics) StartBlockedUsersCollector(ctx context.Context, logger slog.Logger, clk quartz.Clock, db database.Store, budgetPeriod codersdk.AIBudgetPeriod, interval time.Duration) func() {
	if m == nil {
		return func() {}
	}
	if interval <= 0 {
		interval = blockedUsersRefreshInterval
	}

	ctx, cancel := context.WithCancel(ctx)
	done := make(chan struct{})
	ticker := clk.NewTicker(interval)
	go func() {
		defer close(done)
		defer ticker.Stop()
		// Update immediately so the gauge is populated at startup rather than
		// absent for a full interval.
		for {
			if err := m.updateBlockedUsers(ctx, clk, db, budgetPeriod); err != nil {
				logger.Error(ctx, "update blocked_users gauge", slog.Error(err))
			}
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
			}
		}
	}()
	return func() {
		cancel()
		<-done
	}
}

// updateBlockedUsers sets the blocked_users gauge to the current per-group
// count of users at or over their AI budget for the active period.
func (m *Metrics) updateBlockedUsers(ctx context.Context, clk quartz.Clock, db database.Store, budgetPeriod codersdk.AIBudgetPeriod) error {
	period, err := budget.CurrentPeriod(clk.Now(), budgetPeriod)
	if err != nil {
		return xerrors.Errorf("compute AI budget period: %w", err)
	}
	//nolint:gocritic // Cost-control metrics need deployment-wide access to
	// group budgets and user spend.
	rows, err := db.GetOverBudgetUsersPerGroup(dbauthz.AsSystemRestricted(ctx), period.Start)
	if err != nil {
		return xerrors.Errorf("get over-budget users per group: %w", err)
	}

	// Reset clears groups that dropped to zero since the last cycle so their
	// stale series do not linger.
	m.BlockedUsers.Reset()
	for _, row := range rows {
		m.BlockedUsers.WithLabelValues(row.GroupID.String()).Set(float64(row.OverBudgetUsers))
	}
	return nil
}
