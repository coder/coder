package autochat

import (
	"context"
	"time"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/schedule/cron"
	"github.com/coder/quartz"
)

const (
	// cronTickInterval is how often we check for due cron automations.
	cronTickInterval = 30 * time.Second
)

// CronExecutor periodically checks for cron-triggered automations that
// are due and fires them.
type CronExecutor struct {
	ctx      context.Context
	db       database.Store
	executor *Executor
	log      slog.Logger
	clock    quartz.Clock
}

// NewCronExecutor creates a new cron executor.
func NewCronExecutor(
	ctx context.Context,
	db database.Store,
	executor *Executor,
	log slog.Logger,
	clock quartz.Clock,
) *CronExecutor {
	return &CronExecutor{
		ctx:      ctx,
		db:       db,
		executor: executor,
		log:      log.Named("autochat.cron"),
		clock:    clock,
	}
}

// Run starts the cron executor loop. It blocks until the context is
// cancelled.
func (c *CronExecutor) Run() {
	ticker := c.clock.NewTicker(cronTickInterval, "autochat", "cron")
	defer ticker.Stop()
	for {
		select {
		case <-c.ctx.Done():
			return
		case now := <-ticker.C:
			c.runDue(now)
		}
	}
}

// runDue checks all enabled cron automations and fires any that are due
// within the current tick window.
func (c *CronExecutor) runDue(now time.Time) {
	automations, err := c.db.GetEnabledCronChatAutomations(c.ctx)
	if err != nil {
		c.log.Error(c.ctx, "failed to get cron automations", slog.Error(err))
		return
	}

	window := now.Add(-cronTickInterval)
	for _, automation := range automations {
		if !automation.CronSchedule.Valid {
			continue
		}
		sched, err := cron.Weekly(automation.CronSchedule.String)
		if err != nil {
			c.log.Warn(c.ctx, "invalid cron schedule",
				slog.F("automation_id", automation.ID),
				slog.F("schedule", automation.CronSchedule.String),
				slog.Error(err),
			)
			continue
		}

		// Fire if the schedule's next occurrence after the window start
		// falls within [window, now].
		next := sched.Next(window)
		if next.After(now) {
			continue
		}

		templateData := map[string]any{
			"ScheduledAt": now.Format(time.RFC3339),
		}
		if _, err := c.executor.Fire(c.ctx, automation, nil, templateData); err != nil {
			c.log.Error(c.ctx, "failed to fire cron automation",
				slog.F("automation_id", automation.ID),
				slog.F("automation_name", automation.Name),
				slog.Error(err),
			)
		}
	}
}
